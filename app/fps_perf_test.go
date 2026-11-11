package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFrameRateLimitAndMatchDisplay: the NEXUS frame-rate limit reaches the engine's Switch 2
// limit key; "match my display" replaces it with the main display's refresh rate (clamped
// 30–240); per-game values win over the global ones.
func TestFrameRateLimitAndMatchDisplay(t *testing.T) {
	hw := filepath.Join(t.TempDir(), "hw.json")
	data, _ := json.Marshal(Hardware{Displays: []Display{{Name: "A", RefreshHz: 60}, {Name: "B", RefreshHz: 165, Primary: true}}})
	os.WriteFile(hw, data, 0o644)
	t.Setenv("NEXUS_HARDWARE", hw)
	app, _ := newTestApp(t, "")
	app.DetectHardware(true)
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	other, _ := app.Library.Get("t-0100aaaa00020000")
	chk := func(want string) {
		t.Helper()
		ini, _ := LoadIni(app.Emu.SessionPath())
		if v, _ := ini.GetSetting("Renderer", "nx2_frame_rate_limit"); v != want {
			t.Fatalf("nx2_frame_rate_limit = %q, want %q", v, want)
		}
	}
	if err := app.Emu.SetGlobal(map[string]*string{"graphics.fps_limit": sp("90")}); err != nil {
		t.Fatal(err)
	}
	if err := app.Emu.SetGlobal(map[string]*string{"graphics.fps_limit": sp("500")}); err == nil {
		t.Fatal("limit above 240 accepted")
	}
	app.Emu.writeSessionConfig(&g, nil)
	chk("90")
	if err := app.Emu.SetGameSettings(g, map[string]*string{"graphics.fps_match_display": sp("true")}, nil); err != nil {
		t.Fatal(err)
	}
	app.Emu.writeSessionConfig(&g, nil)
	chk("165") // the primary display, not the first one
	app.Emu.writeSessionConfig(&other, nil)
	chk("90") // other games keep the global limit
	if primaryRefresh(Hardware{Displays: []Display{{RefreshHz: 360, Primary: true}}}) != 360 {
		t.Fatal("primaryRefresh")
	}
}

func TestPerfMonitorSelectionAndFormat(t *testing.T) {
	p := PerfMonitorSettings{Mode: "selected", Games: []string{"a"}}
	if !p.For("a") || p.For("b") {
		t.Fatal("selected mode")
	}
	p.Mode = "all"
	if !p.For("b") {
		t.Fatal("all mode")
	}
	p.Mode = "off"
	if p.For("a") {
		t.Fatal("off mode")
	}
	fps := 59.7
	out := formatPerf(PerfSample{FPS: &fps}, []string{"fps", "cputemp", "gpu"}, " | ")
	if !strings.Contains(out, "FPS 60") || strings.Count(out, "N/A") != 2 {
		t.Fatalf("unavailable metrics must read N/A, not a made-up value: %q", out)
	}
}

// TestCompatEvidenceFollowsEngine: problems seen with an older engine are history once the engine
// changes; a known fix is named; good results are kept with their engine; nothing becomes
// "works" without evidence.
func TestCompatEvidenceFollowsEngine(t *testing.T) {
	c := NewCompatStore(t.TempDir())
	ver := "0.3.0"
	c.SetEngine(func() string { return ver })
	oot, other := "t-0400486024dec000", "t-0100aaaa00010000"
	c.Record(oot, "OOT", "switch2", true, true, 300, "crash")
	c.Rate(other, "Other", "issues", "audio crackles")
	c.Record("t-good", "Good", "switch1", true, false, 400, "")
	c.Record("t-good", "Good", "switch1", true, false, 400, "")
	if c.Get(oot).Status != "issues" || c.Get(other).Status != "issues" || c.Get("t-good").Status != "works" {
		t.Fatal("before update")
	}
	ver = "0.4.0"
	v := c.Get(oot)
	if v.Status != "untested" || !strings.Contains(v.Why, "guard house") || !strings.Contains(v.Why, "0.3.0") {
		t.Fatalf("OOT after update: %+v", v)
	}
	if v := c.Get(other); v.Status != "untested" || strings.Contains(v.Why, "guard house") {
		t.Fatalf("other after update: %+v", v)
	}
	if v := c.Get("t-good"); v.Status != "works" || !strings.Contains(v.Why, "0.3.0") {
		t.Fatalf("good after update: %+v", v)
	}
	// new evidence with the new engine decides again (and a crash is reported again)
	c.Record(oot, "OOT", "switch2", true, true, 60, "crash")
	if c.Get(oot).Status != "issues" {
		t.Fatal("new crash not reported")
	}
}
