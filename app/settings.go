package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// GlobalSettings are machine-wide (startup happens before a profile is chosen).
// Per-profile preferences (theme, wallpaper, controls) live on the Profile.
type GlobalSettings struct {
	Version       int    `json:"version"`
	LastProfileID string `json:"lastProfileId"`
	AskProfile    bool   `json:"askProfileOnStart"` // show the profile picker at every launch

	Display struct {
		Fullscreen bool `json:"fullscreen"`
		Width      int  `json:"width"`
		Height     int  `json:"height"`
	} `json:"display"`

	// PerfMonitor: the in-game performance overlay (perf.go, overlay_windows.go)
	PerfMonitor PerfMonitorSettings `json:"perfMonitor"`

	Startup struct {
		Enabled bool   `json:"enabled"`
		Color   string `json:"color"`
		Style   string `json:"style"` // standard | short
		Sound   bool   `json:"sound"`
	} `json:"startup"`

	Sound struct {
		Enabled bool    `json:"enabled"`
		Volume  float64 `json:"volume"`
	} `json:"sound"`

	Library struct {
		Dirs           []string `json:"dirs"` // extra directories (the Games folder is always scanned)
		Recursive      bool     `json:"recursive"`
		ScanOnStart    bool     `json:"scanOnStart"`
		ShowFormats    []string `json:"formats"`
		ReadKeysForIDs bool     `json:"readKeysForIds"` // decrypt NCA headers with prod.keys to identify titles
		OnlineArtwork  bool     `json:"onlineArtwork"`  // download missing icons by Title ID
		NlibURL        string   `json:"nlibUrl"`        // Nlib API base URL
		ArtProviders   []string `json:"artProviders"`   // extra icon URL templates containing {tid}
	} `json:"library"`

	Emulator EmulatorSettings `json:"emulator"` // game session behaviour (NEXUS engine)

	ThemeVersion int `json:"themeVersion"` // 2 = 1.6 home screen (red accent)

	// Engine holds global emulation settings as NEXUS values (see enginesettings.go).
	Engine map[string]string `json:"engine"`
}

type EmulatorSettings struct {
	Fullscreen  bool `json:"fullscreen"`  // start games fullscreen
	QuitCombo   bool `json:"quitCombo"`   // hold Back+Start (View+Menu) to quit the game
	ManageSaves bool `json:"manageSaves"` // separate saves for every NEXUS user
	// WhoPlays: when to ask "Who's playing?" before a game starts: "several" (when there is more
	// than one user; default), "always" or "never" (the current user plays)
	WhoPlays string `json:"whoPlays"`
	// NEXUS Home while playing: keyboard key (SDL key name, "" = off) and the controller's
	// Guide/Home button. A key that is one of the game's controls stays with the game.
	HomeKey    string `json:"homeKey"`
	HomeButton bool   `json:"homeButton"`
	// Overrides holds engine keys saved by NEXUS 1.2-1.5; migrated into Engine on load.
	Overrides map[string]string `json:"overrides"`
}

func DefaultSettings() GlobalSettings {
	var s GlobalSettings
	s.Version = 1
	s.Display.Fullscreen = true
	s.Display.Width, s.Display.Height = 1280, 760
	s.PerfMonitor = PerfMonitorSettings{Mode: "off", Position: "top", Opacity: 80, Scale: 100,
		Metrics: []string{"fps", "frametime", "cpu", "gpu", "vram", "ram"}}
	s.Startup.Enabled = true
	s.Startup.Color = "#e60012"
	s.Startup.Style = "standard"
	s.Startup.Sound = true
	s.Sound.Enabled = true
	s.Sound.Volume = 0.7
	s.Library.Recursive = true
	s.Library.ScanOnStart = true
	s.Library.ShowFormats = []string{"nsp", "xci", "nro", "nsz", "xcz", "nup"}
	s.Library.ReadKeysForIDs = true
	s.Library.OnlineArtwork = true
	s.Library.NlibURL = "https://api.nlib.cc"
	s.Library.ArtProviders = []string{}
	s.Engine = map[string]string{}
	s.Emulator = EmulatorSettings{Fullscreen: true, QuitCombo: true, ManageSaves: true, WhoPlays: "several", HomeKey: "H", HomeButton: true, Overrides: map[string]string{}}
	return s
}

