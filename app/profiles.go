package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Wallpaper struct {
	File       string  `json:"file"`       // file name inside System/Wallpapers ("" = built-in background)
	Brightness float64 `json:"brightness"` // 0.1 .. 1
	Blur       float64 `json:"blur"`       // px
	Fit        string  `json:"fit"`        // cover | contain | stretch | center | tile
	// Animated wallpaper (optional): a GIF, animated WebP, MP4 or WebM in System/Wallpapers,
	// plus a still frame (Poster) shown whenever the animation is paused or cannot play.
	Animated     string `json:"animated,omitempty"`
	AnimatedType string `json:"animatedType,omitempty"` // video/mp4 | video/webm | image/gif | image/webp
	Poster       string `json:"poster,omitempty"`
	Use          string `json:"use"`    // static | animated (which one the home screen shows)
	Motion       string `json:"motion"` // on | off: play animated wallpapers (off = always the still frame)
}

type Theme struct {
	Mode       string `json:"mode"`       // dark | light
	Accent     string `json:"accent"`     // hex
	Animations string `json:"animations"` // full | reduced | off
	// HomeTransition: how NEXUS animates into the Home screen (zoom | fade | slide | off).
	HomeTransition string    `json:"homeTransition"`
	Wallpaper      Wallpaper `json:"wallpaper"`
}

type Controls struct {
	ConfirmButton string         `json:"confirmButton"` // south | east  (east = Nintendo-style A on the right)
	Mapping       map[string]int `json:"mapping"`       // action -> standard gamepad button index
	Vibration     bool           `json:"vibration"`
	PreferredPad  string         `json:"preferredPad"`               // the menu controller's key (web/pads.js; older: gamepad id), "" = any
	PreferredName string         `json:"preferredPadName,omitempty"` // its name, shown while it is not connected
	EmuInput      string         `json:"emuInputProfile"`            // Lichii input profile name passed with -i
	EmuUser       int            `json:"emuUserIndex"`               // -1 = leave Lichii's current user
	KeyRepeatMs   int            `json:"keyRepeatMs"`
	// GameLayout: this user's controller layout in games (see controls.go).
	GameLayout GameLayout `json:"gameLayout"`
}

type Profile struct {
	ID        string   `json:"id"`
	PlayerID  string   `json:"playerId,omitempty"` // 32-hex player identifier: names the save folder, never changes
	Name      string   `json:"name"`
	Avatar    Avatar   `json:"avatar"`
	CreatedAt int64    `json:"createdAt"`
	Theme     Theme    `json:"theme"`
	Controls  Controls `json:"controls"`
	Sort      string   `json:"sort"` // recent | alpha | playtime | added | custom
	// Home: this user's home screen (layouts, organisation, widgets; docs/HOME.md)
	Home HomePrefs `json:"home"`
}

// HomePrefs is one user's home screen. Every list holds game keys; a key of a game that is gone
// is ignored where it is used.
type HomePrefs struct {
	Layout     string          `json:"layout"`               // classic | cards | grid
	Order      []string        `json:"order,omitempty"`      // the custom order (sort "custom")
	Favorites  []string        `json:"favorites,omitempty"`  // favourite games
	Hidden     []string        `json:"hidden,omitempty"`     // not on Home (still in All Software)
	Folders    []HomeGroup     `json:"folders,omitempty"`    // folders on Home (a game in one is shown in it)
	Pages      []HomeGroup     `json:"pages,omitempty"`      // custom pages (a game can be on several)
	Dock       []string        `json:"dock,omitempty"`       // order of the shortcut buttons (dock / sidebar)
	DockHidden []string        `json:"dockHidden,omitempty"` // shortcut buttons not shown
	Widgets    map[string]bool `json:"widgets,omitempty"`    // recent | playtime | favorites | added | storage | details
	Effects    string          `json:"effects"`              // glass | flat (glass: frosted panels; flat for the lowest GPU use)
	GameBg     bool            `json:"gameBackground"`       // Cards: the selected game's picture behind
	BgColor    string          `json:"bgColor,omitempty"`    // a plain colour instead of the wallpaper
	Count      int             `json:"count"`                // games on the classic Home row
}

