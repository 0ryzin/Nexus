package main

// Manage Mods (docs/MODS.md): what NEXUS knows about a game's mods beyond "installed, on or off".
//
//   - Information: an optional nexus-mod.json in the mod's folder (name, description, version,
//     author, category, game version, build IDs, requirements); otherwise what NEXUS can read from
//     the mod itself (how many game files it replaces, which game builds its code patches are for).
//   - Compatibility: code patches (.ips named after a build ID, .pchtxt with @nsobid-) only apply
//     to one build of the game. NEXUS learns the game's build ID from the engine log of the last
//     session and compares; it never guesses. RomFS replacements have no build to check.
//   - Conflicts: two switched-on mods that replace the same game file, or both replace the same
//     ExeFS file; the engine then uses one of them.
//   - Built-in mods: patches compiled into the NEXUS engine by Lichii (core/file_sys/patch_manager.cpp,
//     BUILTIN_PATCHES). They are listed only for the game and build they are made for, and switched
//     with the engine's own add-on mechanism (disabled add-ons), like installed mods.
//   - What applied: after a session NEXUS reads which mods and built-in patches the engine applied
//     (or skipped, and why) from its log, so the list shows facts from the last start.
//
// Mods are stored per game under <engine data>\load\<GAME_ID>\ (gamecontent.go); the game's own
// files are never changed: the engine layers mods over them while the game runs.

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ModMeta is nexus-mod.json (every field optional).
type ModMeta struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Author      string   `json:"author,omitempty"`
	Category    string   `json:"category,omitempty"` // fps | performance | resolution | visual | gameplay | other
	GameVersion string   `json:"gameVersion,omitempty"`
	BuildIDs    []string `json:"buildIds,omitempty"`
	Requires    []string `json:"requires,omitempty"` // names of other mods it needs
	Website     string   `json:"website,omitempty"`
}

// ModCompat: whether a mod can work with the installed game.
type ModCompat struct {
	State string `json:"state"` // ok | mismatch | unknown | any
	Note  string `json:"note"`
}

// ModDetail is one mod in Manage Mods.
type ModDetail struct {
	GameMod
	Builtin   bool      `json:"builtin,omitempty"`
	Meta      ModMeta   `json:"meta"`
	Files     int       `json:"files"`              // game files it replaces (RomFS) or adds
	BuildIDs  []string  `json:"buildIds,omitempty"` // builds its code patches are for
	Compat    ModCompat `json:"compat"`
	Conflicts []string  `json:"conflicts,omitempty"` // switched-on mods replacing the same files
	Missing   []string  `json:"missing,omitempty"`   // required mods not installed or switched off
	Applied   string    `json:"applied,omitempty"`   // last session: applied | skipped: <why> | ""
}

// ModsView is the Manage Mods screen of a game.
type ModsView struct {
	Title     string      `json:"title"`
	TitleID   string      `json:"titleId"`
	Version   string      `json:"version,omitempty"`
	BuildID   string      `json:"buildId,omitempty"` // the game's main build ID from the last session
	LastRun   int64       `json:"lastRun,omitempty"`
	Mods      []ModDetail `json:"mods"`
	Builtin   []ModDetail `json:"builtin"`
	ModsDir   string      `json:"modsDir"`
	Busy      string      `json:"busy,omitempty"`
	CanChange bool        `json:"canChange"`
}

// builtinMod describes a patch compiled into the engine (names must match BUILTIN_PATCHES).
type builtinMod struct {
	Name, TitleID, BuildID, Category, Description string
	Requires                                      []string
	Replaces                                      string
}

// builtinMods: the engine's built-in patches that are listed in Add-Ons (the DLSS render-scale
// patches are not: the engine applies them with DLSS settings, not as mods). From
// src/core/file_sys/patch_manager.cpp on Lichii build 316.
var builtinMods = []builtinMod{
	{Name: "Max Resolution (built-in)", TitleID: "0400486024DEC000", BuildID: "E0F3CCFAF50065AE4EA01EB5AEB13B1A45DA3348", Category: "resolution",
		Description: "The game's 3D always renders at full scale instead of following its dynamic resolution down to 480p.",
		Requires:    []string{"Texture Pool x2 (built-in)"}},
	{Name: "Texture Pool x2 (built-in)", TitleID: "0400486024DEC000", BuildID: "E0F3CCFAF50065AE4EA01EB5AEB13B1A45DA3348", Category: "performance",
		Description: "Doubles the game's texture pool. Max Resolution needs it: without it the guard house runs out of texture memory and the game crashes."},
	{Name: "FPS Unlock (built-in)", TitleID: "0400486024DEC000", BuildID: "E0F3CCFAF50065AE4EA01EB5AEB13B1A45DA3348", Category: "fps",
		Description: "Lets the game run above its own frame rate cap while keeping its real speed (the engine's frame rate limit decides how high). Includes Dynamic FPS.",
		Replaces:    "Dynamic FPS by Dark (built-in)"},
	{Name: "Dynamic FPS by Dark (built-in)", TitleID: "0400486024DEC000", BuildID: "E0F3CCFAF50065AE4EA01EB5AEB13B1A45DA3348", Category: "fps",
		Description: "Dark's dynamic frame rate routine: game speed follows the real frame time. Not used while FPS Unlock is on (FPS Unlock contains it)."},
}

