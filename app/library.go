package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Game struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	FileTitle  string `json:"fileTitle"`
	EmbedName  string `json:"embedName,omitempty"`
	Publisher  string `json:"publisher,omitempty"`
	BuiltinArt string `json:"builtinArt,omitempty"` // id of the built-in artwork database entry that matched
	BuiltinWhy string `json:"builtinWhy,omitempty"`
	Released   string `json:"released,omitempty"`
	OnlineName string `json:"onlineName,omitempty"`
	Path       string `json:"path"`
	Dir        string `json:"dir"`
	Format     string `json:"format"`
	Size       int64  `json:"size"`
	TitleID    string `json:"titleId,omitempty"`
	FullID     string `json:"fullId,omitempty"`
	Kind       string `json:"kind"`
	Version    string `json:"version,omitempty"`
	IDSource   string `json:"idSource,omitempty"`
	ArtSource  string `json:"artSource,omitempty"` // custom | sidecar | artwork-folder | embedded | online | game-list-cache | ""
	ArtRev     int64  `json:"artRev,omitempty"`
	ArtPref    string `json:"artPref,omitempty"`
	ArtFit     string `json:"artFit,omitempty"`
	HasCustom  bool   `json:"hasCustom,omitempty"`
	HasEmbed   bool   `json:"hasEmbedded,omitempty"`
	HasOnline  bool   `json:"hasOnline,omitempty"`
	OnlineErr  string `json:"onlineError,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
	PerGame    int    `json:"perGame,omitempty"`  // number of per-game emulator overrides
	Emulator   string `json:"emulator,omitempty"` // per-game emulator choice ("" = default)
	Note       string `json:"note,omitempty"`
}

// cached identification so rescans are instant
type cacheEntry struct {
	Size      int64  `json:"size"`
	ModTime   int64  `json:"mtime"`
	TitleID   string `json:"titleId"`
	FullID    string `json:"fullId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Source    string `json:"source"`
	Publisher string `json:"publisher,omitempty"`
	V         int    `json:"v,omitempty"` // 2 = includes publisher
	HasIcon   bool   `json:"hasIcon"`
	Err       string `json:"err,omitempty"`
	KeysRev   string `json:"keysRev"`
}

type libraryDoc struct {
	// legacy (v1.0/1.1) fields, migrated into System/GameProfiles on load
	Artwork map[string]string     `json:"artwork,omitempty"`
	Titles  map[string]string     `json:"titles,omitempty"`
	Hidden  map[string]bool       `json:"hidden,omitempty"`
	Cache   map[string]cacheEntry `json:"cache"` // file path -> identification
}

type Library struct {
	bdbOnce sync.Once
	bdb     *builtinDB
	acOnce  sync.Once
	ac      *artCache

	app      *App
	mu       sync.Mutex
	doc      libraryDoc
	games    []Game
	byKey    map[string]*Game
	scanning bool
	lastScan time.Time
	scanErrs []string
	path     string
	GP       *GameProfiles
	online   *OnlineArtwork
}

func NewLibrary(a *App) *Library {
	l := &Library{app: a, path: filepath.Join(a.Paths.Settings, "library.json"), byKey: map[string]*Game{},
		GP: NewGameProfiles(filepath.Join(a.Paths.System, "GameProfiles"))}
	_ = readJSON(l.path, &l.doc)
	if l.doc.Cache == nil {
		l.doc.Cache = map[string]cacheEntry{}
	}
	l.online = NewOnlineArtwork(a, l)
	l.migrateLegacy()
	return l
}

// migrateLegacy moves v1 custom artwork, titles and hidden flags into per-game profiles.
func (l *Library) migrateLegacy() {
	if len(l.doc.Artwork)+len(l.doc.Titles)+len(l.doc.Hidden) == 0 {
		return
	}
	for key, f := range l.doc.Artwork {
		src := filepath.Join(l.app.Paths.Artwork, "custom", f)
		if fileExists(src) {
			name := "custom-icon" + filepath.Ext(f)
			_ = os.MkdirAll(l.GP.Dir(key), 0o755)
			if copyFile(src, l.GP.File(key, name)) == nil {
				_, _ = l.GP.Update(key, func(p *GameProfile) { p.CustomIcon = name })
				_ = os.Remove(src)
			}
		}
	}
	for key, t := range l.doc.Titles {
		_, _ = l.GP.Update(key, func(p *GameProfile) { p.Title = t })
	}
	for key, h := range l.doc.Hidden {
		_, _ = l.GP.Update(key, func(p *GameProfile) { p.Hidden = h })
	}
	l.doc.Artwork, l.doc.Titles, l.doc.Hidden = nil, nil, nil
	l.saveLocked()
}

