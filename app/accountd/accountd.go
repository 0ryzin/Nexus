// Package accountd is the NEXUS account service: accounts, signed-in devices, device linking
// with one-time codes, and synchronisation of NEXUS data between a user's devices.
//
// It is a small HTTP JSON API meant to run behind a TLS terminator (Caddy in deploy/) or with
// its own certificate (cmd/nexus-accountd -tls-cert/-tls-key). NEXUS clients only talk to it
// over HTTPS (plain HTTP is accepted for 127.0.0.1/localhost, for testing).
//
// Security model:
//   - Passwords: PBKDF2-HMAC-SHA256 (600,000 iterations by default) with a random salt per
//     account; only the hash is stored. Unknown e-mail addresses cost the same time as wrong
//     passwords. Repeated failures lock the account's sign-in for 15 minutes.
//   - Each signed-in device has its own random token. The server stores only its SHA-256, so a
//     copy of the database cannot be used to sign in. Tokens expire after 180 days without use
//     and are revoked when the device is removed, signs out, or the password is reset.
//   - Recovery code (shown once at registration) resets a forgotten password without e-mail.
//   - Device linking: a signed-in device asks for a code (8 characters, 10 minutes, single
//     use). The new device sends the code; the signed-in device must approve it. Only then does
//     the new device receive its own token. Codes carry no account access by themselves.
//   - Per-address rate limits on registration, sign-in, recovery and code entry.
//   - Synchronised items are limited to known NEXUS data kinds (the server refuses anything
//     else, such as saves, keys or game files), with size and count limits per account.
//   - Nothing secret is logged.
package accountd

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ProtocolVersion is reported by /v1/health; clients refuse a server with another major.
const ProtocolVersion = 1

const (
	DefaultIterations = 600_000
	tokenLifetime     = 180 * 24 * time.Hour
	pairCodeLifetime  = 10 * time.Minute
	claimLifetime     = 10 * time.Minute
	lockAfter         = 5
	lockFor           = 15 * time.Minute
	maxItemBytes      = 1 << 20
	maxAccountBytes   = 64 << 20
	maxItems          = 5000
	maxDevices        = 20
	maxPageItems      = 500
)

// Config of a server. Zero values give production defaults.
type Config struct {
	DataDir    string // where the database files are kept
	Iterations int    // PBKDF2 iterations (tests use fewer)
	TrustProxy bool   // take the client address from X-Forwarded-For (behind Caddy)
	Now        func() time.Time
	Logger     *log.Logger
	// RateLimit: requests per minute per address on sign-in style endpoints (default 20).
	RateLimit int
}

type Server struct {
	cfg Config
	mu  sync.Mutex
	db  database
	// in memory: short-lived codes and claims (a restart cancels them, which is safe)
	codes  map[string]*pairCode // by code hash
	claims map[string]*claim    // by claim id
	limits map[string][]time.Time
	mux    *http.ServeMux
}

type database struct {
	Accounts map[string]*Account `json:"accounts"`
	Devices  map[string]*Device  `json:"devices"`
	byEmail  map[string]string
	byToken  map[string]string
	items    map[string]*accountItems // loaded lazily per account
}

type Account struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"displayName"`
	PasswordHash string `json:"passwordHash"`
	RecoveryHash string `json:"recoveryHash"`
	Created      int64  `json:"created"`
	Failed       int    `json:"failed,omitempty"`
	LockedUntil  int64  `json:"lockedUntil,omitempty"`
}

type Device struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	TokenHash string `json:"tokenHash"`
	Created   int64  `json:"created"`
	LastSeen  int64  `json:"lastSeen"`
	Expires   int64  `json:"expires"`
}

// Item is one synchronised piece of NEXUS data.
type Item struct {
	Key        string `json:"key"`
	Version    int64  `json:"version"` // per item, for conflict detection
	Rev        int64  `json:"rev"`     // per account, for "changes since"
	Deleted    bool   `json:"deleted,omitempty"`
	Data       []byte `json:"data,omitempty"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Updated    int64  `json:"updated"`
}

type accountItems struct {
	Rev   int64            `json:"rev"`
	Items map[string]*Item `json:"items"`
	bytes int
}

type pairCode struct {
	AccountID string
	DeviceID  string
	Expires   time.Time
}

type claim struct {
	ID         string
	SecretHash string
	AccountID  string
	Name       string
	Platform   string
	Address    string
	Status     string // pending | approved | declined
	Expires    time.Time
	token      string // handed over once, after approval
	device     *Device
}

