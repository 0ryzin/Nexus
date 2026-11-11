package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHomePressNeverFlips: one Home press suspends and shows NEXUS Home; further presses while
// NEXUS is showing never resume the game (no flickering between the game and NEXUS); only
// Resume Game resumes, after which Home works again. The game window waits for the loading
// screen to fade out (reveal); screenshots land in the Media library with their game.
func TestHomePressNeverFlips(t *testing.T) {
	app, _ := newTestApp(t, "")
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	p, _ := app.Profiles.Create("Kai", Avatar{Kind: "glyph"})
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	req := fakeLast.req
	if !req.RevealOnRequest || req.ScreenshotDir != app.Media.ScreenshotsDir() || req.ScreenshotName != "AQ" || req.FullscreenKey != "F11" || req.ScreenshotKey != "F12" || !req.CaptureButton || req.FadeInMs != 300 {
		t.Fatalf("request %+v", req)
	}
	if waitEvent(t, ev, "launch.reveal") == nil {
		t.Fatal("no reveal request for the loading screen")
	}
	if fakeLast.Status().Shown {
		t.Fatal("the game window appeared before the loading screen faded out")
	}
	app.Emu.mu.Lock()
	h := app.Emu.host
	app.Emu.mu.Unlock()
	h.Reveal()
	for i := 0; i < 50 && !fakeLast.Status().Shown; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if !fakeLast.Status().Shown {
		t.Fatal("not shown after reveal")
	}
	fakeLast.PressHome()
	if waitEvent(t, ev, "emulator.home") == nil || !app.Emu.State().Suspended {
		t.Fatal("Home did not suspend")
	}
	for i := 0; i < 4; i++ { // presses while NEXUS shows (and the engine's own count, repeated)
		time.Sleep(550 * time.Millisecond)
		fakeLast.PressHome()
		fakeLast.set(func(s *EngineStatus) { s.HomePresses++ })
	}
	time.Sleep(400 * time.Millisecond)
	if !app.Emu.State().Suspended {
		t.Fatal("a Home press while NEXUS was showing switched back to the game")
	}
	app.Emu.Suspend(false)
	time.Sleep(600 * time.Millisecond)
	if app.Emu.State().Suspended {
		t.Fatal("Resume Game did not resume")
	}
	fakeLast.PressHome()
	time.Sleep(600 * time.Millisecond)
	if !app.Emu.State().Suspended {
		t.Fatal("Home did not work after resuming")
	}
	app.Emu.Suspend(false)
	time.Sleep(400 * time.Millisecond)
	if err := fakeLast.PressScreenshot(); err != nil {
		t.Fatal(err)
	}
	if waitEvent(t, ev, "media.added") == nil {
		t.Fatal("screenshot not added")
	}
	items := app.Media.List()
	if len(items) != 1 || items[0].GameKey != g.Key || items[0].Title != "Alpha Quest" || items[0].ProfileID != p.ID || items[0].Type != "screenshot" ||
		!strings.HasPrefix(items[0].Name, "AQ 20") {
		t.Fatalf("media %+v", items)
	}
	app.Emu.Stop(false)
	waitIdle(t, app)
}

// TestMediaLibraryFollowsFolders: the library is what is on disk; files added by hand are
// matched to their game by name; deleted/moved files disappear; paths outside are refused.
func TestMediaLibraryFollowsFolders(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	writeSave(t, filepath.Join(app.Media.ScreenshotsDir(), "AQ 2026-11-11 10-00-00.png"), "png")
	writeSave(t, filepath.Join(app.Media.RecordingsDir(), "AQ 2026-11-11 10-05-00.mp4"), "mp4")
	writeSave(t, filepath.Join(app.Media.RecordingsDir(), "notes.txt"), "not media")
	writeSave(t, filepath.Join(app.Media.ScreenshotsDir(), "half.png.part"), "x")
	items := app.Media.List()
	if len(items) != 2 {
		t.Fatalf("%+v", items)
	}
	for _, it := range items {
		if it.GameKey != "t-0100aaaa00010000" {
			t.Fatalf("not matched to its game: %+v", it)
		}
	}
	if items[0].Type == items[1].Type {
		t.Fatal("screenshot and recording types")
	}
	app.Media.Record(filepath.Join(app.Media.ScreenshotsDir(), "AQ 2026-11-11 10-00-00.png"), MediaInfo{GameKey: "t-x", Title: "Recorded"})
	os.Remove(filepath.Join(app.Media.ScreenshotsDir(), "AQ 2026-11-11 10-00-00.png")) // deleted in Explorer
	if items := app.Media.List(); len(items) != 1 || items[0].Type != "recording" {
		t.Fatalf("after outside delete %+v", items)
	}
	if err := app.Media.Delete("Recordings/AQ 2026-11-11 10-05-00.mp4"); err != nil {
		t.Fatal(err)
	}
	if len(app.Media.List()) != 0 {
		t.Fatal("delete")
	}
	for _, bad := range []string{"../System/x", "Screenshots/../../x", `C:\x`, "Other/x.png"} {
		if _, err := app.Media.Path(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

// TestControllersRemembered: controllers are remembered with their type, chosen per user in
// player order, and passed to the engine; a forgotten controller is gone; a controller's own
// layout applies when it is player 1.
func TestControllersRemembered(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	tom, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	list, errText := app.Controllers.List(tom.ID)
	if errText != "" || len(list) != 2 || list[0].Label == "" || list[0].Player != 0 {
		t.Fatalf("%+v %s", list, errText)
	}
	var xbox, ps string
	for _, p := range list {
		if p.Type == "xboxone" {
			xbox = p.Key
		}
		if p.Type == "ps5" {
			ps = p.Key
			if p.Label != "PlayStation controller (DualSense)" {
				t.Fatal(p.Label)
			}
		}
	}
	if err := app.Controllers.SetPlayers(tom.ID, []string{ps, xbox}); err != nil {
		t.Fatal(err)
	}
	pads := app.Controllers.ForSession(&tom)
	if len(pads) != 2 || pads[0].GUID != "050000004c050000e60c000000006801" {
		t.Fatalf("session pads %+v", pads)
	}
	// remembered after a restart, and while disconnected
	t.Setenv("NEXUS_FAKE_PADS", `[{"guid":"030000005e040000e002000000007801","name":"Xbox Wireless Controller","type":"xboxone","vendor":1118,"product":736,"serial":"XB-0001"}]`)
	app2, _ := testNewApp(t, app.Paths.Root)
	l2, _ := app2.Controllers.List(tom.ID)
	if len(l2) != 2 || l2[0].Key != ps || l2[0].Connected || l2[0].Player != 1 || !l2[1].Connected {
		t.Fatalf("after restart %+v", l2)
	}
	if p := app2.Controllers.ForSession(&tom); len(p) != 1 || p[0].GUID != "030000005e040000e002000000007801" {
		t.Fatalf("only connected chosen pads go to the engine: %+v", p)
	}
	// a controller's own layout: used while it is the first connected chosen controller
	app2.Controllers.Update(xbox, nil, &GameLayout{Base: "xbox"}, false)
	if l, src, _ := app2.Emu.EffectiveLayout(nil, &tom); src != "controller" || l.Base != "xbox" {
		t.Fatalf("layout %v %s", l, src)
	}
	if err := app2.Controllers.Forget(xbox); err != nil {
		t.Fatal(err)
	}
	l3, _ := app2.Controllers.List(tom.ID)
	for _, p := range l3 {
		if p.Key == xbox && p.Layout != nil {
			t.Fatal("forgotten controller kept its layout")
		}
	}
}
