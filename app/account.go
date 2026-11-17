package main

// NEXUS Account: sign-in to the NEXUS account service (accountd/), linking devices with a
// one-time code, and synchronisation of NEXUS data between the account's devices.
//
// Local first: nothing in NEXUS needs an account or a connection. Changes are always made
// locally; synchronisation compares each item with the version both sides last agreed on
// (three-way), applies one-sided changes, uploads with the server version it is based on (the
// server refuses an upload based on an older version), and keeps real conflicts until the user
// chooses. Offline changes stay pending until the service can be reached.
//
// What synchronises (each category can be switched off):
//   profile   users: name, picture, colours and theme (not the wallpaper file), controller layout
//   library   per game: display name, hidden, artwork choice, the game's own controller layout
//   prefs     NEXUS preferences: sounds, start-up animation, shortcut keys, Home behaviour
//   emulation global and per-game emulation settings (off by default: devices differ)
//   amiibo    the amiibo library's details and owners, and NEXUS virtual amiibo data
// Never: games, updates, keys, firmware, imported amiibo dumps, shader caches, media, and
// (for now) game saves.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// defaultAccountServer is the NEXUS account service address built into release builds. It is
// empty until the service is deployed; then users only choose "Create account" or "Sign in".
var defaultAccountServer = ""

var SyncCategories = []string{"profile", "playtime", "library", "prefs", "emulation", "amiibo"}

func defaultSyncCategories() map[string]bool {
	return map[string]bool{"profile": true, "playtime": true, "library": true, "prefs": true, "emulation": false, "amiibo": true}
}

type syncBase struct {
	Version int64  `json:"v"`
	Hash    string `json:"h"`
}

type AccountConflict struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Kind       string `json:"kind"` // change | user
	Title      string `json:"title"`
	Local      []byte `json:"local,omitempty"`
	Remote     []byte `json:"remote,omitempty"`
	RemoteVer  int64  `json:"remoteVersion"`
	RemoteDel  bool   `json:"remoteDeleted,omitempty"`
	RemoteFrom string `json:"remoteFrom"`
	At         int64  `json:"at"`
}

type SyncEvent struct {
	At      int64  `json:"at"`
	Kind    string `json:"kind"` // synced | failed | conflict | signin | signout | linked
	Message string `json:"message"`
}

type accountState struct {
	Server      string              `json:"server,omitempty"`
	AccountID   string              `json:"accountId,omitempty"`
	Email       string              `json:"email,omitempty"`
	DisplayName string              `json:"displayName,omitempty"`
	DeviceID    string              `json:"deviceId,omitempty"`
	DeviceName  string              `json:"deviceName,omitempty"`
	SignedIn    bool                `json:"signedIn"`
	Ended       string              `json:"ended,omitempty"` // why the service signed this device out
	SyncOn      bool                `json:"syncOn"`
	Categories  map[string]bool     `json:"categories"`
	Cursor      int64               `json:"cursor"`
	Base        map[string]syncBase `json:"base"`
	ProfileMap  map[string]string   `json:"profileMap,omitempty"` // account user id -> local profile id
	Conflicts   []AccountConflict   `json:"conflicts,omitempty"`
	History     []SyncEvent         `json:"history,omitempty"`
	LastSync    int64               `json:"lastSync,omitempty"`
	LastError   string              `json:"lastError,omitempty"`
	Offline     bool                `json:"offline,omitempty"`
	Devices     []accountDevice     `json:"devices,omitempty"`
	OnboardSeen bool                `json:"onboardSeen,omitempty"`
	Provider    string              `json:"provider,omitempty"` // github | server
	GistID      string              `json:"gistId,omitempty"`
	// HoldUploads: signed in to a different account than before on this device. Downloads go
	// on; this device's data is not sent to the new account until the user chooses (so data
	// that came from the other account is never copied into this one by accident).
	HoldUploads bool   `json:"holdUploads,omitempty"`
	HoldFrom    string `json:"holdFrom,omitempty"` // the account signed in before
	// TokenUnread: the stored sign-in could not be read at start; retried before giving up
	TokenUnread int                        `json:"tokenUnread,omitempty"`
	Remote      map[string]json.RawMessage `json:"-"`
}

type accountDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Created  int64  `json:"created"`
	LastSeen int64  `json:"lastSeen"`
	Current  bool   `json:"current"`
}

