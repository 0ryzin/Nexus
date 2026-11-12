package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Compatibility records what actually happened when a game was run in the NEXUS engine on this
// PC, plus the user's own rating. Nothing here is copied from an upstream project's claims: a
// game is "untested" until it has been started in NEXUS.
//
//   works       – the user rated it as working, or it ran ≥ 10 minutes over ≥ 2 sessions with no crash
//   issues      – it ran but crashed at least once, or the user reported issues
//   unsupported – it never reached the first frame (load error / crash before rendering), or the user said so
//   boots       – reached gameplay without crashing but not played long enough to judge
//   untested    – never started in NEXUS

type CompatEntry struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	System     string `json:"system,omitempty"` // switch1 | switch2, as reported by the engine
	Sessions   int    `json:"sessions"`
	Booted     int    `json:"booted"`  // sessions that presented at least one frame
	Crashes    int    `json:"crashes"` // sessions that ended in an engine error
	Seconds    int64  `json:"seconds"`
	LastCode   string `json:"lastCode,omitempty"`
	LastRun    int64  `json:"lastRun"`
	UserRating string `json:"userRating,omitempty"` // works | issues | unsupported | ""
	UserNote   string `json:"userNote,omitempty"`
	// Engine: the NEXUS Engine version the counters above were collected with ("" = before 0.3.0,
	// which did not record it). RatedEngine: the version the user's rating was given with.
	Engine      string        `json:"engine,omitempty"`
	RatedEngine string        `json:"ratedEngine,omitempty"`
	Earlier     []CompatEpoch `json:"earlier,omitempty"` // evidence from older engine versions, newest first
}

// CompatEpoch is what was seen with one older engine version.
type CompatEpoch struct {
	Engine     string `json:"engine"`
	Sessions   int    `json:"sessions"`
	Booted     int    `json:"booted"`
	Crashes    int    `json:"crashes"`
	Seconds    int64  `json:"seconds"`
	LastCode   string `json:"lastCode,omitempty"`
	UserRating string `json:"userRating,omitempty"`
	UserNote   string `json:"userNote,omitempty"`
}

// knownFixes: problems fixed in a NEXUS Engine version, by base title ID. Evidence of problems
// collected with an older engine no longer marks these games: they show as not tested since the
// fix until they are played again (never as "works" without new evidence).
var knownFixes = map[string][]struct{ Engine, Text string }{
	"0400486024DEC000": {{"0.4.0", "The Castle Town guard house freeze (with Max Resolution) is fixed in NEXUS Engine 0.4.0 (engine fix #1313)."}},
}

type CompatView struct {
	CompatEntry
	Status string `json:"status"`
	Why    string `json:"why"`
	Source string `json:"source"` // user | automatic | none
}

type CompatStore struct {
	path     string
	mu       sync.Mutex
	m        map[string]*CompatEntry
	engineFn func() string
}

func NewCompatStore(dir string) *CompatStore {
	c := &CompatStore{path: filepath.Join(dir, "compatibility.json"), m: map[string]*CompatEntry{}}
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, &c.m)
	}
	return c
}

func (c *CompatStore) saveLocked() {
	data, _ := json.MarshalIndent(c.m, "", "  ")
	_ = os.MkdirAll(filepath.Dir(c.path), 0o755)
	tmp := c.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, c.path)
	}
}

func (c *CompatStore) entry(key, title string) *CompatEntry {
	e := c.m[key]
	if e == nil {
		e = &CompatEntry{Key: key}
		c.m[key] = e
	}
	if title != "" {
		e.Title = title
	}
	return e
}

// SetEngine sets how to read the engine version that new evidence is recorded with (and views
// are computed for).
func (c *CompatStore) SetEngine(f func() string) {
	c.mu.Lock()
	c.engineFn = f
	c.mu.Unlock()
}

func (c *CompatStore) engineLocked() string {
	if c.engineFn == nil {
		return ""
	}
	return c.engineFn()
}

// rollLocked moves evidence collected with another engine version into Earlier.
func (c *CompatStore) rollLocked(e *CompatEntry) {
	engine := c.engineLocked()
	if engine == "" || e.Engine == engine {
		return
	}
	if e.Sessions > 0 || e.UserRating != "" {
		ep := CompatEpoch{Engine: e.Engine, Sessions: e.Sessions, Booted: e.Booted, Crashes: e.Crashes, Seconds: e.Seconds, LastCode: e.LastCode}
		if e.UserRating != "" && e.RatedEngine != engine {
			ep.UserRating, ep.UserNote = e.UserRating, e.UserNote
			e.UserRating, e.UserNote, e.RatedEngine = "", "", ""
		}
		e.Earlier = append([]CompatEpoch{ep}, e.Earlier...)
		if len(e.Earlier) > 8 {
			e.Earlier = e.Earlier[:8]
		}
	}
	e.Sessions, e.Booted, e.Crashes, e.Seconds, e.LastCode = 0, 0, 0, 0, ""
	e.Engine = engine
}

