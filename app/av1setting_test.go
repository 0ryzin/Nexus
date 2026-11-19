package main

import "testing"

// TestAV1MoviesSetting: on by default (also for settings from before 0.8.1), off for all games or
// one game, and the per-game value wins.
func TestAV1MoviesSetting(t *testing.T) {
	a, _ := newTestApp(t, "")
	if v := a.Settings.Get().Engine["adv.av1_movies"]; v != "true" {
		t.Fatalf("default %q", v)
	}
	if !a.Emu.av1MoviesFor(nil) {
		t.Fatal("off by default")
	}
	a.Settings.Update(func(s *GlobalSettings) { s.Engine["adv.av1_movies"] = "false" })
	if a.Emu.av1MoviesFor(nil) {
		t.Fatal("global off ignored")
	}
}