type remoteItem struct {
	Key        string `json:"key"`
	Version    int64  `json:"version"`
	Rev        int64  `json:"rev"`
	Deleted    bool   `json:"deleted,omitempty"`
	Data       []byte `json:"data,omitempty"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Updated    int64  `json:"updated"`
}

type sessionResult struct {
	Token        string `json:"token"`
	DeviceID     string `json:"deviceId"`
	RecoveryCode string `json:"recoveryCode,omitempty"`
	Account      struct {
		ID          string          `json:"id"`
		Email       string          `json:"email"`
		DisplayName string          `json:"displayName"`
		Devices     []accountDevice `json:"devices"`
	} `json:"account"`
}

// Account is the NEXUS Account subsystem.
type Account struct {
	app   *App
	dir   string
	mu    sync.Mutex
	st    accountState
	token string
	// GitHub's short-lived sign-in (OAuth apps since August 2026): the access token lasts
	// eight hours and the refresh token renews it (account_github_cred.go). Empty for a
	// token that does not expire.
	refresh     string
	tokExp      int64 // unix seconds; 0 = does not expire
	refreshExp  int64
	refreshMu   sync.Mutex // one renewal at a time (GitHub rotates the refresh token)
	renewing    bool       // a renewal is in progress (status: Reconnecting)
	credUnsaved bool       // a renewed sign-in is only in memory so far (saving failed)
	syncMu      sync.Mutex // one synchronisation at a time
	busy        bool
	claim       *linkClaim // this device is waiting for approval of a linking code
	client      *http.Client
	wake        chan struct{}
	stop        chan struct{}
	touch       *time.Timer
	ghFlow      *ghFlow // GitHub sign-in in progress
}

type linkClaim struct {
	Server, ID, Secret, Account, DisplayName string
	Expires                                  int64
}

func NewAccount(a *App) *Account {
	ac := &Account{app: a, dir: filepath.Join(a.Paths.System, "Account"), client: &http.Client{Timeout: 25 * time.Second},
		wake: make(chan struct{}, 1), stop: make(chan struct{})}
	_ = os.MkdirAll(ac.dir, 0o700)
	_ = readJSON(filepath.Join(ac.dir, "account.json"), &ac.st)
	if ac.st.Categories == nil {
		ac.st.Categories = defaultSyncCategories()
	}
	if ac.st.Base == nil {
		ac.st.Base = map[string]syncBase{}
	}
	if ac.st.SignedIn {
		ac.loadTokenLocked()
	}
	a.markStartup("account restored", ac.restoredSummary())
	return ac
}

// restoredSummary: how the account came back at startup, for the log (no names or tokens).
func (ac *Account) restoredSummary() string {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	switch {
	case ac.st.SignedIn && ac.token != "" && ac.refresh != "":
		return ac.st.Provider + ", connected, renewable sign-in"
	case ac.st.SignedIn && ac.token != "":
		return ac.st.Provider + ", connected"
	case ac.st.SignedIn:
		return ac.st.Provider + ", stored sign-in not readable yet"
	case ac.st.Ended != "":
		return "not connected: the previous sign-in ended"
	}
	return "not connected"
}

func (ac *Account) tokenPath() string { return filepath.Join(ac.dir, "session.bin") }

// noteAccountSwitchLocked: signing in to another account than the one this device used before
// holds this device's uploads until the user chooses (ReleaseHold), so data that came from the
// previous account is not copied into the new one by accident. The same account again, or a
// device's first account, starts normally.
func (ac *Account) noteAccountSwitchLocked(newID, newName string) {
	if ac.st.AccountID != "" && ac.st.AccountID != newID {
		ac.st.HoldUploads, ac.st.HoldFrom = true, firstNonEmpty(ac.st.Email, ac.st.DisplayName, "another account")
		ac.noteLocked("held", fmt.Sprintf("Signed in to %s, a different account than before (%s): this device's data is not sent until you choose.", newName, ac.st.HoldFrom))
	} else if ac.st.AccountID == newID {
		ac.st.HoldUploads, ac.st.HoldFrom = false, ""
	}
}

// ReleaseHold ends the hold after an account switch: send=true lets this device's data go to
// the new account; false keeps only what the account already has (this device's own users and
// settings stay on it, unsent; new local changes are sent from now on).
func (ac *Account) ReleaseHold(send bool) error {
	ac.mu.Lock()
	if !ac.st.HoldUploads {
		ac.mu.Unlock()
		return nil
	}
	if !send {
		// what is local now counts as agreed: only later changes are sent
		ac.mu.Unlock()
		cats, pm, _ := ac.snapshot()
		local := ac.localItems(cats, pm)
		ac.mu.Lock()
		for k, b := range local {
			if base, ok := ac.st.Base[k]; !ok || base.Hash != hashBytes(b) {
				ac.st.Base[k] = syncBase{Version: base.Version, Hash: hashBytes(b)}
			}
		}
	}
	ac.st.HoldUploads, ac.st.HoldFrom = false, ""
	ac.noteLocked("synced", map[bool]string{true: "This device's data will be sent to the account.", false: "This device keeps its own data; only later changes are sent."}[send])
	ac.saveLocked()
	ac.mu.Unlock()
	ac.changed()
	ac.Kick()
	return nil
}

// loadTokenLocked reads the stored sign-in. A missing or empty file ends the session. A file
// that cannot be read right now (another program has it open, a passing data-protection error)
// keeps the device signed in and is tried again at each synchronisation, up to five times,
// before NEXUS asks to sign in again: one failed read used to sign the device out for good.
func (ac *Account) loadTokenLocked() bool {
	b, err := loadSecretFile(ac.tokenPath())
	if err == nil && len(b) > 0 {
		ac.setCredLocked(decodeCred(b))
		ac.st.TokenUnread = 0
		return true
	}
	if err == nil || errors.Is(err, os.ErrNotExist) || ac.st.TokenUnread >= 5 {
		ac.st.SignedIn, ac.st.Ended, ac.st.TokenUnread = false, "The sign-in on this device could not be read. Sign in again.", 0
		return false
	}
	ac.st.TokenUnread++
	ac.st.LastError = "The sign-in on this device could not be read just now; NEXUS tries again."
	return false
}

func (ac *Account) saveLocked() {
	if err := writeJSONAtomic(filepath.Join(ac.dir, "account.json"), ac.st); err != nil {
		ac.app.Log.Printf("account: %v", err)
	}
}

func (ac *Account) noteLocked(kind, msg string) {
	ac.st.History = append(ac.st.History, SyncEvent{At: nowUnix(), Kind: kind, Message: msg})
	if n := len(ac.st.History); n > 50 {
		ac.st.History = ac.st.History[n-50:]
	}
}

func (ac *Account) changed() { ac.app.Events.Emit("account.changed", ac.Status()) }

// Server is the account service address in use ("" = not available in this build).
func (ac *Account) Server() string {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.serverLocked()
}

func (ac *Account) serverLocked() string {
	if ac.st.Server != "" {
		return ac.st.Server
	}
	if s := os.Getenv("NEXUS_ACCOUNT_SERVER"); s != "" {
		return strings.TrimRight(s, "/")
	}
	return defaultAccountServer
}

// checkServerURL: https only (http for this computer, for testing a local service).
func checkServerURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" {
		return "", errors.New("Enter the account service address, for example https://accounts.example.org.")
	}
	host := u.Hostname()
	local := host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errors.New("The account service must use a secure address (https://).")
	}
	return raw, nil
}

// SetServer chooses the account service (only while signed out).
func (ac *Account) SetServer(raw string) error {
	s, err := checkServerURL(raw)
	if err != nil {
		return err
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.st.SignedIn {
		return errors.New("Sign out before changing the account service.")
	}
	ac.st.Server = s
	ac.saveLocked()
	return nil
}

// AccountStatus is what the interface shows.
type AccountStatus struct {
	Available    bool              `json:"available"` // an account service is configured
	Server       string            `json:"server"`
	Custom       bool              `json:"custom"`
	SignedIn     bool              `json:"signedIn"`
	Email        string            `json:"email,omitempty"`
	DisplayName  string            `json:"displayName,omitempty"`
	DeviceID     string            `json:"deviceId,omitempty"`
	DeviceName   string            `json:"deviceName"`
	Platform     string            `json:"platform"`
	Ended        string            `json:"ended,omitempty"`
	SyncOn       bool              `json:"syncOn"`
	State        string            `json:"state"` // off | synced | syncing | offline | pending | failed | conflict | held | signedout | unavailable
	HoldUploads  bool              `json:"holdUploads,omitempty"`
	HoldFrom     string            `json:"holdFrom,omitempty"`
	Pending      int               `json:"pending"`
	LastSync     int64             `json:"lastSync,omitempty"`
	LastError    string            `json:"lastError,omitempty"`
	Categories   map[string]bool   `json:"categories"`
	Conflicts    []AccountConflict `json:"conflicts"`
	History      []SyncEvent       `json:"history"`
	Devices      []accountDevice   `json:"devices"`
	Linking      *linkClaimView    `json:"linking,omitempty"`
	Provider     string            `json:"provider"`         // github (default) | server (self-hosted NEXUS account service)
	GitHub       map[string]any    `json:"github,omitempty"` // sign-in code while signing in with GitHub
	OnboardSeen  bool              `json:"onboardSeen"`
	TokenStorage string            `json:"tokenStorage"`
	// Connection is the GitHub (or service) connection on its own, apart from synchronisation:
	// connected | notconnected | reconnecting (renewing the sign-in) | expired (GitHub refused
	// it: reconnect) | problem (cannot reach the service just now; still connected) | syncing
	Connection string `json:"connection"`
	Renewable  bool   `json:"renewable,omitempty"` // the sign-in renews itself (GitHub's eight-hour tokens)
}

type linkClaimView struct {
	Account     string `json:"account"`
	DisplayName string `json:"displayName"`
	Expires     int64  `json:"expires"`
}

func (ac *Account) Status() AccountStatus {
	pending := 0
	ac.mu.Lock()
	signed, on := ac.st.SignedIn, ac.st.SyncOn
	ac.mu.Unlock()
	if signed && on {
		pending = len(ac.pendingKeys())
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	st := AccountStatus{Server: ac.serverLocked(), Custom: ac.st.Server != "", SignedIn: ac.st.SignedIn, Email: ac.st.Email, DisplayName: ac.st.DisplayName,
		DeviceID: ac.st.DeviceID, DeviceName: firstNonEmpty(ac.st.DeviceName, defaultDeviceName()), Platform: devicePlatform(), Ended: ac.st.Ended,
		SyncOn: ac.st.SyncOn, Pending: pending, LastSync: ac.st.LastSync, LastError: ac.st.LastError, Categories: ac.st.Categories,
		Conflicts: append([]AccountConflict{}, ac.st.Conflicts...), History: append([]SyncEvent{}, ac.st.History...),
		Devices: append([]accountDevice{}, ac.st.Devices...), OnboardSeen: ac.st.OnboardSeen, TokenStorage: secretStorageName,
		HoldUploads: ac.st.HoldUploads, HoldFrom: ac.st.HoldFrom}
	st.Available = st.Server != "" || githubClientID != ""
	st.Provider = "server"
	if ac.st.SignedIn && ac.st.Provider == "github" || !ac.st.SignedIn && st.Server == "" {
		st.Provider = "github"
	}
	if f := ac.ghFlow; f != nil {
		st.GitHub = map[string]any{"userCode": f.UserCode, "uri": f.URI, "expires": f.Expires.Unix()}
	}
	for i := range st.Conflicts { // the interface needs the titles, not the data
		st.Conflicts[i].Local, st.Conflicts[i].Remote = nil, nil
	}
	if ac.claim != nil {
		st.Linking = &linkClaimView{ac.claim.Account, ac.claim.DisplayName, ac.claim.Expires}
	}
	st.Renewable = ac.refresh != ""
	switch {
	case !st.SignedIn && st.Ended != "" && ac.st.AccountID != "":
		st.Connection = "expired"
	case !st.SignedIn:
		st.Connection = "notconnected"
	case ac.renewing:
		st.Connection = "reconnecting"
	case ac.busy:
		st.Connection = "syncing"
	case ac.st.Offline:
		st.Connection = "problem"
	default:
		st.Connection = "connected"
	}
	switch {
	case !st.Available && !st.SignedIn:
		st.State = "unavailable"
	case !st.SignedIn:
		st.State = "signedout"
	case !st.SyncOn:
		st.State = "off"
	case ac.busy:
		st.State = "syncing"
	case len(st.Conflicts) > 0:
		st.State = "conflict"
	case ac.st.HoldUploads:
		st.State = "held"
	case ac.st.Offline:
		st.State = "offline"
	case st.LastError != "":
		st.State = "failed"
	case pending > 0:
		st.State = "pending"
	default:
		st.State = "synced"
	}
	return st
}

func syncOutcome(err error) string {
	if errors.Is(err, errOffline) {
		return "offline; retried later"
	}
	return "failed"
}

func devicePlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows", "linux", "android":
		return runtime.GOOS
	}
	return "other"
}

func defaultDeviceName() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "This device"
	}
	if fileExists("/etc/steamos-release") || os.Getenv("SteamDeck") == "1" {
		return "Steam Deck"
	}
	if len(h) > 60 {
		h = h[:60]
	}
	return h
}

// ---------------------------------------------------------------- HTTP

type accountError struct {
	Status  int
	Code    string
	Message string
	Body    []byte
}

func (e *accountError) Error() string { return e.Message }

var errOffline = errors.New("The NEXUS account service cannot be reached. Check your internet connection; NEXUS keeps your changes and synchronises them later.")

func (ac *Account) call(method, path string, body, out any) error {
	ac.mu.Lock()
	server, tok := ac.serverLocked(), ac.token
	ac.mu.Unlock()
	return ac.callAt(server, tok, method, path, body, out)
}

func (ac *Account) callAt(server, tok, method, path string, body, out any) error {
	if _, err := checkServerURL(server); err != nil || server == "" {
		return errors.New("NEXUS accounts are not available in this build yet: no account service has been set up.")
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, server+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := ac.client.Do(req)
	if err != nil {
		return errOffline
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return errOffline
	}
	if resp.StatusCode >= 300 {
		var e struct{ Error, Code string }
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("The account service answered %s.", resp.Status)
			if resp.StatusCode >= 500 || resp.StatusCode == 404 {
				return errOffline
			}
		}
		ae := &accountError{resp.StatusCode, e.Code, e.Error, data}
		if resp.StatusCode == 401 && tok != "" {
			ac.endSession(e.Error)
		}
		return ae
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// endSession: the service no longer accepts this device's sign-in (signed out elsewhere,
// removed, expired). Local data is untouched.
func (ac *Account) endSession(msg string) {
	ac.mu.Lock()
	ac.setCredLocked(ghCred{})
	ac.st.SignedIn, ac.st.Ended = false, msg
	ac.noteLocked("signout", msg)
	ac.saveLocked()
	ac.mu.Unlock()
	_ = os.Remove(ac.tokenPath())
	ac.changed()
}

func (ac *Account) startSession(server string, r sessionResult, how string) error {
	if err := storeSecretFile(ac.tokenPath(), []byte(r.Token)); err != nil {
		return fmt.Errorf("NEXUS could not store the sign-in safely on this device: %v", err)
	}
	ac.mu.Lock()
	sameAccount := ac.st.AccountID == r.Account.ID
	ac.noteAccountSwitchLocked(r.Account.ID, firstNonEmpty(r.Account.Email, r.Account.DisplayName))
	ac.token = r.Token
	ac.st.Server = ""
	if server != ac.serverLocked() {
		ac.st.Server = server
	}
	ac.st.AccountID, ac.st.Email, ac.st.DisplayName = r.Account.ID, r.Account.Email, r.Account.DisplayName
	ac.st.DeviceID, ac.st.SignedIn, ac.st.Ended, ac.st.Devices = r.DeviceID, true, "", r.Account.Devices
	for _, d := range r.Account.Devices {
		if d.ID == r.DeviceID {
			ac.st.DeviceName = d.Name
		}
	}
	if !sameAccount { // another account: start synchronisation from scratch (local data kept)
		ac.st.Cursor, ac.st.Base, ac.st.ProfileMap, ac.st.Conflicts = 0, map[string]syncBase{}, nil, nil
	}
	ac.st.SyncOn, ac.st.Provider = true, "server"
	ac.noteLocked(how, fmt.Sprintf("Signed in as %s on %s.", r.Account.Email, ac.st.DeviceName))
	ac.saveLocked()
	ac.mu.Unlock()
	ac.app.Log.Printf("account: signed in (%s)", how)
	ac.changed()
	ac.Kick()
	return nil
}

func (ac *Account) deviceInfo(name string) map[string]string {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	n := strings.TrimSpace(name)
	if n == "" {
		n = firstNonEmpty(ac.st.DeviceName, defaultDeviceName())
	}
	ac.st.DeviceName = n
	return map[string]string{"name": n, "platform": devicePlatform()}
}

// Register creates an account and signs this device in. It returns the recovery code, which
// the user must keep (it is shown once).
func (ac *Account) Register(email, password, displayName, deviceName string) (string, error) {
	server := ac.Server()
	var r sessionResult
	if err := ac.callAt(server, "", "POST", "/v1/accounts", map[string]any{"email": email, "password": password, "displayName": displayName, "device": ac.deviceInfo(deviceName)}, &r); err != nil {
		return "", err
	}
	return r.RecoveryCode, ac.startSession(server, r, "signin")
}

func (ac *Account) SignIn(email, password, deviceName string) error {
	server := ac.Server()
	var r sessionResult
	if err := ac.callAt(server, "", "POST", "/v1/sessions", map[string]any{"email": email, "password": password, "device": ac.deviceInfo(deviceName)}, &r); err != nil {
		return err
	}
	return ac.startSession(server, r, "signin")
}

// Recover sets a new password with the recovery code. All devices are signed out; this one is
// signed in again. Returns the new recovery code.
func (ac *Account) Recover(email, code, newPassword, deviceName string) (string, error) {
	server := ac.Server()
	var r sessionResult
	if err := ac.callAt(server, "", "POST", "/v1/recover", map[string]any{"email": email, "recoveryCode": code, "newPassword": newPassword, "device": ac.deviceInfo(deviceName)}, &r); err != nil {
		return "", err
	}
	return r.RecoveryCode, ac.startSession(server, r, "signin")
}

// SignOut ends this device's sign-in at the service (when reachable) and here. Local data,
// users, saves and amiibo stay on this device.
func (ac *Account) SignOut() error {
	var err error
	if ac.isGitHub() {
		ac.ghSignOut() // leaves the device list; best effort when offline
	} else {
		err = ac.call("DELETE", "/v1/sessions/current", nil, nil)
	}
	var ae *accountError
	if err != nil && !errors.Is(err, errOffline) && !(errors.As(err, &ae) && ae.Status == 401) {
		return err
	}
	ac.mu.Lock()
	ac.setCredLocked(ghCred{})
	ac.st.SignedIn, ac.st.Ended, ac.st.Devices, ac.st.LastError, ac.st.Offline = false, "", nil, "", false
	ac.noteLocked("signout", "Signed out on this device.")
	ac.saveLocked()
	ac.mu.Unlock()
	_ = os.Remove(ac.tokenPath())
	ac.changed()
	if errors.Is(err, errOffline) {
		return errors.New("Signed out on this device. The account service could not be reached, so this device still appears in your device list until you remove it from another device.")
	}
	return nil
}

// Refresh reloads the account and its devices.
func (ac *Account) Refresh() error {
	if ac.isGitHub() {
		return ac.ghRefresh()
	}
	var v struct {
		Email, DisplayName string
		Devices            []accountDevice
	}
	if err := ac.call("GET", "/v1/account", nil, &v); err != nil {
		return err
	}
	ac.mu.Lock()
	ac.st.Email, ac.st.DisplayName, ac.st.Devices = v.Email, v.DisplayName, v.Devices
	ac.saveLocked()
	ac.mu.Unlock()
	return nil
}

func (ac *Account) UpdateAccount(patch map[string]any) error {
	if ac.isGitHub() {
		if n, ok := patch["deviceName"].(string); ok {
			if err := ac.ghRenameDevice(n); err != nil {
				return err
			}
			ac.changed()
			return nil
		}
		return errors.New("Your name and password are managed by GitHub (github.com/settings).")
	}
	var v struct {
		Email, DisplayName string
		Devices            []accountDevice
	}
	if err := ac.call("PATCH", "/v1/account", patch, &v); err != nil {
		return err
	}
	ac.mu.Lock()
	ac.st.Email, ac.st.DisplayName, ac.st.Devices = v.Email, v.DisplayName, v.Devices
	for _, d := range v.Devices {
		if d.Current {
			ac.st.DeviceName = d.Name
		}
	}
	ac.saveLocked()
	ac.mu.Unlock()
	ac.changed()
	return nil
}

func (ac *Account) RemoveDevice(id string) error {
	ac.mu.Lock()
	self := id == ac.st.DeviceID
	ac.mu.Unlock()
	if self {
		return ac.SignOut()
	}
	var v struct{ Devices []accountDevice }
	if ac.isGitHub() {
		if err := ac.ghRemoveDevice(id); err != nil {
			return err
		}
		ac.mu.Lock()
		v.Devices = ac.st.Devices
		ac.mu.Unlock()
	} else if err := ac.call("DELETE", "/v1/devices/"+url.PathEscape(id), nil, &v); err != nil {
		return err
	}
	ac.mu.Lock()
	ac.st.Devices = v.Devices
	ac.noteLocked("signout", "A device was removed from the account.")
	ac.saveLocked()
	ac.mu.Unlock()
	ac.changed()
	return nil
}

// ---------------------------------------------------------------- device linking

// LinkCode asks the service for a one-time code another device can enter (10 minutes, once).
func (ac *Account) LinkCode() (string, int64, error) {
	var r struct {
		Code    string
		Expires int64
	}
	err := ac.call("POST", "/v1/pair/codes", nil, &r)
	return r.Code, r.Expires, err
}

type LinkRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Address  string `json:"address"`
	Expires  int64  `json:"expires"`
}

func (ac *Account) LinkRequests() ([]LinkRequest, error) {
	var r struct{ Requests []LinkRequest }
	err := ac.call("GET", "/v1/pair/requests", nil, &r)
	return r.Requests, err
}

func (ac *Account) DecideLink(id string, approve bool) error {
	err := ac.call("POST", "/v1/pair/requests/"+url.PathEscape(id), map[string]bool{"approve": approve}, nil)
	if err == nil && approve {
		ac.mu.Lock()
		ac.noteLocked("linked", "A new device was linked to the account.")
		ac.saveLocked()
		ac.mu.Unlock()
		_ = ac.Refresh()
		ac.changed()
	}
	return err
}

// ClaimCode: this (signed-out) device enters a code shown on a signed-in device. The other
// device must then approve; LinkStatus reports when it has.
func (ac *Account) ClaimCode(code, deviceName string) (linkClaimView, error) {
	server := ac.Server()
	var r struct {
		ClaimID, ClaimSecret, Account, DisplayName string
		Expires                                    int64
	}
	if err := ac.callAt(server, "", "POST", "/v1/pair/claim", map[string]any{"code": code, "device": ac.deviceInfo(deviceName)}, &r); err != nil {
		return linkClaimView{}, err
	}
	ac.mu.Lock()
	ac.claim = &linkClaim{Server: server, ID: r.ClaimID, Secret: r.ClaimSecret, Account: r.Account, DisplayName: r.DisplayName, Expires: r.Expires}
	ac.mu.Unlock()
	ac.changed()
	return linkClaimView{r.Account, r.DisplayName, r.Expires}, nil
}

// LinkStatus: pending | approved | declined | expired | none.
func (ac *Account) LinkStatus() (string, error) {
	ac.mu.Lock()
	c := ac.claim
	ac.mu.Unlock()
	if c == nil {
		return "none", nil
	}
	var r struct {
		Status  string
		Session sessionResult
	}
	if err := ac.callAt(c.Server, "", "POST", "/v1/pair/claim/"+url.PathEscape(c.ID), map[string]string{"claimSecret": c.Secret}, &r); err != nil {
		var ae *accountError
		if errors.As(err, &ae) && ae.Status == 404 {
			ac.CancelLink()
			return "expired", nil
		}
		return "pending", err
	}
	switch r.Status {
	case "approved":
		ac.CancelLink()
		return "approved", ac.startSession(c.Server, r.Session, "linked")
	case "declined", "expired":
		ac.CancelLink()
	}
	return r.Status, nil
}

func (ac *Account) CancelLink() {
	ac.mu.Lock()
	ac.claim = nil
	ac.mu.Unlock()
	ac.changed()
}

// ---------------------------------------------------------------- settings

func (ac *Account) SetSync(on *bool, categories map[string]bool) {
	ac.mu.Lock()
	if on != nil {
		ac.st.SyncOn = *on
	}
	for k, v := range categories {
		for _, c := range SyncCategories {
			if c == k {
				ac.st.Categories[k] = v
			}
		}
	}
	ac.st.LastError = ""
	ac.saveLocked()
	ac.mu.Unlock()
	ac.changed()
	ac.Kick()
}

func (ac *Account) SetOnboardSeen() {
	ac.mu.Lock()
	ac.st.OnboardSeen = true
	ac.saveLocked()
	ac.mu.Unlock()
}

// ---------------------------------------------------------------- local items

var tidKeyRe = regexp.MustCompile(`^[0-9A-F]{16}$`)

func canon(v any) []byte { b, _ := json.Marshal(v); return b }

func hashBytes(b []byte) string {
	if b == nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

type syncProfile struct {
	ProfileExport
	Image []byte `json:"image,omitempty"` // the user's picture, when it is an image file
}

type syncLibrary struct {
	Title         string      `json:"title,omitempty"`
	Hidden        bool        `json:"hidden,omitempty"`
	ArtPreference string      `json:"artPreference,omitempty"`
	ArtFit        string      `json:"artFit,omitempty"`
	Controls      *GameLayout `json:"controls,omitempty"`
}

type syncPrefs struct {
	Sound struct {
		Enabled bool    `json:"enabled"`
		Volume  float64 `json:"volume"`
	} `json:"sound"`
	Startup struct {
		Enabled bool   `json:"enabled"`
		Color   string `json:"color"`
		Style   string `json:"style"`
		Sound   bool   `json:"sound"`
	} `json:"startup"`
	Emulator struct {
		Fullscreen    bool   `json:"fullscreen"`
		QuitCombo     bool   `json:"quitCombo"`
		WhoPlays      string `json:"whoPlays"`
		HomeKey       string `json:"homeKey"`
		HomeButton    bool   `json:"homeButton"`
		FullscreenKey string `json:"fullscreenKey"`
		ScreenshotKey string `json:"screenshotKey"`
		CaptureButton bool   `json:"captureButton"`
		FadeIn        bool   `json:"fadeIn"`
		HomeAction    string `json:"homeAction"`
		GameWindow    string `json:"gameWindow"`
		AmiiboPrompt  string `json:"amiiboPrompt"`
	} `json:"emulator"`
	RecordKey string `json:"recordKey"`
}

type syncAmiibo struct {
	Amiibo
	Image []byte `json:"image,omitempty"` // NEXUS virtual amiibo data (never an imported dump)
}

// deviceSpecific engine settings are never synchronised (they describe this computer).
var deviceSpecificSettings = map[string]bool{"graphics.gpu": true}

func filterSettings(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if !deviceSpecificSettings[k] {
			out[k] = v
		}
	}
	return out
}

// localItems encodes this device's data for the enabled categories. profileKey maps local
// profile ids to account user ids (and back in applyItem).
func (ac *Account) localItems(cats map[string]bool, pmap map[string]string) map[string][]byte {
	a := ac.app
	out := map[string][]byte{}
	toAccount := map[string]string{}
	for acc, local := range pmap {
		toAccount[local] = acc
	}
	userKey := func(local string) string {
		if k, ok := toAccount[local]; ok {
			return k
		}
		return local
	}
	if cats["profile"] {
		for _, p := range a.Profiles.List() {
			x := syncProfile{ProfileExport: profileExport(p)}
			x.ID = userKey(p.ID)
			if p.Avatar.Kind == "image" && p.Avatar.File != "" {
				if b, err := os.ReadFile(filepath.Join(a.Paths.Profiles, p.ID, filepath.Base(p.Avatar.File))); err == nil && len(b) < 512<<10 {
					x.Image = b
				}
				x.Avatar.File = ""
			}
			out["profile/"+x.ID] = canon(x)
		}
	}
	if cats["playtime"] { // this device's own play time per user; other devices' arrive as their own items
		ac.mu.Lock()
		me := ac.st.DeviceID
		ac.mu.Unlock()
		for _, p := range a.Profiles.List() {
			if me != "" {
				out["playtime/"+userKey(p.ID)+"."+me] = canon(a.Profiles.LocalStats(p.ID))
			}
		}
	}
	if cats["library"] || cats["emulation"] {
		entries, _ := os.ReadDir(a.Library.GP.root)
		for _, e := range entries {
			if !e.IsDir() || !tidKeyRe.MatchString(e.Name()) {
				continue
			}
			key := "t-" + strings.ToLower(e.Name())
			gp := a.Library.GP.Get(key)
			if cats["library"] {
				l := syncLibrary{Title: gp.Title, Hidden: gp.Hidden, ArtPreference: gp.ArtPreference, ArtFit: gp.ArtFit, Controls: gp.Controls}
				if l.Title != "" || l.Hidden || l.ArtPreference != "" || l.ArtFit != "" || l.Controls != nil {
					out["library/"+e.Name()] = canon(l)
				}
			}
			if cats["emulation"] {
				if s := filterSettings(gp.Settings); len(s) > 0 {
					out["emulation/game-"+e.Name()] = canon(s)
				}
			}
		}
	}
	gs := a.Settings.Get()
	if cats["prefs"] {
		out["prefs/global"] = canon(prefsOf(gs))
	}
	if cats["emulation"] {
		out["emulation/global"] = canon(filterSettings(gs.Engine))
	}
	if cats["amiibo"] {
		for _, am := range a.Amiibo.List() {
			x := syncAmiibo{Amiibo: am}
			x.Owner = userKey(am.Owner)
			x.File, x.Art = "", "" // device-local file names
			if am.Kind == "virtual" && am.Available() {
				x.Image, _ = os.ReadFile(a.Amiibo.Path(am))
			}
			if am.Kind != "virtual" && x.Origin == "" {
				x.Origin = ac.thisDeviceName()
			}
			out["amiibo/"+am.ID] = canon(x)
		}
	}
	return out
}

func prefsOf(gs GlobalSettings) syncPrefs {
	var p syncPrefs
	p.Sound.Enabled, p.Sound.Volume = gs.Sound.Enabled, gs.Sound.Volume
	p.Startup.Enabled, p.Startup.Color, p.Startup.Style, p.Startup.Sound = gs.Startup.Enabled, gs.Startup.Color, gs.Startup.Style, gs.Startup.Sound
	e := gs.Emulator
	p.Emulator.Fullscreen, p.Emulator.QuitCombo, p.Emulator.WhoPlays, p.Emulator.HomeKey, p.Emulator.HomeButton = e.Fullscreen, e.QuitCombo, e.WhoPlays, e.HomeKey, e.HomeButton
	p.Emulator.FullscreenKey, p.Emulator.ScreenshotKey, p.Emulator.CaptureButton, p.Emulator.FadeIn = e.FullscreenKey, e.ScreenshotKey, e.CaptureButton, e.FadeIn
	p.Emulator.HomeAction, p.Emulator.GameWindow, p.Emulator.AmiiboPrompt = e.HomeAction, e.GameWindow, e.AmiiboPrompt
	p.RecordKey = gs.Recording.Key
	return p
}

// untouchedDefault: this device never synchronised the item and still has NEXUS's defaults
// there, so the account's version is simply taken (no conflict on a newly linked device).
func untouchedDefault(key string, data []byte) bool {
	switch key {
	case "prefs/global":
		return bytes.Equal(data, canon(prefsOf(DefaultSettings())))
	case "emulation/global":
		return bytes.Equal(data, canon(filterSettings(DefaultSettings().Engine)))
	}
	return false
}

func keyCategory(key string) string {
	c, _, _ := strings.Cut(key, "/")
	return c
}

func (ac *Account) snapshot() (map[string]bool, map[string]string, map[string]syncBase) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	cats := map[string]bool{}
	for k, v := range ac.st.Categories {
		cats[k] = v
	}
	pm := map[string]string{}
	for k, v := range ac.st.ProfileMap {
		pm[k] = v
	}
	base := map[string]syncBase{}
	for k, v := range ac.st.Base {
		base[k] = v
	}
	return cats, pm, base
}

// pendingKeys: items changed on this device since the last synchronisation.
func (ac *Account) pendingKeys() []string {
	cats, pm, base := ac.snapshot()
	var out []string
	local := ac.localItems(cats, pm)
	for k, b := range local {
		if base[k].Hash != hashBytes(b) {
			out = append(out, k)
		}
	}
	if cats["amiibo"] { // removed here since the last synchronisation
		for k, b := range base {
			if keyCategory(k) == "amiibo" && b.Hash != "" {
				if _, ok := local[k]; !ok {
					out = append(out, k)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// applyItem writes a synchronised item into this device's data. Returns the local encoding
// afterwards (nil if the item is not kept here).
func (ac *Account) applyItem(key string, data []byte, deleted bool, from string) error {
	a := ac.app
	cat, name, _ := strings.Cut(key, "/")
	switch cat {
	case "profile":
		if deleted {
			return nil // users are never deleted by synchronisation (their saves are here)
		}
		var x syncProfile
		if err := json.Unmarshal(data, &x); err != nil {
			return err
		}
		ac.mu.Lock()
		localID := name
		if m, ok := ac.st.ProfileMap[name]; ok {
			localID = m
		}
		ac.mu.Unlock()
		x.ID = localID
		if x.Image != nil {
			x.Avatar.Kind = "preset" // set below from the image
		}
		if _, ok := a.Profiles.Get(localID); ok {
			if err := a.Profiles.applyExport(x.ProfileExport); err != nil {
				return err
			}
		} else if err := a.Profiles.ImportLinked(x.ProfileExport); err != nil {
			return err
		}
		if x.Image != nil {
			if _, err := a.Profiles.SetAvatarImage(localID, x.Image, imageExt(x.Image)); err != nil {
				a.Log.Printf("account: picture for %s: %v", x.Name, err)
			}
		}
		return nil
	case "playtime":
		// another device's play time for a user: kept next to this device's own (Stats adds them)
		i := strings.LastIndex(name, ".")
		if i < 0 || deleted {
			return nil
		}
		user, dev := name[:i], name[i+1:]
		ac.mu.Lock()
		me := ac.st.DeviceID
		if m, ok := ac.st.ProfileMap[user]; ok {
			user = m
		}
		ac.mu.Unlock()
		if dev == me || !regexp.MustCompile(`^[0-9a-zA-Z_-]{1,64}$`).MatchString(dev) {
			return nil
		}
		if _, ok := a.Profiles.Get(user); !ok {
			return nil
		}
		var stats ProfileStats
		if err := json.Unmarshal(data, &stats); err != nil {
			return err
		}
		dir := filepath.Join(a.Paths.Profiles, user, "stats-devices")
		_ = os.MkdirAll(dir, 0o755)
		err := writeJSONAtomic(filepath.Join(dir, "account-"+dev+".json"), map[string]any{"stats": stats, "device": from})
		a.Profiles.dropStatsCache(user)
		return err
	case "library":
		var x syncLibrary
		if !deleted {
			if err := json.Unmarshal(data, &x); err != nil {
				return err
			}
		}
		_, err := a.Library.GP.Update("t-"+strings.ToLower(name), func(p *GameProfile) {
			p.Title, p.Hidden, p.ArtPreference, p.ArtFit, p.Controls = x.Title, x.Hidden, x.ArtPreference, x.ArtFit, x.Controls
		})
		return err
	case "prefs":
		var p syncPrefs
		if deleted || json.Unmarshal(data, &p) != nil {
			return nil
		}
		_, err := a.Settings.Update(func(s *GlobalSettings) {
			s.Sound.Enabled, s.Sound.Volume = p.Sound.Enabled, p.Sound.Volume
			s.Startup.Enabled, s.Startup.Color, s.Startup.Style, s.Startup.Sound = p.Startup.Enabled, p.Startup.Color, p.Startup.Style, p.Startup.Sound
			e := &s.Emulator
			e.Fullscreen, e.QuitCombo, e.WhoPlays, e.HomeKey, e.HomeButton = p.Emulator.Fullscreen, p.Emulator.QuitCombo, p.Emulator.WhoPlays, p.Emulator.HomeKey, p.Emulator.HomeButton
			e.FullscreenKey, e.ScreenshotKey, e.CaptureButton, e.FadeIn = p.Emulator.FullscreenKey, p.Emulator.ScreenshotKey, p.Emulator.CaptureButton, p.Emulator.FadeIn
			e.HomeAction, e.GameWindow, e.AmiiboPrompt = p.Emulator.HomeAction, p.Emulator.GameWindow, p.Emulator.AmiiboPrompt
			if p.RecordKey != "" {
				s.Recording.Key = p.RecordKey
			}
		})
		return err
	case "emulation":
		var m map[string]string
		if !deleted {
			if err := json.Unmarshal(data, &m); err != nil {
				return err
			}
		}
		m = filterSettings(m)
		if name == "global" {
			_, err := a.Settings.Update(func(s *GlobalSettings) {
				keep := map[string]string{}
				for k, v := range s.Engine {
					if deviceSpecificSettings[k] {
						keep[k] = v
					}
				}
				for k, v := range m {
					keep[k] = v
				}
				s.Engine = keep
			})
			return err
		}
		tid, ok := strings.CutPrefix(name, "game-")
		if !ok || !tidKeyRe.MatchString(tid) {
			return nil
		}
		_, err := a.Library.GP.Update("t-"+strings.ToLower(tid), func(p *GameProfile) {
			keep := map[string]string{}
			for k, v := range p.Settings {
				if deviceSpecificSettings[k] {
					keep[k] = v
				}
			}
			for k, v := range m {
				keep[k] = v
			}
			p.Settings = keep
		})
		return err
	case "amiibo":
		if deleted {
			if _, ok := a.Amiibo.Get(name); ok {
				return a.Amiibo.Remove(name)
			}
			return nil
		}
		var x syncAmiibo
		if err := json.Unmarshal(data, &x); err != nil {
			return err
		}
		ac.mu.Lock()
		if m, ok := ac.st.ProfileMap[x.Owner]; ok {
			x.Owner = m
		}
		ac.mu.Unlock()
		x.ID = name
		return a.Amiibo.applySynced(x.Amiibo, x.Image)
	}
	return nil
}

// ---------------------------------------------------------------- synchronisation

type SyncSummary struct {
	Downloaded int `json:"downloaded"`
	Uploaded   int `json:"uploaded"`
	Conflicts  int `json:"conflicts"`
}

// Touch: something may have changed locally; synchronise in about 10 seconds (several changes
// in a row are sent together).
func (ac *Account) Touch() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if !ac.st.SignedIn || !ac.st.SyncOn {
		return
	}
	if ac.touch == nil {
		ac.touch = time.AfterFunc(10*time.Second, ac.Kick)
	} else {
		ac.touch.Reset(10 * time.Second)
	}
}

// Kick asks the background synchroniser to run soon.
func (ac *Account) Kick() {
	select {
	case ac.wake <- struct{}{}:
	default:
	}
}

// Run is the background synchroniser: at start, when kicked, when local changes are pending
// (checked every 2 minutes), every 15 minutes, after the computer wakes from sleep, and when
// NEXUS comes to the front after more than a minute. It never blocks NEXUS. While offline it
// retries with a growing delay (30 s up to 10 minutes) instead of sending requests constantly;
// waking up or coming to the front tries again at once (at most every 30 s).
//
// Times are wall-clock: Windows' monotonic clock stops while the computer sleeps, so "15 minutes
// since the last synchronisation" measured with it could take hours of sleep plus 15 awake
// minutes (docs/REGRESSION_HISTORY.md, account unsynchronised after inactivity).
func (ac *Account) Run() {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	ev := ac.app.Events.Subscribe()
	defer ac.app.Events.Unsubscribe(ev)
	var last time.Time // wall clock (Round(0) drops the monotonic reading)
	lastTry := time.Time{}
	retry := 30 * time.Second
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	prevWall, prevMono := time.Now().Round(0), time.Now()
	for {
		why := ""
		select {
		case <-ac.stop:
			return
		case <-ac.wake:
			why = "requested"
		case <-timer.C:
			why = "scheduled"
		case m := <-ev:
			if !bytes.Contains(m, []byte(`"type":"app.focus","data":true`)) || time.Now().Round(0).Sub(last) < time.Minute || time.Since(lastTry) < 30*time.Second {
				continue
			}
			why = "NEXUS came to the front"
		case <-tick.C:
			wall, mono := time.Now().Round(0), time.Now()
			slept := wall.Sub(prevWall)-mono.Sub(prevMono) > time.Minute
			prevWall, prevMono = wall, mono
			switch {
			case slept:
				why = "the computer woke up"
				retry = 30 * time.Second
			case time.Now().Round(0).Sub(last) >= 15*time.Minute:
				why = "15 minutes"
			case len(ac.pendingKeys()) > 0:
				why = "local changes"
			default:
				continue
			}
		}
		st := ac.Status()
		if !st.SignedIn || !st.SyncOn {
			continue
		}
		// only a game in front waits (NEXUS is busy with it); a game kept running behind Home
		// does not stop synchronisation, or a session left there for hours would never sync
		if syncWaitsForGame(ac.app.Emu.State(), st.State) {
			continue
		}
		if why == "the computer woke up" || why == "NEXUS came to the front" {
			ac.app.Log.Printf("account: synchronising (%s)", why)
		}
		lastTry = time.Now()
		_, err := ac.Sync()
		last = time.Now().Round(0)
		ac.app.markStartup("first background sync finished", map[bool]string{true: "ok", false: syncOutcome(err)}[err == nil])
		if errors.Is(err, errOffline) { // retry with a growing delay while offline
			timer.Reset(retry)
			retry = min(retry*2, 10*time.Minute)
		} else {
			retry = 30 * time.Second
		}
	}
}

// syncWaitsForGame: synchronisation waits only while a game is in front (NEXUS is busy with
// it) and nothing local is waiting to be sent; a game kept running behind Home does not stop it.
func syncWaitsForGame(es RunState, state string) bool {
	return es.Running && !es.Suspended && state != "pending"
}

func (ac *Account) Close() {
	select {
	case <-ac.stop:
	default:
		close(ac.stop)
	}
}

// Sync runs one synchronisation now.
func (ac *Account) Sync() (SyncSummary, error) {
	ac.syncMu.Lock()
	defer ac.syncMu.Unlock()
	var sum SyncSummary
	ac.mu.Lock()
	if !ac.st.SignedIn {
		ac.mu.Unlock()
		return sum, errors.New("Sign in to your NEXUS account first.")
	}
	if !ac.st.SyncOn {
		ac.mu.Unlock()
		return sum, errors.New("Synchronisation is switched off on this device.")
	}
	if ac.token == "" && !ac.loadTokenLocked() {
		ac.saveLocked()
		ac.mu.Unlock()
		ac.changed()
		return sum, errOffline // tried again with the offline back-off
	}
	ac.busy = true
	cursor := ac.st.Cursor
	ac.mu.Unlock()
	ac.changed()
	err := ac.syncOnce(cursor, &sum)
	ac.mu.Lock()
	ac.busy = false
	ac.st.Offline = errors.Is(err, errOffline)
	if err != nil {
		ac.st.LastError = err.Error()
		if !ac.st.Offline {
			ac.noteLocked("failed", err.Error())
		}
	} else {
		ac.st.LastError, ac.st.LastSync = "", nowUnix()
		msg := "Up to date."
		if sum.Downloaded+sum.Uploaded > 0 {
			msg = fmt.Sprintf("%d change(s) received, %d sent.", sum.Downloaded, sum.Uploaded)
		}
		if sum.Conflicts > 0 {
			ac.noteLocked("conflict", fmt.Sprintf("%d item(s) were changed on two devices and need your choice.", sum.Conflicts))
		}
		if sum.Downloaded+sum.Uploaded > 0 || len(ac.st.History) == 0 || ac.st.History[len(ac.st.History)-1].Kind != "synced" {
			ac.noteLocked("synced", msg)
		}
	}
	ac.saveLocked()
	ac.mu.Unlock()
	ac.changed()
	if sum.Downloaded > 0 {
		ac.app.Events.Emit("library.changed", nil)
		ac.app.Events.Emit("profiles.changed", nil)
	}
	return sum, err
}

func (ac *Account) conflicted(key string) bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	for _, c := range ac.st.Conflicts {
		if c.Key == key {
			return true
		}
	}
	return false
}

func (ac *Account) setBase(key string, b syncBase) {
	ac.mu.Lock()
	ac.st.Base[key] = b
	ac.mu.Unlock()
}

func (ac *Account) addConflict(c AccountConflict) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	for _, x := range ac.st.Conflicts {
		if x.Key == c.Key {
			return
		}
	}
	c.ID, c.At = newAmiiboEntryID(), nowUnix()
	ac.st.Conflicts = append(ac.st.Conflicts, c)
}

func itemTitle(key string, data []byte) string {
	var v struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	_ = json.Unmarshal(data, &v)
	cat, name, _ := strings.Cut(key, "/")
	switch cat {
	case "profile":
		return "User " + firstNonEmpty(v.Name, name)
	case "playtime":
		return "Play time"
	case "amiibo":
		return "Amiibo " + firstNonEmpty(v.Name, name)
	case "library":
		return "Game " + firstNonEmpty(v.Title, name)
	case "prefs":
		return "NEXUS preferences"
	case "emulation":
		if name == "global" {
			return "Emulation settings"
		}
		return "Emulation settings for game " + strings.TrimPrefix(name, "game-")
	}
	return key
}

// syncBackend is where synchronised items live: the NEXUS account service (accountd) or the
// user's own GitHub account (account_github.go).
type syncBackend interface {
	// changes returns items changed since the cursor (all items for backends without one)
	changes(cursor int64) (map[string]remoteItem, int64, error)
	// put uploads an item based on baseVersion; conflict != nil when the stored version differs
	put(key string, baseVersion int64, deleted bool, data []byte) (res remoteItem, conflict *remoteItem, err error)
	// flush writes what put collected (backends that write in one request)
	flush() error
}

type serverBackend struct{ ac *Account }

func (b serverBackend) changes(cursor int64) (map[string]remoteItem, int64, error) {
	remote := map[string]remoteItem{}
	rev := cursor
	for {
		var page struct {
			Rev   int64
			Items []remoteItem
			More  bool
		}
		if err := b.ac.call("GET", fmt.Sprintf("/v1/sync?since=%d", rev), nil, &page); err != nil {
			return nil, 0, err
		}
		for _, it := range page.Items {
			remote[it.Key] = it
		}
		rev = page.Rev
		if !page.More {
			return remote, rev, nil
		}
	}
}

func (b serverBackend) put(key string, base int64, deleted bool, data []byte) (remoteItem, *remoteItem, error) {
	var res remoteItem
	err := b.ac.call("PUT", "/v1/sync/"+key, map[string]any{"baseVersion": base, "deleted": deleted, "data": data}, &res)
	var ae *accountError
	if errors.As(err, &ae) && ae.Status == 409 {
		var c struct{ Current *remoteItem }
		_ = json.Unmarshal(ae.Body, &c)
		if c.Current == nil {
			c.Current = &remoteItem{Key: key}
		}
		return res, c.Current, nil
	}
	return res, nil, err
}

func (b serverBackend) flush() error { return nil }

func (ac *Account) backend() syncBackend {
	if ac.isGitHub() {
		return &gistBackend{ac: ac}
	}
	return serverBackend{ac}
}

func (ac *Account) syncOnce(cursor int64, sum *SyncSummary) error {
	be := ac.backend()
	// 1. what changed on the service
	remote, rev, err := be.changes(cursor)
	if err != nil {
		return err
	}
	ac.mu.Lock()
	myDevice := ac.st.DeviceID
	ac.mu.Unlock()
	cats, _, _ := ac.snapshot()

	// 2. users first (so amiibo owners and layouts find them), matching same-named users
	keys := make([]string, 0, len(remote))
	for k := range remote {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, pj := keyCategory(keys[i]) == "profile", keyCategory(keys[j]) == "profile"
		if pi != pj {
			return pi
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		if keyCategory(k) == "profile" && !remote[k].Deleted {
			ac.matchUser(k, remote[k])
		}
	}
	_, pm, base := ac.snapshot()
	local := ac.localItems(cats, pm)

	for _, k := range keys {
		it := remote[k]
		if !cats[keyCategory(k)] || ac.conflicted(k) {
			continue
		}
		b := base[k]
		if it.Version == b.Version {
			continue
		}
		lb, haveLocal := local[k]
		localChanged := haveLocal && hashBytes(lb) != b.Hash || !haveLocal && b.Hash != "" && keyCategory(k) == "amiibo"
		if b.Version == 0 && haveLocal && untouchedDefault(k, lb) {
			localChanged = false
		}
		if it.DeviceID == myDevice && !localChanged { // our own upload coming back
			ac.setBase(k, syncBase{it.Version, hashBytes(lb)})
			continue
		}
		switch {
		case !localChanged || haveLocal && !it.Deleted && hashBytes(lb) == hashBytes(it.Data):
			if err := ac.applyItem(k, it.Data, it.Deleted, it.DeviceName); err != nil {
				if keyCategory(k) == "profile" {
					ac.addConflict(AccountConflict{Key: k, Kind: "user", Title: itemTitle(k, it.Data), Remote: it.Data, RemoteVer: it.Version, RemoteFrom: it.DeviceName})
					sum.Conflicts++
					continue
				}
				return fmt.Errorf("%s: %v", itemTitle(k, it.Data), err)
			}
			now := ac.localItems(map[string]bool{keyCategory(k): true}, ac.profileMap())[k]
			ac.setBase(k, syncBase{it.Version, hashBytes(now)})
			local[k] = now
			sum.Downloaded++
		default:
			ac.addConflict(AccountConflict{Key: k, Kind: "change", Title: itemTitle(k, firstBytes(lb, it.Data)), Local: lb, Remote: it.Data, RemoteVer: it.Version, RemoteDel: it.Deleted, RemoteFrom: it.DeviceName})
			sum.Conflicts++
		}
	}

	// 3. send this device's changes
	_, pm, base = ac.snapshot()
	local = ac.localItems(cats, pm) // after what arrived (new users, amiibo) was applied
	pk := ac.pendingKeys()
	var sent []func()
	ac.mu.Lock()
	hold := ac.st.HoldUploads
	ac.mu.Unlock()
	for _, k := range pk {
		if hold {
			break // a different account than before: nothing is sent until the user chooses
		}
		if !cats[keyCategory(k)] || ac.conflicted(k) {
			continue
		}
		data, ok := local[k]
		res, cur, err := be.put(k, base[k].Version, !ok, data)
		if err != nil {
			return err
		}
		if cur != nil {
			ac.addConflict(AccountConflict{Key: k, Kind: "change", Title: itemTitle(k, firstBytes(data, cur.Data)), Local: data, Remote: cur.Data, RemoteVer: cur.Version, RemoteDel: cur.Deleted, RemoteFrom: cur.DeviceName})
			sum.Conflicts++
			continue
		}
		sent = append(sent, func() { ac.setBase(k, syncBase{res.Version, hashBytes(data)}) })
		sum.Uploaded++
	}
	if err := be.flush(); err != nil { // nothing counts as sent unless it was written
		return err
	}
	for _, f := range sent {
		f()
	}
	ac.mu.Lock()
	ac.st.Cursor = rev
	ac.mu.Unlock()
	return nil
}

func firstBytes(a, b []byte) []byte {
	if a != nil {
		return a
	}
	return b
}

func (ac *Account) profileMap() map[string]string {
	_, pm, _ := ac.snapshot()
	return pm
}

// matchUser: a user from the account whose name matches a user of this device that has never
// been synchronised is the same person (for example "Tom" created on both devices before
// linking): NEXUS links the two instead of creating a second "Tom". Their details are then
// compared like any other item, so differences become a conflict to choose from.
func (ac *Account) matchUser(key string, it remoteItem) {
	var x syncProfile
	if json.Unmarshal(it.Data, &x) != nil {
		return
	}
	id := strings.TrimPrefix(key, "profile/")
	if _, ok := ac.app.Profiles.Get(id); ok {
		return
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if _, ok := ac.st.ProfileMap[id]; ok {
		return
	}
	mapped := map[string]bool{}
	for _, l := range ac.st.ProfileMap {
		mapped[l] = true
	}
	for _, p := range ac.app.Profiles.List() {
		if !strings.EqualFold(p.Name, x.Name) || mapped[p.ID] {
			continue
		}
		if _, synced := ac.st.Base["profile/"+p.ID]; synced {
			continue
		}
		if ac.st.ProfileMap == nil {
			ac.st.ProfileMap = map[string]string{}
		}
		ac.st.ProfileMap[id] = p.ID
		ac.app.Log.Printf("account: user %q on this device is the account's user %q", p.Name, x.Name)
		return
	}
}

// ResolveConflict: keep "this" device's version, or use the "other" device's.
func (ac *Account) ResolveConflict(id, keep string) error {
	ac.mu.Lock()
	var c *AccountConflict
	idx := -1
	for i := range ac.st.Conflicts {
		if ac.st.Conflicts[i].ID == id {
			c, idx = &ac.st.Conflicts[i], i
		}
	}
	if c == nil {
		ac.mu.Unlock()
		return errors.New("That conflict has already been resolved.")
	}
	cc := *c
	ac.mu.Unlock()
	switch {
	case cc.Kind == "user" && keep == "separate":
		// keep both users: the account's user arrives under another name
		var x syncProfile
		if err := json.Unmarshal(cc.Remote, &x); err != nil {
			return err
		}
		x.Name = firstNonEmpty(strings.TrimSpace(x.Name), "User")
		if len([]rune(x.Name)) > 17 {
			x.Name = string([]rune(x.Name)[:17])
		}
		x.Name += " 2"
		if err := ac.applyItem(cc.Key, canon(x), false, cc.RemoteFrom); err != nil {
			return err
		}
		ac.setBase(cc.Key, syncBase{cc.RemoteVer, ""}) // the renamed user is sent back as a change
	case keep == "this":
		ac.setBase(cc.Key, syncBase{cc.RemoteVer, "-"}) // pending: uploaded on top of the other version
	case keep == "other":
		if err := ac.applyItem(cc.Key, cc.Remote, cc.RemoteDel, cc.RemoteFrom); err != nil {
			return err
		}
		now := ac.localItems(map[string]bool{keyCategory(cc.Key): true}, ac.profileMap())[cc.Key]
		ac.setBase(cc.Key, syncBase{cc.RemoteVer, hashBytes(now)})
	default:
		return errors.New("Choose which version to keep.")
	}
	ac.mu.Lock()
	if idx < len(ac.st.Conflicts) && ac.st.Conflicts[idx].ID == id {
		ac.st.Conflicts = append(ac.st.Conflicts[:idx], ac.st.Conflicts[idx+1:]...)
	}
	ac.noteLocked("synced", "Resolved: "+cc.Title+".")
	ac.saveLocked()
	ac.mu.Unlock()
	ac.changed()
	ac.Kick()
	return nil
}

// ConflictDetails returns both versions of a conflict for the interface (as JSON).
func (ac *Account) ConflictDetails(id string) (map[string]any, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	for _, c := range ac.st.Conflicts {
		if c.ID == id {
			var l, r any
			_ = json.Unmarshal(c.Local, &l)
			_ = json.Unmarshal(c.Remote, &r)
			strip := func(v any) any {
				if m, ok := v.(map[string]any); ok {
					delete(m, "image")
				}
				return v
			}
			return map[string]any{"conflict": c.Title, "this": strip(l), "other": strip(r), "otherDevice": c.RemoteFrom, "otherDeleted": c.RemoteDel, "kind": c.Kind}, nil
		}
	}
	return nil, errors.New("That conflict has already been resolved.")
}

func (ac *Account) thisDeviceName() string {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return firstNonEmpty(ac.st.DeviceName, defaultDeviceName())
}

func imageExt(b []byte) string {
	switch http.DetectContentType(b) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	return ".png"
}