func (l *Library) saveLocked() { _ = writeJSONAtomic(l.path, l.doc) }

func (l *Library) Games() []Game {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Game, len(l.games))
	copy(out, l.games)
	return out
}

func (l *Library) Get(key string) (Game, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	g, ok := l.byKey[key]
	if !ok {
		return Game{}, false
	}
	return *g, true
}

func (l *Library) Status() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	return map[string]any{"scanning": l.scanning, "lastScan": l.lastScan.Unix(), "count": len(l.games), "errors": l.scanErrs,
		"online": l.online.Status()}
}

// ScanDirs returns the directories to scan: the Games folder plus user-added ones.
func (l *Library) ScanDirs() []string {
	s := l.app.Settings.Get()
	dirs := []string{l.app.Paths.Games}
	seen := map[string]bool{strings.ToLower(filepath.Clean(l.app.Paths.Games)): true}
	for _, d := range s.Library.Dirs {
		abs := l.app.Paths.Resolve(d)
		k := strings.ToLower(filepath.Clean(abs))
		if !seen[k] {
			seen[k] = true
			dirs = append(dirs, abs)
		}
	}
	return dirs
}

func (l *Library) keyPaths() []string {
	paths := []string{}
	if l.app.Sys != nil {
		paths = append(paths, l.app.Sys.ProdKeys(), l.app.Sys.TitleKeys())
	}
	paths = append(paths, filepath.Join(l.app.Emu.KeysDir(), "prod.keys"))
	if ad := os.Getenv("APPDATA"); ad != "" {
		paths = append(paths, filepath.Join(ad, "eden", "keys", "prod.keys"))
	}
	return paths
}