// HomeGroup is a folder or a custom page.
type HomeGroup struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Games []string `json:"games"`
}

type Avatar struct {
	Kind   string `json:"kind"`   // preset | image
	Preset string `json:"preset"` // glyph id
	Color  string `json:"color"`
	File   string `json:"file"` // avatar.<ext> inside the profile folder
	Rev    int64  `json:"rev"`
	// how a composed icon was built (so it can be re-edited): library paths
	Char  string `json:"char,omitempty"`
	Bg    string `json:"bg,omitempty"`
	Frame string `json:"frame,omitempty"`
}

type GameStat struct {
	Seconds    int64            `json:"seconds"`
	LastPlayed int64            `json:"lastPlayed"`
	Launches   int              `json:"launches"`
	Sessions   int              `json:"sessions"` // launches that actually reached gameplay
	Title      string           `json:"title"`
	ByEmulator map[string]int64 `json:"byEmulator,omitempty"`
	FirstPlay  int64            `json:"firstPlayed,omitempty"`
	Last       *SessionRec      `json:"lastSession,omitempty"`
	History    []SessionRec     `json:"history,omitempty"`  // most recent first, capped
	Adjusted   int64            `json:"adjusted,omitempty"` // seconds added (or removed) by manual corrections
}

// SessionRec is one play session.
type SessionRec struct {
	Start     int64  `json:"start"`
	End       int64  `json:"end"`
	Seconds   int64  `json:"seconds"`
	Emulator  string `json:"emulator"`
	Recovered bool   `json:"recovered,omitempty"` // restored after a crash of NEXUS/the PC
	Crashed   bool   `json:"crashed,omitempty"`   // the emulator exited with an error
}

type ProfileStats struct {
	Games map[string]*GameStat `json:"games"`
}

func DefaultControls() Controls {
	return Controls{
		ConfirmButton: "south",
		Mapping: map[string]int{ // W3C "standard" gamepad layout indices
			"confirm": 0, "back": 1, "options": 3, "search": 2,
			"prevTab": 4, "nextTab": 5, "menu": 9, "home": 16, "view": 8,
		},
		Vibration:   true,
		EmuUser:     -1,
		KeyRepeatMs: 140,
	}
}

// NexusRed is the NEXUS accent colour (a brighter tint of the #e60012 brand red
// so text and focus rings stay readable on the dark theme).
const NexusRed = "#ff2a3a"

func DefaultTheme() Theme {
	return Theme{Mode: "dark", Accent: NexusRed, Animations: "full",
		Wallpaper: Wallpaper{Brightness: 0.55, Blur: 0, Fit: "cover"}}
}

var avatarColors = []string{"#ff3b30", "#ff9500", "#ffcc00", "#34c759", "#00c7be", "#0a84ff", "#5e5ce6", "#bf5af2", "#ff2d55"}
var avatarGlyphs = []string{"grip-l", "grip-r", "star", "bolt", "leaf", "flame", "planet", "heart", "crown", "ghost", "note", "rocket"}

type ProfileStore struct {
	mu       sync.Mutex
	p        *Paths
	path     string
	Profiles []*Profile `json:"profiles"`
	stats    map[string]*ProfileStats
}

func LoadProfiles(p *Paths) (*ProfileStore, error) {
	st := &ProfileStore{p: p, path: filepath.Join(p.Profiles, "profiles.json"), stats: map[string]*ProfileStats{}}
	var doc struct {
		Profiles []*Profile `json:"profiles"`
	}
	if err := readJSON(st.path, &doc); err != nil && !errors.Is(err, os.ErrNotExist) {
		return st, err
	}
	st.Profiles = doc.Profiles
	for _, pr := range st.Profiles {
		st.normalise(pr)
	}
	return st, nil
}