// SupportedFormats are the file types the NEXUS engine's loader opens
// (engine src/core/loader/loader.cpp GuessFromFilename).
var SupportedFormats = map[string]string{
	"nsp": "Nintendo Submission Package",
	"xci": "Game card image",
	"nro": "Homebrew executable",
	"nsz": "Compressed NSP",
	"xcz": "Compressed XCI",
	"nup": "NUP package",
	"nca": "Content archive",
	"nso": "Executable",
}

type SettingsStore struct {
	mu   sync.Mutex
	path string
	s    GlobalSettings
}

func LoadSettings(p *Paths) (*SettingsStore, error) {
	st := &SettingsStore{path: filepath.Join(p.Settings, "settings.json"), s: DefaultSettings()}
	if _, err := os.Stat(st.path); errors.Is(err, os.ErrNotExist) {
		return st, st.saveLocked()
	}
	// Unmarshal over defaults so new fields keep their default values.
	if err := readJSON(st.path, &st.s); err != nil {
		st.s = DefaultSettings()
		return st, err
	}
	if st.s.Emulator.Overrides == nil {
		st.s.Emulator.Overrides = map[string]string{}
	}
	if st.s.Startup.Color == "" {
		st.s.Startup.Color = "#e60012"
	}
	if st.s.Engine == nil {
		st.s.Engine = map[string]string{}
	}
	if migrateGlobalSettings(&st.s) {
		_ = st.saveLocked()
	}
	return st, nil
}

func (st *SettingsStore) Get() GlobalSettings {
	st.mu.Lock()
	defer st.mu.Unlock()
	// deep copy via JSON so callers can't mutate shared maps/slices
	var out GlobalSettings
	b, _ := json.Marshal(st.s)
	_ = json.Unmarshal(b, &out)
	return out
}

func (st *SettingsStore) Update(fn func(s *GlobalSettings)) (GlobalSettings, error) {
	st.mu.Lock()
	fn(&st.s)
	err := st.saveLocked()
	st.mu.Unlock()
	return st.Get(), err
}

// Patch merges a partial JSON document into the settings.
func (st *SettingsStore) Patch(raw json.RawMessage) (GlobalSettings, error) {
	st.mu.Lock()
	next := st.s
	b, _ := json.Marshal(st.s)
	_ = json.Unmarshal(b, &next) // copy
	if err := mergeJSON(&next, raw); err != nil {
		st.mu.Unlock()
		return st.Get(), err
	}
	st.s = next
	err := st.saveLocked()
	st.mu.Unlock()
	return st.Get(), err
}

func (st *SettingsStore) saveLocked() error { return writeJSONAtomic(st.path, st.s) }

// mergeJSON applies a partial JSON object onto v (deep merge for objects).
func mergeJSON(v any, patch json.RawMessage) error {
	cur, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var base, p map[string]any
	if err := json.Unmarshal(cur, &base); err != nil {
		return err
	}
	if err := json.Unmarshal(patch, &p); err != nil {
		return err
	}
	deepMerge(base, p)
	merged, _ := json.Marshal(base)
	return json.Unmarshal(merged, v)
}

func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok && k != "overrides" && k != "mapping" {
				deepMerge(dm, sm)
				continue
			}
		}
		dst[k] = sv
	}
}

type PerfMonitorSettings struct {
	Mode     string   `json:"mode"`     // off | all (every game) | selected (the games in Games)
	Games    []string `json:"games"`    // game keys for "selected"
	Position string   `json:"position"` // top | bottom
	Metrics  []string `json:"metrics"`  // ids from PerfMetrics, in display order
	Opacity  int      `json:"opacity"`  // 30-100 %
	Scale    int      `json:"scale"`    // 75-200 %
}

// PerfMonitorFor: whether the overlay shows for this game.
func (p PerfMonitorSettings) For(gameKey string) bool {
	switch p.Mode {
	case "all":
		return true
	case "selected":
		for _, g := range p.Games {
			if g == gameKey {
				return true
			}
		}
	}
	return false
}
