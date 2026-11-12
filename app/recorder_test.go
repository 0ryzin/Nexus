package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func playing(t *testing.T, app *App) (Profile, *fakeEngine) {
	t.Helper()
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	p, _ := app.Profiles.Create("Kai", Avatar{Kind: "glyph"})
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200 && app.Emu.State().Phase != "playing"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	f := fakeLast
	f.Reveal()
	for i := 0; i < 100 && app.Emu.GameWindow() == 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	return p, f
}

// TestRecordingWithF12: F12 in the game starts a recording, F12 again stops it; the file is
// renamed from .part, appears in Media as a recording of that game, and the game is unaffected.
func TestRecordingWithF12(t *testing.T) {
	app, _ := newTestApp(t, "")
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	_, f := playing(t, app)
	if f.req.RecordKey != "F12" || f.req.ScreenshotKey != "Alt+F12" {
		t.Fatalf("keys %q %q", f.req.RecordKey, f.req.ScreenshotKey)
	}
	f.PressRecord()
	if waitEvent(t, ev, "recording.started") == nil || !app.Recorder.Active() {
		t.Fatal("F12 did not start recording")
	}
	parts, _ := filepath.Glob(filepath.Join(app.Media.RecordingsDir(), "*.mp4.part"))
	if len(parts) != 1 || !strings.HasPrefix(filepath.Base(parts[0]), "AQ ") {
		t.Fatalf("part file %v", parts)
	}
	time.Sleep(300 * time.Millisecond)
	f.PressRecord()
	if waitEvent(t, ev, "recording.saved") == nil || app.Recorder.Active() {
		t.Fatal("F12 did not stop recording")
	}
	items := app.Media.List()
	if len(items) != 1 || items[0].Type != "recording" || items[0].GameKey != "t-0100aaaa00010000" || items[0].Ext != "mp4" {
		t.Fatalf("media %+v", items)
	}
	// a second recording gets its own file; closing the game stops and keeps it
	if err := app.Recorder.Start(); err != nil {
		t.Fatal(err)
	}
	app.Emu.Stop(false)
	waitIdle(t, app)
	if app.Recorder.Active() || len(app.Media.List()) != 2 {
		t.Fatalf("recording at game close: %d items", len(app.Media.List()))
	}
	if err := app.Recorder.Start(); err == nil {
		t.Fatal("recording without a game")
	}
}

// TestRecordingRecoveryAndSettings: a recording cut off by a crash is kept as "(interrupted)";
// software-only PCs are not offered 4K; F12 screenshots move to Alt+F12 once.
func TestRecordingRecoveryAndSettings(t *testing.T) {
	app, _ := newTestApp(t, "")
	dir := app.Media.RecordingsDir()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "AQ 2026-11-12 10-00-00.mp4.part"), []byte("fragmented mp4"), 0o644)
	os.WriteFile(filepath.Join(dir, "empty.mp4.part"), nil, 0o644)
	app.Recorder.RecoverInterrupted()
	if !fileExists(filepath.Join(dir, "AQ 2026-11-12 10-00-00 (interrupted).mp4")) || fileExists(filepath.Join(dir, "empty.mp4.part")) {
		entries, _ := os.ReadDir(dir)
		t.Fatalf("recovery: %v", entries)
	}
	c := app.Recorder.Caps()
	if !c.Available || !c.Hardware || c.Vendor != "nvidia" || !contains(c.Resolutions, "2160p") {
		t.Fatalf("caps %+v", c)
	}
	soft := CaptureCaps{Available: true, Resolutions: []string{"native", "720p", "1080p"}}
	s := app.Settings.Get().Recording
	s.Resolution, s.Codec = "2160p", "hevc"
	app.Settings.Update(func(g *GlobalSettings) { g.Recording = s })
	eff := app.Recorder.settingsFor(soft)
	if eff.Resolution != "1080p" || eff.Codec != "h264" {
		t.Fatalf("software PC: %+v", eff)
	}
	if b := RecordingBitrate(RecordingSettings{Resolution: "1080p", FPS: 60, Quality: "high", Codec: "h264"}); b < 10000 || b > 15000 {
		t.Fatalf("bitrate %d", b)
	}

	// settings from 0.3: screenshot key F12 -> Alt+F12
	p := filepath.Join(app.Paths.Settings, "settings.json")
	var raw map[string]any
	data, _ := os.ReadFile(p)
	json.Unmarshal(data, &raw)
	delete(raw, "keysVersion")
	delete(raw, "recording")
	em := raw["emulator"].(map[string]any)
	em["screenshotKey"] = "F12"
	delete(em, "homeAction")
	data, _ = json.Marshal(raw)
	os.WriteFile(p, data, 0o644)
	st, err := LoadSettings(app.Paths)
	if err != nil {
		t.Fatal(err)
	}
	g := st.Get()
	if g.Emulator.ScreenshotKey != "Alt+F12" || g.Recording.Key != "F12" || !g.Recording.Enabled || g.Emulator.HomeAction != "background" {
		t.Fatalf("migration %+v %+v", g.Emulator.ScreenshotKey, g.Recording)
	}
}

// TestHomeKeepsGameRunning: NEXUS Home hides the game but keeps it running (frames go on),
// muted; Resume restores the volume. With "pause" the game is paused instead.
func TestHomeKeepsGameRunning(t *testing.T) {
	app, _ := newTestApp(t, "")
	_, f := playing(t, app)
	if err := app.Emu.Suspend(true); err != nil {
		t.Fatal(err)
	}
	st := f.Status()
	if !st.Suspended || !st.Background || f.volume != 0 || !app.Emu.State().Background {
		t.Fatalf("background: %+v volume %d", st, f.volume)
	}
	fr := st.Frames
	time.Sleep(300 * time.Millisecond)
	if f.Status().Frames <= fr {
		t.Fatal("the game stopped running behind NEXUS")
	}
	if err := app.Emu.Screenshot(); err != nil { // screenshots also work in the background
		t.Fatal(err)
	}
	if err := app.Emu.Suspend(false); err != nil {
		t.Fatal(err)
	}
	if st := f.Status(); st.Suspended || st.Background || f.volume != app.Emu.State().Volume {
		t.Fatalf("resume: %+v volume %d", st, f.volume)
	}
	app.Settings.Update(func(g *GlobalSettings) { g.Emulator.HomeAction = "pause" })
	app.Emu.Suspend(true)
	if st := f.Status(); !st.Suspended || st.Background {
		t.Fatalf("pause: %+v", st)
	}
	fr = f.Status().Frames
	time.Sleep(300 * time.Millisecond)
	if f.Status().Frames != fr {
		t.Fatal("paused game kept running")
	}
	app.Emu.Stop(false)
	waitIdle(t, app)
}
