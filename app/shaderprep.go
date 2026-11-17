package main

// Shader preparation (docs/SHADERS.md).
//
// What NEXUS prepares before a game plays, with engine 0.8 (Lichii build 316 + NEXUS patches):
//
//  1. Discovery from the game's own files: the engine's shader precache builder reads the game
//     (the user's copy and keys), finds the shader programs in its shader archives and converts
//     the compute programs into the title's precache file (stages "Reading game files",
//     "Extracting shaders", "Converting", "Writing"). Done once per game and engine build;
//     current files are skipped at once.
//  2. Compilation before the game starts: with precache_shaders = Always the engine builds those
//     compute programs (SPIR-V translation and driver compilation) at boot, before the game runs
//     (load stage "precache"), once per driver; "Start now" skips the rest.
//  3. The game's recorded pipelines (graphics and compute of earlier sessions, on this PC or
//     brought from another device): built at Loading Shaders (stage "shaders").
//
// What stays for gameplay: graphics pipelines the game has never drawn on any of your devices.
// Their render state is not in the game files (Fritchii predicts it with recorded sessions and
// per-game recipes; NEXUS does not have those yet). They are compiled when first drawn and kept
// in the cache, so each happens once.

import (
	"errors"
	"sync"
	"time"
)

type shaderPrep struct {
	mu       sync.Mutex
	key      string // game being prepared, "" when none
	cancel   bool
	lastByID map[string]PrepStatus // last result per title id, for the interface
}

// prepEvent is what the interface receives ("shaders.prepare") while a game is prepared on Home.
type prepEvent struct {
	GameKey string `json:"gameKey"`
	Title   string `json:"title"`
	PrepStatus
}

func (e *Emulator) preparer() (ShaderPreparer, EngineHost, bool) {
	host, err := e.Host()
	if err != nil {
		return nil, nil, false
	}
	sp, ok := host.(ShaderPreparer)
	return sp, host, ok
}

// runPrep starts a preparation and follows it until it ends, calling progress with each change.
// The final status is returned; cancel() (Stop on the loading screen, or the menu) stops it.
func (e *Emulator) runPrep(sp ShaderPreparer, host EngineHost, req PrepRequest, key string, progress func(PrepStatus)) (PrepStatus, error) {
	e.prep.mu.Lock()
	if e.prep.key != "" {
		e.prep.mu.Unlock()
		return PrepStatus{}, errors.New("Shaders are already being prepared for another game.")
	}
	e.prep.key, e.prep.cancel = key, false
	e.prep.mu.Unlock()
	defer func() {
		e.prep.mu.Lock()
		e.prep.key = ""
		e.prep.mu.Unlock()
	}()
	switch sp.PrepareShaders(req) {
	case 0:
	case -1:
		return PrepStatus{}, errors.New("The engine is busy with a game.")
	case -3:
		return PrepStatus{}, errors.New("This engine build cannot prepare shaders.")
	default:
		return PrepStatus{}, errors.New("The engine refused the preparation request.")
	}
	var last PrepStatus
	cancelled := false
	for {
		time.Sleep(150 * time.Millisecond)
		e.prep.mu.Lock()
		if e.prep.cancel && !cancelled {
			cancelled = true
			sp.CancelPrepare()
		}
		e.prep.mu.Unlock()
		es := host.Status()
		if es.Prepare == nil || es.Prepare.TitleID != req.TitleID {
			continue
		}
		p := *es.Prepare
		if p != last {
			last = p
			if progress != nil {
				progress(p)
			}
		}
		if p.State != "running" {
			e.prep.mu.Lock()
			if e.prep.lastByID == nil {
				e.prep.lastByID = map[string]PrepStatus{}
			}
			e.prep.lastByID[req.TitleID] = p
			e.prep.mu.Unlock()
			e.app.Log.Printf("shaders: preparation for %s %s in %.1f s (%d archives, %d compute programs, %d skipped)%s",
				req.TitleID, p.State, p.Seconds, p.Archives, p.Programs, p.Skipped, map[bool]string{true: ": " + p.Error, false: ""}[p.Error != ""])
			return p, nil
		}
	}
}

// cancelPrep stops a running preparation (the loading screen's cancel, or the game's menu).
func (e *Emulator) cancelPrep() bool {
	e.prep.mu.Lock()
	defer e.prep.mu.Unlock()
	if e.prep.key == "" {
		return false
	}
	e.prep.cancel = true
	return true
}

