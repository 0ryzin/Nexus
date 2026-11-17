package main

// NEXUS Account with GitHub (the default since NEXUS 0.6.0): no NEXUS server is needed.
//
//   - Sign-in: GitHub's device flow. NEXUS shows a short code; the user enters it at
//     github.com/login/device on any phone or computer and approves NEXUS. Handy on a Steam Deck:
//     no password is typed on it. The NEXUS OAuth app's client ID is public; device flow needs
//     no client secret, so the application contains no secret.
//   - Storage: one private (secret) Gist in the user's own GitHub account, "NEXUS sync data",
//     with one file per kind of data (users, library, preferences, emulation, amiibo, play time)
//     plus the device list. Each item keeps a version, the device that wrote it and when.
//   - Linking another PC or the Steam Deck: sign in with the same GitHub account there.
//   - Removing a device marks it removed in the device list; that device signs itself out the
//     next time it synchronises. (Its GitHub authorisation stays until the user revokes NEXUS at
//     github.com/settings/applications, which the interface links to.)
//   - Conflicts: an item is only written if its version is still the one this device based its
//     change on; otherwise the user chooses (account.go). A Gist has no atomic compare-and-set,
//     so two devices writing in the same second could still overwrite each other's change to
//     the same file; NEXUS re-reads before every write to keep that window small.
//   - Scope: "gist" only (and the public profile, which every token can read). NEXUS cannot
//     see repositories or other GitHub data.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var (
	githubClientID = "Ov23liHJxLIogCP7yvPE" // the NEXUS OAuth app (public identifier)
	githubWeb      = "https://github.com"
	githubAPI      = "https://api.github.com"
)

const (
	gistDescription = "NEXUS sync data - managed by NEXUS (github.com/0ryzin/Nexus); please do not edit"
	gistDevicesFile = "nexus-devices.json"
)

type ghFlow struct {
	DeviceCode string
	UserCode   string
	URI        string
	Interval   time.Duration
	Expires    time.Time
	DeviceName string
	next       time.Time
}

type ghDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Created  int64  `json:"created"`
	LastSeen int64  `json:"lastSeen"`
	Removed  bool   `json:"removed,omitempty"`
}

func (ac *Account) isGitHub() bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.st.SignedIn {
		return ac.st.Provider == "github"
	}
	return ac.serverLocked() == "" && githubClientID != ""
}

func (ac *Account) gh(method, rawURL string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	if strings.HasPrefix(rawURL, githubAPI) {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		ac.mu.Lock()
		if ac.token != "" {
			req.Header.Set("Authorization", "Bearer "+ac.token)
		}
		ac.mu.Unlock()
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ac.client.Do(req)
	if err != nil {
		return 0, errOffline
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode == 401 && strings.HasPrefix(rawURL, githubAPI) {
		// One 401 is not proof: GitHub can answer it for a moment during an incident. The sign-in
		// ends only when GitHub also refuses it for /user; otherwise it is tried again later.
		if rawURL != githubAPI+"/user" && ac.ghTokenStillValid() {
			return 401, errOffline
		}
		ac.endSession("GitHub no longer accepts NEXUS's sign-in on this device (it was revoked or expired). Sign in with GitHub again.")
		return 401, errors.New("Signed out: GitHub no longer accepts this device's sign-in.")
	}
	if resp.StatusCode >= 500 {
		return resp.StatusCode, errOffline
	}
	if out != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("GitHub sent an unexpected answer (%v)", err)
		}
	}
	if resp.StatusCode >= 300 {
		var e struct{ Message string }
		_ = json.Unmarshal(data, &e)
		if resp.StatusCode == 403 && strings.Contains(strings.ToLower(e.Message), "rate limit") {
			return resp.StatusCode, errors.New("GitHub asked NEXUS to wait a little (rate limit). NEXUS tries again later.")
		}
		return resp.StatusCode, fmt.Errorf("GitHub answered %s: %s", resp.Status, e.Message)
	}
	return resp.StatusCode, nil
}

