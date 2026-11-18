package main

// Steam integration on Linux (Steam Deck and desktop Steam), each step only when the user asks:
//
//   - App menu entry: a desktop file and NEXUS's icon in the user's own folders
//     (~/.local/share/applications, ~/.local/share/icons). Steam's "Add a Non-Steam Game" and
//     the Steam Deck's Desktop Mode "Add to Steam" then show NEXUS by name with its icon,
//     launching the AppImage (or the tar.gz's nexus) from where it is.
//   - Steam library artwork: Steam has no way for a program to set the artwork of a non-Steam
//     game it was added as. NEXUS finds its own shortcut in Steam's shortcuts.vdf (read only,
//     never changed) and copies its artwork (Resources/Steam) into Steam's grid folder under that
//     shortcut's id, the same place Steam's "Set custom artwork" writes. Images the user already
//     set there are never replaced. Steam shows them after it restarts.
//
// Nothing is written to system folders, and Steam's own files (shortcuts.vdf, config) are only read.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SteamStatus is Settings › General › Steam.
type SteamStatus struct {
	Supported    bool            `json:"supported"`
	Launcher     string          `json:"launcher"`   // what Steam should start (AppImage or nexus)
	MenuEntry    bool            `json:"menuEntry"`  // the app menu entry exists and points here
	MenuStale    bool            `json:"menuStale"`  // it exists but starts another copy
	SteamFound   bool            `json:"steamFound"` // Steam's user data was found
	Shortcuts    []steamShort    `json:"shortcuts"`  // NEXUS shortcuts in Steam
	ArtInstalled map[string]bool `json:"artInstalled,omitempty"`
	// Offered: what NEXUS already offered once at start ("menu", "art"), so it never asks twice
	Offered  map[string]bool `json:"offered"`
	IconPath string          `json:"iconPath"` // the icon file to pick in Steam › Properties
}

func (a *App) steamOfferPath() string { return filepath.Join(a.Paths.System, "steam-offers.json") }

// SteamOffered records that NEXUS offered (and the user answered) one of the Steam steps.
func (a *App) SteamOffered(what string) {
	m := map[string]bool{}
	_ = readJSON(a.steamOfferPath(), &m)
	m[what] = true
	_ = writeJSONAtomic(a.steamOfferPath(), m)
}

type steamShort struct {
	User    string `json:"user"`
	AppID   uint32 `json:"appId"`
	Name    string `json:"name"`
	Exe     string `json:"exe"`
	ArtDone bool   `json:"artDone"` // NEXUS's artwork (or the user's own) is in place
}

func (a *App) steamLauncher() string {
	if p := os.Getenv("APPIMAGE"); p != "" {
		return p
	}
	exe, _ := os.Executable()
	return exe
}

func xdgDataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func desktopEntryPath() string { return filepath.Join(xdgDataHome(), "applications", "nexus.desktop") }

// InstallMenuEntry writes the app menu entry and icon (user folders only).
func (a *App) InstallMenuEntry() error {
	if runtime.GOOS != "linux" {
		return errors.New("The app menu entry is for Linux.")
	}
	launcher := a.steamLauncher()
	iconDir := filepath.Join(xdgDataHome(), "icons", "hicolor", "512x512", "apps")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return err
	}
	if err := copyFileOverwrite(filepath.Join(a.Paths.Root, "Resources", "Icons", "app-icon-512.png"), filepath.Join(iconDir, "nexus.png")); err != nil {
		return fmt.Errorf("the icon could not be copied: %w", err)
	}
	entry := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=NEXUS",
		"GenericName=Game launcher",
		"Comment=Your games, users and saves in one place",
		"Exec=" + desktopQuote(launcher),
		"Path=" + filepath.Dir(launcher),
		"Icon=nexus",
		"Terminal=false",
		"Categories=Game;Emulator;",
		"StartupWMClass=nexus",
		"X-NEXUS-Managed=true",
		"",
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(desktopEntryPath()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(desktopEntryPath(), []byte(entry), 0o644); err != nil {
		return err
	}
	a.Log.Printf("steam: app menu entry for %s", launcher)
	return nil
}

func desktopQuote(p string) string {
	if !strings.ContainsAny(p, " \t\"'\\$`") {
		return p
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`)
	return `"` + r.Replace(p) + `"`
}

// steamUserdata: Steam's per-account folders on this machine.
func steamUserdata() []string {
	home, _ := os.UserHomeDir()
	var out []string
	seen := map[string]bool{}
	for _, root := range []string{filepath.Join(home, ".steam", "steam"), filepath.Join(home, ".local", "share", "Steam"), filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam")} {
		real, err := filepath.EvalSymlinks(root)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		users, _ := filepath.Glob(filepath.Join(real, "userdata", "[0-9]*"))
		out = append(out, users...)
	}
	return out
}

// SteamStatus reports the menu entry and NEXUS's shortcuts in Steam.
func (a *App) SteamStatus() SteamStatus {
	st := SteamStatus{Supported: runtime.GOOS == "linux", Launcher: a.steamLauncher(), Shortcuts: []steamShort{}, Offered: map[string]bool{}}
	_ = readJSON(a.steamOfferPath(), &st.Offered)
	if ic := filepath.Join(xdgDataHome(), "icons", "hicolor", "512x512", "apps", "nexus.png"); fileExists(ic) {
		st.IconPath = ic
	} else {
		st.IconPath = filepath.Join(a.Paths.Root, "Resources", "Icons", "app-icon-512.png")
	}
	if !st.Supported {
		return st
	}
	if b, err := os.ReadFile(desktopEntryPath()); err == nil {
		exe := ""
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "Exec="); ok {
				exe = strings.Trim(v, `"`)
			}
		}
		st.MenuEntry = exe == st.Launcher
		st.MenuStale = !st.MenuEntry
	}
	for _, u := range steamUserdata() {
		st.SteamFound = true
		data, err := os.ReadFile(filepath.Join(u, "config", "shortcuts.vdf"))
		if err != nil {
			continue
		}
		for _, sc := range parseShortcuts(data) {
			if !isNexusShortcut(sc, st.Launcher) {
				continue
			}
			s := steamShort{User: filepath.Base(u), AppID: sc.appID, Name: sc.name, Exe: sc.exe}
			s.ArtDone = fileExists(filepath.Join(u, "config", "grid", fmt.Sprintf("%dp.png", sc.appID))) || fileExists(filepath.Join(u, "config", "grid", fmt.Sprintf("%dp.jpg", sc.appID)))
			st.Shortcuts = append(st.Shortcuts, s)
		}
	}
	return st
}