func keysRevision(paths []string) string {
	h := sha1.New()
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			fmt.Fprintf(h, "%s|%d|%d;", p, fi.Size(), fi.ModTime().Unix())
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func gameKey(info GameInfo, path string) string {
	if info.TitleID != "" && (info.Kind == "update" || info.Kind == "dlc") && info.FullID != "" {
		return "x-" + strings.ToLower(info.FullID)
	}
	if info.TitleID != "" && info.Kind != "homebrew" {
		return "t-" + strings.ToLower(info.TitleID)
	}
	h := sha1.Sum([]byte(strings.ToLower(pathBase(path))))
	return "f-" + hex.EncodeToString(h[:6])
}

// Scan walks every library directory and rebuilds the game list. force
// re-identifies every file (ignores the cache).
func (l *Library) Scan() error { return l.scan(false, "") }

func (l *Library) scan(force bool, onlyPath string) error {
	l.mu.Lock()
	if l.scanning {
		l.mu.Unlock()
		return errors.New("A scan is already running.")
	}
	l.scanning = true
	l.mu.Unlock()
	l.app.Events.Emit("library.scan", map[string]any{"state": "start"})

	settings := l.app.Settings.Get()
	formats := map[string]bool{}
	for _, f := range settings.Library.ShowFormats {
		if _, ok := SupportedFormats[strings.ToLower(f)]; ok {
			formats[strings.ToLower(f)] = true
		}
	}
	kp := l.keyPaths()
	var ks KeySet
	krev := "nokeys"
	if settings.Library.ReadKeysForIDs {
		ks = LoadKeys(kp...)
		krev = keysRevision(kp)
	}

	var files []string
	var errs []string
	for _, dir := range l.ScanDirs() {
		if !dirExists(dir) {
			errs = append(errs, fmt.Sprintf("Folder not found: %s", dir))
			continue
		}
		root := dir
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				errs = append(errs, fmt.Sprintf("Cannot read %s: %v", path, err))
				return nil
			}
			if d.IsDir() {
				if path != root {
					if !settings.Library.Recursive {
						return filepath.SkipDir
					}
					rel, _ := filepath.Rel(root, path)
					if strings.Count(rel, string(filepath.Separator)) >= 5 || strings.HasPrefix(d.Name(), ".") {
						return filepath.SkipDir
					}
				}
				return nil
			}
			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
			if formats[ext] {
				files = append(files, path)
			}
			return nil
		})
	}

	l.mu.Lock()
	cache := l.doc.Cache
	l.mu.Unlock()

	newCache := map[string]cacheEntry{}
	byKey := map[string]*Game{}
	for i, path := range files {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		ce, ok := cache[path]
		redo := force && (onlyPath == "" || onlyPath == path)
		if ok && ce.HasIcon {
			k := gameKey(GameInfo{TitleID: ce.TitleID, FullID: ce.FullID, Kind: ce.Kind}, path)
			if !fileExists(l.GP.File(k, "embedded-icon.jpg")) {
				// v1 kept extracted icons in System/Artwork/embedded; otherwise re-read the game
				if old := filepath.Join(l.app.Paths.Artwork, "embedded", k+".jpg"); fileExists(old) {
					_ = os.MkdirAll(l.GP.Dir(k), 0o755)
					_ = copyFile(old, l.GP.File(k, "embedded-icon.jpg"))
				} else {
					redo = true
				}
			}
		}
		if ok && ce.V < 2 && ce.Source == "nca" {
			redo = true // re-read once to learn the publisher (artwork database matching)
		}
		if redo || !ok || ce.Size != fi.Size() || ce.ModTime != fi.ModTime().Unix() || ce.KeysRev != krev {
			info, ierr := IdentifyFile(path, ks)
			ce = cacheEntry{Size: fi.Size(), ModTime: fi.ModTime().Unix(), TitleID: info.TitleID, FullID: info.FullID,
				Kind: info.Kind, Name: info.Name, Version: info.Version, Source: info.Source, KeysRev: krev,
				Publisher: info.Publisher, V: 2}
			if ierr != nil {
				ce.Err = ierr.Error()
			}
			if len(info.Icon) > 0 {
				key := gameKey(GameInfo{TitleID: info.TitleID, FullID: info.FullID, Kind: info.Kind}, path)
				_ = os.MkdirAll(l.GP.Dir(key), 0o755)
				if err := os.WriteFile(l.GP.File(key, "embedded-icon.jpg"), info.Icon, 0o644); err == nil {
					ce.HasIcon = true
				}
			}
			if i%5 == 0 {
				l.app.Events.Emit("library.scan", map[string]any{"state": "progress", "done": i, "total": len(files)})
			}
		}
		newCache[path] = ce
		key := gameKey(GameInfo{TitleID: ce.TitleID, FullID: ce.FullID, Kind: ce.Kind}, path)
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		g := Game{Key: key, Path: path, Dir: filepath.Dir(path), Format: ext, Size: fi.Size(),
			TitleID: ce.TitleID, FullID: ce.FullID, Kind: ce.Kind, Version: ce.Version, IDSource: ce.Source,
			FileTitle: CleanTitle(path), EmbedName: ce.Name, Publisher: ce.Publisher, Note: ce.Err}
		if g.Kind == "" {
			g.Kind = "unknown"
		}
		if existing, dup := byKey[key]; dup && existing.Size >= g.Size {
			continue // same title in two files: keep the larger one
		}
		gc := g
		byKey[key] = &gc
	}
	l.mu.Lock()
	games := make([]Game, 0, len(byKey))
	for _, g := range byKey {
		l.decorateLocked(g)
		games = append(games, *g)
	}
	sort.Slice(games, func(i, j int) bool { return strings.ToLower(games[i].Title) < strings.ToLower(games[j].Title) })
	l.games = games
	l.byKey = map[string]*Game{}
	for i := range l.games {
		l.byKey[l.games[i].Key] = &l.games[i]
	}
	l.doc.Cache = newCache
	l.saveLocked()
	l.scanning = false
	l.lastScan = time.Now()
	l.scanErrs = errs
	l.mu.Unlock()

	for _, g := range games {
		if g.TitleID != "" && g.Kind != "update" && g.Kind != "dlc" {
			_, _ = l.GP.Update(g.Key, func(p *GameProfile) { p.TitleID, p.LastPath = g.TitleID, g.Path })
		}
	}
	l.app.Log.Printf("library: %d files, %d games, %d issues", len(files), len(games), len(errs))
	l.app.Events.Emit("library.scan", map[string]any{"state": "done", "count": len(games), "errors": errs})
	l.online.Queue(games, false)
	return nil
}

