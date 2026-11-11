package main

// Game updates and mods, managed per game (Configure Game › Game Updates / Mods).
//
// Updates: NEXUS reads the package the user selects and accepts it only if it really is an update
// (by its content, not its file name) for this game. Accepted updates are copied into
//   <data>\UserData\Games\<GAME_ID>\Updates\
// and given to the engine when the game starts (disabled ones are not). Updates found in the
// game folders keep working as before.
//
// Mods: a mod is a folder with romfs, exefs, romfs_ext and/or cheats inside (the layout the
// engine loads), or exefs patches (.pchtxt/.ips). NEXUS installs it into the engine's mod folder
// for this game only (<engine data>\load\<GAME_ID>\<mod name>), where the engine finds it; a mod
// that names another game's ID is refused. Disabled mods are passed to the engine as disabled
// add-ons for this game. Both folders are user data: NEXUS updates never touch them.

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type GameUpdate struct {
	File     string `json:"file"`
	FullID   string `json:"fullId"`
	Version  string `json:"version,omitempty"`
	Enabled  bool   `json:"enabled"`
	Added    int64  `json:"added"`
	Bytes    int64  `json:"bytes"`
	Source   string `json:"source"`             // how the package was identified (nca, ticket, nup)
	Original string `json:"original,omitempty"` // where it was added from
}

type GameMod struct {
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Bytes    int64    `json:"bytes"`
	Contents []string `json:"contents"` // romfs, exefs, cheats, …
	Modified int64    `json:"modified"`
}

type gameContentDoc struct {
	Updates      []GameUpdate `json:"updates"`
	DisabledMods []string     `json:"disabledMods"`
}

type GameContent struct {
	app *App
	mu  sync.Mutex
	job map[string]string // game key -> what is being added (one at a time per game)
}

func NewGameContent(a *App) *GameContent { return &GameContent{app: a, job: map[string]string{}} }

func (c *GameContent) gameDir(titleID string) string {
	return filepath.Join(c.app.Paths.UserData, "Games", strings.ToUpper(titleID))
}
func (c *GameContent) updatesDir(titleID string) string {
	return filepath.Join(c.gameDir(titleID), "Updates")
}
func (c *GameContent) modsDir(titleID string) string {
	return filepath.Join(c.app.Emu.DataDir(), "load", strings.ToUpper(titleID))
}

func (c *GameContent) load(titleID string) gameContentDoc {
	var d gameContentDoc
	_ = readJSON(filepath.Join(c.gameDir(titleID), "content.json"), &d)
	return d
}

func (c *GameContent) save(titleID string, d gameContentDoc) error {
	return writeJSONAtomic(filepath.Join(c.gameDir(titleID), "content.json"), d)
}

func (c *GameContent) game(key string) (Game, error) {
	g, ok := c.app.Library.Get(key)
	if !ok {
		return Game{}, errors.New("Game not found.")
	}
	if g.TitleID == "" || g.Kind == "homebrew" {
		return Game{}, errors.New("NEXUS does not know this game's ID, so updates and mods cannot be matched to it.")
	}
	return g, nil
}

// ---------- updates

// Updates lists a game's managed updates (missing files are reported, not hidden).
func (c *GameContent) Updates(key string) ([]GameUpdate, error) {
	g, err := c.game(key)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.load(g.TitleID)
	out := []GameUpdate{}
	for _, u := range d.Updates {
		if !fileExists(filepath.Join(c.updatesDir(g.TitleID), u.File)) {
			u.Enabled = false
			u.Source = "missing"
		}
		out = append(out, u)
	}
	return out, nil
}