func builtinFor(titleID string) []builtinMod {
	var out []builtinMod
	for _, b := range builtinMods {
		if strings.EqualFold(b.TitleID, titleID) {
			out = append(out, b)
		}
	}
	return out
}

func isBuiltinMod(titleID, name string) bool {
	for _, b := range builtinFor(titleID) {
		if b.Name == name {
			return true
		}
	}
	return false
}

// normBuildID: upper-case hex without the trailing zeros the engine also trims.
func normBuildID(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.TrimRight(s, "0")
}

func sameBuild(a, b string) bool {
	a, b = normBuildID(a), normBuildID(b)
	if a == "" || b == "" {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	return strings.HasPrefix(b, a) && len(a) >= 16
}

var (
	reHexName = regexp.MustCompile(`^[0-9A-Fa-f]{16,64}$`)
	reNsobid  = regexp.MustCompile(`(?i)@nsobid-([0-9a-f]{16,64})`)
)

// scanMod reads what a mod folder contains: replaced files, code-patch build IDs, its metadata.
func scanMod(dir string) (meta ModMeta, files int, builds []string, romfs, exefs map[string]bool) {
	romfs, exefs = map[string]bool{}, map[string]bool{}
	if data, err := os.ReadFile(filepath.Join(dir, "nexus-mod.json")); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	seen := map[string]bool{}
	addBuild := func(b string) {
		if n := normBuildID(b); n != "" && !seen[n] {
			seen[n] = true
			builds = append(builds, n)
		}
	}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		top, rest, _ := strings.Cut(rel, "/")
		switch strings.ToLower(top) {
		case "romfs", "romfs_ext":
			files++
			romfs[strings.ToLower(rest)] = true
		case "exefs", "exefs_patches":
			files++
			name := d.Name()
			ext := strings.ToLower(filepath.Ext(name))
			switch ext {
			case ".ips":
				if stem := strings.TrimSuffix(name, filepath.Ext(name)); reHexName.MatchString(stem) {
					addBuild(stem)
				}
			case ".pchtxt":
				if f, err := os.Open(p); err == nil {
					sc := bufio.NewScanner(f)
					for i := 0; sc.Scan() && i < 400; i++ {
						if m := reNsobid.FindStringSubmatch(sc.Text()); m != nil {
							addBuild(m[1])
						}
					}
					f.Close()
				}
			default:
				exefs[strings.ToLower(name)] = true // a replaced ExeFS file (main, main.npdm, …)
			}
		case "cheats":
			files++
			if stem := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())); reHexName.MatchString(stem) {
				addBuild(stem)
			}
		}
		return nil
	})
	for _, b := range meta.BuildIDs {
		addBuild(b)
	}
	sort.Strings(builds)
	return
}

