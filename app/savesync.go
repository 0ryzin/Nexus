package main

// Game save synchronisation through the NEXUS account (GitHub).
//
// Each game save of each user (the folder "<GAME_ID>-<SHORT>" or "<GAME_ID>-<SHORT> (device)" in
// the user's save folder, saves.go) is one synchronised item:
//
//   - its data: a ZIP of the folder, base64, in a secret Gist of its own in the user's GitHub
//     account ("NEXUS save: <game> (<user>)", file save.zip.b64). The Gist's history keeps every
//     earlier version, so GitHub itself holds older copies too.
//   - its description: an entry in the index file nexussaves-index.json of the NEXUS sync Gist
//     (account_github.go): version, content hash, size, the Gist and the exact revision's raw URL,
//     and which device wrote it when.
//
// The content hash covers every save file (path and SHA-256), not times: a save that did not
// change is never sent again, and a save that arrived is checked against it before it is used.
//
// What happens to a save, from its content now (L), the version this device last synchronised
// (B, kept in save-sync.json) and the account's version (R):
//
//	L == R                          up to date (B is updated)
//	only L changed since B          sent (R+1)
//	only R changed since B          received: the device's copy is backed up first (Save Data ›
//	                                Backups, "before-sync"), the new one is unpacked, checked
//	                                against R's hash, and only then replaces the old folder
//	both changed, or no B and L!=R  conflict: nothing is overwritten on either side; the user
//	                                chooses (keep this device's / use the other device's)
//	L missing (never had it here)   received
//	R missing                       sent
//
// Nothing is ever deleted by synchronisation, on this device or in the account. Saves of a user
// whose game is running are left alone until it closes. Saves larger than maxSyncSave are not
// synchronised (shown as such). Sync runs with the account's own synchronisation (after a game
// closes, on start, on wake, Synchronise now), so offline changes simply wait for the next one.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	saveIndexFile = "nexussaves-index.json" // in the NEXUS sync Gist (not "nexus-": not an item file)
	saveBlobFile  = "save.zip.b64"
	maxSyncSave   = 20 << 20 // ZIP size; larger saves are not synchronised
)

// SaveMeta is the account's description of one save.
type SaveMeta struct {
	Version    int64  `json:"version"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`  // ZIP bytes
	Files      int    `json:"files"` // save files
	TitleID    string `json:"titleId"`
	Title      string `json:"title,omitempty"`
	User       string `json:"user"` // account user id
	UserName   string `json:"userName,omitempty"`
	Device     bool   `json:"device,omitempty"` // the game's device save
	Folder     string `json:"folder,omitempty"` // the folder's name on the device that sent it
	Gist       string `json:"gist"`
	Raw        string `json:"raw"`      // raw URL of exactly this version
	Modified   int64  `json:"modified"` // newest save file's time on the device that sent it
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Updated    int64  `json:"updated"`
}

type saveBase struct {
	Hash    string `json:"hash"`
	Version int64  `json:"version"`
}

// SaveSyncConflict: a save changed on this device and on another since they last agreed.
type SaveSyncConflict struct {
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	UserName      string   `json:"userName"`
	LocalModified int64    `json:"localModified"`
	Remote        SaveMeta `json:"remote"`
	Found         int64    `json:"found"`
}

type saveSyncState struct {
	Account   string                  `json:"account"` // the account the bases belong to
	Base      map[string]saveBase     `json:"base"`
	Conflicts map[string]SaveSyncConflict `json:"conflicts,omitempty"`
	// Resolve: the user's choice for a conflict, carried out at the next run ("local"|"remote")
	Resolve map[string]string `json:"resolve,omitempty"`
}

// SaveSyncStatus is shown in Settings › NEXUS Account › Game saves.
type SaveSyncStatus struct {
	Enabled    bool           `json:"enabled"`
	Supported  bool           `json:"supported"`
	State      string         `json:"state"` // idle | running | done | failed | offline
	Message    string         `json:"message,omitempty"`
	Last       int64          `json:"last,omitempty"`
	Saves      int            `json:"saves"`
	Pending    int            `json:"pending"`
	Sent       int            `json:"sent"`
	Received   int            `json:"received"`
	Conflicts  []SaveSyncConflict `json:"conflicts"`
	TooLarge   []string       `json:"tooLarge,omitempty"`
	WaitingFor []string       `json:"waitingFor,omitempty"` // users whose game is running
}