// Record stores the outcome of one session.
func (c *CompatStore) Record(key, title, system string, booted, crashed bool, secs int64, code string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entry(key, title)
	c.rollLocked(e)
	e.Sessions++
	if booted {
		e.Booted++
	}
	if crashed {
		e.Crashes++
	}
	if system != "" {
		e.System = system
	}
	e.Seconds += secs
	e.LastCode = code
	e.LastRun = time.Now().Unix()
	c.saveLocked()
}

// Rate stores the user's rating ("" clears it).
func (c *CompatStore) Rate(key, title, rating, note string) error {
	switch rating {
	case "", "works", "issues", "unsupported":
	default:
		return errors.New("rating must be works, issues, unsupported or empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entry(key, title)
	c.rollLocked(e)
	e.UserRating, e.UserNote, e.RatedEngine = rating, note, c.engineLocked()
	c.saveLocked()
	return nil
}

func (c *CompatStore) Get(key string) CompatView {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.m[key]; e != nil {
		return compatView(*e, c.engineLocked())
	}
	return CompatView{CompatEntry: CompatEntry{Key: key}, Status: "untested", Source: "none",
		Why: "Not started in NEXUS yet."}
}

func (c *CompatStore) All() map[string]CompatView {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]CompatView, len(c.m))
	for k, e := range c.m {
		out[k] = compatView(*e, c.engineLocked())
	}
	return out
}

func compatView(e CompatEntry, engine string) (v CompatView) {
	if engine != "" && e.Engine != engine {
		// what was seen with an older engine: shown as history, not as the current status
		old := e
		e.Earlier = append([]CompatEpoch{{Engine: old.Engine, Sessions: old.Sessions, Booted: old.Booted, Crashes: old.Crashes,
			Seconds: old.Seconds, LastCode: old.LastCode}}, old.Earlier...)
		if old.UserRating != "" && old.RatedEngine != engine {
			e.Earlier[0].UserRating, e.Earlier[0].UserNote = old.UserRating, old.UserNote
			e.UserRating, e.UserNote = "", ""
		}
		e.Sessions, e.Booted, e.Crashes, e.Seconds, e.LastCode, e.Engine = 0, 0, 0, 0, "", engine
	}
	v = CompatView{CompatEntry: e, Source: "automatic"}
	defer func() { compatHistory(&v, engine) }()
	switch {
	case e.UserRating != "":
		v.Status, v.Source = e.UserRating, "user"
		v.Why = "Your rating."
		if e.UserNote != "" {
			v.Why = e.UserNote
		}
	case e.Sessions == 0:
		v.Status, v.Source, v.Why = "untested", "none", "Not started in NEXUS yet."
	case e.Booted == 0:
		v.Status = "unsupported"
		v.Why = "It has never reached the first frame in NEXUS"
		if e.LastCode != "" {
			v.Why += " (last error: " + e.LastCode + ")"
		}
		v.Why += "."
	case e.Crashes > 0:
		v.Status, v.Why = "issues", "It runs, but has stopped unexpectedly in NEXUS."
	case e.Seconds >= 600 && e.Booted >= 2:
		v.Status, v.Why = "works", "Played without crashes in NEXUS."
	default:
		v.Status, v.Why = "boots", "Starts in NEXUS; not played long enough (10 min over 2 sessions) to call it working yet."
	}
	return v
}

// compatHistory explains older evidence when nothing has been seen with this engine yet:
// earlier good results are kept (with the engine they were seen with), earlier problems are
// reported as history, together with any known fix.
func compatHistory(v *CompatView, engine string) {
	if v.Status != "untested" || len(v.Earlier) == 0 {
		return
	}
	last := v.Earlier[0]
	ver := last.Engine
	if ver == "" {
		ver = "an earlier version"
	} else {
		ver = "NEXUS Engine " + ver
	}
	bad := last.UserRating == "issues" || last.UserRating == "unsupported" || (last.UserRating == "" && (last.Crashes > 0 || (last.Sessions > 0 && last.Booted == 0)))
	good := last.UserRating == "works" || (last.UserRating == "" && !bad && last.Seconds >= 600 && last.Booted >= 2)
	switch {
	case good:
		v.Status, v.Source = "works", "automatic"
		v.Why = "Worked with " + ver + "; not played since the engine was updated."
	case bad:
		v.Why = "Had problems with " + ver + "; not played since the engine was updated."
	default:
		v.Why = "Started with " + ver + "; not played since the engine was updated."
	}
	tid := strings.ToUpper(strings.TrimPrefix(v.Key, "t-"))
	for _, f := range knownFixes[tid] {
		if engine == "" || CompareVersions(engine, f.Engine) >= 0 {
			v.Why += " " + f.Text
		}
	}
}
