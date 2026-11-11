package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	AppName      = "NEXUS"
	ConsoleModel = "NEXUS Switch"
)

// AppVersion is overridden at build time with -ldflags "-X main.AppVersion=..."
var AppVersion = "1.0.0-dev"

// Paths holds every folder NEXUS uses. All of them live under Root so the
// whole install stays portable.
type Paths struct {
	Root       string // install folder: NEXUS.exe, Engine, Games, Resources (replaceable on update)
	Data       string // user data: profiles, saves, settings, artwork, logs (survives updates)
	IsPortable bool
	Games      string
	System     string
	Profiles   string
	Saves      string
	Settings   string
	Artwork    string
	Wallpapers string
	Resources  string
	Logs       string
	Licences   string
	WebView    string
	Trash      string
}

func resolveRoot() string {
	if r := os.Getenv("NEXUS_ROOT"); r != "" {
		abs, _ := filepath.Abs(r)
		return abs
	}
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}

// resolveData picks where user data lives. Like Eden (%APPDATA%\eden), NEXUS keeps
// profiles, saves and settings outside its own folder so deleting/replacing the
// NEXUS folder for an update never loses them. A file named "portable.txt" next
// to NEXUS.exe keeps everything inside the NEXUS folder instead.
func resolveData(root string) (string, bool) {
	if d := os.Getenv("NEXUS_DATA"); d != "" {
		abs, _ := filepath.Abs(d)
		return abs, false
	}
	if fileExists(filepath.Join(root, "portable.txt")) {
		return root, true
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		return filepath.Join(ad, "NEXUS"), false
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "NEXUS"), false
	}
	return root, true
}

// NewPaths keeps everything under root (portable layout, used by tests).
func NewPaths(root string) *Paths { return NewPathsSplit(root, root, true) }

func NewPathsSplit(root, data string, portable bool) *Paths {
	p := &Paths{Root: root, Data: data, IsPortable: portable}
	p.Games = filepath.Join(root, "Games")
	p.Resources = filepath.Join(root, "Resources")
	p.Licences = filepath.Join(root, "Licences")
	p.System = filepath.Join(data, "System")
	if !portable {
		p.System = data
	}
	p.Profiles = filepath.Join(p.System, "Profiles")
	p.Saves = filepath.Join(p.System, "Saves")
	p.Settings = filepath.Join(p.System, "Settings")
	p.Artwork = filepath.Join(p.System, "Artwork")
	p.Wallpapers = filepath.Join(p.System, "Wallpapers")
	p.Trash = filepath.Join(p.System, "Archived")
	p.Logs = filepath.Join(data, "Logs")
	p.WebView = filepath.Join(p.System, "WebView")
	if la := os.Getenv("LOCALAPPDATA"); la != "" && !portable {
		p.WebView = filepath.Join(la, "NEXUS", "WebView") // browser cache: local, not roaming
	}
	return p
}

func (p *Paths) Ensure() error {
	// user data must be writable
	for _, d := range []string{p.Profiles, p.Saves, p.Settings, p.Artwork, p.Wallpapers, p.Logs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("cannot create %s: %w", d, err)
		}
	}
	// install-folder layout is best effort (it may be read-only, e.g. Program Files)
	for _, d := range []string{p.Games, p.Licences,
		filepath.Join(p.Resources, "Icons"), filepath.Join(p.Resources, "Sounds"),
		filepath.Join(p.Resources, "Animations"), filepath.Join(p.Resources, "Themes"), filepath.Join(p.Resources, "ProfileIcons")} {
		_ = os.MkdirAll(d, 0o755)
	}
	return nil
}

// App wires every subsystem together.
type App struct {
	Paths    *Paths
	Log      *log.Logger
	Events   *EventBus
	Settings *SettingsStore
	Profiles *ProfileStore
	Library  *Library
	Emu      *Emulator
	Avatars  *AvatarSet
	Sys      *SystemFiles
	Link     *DeviceLink // experimental Device Linking; nil unless the feature is on
	Compat   *CompatStore
	Saves    *SaveOrganizer
	UI       UIHost // platform window (may be a no-op)

	logFile  *os.File
	migrated []string
	mu       sync.Mutex
}