type SaveSync struct {
	app    *App
	mu     sync.Mutex
	runMu  sync.Mutex
	st     saveSyncState
	status SaveSyncStatus
}

func NewSaveSync(a *App) *SaveSync {
	s := &SaveSync{app: a}
	_ = readJSONFile(s.path(), &s.st)
	if s.st.Base == nil {
		s.st.Base = map[string]saveBase{}
	}
	if s.st.Conflicts == nil {
		s.st.Conflicts = map[string]SaveSyncConflict{}
	}
	if s.st.Resolve == nil {
		s.st.Resolve = map[string]string{}
	}
	s.status.State = "idle"
	return s
}

func (s *SaveSync) path() string { return filepath.Join(s.app.Paths.System, "save-sync.json") }

func (s *SaveSync) saveLocked() {
	if err := writeJSONAtomic(s.path(), s.st); err != nil {
		s.app.Log.Printf("save sync: %v", err)
	}
}

func readJSONFile(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Status for the interface.
func (s *SaveSync) Status() SaveSyncStatus {
	ac := s.app.Account
	ac.mu.Lock()
	signed, prov := ac.st.SignedIn, ac.st.Provider
	enabled := ac.st.SyncOn && savesCategoryOn(ac.st.Categories)
	ac.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	st.Supported = signed && prov == "github"
	st.Enabled = st.Supported && enabled
	st.Conflicts = []SaveSyncConflict{}
	for _, c := range s.st.Conflicts {
		st.Conflicts = append(st.Conflicts, c)
	}
	sort.Slice(st.Conflicts, func(i, j int) bool { return st.Conflicts[i].Key < st.Conflicts[j].Key })
	return st
}

func savesCategoryOn(c map[string]bool) bool {
	v, ok := c["saves"]
	return !ok || v // on unless switched off (accounts from before save sync have no entry)
}

// Resolve records the user's choice for a conflict and synchronises.
func (s *SaveSync) Resolve(key, keep string) error {
	if keep != "local" && keep != "remote" {
		return errors.New("Choose this device's save or the other device's.")
	}
	s.mu.Lock()
	if _, ok := s.st.Conflicts[key]; !ok {
		s.mu.Unlock()
		return errors.New("That save no longer needs a choice.")
	}
	s.st.Resolve[key] = keep
	s.saveLocked()
	s.mu.Unlock()
	_, err := s.Run()
	return err
}

// ---------------------------------------------------------------- local saves

type localSave struct {
	Key, ProfileID, Folder, Dir, TitleID, UserName string
	Device                                         bool
	Hash                                           string
	Files                                          int
	Modified                                       int64
}

func syncFileIgnored(name string) bool {
	return strings.HasPrefix(name, ".yuzu") || strings.HasPrefix(name, ".nexus") || strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".tmp")
}

// hashSaveDir: the content hash of a save folder (paths and SHA-256 of every save file).
func hashSaveDir(dir string) (hash string, files int, modified int64, err error) {
	type ent struct{ rel, sum string }
	var list []ent
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || syncFileIgnored(info.Name()) {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		list = append(list, ent{filepath.ToSlash(rel), hex.EncodeToString(h.Sum(nil))})
		if t := info.ModTime().Unix(); t > modified {
			modified = t
		}
		return nil
	})
	if err != nil {
		return "", 0, 0, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].rel < list[j].rel })
	h := sha256.New()
	for _, e := range list {
		fmt.Fprintf(h, "%s\x00%s\n", e.rel, e.sum)
	}
	return hex.EncodeToString(h.Sum(nil)), len(list), modified, nil
}

func saveKey(user, titleID string, device bool) string {
	k := user + "/" + strings.ToUpper(titleID)
	if device {
		k += ".device"
	}
	return k
}

func parseSaveKey(k string) (user, titleID string, device bool, ok bool) {
	user, rest, found := strings.Cut(k, "/")
	if !found || user == "" {
		return "", "", false, false
	}
	device = strings.HasSuffix(rest, ".device")
	rest = strings.TrimSuffix(rest, ".device")
	if !reTitleDir.MatchString(rest) {
		return "", "", false, false
	}
	return user, strings.ToUpper(rest), device, true
}

