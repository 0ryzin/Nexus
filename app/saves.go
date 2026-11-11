package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Save organisation.
//
// Every NEXUS user has one save folder, named "<profile id> - <profile name>", in NEXUS data\Saves.
// Inside it each game's saves live in a readable folder such as "BOTW - The Legend of Zelda
// Breath of the Wild". The engine itself stores saves under title-ID paths
// (user/save/0000000000000000/<console user>/<title id>); NEXUS keeps a mapping
// (.nexus-saves.json in the user's folder) between the readable folders and those paths:
//
//   - before a game starts, the user's readable folders are moved (renamed, same drive, instant)
//     to the paths the engine expects;
//   - when the session ends — or at the next start after a crash — they are moved back.
//
// Nothing is ever deleted or overwritten: name clashes get a " (2)" suffix and conflicts are left
// where they are and logged. Profile IDs, console user IDs and title IDs are never changed.

const saveMapFile = ".nexus-saves.json"

type saveEntry struct {
	TitleID    string `json:"titleId"`
	Game       string `json:"game"`
	EnginePath string `json:"enginePath,omitempty"` // slash path relative to the user's save folder ("" = not created by the engine yet)
	Kind       string `json:"kind"`                 // account | device | future-account | future-device | pending
}

type saveMap struct {
	Version int                   `json:"version"`
	Folders map[string]*saveEntry `json:"folders"` // readable folder name -> entry
	Session []string              `json:"session,omitempty"`
}

type SaveFolder struct {
	Folder  string `json:"folder"`
	Path    string `json:"path"`
	TitleID string `json:"titleId"`
	Game    string `json:"game"`
	Bytes   int64  `json:"bytes"`
	Files   int    `json:"files"`
	Kind    string `json:"kind"`
}

type SaveOrganizer struct {
	app *App
	mu  sync.Mutex
}

func NewSaveOrganizer(a *App) *SaveOrganizer { return &SaveOrganizer{app: a} }

var (
	reTitleDir   = regexp.MustCompile(`^[0-9A-Fa-f]{16}$`)
	reUserDir    = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)
	reBadWinChar = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	winReserved  = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "COM1": true, "COM2": true, "COM3": true,
		"COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true, "LPT1": true, "LPT2": true, "LPT3": true}
)

// safeFolderName makes a name valid as a Windows folder name.
func safeFolderName(s string) string {
	s = reBadWinChar.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimRight(s, ". ")
	if len([]rune(s)) > 80 {
		s = strings.TrimRight(string([]rune(s)[:80]), ". ")
	}
	if s == "" || winReserved[strings.ToUpper(s)] {
		s = "_" + s
	}
	return s
}

// gameCodes: well-known short names. Games not listed use their full name.
var gameCodes = []struct {
	re   *regexp.Regexp
	code string
}{
	{regexp.MustCompile(`(?i)tears of the kingdom`), "TOTK"},
	{regexp.MustCompile(`(?i)breath of the wild`), "BOTW"},
	{regexp.MustCompile(`(?i)ocarina of time`), "OOT"},
	{regexp.MustCompile(`(?i)link'?s awakening`), "LA"},
	{regexp.MustCompile(`(?i)skyward sword`), "SSHD"},
	{regexp.MustCompile(`(?i)echoes of wisdom`), "EOW"},
	{regexp.MustCompile(`(?i)mario kart 8`), "MK8DX"},
	{regexp.MustCompile(`(?i)mario kart world`), "MKW"},
	{regexp.MustCompile(`(?i)super mario odyssey`), "SMO"},
	{regexp.MustCompile(`(?i)super mario bros\.? wonder`), "SMBW"},
	{regexp.MustCompile(`(?i)super mario 3d world`), "SM3DW"},
	{regexp.MustCompile(`(?i)smash bros\.? ultimate`), "SSBU"},
	{regexp.MustCompile(`(?i)animal crossing:? new horizons`), "ACNH"},
	{regexp.MustCompile(`(?i)metroid dread`), "MD"},
	{regexp.MustCompile(`(?i)metroid prime 4`), "MP4"},
	{regexp.MustCompile(`(?i)metroid prime remastered`), "MPR"},
	{regexp.MustCompile(`(?i)splatoon 3`), "S3"},
	{regexp.MustCompile(`(?i)splatoon 2`), "S2"},
	{regexp.MustCompile(`(?i)xenoblade chronicles 3`), "XC3"},
	{regexp.MustCompile(`(?i)xenoblade chronicles 2`), "XC2"},
	{regexp.MustCompile(`(?i)fire emblem:? three houses`), "FE3H"},
	{regexp.MustCompile(`(?i)pok[eé]mon (scarlet|violet)`), "PKMN SV"},
	{regexp.MustCompile(`(?i)pok[eé]mon legends:? arceus`), "PLA"},
	{regexp.MustCompile(`(?i)kirby and the forgotten land`), "KATFL"},
	{regexp.MustCompile(`(?i)luigi'?s mansion 3`), "LM3"},
}

