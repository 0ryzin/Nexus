package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
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

// writeSymbols writes an engine symbol table (tools/release/make_symbols.py's format).
func writeSymbols(t *testing.T, dir, sha string, lines ...string) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, engineSymbolsFile))
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	fmt.Fprintf(zw, "NEXUSSYM 1 %s 180000000 %d\n", sha, len(lines))
	for _, l := range lines {
		fmt.Fprintln(zw, l)
	}
	zw.Close()
	f.Close()
}

// TestCrashReportDecodesEngineFrames: the engine's frames are named from the symbol table that
// came with that engine build, and only from that one.
func TestCrashReportDecodesEngineFrames(t *testing.T) {
	app, root := newTestApp(t, "")
	engineDir := filepath.Join(root, "engine-build")
	os.MkdirAll(engineDir, 0o755)
	sha := strings.Repeat("ab", 32)
	writeSymbols(t, engineDir, sha,
		"1000 vk_scheduler.obj public: void __cdecl Vulkan::Scheduler::WorkerThread(class std::stop_token)",
		"842a00 vk_present.obj public: void __cdecl Vulkan::PresentManager::Shutdown(void)",
		"843000 other.obj nexus_engine_stop")
	logDir := filepath.Join(app.Emu.DataDir(), "log")
	os.MkdirAll(logDir, 0o755)
	crash := `[ 501.541319] Common <Critical> common\windows\crash_handler.cpp:107:Report: Host crash: unhandled exception 0xc0000005 at nvoglv64.dll+0xe4e577 on thread 24248 "VulkanWorker", info 0x0 0x0; stack: nvoglv64.dll+0xe4e577 graphics-hook64.dll+0xe99c nexus_engine.dll+0x842b36 nexus_engine.dll+0x1056`
	os.WriteFile(filepath.Join(logDir, "eden_log.txt"), []byte(crash+"\n"), 0o644)
	report := func(recSHA string) string {
		rec := sessionRecord{PID: os.Getpid() + 1, Started: time.Now().Add(-time.Minute), Updated: time.Now(), Game: "Alpha",
			Phase: "stopping", EngineDir: engineDir, EngineSHA: recSHA, Settings: map[string]string{"backend": "1"}}
		data, _ := json.Marshal(rec)
		os.MkdirAll(app.Paths.Logs, 0o755)
		os.WriteFile(filepath.Join(app.Paths.Logs, "session.json"), data, 0o644)
		n := app.checkLastSession()
		if n == nil {
			t.Fatal("no report")
		}
		b, _ := os.ReadFile(n.Log)
		return string(b) + "\n=== technical\n" + n.Technical
	}
	s := report(sha)
	for _, want := range []string{"Renderer:       Vulkan",
		"nexus_engine.dll+0x842b36  public: void __cdecl Vulkan::PresentManager::Shutdown(void)+0x136 (vk_present.obj)",
		"nexus_engine.dll+0x1056  public: void __cdecl Vulkan::Scheduler::WorkerThread(class std::stop_token)+0x56 (vk_scheduler.obj)"} {
		if !strings.Contains(s, want) {
			t.Errorf("report lacks %q:\n%s", want, s)
		}
	}
	if !strings.Contains(s[strings.Index(s, "=== technical"):], "PresentManager::Shutdown") {
		t.Error("the decoded frames are not in the technical details")
	}
	// another engine build: never decoded with this table
	s = report(strings.Repeat("cd", 32))
	if strings.Contains(s, "PresentManager") || !strings.Contains(s, "not decoded: the engine symbols are for another engine build") {
		t.Errorf("decoded with another build's table:\n%s", s)
	}
	// no table
	os.Remove(filepath.Join(engineDir, engineSymbolsFile))
	if s = report(sha); !strings.Contains(s, "not decoded: no engine symbols") {
		t.Errorf("missing table:\n%s", s)
	}
}

func TestEngineSymbolsDamaged(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("ab", 32)
	writeSymbols(t, dir, sha, "2000 a.obj B", "1000 a.obj A") // out of order
	if _, err := loadEngineSymbols(dir, sha); err == nil {
		t.Error("unsorted table accepted")
	}
	f, _ := os.Create(filepath.Join(dir, engineSymbolsFile))
	zw := gzip.NewWriter(f)
	fmt.Fprintf(zw, "NEXUSSYM 1 %s 180000000 3\n1000 a.obj A\n", sha) // cut short
	zw.Close()
	f.Close()
	if _, err := loadEngineSymbols(dir, sha); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("cut-short table: %v", err)
	}
	os.WriteFile(filepath.Join(dir, engineSymbolsFile), []byte("not gzip"), 0o644)
	if _, err := loadEngineSymbols(dir, sha); err == nil {
		t.Error("damaged table accepted")
	}
}
