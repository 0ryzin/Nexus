package main

// Save data (NEXUS 0.3.0 layout).
//
//	<data>\UserData\Saves\
//	  <PLAYER_ID>-<Profile name>\        one folder per NEXUS user; PLAYER_ID is that user's
//	                                      32-hex player identifier (the engine's user ID form)
//	    <GAME_ID>-<SHORT>\                the game's save, exactly as the game wrote it
//	      slot_00\ … account.sav option.sav
//	    <GAME_ID>-<SHORT> (device)\       the game's device save, if it has one
//	    user\                             engine caches and temporary data (hidden)
//
// The NEXUS engine is told, for every library game, which folder holds its account and device
// save (request "saveFolders"), and reads and writes there directly: nothing is moved before or
// after a session. Each NEXUS user has their own folder, given to the engine as its save
// directory, so users never see each other's saves.
//
// Save data is never deleted or overwritten without the user's confirmation, and never without a
// backup first (UserData\Backups\Saves). Saves written by an older engine in its own layout
// (user\save\0000000000000000\<user>\<game>) are adopted into the layout above at the end of a
// session or at start-up; a game folder that already has data is never replaced by them — the
// other copy is kept next to it as "<GAME_ID>-<SHORT> (other copy N)" and shown in Save Data.

import (
	"archive/zip"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
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
	"time"
)

const (
	saveLayoutFile   = ".nexus-layout.json" // in the Saves root: layout version
	saveLayoutV      = 2
	oldSaveMapFile   = ".nexus-saves.json" // 0.2.x readable-folder mapping (kept, renamed)
	saveExportMarker = "nexus-saves.json"  // in exported ZIPs
)

var (
	reTitleDir   = regexp.MustCompile(`^[0-9A-Fa-f]{16}$`)
	reUserDir    = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)
	reBadWinChar = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	reGameFolder = regexp.MustCompile(`^([0-9A-Fa-f]{16})(-[^()]*)?( \(device\))?$`)
	reOtherCopy  = regexp.MustCompile(`^([0-9A-Fa-f]{16})(-[^()]*)?( \(device\))? \(other copy \d+\)$`)
	rePlayerDir  = regexp.MustCompile(`^([0-9A-Fa-f]{32})(-.*)?$`)
	reSlotDir    = regexp.MustCompile(`(?i)^slot_?(\d+)$`)
	winReserved  = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "COM1": true, "COM2": true, "COM3": true,
		"COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true, "LPT1": true, "LPT2": true, "LPT3": true,
		"LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}
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

// gameCodes: well-known short names.
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
	{regexp.MustCompile(`(?i)pok[eé]mon (scarlet|violet)`), "PKMNSV"},
	{regexp.MustCompile(`(?i)pok[eé]mon legends:? arceus`), "PLA"},
	{regexp.MustCompile(`(?i)kirby and the forgotten land`), "KATFL"},
	{regexp.MustCompile(`(?i)luigi'?s mansion 3`), "LM3"},
	{regexp.MustCompile(`(?i)donkey kong bananza`), "DKB"},
}

var smallWords = map[string]bool{"the": true, "of": true, "and": true, "a": true, "an": true, "to": true, "in": true, "for": true, "on": true, "at": true, "de": true, "la": true, "le": true}

// GameShortName is the short identifier used in save folder names: a well-known code (OOT,
// BOTW…) or the initials of the title ("Cloud Garden" → "CG", "Mario Kart 8" → "MK8").
func GameShortName(title string) string {
	for _, g := range gameCodes {
		if g.re.MatchString(title) {
			return g.code
		}
	}
	var b strings.Builder
	for _, w := range strings.FieldsFunc(title, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 127)
	}) {
		if smallWords[strings.ToLower(w)] {
			continue
		}
		r := []rune(w)
		if r[0] >= '0' && r[0] <= '9' {
			b.WriteString(w)
		} else {
			b.WriteString(strings.ToUpper(string(r[0])))
		}
		if b.Len() >= 8 {
			break
		}
	}
	s := safeFolderName(b.String())
	if s == "" || strings.HasPrefix(s, "_") || strings.ContainsAny(s, "()") {
		return ""
	}
	return s
}

// GameSaveFolderName is "<GAME_ID>-<SHORT>" (or "<GAME_ID>" when the game is unknown).
func GameSaveFolderName(titleID, title string) string {
	id := strings.ToUpper(titleID)
	if sh := GameShortName(title); sh != "" {
		return id + "-" + sh
	}
	return id
}

// itemLabel names a file or folder of a game's save for people: slot_00 → "Slot 00",
// account.sav → "Account Data".
func itemLabel(name string, dir bool) string {
	if m := reSlotDir.FindStringSubmatch(name); m != nil && dir {
		n, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("Slot %02d", n)
	}
	if !dir && strings.EqualFold(filepath.Ext(name), ".sav") {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		base = strings.ReplaceAll(strings.ReplaceAll(base, "_", " "), "-", " ")
		if base != "" {
			return strings.ToUpper(base[:1]) + base[1:] + " Data"
		}
	}
	return name
}

// ---------- types shown in Save Data

type SaveItem struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Dir      bool   `json:"dir"`
	Bytes    int64  `json:"bytes"`
	Modified int64  `json:"modified"`
}

type SaveGame struct {
	TitleID  string     `json:"titleId"`
	Title    string     `json:"title"`
	InLib    bool       `json:"inLibrary"`
	Folder   string     `json:"folder"`
	Kind     string     `json:"kind"` // account | device | other-copy
	Bytes    int64      `json:"bytes"`
	Files    int        `json:"files"`
	Modified int64      `json:"modified"`
	Items    []SaveItem `json:"items"`
}

type SavePlayer struct {
	ProfileID string     `json:"profileId"`
	Name      string     `json:"name"`
	PlayerID  string     `json:"playerId"`
	Folder    string     `json:"folder"`
	Path      string     `json:"path"`
	Games     []SaveGame `json:"games"`
	Other     []string   `json:"otherFolders"` // folders NEXUS cannot place (no game ID)
}

type SaveBackup struct {
	File    string `json:"file"`
	TitleID string `json:"titleId,omitempty"`
	Reason  string `json:"reason"`
	Created int64  `json:"created"`
	Bytes   int64  `json:"bytes"`
}

// ---------- organiser

type SaveOrganizer struct {
	app *App
	mu  sync.Mutex
}

func NewSaveOrganizer(a *App) *SaveOrganizer { return &SaveOrganizer{app: a} }

func (o *SaveOrganizer) root() string { return o.app.Paths.Saves }
func (o *SaveOrganizer) backupsRoot() string {
	return filepath.Join(o.app.Paths.UserData, "Backups", "Saves")
}

// ProfileDir is the user's save folder (see ProfileStore.SaveDir).
func (o *SaveOrganizer) ProfileDir(id string) string { return o.app.Profiles.SaveDir(id) }

// busy: a game of this user is running (or suspended) — their saves are not touched meanwhile.
func (o *SaveOrganizer) busy(profileID string) bool {
	st := o.app.Emu.State()
	return st.Running && (profileID == "" || st.ProfileID == profileID)
}

var errSavesBusy = errors.New("Close the game this user is playing first.")

func newPlayerID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

// titleNames: library titles by base title ID.
func (o *SaveOrganizer) titleNames() map[string]string {
	out := map[string]string{}
	if o.app.Library == nil {
		return out
	}
	for _, g := range o.app.Library.Games() {
		if g.TitleID != "" && g.Kind != "update" && g.Kind != "dlc" {
			out[strings.ToUpper(g.TitleID)] = g.Title
		}
	}
	return out
}

func baseTitleOf(id string) string {
	id = strings.ToUpper(id)
	if v, err := strconv.ParseUint(id, 16, 64); err == nil {
		if b, _ := classifyID(v); b != v {
			return fmt.Sprintf("%016X", b)
		}
	}
	return id
}

