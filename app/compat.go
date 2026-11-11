package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
}

type CompatView struct {
	CompatEntry
	Status string `json:"status"`
	Why    string `json:"why"`
	Source string `json:"source"` // user | automatic | none
}

type CompatStore struct {
	path string
	mu   sync.Mutex
	m    map[string]*CompatEntry
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

// Record stores the outcome of one session.
func (c *CompatStore) Record(key, title, system string, booted, crashed bool, secs int64, code string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entry(key, title)
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
	e.UserRating, e.UserNote = rating, note
	c.saveLocked()
	return nil
}

func (c *CompatStore) Get(key string) CompatView {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.m[key]; e != nil {
		return compatView(*e)
	}
	return CompatView{CompatEntry: CompatEntry{Key: key}, Status: "untested", Source: "none",
		Why: "Not started in NEXUS yet."}
}

func (c *CompatStore) All() map[string]CompatView {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]CompatView, len(c.m))
	for k, e := range c.m {
		out[k] = compatView(*e)
	}
	return out
}

func compatView(e CompatEntry) CompatView {
	v := CompatView{CompatEntry: e, Source: "automatic"}
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