// InspectUpdate reads a package and says whether it is an update for this game.
func (c *GameContent) InspectUpdate(key, path string) (GameUpdate, error) {
	g, err := c.game(key)
	if err != nil {
		return GameUpdate{}, err
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if ext != "nsp" && ext != "nsz" && ext != "xci" && ext != "xcz" && ext != "nup" {
		return GameUpdate{}, errors.New("Game updates are NSP, NSZ, XCI, XCZ or NUP packages.")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return GameUpdate{}, errors.New("That file cannot be read.")
	}
	var ks KeySet
	if c.app.Settings.Get().Library.ReadKeysForIDs {
		ks = LoadKeys(c.app.Library.keyPaths()...)
	}
	info, ierr := IdentifyFile(path, ks)
	if ext == "nup" {
		// a NUP package may carry the base game and its update: the update package counts
		f, err := os.Open(path)
		if err == nil {
			pkgs, perr := ReadNupPackages(f, fi.Size())
			f.Close()
			if perr == nil {
				for _, p := range pkgs {
					if p.Kind == "update" {
						b, _ := classifyID(p.TitleID)
						info = GameInfo{TitleID: fmtID(b), FullID: fmtID(p.TitleID), Kind: "update", Version: p.DisplayVersion, Source: "nup"}
						break
					}
				}
			}
		}
	}
	switch {
	case info.Source == "" || info.Source == "filename":
		msg := "NEXUS could not read what this package contains"
		if ierr != nil {
			msg += " (" + ierr.Error() + ")"
		}
		return GameUpdate{}, errors.New(msg + ". Install your keys in Settings › System files so NEXUS can check it.")
	case info.Kind == "base":
		return GameUpdate{}, errors.New("This package is a game, not an update.")
	case info.Kind == "dlc":
		return GameUpdate{}, errors.New("This package is downloadable content (DLC), not an update. Put DLC in your games folder.")
	case info.Kind != "update":
		return GameUpdate{}, errors.New("This package is not a game update.")
	case !strings.EqualFold(info.TitleID, g.TitleID):
		other := info.TitleID
		for _, o := range c.app.Library.Games() {
			if strings.EqualFold(o.TitleID, info.TitleID) && o.Kind != "update" && o.Kind != "dlc" {
				other = o.Title + " (" + info.TitleID + ")"
				break
			}
		}
		return GameUpdate{}, fmt.Errorf("This update is for another game: %s. It can't be added to %s.", other, g.Title)
	}
	u := GameUpdate{FullID: info.FullID, Version: info.Version, Bytes: fi.Size(), Source: info.Source, Original: path, Enabled: true}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, have := range c.load(g.TitleID).Updates {
		if have.FullID == u.FullID && have.Version == u.Version && have.Bytes == u.Bytes {
			return GameUpdate{}, fmt.Errorf("This update (%s) has already been added.", firstNonEmpty(u.Version, u.FullID))
		}
	}
	return u, nil
}

// AddUpdate copies a checked update package into the game's Updates folder (in the background:
// update packages can be large). Emits "gamecontent.changed" when done.
func (c *GameContent) AddUpdate(key, path string) (GameUpdate, error) {
	u, err := c.InspectUpdate(key, path)
	if err != nil {
		return u, err
	}
	g, _ := c.game(key)
	c.mu.Lock()
	if c.job[key] != "" {
		c.mu.Unlock()
		return u, errors.New("An update is still being added to this game.")
	}
	c.job[key] = filepath.Base(path)
	c.mu.Unlock()
	go func() {
		err := c.copyUpdate(g, u)
		c.mu.Lock()
		delete(c.job, key)
		c.mu.Unlock()
		ev := map[string]any{"gameKey": key, "kind": "update"}
		if err != nil {
			ev["error"] = err.Error()
			c.app.Log.Printf("updates: %v", err)
		}
		c.app.Events.Emit("gamecontent.changed", ev)
	}()
	return u, nil
}

func (c *GameContent) copyUpdate(g Game, u GameUpdate) error {
	dir := c.updatesDir(g.TitleID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := safeFolderName(strings.TrimSuffix(filepath.Base(u.Original), filepath.Ext(u.Original))) + strings.ToLower(filepath.Ext(u.Original))
	dst := filepath.Join(dir, name)
	for i := 2; fileExists(dst); i++ {
		dst = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, filepath.Ext(name)), i, filepath.Ext(name)))
	}
	if err := copyFileTmpRename(u.Original, dst); err != nil {
		return fmt.Errorf("the update could not be copied: %w", err)
	}
	u.File, u.Added = filepath.Base(dst), time.Now().Unix()
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.load(g.TitleID)
	d.Updates = append(d.Updates, u)
	c.app.Log.Printf("updates: added %s (%s) to %s", u.File, firstNonEmpty(u.Version, u.FullID), g.Title)
	return c.save(g.TitleID, d)
}

func (c *GameContent) Busy(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.job[key]
}

func (c *GameContent) SetUpdateEnabled(key, file string, on bool) error {
	g, err := c.game(key)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.load(g.TitleID)
	for i := range d.Updates {
		if d.Updates[i].File == file {
			d.Updates[i].Enabled = on
			return c.save(g.TitleID, d)
		}
	}
	return errors.New("That update is not installed.")
}