// gameDir finds a game's save folder in a user folder (by its ID, whatever follows it).
func gameDir(dir, titleID string, device bool) string {
	entries, _ := os.ReadDir(dir)
	id := strings.ToUpper(titleID)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := reGameFolder.FindStringSubmatch(e.Name())
		if m != nil && strings.ToUpper(m[1]) == id && (m[3] != "") == device {
			return e.Name()
		}
	}
	return ""
}

func (o *SaveOrganizer) wantGameFolder(titleID, title string, device bool) string {
	n := GameSaveFolderName(titleID, title)
	if device {
		n += " (device)"
	}
	return n
}

// SaveFolders is what the engine is told for a session: for each library game, the folder of
// its account save and of its device save in the user's folder (existing ones, or the names
// to create).
func (o *SaveOrganizer) SaveFolders(profileID string) map[string]map[string]string {
	dir := o.ProfileDir(profileID)
	out := map[string]map[string]string{}
	for id, title := range o.titleNames() {
		acc := gameDir(dir, id, false)
		if acc == "" {
			acc = o.wantGameFolder(id, title, false)
		}
		dev := gameDir(dir, id, true)
		if dev == "" {
			dev = o.wantGameFolder(id, title, true)
		}
		out[id] = map[string]string{"account": acc, "device": dev}
	}
	return out
}

func dirEmpty(p string) bool {
	es, err := os.ReadDir(p)
	return err == nil && len(es) == 0
}

// hasSaveData: the folder holds at least one file (engine bookkeeping files excluded).
func hasSaveData(p string) bool {
	found := false
	_ = filepath.Walk(p, func(q string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() && !strings.HasPrefix(info.Name(), ".yuzu") && !strings.HasPrefix(info.Name(), ".nexus") {
			found = true
			return io.EOF
		}
		return nil
	})
	return found
}

func otherCopyName(dir, base string) string {
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s (other copy %d)", base, i)
		if !fileExists(filepath.Join(dir, n)) && !dirExists(filepath.Join(dir, n)) {
			return n
		}
	}
}

// placeGameSave moves src (a game's save folder) to the game's folder in dir: into it when that
// folder is missing or empty, otherwise next to it as an "other copy" (never merged or replaced).
func (o *SaveOrganizer) placeGameSave(dir, src, titleID, title string, device bool) (string, error) {
	name := gameDir(dir, titleID, device)
	if name == "" {
		name = o.wantGameFolder(titleID, title, device)
	}
	dst := filepath.Join(dir, name)
	if dirExists(dst) && hasSaveData(dst) {
		name = otherCopyName(dir, name)
		dst = filepath.Join(dir, name)
		o.app.Log.Printf("saves: %s already has data; the other save is kept as %q", filepath.Base(filepath.Join(dir, gameDir(dir, titleID, device))), name)
	} else if dirExists(dst) {
		if err := os.RemoveAll(dst); err != nil { // empty (no save files): safe to replace
			return "", err
		}
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return name, nil
}

// adoptEngineLayout moves saves the engine wrote in its own layout (an older engine, or a game
// not in the library) into the NEXUS layout. Returns the number of saves moved.
func (o *SaveOrganizer) adoptEngineLayout(dir string) int {
	names := o.titleNames()
	moved := 0
	base := filepath.Join(dir, "user", "save")
	type unit struct {
		path, title string
		device      bool
	}
	var units []unit
	users, _ := os.ReadDir(filepath.Join(base, "0000000000000000"))
	for _, u := range users {
		if !u.IsDir() || !reUserDir.MatchString(u.Name()) {
			continue
		}
		titles, _ := os.ReadDir(filepath.Join(base, "0000000000000000", u.Name()))
		for _, t := range titles {
			if t.IsDir() && reTitleDir.MatchString(t.Name()) {
				units = append(units, unit{filepath.Join(base, "0000000000000000", u.Name(), t.Name()), strings.ToUpper(t.Name()), strings.Trim(u.Name(), "0") == ""})
			}
		}
	}
	accts, _ := os.ReadDir(filepath.Join(base, "account"))
	for _, a := range accts {
		titles, _ := os.ReadDir(filepath.Join(base, "account", a.Name()))
		for _, t := range titles {
			if t.IsDir() && reTitleDir.MatchString(t.Name()) {
				units = append(units, unit{filepath.Join(base, "account", a.Name(), t.Name()), strings.ToUpper(t.Name()), false})
			}
		}
	}
	devs, _ := os.ReadDir(filepath.Join(base, "device"))
	for _, t := range devs {
		if t.IsDir() && reTitleDir.MatchString(t.Name()) {
			units = append(units, unit{filepath.Join(base, "device", t.Name()), strings.ToUpper(t.Name()), true})
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].path < units[j].path })
	for _, u := range units {
		if !hasSaveData(u.path) {
			continue // empty folders the engine creates for games without saves yet
		}
		tid := baseTitleOf(u.title)
		if _, err := o.placeGameSave(dir, u.path, tid, names[tid], u.device); err != nil {
			o.app.Log.Printf("saves: could not move %s: %v", u.path, err)
			continue
		}
		moved++
	}
	if moved > 0 {
		o.app.Log.Printf("saves: %d save(s) in %s moved to the NEXUS layout", moved, filepath.Base(dir))
	}
	if dirExists(filepath.Join(dir, "user")) {
		setHidden(filepath.Join(dir, "user"))
	}
	return moved
}

// nameGameFolders gives "<GAME_ID>" folders their short name once the game is known.
func (o *SaveOrganizer) nameGameFolders(dir string) {
	names := o.titleNames()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		m := reGameFolder.FindStringSubmatch(e.Name())
		if !e.IsDir() || m == nil || m[2] != "" {
			continue
		}
		id := strings.ToUpper(m[1])
		title := names[id]
		if title == "" {
			continue
		}
		want := o.wantGameFolder(id, title, m[3] != "")
		if want != e.Name() && !fileExists(filepath.Join(dir, want)) && !dirExists(filepath.Join(dir, want)) {
			_ = os.Rename(filepath.Join(dir, e.Name()), filepath.Join(dir, want))
		}
	}
}

// Tidy runs for one user when they are not playing: adopts engine-layout saves and names
// game folders. Never deletes or overwrites save data.
func (o *SaveOrganizer) Tidy(profileID string) {
	if o.busy(profileID) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	dir := o.ProfileDir(profileID)
	if !dirExists(dir) {
		return
	}
	o.adoptEngineLayout(dir)
	o.nameGameFolders(dir)
}

// TidyAll runs at start-up (after the layout migration).
func (o *SaveOrganizer) TidyAll() {
	if o.app.Profiles == nil {
		return
	}
	for _, p := range o.app.Profiles.List() {
		o.SyncProfileName(p.ID)
		o.Tidy(p.ID)
	}
}

// SyncProfileName renames the user's save folder after a name change (never while that user is
// playing). The player ID stays at the start of the name, which is how NEXUS finds it.
func (o *SaveOrganizer) SyncProfileName(id string) {
	if o.busy(id) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	p, ok := o.app.Profiles.Get(id)
	if !ok || p.PlayerID == "" {
		return
	}
	cur := o.ProfileDir(id)
	want := filepath.Join(o.root(), playerFolderName(p.PlayerID, p.Name))
	if cur == want || !dirExists(cur) || fileExists(want) || dirExists(want) {
		return
	}
	if err := os.Rename(cur, want); err != nil {
		o.app.Log.Printf("saves: could not rename %s: %v", cur, err)
	}
}

func playerFolderName(playerID, name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return playerID
	}
	return playerID + "-" + safeFolderName(n)
}

// ---------- listing