func (st *ProfileStore) normalise(pr *Profile) {
	dt, dc := DefaultTheme(), DefaultControls()
	if pr.Theme.Mode == "" {
		pr.Theme.Mode = dt.Mode
	}
	if pr.Theme.Accent == "" {
		pr.Theme.Accent = dt.Accent
	}
	if pr.Theme.Animations == "" {
		pr.Theme.Animations = dt.Animations
	}
	if pr.Theme.Wallpaper.Fit == "" {
		pr.Theme.Wallpaper.Fit = "cover"
	}
	if pr.Theme.Wallpaper.Brightness == 0 {
		pr.Theme.Wallpaper.Brightness = dt.Wallpaper.Brightness
	}
	if pr.Theme.Wallpaper.Use != "animated" || pr.Theme.Wallpaper.Animated == "" {
		pr.Theme.Wallpaper.Use = "static"
	}
	if pr.Theme.Wallpaper.Motion != "off" {
		pr.Theme.Wallpaper.Motion = "on"
	}
	if pr.Theme.HomeTransition == "" {
		pr.Theme.HomeTransition = "zoom"
	}
	normaliseHome(&pr.Home)
	if pr.Controls.Mapping == nil {
		pr.Controls.Mapping = dc.Mapping
	}
	for k, v := range dc.Mapping {
		if _, ok := pr.Controls.Mapping[k]; !ok {
			pr.Controls.Mapping[k] = v
		}
	}
	if pr.Controls.ConfirmButton == "" {
		pr.Controls.ConfirmButton = dc.ConfirmButton
	}
	if pr.Controls.KeyRepeatMs == 0 {
		pr.Controls.KeyRepeatMs = dc.KeyRepeatMs
	}
	if pr.Sort == "" {
		pr.Sort = "recent"
	}
}

func (st *ProfileStore) saveLocked() error {
	return writeJSONAtomic(st.path, map[string]any{"profiles": st.Profiles})
}

func (st *ProfileStore) List() []Profile {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]Profile, 0, len(st.Profiles))
	for _, p := range st.Profiles {
		out = append(out, *p)
	}
	return out
}

func (st *ProfileStore) Get(id string) (Profile, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, p := range st.Profiles {
		if p.ID == id {
			return *p, true
		}
	}
	return Profile{}, false
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("Please enter a name.")
	}
	if utf8.RuneCountInString(name) > 20 {
		return "", errors.New("Names can be up to 20 characters.")
	}
	return name, nil
}

// MaxProfiles is the number of users NEXUS allows (like a console). Existing data with more
// users from older versions is kept; only creating new ones is refused.
const MaxProfiles = 8