// decorateLocked fills the title, artwork and per-game fields from the game's profile.
func (l *Library) decorateLocked(g *Game) {
	p := l.GP.Get(g.Key)
	g.Hidden = p.Hidden
	g.ArtPref = p.ArtPreference
	if g.ArtPref == "" {
		g.ArtPref = "auto"
	}
	g.ArtFit = p.ArtFit
	if g.ArtFit == "" {
		g.ArtFit = "cover"
	}
	g.OnlineName = p.Online.Name
	if g.Publisher == "" {
		g.Publisher = p.Online.Publisher
	}
	g.Released = p.Online.Released
	g.OnlineErr = p.Online.Error
	g.PerGame = len(gameNexusSettings(p))
	g.Emulator = p.Emulator
	if p.InputProfile != "" {
		g.PerGame++
	}
	switch {
	case p.Title != "":
		g.Title = p.Title
	case g.EmbedName != "":
		g.Title = g.EmbedName
	case l.gameListCachedName(g.TitleID) != "":
		g.Title = l.gameListCachedName(g.TitleID)
	case p.Online.Name != "":
		g.Title = p.Online.Name
	default:
		g.Title = g.FileTitle
	}
	g.HasCustom = p.CustomIcon != "" && fileExists(l.GP.File(g.Key, p.CustomIcon))
	g.HasEmbed = fileExists(l.GP.File(g.Key, "embedded-icon.jpg"))
	g.HasOnline = p.Online.File != "" && fileExists(l.GP.File(g.Key, p.Online.File))
	path, src := l.artFor(g, p)
	g.ArtSource = src
	g.ArtRev = 0
	if path != "" {
		if fi, err := os.Stat(path); err == nil {
			g.ArtRev = fi.ModTime().UnixMilli()
		}
	}
}

func (l *Library) gameListCacheDir() string {
	return filepath.Join(l.app.Emu.UserDir(), "cache", "game_list")
}

