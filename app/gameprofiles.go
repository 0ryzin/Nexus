package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// GameProfile is everything NEXUS stores about one game, kept apart from the
// game file itself in System/GameProfiles/<TITLE_ID>/settings.json.
// Regional releases have different Title IDs and therefore separate profiles;
// updates share the base game's Title ID (as in Lichii's own per-game configs).
type GameProfile struct {
	TitleID       string `json:"titleId,omitempty"`
	LastPath      string `json:"lastPath,omitempty"`
	Title         string `json:"title,omitempty"`         // user's display-name override
	ArtPreference string `json:"artPreference,omitempty"` // auto | custom | embedded | online
	ArtFit        string `json:"artFit,omitempty"`        // cover | contain
	CustomIcon    string `json:"customIcon,omitempty"`    // file name inside the profile folder
	// the Cards home layout's background and title pictures (custom-background.*, custom-logo.*)
	CustomBackground string            `json:"customBackground,omitempty"`
	CustomLogo       string            `json:"customLogo,omitempty"`
	Hidden           bool              `json:"hidden,omitempty"`       // removed from library (file untouched)
	Emu              map[string]string `json:"emu,omitempty"`          // per-game Lichii overrides (option id -> value)
	InputProfile     string            `json:"inputProfile,omitempty"` // per-game Lichii input profile for player 1
	EmuApplied       []string          `json:"emuApplied,omitempty"`   // keys NEXUS has written to Lichii's custom config
	Online           OnlineArt         `json:"online"`
	Emulator         string            `json:"emulator,omitempty"` // "" = default | lichii | ryujinx
	Ryu              map[string]string `json:"ryujinx,omitempty"`  // 1.5 per-game Ryujinx overrides (migrated into Settings)
	Settings         map[string]string `json:"settings,omitempty"` // per-game NEXUS settings (enginesettings.go)
	// Controls: this game's own controller layout (nil = the user's layout, plus the game's
	// built-in preset if it has one).
	Controls *GameLayout `json:"controls,omitempty"`
}

type OnlineArt struct {
	CheckedAt int64  `json:"checkedAt,omitempty"`
	Name      string `json:"name,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	Released  string `json:"released,omitempty"`
	Source    string `json:"source,omitempty"`
	File      string `json:"file,omitempty"`
	Error     string `json:"error,omitempty"`
}

type GameProfiles struct {
	mu    sync.Mutex
	root  string
	cache map[string]*GameProfile
}

func NewGameProfiles(root string) *GameProfiles {
	_ = os.MkdirAll(root, 0o755)
	return &GameProfiles{root: root, cache: map[string]*GameProfile{}}
}

// FolderFor maps a library key to a stable folder name.
func FolderFor(key string) string {
	switch {
	case strings.HasPrefix(key, "t-"), strings.HasPrefix(key, "x-"):
		return strings.ToUpper(key[2:])
	case strings.HasPrefix(key, "f-"):
		return "file-" + key[2:]
	}
	return safeName(key)
}

func (g *GameProfiles) Dir(key string) string { return filepath.Join(g.root, FolderFor(key)) }

func (g *GameProfiles) loadLocked(key string) *GameProfile {
	if p, ok := g.cache[key]; ok {
		return p
	}
	p := &GameProfile{}
	_ = readJSON(filepath.Join(g.Dir(key), "settings.json"), p)
	g.cache[key] = p
	return p
}

func (g *GameProfiles) Get(key string) GameProfile {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := *g.loadLocked(key)
	if p.Emu != nil {
		m := map[string]string{}
		for k, v := range p.Emu {
			m[k] = v
		}
		p.Emu = m
	}
	p.EmuApplied = append([]string(nil), p.EmuApplied...)
	cp := func(m map[string]string) map[string]string {
		if m == nil {
			return nil
		}
		o := make(map[string]string, len(m))
		for k, v := range m {
			o[k] = v
		}
		return o
	}
	p.Ryu, p.Settings = cp(p.Ryu), cp(p.Settings)
	return p
}

func (g *GameProfiles) Update(key string, fn func(p *GameProfile)) (GameProfile, error) {
	if key == "" {
		return GameProfile{}, errors.New("no game key")
	}
	g.mu.Lock()
	p := g.loadLocked(key)
	fn(p)
	err := writeJSONAtomic(filepath.Join(g.Dir(key), "settings.json"), p)
	g.mu.Unlock()
	return g.Get(key), err
}

// File returns an absolute path inside the game's profile folder.
func (g *GameProfiles) File(key, name string) string {
	return filepath.Join(g.Dir(key), filepath.Base(name))
}
