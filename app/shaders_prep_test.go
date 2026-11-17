package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// prepTestApp: a test app with shader preparation on, one user and the first game.
func prepTestApp(t *testing.T) (*App, *Game, string) {
	t.Helper()
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	app, _ := newTestApp(t, "")
	app.Settings.Update(func(g *GlobalSettings) { g.Emulator.PrepareShaders = true })
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	pr, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	return app, &g, pr.ID
}

// stateLog follows emulator.state events until phase "playing" or the session ends, and returns
// the stages seen, with the preparation's own stages.
func stateLog(t *testing.T, ev chan []byte, until func(RunState) bool) (stages, prepStages []string, last RunState) {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case m := <-ev:
			var e struct {
				Type string
				Data json.RawMessage
			}
			json.Unmarshal(m, &e)
			if e.Type == "emulator.exit" {
				return stages, prepStages, RunState{}
			}
			if e.Type != "emulator.state" {
				continue
			}
			var st RunState
			json.Unmarshal(e.Data, &st)
			if len(stages) == 0 || stages[len(stages)-1] != st.Stage {
				stages = append(stages, st.Stage)
			}
			if st.Prep != nil && st.Prep.Stage != "" && (len(prepStages) == 0 || prepStages[len(prepStages)-1] != st.Prep.Stage) {
				prepStages = append(prepStages, st.Prep.Stage)
			}
			if until(st) {
				return stages, prepStages, st
			}
		case <-deadline:
			t.Fatalf("timed out; stages so far %v", stages)
		}
	}
}

func isPlaying(st RunState) bool { return st.Phase == "playing" }

// stopAndDrain ends the session and drops the events it left in ev.
func stopAndDrain(app *App, ev chan []byte) {
	stopTestSession(app)
	time.Sleep(300 * time.Millisecond)
	for {
		select {
		case <-ev:
		default:
			return
		}
	}
}

// TestShaderPreparationBeforeLaunch: the first launch finds and converts the game's shaders
// (the engine's builder stages, in order), then compiles them at boot ("precache") before the
// game plays; the second launch finds the precache file current and the boot precache done, so
// it goes straight to loading. The session config asks the engine to precache.
func TestShaderPreparationBeforeLaunch(t *testing.T) {
	app, g, user := prepTestApp(t)
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	if err := app.Emu.Launch(g, user); err != nil {
		t.Fatal(err)
	}
	stages, prep, _ := stateLog(t, ev, isPlaying)
	if !slices.Equal(prep, []string{"Reading game files", "Extracting shaders", "Converting", "Writing"}) {
		t.Fatalf("preparation stages: %v", prep)
	}
	pi, ci := slices.Index(stages, "prepare"), slices.Index(stages, "precache")
	if pi != 0 || ci < 0 || !slices.Contains(stages[ci:], "playing") && stages[len(stages)-1] != "" {
		t.Fatalf("stages: %v", stages)
	}
	cfg, _ := os.ReadFile(app.Emu.SessionPath())
	if !strings.Contains(string(cfg), "precache_shaders=2") {
		t.Fatal("session config does not ask the engine to precache")
	}
	if p, ok := app.Emu.LastPrep(g.TitleID); !ok || p.State != "done" || p.Programs == 0 {
		t.Fatalf("last preparation: %+v", p)
	}
	stopAndDrain(app, ev)

	if err := app.Emu.Launch(g, user); err != nil {
		t.Fatal(err)
	}
	stages, prep, _ = stateLog(t, ev, isPlaying)
	if len(prep) != 0 || slices.Contains(stages, "precache") {
		t.Fatalf("second launch prepared again: stages %v, preparation %v", stages, prep)
	}
	if p, _ := app.Emu.LastPrep(g.TitleID); p.State != "current" {
		t.Fatalf("second launch: %+v", p)
	}
	stopTestSession(app)
}