func (st *ProfileStore) Create(name string, av Avatar) (Profile, error) {
	name, err := validName(name)
	if err != nil {
		return Profile{}, err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, p := range st.Profiles {
		if strings.EqualFold(p.Name, name) {
			return Profile{}, fmt.Errorf("A profile called %q already exists.", name)
		}
	}
	if len(st.Profiles) >= MaxProfiles {
		return Profile{}, fmt.Errorf("NEXUS supports up to %d users. Delete a user to make room for a new one.", MaxProfiles)
	}
	n := len(st.Profiles)
	if av.Kind == "" {
		av = Avatar{Kind: "preset", Preset: avatarGlyphs[n%len(avatarGlyphs)], Color: avatarColors[n%len(avatarColors)]}
	}
	pr := &Profile{ID: newID(), PlayerID: newPlayerID(), Name: name, Avatar: av, CreatedAt: nowUnix(),
		Theme: DefaultTheme(), Controls: DefaultControls(), Sort: "recent"}
	st.normalise(pr)
	if err := os.MkdirAll(st.profileDir(pr.ID), 0o755); err != nil {
		return Profile{}, err
	}
	if err := os.MkdirAll(filepath.Join(st.p.Saves, playerFolderName(pr.PlayerID, pr.Name)), 0o755); err != nil {
		return Profile{}, err
	}
	st.Profiles = append(st.Profiles, pr)
	return *pr, st.saveLocked()
}

// Update applies a partial JSON patch to the profile.
func (st *ProfileStore) Update(id string, patch json.RawMessage) (Profile, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, p := range st.Profiles {
		if p.ID != id {
			continue
		}
		next := *p
		if err := mergeJSON(&next, patch); err != nil {
			return *p, err
		}
		name, err := validName(next.Name)
		if err != nil {
			return *p, err
		}
		for _, o := range st.Profiles {
			if o.ID != id && strings.EqualFold(o.Name, name) {
				return *p, fmt.Errorf("A profile called %q already exists.", name)
			}
		}
		next.Name = name
		next.ID = p.ID
		next.PlayerID = p.PlayerID
		next.CreatedAt = p.CreatedAt
		st.normalise(&next)
		*p = next
		return *p, st.saveLocked()
	}
	return Profile{}, errors.New("Profile not found.")
}

// Delete removes the profile. Its save data is NEVER deleted here: the profile
// folder and its saves are moved to System/Archived so they can be restored.
func (st *ProfileStore) Delete(id string) (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	idx := -1
	for i, p := range st.Profiles {
		if p.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return "", errors.New("Profile not found.")
	}
	pr := st.Profiles[idx]
	stamp := time.Now().Format("20060102-150405")
	archive := filepath.Join(st.p.Trash, fmt.Sprintf("%s-%s-%s", safeName(pr.Name), pr.ID, stamp))
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return "", err
	}
	if sd := st.SaveDir(id); dirExists(sd) {
		if err := os.Rename(sd, filepath.Join(archive, "Saves")); err != nil {
			return "", fmt.Errorf("could not archive saves: %w", err)
		}
	}
	if dirExists(st.profileDir(id)) {
		if err := os.Rename(st.profileDir(id), filepath.Join(archive, "Profile")); err != nil {
			return "", fmt.Errorf("could not archive profile: %w", err)
		}
	}
	_ = writeJSONAtomic(filepath.Join(archive, "profile.json"), pr)
	st.Profiles = append(st.Profiles[:idx], st.Profiles[idx+1:]...)
	delete(st.stats, id)
	return archive, st.saveLocked()
}

func (st *ProfileStore) profileDir(id string) string { return filepath.Join(st.p.Profiles, id) }

// SaveDir is the user's save folder, "<PLAYER_ID>-<name>" in UserData\Saves (see saves.go). It
// is found by the player ID, so a renamed folder name is still found.
func (st *ProfileStore) SaveDir(id string) string {
	p, ok := st.Get(id)
	if !ok {
		return filepath.Join(st.p.Saves, "_unknown-"+safeFolderName(id))
	}
	if p.PlayerID == "" { // not migrated yet (Migrate assigns one)
		return filepath.Join(st.p.Saves, safeFolderName(id))
	}
	entries, _ := os.ReadDir(st.p.Saves)
	for _, e := range entries {
		if m := rePlayerDir.FindStringSubmatch(e.Name()); e.IsDir() && m != nil && strings.EqualFold(m[1], p.PlayerID) {
			return filepath.Join(st.p.Saves, e.Name())
		}
	}
	return filepath.Join(st.p.Saves, playerFolderName(p.PlayerID, p.Name))
}

// SetPlayerID gives a user their player ID (once; used by the save migration).
func (st *ProfileStore) SetPlayerID(id, playerID string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, p := range st.Profiles {
		if p.ID == id {
			if p.PlayerID != "" {
				return nil
			}
			for _, o := range st.Profiles {
				if strings.EqualFold(o.PlayerID, playerID) {
					playerID = newPlayerID() // two users never share a player ID
				}
			}
			p.PlayerID = strings.ToUpper(playerID)
			return st.saveLocked()
		}
	}
	return errors.New("Profile not found.")
}

func (st *ProfileStore) AvatarPath(id string) string {
	pr, ok := st.Get(id)
	if !ok || pr.Avatar.Kind != "image" || pr.Avatar.File == "" {
		return ""
	}
	return filepath.Join(st.profileDir(id), pr.Avatar.File)
}

func (st *ProfileStore) SetAvatarImage(id string, data []byte, ext string) (Profile, error) {
	if _, ok := st.Get(id); !ok {
		return Profile{}, errors.New("Profile not found.")
	}
	name := "avatar" + ext
	dir := st.profileDir(id)
	_ = os.MkdirAll(dir, 0o755)
	// remove older avatar images with other extensions
	for _, e := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif"} {
		if e != ext {
			_ = os.Remove(filepath.Join(dir, "avatar"+e))
		}
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return Profile{}, err
	}
	patch, _ := json.Marshal(map[string]any{"avatar": map[string]any{"kind": "image", "file": name, "rev": time.Now().UnixMilli()}})
	return st.Update(id, patch)
}