// ModsView builds the Manage Mods screen for a game.
func (c *GameContent) ModsView(key string) (ModsView, error) {
	g, err := c.game(key)
	if err != nil {
		return ModsView{}, err
	}
	c.mu.Lock()
	doc := c.load(g.TitleID)
	c.mu.Unlock()
	st := c.app.Emu.State()
	v := ModsView{Title: g.Title, TitleID: strings.ToUpper(g.TitleID), Version: g.Version, BuildID: doc.MainBuildID, LastRun: doc.LearnedAt,
		ModsDir: c.modsDir(g.TitleID), Busy: c.Busy(key), CanChange: !(st.Running && st.GameKey == key), Mods: []ModDetail{}, Builtin: []ModDetail{}}
	mods, err := c.Mods(key)
	if err != nil {
		return v, err
	}
	type sets struct{ romfs, exefs map[string]bool }
	files := map[string]sets{}
	enabled := map[string]bool{}
	for _, m := range mods {
		d := ModDetail{GameMod: m}
		var rs, es map[string]bool
		d.Meta, d.Files, d.BuildIDs, rs, es = scanMod(filepath.Join(v.ModsDir, m.Name))
		files[m.Name] = sets{rs, es}
		if m.Enabled {
			enabled[m.Name] = true
		}
		d.Compat = modCompat(d.BuildIDs, doc.MainBuildID)
		d.Applied = doc.Applied[m.Name]
		v.Mods = append(v.Mods, d)
	}
	for _, b := range builtinFor(g.TitleID) {
		on := !contains(doc.DisabledMods, b.Name)
		if on {
			enabled[b.Name] = true
		}
		d := ModDetail{GameMod: GameMod{Name: b.Name, Enabled: on, Contents: []string{"exefs"}}, Builtin: true,
			Meta:     ModMeta{Name: strings.TrimSuffix(b.Name, " (built-in)"), Description: b.Description, Category: b.Category, Author: "Included in the NEXUS engine", Requires: b.Requires},
			BuildIDs: []string{normBuildID(b.BuildID)}, Applied: doc.Applied[b.Name]}
		d.Compat = modCompat(d.BuildIDs, doc.MainBuildID)
		v.Builtin = append(v.Builtin, d)
	}
	// conflicts and missing requirements among switched-on mods
	for i := range v.Mods {
		m := &v.Mods[i]
		if !m.Enabled {
			continue
		}
		for _, o := range v.Mods {
			if o.Name == m.Name || !o.Enabled {
				continue
			}
			if overlaps(files[m.Name].romfs, files[o.Name].romfs) || overlaps(files[m.Name].exefs, files[o.Name].exefs) {
				m.Conflicts = append(m.Conflicts, o.Name)
			}
		}
	}
	all := append(append([]ModDetail{}, v.Mods...), v.Builtin...)
	for _, list := range [][]ModDetail{v.Mods, v.Builtin} {
		for i := range list {
			m := &list[i]
			if !m.Enabled {
				continue
			}
			for _, r := range m.Meta.Requires {
				ok := false
				for _, o := range all {
					if (o.Name == r || o.Meta.Name == r) && o.Enabled {
						ok = true
					}
				}
				if !ok {
					m.Missing = append(m.Missing, r)
				}
			}
		}
	}
	return v, nil
}

func overlaps(a, b map[string]bool) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func modCompat(builds []string, game string) ModCompat {
	switch {
	case len(builds) == 0:
		return ModCompat{State: "any", Note: "Replaces or adds game files; it does not depend on one build of the game."}
	case game == "":
		return ModCompat{State: "unknown", Note: "Made for game build " + shortBuild(builds[0]) + ". NEXUS learns this game's build when it is played once; it is checked then."}
	}
	for _, b := range builds {
		if sameBuild(b, game) {
			return ModCompat{State: "ok", Note: "Made for this game's build (" + shortBuild(game) + ")."}
		}
	}
	return ModCompat{State: "mismatch", Note: "Made for game build " + shortBuild(builds[0]) + ", but this game is build " + shortBuild(game) +
		". The engine skips its code patches: it needs the matching game version (base game or update)."}
}

func shortBuild(b string) string {
	if len(b) > 16 {
		return b[:16] + "…"
	}
	return b
}

// ---------- what the engine applied (from its log)

var (
	reBuildMain   = regexp.MustCompile(`Querying NSO patch existence for build_id=([0-9A-Fa-f]+), name=main\b`)
	reApplyMod    = regexp.MustCompile(`- Applying (?:IPS|IPSwitch) patch from mod "([^"]+)"`)
	reApplyLayer  = regexp.MustCompile(`(?:ExeFS: LayeredExeFS|RomFS: LayeredFS) mod (.+?)(?: \(exefs\))?$`)
	reApplyBuilt  = regexp.MustCompile(`- Applying built-in patch "([^"]+)"`)
	reSkipBuiltin = regexp.MustCompile(`- Skipping "([^"]+)": (.+)$`)
	rePatchingFor = regexp.MustCompile(`Patching ExeFS for title_id=([0-9A-Fa-f]{16})`)
)

// LearnFromEngineLog reads, after a session of titleID, the game's main build ID and which mods
// and built-in patches the engine applied or skipped. Only a log that belongs to this game is used.
func (c *GameContent) LearnFromEngineLog(titleID, logPath string) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer f.Close()
	build := ""
	applied := map[string]string{}
	mine := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		if m := rePatchingFor.FindStringSubmatch(l); m != nil {
			mine = strings.EqualFold(m[1], titleID)
		}
		if !strings.Contains(l, "Loader") {
			continue
		}
		if m := reBuildMain.FindStringSubmatch(l); m != nil && build == "" {
			build = normBuildID(m[1])
		}
		if m := reApplyMod.FindStringSubmatch(l); m != nil {
			applied[m[1]] = "applied"
		}
		if m := reApplyLayer.FindStringSubmatch(l); m != nil {
			applied[strings.TrimSpace(m[1])] = "applied"
		}
		if m := reApplyBuilt.FindStringSubmatch(l); m != nil {
			applied[m[1]] = "applied"
		}
		if m := reSkipBuiltin.FindStringSubmatch(l); m != nil {
			applied[m[1]] = "skipped: " + m[2]
		}
	}
	if !mine || build == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.load(titleID)
	d.MainBuildID, d.Applied, d.LearnedAt = build, applied, time.Now().Unix()
	_ = c.save(titleID, d)
}
