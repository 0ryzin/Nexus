package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStatsOneDeviceUnderSeveralIDs: the reported case. A PC uploaded its Ocarina of Time time
// under three device IDs (one per sign-in); the Steam Deck added all of them up and showed about
// 31 h instead of 15.6 h. Older uploads of the same install now count once; a genuinely
// different device still adds its own time.
func TestStatsOneDeviceUnderSeveralIDs(t *testing.T) {
	app, _ := newTestApp(t, "")
	p, _ := app.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	key := "t-0400486024dec000"
	// the Deck's own play: 350 s
	app.Profiles.mu.Lock()
	s := app.Profiles.loadStatsLocked(p.ID)
	s.Games[key] = &GameStat{Seconds: 350, FirstPlay: 1795030174, LastPlayed: 1795042167, Sessions: 2,
		History: []SessionRec{{Start: 1795041995, End: 1795042015, Seconds: 20}, {Start: 1795030174, End: 1795030504, Seconds: 330}}}
	writeJSONAtomic(app.Profiles.statsPath(p.ID), s)
	delete(app.Profiles.stats, p.ID)
	app.Profiles.mu.Unlock()
	dir := filepath.Join(app.Profiles.profileDir(p.ID), "stats-devices")
	os.MkdirAll(dir, 0o755)
	pc := func(id string, secs, last int64, hist []SessionRec) {
		writeJSONAtomic(filepath.Join(dir, "account-"+id+".json"), map[string]any{"device": "Toms_Pc", "stats": ProfileStats{Games: map[string]*GameStat{
			key: {Seconds: secs, FirstPlay: 1794349787, LastPlayed: last, History: hist}}}})
	}
	h1 := []SessionRec{{Start: 1794601357, Seconds: 278}, {Start: 1794349787, Seconds: 12}}
	h2 := append([]SessionRec{{Start: 1794938749, Seconds: 386}}, h1...)
	h3 := append([]SessionRec{{Start: 1795032272, Seconds: 244}}, h2...)
	pc("848e1f3e67009200c7d0752a", 1500, 1794601635, h1)
	pc("fe1a753f9a73200f4b561d26", 55140, 1794939135, h2)
	pc("d32da31ff5df9a102cf00294", 55770, 1795032516, h3)
	app.Profiles.dropStatsCache(p.ID)
	got := app.Profiles.Stats(p.ID).Games[key].Seconds
	if got != 350+55770 {
		t.Fatalf("total %d s (%.1f h), want %d (the PC once, newest)", got, float64(got)/3600, 350+55770)
	}
	// another, different device adds its own time
	writeJSONAtomic(filepath.Join(dir, "account-laptop.json"), map[string]any{"device": "Laptop", "stats": ProfileStats{Games: map[string]*GameStat{
		key: {Seconds: 600, FirstPlay: 1795000000, LastPlayed: 1795000600, History: []SessionRec{{Start: 1795000000, Seconds: 600}}}}}})
	app.Profiles.dropStatsCache(p.ID)
	if got := app.Profiles.Stats(p.ID).Games[key].Seconds; got != 350+55770+600 {
		t.Fatalf("with a second device: %d", got)
	}
	// this device's own older upload (shares a session) is not added to itself
	writeJSONAtomic(filepath.Join(dir, "account-olddeck.json"), map[string]any{"device": "Steam Deck", "stats": ProfileStats{Games: map[string]*GameStat{
		key: {Seconds: 330, FirstPlay: 1795030174, LastPlayed: 1795030504, History: []SessionRec{{Start: 1795030174, Seconds: 330}}}}}})
	app.Profiles.dropStatsCache(p.ID)
	if got := app.Profiles.Stats(p.ID).Games[key].Seconds; got != 350+55770+600 {
		t.Fatalf("own older upload counted: %d", got)
	}
}