func (e *Emulator) prepRequest(g *Game, cfg string, force bool) PrepRequest {
	return PrepRequest{Game: g.Path, DataDir: e.DataDir(), ConfigFile: cfg, TitleID: g.TitleID, Contents: e.extraContents(g), Force: force}
}

// PrepareShadersNow prepares a game's shaders on Home, in the background (the game's menu).
// force builds the precache file again although it is current.
func (e *Emulator) PrepareShadersNow(g *Game, force bool) error {
	if g == nil || g.TitleID == "" {
		return errors.New("This game has no title ID to prepare shaders for.")
	}
	if e.State().Running {
		return errors.New("Close the running game first: shaders are prepared while no game runs.")
	}
	sp, host, ok := e.preparer()
	if !ok {
		return errors.New("This engine build cannot prepare shaders.")
	}
	cfg, err := e.writeSessionConfig(g, nil)
	if err != nil {
		return err
	}
	req := e.prepRequest(g, cfg, force)
	emit := func(p PrepStatus) {
		e.app.Events.Emit("shaders.prepare", prepEvent{GameKey: g.Key, Title: g.Title, PrepStatus: p})
	}
	emit(PrepStatus{State: "running", TitleID: g.TitleID})
	go func() {
		final, err := e.runPrep(sp, host, req, g.Key, emit)
		if err != nil {
			emit(PrepStatus{State: "failed", TitleID: g.TitleID, Error: err.Error()})
			return
		}
		emit(final)
	}()
	return nil
}

// PreparingKey is the game being prepared on Home ("" when none).
func (e *Emulator) PreparingKey() string {
	e.prep.mu.Lock()
	defer e.prep.mu.Unlock()
	return e.prep.key
}

// LastPrep is the last preparation result for a title, if any this session.
func (e *Emulator) LastPrep(titleID string) (PrepStatus, bool) {
	e.prep.mu.Lock()
	defer e.prep.mu.Unlock()
	p, ok := e.prep.lastByID[titleID]
	return p, ok
}

// SkipPrecache: the loading screen's "Start now" while the boot precache runs.
func (e *Emulator) SkipPrecache() {
	if sp, _, ok := e.preparer(); ok {
		sp.SkipPrecache()
	}
}

// prepareThenStart is Launch with shader preparation first: the loading screen shows the
// preparation's stages, then the engine starts. A failed preparation does not stop the game (it
// only means more shaders are compiled while playing); a cancelled one ends the launch.
func (e *Emulator) prepareThenStart(host EngineHost, sp ShaderPreparer, g *Game, req EngineRequest, st RunState) {
	final, err := e.runPrep(sp, host, e.prepRequest(g, req.ConfigFile, false), g.Key, func(p PrepStatus) {
		if !e.State().Running { // cancelled meanwhile
			return
		}
		st.Stage, st.Prep, st.LoadDone, st.LoadTotal = "prepare", &p, p.Done, p.Total
		e.setState(st)
	})
	if final.State == "stopped" || !e.State().Running {
		e.app.Log.Printf("engine: launch of %s cancelled while preparing shaders", g.Title)
		e.setState(RunState{Volume: e.Volume()})
		e.app.UI.Restore()
		e.app.Events.Emit("emulator.exit", map[string]any{"gameKey": st.GameKey, "title": st.Title, "seconds": 0, "mode": "game", "emulator": engineID, "cancelled": true})
		return
	}
	if err != nil || final.State == "failed" {
		msg := firstNonEmpty(final.Error, errString(err))
		e.app.Log.Printf("shaders: preparation failed (%s); starting the game without it", msg)
		e.app.Events.Emit("launch.warning", map[string]any{"message": "Shaders could not be prepared before the game (" + msg + "). The game starts; new shaders are compiled while you play."})
	}
	st.Stage, st.Prep, st.LoadDone, st.LoadTotal = "preparing", nil, 0, 0
	if err := e.startEngine(host, req, st); err != nil {
		e.setState(RunState{Volume: e.Volume()})
		e.app.UI.Restore()
		e.app.Events.Emit("emulator.exit", map[string]any{"gameKey": st.GameKey, "title": st.Title, "seconds": 0, "mode": "game", "emulator": engineID,
			"error": &LaunchError{Code: "start-failed", Message: "The NEXUS engine could not start the game.", Detail: err.Error()}})
	}
}