// ghTokenStillValid asks GitHub whether the stored sign-in is still accepted (GET /user).
// Only a 401 there means no; a network error or any other answer counts as still valid.
func (ac *Account) ghTokenStillValid() bool {
	req, err := http.NewRequest("GET", githubAPI+"/user", nil)
	if err != nil {
		return true
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	ac.mu.Lock()
	req.Header.Set("Authorization", "Bearer "+ac.token)
	ac.mu.Unlock()
	resp, err := ac.client.Do(req)
	if err != nil {
		return true
	}
	resp.Body.Close()
	return resp.StatusCode != 401
}

// StartGitHubSignIn begins GitHub's device flow: the user enters the returned code at the URI.
func (ac *Account) StartGitHubSignIn(deviceName string) (map[string]any, error) {
	if githubClientID == "" {
		return nil, errors.New("GitHub sign-in is not configured in this build.")
	}
	ac.deviceInfo(deviceName)
	var r struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	form := url.Values{"client_id": {githubClientID}, "scope": {"gist"}}
	if _, err := ac.gh("POST", githubWeb+"/login/device/code?"+form.Encode(), nil, &r); err != nil {
		return nil, err
	}
	if r.Error != "" || r.DeviceCode == "" {
		return nil, fmt.Errorf("GitHub did not start the sign-in: %s", firstNonEmpty(r.ErrorDesc, r.Error, "no code"))
	}
	f := &ghFlow{DeviceCode: r.DeviceCode, UserCode: r.UserCode, URI: firstNonEmpty(r.VerificationURI, "https://github.com/login/device"),
		Interval: time.Duration(max(r.Interval, 5)) * time.Second, Expires: time.Now().Add(time.Duration(r.ExpiresIn) * time.Second), DeviceName: ac.thisDeviceName()}
	ac.mu.Lock()
	ac.ghFlow = f
	ac.mu.Unlock()
	ac.changed()
	return map[string]any{"userCode": f.UserCode, "uri": f.URI, "expires": f.Expires.Unix()}, nil
}

// GitHubSignInStatus polls GitHub (no faster than it allows): pending | approved | expired |
// denied | none.
func (ac *Account) GitHubSignInStatus() (string, error) {
	ac.mu.Lock()
	f := ac.ghFlow
	ac.mu.Unlock()
	if f == nil {
		return "none", nil
	}
	if time.Now().After(f.Expires) {
		ac.CancelGitHubSignIn()
		return "expired", nil
	}
	if time.Now().Before(f.next) {
		return "pending", nil
	}
	f.next = time.Now().Add(f.Interval)
	var r struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Interval    int    `json:"interval"`
	}
	form := url.Values{"client_id": {githubClientID}, "device_code": {f.DeviceCode}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}}
	if _, err := ac.gh("POST", githubWeb+"/login/oauth/access_token?"+form.Encode(), nil, &r); err != nil {
		return "pending", err
	}
	switch r.Error {
	case "authorization_pending":
		return "pending", nil
	case "slow_down":
		f.Interval = time.Duration(max(r.Interval, int(f.Interval.Seconds())+5)) * time.Second
		return "pending", nil
	case "expired_token":
		ac.CancelGitHubSignIn()
		return "expired", nil
	case "access_denied":
		ac.CancelGitHubSignIn()
		return "denied", nil
	case "":
	default:
		ac.CancelGitHubSignIn()
		return "denied", fmt.Errorf("GitHub refused the sign-in (%s).", r.Error)
	}
	if r.AccessToken == "" {
		return "pending", nil
	}
	ac.CancelGitHubSignIn()
	return "approved", ac.finishGitHubSignIn(r.AccessToken, f.DeviceName)
}

func (ac *Account) CancelGitHubSignIn() {
	ac.mu.Lock()
	ac.ghFlow = nil
	ac.mu.Unlock()
	ac.changed()
}

func (ac *Account) finishGitHubSignIn(token, deviceName string) error {
	ac.mu.Lock()
	prevToken := ac.token
	ac.token = token
	ac.mu.Unlock()
	fail := func(err error) error {
		ac.mu.Lock()
		ac.token = prevToken
		ac.mu.Unlock()
		return err
	}
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if _, err := ac.gh("GET", githubAPI+"/user", nil, &u); err != nil {
		return fail(err)
	}
	gistID, err := ac.findOrCreateGist()
	if err != nil {
		return fail(err)
	}
	if err := storeSecretFile(ac.tokenPath(), []byte(token)); err != nil {
		return fail(fmt.Errorf("NEXUS could not store the sign-in safely on this device: %v", err))
	}
	accountID := fmt.Sprintf("github:%d", u.ID)
	ac.mu.Lock()
	if ac.st.AccountID != accountID || ac.st.GistID != gistID {
		ac.st.Cursor, ac.st.Base, ac.st.ProfileMap, ac.st.Conflicts = 0, map[string]syncBase{}, nil, nil
	}
	ac.noteAccountSwitchLocked(accountID, "@"+u.Login)
	ac.st.Provider, ac.st.GistID, ac.st.AccountID = "github", gistID, accountID
	ac.st.Email, ac.st.DisplayName = "@"+u.Login, firstNonEmpty(u.Name, u.Login)
	ac.st.DeviceID, ac.st.DeviceName = newAmiiboEntryID()+newAmiiboEntryID(), firstNonEmpty(deviceName, ac.st.DeviceName, defaultDeviceName())
	ac.st.SignedIn, ac.st.Ended, ac.st.SyncOn = true, "", true
	ac.noteLocked("signin", fmt.Sprintf("Signed in with GitHub as @%s on %s.", u.Login, ac.st.DeviceName))
	ac.saveLocked()
	ac.mu.Unlock()
	if err := ac.ghUpdateDevices(func(m map[string]*ghDevice) {
		m[ac.st.DeviceID] = &ghDevice{ID: ac.st.DeviceID, Name: ac.st.DeviceName, Platform: devicePlatform(), Created: nowUnix(), LastSeen: nowUnix()}
	}); err != nil {
		ac.app.Log.Printf("account: device list: %v", err)
	}
	ac.app.Log.Printf("account: signed in with GitHub")
	ac.changed()
	ac.Kick()
	return nil
}