// RemoveUpdate deletes NEXUS's copy of an update (the package it was added from is not touched).
func (c *GameContent) RemoveUpdate(key, file string) error {
	g, err := c.game(key)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.load(g.TitleID)
	for i, u := range d.Updates {
		if u.File == file {
			if p := filepath.Join(c.updatesDir(g.TitleID), filepath.Base(file)); fileExists(p) {
				if err := os.Remove(p); err != nil {
					return fmt.Errorf("the update file could not be removed (is the game running?): %w", err)
				}
			}
			d.Updates = append(d.Updates[:i], d.Updates[i+1:]...)
			return c.save(g.TitleID, d)
		}
	}
	return errors.New("That update is not installed.")
}

// EnabledUpdateFiles: the managed updates the engine gets for a game.
func (c *GameContent) EnabledUpdateFiles(titleID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, u := range c.load(titleID).Updates {
		if p := filepath.Join(c.updatesDir(titleID), u.File); u.Enabled && fileExists(p) {
			out = append(out, p)
		}
	}
	return out
}

// ---------- mods

var modParts = []string{"romfs", "exefs", "romfs_ext", "cheats", "exefs_patches"}

func isModRoot(v saveVFS, dir string) []string {
	var parts []string
	for _, fi := range v.list(dir) {
		n := strings.ToLower(fi.Name())
		if fi.IsDir() && contains(modParts, n) {
			parts = append(parts, n)
		}
	}
	return parts
}