// ---------- per-profile play statistics ----------

func (st *ProfileStore) statsPath(id string) string {
	return filepath.Join(st.profileDir(id), "stats.json")
}

// Stats is the user's play statistics: this device's own, plus — for a user linked to other
// devices (experimental Device Linking) — the play time those devices reported, added up.
func (st *ProfileStore) Stats(id string) ProfileStats {
	out := st.LocalStats(id)
	entries, _ := os.ReadDir(filepath.Join(st.profileDir(id), "stats-devices"))
	for _, e := range entries {
		var snap struct {
			Stats ProfileStats `json:"stats"`
		}
		if readJSON(filepath.Join(st.profileDir(id), "stats-devices", e.Name()), &snap) != nil {
			continue
		}
		for k, g := range snap.Stats.Games {
			if g == nil {
				continue
			}
			cur := out.Games[k]
			if cur == nil {
				c := *g
				c.History = append([]SessionRec{}, g.History...)
				out.Games[k] = &c
				continue
			}
			cur.Seconds += g.Seconds
			cur.Sessions += g.Sessions
			cur.Launches += g.Launches
			if cur.Title == "" {
				cur.Title = g.Title
			}
			if g.LastPlayed > cur.LastPlayed {
				cur.LastPlayed, cur.Last = g.LastPlayed, g.Last
			}
			if g.FirstPlay > 0 && (cur.FirstPlay == 0 || g.FirstPlay < cur.FirstPlay) {
				cur.FirstPlay = g.FirstPlay
			}
			if len(g.ByEmulator) > 0 {
				m := map[string]int64{}
				for e, v := range cur.ByEmulator {
					m[e] = v
				}
				for e, v := range g.ByEmulator {
					m[e] += v
				}
				cur.ByEmulator = m
			}
			cur.History = append(append([]SessionRec{}, cur.History...), g.History...)
			sort.Slice(cur.History, func(i, j int) bool { return cur.History[i].Start > cur.History[j].Start })
			if len(cur.History) > 25 {
				cur.History = cur.History[:25]
			}
		}
	}
	return out
}

// LocalStats is the play time recorded on this device only.
func (st *ProfileStore) LocalStats(id string) ProfileStats {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.loadStatsLocked(id)
	out := ProfileStats{Games: map[string]*GameStat{}}
	for k, v := range s.Games {
		c := *v
		c.History = append([]SessionRec{}, v.History...)
		out.Games[k] = &c
	}
	return out
}

// dropStatsCache forgets cached statistics (after other devices' play time arrived).
func (st *ProfileStore) dropStatsCache(id string) {
	st.mu.Lock()
	delete(st.stats, id)
	st.mu.Unlock()
}

func (st *ProfileStore) loadStatsLocked(id string) *ProfileStats {
	if s, ok := st.stats[id]; ok {
		return s
	}
	s := &ProfileStats{Games: map[string]*GameStat{}}
	_ = readJSON(st.statsPath(id), s)
	if s.Games == nil {
		s.Games = map[string]*GameStat{}
	}
	st.stats[id] = s
	return s
}

func (st *ProfileStore) RecordLaunch(id, key, title string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.loadStatsLocked(id)
	g := s.Games[key]
	if g == nil {
		g = &GameStat{}
		s.Games[key] = g
	}
	g.Launches++
	g.LastPlayed = nowUnix()
	g.Title = title
	_ = writeJSONAtomic(st.statsPath(id), s)
}