func (ac *Account) findOrCreateGist() (string, error) {
	for page := 1; page <= 10; page++ {
		var list []struct {
			ID          string                     `json:"id"`
			Description string                     `json:"description"`
			Files       map[string]json.RawMessage `json:"files"`
		}
		if _, err := ac.gh("GET", fmt.Sprintf("%s/gists?per_page=100&page=%d", githubAPI, page), nil, &list); err != nil {
			return "", err
		}
		for _, g := range list {
			if _, ok := g.Files[gistDevicesFile]; ok && g.Description == gistDescription {
				return g.ID, nil
			}
		}
		if len(list) < 100 {
			break
		}
	}
	var created struct {
		ID string `json:"id"`
	}
	body := map[string]any{"description": gistDescription, "public": false,
		"files": map[string]any{gistDevicesFile: map[string]string{"content": "{}"}}}
	if _, err := ac.gh("POST", githubAPI+"/gists", body, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", errors.New("GitHub did not create the NEXUS sync data.")
	}
	return created.ID, nil
}

type gistFile struct {
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	RawURL    string `json:"raw_url"`
}

// gistLoad reads every NEXUS file of the Gist.
func (ac *Account) gistLoad() (map[string]string, error) {
	ac.mu.Lock()
	id := ac.st.GistID
	ac.mu.Unlock()
	var g struct {
		Files map[string]gistFile `json:"files"`
	}
	code, err := ac.gh("GET", githubAPI+"/gists/"+url.PathEscape(id), nil, &g)
	if code == 404 { // deleted on GitHub: start a new one
		nid, err := ac.findOrCreateGist()
		if err != nil {
			return nil, err
		}
		ac.mu.Lock()
		ac.st.GistID, ac.st.Base = nid, map[string]syncBase{}
		ac.saveLocked()
		ac.mu.Unlock()
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for name, f := range g.Files {
		if !strings.HasPrefix(name, "nexus-") {
			continue
		}
		if f.Truncated && f.RawURL != "" { // larger than the API returns inline
			req, _ := http.NewRequest("GET", f.RawURL, nil)
			resp, err := ac.client.Do(req)
			if err != nil {
				return nil, errOffline
			}
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
			resp.Body.Close()
			f.Content = string(b)
		}
		out[name] = f.Content
	}
	return out, nil
}

func (ac *Account) gistWrite(files map[string]string) error {
	ac.mu.Lock()
	id := ac.st.GistID
	ac.mu.Unlock()
	fs := map[string]any{}
	for n, c := range files {
		fs[n] = map[string]string{"content": c}
	}
	_, err := ac.gh("PATCH", githubAPI+"/gists/"+url.PathEscape(id), map[string]any{"files": fs}, nil)
	return err
}

func parseDevices(s string) map[string]*ghDevice {
	m := map[string]*ghDevice{}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

func (ac *Account) ghUpdateDevices(fn func(map[string]*ghDevice)) error {
	files, err := ac.gistLoad()
	if err != nil {
		return err
	}
	m := parseDevices(files[gistDevicesFile])
	fn(m)
	b, _ := json.MarshalIndent(m, "", " ")
	if err := ac.gistWrite(map[string]string{gistDevicesFile: string(b)}); err != nil {
		return err
	}
	ac.setDevicesFrom(m)
	return nil
}

func (ac *Account) setDevicesFrom(m map[string]*ghDevice) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	var list []accountDevice
	for _, d := range m {
		if !d.Removed {
			list = append(list, accountDevice{ID: d.ID, Name: d.Name, Platform: d.Platform, Created: d.Created, LastSeen: d.LastSeen, Current: d.ID == ac.st.DeviceID})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created < list[j].Created })
	ac.st.Devices = list
	ac.saveLocked()
}

// ---------------------------------------------------------------- the sync backend

type gistBackend struct {
	ac    *Account
	files map[string]string
	items map[string]map[string]*remoteItem // file -> key -> item
	dirty map[string]bool
}

func gistFileFor(key string) string { return "nexus-" + keyCategory(key) + ".json" }

func (b *gistBackend) changes(int64) (map[string]remoteItem, int64, error) {
	files, err := b.ac.gistLoad()
	if err != nil {
		return nil, 0, err
	}
	b.files, b.items, b.dirty = files, map[string]map[string]*remoteItem{}, map[string]bool{}
	devs := parseDevices(files[gistDevicesFile])
	b.ac.mu.Lock()
	me := b.ac.st.DeviceID
	b.ac.mu.Unlock()
	if d := devs[me]; d != nil && d.Removed {
		b.ac.mu.Lock()
		b.ac.st.Provider = "github"
		b.ac.mu.Unlock()
		b.ac.endSession("This device was removed from your NEXUS account on another device. Sign in with GitHub again to use it.")
		return nil, 0, errors.New("This device was removed from your NEXUS account.")
	}
	out := map[string]remoteItem{}
	for name, content := range files {
		if name == gistDevicesFile {
			continue
		}
		m := map[string]*remoteItem{}
		if err := json.Unmarshal([]byte(content), &m); err != nil {
			return nil, 0, fmt.Errorf("the NEXUS sync data on GitHub (%s) is damaged: %v", name, err)
		}
		b.items[name] = m
		for k, it := range m {
			it.Key = k
			out[k] = *it
		}
	}
	b.ac.setDevicesFrom(devs)
	return out, 0, nil
}

func (b *gistBackend) put(key string, base int64, deleted bool, data []byte) (remoteItem, *remoteItem, error) {
	name := gistFileFor(key)
	m := b.items[name]
	if m == nil {
		m = map[string]*remoteItem{}
		b.items[name] = m
	}
	cur := m[key]
	ver := int64(0)
	if cur != nil {
		ver = cur.Version
	}
	if ver != base {
		c := *cur
		return remoteItem{}, &c, nil
	}
	b.ac.mu.Lock()
	dev, devName := b.ac.st.DeviceID, b.ac.st.DeviceName
	b.ac.mu.Unlock()
	if deleted {
		data = nil
	}
	it := &remoteItem{Key: key, Version: ver + 1, Deleted: deleted, Data: data, DeviceID: dev, DeviceName: devName, Updated: nowUnix()}
	m[key] = it
	b.dirty[name] = true
	return *it, nil, nil
}

func (b *gistBackend) flush() error {
	b.ac.mu.Lock()
	me := b.ac.st.DeviceID
	b.ac.mu.Unlock()
	devs := parseDevices(b.files[gistDevicesFile])
	if d := devs[me]; d != nil && nowUnix()-d.LastSeen > 6*3600 {
		d.LastSeen = nowUnix()
		bs, _ := json.MarshalIndent(devs, "", " ")
		b.files[gistDevicesFile] = string(bs)
		b.dirty[gistDevicesFile] = true
	}
	if len(b.dirty) == 0 {
		return nil
	}
	// re-read just before writing: items another device wrote meanwhile are kept (only this
	// device's items are replaced), and a version that moved on becomes a conflict next time
	fresh, err := b.ac.gistLoad()
	if err != nil {
		return err
	}
	out := map[string]string{}
	for name := range b.dirty {
		if name == gistDevicesFile {
			out[name] = b.files[name]
			continue
		}
		m := map[string]*remoteItem{}
		_ = json.Unmarshal([]byte(fresh[name]), &m)
		for k, it := range b.items[name] {
			if prev := m[k]; prev == nil || prev.Version < it.Version {
				m[k] = it
			}
		}
		for _, it := range m {
			it.Key = "" // the map key is the item key
		}
		bs, _ := json.Marshal(m)
		out[name] = string(bs)
	}
	return b.ac.gistWrite(out)
}

// ---------------------------------------------------------------- account operations (GitHub)

func (ac *Account) ghSignOut() {
	ac.mu.Lock()
	me := ac.st.DeviceID
	ac.mu.Unlock()
	_ = ac.ghUpdateDevices(func(m map[string]*ghDevice) {
		if d := m[me]; d != nil {
			d.Removed = true
		}
	})
}

func (ac *Account) ghRemoveDevice(id string) error {
	return ac.ghUpdateDevices(func(m map[string]*ghDevice) {
		if d := m[id]; d != nil {
			d.Removed = true
		}
	})
}

func (ac *Account) ghRenameDevice(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return errors.New("Give this device a name of up to 60 characters.")
	}
	ac.mu.Lock()
	me := ac.st.DeviceID
	ac.st.DeviceName = name
	ac.mu.Unlock()
	return ac.ghUpdateDevices(func(m map[string]*ghDevice) {
		if d := m[me]; d != nil {
			d.Name = name
		}
	})
}

func (ac *Account) ghRefresh() error {
	files, err := ac.gistLoad()
	if err != nil {
		return err
	}
	ac.setDevicesFrom(parseDevices(files[gistDevicesFile]))
	return nil
}
