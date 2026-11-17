package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// playOnce plays the first test game for a moment as the first user of app and returns the shader
// counts the engine reported while playing.
func playOnce(t *testing.T, app *App, d time.Duration) ShaderStatus {
	t.Helper()
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	ps := app.Profiles.List()
	var pid string
	if len(ps) > 0 {
		pid = ps[0].ID
	} else {
		p, _ := app.Profiles.Create("Player", Avatar{Kind: "glyph"})
		pid = p.ID
	}
	if err := app.Emu.Launch(&g, pid); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return app.Emu.State().Phase == "playing" })
	var first *ShaderStatus
	deadline := time.Now().Add(d)
	var last ShaderStatus
	for time.Now().Before(deadline) {
		host, _ := app.Emu.Host()
		if st := host.Status(); st.Shaders != nil && st.State == "running" {
			if first == nil {
				f := *st.Shaders
				first = &f
			}
			last = *st.Shaders
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopTestSession(app)
	waitFor(t, func() bool { return !app.Emu.State().Running })
	if first != nil {
		last.FromCache = first.FromCache // what was ready from the cache when play began
	}
	return last
}

// TestSharedShaderCacheTwoUsers: two NEXUS data folders - two Windows users, each with their
// own NEXUS and their own profiles - and one shared shader folder. What the first user's session
// built is in the second user's cache from the start; nothing of the first user's own data is
// in the second's. (Stand-in engine: the real engine's part is engine patch 0030.)
func TestSharedShaderCacheTwoUsers(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "ProgramData", "NEXUS", "ShaderCache")
	t.Setenv("NEXUS_SHARED_SHADER_DIR", shared)
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	a, _ := newTestApp(t, "")
	b, _ := newTestApp(t, "")
	if a.Emu.ShaderDir() != shared || b.Emu.ShaderDir() != shared {
		t.Fatalf("shader folders %s, %s", a.Emu.ShaderDir(), b.Emu.ShaderDir())
	}
	sa := playOnce(t, a, 6*time.Second)
	if sa.FromCache != 0 || sa.Compiled != fakeGamePipelines {
		t.Fatalf("first user, first session: %+v", sa)
	}
	b.Library.Scan()
	st := b.Emu.SharedCacheStatus()
	if !st.Shared || len(st.Titles) != 1 || st.Titles[0].Graphics != fakeGamePipelines || st.Titles[0].Title != "Alpha Quest" {
		t.Fatalf("the second user's view of the shared cache: %+v", st)
	}
	sb := playOnce(t, b, 2*time.Second)
	if sb.FromCache != fakeGamePipelines || sb.Compiled != 0 {
		t.Fatalf("second user: %d from the cache, %d built (want %d, 0)", sb.FromCache, sb.Compiled, fakeGamePipelines)
	}
	// per-user data stays apart: no profile, setting or save of one is in the other's folder
	if len(b.Profiles.List()) != 1 || filepath.Dir(a.Paths.Logs) == filepath.Dir(b.Paths.Logs) {
		t.Fatal("users' data mixed")
	}
	// the cache survives a restart: a third session (first user again) builds nothing
	if s := playOnce(t, a, time.Second); s.FromCache != fakeGamePipelines || s.Compiled != 0 {
		t.Fatalf("after the other user played: %+v", s)
	}
}

// TestSharedShaderCacheConcurrent: two users playing the same game at once and both writing the
// shared log: it stays readable, every pipeline is in it once, nothing is damaged.
func TestSharedShaderCacheConcurrent(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "shared")
	t.Setenv("NEXUS_SHARED_SHADER_DIR", shared)
	path := filepath.Join(shared, "0100aaaa00010000", v2LogName)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			c := &fakeCache{path: path, title: "0100aaaa00010000", have: map[uint64]bool{}, built: map[uint64]bool{}}
			for i := 1; i <= fakeGamePipelines+3; i += 4 {
				c.build(min(i, fakeGamePipelines))
				c.flush()
			}
		}(w)
	}
	wg.Wait()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, recs, end, err := parseV2Log(b)
	if err != nil || end != len(b) {
		t.Fatalf("log damaged: %v (valid to %d of %d)", err, end, len(b))
	}
	live, _, _ := liveRecords(b, recs)
	if len(live) != fakeGamePipelines {
		t.Fatalf("%d pipelines in the log, want %d", len(live), fakeGamePipelines)
	}
	if len(recs) > fakeGamePipelines*2 {
		t.Logf("%d records for %d pipelines (duplicates from racing writers are harmless: the latest wins)", len(recs), len(live))
	}
}

// TestSharedShaderCacheTakesUsersOwn: a user's earlier private cache is added to the shared one
// once, without changing or removing the user's files; a damaged shared log is reported.
func TestSharedShaderCacheTakesUsersOwn(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "shared")
	t.Setenv("NEXUS_SHARED_SHADER_DIR", shared)
	app, _ := newTestApp(t, "")
	own := filepath.Join(app.Emu.UserShaderDir(), "0100aaaa00010000")
	writeLog(t, filepath.Join(own, v2LogName), v2rec(v2KindGraphics, 1, "a"), v2rec(v2KindProgram, 2, "p"))
	os.WriteFile(filepath.Join(own, "warm-compute.bin"), []byte("precache"), 0o644)
	os.WriteFile(filepath.Join(own, "warm-compute.done"), []byte("v2 marker"), 0o644)
	before, _ := os.ReadFile(filepath.Join(own, v2LogName))
	app.Emu.shareUserCaches()
	app.Emu.shareUserCaches() // once
	got := liveOf(t, filepath.Join(shared, "0100aaaa00010000", v2LogName))
	if len(got) != 2 || got[rk{v2KindGraphics, 1}] != "a" {
		t.Fatalf("shared log: %v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(shared, "0100aaaa00010000", "warm-compute.bin")); string(b) != "precache" {
		t.Fatal("precache file not shared")
	}
	if fileExists(filepath.Join(shared, "0100aaaa00010000", "warm-compute.done")) {
		t.Fatal("this user's driver marker was shared")
	}
	if after, _ := os.ReadFile(filepath.Join(own, v2LogName)); string(after) != string(before) {
		t.Fatal("the user's own cache was changed")
	}
	st := app.Emu.SharedCacheStatus()
	if len(st.Titles) != 1 || !st.Titles[0].Precache || !st.Titles[0].DoneHere || st.Titles[0].Programs != 1 {
		t.Fatalf("status %+v", st)
	}
	os.WriteFile(filepath.Join(shared, "0100aaaa00010000", v2LogName), []byte("garbage, not a log"), 0o644)
	if st := app.Emu.SharedCacheStatus(); !st.Titles[0].Damaged {
		t.Fatalf("damaged log not reported: %+v", st.Titles[0])
	}
	// switched off: the user's own folder again
	app.Settings.Update(func(g *GlobalSettings) { g.Emulator.PrivateShaderCache = true })
	if app.Emu.ShaderDir() != app.Emu.UserShaderDir() {
		t.Fatal("private cache setting ignored")
	}
}
