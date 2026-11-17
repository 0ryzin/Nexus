package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSessionRecordFollowsTheSession: while a game session runs NEXUS keeps Logs\session.json
// with the game and what it is doing; a session that ends normally removes it.
func TestSessionRecordFollowsTheSession(t *testing.T) {
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	pr, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	rec := filepath.Join(app.Paths.Logs, "session.json")
	if err := app.Emu.Launch(&g, pr.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return app.Emu.State().Phase == "playing" })
	var r sessionRecord
	waitFor(t, func() bool {
		data, err := os.ReadFile(rec)
		return err == nil && json.Unmarshal(data, &r) == nil && r.Phase == "playing" && r.Frames > 0
	})
	if r.Game != g.Title || r.TitleID != g.TitleID || r.PID != os.Getpid() || r.NEXUS != AppVersion {
		t.Fatalf("record %+v", r)
	}
	if r.Settings["use_asynchronous_shaders"] == "" && len(r.Settings) == 0 {
		t.Fatalf("no session settings in the record: %+v", r)
	}
	stopTestSession(app)
	waitFor(t, func() bool { return !app.Emu.State().Running })
	waitFor(t, func() bool { _, err := os.Stat(rec); return os.IsNotExist(err) })
	// a NEXUS that started cleanly after that has nothing to report
	if n := app.checkLastSession(); n != nil {
		t.Fatalf("clean end reported as a crash: %+v", n)
	}
}

// TestCrashReportAfterUncleanEnd: a session record left by an earlier NEXUS process means it
// ended without NEXUS seeing it end. The next start writes one report (with the engine's crash
// line and dump, without personal data) and tells the user once.
func TestCrashReportAfterUncleanEnd(t *testing.T) {
	app, root := newTestApp(t, "")
	home := filepath.Join(root, "home", "tmill")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "nolocal"))
	start := time.Now().Add(-11 * time.Minute)
	rec := sessionRecord{NEXUS: "0.7.0", Engine: "0.8.0", PID: os.Getpid() + 1, Started: start, Updated: time.Now().Add(-time.Minute),
		Game: "The Legend of Zelda: Ocarina of Time", TitleID: "0400486024DEC000", Phase: "playing", EngineState: "running", Frames: 38000,
		Prep:    &PrepStatus{State: "done", Archives: 2916, Programs: 4794, Skipped: 18, Seconds: 14.6},
		Shaders: &ShaderStatus{FromCache: 120, Compiled: 7, Queued: ptr(uint64(500))},
		GPU:     "NVIDIA RTX", Settings: map[string]string{"resolution_setup": "3", "use_asynchronous_shaders": "true"}}
	data, _ := json.Marshal(rec)
	os.MkdirAll(app.Paths.Logs, 0o755)
	os.WriteFile(filepath.Join(app.Paths.Logs, "session.json"), data, 0o644)
	logDir := filepath.Join(app.Emu.DataDir(), "log")
	os.MkdirAll(logDir, 0o755)
	engineLog := strings.Join([]string{
		"[ 665.522175] HW.Memory <Error> core/device_memory_manager.inc:169:ReportUnmappedRead: Unmapped Device ReadBlock",
		"[ 666.000000] Frontend <Info> loading " + filepath.Join(home, "Games", "oot.nup"),
		"[ 666.100000] Crypto <Info> loaded prod.keys from somewhere",
		"[ 666.200000] Service <Info> signed in as someone@example.com",
		"[ 666.672775] Common <Critical> Host crash: unhandled exception 0xc0000005 at nexus_engine.dll+0x1234 on thread 42 \"GPU\", info 0x1 0xdead; stack: nexus_engine.dll+0x1234",
	}, "\n")
	os.WriteFile(filepath.Join(logDir, "eden_log.txt"), []byte(engineLog), 0o644)
	os.WriteFile(filepath.Join(logDir, "eden_crash.dmp"), []byte("MDMP"), 0o644)

	n := app.checkLastSession()
	if n == nil {
		t.Fatal("no crash notice")
	}
	if n.Title != rec.Game || !strings.Contains(n.Detail, "while you were playing") || !strings.Contains(n.Technical, "Host crash") {
		t.Fatalf("notice %+v", n)
	}
	report, err := os.ReadFile(n.Log)
	if err != nil {
		t.Fatal(err)
	}
	s := string(report)
	for _, want := range []string{"Ocarina of Time (0400486024DEC000)", "Phase / stage:  playing", "2916 archives",
		"500 queued", "resolution_setup=3", "Host crash: unhandled exception 0xc0000005", "Minidump: crash-", "%USERPROFILE%"} {
		if !strings.Contains(s, want) {
			t.Errorf("report lacks %q:\n%s", want, s)
		}
	}
	for _, never := range []string{"tmill", "prod.keys", "someone@example.com"} {
		if strings.Contains(s, never) {
			t.Errorf("report contains %q", never)
		}
	}
	if _, err := os.Stat(filepath.Join(logDir, "eden_crash.dmp")); !os.IsNotExist(err) {
		t.Error("the engine's dump was not kept with the report")
	}
	dumps, _ := filepath.Glob(filepath.Join(app.Paths.Logs, "crashes", "crash-*.dmp"))
	if len(dumps) != 1 {
		t.Errorf("dumps kept: %v", dumps)
	}
	if _, err := os.Stat(filepath.Join(app.Paths.Logs, "session.json")); !os.IsNotExist(err) {
		t.Error("session record not removed")
	}
	// once: the next start has nothing to say, and the interface gets the notice once
	if app.checkLastSession() != nil {
		t.Error("reported twice")
	}
	app.setCrashNotice(n)
	if app.takeCrashNotice() == nil || app.takeCrashNotice() != nil {
		t.Error("notice not handed out exactly once")
	}
}

// TestCrashReportWithoutEngineHandler: no "Host crash" line (a fail-fast, or the process ended
// from outside) still gives a report that says so and the last engine errors.
func TestCrashReportWithoutEngineHandler(t *testing.T) {
	app, _ := newTestApp(t, "")
	rec := sessionRecord{PID: os.Getpid() + 1, Started: time.Now().Add(-time.Minute), Updated: time.Now(), Game: "Alpha Quest",
		Phase: "starting", Stage: "prepare", Prep: &PrepStatus{State: "running", Stage: "Converting", Done: 10, Total: 40}}
	data, _ := json.Marshal(rec)
	os.MkdirAll(app.Paths.Logs, 0o755)
	os.WriteFile(filepath.Join(app.Paths.Logs, "session.json"), data, 0o644)
	n := app.checkLastSession()
	if n == nil || !strings.Contains(n.Detail, "while preparing shaders") {
		t.Fatalf("notice %+v", n)
	}
	s, _ := os.ReadFile(n.Log)
	if !strings.Contains(string(s), "the engine's handler did not run") || !strings.Contains(string(s), `stage "Converting", 10/40`) {
		t.Fatalf("report:\n%s", s)
	}
}

func ptr[T any](v T) *T { return &v }