// AddSession records a finished play session (only time spent in-game counts).
func (st *ProfileStore) AddSession(id, key, title string, rec SessionRec) {
	if rec.Seconds <= 0 {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.loadStatsLocked(id)
	g := s.Games[key]
	if g == nil {
		g = &GameStat{}
		s.Games[key] = g
	}
	if title != "" {
		g.Title = title
	}
	g.Seconds += rec.Seconds
	g.Sessions++
	if g.ByEmulator == nil {
		g.ByEmulator = map[string]int64{}
	}
	g.ByEmulator[rec.Emulator] += rec.Seconds
	if g.FirstPlay == 0 || rec.Start < g.FirstPlay {
		g.FirstPlay = rec.Start
	}
	if rec.End > g.LastPlayed {
		g.LastPlayed = rec.End
	}
	r := rec
	g.Last = &r
	g.History = append([]SessionRec{rec}, g.History...)
	if len(g.History) > 25 {
		g.History = g.History[:25]
	}
	_ = writeJSONAtomic(st.statsPath(id), s)
}

// AddPlaytime is kept for older callers (counts as one session).
func (st *ProfileStore) AddPlaytime(id, key string, secs int64) {
	now := nowUnix()
	st.AddSession(id, key, "", SessionRec{Start: now - secs, End: now, Seconds: secs, Emulator: "lichii"})
}

// live session checkpoints let NEXUS recover play time after a crash/power loss.
type LiveSession struct {
	ProfileID string `json:"profileId"`
	Key       string `json:"key"`
	Title     string `json:"title"`
	Emulator  string `json:"emulator"`
	Start     int64  `json:"start"`
	LastSeen  int64  `json:"lastSeen"`
}

func (st *ProfileStore) livePath() string { return filepath.Join(st.p.Profiles, "live-session.json") }

func (st *ProfileStore) Checkpoint(ls *LiveSession) {
	if ls == nil {
		_ = os.Remove(st.livePath())
		return
	}
	_ = writeJSONAtomic(st.livePath(), ls)
}

// RecoverLiveSession credits a session that was interrupted (NEXUS or the PC
// crashed) up to its last checkpoint.
func (st *ProfileStore) RecoverLiveSession() *SessionRec {
	var ls LiveSession
	if err := readJSON(st.livePath(), &ls); err != nil || ls.Key == "" {
		return nil
	}
	_ = os.Remove(st.livePath())
	_ = os.Remove(st.livePath() + ".bak")
	secs := ls.LastSeen - ls.Start
	if secs <= 0 || secs > 48*3600 {
		return nil
	}
	rec := SessionRec{Start: ls.Start, End: ls.LastSeen, Seconds: secs, Emulator: ls.Emulator, Recovered: true}
	st.AddSession(ls.ProfileID, ls.Key, ls.Title, rec)
	return &rec
}

// Recent returns game keys ordered by last played (most recent first).
func (s ProfileStats) Recent() []string {
	keys := make([]string, 0, len(s.Games))
	for k, v := range s.Games {
		if v.LastPlayed > 0 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return s.Games[keys[i]].LastPlayed > s.Games[keys[j]].LastPlayed })
	return keys
}

// normaliseHome keeps a user's home preferences valid (unknown values back to the defaults).
func normaliseHome(h *HomePrefs) {
	switch h.Layout {
	case "classic", "cards", "grid":
	default:
		h.Layout = "classic"
	}
	if h.Effects != "flat" {
		h.Effects = "glass"
	}
	if h.Count < 6 || h.Count > 48 {
		h.Count = 12
	}
	if h.Widgets == nil {
		h.Widgets = map[string]bool{"recent": true, "details": true}
	}
	if h.BgColor != "" && !reHexColor.MatchString(h.BgColor) {
		h.BgColor = ""
	}
	clean := func(gs []HomeGroup, max int) []HomeGroup {
		out := gs[:0]
		seen := map[string]bool{}
		for _, g := range gs {
			g.Name = strings.TrimSpace(g.Name)
			if g.ID == "" || seen[g.ID] || g.Name == "" || len(out) >= max {
				continue
			}
			if len(g.Name) > 40 {
				g.Name = g.Name[:40]
			}
			seen[g.ID] = true
			if g.Games == nil {
				g.Games = []string{}
			}
			out = append(out, g)
		}
		return out
	}
	h.Folders = clean(h.Folders, 32)
	h.Pages = clean(h.Pages, 12)
}

var reHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