// Mods lists the mods in the engine's mod folder for this game (also ones copied in by hand).
func (c *GameContent) Mods(key string) ([]GameMod, error) {
	g, err := c.game(key)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	disabled := c.load(g.TitleID).DisabledMods
	c.mu.Unlock()
	dir := c.modsDir(g.TitleID)
	entries, _ := os.ReadDir(dir)
	out := []GameMod{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, _, mod := dirStat(p)
		out = append(out, GameMod{Name: e.Name(), Enabled: !contains(disabled, e.Name()), Bytes: b, Modified: mod, Contents: isModRoot(osVFS{root: p}, "")})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// DisabledMods: names the engine is told to skip for this game.
func (c *GameContent) DisabledMods(titleID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.load(titleID).DisabledMods...)
}

func modName(s string) string {
	s = strings.NewReplacer(`"`, "", "/", " ", `\`, " ").Replace(s)
	return safeFolderName(s)
}

type modFound struct {
	name, dir string // dir: path inside the source to the mod's root ("" = root)
	patches   bool   // loose .pchtxt/.ips files: installed under exefs
}

// findMods looks for mods in a folder or ZIP for game titleID.
func findMods(v saveVFS, rootName, titleID string) ([]modFound, error) {
	var out []modFound
	var walk func(dir, name string, depth int) error
	walk = func(dir, name string, depth int) error {
		if depth > 6 {
			return nil
		}
		if len(isModRoot(v, dir)) > 0 {
			out = append(out, modFound{name: name, dir: dir})
			return nil
		}
		loose := false
		for _, fi := range v.list(dir) {
			ext := strings.ToLower(filepath.Ext(fi.Name()))
			if !fi.IsDir() && (ext == ".pchtxt" || ext == ".ips") {
				loose = true
			}
		}
		if loose {
			out = append(out, modFound{name: name, dir: dir, patches: true})
			return nil
		}
		for _, fi := range v.list(dir) {
			if !fi.IsDir() {
				continue
			}
			sub := strings.TrimPrefix(dir+"/"+fi.Name(), "/")
			if reTitleDir.MatchString(fi.Name()) {
				// atmosphere/contents/<ID>/… or <ID>/<mod>/…: the ID must be this game's
				if !strings.EqualFold(baseTitleOf(fi.Name()), titleID) && !strings.EqualFold(fi.Name(), titleID) {
					return fmt.Errorf("This mod is for another game (%s).", strings.ToUpper(fi.Name()))
				}
				if err := walk(sub, name, depth+1); err != nil {
					return err
				}
				continue
			}
			n := fi.Name()
			if strings.EqualFold(n, "atmosphere") || strings.EqualFold(n, "contents") {
				n = name
			}
			if err := walk(sub, n, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("", rootName, 0); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("No mod was found there. A mod has romfs, exefs, romfs_ext or cheats folders, or .pchtxt / .ips patch files.")
	}
	return out, nil
}

// AddMod installs the mods in a folder or ZIP for this game. Mods whose name is already
// installed are refused (remove the old one first), so nothing is overwritten.
func (c *GameContent) AddMod(key, path string, zipData []byte, zipName string) ([]string, error) {
	g, err := c.game(key)
	if err != nil {
		return nil, err
	}
	if st := c.app.Emu.State(); st.Running && st.GameKey == key {
		return nil, errors.New("Close the game first.")
	}
	var v saveVFS
	var zr *zip.Reader
	root := ""
	switch {
	case zipData != nil:
		zr, err = zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
		if err != nil {
			return nil, errors.New("That file is not a ZIP archive. Extract other archive types first.")
		}
		v, root = newZipVFS(zr), strings.TrimSuffix(zipName, filepath.Ext(zipName))
	case strings.EqualFold(filepath.Ext(path), ".zip"):
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return c.AddMod(key, "", data, filepath.Base(path))
	default:
		fi, err := os.Stat(path)
		if err != nil || !fi.IsDir() {
			return nil, errors.New("Choose a mod folder or a ZIP archive of one.")
		}
		v, root = osVFS{root: path}, filepath.Base(path)
	}
	mods, err := findMods(v, root, g.TitleID)
	if err != nil {
		return nil, err
	}
	dst := c.modsDir(g.TitleID)
	for _, m := range mods {
		if dirExists(filepath.Join(dst, modName(m.name))) {
			return nil, fmt.Errorf("A mod called %q is already installed for this game. Remove it first to replace it.", modName(m.name))
		}
	}
	var added []string
	for _, m := range mods {
		name := modName(m.name)
		stage := filepath.Join(dst, ".nexus-new-"+name)
		_ = os.RemoveAll(stage)
		target := stage
		if m.patches {
			target = filepath.Join(stage, "exefs")
		}
		if zr != nil {
			err = extractPrefix(zr, m.dir, target)
		} else {
			err = backupTree(filepath.Join(path, filepath.FromSlash(m.dir)), target)
		}
		if err == nil {
			err = os.Rename(stage, filepath.Join(dst, name))
		}
		if err != nil {
			_ = os.RemoveAll(stage)
			return added, fmt.Errorf("the mod %q could not be installed: %w", name, err)
		}
		added = append(added, name)
		c.app.Log.Printf("mods: installed %q for %s", name, g.Title)
	}
	return added, nil
}

func (c *GameContent) SetModEnabled(key, name string, on bool) error {
	g, err := c.game(key)
	if err != nil {
		return err
	}
	if !dirExists(filepath.Join(c.modsDir(g.TitleID), filepath.Base(name))) {
		return errors.New("That mod is not installed.")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.load(g.TitleID)
	var keep []string
	for _, n := range d.DisabledMods {
		if n != name {
			keep = append(keep, n)
		}
	}
	if !on {
		keep = append(keep, name)
	}
	d.DisabledMods = keep
	return c.save(g.TitleID, d)
}

// RemoveMod moves a mod to the Recycle Bin.
func (c *GameContent) RemoveMod(key, name string) error {
	g, err := c.game(key)
	if err != nil {
		return err
	}
	if st := c.app.Emu.State(); st.Running && st.GameKey == key {
		return errors.New("Close the game first.")
	}
	p := filepath.Join(c.modsDir(g.TitleID), filepath.Base(name))
	if name == "" || !dirExists(p) {
		return errors.New("That mod is not installed.")
	}
	if err := recycleFile(p); err != nil {
		return err
	}
	_ = c.SetModEnabledLocked(g.TitleID, name)
	return nil
}

// SetModEnabledLocked drops a removed mod from the disabled list.
func (c *GameContent) SetModEnabledLocked(titleID, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.load(titleID)
	var keep []string
	for _, n := range d.DisabledMods {
		if n != name {
			keep = append(keep, n)
		}
	}
	d.DisabledMods = keep
	return c.save(titleID, d)
}

// writeDisabledAddOns puts a game's disabled mods into the session configuration the engine
// reads ([DisabledAddOns], in the engine's array format).
func writeDisabledAddOns(ini *IniFile, titleID string, names []string) {
	ini.DeleteSection("DisabledAddOns")
	if len(names) == 0 {
		return
	}
	id, err := parseUint16Hex(titleID)
	if err != nil {
		return
	}
	ini.Set("DisabledAddOns", "size", "1")
	ini.Set("DisabledAddOns", `1\title_id\default`, "false")
	ini.Set("DisabledAddOns", `1\title_id`, fmt.Sprintf("%d", id))
	ini.Set("DisabledAddOns", `1\disabled\size`, fmt.Sprintf("%d", len(names)))
	for i, n := range names {
		ini.Set("DisabledAddOns", fmt.Sprintf(`1\disabled\%d\d\default`, i+1), "false")
		ini.Set("DisabledAddOns", fmt.Sprintf(`1\disabled\%d\d`, i+1), n)
	}
}

func parseUint16Hex(s string) (uint64, error) {
	var v uint64
	_, err := fmt.Sscanf(strings.ToUpper(s), "%016X", &v)
	return v, err
}