// New opens (or creates) a server database in cfg.DataDir.
func New(cfg Config) (*Server, error) {
	if cfg.Iterations <= 0 {
		cfg.Iterations = DefaultIterations
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stderr, "accountd ", log.LstdFlags)
	}
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = 20
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "items"), 0o700); err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, codes: map[string]*pairCode{}, claims: map[string]*claim{}, limits: map[string][]time.Time{}}
	s.db = database{Accounts: map[string]*Account{}, Devices: map[string]*Device{}}
	if b, err := os.ReadFile(s.dbPath()); err == nil {
		if err := json.Unmarshal(b, &s.db); err != nil {
			return nil, fmt.Errorf("database %s is damaged: %w", s.dbPath(), err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if s.db.Accounts == nil {
		s.db.Accounts = map[string]*Account{}
	}
	if s.db.Devices == nil {
		s.db.Devices = map[string]*Device{}
	}
	s.db.byEmail, s.db.byToken, s.db.items = map[string]string{}, map[string]string{}, map[string]*accountItems{}
	for id, a := range s.db.Accounts {
		s.db.byEmail[a.Email] = id
	}
	for id, d := range s.db.Devices {
		s.db.byToken[d.TokenHash] = id
	}
	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	s.mux.ServeHTTP(w, r)
}

// ---------------------------------------------------------------- storage

func (s *Server) dbPath() string { return filepath.Join(s.cfg.DataDir, "accounts.json") }

func writeAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

func (s *Server) saveLocked() error { return writeAtomic(s.dbPath(), &s.db) }

func (s *Server) itemsLocked(accountID string) *accountItems {
	if it := s.db.items[accountID]; it != nil {
		return it
	}
	it := &accountItems{Items: map[string]*Item{}}
	if b, err := os.ReadFile(s.itemsPath(accountID)); err == nil {
		_ = json.Unmarshal(b, it)
		if it.Items == nil {
			it.Items = map[string]*Item{}
		}
		for _, x := range it.Items {
			it.bytes += len(x.Data)
		}
	}
	s.db.items[accountID] = it
	return it
}

func (s *Server) itemsPath(accountID string) string {
	return filepath.Join(s.cfg.DataDir, "items", accountID+".json")
}

// ---------------------------------------------------------------- secrets

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func newID() string { return hex.EncodeToString(randomBytes(12)) }

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (s *Server) hashSecret(secret string) string {
	salt := randomBytes(16)
	k, _ := pbkdf2.Key(sha256.New, secret, salt, s.cfg.Iterations, 32)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", s.cfg.Iterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k))
}

func checkSecret(stored, secret string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	var iter int
	if _, err := fmt.Sscanf(parts[1], "%d", &iter); err != nil || iter < 1000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	k, _ := pbkdf2.Key(sha256.New, secret, salt, iter, len(want))
	return subtle.ConstantTimeCompare(k, want) == 1
}

var codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // no 0/O, 1/I/L

func randomCode(n int) string {
	b := randomBytes(n)
	out := make([]byte, n)
	for i := range b {
		out[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(out)
}

// NormalizeCode removes spaces and dashes and upper-cases a code someone typed.
func NormalizeCode(c string) string {
	c = strings.ToUpper(c)
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, c)
}

func newRecoveryCode() string {
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes(15)) // 24 chars
	return raw[0:6] + "-" + raw[6:12] + "-" + raw[12:18] + "-" + raw[18:24]
}

// ---------------------------------------------------------------- HTTP helpers

type apiError struct {
	status int
	code   string
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func fail(status int, code, msg string) error { return &apiError{status, code, msg} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeJSON(w, ae.status, map[string]string{"error": ae.msg, "code": ae.code})
		return
	}
	writeJSON(w, 500, map[string]string{"error": "The account service had a problem. Try again later.", "code": "server"})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fail(400, "bad_request", "The request was not understood.")
	}
	return nil
}

func (s *Server) clientAddr(r *http.Request) string {
	if s.cfg.TrustProxy {
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			return strings.TrimSpace(strings.Split(f, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limit allows cfg.RateLimit requests per minute per address and bucket.
func (s *Server) limit(r *http.Request, bucket string) error {
	key := bucket + "|" + s.clientAddr(r)
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := s.limits[key][:0]
	for _, t := range s.limits[key] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= s.cfg.RateLimit {
		s.limits[key] = keep
		return fail(429, "rate_limited", "Too many attempts. Wait a minute and try again.")
	}
	s.limits[key] = append(keep, now)
	return nil
}

type authed struct {
	acc *Account
	dev *Device
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// auth checks the device token (and refreshes its expiry) - called with s.mu held.
func (s *Server) authLocked(r *http.Request) (authed, error) {
	tok := bearer(r)
	if tok == "" {
		return authed{}, fail(401, "signed_out", "You are not signed in.")
	}
	id, ok := s.db.byToken[sha(tok)]
	d := s.db.Devices[id]
	if !ok || d == nil {
		return authed{}, fail(401, "signed_out", "This device has been signed out of your NEXUS account. Sign in again.")
	}
	now := s.cfg.Now()
	if now.Unix() > d.Expires {
		s.removeDeviceLocked(d.ID)
		_ = s.saveLocked()
		return authed{}, fail(401, "expired", "Your sign-in on this device has expired. Sign in again.")
	}
	a := s.db.Accounts[d.AccountID]
	if a == nil {
		return authed{}, fail(401, "signed_out", "This NEXUS account no longer exists.")
	}
	if now.Unix()-d.LastSeen > 3600 {
		d.LastSeen = now.Unix()
		d.Expires = now.Add(tokenLifetime).Unix()
		_ = s.saveLocked()
	}
	return authed{a, d}, nil
}

func (s *Server) removeDeviceLocked(id string) {
	if d := s.db.Devices[id]; d != nil {
		delete(s.db.byToken, d.TokenHash)
		delete(s.db.Devices, id)
	}
}

func (s *Server) newDeviceLocked(accountID string, info DeviceInfo) (*Device, string, error) {
	n := 0
	for _, d := range s.db.Devices {
		if d.AccountID == accountID {
			n++
		}
	}
	if n >= maxDevices {
		return nil, "", fail(409, "too_many_devices", fmt.Sprintf("This account already has %d devices. Remove one you no longer use, then try again.", maxDevices))
	}
	tok := "nxa_" + base64.RawURLEncoding.EncodeToString(randomBytes(32))
	now := s.cfg.Now()
	d := &Device{ID: newID(), AccountID: accountID, Name: info.Name, Platform: info.Platform, TokenHash: sha(tok), Created: now.Unix(), LastSeen: now.Unix(), Expires: now.Add(tokenLifetime).Unix()}
	s.db.Devices[d.ID] = d
	s.db.byToken[d.TokenHash] = d.ID
	return d, tok, nil
}

// DeviceInfo identifies a device to the user (shown in the device list).
type DeviceInfo struct {
	Name     string `json:"name"`
	Platform string `json:"platform"` // windows | linux | macos | android
}

func (d *DeviceInfo) clean() error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || utf8.RuneCountInString(d.Name) > 60 {
		return fail(400, "bad_device", "Give this device a name of up to 60 characters.")
	}
	switch d.Platform {
	case "windows", "linux", "macos", "android", "ios":
	default:
		d.Platform = "other"
	}
	return nil
}

func cleanEmail(e string) (string, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e || len(e) > 254 || !strings.Contains(e[strings.LastIndex(e, "@"):], ".") {
		return "", fail(400, "bad_email", "Enter a valid e-mail address.")
	}
	return e, nil
}

func checkPassword(p string) error {
	if utf8.RuneCountInString(p) < 10 {
		return fail(400, "weak_password", "Use a password of at least 10 characters.")
	}
	if len(p) > 512 {
		return fail(400, "weak_password", "That password is too long.")
	}
	return nil
}

// AccountView is what clients see of an account.
type AccountView struct {
	ID          string       `json:"id"`
	Email       string       `json:"email"`
	DisplayName string       `json:"displayName"`
	Created     int64        `json:"created"`
	Devices     []DeviceView `json:"devices"`
}

type DeviceView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Created  int64  `json:"created"`
	LastSeen int64  `json:"lastSeen"`
	Current  bool   `json:"current"`
}

func (s *Server) viewLocked(a *Account, current string) AccountView {
	v := AccountView{ID: a.ID, Email: a.Email, DisplayName: a.DisplayName, Created: a.Created, Devices: []DeviceView{}}
	for _, d := range s.db.Devices {
		if d.AccountID == a.ID {
			v.Devices = append(v.Devices, DeviceView{d.ID, d.Name, d.Platform, d.Created, d.LastSeen, d.ID == current})
		}
	}
	sort.Slice(v.Devices, func(i, j int) bool { return v.Devices[i].Created < v.Devices[j].Created })
	return v
}

// SessionResult is returned when a device signs in.
type SessionResult struct {
	Token        string      `json:"token"`
	DeviceID     string      `json:"deviceId"`
	Account      AccountView `json:"account"`
	RecoveryCode string      `json:"recoveryCode,omitempty"` // only at registration and recovery
}

var keyRe = regexp.MustCompile(`^(profile|playtime|library|prefs|emulation|amiibo|amiibo-file)/[A-Za-z0-9._-]{1,80}$`)

// ---------------------------------------------------------------- routes

func (s *Server) routes() {
	m := http.NewServeMux()
	s.mux = m
	m.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"service": "nexus-accountd", "protocol": ProtocolVersion})
	})

	m.HandleFunc("POST /v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Email, Password, DisplayName string
			Device                       DeviceInfo
		}
		if err := s.limit(r, "register"); err != nil {
			writeErr(w, err)
			return
		}
		if err := readJSON(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		email, err := cleanEmail(b.Email)
		if err == nil {
			err = checkPassword(b.Password)
		}
		if err == nil {
			err = b.Device.clean()
		}
		b.DisplayName = strings.TrimSpace(b.DisplayName)
		if err == nil && (b.DisplayName == "" || utf8.RuneCountInString(b.DisplayName) > 40) {
			err = fail(400, "bad_name", "Enter a name of up to 40 characters.")
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		pw, rec := s.hashSecret(b.Password), newRecoveryCode()
		rh := s.hashSecret(NormalizeCode(rec))
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, taken := s.db.byEmail[email]; taken {
			writeErr(w, fail(409, "email_taken", "There is already a NEXUS account with this e-mail address. Sign in instead, or use account recovery if you forgot the password."))
			return
		}
		a := &Account{ID: newID(), Email: email, DisplayName: b.DisplayName, PasswordHash: pw, RecoveryHash: rh, Created: s.cfg.Now().Unix()}
		s.db.Accounts[a.ID] = a
		s.db.byEmail[email] = a.ID
		d, tok, err := s.newDeviceLocked(a.ID, b.Device)
		if err == nil {
			err = s.saveLocked()
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		s.cfg.Logger.Printf("account created %s (device %s, %s)", a.ID, d.ID, d.Platform)
		writeJSON(w, 201, SessionResult{Token: tok, DeviceID: d.ID, Account: s.viewLocked(a, d.ID), RecoveryCode: rec})
	})

	dummy := s.hashSecret("not a real password")
	m.HandleFunc("POST /v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Email, Password string
			Device          DeviceInfo
		}
		if err := s.limit(r, "signin"); err != nil {
			writeErr(w, err)
			return
		}
		if err := readJSON(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := b.Device.clean(); err != nil {
			writeErr(w, err)
			return
		}
		email := strings.ToLower(strings.TrimSpace(b.Email))
		s.mu.Lock()
		a := s.db.Accounts[s.db.byEmail[email]]
		hash := dummy
		if a != nil {
			hash = a.PasswordHash
			if a.LockedUntil > s.cfg.Now().Unix() {
				s.mu.Unlock()
				writeErr(w, fail(429, "locked", "Too many wrong passwords. Sign-in for this account is paused for 15 minutes."))
				return
			}
		}
		s.mu.Unlock()
		ok := checkSecret(hash, b.Password) // outside the lock: slow on purpose
		s.mu.Lock()
		defer s.mu.Unlock()
		if a == nil || !ok {
			if a != nil {
				a.Failed++
				if a.Failed >= lockAfter {
					a.Failed, a.LockedUntil = 0, s.cfg.Now().Add(lockFor).Unix()
				}
				_ = s.saveLocked()
			}
			writeErr(w, fail(401, "bad_credentials", "The e-mail address or password is wrong."))
			return
		}
		a.Failed, a.LockedUntil = 0, 0
		d, tok, err := s.newDeviceLocked(a.ID, b.Device)
		if err == nil {
			err = s.saveLocked()
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, SessionResult{Token: tok, DeviceID: d.ID, Account: s.viewLocked(a, d.ID)})
	})

	m.HandleFunc("POST /v1/recover", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Email, RecoveryCode, NewPassword string
			Device                           DeviceInfo
		}
		if err := s.limit(r, "recover"); err != nil {
			writeErr(w, err)
			return
		}
		if err := readJSON(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := checkPassword(b.NewPassword); err != nil {
			writeErr(w, err)
			return
		}
		if err := b.Device.clean(); err != nil {
			writeErr(w, err)
			return
		}
		s.mu.Lock()
		a := s.db.Accounts[s.db.byEmail[strings.ToLower(strings.TrimSpace(b.Email))]]
		hash := dummy
		if a != nil {
			hash = a.RecoveryHash
		}
		s.mu.Unlock()
		ok := checkSecret(hash, NormalizeCode(b.RecoveryCode))
		if a == nil || !ok {
			writeErr(w, fail(401, "bad_recovery", "The e-mail address or recovery code is wrong."))
			return
		}
		pw, rec := s.hashSecret(b.NewPassword), newRecoveryCode()
		rh := s.hashSecret(NormalizeCode(rec))
		s.mu.Lock()
		defer s.mu.Unlock()
		a.PasswordHash, a.RecoveryHash, a.Failed, a.LockedUntil = pw, rh, 0, 0
		for id, d := range s.db.Devices { // every device signs in again
			if d.AccountID == a.ID {
				s.removeDeviceLocked(id)
			}
		}
		d, tok, err := s.newDeviceLocked(a.ID, b.Device)
		if err == nil {
			err = s.saveLocked()
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		s.cfg.Logger.Printf("account %s recovered; all devices signed out", a.ID)
		writeJSON(w, 200, SessionResult{Token: tok, DeviceID: d.ID, Account: s.viewLocked(a, d.ID), RecoveryCode: rec})
	})

	// authenticated routes
	with := func(fn func(w http.ResponseWriter, r *http.Request, au authed) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			au, err := s.authLocked(r)
			s.mu.Unlock()
			if err == nil {
				err = fn(w, r, au)
			}
			if err != nil {
				writeErr(w, err)
			}
		}
	}
	m.HandleFunc("GET /v1/account", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, 200, s.viewLocked(au.acc, au.dev.ID))
		return nil
	}))
	m.HandleFunc("PATCH /v1/account", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		var b struct {
			DisplayName           *string
			Password, NewPassword string
			DeviceName            *string
		}
		if err := readJSON(r, &b); err != nil {
			return err
		}
		if b.NewPassword != "" {
			if err := checkPassword(b.NewPassword); err != nil {
				return err
			}
			if !checkSecret(s.passwordHash(au.acc), b.Password) {
				return fail(403, "bad_credentials", "The current password is wrong.")
			}
		}
		var newHash string
		if b.NewPassword != "" {
			newHash = s.hashSecret(b.NewPassword)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if b.DisplayName != nil {
			n := strings.TrimSpace(*b.DisplayName)
			if n == "" || utf8.RuneCountInString(n) > 40 {
				return fail(400, "bad_name", "Enter a name of up to 40 characters.")
			}
			au.acc.DisplayName = n
		}
		if b.DeviceName != nil {
			di := DeviceInfo{Name: *b.DeviceName, Platform: au.dev.Platform}
			if err := di.clean(); err != nil {
				return err
			}
			au.dev.Name = di.Name
		}
		if newHash != "" {
			au.acc.PasswordHash = newHash
			for id, d := range s.db.Devices { // other devices sign in again
				if d.AccountID == au.acc.ID && id != au.dev.ID {
					s.removeDeviceLocked(id)
				}
			}
		}
		if err := s.saveLocked(); err != nil {
			return err
		}
		writeJSON(w, 200, s.viewLocked(au.acc, au.dev.ID))
		return nil
	}))
	m.HandleFunc("DELETE /v1/account", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		var b struct{ Password string }
		if err := readJSON(r, &b); err != nil {
			return err
		}
		if !checkSecret(s.passwordHash(au.acc), b.Password) {
			return fail(403, "bad_credentials", "The password is wrong.")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for id, d := range s.db.Devices {
			if d.AccountID == au.acc.ID {
				s.removeDeviceLocked(id)
			}
		}
		delete(s.db.byEmail, au.acc.Email)
		delete(s.db.Accounts, au.acc.ID)
		delete(s.db.items, au.acc.ID)
		_ = os.Remove(s.itemsPath(au.acc.ID))
		s.cfg.Logger.Printf("account %s deleted", au.acc.ID)
		writeJSON(w, 200, map[string]bool{"deleted": true})
		return s.saveLocked()
	}))
	m.HandleFunc("DELETE /v1/sessions/current", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.removeDeviceLocked(au.dev.ID)
		writeJSON(w, 200, map[string]bool{"signedOut": true})
		return s.saveLocked()
	}))
	m.HandleFunc("DELETE /v1/devices/{id}", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		d := s.db.Devices[r.PathValue("id")]
		if d == nil || d.AccountID != au.acc.ID {
			return fail(404, "no_device", "That device is not linked to this account.")
		}
		s.removeDeviceLocked(d.ID)
		if err := s.saveLocked(); err != nil {
			return err
		}
		writeJSON(w, 200, s.viewLocked(au.acc, au.dev.ID))
		return nil
	}))

	// ----- device linking
	m.HandleFunc("POST /v1/pair/codes", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		now := s.cfg.Now()
		n := 0
		for h, c := range s.codes {
			if now.After(c.Expires) {
				delete(s.codes, h)
			} else if c.AccountID == au.acc.ID {
				n++
			}
		}
		if n >= 3 {
			return fail(429, "too_many_codes", "Three linking codes are already waiting. Use one of them or wait 10 minutes.")
		}
		code := randomCode(8)
		s.codes[sha(code)] = &pairCode{AccountID: au.acc.ID, DeviceID: au.dev.ID, Expires: now.Add(pairCodeLifetime)}
		writeJSON(w, 201, map[string]any{"code": code[:4] + "-" + code[4:], "expires": now.Add(pairCodeLifetime).Unix()})
		return nil
	}))
	m.HandleFunc("POST /v1/pair/claim", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Code   string
			Device DeviceInfo
		}
		if err := s.limit(r, "pair"); err != nil {
			writeErr(w, err)
			return
		}
		if err := readJSON(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := b.Device.clean(); err != nil {
			writeErr(w, err)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		h := sha(NormalizeCode(b.Code))
		c := s.codes[h]
		now := s.cfg.Now()
		if c == nil || now.After(c.Expires) {
			delete(s.codes, h)
			writeErr(w, fail(400, "bad_code", "That code is not valid or has expired. Make a new code on your other device (NEXUS Account › Link a device)."))
			return
		}
		delete(s.codes, h) // single use
		a := s.db.Accounts[c.AccountID]
		if a == nil {
			writeErr(w, fail(400, "bad_code", "That code is not valid or has expired."))
			return
		}
		secret := base64.RawURLEncoding.EncodeToString(randomBytes(24))
		cl := &claim{ID: newID(), SecretHash: sha(secret), AccountID: a.ID, Name: b.Device.Name, Platform: b.Device.Platform, Address: s.clientAddr(r), Status: "pending", Expires: now.Add(claimLifetime)}
		s.claims[cl.ID] = cl
		writeJSON(w, 201, map[string]any{"claimId": cl.ID, "claimSecret": secret, "account": maskEmail(a.Email), "displayName": a.DisplayName, "expires": cl.Expires.Unix()})
	})
	m.HandleFunc("GET /v1/pair/requests", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []map[string]any{}
		now := s.cfg.Now()
		for id, c := range s.claims {
			if now.After(c.Expires) {
				delete(s.claims, id)
				continue
			}
			if c.AccountID == au.acc.ID && c.Status == "pending" {
				out = append(out, map[string]any{"id": c.ID, "name": c.Name, "platform": c.Platform, "address": c.Address, "expires": c.Expires.Unix()})
			}
		}
		writeJSON(w, 200, map[string]any{"requests": out})
		return nil
	}))
	m.HandleFunc("POST /v1/pair/requests/{id}", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		var b struct{ Approve bool }
		if err := readJSON(r, &b); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		c := s.claims[r.PathValue("id")]
		if c == nil || c.AccountID != au.acc.ID || c.Status != "pending" || s.cfg.Now().After(c.Expires) {
			return fail(404, "no_request", "That request is no longer waiting.")
		}
		if !b.Approve {
			c.Status = "declined"
			writeJSON(w, 200, map[string]string{"status": "declined"})
			return nil
		}
		d, tok, err := s.newDeviceLocked(c.AccountID, DeviceInfo{Name: c.Name, Platform: c.Platform})
		if err != nil {
			return err
		}
		c.Status, c.token, c.device = "approved", tok, d
		if err := s.saveLocked(); err != nil {
			return err
		}
		s.cfg.Logger.Printf("account %s: device %s linked by code", c.AccountID, d.ID)
		writeJSON(w, 200, map[string]string{"status": "approved", "deviceId": d.ID})
		return nil
	}))
	m.HandleFunc("POST /v1/pair/claim/{id}", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ClaimSecret string }
		if err := readJSON(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		c := s.claims[r.PathValue("id")]
		if c == nil || subtle.ConstantTimeCompare([]byte(c.SecretHash), []byte(sha(b.ClaimSecret))) != 1 {
			writeErr(w, fail(404, "no_request", "This linking request has ended. Enter a new code."))
			return
		}
		if s.cfg.Now().After(c.Expires) && c.Status == "pending" {
			delete(s.claims, c.ID)
			writeJSON(w, 200, map[string]string{"status": "expired"})
			return
		}
		switch c.Status {
		case "approved":
			delete(s.claims, c.ID) // the token is handed over exactly once
			a := s.db.Accounts[c.AccountID]
			if a == nil || s.db.Devices[c.device.ID] == nil {
				writeErr(w, fail(404, "no_request", "This linking request has ended. Enter a new code."))
				return
			}
			writeJSON(w, 200, map[string]any{"status": "approved", "session": SessionResult{Token: c.token, DeviceID: c.device.ID, Account: s.viewLocked(a, c.device.ID)}})
		case "declined":
			delete(s.claims, c.ID)
			writeJSON(w, 200, map[string]string{"status": "declined"})
		default:
			writeJSON(w, 200, map[string]string{"status": "pending"})
		}
	})

	// ----- synchronisation
	m.HandleFunc("GET /v1/sync", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		var since int64
		fmt.Sscanf(r.URL.Query().Get("since"), "%d", &since)
		s.mu.Lock()
		defer s.mu.Unlock()
		it := s.itemsLocked(au.acc.ID)
		out := []*Item{}
		for _, x := range it.Items {
			if x.Rev > since {
				out = append(out, x)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Rev < out[j].Rev })
		more := false
		rev := it.Rev
		if len(out) > maxPageItems {
			out, more = out[:maxPageItems], true
			rev = out[len(out)-1].Rev
		}
		writeJSON(w, 200, map[string]any{"rev": rev, "items": out, "more": more})
		return nil
	}))
	m.HandleFunc("PUT /v1/sync/{kind}/{name}", with(func(w http.ResponseWriter, r *http.Request, au authed) error {
		key := r.PathValue("kind") + "/" + r.PathValue("name")
		if !keyRe.MatchString(key) {
			return fail(400, "bad_key", "NEXUS does not synchronise this kind of data.")
		}
		var b struct {
			BaseVersion int64
			Deleted     bool
			Data        []byte
		}
		if err := readJSON(r, &b); err != nil {
			return err
		}
		if len(b.Data) > maxItemBytes {
			return fail(413, "too_large", "This item is too large to synchronise.")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		it := s.itemsLocked(au.acc.ID)
		cur := it.Items[key]
		curVersion := int64(0)
		if cur != nil {
			curVersion = cur.Version
		}
		if b.BaseVersion != curVersion {
			writeJSON(w, 409, map[string]any{"error": "Changed on another device.", "code": "conflict", "current": cur})
			return nil
		}
		size := it.bytes + len(b.Data)
		if cur != nil {
			size -= len(cur.Data)
		}
		if size > maxAccountBytes || (cur == nil && len(it.Items) >= maxItems) {
			return fail(413, "quota", "Your NEXUS account's synchronised data is full.")
		}
		if b.Deleted {
			b.Data = nil
		}
		it.Rev++
		x := &Item{Key: key, Version: curVersion + 1, Rev: it.Rev, Deleted: b.Deleted, Data: b.Data, DeviceID: au.dev.ID, DeviceName: au.dev.Name, Updated: s.cfg.Now().Unix()}
		it.Items[key] = x
		it.bytes = size
		if b.Deleted {
			it.bytes -= len(b.Data)
		}
		if err := writeAtomic(s.itemsPath(au.acc.ID), it); err != nil {
			return err
		}
		writeJSON(w, 200, x)
		return nil
	}))
}

func (s *Server) passwordHash(a *Account) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return a.PasswordHash
}

func maskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at < 1 {
		return "***"
	}
	return e[:1] + strings.Repeat("•", min(at-1, 6)) + e[at:]
}