func (l *Library) gameListCachedName(titleID string) string {
	if titleID == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(l.gameListCacheDir(), titleID+".appname.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

var imageExts = []string{".png", ".jpg", ".jpeg", ".webp"}

// artFor picks the artwork for a game. With preference "auto" the order is:
// custom pick > image beside the game file > System/Artwork/<TitleID|name> >
// icon embedded in the game > downloaded artwork > Lichii's icon cache.
// "embedded" / "online" / "custom" put that source first.
func (l *Library) artFor(g *Game, p GameProfile) (string, string) {
	custom := func() string {
		if p.CustomIcon != "" {
			if f := l.GP.File(g.Key, p.CustomIcon); fileExists(f) {
				return f
			}
		}
		return ""
	}
	user := func() (string, string) {
		stem := strings.TrimSuffix(g.Path, filepath.Ext(g.Path))
		for _, e := range imageExts {
			if fileExists(stem + e) {
				return stem + e, "sidecar"
			}
		}
		names := []string{}
		if g.TitleID != "" {
			names = append(names, g.TitleID, strings.ToLower(g.TitleID))
		}
		names = append(names, safeName(g.Title), safeName(g.FileTitle))
		for _, n := range names {
			for _, e := range imageExts {
				if pth := filepath.Join(l.app.Paths.Artwork, n+e); fileExists(pth) {
					return pth, "artwork-folder"
				}
			}
		}
		return "", ""
	}
	embedded := func() string {
		if f := l.GP.File(g.Key, "embedded-icon.jpg"); fileExists(f) {
			return f
		}
		return ""
	}
	online := func() string {
		if p.Online.File != "" {
			if f := l.GP.File(g.Key, p.Online.File); fileExists(f) {
				return f
			}
		}
		return ""
	}
	cached := func() string {
		if g.TitleID != "" {
			pth := filepath.Join(l.gameListCacheDir(), g.TitleID+".jpeg")
			if fi, err := os.Stat(pth); err == nil && fi.Size() > 0 {
				return pth
			}
		}
		return ""
	}
	switch p.ArtPreference {
	case "custom":
		if f := custom(); f != "" {
			return f, "custom"
		}
	case "embedded":
		if f := embedded(); f != "" {
			return f, "embedded"
		}
	case "online":
		if f := online(); f != "" {
			return f, "online"
		}
	case "builtin":
		if f := l.builtinArt(g, p); f != "" {
			return f, "builtin"
		}
	}
	if f := custom(); f != "" {
		return f, "custom"
	}
	if f, src := user(); f != "" {
		return f, src
	}
	if f := l.builtinArt(g, p); f != "" {
		return f, "builtin"
	}
	if f := embedded(); f != "" {
		return f, "embedded"
	}
	if f := online(); f != "" {
		return f, "online"
	}
	if f := cached(); f != "" {
		return f, "game-list-cache"
	}
	return "", ""
}

// ArtPath serves the current artwork; variant "embedded"/"online"/"custom"
// returns that specific source (for the artwork editor preview).
func (l *Library) ArtPath(key, variant string) string {
	l.mu.Lock()
	g, ok := l.byKey[key]
	l.mu.Unlock()
	if !ok {
		return ""
	}
	p := l.GP.Get(key)
	switch variant {
	case "embedded":
		return existing(l.GP.File(key, "embedded-icon.jpg"))
	case "builtin":
		return l.builtinArt(g, p)
	case "online":
		if p.Online.File != "" {
			return existing(l.GP.File(key, p.Online.File))
		}
		return ""
	case "custom":
		if p.CustomIcon != "" {
			return existing(l.GP.File(key, p.CustomIcon))
		}
		return ""
	}
	path, _ := l.artFor(g, p)
	return path
}

func existing(p string) string {
	if fileExists(p) {
		return p
	}
	return ""
}

func (l *Library) redecorate(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if g, ok := l.byKey[key]; ok {
		l.decorateLocked(g)
	}
}

func (l *Library) has(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.byKey[key]
	return ok
}

// SetCustomArt stores the user's image (original quality, any size/aspect)
// as System/GameProfiles/<ID>/custom-icon.<ext>.
func (l *Library) SetCustomArt(key string, data []byte, ext string) error {
	if !l.has(key) {
		return errors.New("Game not found.")
	}
	_ = os.MkdirAll(l.GP.Dir(key), 0o755)
	for _, e := range append(imageExts, ".gif") {
		_ = os.Remove(l.GP.File(key, "custom-icon"+e))
	}
	name := "custom-icon" + ext
	if err := os.WriteFile(l.GP.File(key, name), data, 0o644); err != nil {
		return err
	}
	if _, err := l.GP.Update(key, func(p *GameProfile) { p.CustomIcon = name; p.ArtPreference = "custom" }); err != nil {
		return err
	}
	l.redecorate(key)
	return nil
}

func (l *Library) ClearCustomArt(key string) error {
	if !l.has(key) {
		return errors.New("Game not found.")
	}
	p := l.GP.Get(key)
	if p.CustomIcon != "" {
		_ = os.Remove(l.GP.File(key, p.CustomIcon))
	}
	_, err := l.GP.Update(key, func(p *GameProfile) {
		p.CustomIcon = ""
		if p.ArtPreference == "custom" {
			p.ArtPreference = "auto"
		}
	})
	l.redecorate(key)
	return err
}

// RemoveOnlineArt deletes downloaded artwork and stops NEXUS re-downloading it.
func (l *Library) RemoveOnlineArt(key string) error {
	if !l.has(key) {
		return errors.New("Game not found.")
	}
	p := l.GP.Get(key)
	if p.Online.File != "" {
		_ = os.Remove(l.GP.File(key, p.Online.File))
	}
	_, err := l.GP.Update(key, func(p *GameProfile) {
		p.Online.File = ""
		p.Online.Error = "removed by user"
		p.Online.CheckedAt = time.Now().Add(100 * 365 * 24 * time.Hour).Unix() // never auto-retry
		if p.ArtPreference == "online" {
			p.ArtPreference = "auto"
		}
	})
	l.redecorate(key)
	return err
}

func (l *Library) SetArtOptions(key, pref, fit string) error {
	if !l.has(key) {
		return errors.New("Game not found.")
	}
	_, err := l.GP.Update(key, func(p *GameProfile) {
		if pref != "" {
			p.ArtPreference = pref
		}
		if fit == "cover" || fit == "contain" {
			p.ArtFit = fit
		}
	})
	l.redecorate(key)
	return err
}

func (l *Library) SetTitle(key, title string) error {
	if !l.has(key) {
		return errors.New("Game not found.")
	}
	_, err := l.GP.Update(key, func(p *GameProfile) { p.Title = strings.TrimSpace(title) })
	l.redecorate(key)
	return err
}

func (l *Library) SetHidden(key string, hidden bool) error {
	if !l.has(key) {
		return errors.New("Game not found.")
	}
	_, err := l.GP.Update(key, func(p *GameProfile) { p.Hidden = hidden })
	l.redecorate(key)
	return err
}

// RefreshGame re-reads the game file and re-checks online artwork.
func (l *Library) RefreshGame(key string) error {
	g, ok := l.Get(key)
	if !ok {
		return errors.New("Game not found.")
	}
	_, _ = l.GP.Update(key, func(p *GameProfile) {
		if p.Online.Error == "removed by user" {
			p.Online.Error = ""
		}
		p.Online.CheckedAt = 0
	})
	if err := l.scan(true, g.Path); err != nil {
		return err
	}
	if ng, ok := l.Get(key); ok {
		l.online.Queue([]Game{ng}, true)
	}
	return nil
}

// RefreshArt re-resolves artwork for every game (e.g. after Lichii populated its icon cache).
func (l *Library) RefreshArt() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.games {
		l.decorateLocked(&l.games[i])
	}
}