func isNexusShortcut(sc vdfShortcut, launcher string) bool {
	exe := strings.Trim(sc.exe, `"`)
	base := strings.ToLower(filepath.Base(exe))
	return exe == launcher || strings.EqualFold(strings.TrimSpace(sc.name), "NEXUS") ||
		strings.HasPrefix(base, "nexus-") && strings.HasSuffix(base, ".appimage") || base == "nexus" && strings.Contains(strings.ToLower(exe), "nexus")
}

// InstallSteamArtwork copies NEXUS's artwork for each of its Steam shortcuts. Existing images
// (the user's own choice) are kept. Returns how many shortcuts got artwork.
func (a *App) InstallSteamArtwork() (int, error) {
	st := a.SteamStatus()
	if len(st.Shortcuts) == 0 {
		return 0, errors.New("NEXUS is not in your Steam library yet. Add it first (Steam › Games › Add a Non-Steam Game, or right-click the AppImage › Add to Steam in Desktop Mode), then try again.")
	}
	src := filepath.Join(a.Paths.Root, "Resources", "Steam")
	files := map[string]string{"capsule.png": "%dp.png", "wide.png": "%d.png", "hero.png": "%d_hero.png", "logo.png": "%d_logo.png", "icon.png": "%d_icon.png"}
	n := 0
	for _, sc := range st.Shortcuts {
		var grid string
		for _, u := range steamUserdata() {
			if filepath.Base(u) == sc.User {
				grid = filepath.Join(u, "config", "grid")
			}
		}
		if grid == "" {
			continue
		}
		if err := os.MkdirAll(grid, 0o755); err != nil {
			return n, err
		}
		wrote := false
		for from, pattern := range files {
			dst := filepath.Join(grid, fmt.Sprintf(pattern, sc.AppID))
			if fileExists(dst) || fileExists(strings.TrimSuffix(dst, ".png")+".jpg") {
				continue // the user's own image
			}
			if err := copyFileOverwrite(filepath.Join(src, from), dst); err != nil {
				return n, err
			}
			wrote = true
		}
		if wrote {
			n++
			a.Log.Printf("steam: artwork for shortcut %d (Steam user %s)", sc.AppID, sc.User)
		}
	}
	return n, nil
}

// ---------------------------------------------------------------- shortcuts.vdf (binary VDF)

type vdfShortcut struct {
	appID     uint32
	name, exe string
}

// parseShortcuts reads Steam's binary shortcuts.vdf: a map "shortcuts" of maps "0", "1", ...
// with appid (int32), AppName and Exe (strings). Malformed data gives what was read so far.
func parseShortcuts(b []byte) []vdfShortcut {
	var out []vdfShortcut
	r := bytes.NewReader(b)
	readStr := func() (string, bool) {
		var sb strings.Builder
		for {
			c, err := r.ReadByte()
			if err != nil {
				return "", false
			}
			if c == 0 {
				return sb.String(), true
			}
			sb.WriteByte(c)
		}
	}
	var walk func(depth int, cur *vdfShortcut) bool
	walk = func(depth int, cur *vdfShortcut) bool {
		for {
			t, err := r.ReadByte()
			if err != nil {
				return false
			}
			if t == 0x08 {
				return true
			}
			key, ok := readStr()
			if !ok {
				return false
			}
			switch t {
			case 0x00:
				var sc vdfShortcut
				if !walk(depth+1, &sc) {
					return false
				}
				if depth == 1 && (sc.appID != 0 || sc.exe != "") {
					out = append(out, sc)
				}
			case 0x01:
				v, ok := readStr()
				if !ok {
					return false
				}
				if cur != nil {
					switch strings.ToLower(key) {
					case "appname":
						cur.name = v
					case "exe":
						cur.exe = v
					}
				}
			case 0x02:
				var v uint32
				if binary.Read(r, binary.LittleEndian, &v) != nil {
					return false
				}
				if cur != nil && strings.EqualFold(key, "appid") {
					cur.appID = v
				}
			case 0x07:
				var v uint64
				if binary.Read(r, binary.LittleEndian, &v) != nil {
					return false
				}
			default:
				return false
			}
		}
	}
	walk(0, nil)
	return out
}