func NewApp(root string) (*App, error) { return NewAppWithPaths(NewPaths(root)) }

func NewAppWithPaths(p *Paths) (*App, error) {
	root := p.Root
	if err := p.Ensure(); err != nil {
		return nil, err
	}
	a := &App{Paths: p, Events: NewEventBus(), UI: noopUI{}}
	a.openLog()
	a.Log.Printf("NEXUS %s starting, install=%s data=%s portable=%v", AppVersion, root, p.Data, p.IsPortable)

	var err error
	if a.Settings, err = LoadSettings(p); err != nil {
		a.Log.Printf("settings: %v (defaults used)", err)
	}
	if a.Profiles, err = LoadProfiles(p); err != nil {
		a.Log.Printf("profiles: %v", err)
	}
	a.Library = NewLibrary(a)
	a.Emu = NewEmulator(a)
	a.Compat = NewCompatStore(p.Settings)
	a.Saves = NewSaveOrganizer(a)
	a.Avatars = &AvatarSet{app: a}
	a.Sys = NewSystemFiles(a)
	a.upgradeTheme()
	return a, nil
}

func (a *App) openLog() {
	path := filepath.Join(a.Paths.Logs, "nexus.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 2<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	var w io.Writer = os.Stderr
	if err == nil {
		a.logFile = f
		w = io.MultiWriter(f, os.Stderr)
	}
	a.Log = log.New(w, "", log.LstdFlags|log.Lmicroseconds)
}

func (a *App) Close() {
	if a.logFile != nil {
		_ = a.logFile.Close()
	}
}

// ---------- small helpers shared by subsystems ----------

// writeJSONAtomic writes v as indented JSON via a temp file + rename so a crash
// can never leave a half-written settings or profile file behind.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		_ = copyFile(path, path+".bak")
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		// fall back to the last good backup
		if bak, e2 := os.ReadFile(path + ".bak"); e2 == nil && json.Unmarshal(bak, v) == nil {
			return nil
		}
		return fmt.Errorf("%s is corrupt: %w", filepath.Base(path), err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst + ".part")
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst + ".part")
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(dst+".part", dst)
}

// copyTree copies a directory recursively. It refuses to overwrite any
// existing file in dst (returns errExists) so save data can never be clobbered.
var errExists = errors.New("destination already contains files")

func copyTree(src, dst string, overwrite bool) (int, error) {
	n := 0
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !overwrite {
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("%w: %s", errExists, rel)
			}
		}
		n++
		return copyFile(path, target)
	})
	return n, err
}

func dirSize(root string) (int64, int) {
	var total int64
	files := 0
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
			files++
		}
		return nil
	})
	return total, files
}

func dirHasFiles(root string) bool {
	found := false
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			found = true
			return io.EOF
		}
		return nil
	})
	return found
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func nowUnix() int64 { return time.Now().Unix() }

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// relOrAbs returns p relative to root when it lives inside root (keeps settings
// portable when the NEXUS folder moves), otherwise the absolute path.
func (p *Paths) Portable(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if rel, err := filepath.Rel(p.Root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return abs
}

func (p *Paths) Resolve(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) || (len(path) > 1 && path[1] == ':') {
		return filepath.Clean(path)
	}
	return filepath.Join(p.Root, filepath.FromSlash(path))
}

// upgradeTheme moves users still on the old default accent (cyan) to the NEXUS
// red introduced with the 1.6 home screen. Accents users picked are kept.
func (a *App) upgradeTheme() {
	if a.Settings.Get().ThemeVersion >= 2 {
		return
	}
	for _, p := range a.Profiles.List() {
		if strings.EqualFold(p.Theme.Accent, "#00c8ff") || p.Theme.Accent == "" {
			_, _ = a.Profiles.Update(p.ID, json.RawMessage(`{"theme":{"accent":"`+NexusRed+`"}}`))
		}
	}
	_, _ = a.Settings.Update(func(g *GlobalSettings) { g.ThemeVersion = 2 })
}