// TestShaderPreparationCancel: cancelling on the loading screen while shaders are prepared ends
// the launch without starting the game.
func TestShaderPreparationCancel(t *testing.T) {
	app, g, user := prepTestApp(t)
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	before := fakeLast
	if err := app.Emu.Launch(g, user); err != nil {
		t.Fatal(err)
	}
	stateLog(t, ev, func(st RunState) bool { return st.Prep != nil && st.Prep.Stage == "Extracting shaders" })
	if err := app.Emu.Stop(false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && app.Emu.State().Running; i++ {
		time.Sleep(30 * time.Millisecond)
	}
	if app.Emu.State().Running {
		t.Fatal("still running after cancelling the preparation")
	}
	if fakeLast != before {
		t.Fatal("the engine started the game although the launch was cancelled")
	}
	if p, _ := app.Emu.LastPrep(g.TitleID); p.State != "stopped" {
		t.Fatalf("preparation state %+v", p)
	}
}

// TestShaderPrecacheSkip: "Start now" during the boot precache starts the game; the precache is
// not marked done, so the next launch builds the rest before the game.
func TestShaderPrecacheSkip(t *testing.T) {
	app, g, user := prepTestApp(t)
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	app.Emu.Launch(g, user)
	stateLog(t, ev, func(st RunState) bool { return st.Stage == "precache" })
	app.Emu.SkipPrecache()
	stateLog(t, ev, isPlaying)
	stopAndDrain(app, ev)
	app.Emu.Launch(g, user)
	stages, _, _ := stateLog(t, ev, isPlaying)
	if !slices.Contains(stages, "precache") {
		t.Fatalf("a skipped precache was not resumed at the next launch: %v", stages)
	}
	stopTestSession(app)
}

// TestShaderPreparationOff: with preparation off, launch starts the engine at once and the
// session config does not ask for a precache.
func TestShaderPreparationOff(t *testing.T) {
	app, g, user := prepTestApp(t)
	app.Settings.Update(func(s *GlobalSettings) { s.Emulator.PrepareShaders = false })
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	app.Emu.Launch(g, user)
	stages, prep, _ := stateLog(t, ev, isPlaying)
	if slices.Contains(stages, "prepare") || len(prep) != 0 || slices.Contains(stages, "precache") {
		t.Fatalf("prepared with the setting off: %v %v", stages, prep)
	}
	cfg, _ := os.ReadFile(app.Emu.SessionPath())
	if !strings.Contains(string(cfg), "precache_shaders=0") {
		t.Fatal("session config should switch the precache off")
	}
	stopTestSession(app)
}

// TestPrepareShadersOnHome: "Prepare shaders now" from the game's menu runs in the background,
// reports its stages and result, refuses while a game runs, and makes the next launch skip
// straight to the boot precache.
func TestPrepareShadersOnHome(t *testing.T) {
	app, g, user := prepTestApp(t)
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	if err := app.Emu.PrepareShadersNow(g, false); err != nil {
		t.Fatal(err)
	}
	var states []string
	deadline := time.After(15 * time.Second)
	for done := false; !done; {
		select {
		case m := <-ev:
			if !strings.Contains(string(m), `"type":"shaders.prepare"`) {
				continue
			}
			var e struct{ Data prepEvent }
			json.Unmarshal(m, &e)
			if len(states) == 0 || states[len(states)-1] != e.Data.State {
				states = append(states, e.Data.State)
			}
			done = e.Data.State != "running"
			if done && (e.Data.GameKey != g.Key || e.Data.Programs == 0) {
				t.Fatalf("result: %+v", e.Data)
			}
		case <-deadline:
			t.Fatalf("no result: %v", states)
		}
	}
	if !slices.Equal(states, []string{"running", "done"}) {
		t.Fatalf("states %v", states)
	}
	app.Emu.Launch(g, user)
	stages, prep, _ := stateLog(t, ev, isPlaying)
	if len(prep) != 0 || !slices.Contains(stages, "precache") {
		t.Fatalf("after preparing on Home: stages %v, preparation %v", stages, prep)
	}
	if err := app.Emu.PrepareShadersNow(g, false); err == nil {
		t.Fatal("preparation allowed while a game runs")
	}
	stopTestSession(app)
}