func dirStat(p string) (bytes int64, files int, modified int64) {
	_ = filepath.Walk(p, func(q string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".yuzu") || strings.HasPrefix(info.Name(), ".nexus") {
			return nil
		}
		bytes += info.Size()
		files++
		if t := info.ModTime().Unix(); t > modified {
			modified = t
		}
		return nil
	})
	return
}

func saveItems(p string) []SaveItem {
	out := []SaveItem{}
	entries, _ := os.ReadDir(p)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue // engine bookkeeping (.yuzu_save_size)
		}
		it := SaveItem{Name: e.Name(), Dir: e.IsDir(), Label: itemLabel(e.Name(), e.IsDir())}
		if e.IsDir() {
			it.Bytes, _, it.Modified = dirStat(filepath.Join(p, e.Name()))
		} else if fi, err := e.Info(); err == nil {
			it.Bytes, it.Modified = fi.Size(), fi.ModTime().Unix()
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := reSlotDir.MatchString(out[i].Name) && out[i].Dir, reSlotDir.MatchString(out[j].Name) && out[j].Dir
		if si != sj {
			return si
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Player lists a user's saves: one entry per game, with the slots and files that exist.
func (o *SaveOrganizer) Player(profileID string) (SavePlayer, error) {
	p, ok := o.app.Profiles.Get(profileID)
	if !ok {
		return SavePlayer{}, errors.New("User not found.")
	}
	o.Tidy(profileID)
	dir := o.ProfileDir(profileID)
	sp := SavePlayer{ProfileID: p.ID, Name: p.Name, PlayerID: p.PlayerID, Folder: filepath.Base(dir), Path: dir, Games: []SaveGame{}, Other: []string{}}
	names := o.titleNames()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "user" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		var id, kind string
		if m := reGameFolder.FindStringSubmatch(e.Name()); m != nil {
			id, kind = strings.ToUpper(m[1]), "account"
			if m[3] != "" {
				kind = "device"
			}
		} else if m := reOtherCopy.FindStringSubmatch(e.Name()); m != nil {
			id, kind = strings.ToUpper(m[1]), "other-copy"
		} else {
			sp.Other = append(sp.Other, e.Name())
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, n, mod := dirStat(path)
		if n == 0 {
			continue // empty folder: no save yet
		}
		title, in := names[id]
		if !in {
			title = "Game " + id
		}
		sp.Games = append(sp.Games, SaveGame{TitleID: id, Title: title, InLib: in, Folder: e.Name(), Kind: kind, Bytes: b, Files: n, Modified: mod, Items: saveItems(path)})
	}
	sort.SliceStable(sp.Games, func(i, j int) bool {
		if sp.Games[i].Title != sp.Games[j].Title {
			return strings.ToLower(sp.Games[i].Title) < strings.ToLower(sp.Games[j].Title)
		}
		return sp.Games[i].Folder < sp.Games[j].Folder
	})
	return sp, nil
}

// folderFor resolves a game folder of a user from a folder name given by the interface (only
// names that are game folders of that user are accepted).
func (o *SaveOrganizer) folderFor(profileID, folder string) (string, error) {
	if folder == "" || strings.ContainsAny(folder, `/\:`) || strings.Contains(folder, "..") || !(reGameFolder.MatchString(folder) || reOtherCopy.MatchString(folder)) {
		return "", errors.New("That is not a game save folder.")
	}
	p := filepath.Join(o.ProfileDir(profileID), folder)
	if !dirExists(p) {
		return "", errors.New("That save no longer exists. It may have been moved or deleted outside NEXUS.")
	}
	return p, nil
}

// ---------- backups

// zipDir writes dir (recursively) into zw under prefix.
func zipDir(zw *zip.Writer, dir, prefix string) error {
	return filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		name := filepath.ToSlash(filepath.Join(prefix, rel))
		if rel == "." {
			name = filepath.ToSlash(prefix)
		}
		if info.IsDir() {
			if name == "" || name == "." {
				return nil
			}
			_, err := zw.Create(name + "/")
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		hw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: info.ModTime()})
		if err != nil {
			return err
		}
		_, err = io.Copy(hw, f)
		return err
	})
}

