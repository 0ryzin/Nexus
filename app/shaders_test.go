package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func u64(v uint64) *uint64 { return &v }

// TestShaderSegment: the performance bar's shader reading never invents progress: compiling
// counts only builds the game waits for, preparing is the background prebuild, unknown is N/A.
func TestShaderSegment(t *testing.T) {
	cases := []struct {
		name string
		sh   *ShaderStatus
		want PerfSeg
	}{
		{"no engine data", nil, PerfSeg{"shader", "SHADERS", "N/A", ""}},
		{"builds unknown", &ShaderStatus{Compiled: 4}, PerfSeg{"shader", "SHADERS", "N/A", ""}},
		{"compiling", &ShaderStatus{Queued: u64(7), Running: u64(2), BackgroundQueued: u64(3)}, PerfSeg{"shaderbusy", "SHADERS", "6", "compiling"}},
		{"background only", &ShaderStatus{Queued: u64(3), Running: u64(0), BackgroundQueued: u64(3)}, PerfSeg{"shader", "SHADERS", "3", "preparing"}},
		{"ready", &ShaderStatus{FromCache: 900, Queued: u64(0), Running: u64(0), BackgroundQueued: u64(0), Failed: u64(0)}, PerfSeg{"shader", "SHADERS", "", "ready"}},
		{"ready with new and failed", &ShaderStatus{Compiled: 37, Queued: u64(0), Running: u64(0), Failed: u64(2)}, PerfSeg{"shader", "SHADERS", "", "ready · 37 new · 2 failed"}},
	}
	for _, c := range cases {
		if got := shaderSegment(c.sh); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
	if s := formatPerf(PerfSample{Shaders: cases[2].sh}, []string{"shaders"}, " "); s != "Shaders 6 compiling" {
		t.Errorf("text: %q", s)
	}
}

// TestShaderMetricAddedOnce: existing settings get the shader reading once; removing it is kept.
func TestShaderMetricAddedOnce(t *testing.T) {
	dir := t.TempDir()
	p := &Paths{Settings: dir}
	old := map[string]any{"version": 1, "keysVersion": 3, "perfMonitor": map[string]any{"mode": "all", "metrics": []string{"fps", "cpu"}}}
	b, _ := json.Marshal(old)
	os.WriteFile(filepath.Join(dir, "settings.json"), b, 0o644)
	st, err := LoadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if m := st.Get().PerfMonitor.Metrics; !slices.Equal(m, []string{"fps", "cpu", "shaders"}) {
		t.Fatalf("metrics after upgrade: %v", m)
	}
	st.Update(func(g *GlobalSettings) { g.PerfMonitor.Metrics = []string{"fps"} })
	st2, _ := LoadSettings(p)
	if m := st2.Get().PerfMonitor.Metrics; !slices.Equal(m, []string{"fps"}) {
		t.Fatalf("removed shader reading came back: %v", m)
	}
	if !slices.Contains(DefaultSettings().PerfMonitor.Metrics, "shaders") {
		t.Fatal("new installs do not show shaders")
	}
}

// TestShaderStatusDuringStart: while a game starts, the loading screen's state carries the
// shader numbers; once it plays they leave the state (the performance bar samples them).
func TestShaderStatusDuringStart(t *testing.T) {
	t.Setenv("NEXUS_FAKE_LOAD", "1200ms")
	t.Setenv("NEXUS_FAKE_PLAY", "4s")
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	pr, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	if err := app.Emu.Launch(&g, pr.ID); err != nil {
		t.Fatal(err)
	}
	defer stopTestSession(app)
	sawShaders, played := false, false
	for m := range ev {
		s := string(m)
		if !strings.Contains(s, `"type":"emulator.state"`) {
			continue
		}
		var e struct{ Data RunState }
		json.Unmarshal(m, &e)
		if e.Data.Phase == "starting" && e.Data.Shaders != nil && e.Data.Shaders.FromCache > 0 {
			sawShaders = true
		}
		if e.Data.Phase == "playing" {
			played = true
			if e.Data.Shaders != nil {
				t.Fatal("shader numbers stay in the state while playing")
			}
			break
		}
	}
	if !sawShaders || !played {
		t.Fatalf("shaders during start %v, played %v", sawShaders, played)
	}
	sample := perf.Sample(app.Emu)
	if sample.Shaders == nil {
		t.Fatal("the performance sample has no shader status while playing")
	}
}