// localSaves lists this device's game saves by key; users with a running game are skipped and
// named in waiting.
func (s *SaveSync) localSaves(toAccount map[string]string) (map[string]localSave, []string) {
	a := s.app
	out := map[string]localSave{}
	var waiting []string
	for _, p := range a.Profiles.List() {
		if a.Saves.busy(p.ID) {
			waiting = append(waiting, p.Name)
			continue
		}
		user := p.ID
		if k, ok := toAccount[p.ID]; ok {
			user = k
		}
		dir := a.Saves.ProfileDir(p.ID)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			m := reGameFolder.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if !hasSaveData(full) {
				continue
			}
			hash, n, mod, err := hashSaveDir(full)
			if err != nil {
				a.Log.Printf("save sync: reading %s: %v", full, err)
				continue
			}
			key := saveKey(user, m[1], m[3] != "")
			out[key] = localSave{Key: key, ProfileID: p.ID, Folder: e.Name(), Dir: full, TitleID: strings.ToUpper(m[1]), UserName: p.Name,
				Device: m[3] != "", Hash: hash, Files: n, Modified: mod}
		}
	}
	return out, waiting
}

// ---------------------------------------------------------------- the account side (GitHub)

type saveRemote struct {
	ac *Account
}

// index reads nexussaves-index.json from the NEXUS sync Gist.
func (r saveRemote) index() (map[string]SaveMeta, error) {
	r.ac.mu.Lock()
	id := r.ac.st.GistID
	r.ac.mu.Unlock()
	if id == "" {
		return nil, errors.New("the NEXUS sync data on GitHub is not set up yet")
	}
	var g struct {
		Files map[string]gistFile `json:"files"`
	}
	if _, err := r.ac.gh("GET", githubAPI+"/gists/"+url.PathEscape(id), nil, &g); err != nil {
		return nil, err
	}
	out := map[string]SaveMeta{}
	f, ok := g.Files[saveIndexFile]
	if !ok {
		return out, nil
	}
	content := f.Content
	if f.Truncated && f.RawURL != "" {
		b, err := r.download(f.RawURL, 16<<20)
		if err != nil {
			return nil, err
		}
		content = string(b)
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("the save list on GitHub is damaged: %v", err)
	}
	return out, nil
}

// writeIndex stores changes: each entry replaces the account's only if the account still has
// the version it was based on (expect), read again just before writing. Returns the keys that
// moved on meanwhile (they become conflicts or downloads at the next run).
func (r saveRemote) writeIndex(changes map[string]SaveMeta, expect map[string]int64) ([]string, error) {
	cur, err := r.index()
	if err != nil {
		return nil, err
	}
	var lost []string
	for k, m := range changes {
		if cur[k].Version != expect[k] {
			lost = append(lost, k)
			continue
		}
		cur[k] = m
	}
	if len(lost) == len(changes) {
		return lost, nil
	}
	b, _ := json.MarshalIndent(cur, "", " ")
	r.ac.mu.Lock()
	id := r.ac.st.GistID
	r.ac.mu.Unlock()
	body := map[string]any{"files": map[string]any{saveIndexFile: map[string]string{"content": string(b)}}}
	_, err = r.ac.gh("PATCH", githubAPI+"/gists/"+url.PathEscape(id), body, nil)
	return lost, err
}

// putBlob stores a save's ZIP in its own Gist (new, or a new revision of gist) and returns the
// Gist and the raw URL of exactly this revision.
func (r saveRemote) putBlob(gist, description string, zipData []byte) (string, string, error) {
	files := map[string]any{saveBlobFile: map[string]string{"content": base64.StdEncoding.EncodeToString(zipData)}}
	var resp struct {
		ID    string              `json:"id"`
		Files map[string]gistFile `json:"files"`
	}
	var err error
	if gist != "" {
		var code int
		code, err = r.ac.gh("PATCH", githubAPI+"/gists/"+url.PathEscape(gist), map[string]any{"files": files}, &resp)
		if code == 404 { // removed on GitHub: a new one
			gist, err = "", nil
		}
	}
	if gist == "" && err == nil {
		_, err = r.ac.gh("POST", githubAPI+"/gists", map[string]any{"description": description, "public": false, "files": files}, &resp)
	}
	if err != nil {
		return "", "", err
	}
	f, ok := resp.Files[saveBlobFile]
	if resp.ID == "" || !ok || f.RawURL == "" {
		return "", "", errors.New("GitHub did not confirm the stored save")
	}
	return resp.ID, f.RawURL, nil
}