// writeZipAtomic writes a ZIP through a temporary file and checks it can be read back.
func writeZipAtomic(path string, fill func(zw *zip.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".nexus-tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	err = fill(zw)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		var zr *zip.ReadCloser
		if zr, err = zip.OpenReader(tmp); err == nil {
			zr.Close()
		}
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Backup zips one game's save (or, with folder "", all of the user's saves) into
// UserData\Backups\Saves\<player folder>\. reason: "manual", "before-import", "before-restore",
// "before-delete", "migration".
func (o *SaveOrganizer) Backup(profileID, folder, reason string) (SaveBackup, error) {
	dir := o.ProfileDir(profileID)
	src := dir
	tid := ""
	if folder != "" {
		p, err := o.folderFor(profileID, folder)
		if err != nil {
			return SaveBackup{}, err
		}
		src = p
		if m := reGameFolder.FindStringSubmatch(folder); m != nil {
			tid = strings.ToUpper(m[1])
		} else if m := reOtherCopy.FindStringSubmatch(folder); m != nil {
			tid = strings.ToUpper(m[1])
		}
	}
	stamp := time.Now().Format("2006-01-02 15-04-05")
	name := fmt.Sprintf("%s %s - %s.zip", stamp, reason, safeFolderName(firstNonEmpty(folder, "all saves")))
	out := filepath.Join(o.backupsRoot(), filepath.Base(dir), name)
	for i := 2; fileExists(out); i++ {
		out = filepath.Join(o.backupsRoot(), filepath.Base(dir), fmt.Sprintf("%s %s (%d) - %s.zip", stamp, reason, i, safeFolderName(firstNonEmpty(folder, "all saves"))))
	}
	err := writeZipAtomic(out, func(zw *zip.Writer) error {
		if folder == "" {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if e.IsDir() && e.Name() != "user" && !strings.HasPrefix(e.Name(), ".") {
					if err := zipDir(zw, filepath.Join(dir, e.Name()), e.Name()); err != nil {
						return err
					}
				}
			}
			return nil
		}
		return zipDir(zw, src, folder)
	})
	if err != nil {
		return SaveBackup{}, fmt.Errorf("the backup could not be written: %w", err)
	}
	fi, _ := os.Stat(out)
	o.app.Log.Printf("saves: backup %s", out)
	return SaveBackup{File: filepath.Base(out), TitleID: tid, Reason: reason, Created: time.Now().Unix(), Bytes: fi.Size()}, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Backups lists a user's backups, newest first; with titleID, only that game's.
func (o *SaveOrganizer) Backups(profileID, titleID string) []SaveBackup {
	dir := filepath.Join(o.backupsRoot(), filepath.Base(o.ProfileDir(profileID)))
	entries, _ := os.ReadDir(dir)
	out := []SaveBackup{}
	reName := regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d-\d\d-\d\d) ([a-z-]+)(?: \(\d+\))? - (.*)\.zip$`)
	for _, e := range entries {
		m := reName.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		fi, _ := e.Info()
		t, _ := time.ParseInLocation("2006-01-02 15-04-05", m[1], time.Local)
		b := SaveBackup{File: e.Name(), Reason: m[2], Created: t.Unix(), Bytes: fi.Size()}
		if g := reGameFolder.FindStringSubmatch(m[3]); g != nil {
			b.TitleID = strings.ToUpper(g[1])
		} else if g := reOtherCopy.FindStringSubmatch(m[3]); g != nil {
			b.TitleID = strings.ToUpper(g[1])
		}
		if titleID != "" && !strings.EqualFold(b.TitleID, titleID) {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File > out[j].File })
	return out
}

// Restore replaces a game's save with a backup (the current save is backed up first).
func (o *SaveOrganizer) Restore(profileID, file string) error {
	if o.busy(profileID) {
		return errSavesBusy
	}
	if strings.ContainsAny(file, `/\:`) || strings.Contains(file, "..") {
		return errors.New("invalid backup")
	}
	src := filepath.Join(o.backupsRoot(), filepath.Base(o.ProfileDir(profileID)), file)
	zr, err := zip.OpenReader(src)
	if err != nil {
		return errors.New("That backup cannot be read.")
	}
	defer zr.Close()
	// a game backup holds one top folder; an "all saves" backup holds several
	tops := map[string]bool{}
	for _, f := range zr.File {
		top, _, _ := strings.Cut(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		if top != "" {
			tops[top] = true
		}
	}
	for top := range tops {
		if !(reGameFolder.MatchString(top) || reOtherCopy.MatchString(top)) {
			return fmt.Errorf("The backup contains %q, which is not a game save folder.", top)
		}
	}
	dir := o.ProfileDir(profileID)
	for top := range tops {
		if dirExists(filepath.Join(dir, top)) && hasSaveData(filepath.Join(dir, top)) {
			if _, err := o.Backup(profileID, top, "before-restore"); err != nil {
				return err
			}
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for top := range tops {
		if err := extractTopFolder(&zr.Reader, top, filepath.Join(dir, top)); err != nil {
			return err
		}
	}
	o.app.Log.Printf("saves: restored %s", file)
	return nil
}

// extractTopFolder extracts the files under top/ in a ZIP into a staging folder, then swaps it
// with dst (the old dst is removed only after the new one is complete).
func extractTopFolder(zr *zip.Reader, top, dst string) error {
	stage := dst + ".nexus-new"
	_ = os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		rest, ok := strings.CutPrefix(name, top+"/")
		if !ok || rest == "" {
			continue
		}
		if strings.Contains(rest, "..") || strings.HasPrefix(rest, "/") || strings.Contains(rest, ":") {
			os.RemoveAll(stage)
			return errors.New("The backup contains an invalid path.")
		}
		target := filepath.Join(stage, filepath.FromSlash(rest))
		if strings.HasSuffix(name, "/") {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := extractZipFile(f, target); err != nil {
			os.RemoveAll(stage)
			return err
		}
	}
	return swapDir(stage, dst)
}

func extractZipFile(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	// never more than the entry declares (the ZIP reader also checks size and CRC at the end)
	n, err := io.Copy(out, io.LimitReader(rc, int64(f.UncompressedSize64)+1))
	if err == nil && uint64(n) != f.UncompressedSize64 {
		err = fmt.Errorf("%s is damaged (its size does not match the archive)", f.Name)
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && !f.Modified.IsZero() {
		_ = os.Chtimes(target, f.Modified, f.Modified)
	}
	return err
}

// swapDir puts the complete folder stage at dst. An existing dst is moved aside first and only
// removed after stage is in place (its contents were backed up by the caller).
func swapDir(stage, dst string) error {
	old := ""
	if dirExists(dst) {
		old = dst + ".nexus-old"
		_ = os.RemoveAll(old)
		if err := os.Rename(dst, old); err != nil {
			os.RemoveAll(stage)
			return fmt.Errorf("the current save could not be replaced (is a file open?): %w", err)
		}
	}
	if err := os.Rename(stage, dst); err != nil {
		if old != "" {
			_ = os.Rename(old, dst)
		}
		os.RemoveAll(stage)
		return err
	}
	if old != "" {
		_ = os.RemoveAll(old)
	}
	return nil
}

// Delete removes one game's save after backing it up.
func (o *SaveOrganizer) Delete(profileID, folder string) (SaveBackup, error) {
	return o.DeleteSave(profileID, folder, false)
}

// DeleteSave deletes one user's save of a game: the folder, any copy of the same game the engine
// left in its own layout inside this user's save folder (which Tidy would otherwise move back),
// and with withDevice the game's device save too. Everything is backed up first. The deletion is
// recorded for save synchronisation so the account's copy is not reinstalled. Only this user's
// folder is touched. Never while this user is playing.
func (o *SaveOrganizer) DeleteSave(profileID, folder string, withDevice bool) (SaveBackup, error) {
	if o.busy(profileID) {
		return SaveBackup{}, errSaveDeleteBusy
	}
	p, err := o.folderFor(profileID, folder)
	if err != nil {
		return SaveBackup{}, err
	}
	folders := []string{folder}
	if m := reGameFolder.FindStringSubmatch(folder); m != nil && withDevice && m[3] == "" {
		if dev := o.siblingDeviceFolder(profileID, m[1]); dev != "" {
			folders = append(folders, dev)
		}
	}
	var first SaveBackup
	for i, f := range folders {
		b, err := o.Backup(profileID, f, "before-delete")
		if err != nil {
			return first, err
		}
		if i == 0 {
			first = b
		}
	}
	o.mu.Lock()
	var stale []string
	if m := reGameFolder.FindStringSubmatch(folder); m != nil {
		stale = engineLayoutCopies(o.ProfileDir(profileID), strings.ToUpper(m[1]))
	}
	for _, f := range folders {
		fp := filepath.Join(o.ProfileDir(profileID), f)
		if err := os.RemoveAll(fp); err != nil {
			o.mu.Unlock()
			return first, err
		}
		o.app.Log.Printf("saves: deleted %s (backup made first)", fp)
	}
	for _, sp := range stale { // already in the backup's folder? no: back up under its own name
		if hasSaveData(sp) {
			if err := o.backupPath(profileID, sp, "before-delete"); err != nil {
				o.app.Log.Printf("saves: could not back up %s: %v (kept)", sp, err)
				continue
			}
		}
		if err := os.RemoveAll(sp); err == nil {
			o.app.Log.Printf("saves: deleted the engine-layout copy %s", sp)
		}
	}
	o.mu.Unlock()
	_ = p
	if o.app.SaveSync != nil {
		for _, f := range folders {
			o.app.SaveSync.MarkDeleted(profileID, f)
		}
	}
	if o.app.Account != nil {
		o.app.Account.Kick()
	}
	return first, nil
}

var errSaveDeleteBusy = errors.New("This user is playing right now. Close the game first: a running game keeps its save open and would write it again.")

// deleteForSync deletes a save after the user agreed to a deletion made on another device
// (backed up first; not recorded as a new deletion).
func (o *SaveOrganizer) deleteForSync(profileID, folder string) error {
	if o.busy(profileID) {
		return errSavesBusy
	}
	p, err := o.folderFor(profileID, folder)
	if err != nil {
		return err
	}
	if _, err := o.Backup(profileID, folder, "before-delete"); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := os.RemoveAll(p); err != nil {
		return err
	}
	o.app.Log.Printf("saves: deleted %s (deleted on another device; backup made first)", p)
	return nil
}

// siblingDeviceFolder: the game's device-save folder in this user's save folder, if any.
func (o *SaveOrganizer) siblingDeviceFolder(profileID, tid string) string {
	entries, _ := os.ReadDir(o.ProfileDir(profileID))
	for _, e := range entries {
		if m := reGameFolder.FindStringSubmatch(e.Name()); e.IsDir() && m != nil && m[3] != "" && strings.EqualFold(m[1], tid) {
			return e.Name()
		}
	}
	return ""
}

// engineLayoutCopies: folders of the game (base title ID) in the engine's own save layout inside
// a user's save folder (user/save/0000000000000000/<user>/<title>, account/<uuid>/<title>,
// device/<title>).
func engineLayoutCopies(dir, tid string) []string {
	base := filepath.Join(dir, "user", "save")
	var out []string
	match := func(name string) bool {
		return reTitleDir.MatchString(name) && strings.EqualFold(baseTitleOf(strings.ToUpper(name)), tid)
	}
	users, _ := os.ReadDir(filepath.Join(base, "0000000000000000"))
	for _, u := range users {
		ts, _ := os.ReadDir(filepath.Join(base, "0000000000000000", u.Name()))
		for _, t := range ts {
			if t.IsDir() && match(t.Name()) {
				out = append(out, filepath.Join(base, "0000000000000000", u.Name(), t.Name()))
			}
		}
	}
	accts, _ := os.ReadDir(filepath.Join(base, "account"))
	for _, a := range accts {
		ts, _ := os.ReadDir(filepath.Join(base, "account", a.Name()))
		for _, t := range ts {
			if t.IsDir() && match(t.Name()) {
				out = append(out, filepath.Join(base, "account", a.Name(), t.Name()))
			}
		}
	}
	devs, _ := os.ReadDir(filepath.Join(base, "device"))
	for _, t := range devs {
		if t.IsDir() && match(t.Name()) {
			out = append(out, filepath.Join(base, "device", t.Name()))
		}
	}
	return out
}

// backupPath backs up a folder that is not a NEXUS game folder (called with o.mu held).
func (o *SaveOrganizer) backupPath(profileID, src, reason string) error {
	dir := o.ProfileDir(profileID)
	stamp := time.Now().Format("2006-01-02 15-04-05")
	name := safeFolderName(strings.ToUpper(filepath.Base(src)) + " engine copy")
	out := filepath.Join(o.backupsRoot(), filepath.Base(dir), fmt.Sprintf("%s %s - %s.zip", stamp, reason, name))
	for i := 2; fileExists(out); i++ {
		out = filepath.Join(o.backupsRoot(), filepath.Base(dir), fmt.Sprintf("%s %s (%d) - %s.zip", stamp, reason, i, name))
	}
	return writeZipAtomic(out, func(zw *zip.Writer) error { return zipDir(zw, src, filepath.Base(src)) })
}

// UseOtherCopy makes an "other copy" the game's save: the current save becomes an other copy.
func (o *SaveOrganizer) UseOtherCopy(profileID, folder string) error {
	if o.busy(profileID) {
		return errSavesBusy
	}
	m := reOtherCopy.FindStringSubmatch(folder)
	if m == nil {
		return errors.New("That is not another copy of a save.")
	}
	src, err := o.folderFor(profileID, folder)
	if err != nil {
		return err
	}
	dir := o.ProfileDir(profileID)
	device := m[3] != ""
	if _, err := o.Backup(profileID, "", "before-restore"); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	id := strings.ToUpper(m[1])
	cur := gameDir(dir, id, device)
	want := cur
	if want == "" {
		want = o.wantGameFolder(id, o.titleNames()[id], device)
	}
	if cur != "" {
		if err := os.Rename(filepath.Join(dir, cur), filepath.Join(dir, otherCopyName(dir, cur))); err != nil {
			return err
		}
	}
	return os.Rename(src, filepath.Join(dir, want))
}

// ---------- export

type saveExportDoc struct {
	App      string `json:"app"`
	Version  string `json:"version"`
	PlayerID string `json:"playerId"`
	Profile  string `json:"profile"`
	Games    []struct {
		TitleID string `json:"titleId"`
		Title   string `json:"title"`
		Folder  string `json:"folder"`
	} `json:"games"`
}

// Export writes a ZIP with one game's save (folder) or all of the user's saves:
// "<PLAYER_ID>-<name>/<GAME_ID>-<SHORT>/…" plus nexus-saves.json describing them.
func (o *SaveOrganizer) Export(profileID, folder string, w io.Writer) error {
	sp, err := o.Player(profileID)
	if err != nil {
		return err
	}
	doc := saveExportDoc{App: "NEXUS", Version: AppVersion, PlayerID: sp.PlayerID, Profile: sp.Name}
	zw := zip.NewWriter(w)
	for _, g := range sp.Games {
		if folder != "" && g.Folder != folder {
			continue
		}
		doc.Games = append(doc.Games, struct {
			TitleID string `json:"titleId"`
			Title   string `json:"title"`
			Folder  string `json:"folder"`
		}{g.TitleID, g.Title, g.Folder})
		if err := zipDir(zw, filepath.Join(sp.Path, g.Folder), sp.Folder+"/"+g.Folder); err != nil {
			zw.Close()
			return err
		}
	}
	if folder != "" && len(doc.Games) == 0 {
		zw.Close()
		return errors.New("That save no longer exists.")
	}
	data, _ := json.MarshalIndent(doc, "", "  ")
	if hw, err := zw.Create(saveExportMarker); err == nil {
		_, _ = hw.Write(data)
	}
	return zw.Close()
}

// ---------- import: inspect, then import what the user confirmed

// SaveCandidate is one game save found in what the user selected.
type SaveCandidate struct {
	ID        string     `json:"id"`        // reference within the inspection
	PlayerID  string     `json:"playerId"`  // from the folder structure or save metadata ("" = unknown)
	ProfileID string     `json:"profileId"` // NEXUS user with that player ID ("" = none)
	TitleID   string     `json:"titleId"`   // "" = could not be identified
	Title     string     `json:"title"`
	InLib     bool       `json:"inLibrary"`
	Device    bool       `json:"device"`
	Confident bool       `json:"confident"` // the game ID comes from the save's structure or metadata
	Evidence  string     `json:"evidence"`
	Items     []SaveItem `json:"items"`
	Bytes     int64      `json:"bytes"`
	Files     int        `json:"files"`
	path      string     // inside the source (folder path, or ZIP prefix)
}

type SaveInspection struct {
	Token      string          `json:"token"`
	Source     string          `json:"source"`
	Format     string          `json:"format"`
	Candidates []SaveCandidate `json:"candidates"`
	Note       string          `json:"note,omitempty"`
}

type saveSource struct {
	dir  string      // a folder on disk, or
	zr   *zip.Reader // a ZIP (kept in memory until imported)
	data []byte
	insp SaveInspection
	at   time.Time
}

var (
	inspMu   sync.Mutex
	inspList = map[string]*saveSource{}
)

func (o *SaveOrganizer) remember(src *saveSource) string {
	inspMu.Lock()
	defer inspMu.Unlock()
	for k, v := range inspList { // keep memory bounded
		if time.Since(v.at) > 30*time.Minute {
			delete(inspList, k)
		}
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	src.at = time.Now()
	inspList[tok] = src
	return tok
}

// vfs: a minimal read-only view over a folder or a ZIP, so both are inspected the same way.
type saveVFS interface {
	list(dir string) []os.FileInfo
	read(path string, max int) []byte
}

type osVFS struct{ root string }

func (v osVFS) list(dir string) []os.FileInfo {
	es, _ := os.ReadDir(filepath.Join(v.root, filepath.FromSlash(dir)))
	var out []os.FileInfo
	for _, e := range es {
		if fi, err := e.Info(); err == nil {
			out = append(out, fi)
		}
	}
	return out
}

func (v osVFS) read(p string, max int) []byte {
	f, err := os.Open(filepath.Join(v.root, filepath.FromSlash(p)))
	if err != nil {
		return nil
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, int64(max)))
	return b
}

type zipVFS struct {
	files map[string]*zip.File
	dirs  map[string]map[string]os.FileInfo
}

type zfi struct {
	name string
	dir  bool
	size int64
	mod  time.Time
}

func (z zfi) Name() string { return z.name }
func (z zfi) Size() int64  { return z.size }
func (z zfi) Mode() os.FileMode {
	return map[bool]os.FileMode{true: os.ModeDir | 0o755, false: 0o644}[z.dir]
}
func (z zfi) ModTime() time.Time { return z.mod }
func (z zfi) IsDir() bool        { return z.dir }
func (z zfi) Sys() any           { return nil }

func newZipVFS(zr *zip.Reader) *zipVFS {
	v := &zipVFS{files: map[string]*zip.File{}, dirs: map[string]map[string]os.FileInfo{}}
	add := func(parent, name string, fi os.FileInfo) {
		if v.dirs[parent] == nil {
			v.dirs[parent] = map[string]os.FileInfo{}
		}
		if _, ok := v.dirs[parent][name]; !ok || !fi.IsDir() {
			v.dirs[parent][name] = fi
		}
	}
	for _, f := range zr.File {
		name := strings.Trim(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		if name == "" || strings.Contains(name, "..") || strings.Contains(name, ":") {
			continue
		}
		isDir := strings.HasSuffix(f.Name, "/")
		parts := strings.Split(name, "/")
		for i := 0; i < len(parts)-1; i++ {
			add(strings.Join(parts[:i], "/"), parts[i], zfi{name: parts[i], dir: true})
		}
		add(strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1], zfi{name: parts[len(parts)-1], dir: isDir, size: int64(f.UncompressedSize64), mod: f.Modified})
		if !isDir {
			v.files[name] = f
		}
	}
	return v
}

func (v *zipVFS) list(dir string) []os.FileInfo {
	var out []os.FileInfo
	for _, fi := range v.dirs[strings.Trim(dir, "/")] {
		out = append(out, fi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func (v *zipVFS) read(p string, max int) []byte {
	f := v.files[strings.Trim(p, "/")]
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, _ := io.ReadAll(io.LimitReader(rc, int64(max)))
	return b
}

func vfsStat(v saveVFS, dir string) (items []SaveItem, bytes int64, files int) {
	var walk func(d string) (int64, int, int64)
	walk = func(d string) (int64, int, int64) {
		var b, mod int64
		n := 0
		for _, fi := range v.list(d) {
			if fi.IsDir() {
				bb, nn, mm := walk(strings.TrimPrefix(d+"/"+fi.Name(), "/"))
				b, n = b+bb, n+nn
				if mm > mod {
					mod = mm
				}
				continue
			}
			if strings.HasPrefix(fi.Name(), ".yuzu") || strings.HasPrefix(fi.Name(), ".nexus") {
				continue
			}
			b += fi.Size()
			n++
			if t := fi.ModTime().Unix(); t > mod {
				mod = t
			}
		}
		return b, n, mod
	}
	items = []SaveItem{}
	for _, fi := range v.list(dir) {
		if strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		it := SaveItem{Name: fi.Name(), Dir: fi.IsDir(), Label: itemLabel(fi.Name(), fi.IsDir())}
		if fi.IsDir() {
			it.Bytes, _, it.Modified = walk(strings.TrimPrefix(dir+"/"+fi.Name(), "/"))
		} else {
			it.Bytes, it.Modified = fi.Size(), fi.ModTime().Unix()
		}
		items = append(items, it)
	}
	bytes, files, _ = walk(dir)
	return
}

// ryujinxExtra reads a Ryujinx save's ExtraData0/1: the SaveDataAttribute starts with the
// program ID (u64) and the user ID (u128).
func ryujinxExtra(b []byte) (titleID, userID string, ok bool) {
	if len(b) < 0x18 {
		return "", "", false
	}
	pid := binary.LittleEndian.Uint64(b[0:])
	lo, hi := binary.LittleEndian.Uint64(b[8:]), binary.LittleEndian.Uint64(b[16:])
	if pid == 0 {
		return "", "", false
	}
	if lo != 0 || hi != 0 {
		userID = fmt.Sprintf("%016X%016X", hi, lo)
	}
	return fmt.Sprintf("%016X", pid), userID, true
}

// inspect finds game saves in a folder or ZIP. Rules, most specific first:
//   - an engine user folder: <user 32 hex>/<game 16 hex>/… (the layout of the Switch's own
//     save filesystem, used by the NEXUS engine and many other tools), also with the
//     user/save/0000000000000000 parents;
//   - NEXUS folders: <PLAYER_ID>-<name>/<GAME_ID>-<SHORT>/…, or a single <GAME_ID>-<SHORT>;
//   - a single game folder named by its 16-hex game ID (its parent may be a 32-hex user);
//   - a save with a metadata file (0/, 1/, ExtraData0): game and user from its metadata;
//   - otherwise, if it contains files, a save whose game cannot be identified.
func (o *SaveOrganizer) inspect(v saveVFS, rootName string) ([]SaveCandidate, string) {
	var out []SaveCandidate
	format := ""
	add := func(path, player, tid string, device, confident bool, evidence string) {
		items, b, n := vfsStat(v, path)
		if n == 0 {
			return
		}
		c := SaveCandidate{ID: strconv.Itoa(len(out) + 1), PlayerID: strings.ToUpper(player), TitleID: strings.ToUpper(tid), Device: device,
			Confident: confident && tid != "", Evidence: evidence, Items: items, Bytes: b, Files: n, path: path}
		out = append(out, c)
	}
	isUser := func(n string) bool { return reUserDir.MatchString(n) || rePlayerDir.MatchString(n) }
	playerOf := func(n string) string {
		if m := rePlayerDir.FindStringSubmatch(n); m != nil {
			return m[1]
		}
		return ""
	}
	var scan func(dir, name, parent string, depth int)
	scan = func(dir, name, parent string, depth int) {
		if depth > 14 || len(out) > 500 {
			return
		}
		join := func(n string) string { return strings.TrimPrefix(dir+"/"+n, "/") }
		entries := v.list(dir)
		// a save with its metadata: 0/ (and 1/) plus ExtraData0
		var extra []byte
		hasZero := false
		for _, fi := range entries {
			if fi.Name() == "ExtraData0" && !fi.IsDir() {
				extra = v.read(join(fi.Name()), 0x40)
			}
			if fi.Name() == "0" && fi.IsDir() {
				hasZero = true
			}
		}
		if hasZero && extra != nil {
			if tid, uid, ok := ryujinxExtra(extra); ok {
				if format == "" {
					format = "Switch saves with metadata"
				}
				add(join("0"), uid, baseTitleOf(tid), false, true, "Game and player read from the save's own metadata (ExtraData0).")
				return
			}
		}
		// a NEXUS game folder or a 16-hex game folder: this is one game's save
		if m := reGameFolder.FindStringSubmatch(name); m != nil && depth > 0 && strings.Trim(m[1], "0") != "" {
			ev := "The folder is named after the game's ID (" + strings.ToUpper(m[1]) + ")."
			f := "Save folder"
			switch {
			case m[2] != "":
				ev, f = "NEXUS save folder of game "+strings.ToUpper(m[1])+".", "NEXUS saves"
			case reUserDir.MatchString(parent):
				f = "Switch save folders (player and game IDs)"
			}
			if format == "" {
				format = f
			}
			pl := playerOf(parent)
			add(dir, pl, baseTitleOf(m[1]), m[3] != "" || (pl != "" && strings.Trim(pl, "0") == ""), true, ev)
			return
		}
		if isUser(name) || depth == 0 {
			for _, fi := range entries {
				if fi.IsDir() && reGameFolder.MatchString(fi.Name()) {
					scan(join(fi.Name()), fi.Name(), name, depth+1)
				}
			}
			if len(out) > 0 {
				return
			}
		}
		for _, fi := range entries {
			if notSaveFolder[strings.ToLower(fi.Name())] {
				continue // engine caches, mods, system saves …: never game saves
			}
			if fi.IsDir() {
				scan(join(fi.Name()), fi.Name(), name, depth+1)
			}
		}
	}
	scan("", rootName, "", 0)
	if len(out) == 0 {
		// no structure: the selection itself is one save (game to be chosen by the user)
		if m := reGameFolder.FindStringSubmatch(rootName); m != nil {
			add("", "", baseTitleOf(m[1]), m[3] != "", true, "The folder is named after the game's ID ("+strings.ToUpper(m[1])+").")
		} else {
			add("", "", "", false, false, "No game ID was found in the folder names or save metadata.")
		}
		format = "Save files"
	}
	return out, format
}

// notSaveFolder: folders of an emulator's data tree that can contain game-ID folders but no
// game saves (shader caches, mods, dumps, system saves, the SD card …).
var notSaveFolder = map[string]bool{"cache": true, "temp": true, "system": true, "load": true, "dump": true, "sdmc": true,
	"shader": true, "shaders": true, "screenshots": true, "keys": true, "crash_dumps": true, "log": true, "logs": true,
	"amiibo": true, "mods": true}

// Inspect looks at a folder or ZIP the user selected and reports what it contains.
func (o *SaveOrganizer) Inspect(path string, zipData []byte, displayName string) (SaveInspection, error) {
	src := &saveSource{}
	var v saveVFS
	rootName := ""
	switch {
	case zipData != nil:
		zr, err := openSaveZip(zipData)
		if err != nil {
			return SaveInspection{}, err
		}
		src.zr, src.data = zr, zipData
		v = newZipVFS(zr)
		rootName = strings.TrimSuffix(displayName, filepath.Ext(displayName))
		src.insp.Source = displayName
	case path != "":
		fi, err := os.Stat(path)
		if err != nil {
			return SaveInspection{}, errors.New("That location cannot be read.")
		}
		if !fi.IsDir() {
			if strings.EqualFold(filepath.Ext(path), ".zip") {
				if fi.Size() > maxSaveZipBytes {
					return SaveInspection{}, errZipTooLarge
				}
				data, err := os.ReadFile(path) // read only: the archive itself is never changed
				if err != nil {
					return SaveInspection{}, err
				}
				return o.Inspect("", data, filepath.Base(path))
			}
			return SaveInspection{}, errors.New("Choose a save folder or a ZIP archive of one.")
		}
		src.dir = path
		v = osVFS{root: path}
		rootName = filepath.Base(path)
		src.insp.Source = path
	default:
		return SaveInspection{}, errors.New("Nothing selected.")
	}
	cands, format := o.inspect(v, rootName)
	names := o.titleNames()
	players := map[string]string{}
	for _, p := range o.app.Profiles.List() {
		if p.PlayerID != "" {
			players[strings.ToUpper(p.PlayerID)] = p.ID
		}
	}
	for i := range cands {
		c := &cands[i]
		if c.TitleID != "" {
			c.Title, c.InLib = names[c.TitleID], names[c.TitleID] != ""
			if !c.InLib {
				c.Title = "Game " + c.TitleID + " (not in your library)"
			}
		}
		c.ProfileID = players[c.PlayerID]
	}
	src.insp.Format = format
	src.insp.Candidates = cands
	if len(cands) == 0 {
		src.insp.Note = "No save data was found there. " + unsupportedZipHelp
	} else if len(cands) == 1 && cands[0].TitleID == "" {
		src.insp.Note = "NEXUS found files but no game ID, so you choose the game. " + unsupportedZipHelp
	}
	src.insp.Token = o.remember(src)
	return src.insp, nil
}

// SaveImportPick is one save the user confirmed: the game it goes to (always confirmed by the
// user when it could not be identified), whether it is the device save, and whether existing
// data may be replaced (after a backup).
type SaveImportPick struct {
	ID      string `json:"id"`
	TitleID string `json:"titleId"`
	Device  bool   `json:"device"`
	Replace bool   `json:"replace"`
}

type SaveConflict struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Folder string `json:"folder"`
}

var errSaveConflict = errors.New("Some games already have save data for this user.")

// Import copies the confirmed saves into a user's game folders. A game that already has data is
// only replaced when the pick says so, and then only after a backup.
func (o *SaveOrganizer) Import(token, profileID string, picks []SaveImportPick) ([]SaveConflict, error) {
	if _, ok := o.app.Profiles.Get(profileID); !ok {
		return nil, errors.New("User not found.")
	}
	if o.busy(profileID) {
		return nil, errSavesBusy
	}
	inspMu.Lock()
	src := inspList[token]
	inspMu.Unlock()
	if src == nil {
		return nil, errors.New("Select the saves again (the selection expired).")
	}
	byID := map[string]SaveCandidate{}
	for _, c := range src.insp.Candidates {
		byID[c.ID] = c
	}
	names := o.titleNames()
	dir := o.ProfileDir(profileID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// check first: never guess a game, report conflicts before changing anything
	var conflicts []SaveConflict
	for _, p := range picks {
		c, ok := byID[p.ID]
		if !ok {
			return nil, errors.New("unknown save in the selection")
		}
		tid := strings.ToUpper(p.TitleID)
		if !reTitleDir.MatchString(tid) {
			return nil, fmt.Errorf("Choose the game for %q.", firstNonEmpty(c.Title, "the save"))
		}
		if c.Confident && c.TitleID != "" && tid != c.TitleID {
			return nil, fmt.Errorf("That save belongs to game %s, not %s.", c.TitleID, tid)
		}
		if name := gameDir(dir, tid, p.Device); name != "" && hasSaveData(filepath.Join(dir, name)) && !p.Replace {
			conflicts = append(conflicts, SaveConflict{ID: p.ID, Title: firstNonEmpty(names[tid], tid), Folder: name})
		}
	}
	if len(conflicts) > 0 {
		return conflicts, errSaveConflict
	}
	// 1. copy every selected save into a staging folder next to its destination (archive
	//    entries are decompressed and CRC-checked here); if any fails, nothing has changed
	type staged struct {
		c          SaveCandidate
		name, path string
		replace    bool
	}
	var total int64
	for _, p := range picks {
		total += byID[p.ID].Bytes
	}
	progress := func(done int64, phase string) {
		o.app.Events.Emit("saves.import", map[string]any{"done": done, "total": total, "phase": phase})
	}
	var plan []staged
	cleanup := func() {
		for _, st := range plan {
			os.RemoveAll(st.path)
		}
	}
	var done int64
	progress(0, "copy")
	for _, p := range picks {
		c := byID[p.ID]
		tid := strings.ToUpper(p.TitleID)
		name := gameDir(dir, tid, p.Device)
		replace := name != "" && hasSaveData(filepath.Join(dir, name))
		if name == "" {
			name = o.wantGameFolder(tid, names[tid], p.Device)
		}
		for _, st := range plan { // two saves for the same game in one selection
			if strings.EqualFold(st.name, name) {
				cleanup()
				return nil, fmt.Errorf("Two of the selected saves are for the same game (%s). Select only one of them.", firstNonEmpty(names[tid], tid))
			}
		}
		stage := filepath.Join(dir, name+".nexus-new")
		_ = os.RemoveAll(stage)
		plan = append(plan, staged{c: c, name: name, path: stage, replace: replace})
		var err error
		if src.dir != "" {
			err = backupTree(filepath.Join(src.dir, filepath.FromSlash(c.path)), stage)
		} else {
			err = extractPrefix(src.zr, c.path, stage)
		}
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("Nothing was imported: %s could not be read (%v). Your existing saves are unchanged.", firstNonEmpty(c.Title, c.TitleID, "a save"), err)
		}
		done += c.Bytes
		progress(done, "copy")
	}
	// 2. back up every save that will be replaced; if a backup fails, nothing is replaced
	progress(done, "backup")
	for _, st := range plan {
		if st.replace {
			if _, err := o.Backup(profileID, st.name, "before-import"); err != nil {
				cleanup()
				return nil, fmt.Errorf("Nothing was imported because the backup of %s failed: %v", st.name, err)
			}
		}
	}
	// 3. put the staged saves in place
	for _, st := range plan {
		o.mu.Lock()
		err := swapDir(st.path, filepath.Join(dir, st.name))
		o.mu.Unlock()
		if err != nil {
			cleanup()
			return nil, err
		}
		o.app.Log.Printf("saves: imported %s into %s/%s", firstNonEmpty(st.c.path, src.insp.Source), filepath.Base(dir), st.name)
	}
	progress(total, "done")
	return nil, nil
}

func extractPrefix(zr *zip.Reader, prefix, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	prefix = strings.Trim(prefix, "/")
	for _, f := range zr.File {
		name := strings.Trim(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		rest := name
		if prefix != "" {
			var ok bool
			if rest, ok = strings.CutPrefix(name, prefix+"/"); !ok {
				continue
			}
		}
		if rest == "" || name == saveExportMarker {
			continue
		}
		if _, err := cleanZipName(rest); err != nil {
			return fmt.Errorf("unsafe path in the archive: %w", err)
		}
		if f.Mode()&os.ModeType&^os.ModeDir != 0 {
			return fmt.Errorf("%s is not an ordinary file", name)
		}
		target := filepath.Join(dst, filepath.FromSlash(rest))
		if rel, err := filepath.Rel(dst, target); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("unsafe path in the archive: %s", name)
		}
		if strings.HasSuffix(f.Name, "/") {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := extractZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

// ---------- migration from the 0.2.x layout

type saveLayoutDoc struct {
	Version  int    `json:"version"`
	Migrated int64  `json:"migrated,omitempty"`
	Backup   string `json:"backup,omitempty"`
}

// oldSavesDir is where 0.2.x kept saves (<data>\Saves).
func (o *SaveOrganizer) oldSavesDir() string { return filepath.Join(o.app.Paths.System, "Saves") }

// Migrate converts 0.2.x saves to the 0.3.0 layout once. Everything is backed up into one ZIP
// first; if that fails nothing is changed. Folders are renamed (same drive), never copied and
// deleted. Returns an error to show in Save Data when the migration could not run.
func (o *SaveOrganizer) Migrate() error {
	var doc saveLayoutDoc
	_ = readJSON(filepath.Join(o.root(), saveLayoutFile), &doc)
	if doc.Version >= saveLayoutV {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	old := o.oldSavesDir()
	if dirExists(old) && !sameDir(old, o.root()) {
		if hasSaveData(old) {
			bk := filepath.Join(o.backupsRoot(), fmt.Sprintf("%s migration - all users (NEXUS 0.2 layout).zip", time.Now().Format("2006-01-02 15-04-05")))
			if err := writeZipAtomic(bk, func(zw *zip.Writer) error { return zipDir(zw, old, "Saves") }); err != nil {
				return fmt.Errorf("Your saves were not converted to the new layout because the backup failed (%v). They are untouched in %s.", err, old)
			}
			doc.Backup = bk
			o.app.Log.Printf("saves: backed up the 0.2 save folders to %s", bk)
		}
		if err := os.MkdirAll(filepath.Dir(o.root()), 0o755); err != nil {
			return err
		}
		if !dirExists(o.root()) || dirEmpty(o.root()) {
			_ = os.Remove(o.root())
			if err := os.Rename(old, o.root()); err != nil {
				return fmt.Errorf("Your saves could not be moved to %s (%v). They are untouched in %s.", o.root(), err, old)
			}
		} else {
			// both exist: move each user folder across, keeping anything already there
			entries, _ := os.ReadDir(old)
			for _, e := range entries {
				dst := filepath.Join(o.root(), e.Name())
				if !fileExists(dst) && !dirExists(dst) {
					_ = os.Rename(filepath.Join(old, e.Name()), dst)
				}
			}
		}
	}
	_ = os.MkdirAll(o.root(), 0o755)
	// user folders: "<id> - <name>" / "<id>" → "<PLAYER_ID>-<name>"
	for _, p := range o.app.Profiles.List() {
		cur := o.findOldProfileDir(p.ID)
		pid := p.PlayerID
		if pid == "" {
			pid = o.playerIDFromSaves(cur)
			if pid == "" {
				pid = newPlayerID()
			}
			if err := o.app.Profiles.SetPlayerID(p.ID, pid); err != nil {
				return err
			}
		}
		want := filepath.Join(o.root(), playerFolderName(pid, p.Name))
		if cur != "" && cur != want && !dirExists(want) {
			if err := os.Rename(cur, want); err != nil {
				return fmt.Errorf("the save folder of %s could not be renamed: %v", p.Name, err)
			}
		}
		if dirExists(want) {
			o.migrateReadableFolders(want)
			o.adoptEngineLayout(want)
			o.nameGameFolders(want)
		}
	}
	doc.Version, doc.Migrated = saveLayoutV, time.Now().Unix()
	if err := writeJSONAtomic(filepath.Join(o.root(), saveLayoutFile), doc); err != nil {
		return err
	}
	setHidden(filepath.Join(o.root(), saveLayoutFile))
	o.app.Log.Printf("saves: layout version %d", saveLayoutV)
	return nil
}

func (o *SaveOrganizer) findOldProfileDir(id string) string {
	entries, _ := os.ReadDir(o.root())
	for _, e := range entries {
		if e.IsDir() && (e.Name() == id || strings.HasPrefix(e.Name(), id+" - ")) {
			return filepath.Join(o.root(), e.Name())
		}
	}
	return ""
}

// playerIDFromSaves: the engine user ID the user's saves were written with, if any.
func (o *SaveOrganizer) playerIDFromSaves(dir string) string {
	if dir == "" {
		return ""
	}
	if u := o.consoleUserIn(filepath.Join(dir, "user", "save", "0000000000000000")); u != "" {
		return u
	}
	var m struct {
		Folders map[string]struct{ EnginePath string } `json:"folders"`
	}
	_ = readJSON(filepath.Join(dir, oldSaveMapFile), &m)
	for _, f := range m.Folders {
		parts := strings.Split(f.EnginePath, "/")
		for _, p := range parts {
			if reUserDir.MatchString(p) && strings.Trim(p, "0") != "" {
				return strings.ToUpper(p)
			}
		}
	}
	return ""
}

func (o *SaveOrganizer) consoleUserIn(dir string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && reUserDir.MatchString(e.Name()) && strings.Trim(e.Name(), "0") != "" {
			return strings.ToUpper(e.Name())
		}
	}
	return ""
}

// migrateReadableFolders renames 0.2.x readable folders ("OOT - The Legend of Zelda…") to
// "<GAME_ID>-<SHORT>" using the 0.2.x mapping file, which is then kept as .v02.json.
func (o *SaveOrganizer) migrateReadableFolders(dir string) {
	var m struct {
		Folders map[string]struct {
			TitleID    string `json:"titleId"`
			Game       string `json:"game"`
			EnginePath string `json:"enginePath"`
			Kind       string `json:"kind"`
		} `json:"folders"`
	}
	mp := filepath.Join(dir, oldSaveMapFile)
	if readJSON(mp, &m) != nil {
		return
	}
	names := o.titleNames()
	keys := make([]string, 0, len(m.Folders))
	for k := range m.Folders {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, folder := range keys {
		e := m.Folders[folder]
		src := filepath.Join(dir, folder)
		if e.TitleID == "" || !dirExists(src) {
			continue
		}
		if !hasSaveData(src) {
			_ = os.Remove(src) // an empty placeholder folder made by 0.2.x
			continue
		}
		tid := baseTitleOf(e.TitleID)
		device := strings.Contains(e.Kind, "device")
		if _, err := o.placeGameSave(dir, src, tid, firstNonEmpty(names[tid], e.Game), device); err != nil {
			o.app.Log.Printf("saves: could not convert %q: %v", folder, err)
		}
	}
	clearHidden(mp)
	_ = os.Rename(mp, filepath.Join(dir, ".nexus-saves.v02.json"))
}

// ---------- NEXUS engine storage (saves written when per-user saves were off)

// SharedEngineSaves is the engine's own save folder (used before every user had their own);
// offered in the import, never assigned automatically.
func (o *SaveOrganizer) SharedEngineSaves() string {
	p := filepath.Join(o.app.Emu.DataDir(), "nand", "user", "save")
	if hasSaveData(p) {
		return p
	}
	return ""
}