func gameCode(title string) string {
	for _, g := range gameCodes {
		if g.re.MatchString(title) {
			return g.code
		}
	}
	return ""
}

// GameFolderName is the readable folder for a game: "CODE - Name", or just the name.
func GameFolderName(title, titleID string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = "Unknown game " + strings.ToUpper(titleID)
	}
	if c := gameCode(name); c != "" {
		name = c + " - " + name
	}
	return safeFolderName(name)
}

func (o *SaveOrganizer) savesRoot() string { return o.app.Paths.Saves }

// ProfileDir is the user's save folder (see ProfileStore.SaveDir).
func (o *SaveOrganizer) ProfileDir(id string) string { return o.app.Profiles.SaveDir(id) }

func (o *SaveOrganizer) desiredProfileName(id string) string {
	name := ""
	if o.app.Profiles != nil {
		if p, ok := o.app.Profiles.Get(id); ok {
			name = p.Name
		}
	}
	if strings.TrimSpace(name) == "" {
		return id
	}
	return id + " - " + safeFolderName(name)
}

// SyncProfileName renames the user's save folder after a name change (never while that user is
// playing). The profile ID stays at the start of the name, which is how NEXUS finds it.
func (o *SaveOrganizer) SyncProfileName(id string) {
	if st := o.app.Emu.State(); st.Running && st.ProfileID == id {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	cur := o.ProfileDir(id)
	want := filepath.Join(o.savesRoot(), o.desiredProfileName(id))
	if cur == want || !dirExists(cur) || fileExists(want) || dirExists(want) {
		return
	}
	if err := os.Rename(cur, want); err != nil {
		o.app.Log.Printf("saves: could not rename %s: %v", cur, err)
	}
}

func (o *SaveOrganizer) loadMap(dir string) *saveMap {
	m := &saveMap{Version: 1, Folders: map[string]*saveEntry{}}
	_ = readJSON(filepath.Join(dir, saveMapFile), m)
	if m.Folders == nil {
		m.Folders = map[string]*saveEntry{}
	}
	return m
}

func (o *SaveOrganizer) saveMapTo(dir string, m *saveMap) {
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	p := filepath.Join(dir, saveMapFile)
	tmp := p + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		clearHidden(p) // Windows refuses to replace a hidden file by rename
		if os.Rename(tmp, p) != nil {
			_ = os.Remove(tmp)
		}
	}
	setHidden(p)
}

// titleNames: library titles by base title ID.
func (o *SaveOrganizer) titleNames() map[string]string {
	out := map[string]string{}
	for _, g := range o.app.Library.Games() {
		if g.TitleID != "" && g.Kind != "update" && g.Kind != "dlc" {
			out[strings.ToUpper(g.TitleID)] = g.Title
		}
	}
	return out
}

func gameNameFor(names map[string]string, titleID string) string {
	id := strings.ToUpper(titleID)
	if n := names[id]; n != "" {
		return n
	}
	if v, err := strconv.ParseUint(id, 16, 64); err == nil {
		if base, _ := classifyID(v); base != v {
			if n := names[fmt.Sprintf("%016X", base)]; n != "" {
				return n
			}
		}
	}
	return ""
}

func uniqueFolder(dir, name string, m *saveMap) string {
	cand := name
	for i := 2; ; i++ {
		_, mapped := m.Folders[cand]
		if !mapped && !fileExists(filepath.Join(dir, cand)) && !dirExists(filepath.Join(dir, cand)) {
			return cand
		}
		cand = fmt.Sprintf("%s (%d)", name, i)
	}
}