func (r saveRemote) download(raw string, limit int64) ([]byte, error) {
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, githubAPI) {
		return nil, errors.New("unexpected save address")
	}
	req, _ := http.NewRequest("GET", raw, nil)
	resp, err := r.ac.client.Do(req)
	if err != nil {
		return nil, errOffline
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub answered %d for a save", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errOffline
	}
	if int64(len(b)) > limit {
		return nil, errors.New("a save on GitHub is larger than NEXUS accepts")
	}
	return b, nil
}

// ---------------------------------------------------------------- one run

// Run synchronises every save once. Safe to call at any time; runs one at a time.
func (s *SaveSync) Run() (SaveSyncStatus, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	a := s.app
	ac := a.Account
	ac.mu.Lock()
	ok := ac.st.SignedIn && ac.st.Provider == "github" && ac.st.SyncOn && savesCategoryOn(ac.st.Categories) && !ac.st.HoldUploads
	account := ac.st.AccountID
	devID, devName := ac.st.DeviceID, ac.st.DeviceName
	pm := map[string]string{}
	for k, v := range ac.st.ProfileMap {
		pm[k] = v
	}
	ac.mu.Unlock()
	if !ok {
		return s.Status(), nil
	}
	toAccount := map[string]string{}
	for acc, local := range pm {
		toAccount[local] = acc
	}
	s.setStatus(func(st *SaveSyncStatus) { st.State, st.Message = "running", "" })

	s.mu.Lock()
	if s.st.Account != account { // another account: what this device knew does not apply
		s.st = saveSyncState{Account: account, Base: map[string]saveBase{}, Conflicts: map[string]SaveSyncConflict{}, Resolve: map[string]string{}}
		s.saveLocked()
	}
	s.mu.Unlock()

	remote := saveRemote{ac: ac}
	index, err := remote.index()
	if err != nil {
		return s.fail(err)
	}
	local, waiting := s.localSaves(toAccount)

	var sent, received int
	var tooLarge []string
	changes, expect := map[string]SaveMeta{}, map[string]int64{}
	keys := map[string]bool{}
	for k := range local {
		keys[k] = true
	}
	for k := range index {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	busyUser := map[string]bool{}
	for _, p := range a.Profiles.List() {
		if a.Saves.busy(p.ID) {
			busyUser[p.ID] = true
		}
	}
	for _, key := range sorted {
		l, haveL := local[key]
		r, haveR := index[key]
		s.mu.Lock()
		b, haveB := s.st.Base[key]
		choice := s.st.Resolve[key]
		s.mu.Unlock()
		user, tid, device, valid := parseSaveKey(key)
		if !valid {
			continue
		}
		localID := user
		if m, ok := pm[user]; ok {
			localID = m
		}
		if busyUser[localID] {
			continue // the user's game is running: after it closes
		}
		if _, exists := a.Profiles.Get(localID); !exists {
			continue // a user this device does not have (users synchronise first)
		}
		upload := func(base int64) {
			m, err := s.upload(remote, l, r, base+1, devID, devName)
			if err != nil {
				if errors.Is(err, errSaveTooLarge) {
					tooLarge = append(tooLarge, l.UserName+": "+firstNonEmpty(s.titleOf(tid), tid))
					return
				}
				a.Log.Printf("save sync: sending %s: %v", key, err)
				return
			}
			changes[key], expect[key] = m, base
		}
		switch {
		case choice == "local" && haveL:
			upload(r.Version)
		case choice == "remote" && haveR:
			if err := s.install(remote, r, localID, tid, device); err != nil {
				a.Log.Printf("save sync: receiving %s: %v", key, err)
				continue
			}
			received++
			s.agree(key, r.Hash, r.Version)
		case haveL && haveR && l.Hash == r.Hash:
			s.agree(key, r.Hash, r.Version)
		case haveL && !haveR:
			upload(0)
		case !haveL && haveR:
			if err := s.install(remote, r, localID, tid, device); err != nil {
				a.Log.Printf("save sync: receiving %s: %v", key, err)
				continue
			}
			received++
			s.agree(key, r.Hash, r.Version)
		case haveB && r.Version == b.Version && l.Hash != b.Hash: // only this device changed it
			upload(r.Version)
		case haveB && l.Hash == b.Hash && r.Version > b.Version: // only the account's changed
			if err := s.install(remote, r, localID, tid, device); err != nil {
				a.Log.Printf("save sync: receiving %s: %v", key, err)
				continue
			}
			received++
			s.agree(key, r.Hash, r.Version)
		default: // both changed, or never compared and different: the user chooses
			s.mu.Lock()
			if _, known := s.st.Conflicts[key]; !known {
				a.Log.Printf("save sync: %s changed here and on %s; waiting for a choice", key, r.DeviceName)
			}
			s.st.Conflicts[key] = SaveSyncConflict{Key: key, Title: firstNonEmpty(s.titleOf(tid), r.Title, tid), UserName: l.UserName, LocalModified: l.Modified, Remote: r, Found: nowUnix()}
			s.saveLocked()
			s.mu.Unlock()
		}
	}
	if len(changes) > 0 {
		lost, err := remote.writeIndex(changes, expect)
		if err != nil {
			return s.fail(err)
		}
		lostSet := map[string]bool{}
		for _, k := range lost {
			lostSet[k] = true
			a.Log.Printf("save sync: %s changed on another device meanwhile; compared again next time", k)
		}
		for k, m := range changes {
			if !lostSet[k] {
				sent++
				s.agree(k, m.Hash, m.Version)
			}
		}
	}
	pending := 0
	s.mu.Lock()
	for k, l := range local {
		if s.st.Base[k].Hash != l.Hash {
			pending++
		}
	}
	s.mu.Unlock()
	msg := "Game saves are up to date."
	if sent+received > 0 {
		msg = fmt.Sprintf("%d save(s) received, %d sent.", received, sent)
	}
	s.setStatus(func(st *SaveSyncStatus) {
		st.State, st.Message, st.Last = "done", msg, nowUnix()
		st.Saves, st.Pending, st.Sent, st.Received, st.TooLarge, st.WaitingFor = len(keys), pending, sent, received, tooLarge, waiting
	})
	if sent+received > 0 {
		a.Log.Printf("save sync: %s", msg)
		a.Events.Emit("saves.changed", nil)
	}
	a.Events.Emit("saves.sync", s.Status())
	return s.Status(), nil
}

var errSaveTooLarge = errors.New("save too large to synchronise")

func (s *SaveSync) titleOf(tid string) string { return s.app.Saves.titleNames()[strings.ToUpper(tid)] }

// agree records that this device and the account have the same save (and clears its conflict).
func (s *SaveSync) agree(key, hash string, version int64) {
	s.mu.Lock()
	s.st.Base[key] = saveBase{Hash: hash, Version: version}
	delete(s.st.Conflicts, key)
	delete(s.st.Resolve, key)
	s.saveLocked()
	s.mu.Unlock()
}

func (s *SaveSync) setStatus(fn func(*SaveSyncStatus)) {
	s.mu.Lock()
	fn(&s.status)
	s.mu.Unlock()
	s.app.Events.Emit("saves.sync", s.Status())
}

func (s *SaveSync) fail(err error) (SaveSyncStatus, error) {
	state := "failed"
	if errors.Is(err, errOffline) {
		state = "offline"
	}
	s.setStatus(func(st *SaveSyncStatus) { st.State, st.Message = state, err.Error() })
	if state == "failed" {
		s.app.Log.Printf("save sync: %v", err)
	}
	return s.Status(), err
}

// upload sends a save: its ZIP to its Gist, and returns its new description (not yet in the index).
func (s *SaveSync) upload(remote saveRemote, l localSave, r SaveMeta, version int64, devID, devName string) (SaveMeta, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := zipDir(zw, l.Dir, ""); err != nil {
		return SaveMeta{}, err
	}
	if err := zw.Close(); err != nil {
		return SaveMeta{}, err
	}
	if buf.Len() > maxSyncSave {
		return SaveMeta{}, errSaveTooLarge
	}
	title := s.titleOf(l.TitleID)
	desc := fmt.Sprintf("NEXUS save: %s (%s)", firstNonEmpty(title, l.Folder), l.UserName)
	if l.Device {
		desc = fmt.Sprintf("NEXUS save: %s (device save)", firstNonEmpty(title, l.Folder))
	}
	gist, raw, err := remote.putBlob(r.Gist, desc, buf.Bytes())
	if err != nil {
		return SaveMeta{}, err
	}
	user, _, _, _ := parseSaveKey(l.Key)
	return SaveMeta{Version: version, Hash: l.Hash, Size: int64(buf.Len()), Files: l.Files, TitleID: l.TitleID, Title: title, User: user, UserName: l.UserName,
		Device: l.Device, Folder: l.Folder, Gist: gist, Raw: raw, Modified: l.Modified, DeviceID: devID, DeviceName: devName, Updated: nowUnix()}, nil
}

// install receives a save into the user's folder: unpacked next to it, checked against the
// account's hash, the device's own copy backed up, then swapped in. Nothing is lost on failure.
func (s *SaveSync) install(remote saveRemote, r SaveMeta, profileID, tid string, device bool) error {
	a := s.app
	if a.Saves.busy(profileID) {
		return errSavesBusy
	}
	raw, err := remote.download(r.Raw, maxSyncSave*2)
	if err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return fmt.Errorf("the save from %s is damaged: %v", r.DeviceName, err)
	}
	dir := a.Saves.ProfileDir(profileID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dir, ".nexus-sync-new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage) // gone once renamed into place
	if err := unzipSafe(data, stage, maxSyncSave*8); err != nil {
		return fmt.Errorf("the save from %s could not be unpacked: %v", r.DeviceName, err)
	}
	if got, _, _, err := hashSaveDir(stage); err != nil || got != r.Hash {
		return fmt.Errorf("the save from %s does not match its description; it was not used", r.DeviceName)
	}
	name := gameDir(dir, tid, device)
	if name == "" {
		if t := firstNonEmpty(s.titleOf(tid), r.Title); t != "" {
			name = a.Saves.wantGameFolder(tid, t, device)
		} else if m := reGameFolder.FindStringSubmatch(r.Folder); m != nil && strings.EqualFold(m[1], tid) && (m[3] != "") == device {
			name = r.Folder // the sending device's name for it (its library knows the game)
		} else {
			name = a.Saves.wantGameFolder(tid, "", device)
		}
	}
	dst := filepath.Join(dir, name)
	old := ""
	if dirExists(dst) {
		if hasSaveData(dst) {
			if _, err := a.Saves.Backup(profileID, name, "before-sync"); err != nil {
				return fmt.Errorf("this device's save could not be backed up, so it was kept: %v", err)
			}
		}
		prev, err := os.MkdirTemp(dir, ".nexus-sync-old-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(prev) // the backup above holds this copy
		old = filepath.Join(prev, "save")
		if err := os.Rename(dst, old); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, dst); err != nil {
		if old != "" {
			_ = os.Rename(old, dst) // put this device's copy back
		}
		return err
	}
	a.Log.Printf("save sync: received %s for %s from %s (version %d)", name, profileID, r.DeviceName, r.Version)
	return nil
}

// unzipSafe extracts a ZIP into dir, refusing paths outside it, links and archives that unpack
// to more than limit bytes.
func unzipSafe(data []byte, dir string, limit int64) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	var total int64
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		// rooted ("/x"), drive ("C:x") and backslash names are refused on every system: on Windows
		// "/x" is not "absolute" for filepath.IsAbs but is rooted
		if name == "" || filepath.IsAbs(name) || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, ":") || strings.HasPrefix(filepath.Clean(name), "..") || strings.Contains(f.Name, "\\") {
			return fmt.Errorf("unsafe path %q", f.Name)
		}
		dst := filepath.Join(dir, filepath.Clean(name))
		if rel, err := filepath.Rel(dir, dst); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("unsafe path %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("%q is not a plain file", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		n, err := io.Copy(out, io.LimitReader(rc, limit-total+1))
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
		total += n
		if total > limit {
			return errors.New("the archive unpacks to more than NEXUS accepts")
		}
		_ = os.Chtimes(dst, f.Modified, f.Modified)
	}
	return nil
}

