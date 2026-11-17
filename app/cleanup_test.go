package main

import (
	"runtime"
	"testing"
	"time"
)

// TestSessionCleanup: launching and closing a game five times leaves NEXUS with no more goroutines
// than before the first launch (the session monitor, timers and event followers all end), the
// emulator idle and the engine stopped. The real engine's own teardown (its threads, GPU memory)
// is in the DLL and needs a Windows run with a game.
func TestSessionCleanup(t *testing.T) {
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	pr, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	settle := func() int {
		for i := 0; i < 40; i++ {
			runtime.GC()
			time.Sleep(50 * time.Millisecond)
		}
		return runtime.NumGoroutine()
	}
	base := settle()
	for i := 0; i < 5; i++ {
		if err := app.Emu.Launch(&g, pr.ID); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 200 && app.Emu.State().Phase != "playing"; j++ {
			time.Sleep(20 * time.Millisecond)
		}
		if app.Emu.State().Phase != "playing" {
			t.Fatalf("round %d: never played", i)
		}
		stopTestSession(app)
	}
	after := settle()
	if after > base+2 { // a little slack for the runtime's own helpers
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		t.Fatalf("goroutines: %d before, %d after 5 sessions\n%s", base, after, buf[:n])
	}
	if st := app.Emu.State(); st.Running {
		t.Fatalf("still running: %+v", st)
	}
	h, _ := app.Emu.Host()
	if s := h.Status().State; s != "stopped" && s != "idle" {
		t.Fatalf("engine state after close: %s", s)
	}
	t.Logf("goroutines: %d before, %d after 5 launch/close cycles", base, after)
}