// engineUnits finds the engine's per-game save folders inside a user's save folder.
func engineUnits(dir string) []struct{ rel, title, kind string } {
	var out []struct{ rel, title, kind string }
	base := filepath.Join(dir, "user", "save")
	users, _ := os.ReadDir(filepath.Join(base, "0000000000000000"))
	for _, u := range users {
		if !u.IsDir() || !reUserDir.MatchString(u.Name()) {
			continue
		}
		titles, _ := os.ReadDir(filepath.Join(base, "0000000000000000", u.Name()))
		for _, t := range titles {
			if t.IsDir() && reTitleDir.MatchString(t.Name()) {
				kind := "account"
				if strings.Trim(u.Name(), "0") == "" {
					kind = "device"
				}
				out = append(out, struct{ rel, title, kind string }{"user/save/0000000000000000/" + u.Name() + "/" + t.Name(), strings.ToUpper(t.Name()), kind})
			}
		}
	}
	accts, _ := os.ReadDir(filepath.Join(base, "account"))
	for _, a := range accts {
		titles, _ := os.ReadDir(filepath.Join(base, "account", a.Name()))
		for _, t := range titles {
			if t.IsDir() && reTitleDir.MatchString(t.Name()) {
				out = append(out, struct{ rel, title, kind string }{"user/save/account/" + a.Name() + "/" + t.Name(), strings.ToUpper(t.Name()), "future-account"})
			}
		}
	}
	devs, _ := os.ReadDir(filepath.Join(base, "device"))
	for _, t := range devs {
		if t.IsDir() && reTitleDir.MatchString(t.Name()) {
			out = append(out, struct{ rel, title, kind string }{"user/save/device/" + t.Name(), strings.ToUpper(t.Name()), "future-device"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out
}

func dirEmpty(p string) bool {
	es, err := os.ReadDir(p)
	return err == nil && len(es) == 0
}

// Organize moves the engine's title-ID save folders of one user into readable folders.
// Safe to run at any time the user is not playing; it never overwrites or deletes saves.
func (o *SaveOrganizer) Organize(profileID string) (moved int, err error) {
	if st := o.app.Emu.State(); st.Running && st.ProfileID == profileID {
		return 0, nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	dir := o.ProfileDir(profileID)
	if !dirExists(dir) {
		return 0, nil
	}
	m := o.loadMap(dir)
	names := o.titleNames()
	// reverse index: engine path -> folder
	byPath := map[string]string{}
	for f, e := range m.Folders {
		if e.EnginePath != "" {
			byPath[e.EnginePath] = f
		}
	}
	units := engineUnits(dir)
	counts := map[string]int{}
	for _, u := range units {
		counts[u.title]++
	}
	for _, u := range units {
		src := filepath.Join(dir, filepath.FromSlash(u.rel))
		if dirEmpty(src) {
			continue // the engine creates empty folders for games without saves yet
		}
		game := gameNameFor(names, u.title)
		folder, known := byPath[u.rel]
		if known {
			dst := filepath.Join(dir, folder)
			if dirExists(dst) && dirEmpty(dst) {
				_ = os.Remove(dst)
			} else if dirExists(dst) || fileExists(dst) {
				// the readable folder has other data (e.g. copied in meanwhile): keep both
				m.Folders[folder].EnginePath, m.Folders[folder].Kind = "", "pending"
				known = false
			}
		}
		if !known {
			name := GameFolderName(game, u.title)
			if counts[u.title] > 1 && (u.kind == "device" || u.kind == "future-device") {
				name += " (shared device save)"
			}
			if e := m.Folders[name]; e != nil && e.EnginePath == "" && e.TitleID == u.title && dirExists(filepath.Join(dir, name)) && dirEmpty(filepath.Join(dir, name)) {
				_ = os.Remove(filepath.Join(dir, name)) // the empty placeholder made for this game
				folder = name
			} else {
				folder = uniqueFolder(dir, name, m)
			}
		}
		dst := filepath.Join(dir, folder)
		if err := os.Rename(src, dst); err != nil {
			o.app.Log.Printf("saves: could not move %s: %v", src, err)
			continue
		}
		e := m.Folders[folder]
		if e == nil {
			e = &saveEntry{}
			m.Folders[folder] = e
		}
		e.EnginePath, e.Kind, e.TitleID = u.rel, u.kind, u.title
		if game != "" {
			e.Game = game
		}
		moved++
	}
	m.Session = nil
	o.saveMapTo(dir, m)
	o.pruneEmpty(filepath.Join(dir, "user"))
	if dirExists(filepath.Join(dir, "user")) {
		setHidden(filepath.Join(dir, "user"))
	}
	if moved > 0 {
		o.app.Log.Printf("saves: organised %d game folder(s) for %s", moved, profileID)
	}
	return moved, nil
}

// pruneEmpty removes empty directories under root (never files).
func (o *SaveOrganizer) pruneEmpty(root string) {
	var dirs []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		if dirEmpty(dirs[i]) {
			_ = os.Remove(dirs[i])
		}
	}
}

// EnsureGameFolders creates empty readable folders for the library's games, so a user can drop
// save files into the right place. Empty folders are ignored when a game starts.
func (o *SaveOrganizer) EnsureGameFolders(profileID string) {
	if st := o.app.Emu.State(); st.Running && st.ProfileID == profileID {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	dir := o.ProfileDir(profileID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	m := o.loadMap(dir)
	have := map[string]bool{}
	for _, e := range m.Folders {
		have[e.TitleID] = true
	}
	for id, title := range o.titleNames() {
		if have[id] {
			continue
		}
		name := GameFolderName(title, id)
		if _, used := m.Folders[name]; used || dirExists(filepath.Join(dir, name)) {
			continue
		}
		if os.MkdirAll(filepath.Join(dir, name), 0o755) == nil {
			m.Folders[name] = &saveEntry{TitleID: id, Game: title, Kind: "pending"}
		}
	}
	o.saveMapTo(dir, m)
}

// consoleUser returns the engine's first console user as its save-path folder name
// (from NAND system save 8000000000000010 profiles.dat), "" if it does not exist yet.
func (o *SaveOrganizer) consoleUser() string {
	p := filepath.Join(o.app.Emu.DataDir(), "nand", "system", "save", "8000000000000010", "su", "avators", "profiles.dat")
	data, err := os.ReadFile(p)
	if err != nil || len(data) < 0x10+0xC8 {
		return ""
	}
	for off := 0x10; off+0xC8 <= len(data) && off < 0x10+8*0xC8; off += 0xC8 {
		lo := binary.LittleEndian.Uint64(data[off:])
		hi := binary.LittleEndian.Uint64(data[off+8:])
		if lo != 0 || hi != 0 {
			return fmt.Sprintf("%016X%016X", hi, lo)
		}
	}
	return ""
}

// Assemble puts the user's readable folders where the engine expects them, right before a game
// starts. Folders that the engine has not created yet (dropped in by the user) go to the first
// console user's path. Returns the number of folders placed.
func (o *SaveOrganizer) Assemble(profileID string) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	dir := o.ProfileDir(profileID)
	if !dirExists(dir) {
		return 0, nil
	}
	m := o.loadMap(dir)
	// folders created by the user by hand, named after a library game
	names := o.titleNames()
	byName := map[string]string{}
	for id, t := range names {
		byName[strings.ToLower(GameFolderName(t, id))] = id
	}
	entries, _ := os.ReadDir(dir)
	for _, en := range entries {
		if !en.IsDir() || en.Name() == "user" || strings.HasPrefix(en.Name(), ".") {
			continue
		}
		if _, ok := m.Folders[en.Name()]; ok {
			continue
		}
		if id := byName[strings.ToLower(en.Name())]; id != "" {
			m.Folders[en.Name()] = &saveEntry{TitleID: id, Game: names[id], Kind: "pending"}
		} else if t := reTitleID.FindString(en.Name()); t != "" {
			m.Folders[en.Name()] = &saveEntry{TitleID: strings.ToUpper(t), Kind: "pending"}
		}
	}
	user := o.consoleUser()
	placed := 0
	var session []string
	for folder, e := range m.Folders {
		src := filepath.Join(dir, folder)
		if !dirExists(src) || dirEmpty(src) || (e.TitleID == "" && e.EnginePath == "") {
			continue
		}
		rel := e.EnginePath
		if rel == "" {
			if user == "" {
				o.app.Log.Printf("saves: %q waits until the game has been started once in NEXUS (no console user yet)", folder)
				continue
			}
			rel = "user/save/0000000000000000/" + user + "/" + e.TitleID
			e.EnginePath, e.Kind = rel, "account"
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if dirExists(dst) && !dirEmpty(dst) {
			o.app.Log.Printf("saves: %q not placed: %s already has data", folder, rel)
			continue
		}
		_ = os.Remove(dst) // empty placeholder left by the engine
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			o.app.Log.Printf("saves: could not place %q: %v", folder, err)
			continue
		}
		session = append(session, folder)
		placed++
	}
	m.Session = session
	o.saveMapTo(dir, m)
	return placed, nil
}

// OrganizeAll runs at startup: brings back folders of a session that ended with a crash,
// migrates "<id>" folders to "<id> - <name>" and organises title-ID folders.
func (o *SaveOrganizer) OrganizeAll() {
	if o.app.Profiles == nil {
		return
	}
	for _, p := range o.app.Profiles.List() {
		o.SyncProfileName(p.ID)
		if _, err := o.Organize(p.ID); err != nil {
			o.app.Log.Printf("saves: %v", err)
		}
	}
}

// List returns a user's readable save folders (with data).
func (o *SaveOrganizer) List(profileID string) []SaveFolder {
	dir := o.ProfileDir(profileID)
	m := o.loadMap(dir)
	out := []SaveFolder{}
	entries, _ := os.ReadDir(dir)
	for _, en := range entries {
		if !en.IsDir() || en.Name() == "user" || strings.HasPrefix(en.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, en.Name())
		b, n := dirSize(p)
		sf := SaveFolder{Folder: en.Name(), Path: p, Bytes: b, Files: n}
		if e := m.Folders[en.Name()]; e != nil {
			sf.TitleID, sf.Game, sf.Kind = e.TitleID, e.Game, e.Kind
		}
		if n > 0 {
			out = append(out, sf)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Folder) < strings.ToLower(out[j].Folder) })
	return out
}

// Export writes a ZIP of the user's readable save folders and the mapping file.
func (o *SaveOrganizer) Export(profileID string, w io.Writer) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	dir := o.app.Profiles.SaveDir(profileID)
	zw := zip.NewWriter(w)
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "user" || strings.HasPrefix(rel, "user/") { // engine cache/temp, not saves
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			_, err := zw.Create(rel + "/")
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		hw, err := zw.CreateHeader(&zip.FileHeader{Name: rel, Method: zip.Deflate, Modified: info.ModTime()})
		if err != nil {
			return err
		}
		_, err = io.Copy(hw, f)
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	return err
}

// ImportZip adds the game folders from an exported ZIP to the user. Existing folders are never
// replaced: an imported folder whose name is taken is added as "<name> (2)".
func (o *SaveOrganizer) ImportZip(profileID string, data []byte) (int, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, errors.New("This is not a ZIP file.")
	}
	var src saveMap
	for _, f := range zr.File {
		if f.Name == saveMapFile {
			if rc, err := f.Open(); err == nil {
				_ = json.NewDecoder(rc).Decode(&src)
				rc.Close()
			}
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	dir := o.app.Profiles.SaveDir(profileID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	m := o.loadMap(dir)
	rename := map[string]string{} // top folder in zip -> folder here
	n := 0
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if name == saveMapFile || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			continue
		}
		top, rest, _ := strings.Cut(name, "/")
		if top == "" || top == "user" || strings.HasPrefix(top, ".") {
			continue
		}
		dst, ok := rename[top]
		if !ok {
			dst = uniqueFolder(dir, safeFolderName(top), m)
			rename[top] = dst
			e := &saveEntry{Kind: "pending"}
			if se := src.Folders[top]; se != nil {
				e.TitleID, e.Game = se.TitleID, se.Game // console user paths differ between PCs: placed at the next launch
			} else if t := reTitleID.FindString(top); t != "" {
				e.TitleID = strings.ToUpper(t)
			}
			m.Folders[dst] = e
			if err := os.MkdirAll(filepath.Join(dir, dst), 0o755); err != nil {
				return n, err
			}
			n++
		}
		if rest == "" || strings.HasSuffix(name, "/") {
			_ = os.MkdirAll(filepath.Join(dir, dst, filepath.FromSlash(rest)), 0o755)
			continue
		}
		target := filepath.Join(dir, dst, filepath.FromSlash(rest))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return n, err
		}
		rc, err := f.Open()
		if err != nil {
			return n, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			rc.Close()
			return n, err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return n, err
		}
	}
	// folders without a known game cannot be placed for the engine; keep them, but say so
	for k, e := range m.Folders {
		if e.TitleID == "" {
			o.app.Log.Printf("saves: imported folder %q has no known game; it is kept but not used by games", k)
		}
	}
	o.saveMapTo(dir, m)
	return n, nil
}
