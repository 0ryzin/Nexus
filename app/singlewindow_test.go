package main

import (
	"sync"
	"testing"
	"time"
)

// testUI stands in for the NEXUS window (app_windows.go) and records full-screen changes.
type testUI struct {
	noopUI
	mu    sync.Mutex
	full  bool
	calls []bool
}

func (u *testUI) Native() bool          { return true }
func (u *testUI) WindowHandle() uintptr { return 0x1234 }
func (u *testUI) IsFullscreen() bool    { u.mu.Lock(); defer u.mu.Unlock(); return u.full }
func (u *testUI) SetFullscreen(on bool) {
	u.mu.Lock()
	u.full = on
	u.calls = append(u.calls, on)
	u.mu.Unlock()
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 150; i++ {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestSingleWindowSession: the game is drawn inside the NEXUS window (embed request with the
// NEXUS window as owner, never the engine's own full screen); "start full screen" makes the
// NEXUS window full screen; F11 in the game toggles the NEXUS window; after the game the
// window returns to how it was. With "Separate window" the engine gets no embed request.
func TestSingleWindowSession(t *testing.T) {
	app, _ := newTestApp(t, "")
	ui := &testUI{}
	app.UI = ui
	_, f := playing(t, app)
	if !f.req.Embed || f.req.OwnerWindow != 0x1234 || f.req.Fullscreen {
		t.Fatalf("request: embed %v owner %x fullscreen %v", f.req.Embed, f.req.OwnerWindow, f.req.Fullscreen)
	}
	if !ui.IsFullscreen() {
		t.Fatal("start full screen: the NEXUS window did not go full screen")
	}
	waitUntil(t, "embedded state", func() bool { return app.Emu.State().Embedded })
	f.PressFullscreen()
	waitUntil(t, "F11 to window", func() bool { return !ui.IsFullscreen() })
	f.PressFullscreen()
	waitUntil(t, "F11 to full screen", func() bool { return ui.IsFullscreen() })
	app.Emu.Stop(true)
	waitUntil(t, "session end", func() bool { return !app.Emu.State().Running })
	waitUntil(t, "window restored", func() bool { return !ui.IsFullscreen() })

	// separate window: as before 0.5
	app.Settings.Update(func(s *GlobalSettings) { s.Emulator.GameWindow = "separate" })
	g, _ := app.Library.Get("t-0100aaaa00010000")
	p := app.Profiles.List()[0]
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "second session", func() bool { return fakeLast.Status().State != "" })
	if fakeLast.req.Embed || !fakeLast.req.Fullscreen {
		t.Fatalf("separate window: embed %v fullscreen %v", fakeLast.req.Embed, fakeLast.req.Fullscreen)
	}
	app.Emu.Stop(true)
	waitUntil(t, "session end", func() bool { return !app.Emu.State().Running })
}

// TestBackgroundSessionPausedAfterLimit: a game left running in the background is paused after
// the limit (so it stops using the PC), and Resume still works afterwards.
func TestBackgroundSessionPausedAfterLimit(t *testing.T) {
	testBackgroundLimit = 400 * time.Millisecond
	defer func() { testBackgroundLimit = 0 }()
	app, _ := newTestApp(t, "")
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	_, f := playing(t, app)
	if err := app.Emu.Suspend(true); err != nil {
		t.Fatal(err)
	}
	if st := f.Status(); !st.Suspended || !st.Background {
		t.Fatalf("not in the background: %+v", st)
	}
	if waitEvent(t, ev, "emulator.autopaused") == nil {
		t.Fatal("not paused after the limit")
	}
	if st := f.Status(); !st.Suspended || st.Background || f.volume == 0 {
		t.Fatalf("after the limit: %+v volume %d", st, f.volume)
	}
	if err := app.Emu.Suspend(false); err != nil || f.Status().Suspended {
		t.Fatal("resume after the automatic pause", err)
	}
	app.Emu.Stop(true)
}
