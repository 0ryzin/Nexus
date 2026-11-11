package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Emulator is NEXUS's game-session manager. It prepares the session configuration, starts the
// game inside the NEXUS engine (a library loaded into this process, see enginehost.go), tracks
// play time from the engine's own state and returns to the home screen when the game ends.
// It never starts an emulator program.

type RunState struct {
	Running   bool   `json:"running"`
	Phase     string `json:"phase"` // starting | playing | stopping | ""
	GameKey   string `json:"gameKey,omitempty"`
	Title     string `json:"title,omitempty"`
	ProfileID string `json:"profileId,omitempty"`
	Started   int64  `json:"started,omitempty"`
	Mode      string `json:"mode,omitempty"` // game
	Emulator  string `json:"emulator,omitempty"`
	EmuName   string `json:"emuName,omitempty"`
	Volume    int    `json:"volume"`
	// Suspended: the game is paused and hidden while NEXUS Home is shown (session kept).
	Suspended bool `json:"suspended,omitempty"`
	// Stage, while starting: preparing (NEXUS writes the session) | loading (the engine reads the
	// game) | starting (the game runs, first frame not shown yet). Shown on the loading screen.
	Stage string `json:"stage,omitempty"`
}

const engineID = "nexus"

type Emulator struct {
	app *App
	mu  sync.Mutex

	state   RunState
	host    EngineHost
	hostErr error
	hostDir string

	Builds *EngineBuilds

	// startupCheck is set while the first engine check (verify + load) runs in the background
	// after NEXUS starts; the interface does not wait for it (Status reports "checking").
	startupCheck atomic.Bool
}

// CheckEngineInBackground verifies and loads the engine without holding up the interface.
func (e *Emulator) CheckEngineInBackground(done func(ok bool, msg string)) {
	e.startupCheck.Store(true)
	go func() {
		ok, msg := e.EngineAvailable()
		e.startupCheck.Store(false)
		if done != nil {
			done(ok, msg)
		}
	}()
}

func NewEmulator(a *App) *Emulator {
	e := &Emulator{app: a}
	e.Builds = NewEngineBuilds(e)
	return e
}

// ---------- paths

// DataDir is the engine's own data folder: keys, firmware, emulated NAND (saves), caches,
// configuration and logs. It belongs to NEXUS (%APPDATA%\NEXUS\Engine), separate from the
// engine program files.
func (e *Emulator) DataDir() string { return filepath.Join(e.app.Paths.System, "Engine") }

// UserDir is kept as the name the rest of NEXUS uses for the engine data folder.
func (e *Emulator) UserDir() string { return e.DataDir() }

func (e *Emulator) ConfigDir() string   { return filepath.Join(e.DataDir(), "config") }
func (e *Emulator) ConfigPath() string  { return filepath.Join(e.ConfigDir(), "nexus-engine.ini") }
func (e *Emulator) SessionPath() string { return filepath.Join(e.ConfigDir(), "session.ini") }
func (e *Emulator) KeysDir() string     { return filepath.Join(e.DataDir(), "keys") }
func (e *Emulator) FirmwareDir() string {
	return filepath.Join(e.DataDir(), "nand", "system", "Contents", "registered")
}
func (e *Emulator) KeysPresent() bool     { return fileExists(filepath.Join(e.KeysDir(), "prod.keys")) }
func (e *Emulator) FirmwarePresent() bool { return dirHasFiles(e.FirmwareDir()) }

// InputProfiles lists controller profiles saved in the engine configuration.
func (e *Emulator) InputProfiles() []string {
	entries, _ := os.ReadDir(filepath.Join(e.ConfigDir(), "input"))
	out := []string{}
	for _, en := range entries {
		if strings.HasSuffix(strings.ToLower(en.Name()), ".ini") {
			out = append(out, strings.TrimSuffix(en.Name(), filepath.Ext(en.Name())))
		}
	}
	return out
}

// CurrentEmuValues reads the global engine configuration NEXUS last wrote.
func (e *Emulator) CurrentEmuValues() map[string]string {
	out := map[string]string{}
	ini, err := LoadIni(e.ConfigPath())
	if err != nil {
		return out
	}
	for _, o := range EmuOptions {
		sec, key := splitOpt(o.ID)
		if v, ok := ini.GetSetting(sec, key); ok {
			out[o.ID] = v
		}
	}
	return out
}

// ---------- state

func (e *Emulator) State() RunState {
	e.mu.Lock()
	st := e.state
	e.mu.Unlock()
	if !st.Running {
		st.Volume = e.Volume()
	}
	return st
}

func (e *Emulator) setState(s RunState) {
	e.mu.Lock()
	e.state = s
	e.mu.Unlock()
	e.app.Events.Emit("emulator.state", s)
}

// Host loads the active engine build once (in-process).
func (e *Emulator) Host() (EngineHost, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	dir := e.Builds.ActiveDir()
	if e.host != nil && e.hostDir == dir {
		return e.host, nil
	}
	if e.host != nil && e.hostDir != dir {
		// A different build was activated: Windows can't unload a DLL safely while its threads
		// may exist, so the new build is used after NEXUS restarts.
		return e.host, nil
	}
	h, err := loadEngineHost(dir)
	if err != nil {
		e.hostErr = err
		return nil, err
	}
	e.host, e.hostDir, e.hostErr = h, dir, nil
	return h, nil
}

// EngineAvailable says whether a game can be started (an engine build is installed and loads).
func (e *Emulator) EngineAvailable() (bool, string) {
	if !fakeEngineEnabled() {
		if !e.Builds.HasActive() {
			return false, "The NEXUS engine is missing from the NEXUS folder (Engine\\nexus_engine.dll). Extract the complete NEXUS ZIP again, keeping all folders together."
		}
		if err := e.Builds.VerifyActive(); err != nil {
			return false, "The NEXUS engine files are damaged or incomplete: " + err.Error() + " Extract the complete NEXUS ZIP again."
		}
	}
	if _, err := e.Host(); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// ---------- session configuration

// writeSessionConfig builds the configuration the engine reads for this launch: the global
// NEXUS settings, the game's own settings on top, the user's save folder and controller profile.
func (e *Emulator) writeSessionConfig(g *Game, pr *Profile) (string, error) {
	s := e.app.Settings.Get()
	if err := os.MkdirAll(e.ConfigDir(), 0o755); err != nil {
		return "", err
	}
	// global file (kept so "current engine values" can be shown)
	gini, err := LoadIni(e.ConfigPath())
	if err != nil {
		return "", err
	}
	if !fileExists(e.ConfigPath()) {
		seedNewConfig(gini)
	}
	for id, v := range engineProject(s.Engine, false) {
		sec, key := splitOpt(id)
		gini.SetSetting(sec, key, v)
	}
	if err := gini.Save(e.ConfigPath()); err != nil {
		return "", err
	}
	// session file = global + game + user
	data, _ := os.ReadFile(e.ConfigPath())
	if err := os.WriteFile(e.SessionPath(), data, 0o644); err != nil {
		return "", err
	}
	ini, err := LoadIni(e.SessionPath())
	if err != nil {
		return "", err
	}
	if g != nil {
		p := e.app.Library.GP.Get(g.Key)
		for id, v := range engineProject(gameNexusSettings(p), true) {
			sec, key := splitOpt(id)
			ini.SetSetting(sec, key, v)
		}
		if input := e.inputProfileFor(g, pr); input != "" {
			ini.Set("Controls", "player_0_profile_name", input)
		}
	}
	if s.Emulator.ManageSaves && pr != nil {
		dir := e.app.Profiles.SaveDir(pr.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("cannot create the user's save folder: %w", err)
		}
		abs, _ := filepath.Abs(dir)
		ini.Set("Data%20Storage", "save_directory", iniPath(abs))
	}
	return e.SessionPath(), ini.Save(e.SessionPath())
}

// extraContents lists update and DLC files in the library that belong to g.
func (e *Emulator) extraContents(g *Game) []string {
	base, err := strconv.ParseUint(g.TitleID, 16, 64)
	if err != nil || g.TitleID == "" {
		return nil
	}
	var out []string
	for _, o := range e.app.Library.Games() {
		if o.Kind != "update" && o.Kind != "dlc" || o.TitleID == "" {
			continue
		}
		id, err := strconv.ParseUint(o.FullID, 16, 64)
		if err != nil {
			id, err = strconv.ParseUint(o.TitleID, 16, 64)
		}
		if err != nil {
			continue
		}
		if b, _ := classifyID(id); b == base&^0x1FFF {
			out = append(out, o.Path)
		}
	}
	return out
}

func (e *Emulator) inputProfileFor(g *Game, pr *Profile) string {
	if g != nil {
		if p := e.app.Library.GP.Get(g.Key).InputProfile; p != "" {
			return p
		}
	}
	if pr != nil {
		return pr.Controls.EmuInput
	}
	return ""
}

// ---------- launching

// Preflight returns blocking errors and warnings for launching g.
func (e *Emulator) Preflight(g *Game) (*LaunchError, []LaunchError) {
	if ok, why := e.EngineAvailable(); !ok {
		return &LaunchError{Code: "no-engine", Message: "The NEXUS engine is not installed.", Detail: why}, nil
	}
	if g != nil && !fileExists(g.Path) {
		return &LaunchError{Code: "missing-file", Message: "This game file is no longer available.",
			Detail: g.Path + " could not be found. Was the drive disconnected or the file moved?"}, nil
	}
	if _, ok := SupportedFormats[strings.TrimPrefix(strings.ToLower(filepath.Ext(g.Path)), ".")]; g != nil && !ok {
		return &LaunchError{Code: "format", Message: "NEXUS cannot open this file type.", Detail: g.Path}, nil
	}
	var warns []LaunchError
	if !vcRuntimeAvailable(e.Builds.ActiveDir()) {
		warns = append(warns, LaunchError{Code: "vc-runtime", Message: "Microsoft Visual C++ Runtime not found.",
			Detail: "The NEXUS engine needs the Visual C++ 2015-2022 x64 runtime. It normally ships in NEXUS\\Engine\\Libraries; if that folder is incomplete, install it from Microsoft: https://aka.ms/vs/17/release/vc_redist.x64.exe"})
	}
	if !vulkanLoaderPresent() {
		warns = append(warns, LaunchError{Code: "no-vulkan", Message: "No Vulkan graphics driver found.",
			Detail: "Install the latest driver for your graphics card. Without Vulkan, set Graphics backend to OpenGL in Emulation settings."})
	}
	if g != nil && g.Kind != "homebrew" {
		if !e.KeysPresent() {
			warns = append(warns, LaunchError{Code: "no-keys", Message: "No prod.keys installed.",
				Detail: "Commercial games need the keys from your own console. Install them in Settings › System files."})
		}
		if !e.FirmwarePresent() {
			warns = append(warns, LaunchError{Code: "no-firmware", Message: "No firmware installed.",
				Detail: "Many games need firmware from your own console. Install it in Settings › System files."})
		}
	}
	if g != nil && (g.Kind == "update" || g.Kind == "dlc") {
		warns = append(warns, LaunchError{Code: "not-base", Message: "This file is an update or DLC, not a game.",
			Detail: "Launch the base game; updates and DLC in your game folders are applied automatically."})
	}
	return nil, warns
}

func (e *Emulator) Launch(g *Game, profileID string) error {
	if g == nil {
		return &LaunchError{Code: "no-game", Message: "Choose a game to play."}
	}
	if st := e.State(); st.Running {
		return &LaunchError{Code: "busy", Message: "A game is already running.", Detail: st.Title}
	}
	if fatal, _ := e.Preflight(g); fatal != nil {
		return fatal
	}
	host, err := e.Host()
	if err != nil {
		return &LaunchError{Code: "no-engine", Message: "The NEXUS engine could not be loaded.", Detail: err.Error()}
	}
	var pr *Profile
	if p, ok := e.app.Profiles.Get(profileID); ok {
		pr = &p
	}
	cfg, err := e.writeSessionConfig(g, pr)
	if err != nil {
		return &LaunchError{Code: "config", Message: "Could not prepare the game's settings.", Detail: err.Error()}
	}
	s := e.app.Settings.Get()
	vol := e.Volume()
	if v, ok := gameNexusSettings(e.app.Library.GP.Get(g.Key))["audio.volume"]; ok {
		fmt.Sscanf(v, "%d", &vol)
	}
	req := EngineRequest{Game: g.Path, DataDir: e.DataDir(), ConfigFile: cfg, Title: "NEXUS — " + g.Title,
		Fullscreen: s.Emulator.Fullscreen, Volume: vol, OwnerWindow: e.app.UI.WindowHandle(),
		Contents: e.extraContents(g), AutoMapController: e.inputProfileFor(g, pr) == ""}
	if req.AutoMapController {
		layout, _, _ := e.EffectiveLayout(g, pr)
		req.ButtonMap, req.ControllerType = layout.Resolve(), layout.ControllerType
	}
	_ = os.MkdirAll(e.DataDir(), 0o755)
	e.app.Log.Printf("engine: start %s (%s)", g.Title, g.Path)
	manage := s.Emulator.ManageSaves && pr != nil
	if manage {
		// readable save folders -> the title-ID paths the engine uses (moved back afterwards)
		if n, err := e.app.Saves.Assemble(profileID); err != nil {
			e.app.Log.Printf("saves: %v", err)
		} else if n > 0 {
			e.app.Log.Printf("saves: %d game folder(s) ready for the engine", n)
		}
	}
	if err := host.Start(req); err != nil {
		if manage {
			_, _ = e.app.Saves.Organize(profileID)
		}
		return &LaunchError{Code: "start-failed", Message: "The NEXUS engine could not start the game.", Detail: err.Error()}
	}
	st := RunState{Running: true, Phase: "starting", Stage: "preparing", Started: nowUnix(), ProfileID: profileID, Mode: "game",
		GameKey: g.Key, Title: g.Title, Emulator: engineID, EmuName: "NEXUS", Volume: vol}
	e.app.Profiles.RecordLaunch(profileID, g.Key, g.Title)
	e.setState(st)
	go e.monitor(host, st)
	return nil
}

// checkpointEvery is how often an in-progress session is saved so that a crash of NEXUS or of
// the PC loses at most this much play time.
var checkpointEvery = 30 * time.Second

// monitor follows the engine's state. Play time counts only while the engine is running the game
// and presenting frames.
func (e *Emulator) monitor(host EngineHost, st RunState) {
	s := e.app.Settings.Get()
	start := time.Now()
	var playStart, lastCheckpoint time.Time
	var played time.Duration
	var sessionStart int64
	sawGame := false
	gameSystem := ""
	comboHeld := time.Time{}
	guideWas := false
	checkpoint := func() {
		if st.Phase != "playing" {
			return
		}
		lastCheckpoint = time.Now()
		cur := played
		if !st.Suspended {
			cur += time.Since(playStart)
		}
		e.app.Profiles.Checkpoint(&LiveSession{ProfileID: st.ProfileID, Key: st.GameKey, Title: st.Title,
			Emulator: engineID, Start: sessionStart, LastSeen: sessionStart + int64(cur.Seconds())})
	}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var final EngineStatus
	for range tick.C {
		es := host.Status()
		if es.System != "" {
			gameSystem = es.System
		}
		if st.Phase == "starting" {
			stage := map[string]string{"loading": "loading", "running": "starting"}[es.State]
			if stage != "" && stage != st.Stage {
				st.Stage = stage
				e.setState(st)
			}
		}
		switch es.State {
		case "running":
			if es.Frames > 0 && st.Phase != "playing" {
				if !sawGame {
					sessionStart = nowUnix()
				}
				sawGame = true
				playStart = time.Now()
				st.Phase, st.Stage = "playing", ""
				e.setState(st)
				checkpoint()
			}
		case "stopping", "stopped", "error", "idle":
			final = es
		}
		if es.State == "running" && es.Suspended != st.Suspended {
			// NEXUS Home while playing: time spent suspended is not play time
			if es.Suspended && st.Phase == "playing" {
				played += time.Since(playStart)
			} else if !es.Suspended {
				playStart = time.Now()
			}
			st.Suspended = es.Suspended
			e.setState(st)
			if st.Suspended {
				e.app.UI.Restore()
				e.app.Events.Emit("emulator.home", st)
			}
		}
		// the controller's Home/Guide button toggles between the game and NEXUS Home
		g := guidePressed()
		if g && !guideWas && st.Phase == "playing" {
			host.Suspend(!st.Suspended)
		}
		guideWas = g
		if final.State != "" {
			break
		}
		if st.Phase == "playing" && time.Since(lastCheckpoint) >= checkpointEvery {
			checkpoint()
		}
		if s.Emulator.QuitCombo && st.Phase == "playing" {
			if quitComboPressed() {
				if comboHeld.IsZero() {
					comboHeld = time.Now()
				} else if time.Since(comboHeld) > 1600*time.Millisecond {
					comboHeld = time.Time{}
					e.app.Log.Printf("quit combo held: closing game")
					host.Stop(false)
				}
			} else {
				comboHeld = time.Time{}
			}
		}
	}
	if st.Phase == "playing" && !st.Suspended {
		played += time.Since(playStart)
	}
	// wait for the engine to finish shutting the game down
	for i := 0; i < 400 && final.State == "stopping"; i++ {
		time.Sleep(50 * time.Millisecond)
		final = host.Status()
	}
	secs := int64(played.Seconds())
	crashed := final.State == "error"
	if secs > 0 {
		e.app.Profiles.AddSession(st.ProfileID, st.GameKey, st.Title, SessionRec{Start: sessionStart,
			End: sessionStart + secs, Seconds: secs, Emulator: engineID, Crashed: crashed})
	}
	e.app.Profiles.Checkpoint(nil)
	e.setState(RunState{Volume: e.Volume()}) // session over: allows the save organiser to run
	if _, err := e.app.Saves.Organize(st.ProfileID); err != nil {
		e.app.Log.Printf("saves: %v", err)
	}
	e.app.Compat.Record(st.GameKey, st.Title, gameSystem, sawGame, crashed, secs, final.Code)
	e.app.Log.Printf("engine: session ended state=%s code=%s after %s (played %ds)", final.State, final.Code,
		time.Since(start).Round(time.Second), secs)
	result := map[string]any{"gameKey": st.GameKey, "title": st.Title, "seconds": secs, "mode": "game", "emulator": engineID}
	if crashed {
		tail := tailFile(filepath.Join(e.DataDir(), "log", "eden_log.txt"), 12, func(l string) bool {
			return strings.Contains(l, "<Error>") || strings.Contains(l, "<Critical>")
		})
		le := nexusLaunchError(final.Code, final.Message, sawGame)
		le.Technical = strings.TrimSpace(fmt.Sprintf("Engine code: %s\nEngine message: %s\n\n%s", firstNonEmpty(final.Code, "none"), final.Message, tail))
		le.Log = e.writeDiagnostic(st, le, tail)
		result["error"] = le
	}
	e.setState(RunState{Volume: e.Volume()})
	e.app.UI.Restore()
	e.app.Events.Emit("emulator.exit", result)
	e.app.Events.Emit("stats.changed", map[string]any{"profileId": st.ProfileID, "gameKey": st.GameKey})
	go func() {
		e.app.Avatars.Ensure(false)
		e.app.Events.Emit("library.changed", nil)
	}()
}

func (e *Emulator) Stop(force bool) error {
	if !e.State().Running {
		return errors.New("Nothing is running.")
	}
	h, err := e.Host()
	if err != nil {
		return err
	}
	h.Stop(force)
	return nil
}

// Suspend shows NEXUS Home while the game stays loaded (on=true), or returns to the game.
func (e *Emulator) Suspend(on bool) error {
	st := e.State()
	if !st.Running {
		return errors.New("Nothing is running.")
	}
	h, err := e.Host()
	if err != nil {
		return err
	}
	h.Suspend(on)
	if !on {
		h.Focus()
	}
	return nil
}

func (e *Emulator) FocusRunning() {
	if h, err := e.Host(); err == nil && e.State().Running {
		h.Focus()
	}
}

// ---------- volume (0-200 %)

// Volume is the global NEXUS volume in percent (100 = reference level).
func (e *Emulator) Volume() int {
	v := 100
	if s, ok := e.app.Settings.Get().Engine["audio.volume"]; ok {
		fmt.Sscanf(s, "%d", &v)
	}
	return max(0, min(200, v))
}

// SetVolume stores the volume and, when a game is running, applies it to the engine at once.
func (e *Emulator) SetVolume(p int) error {
	p = max(0, min(200, p))
	val := fmt.Sprint(p)
	if err := e.SetGlobal(map[string]*string{"audio.volume": &val}); err != nil {
		return err
	}
	if st := e.State(); st.Running {
		if h, err := e.Host(); err == nil {
			h.SetVolume(p)
		}
		st.Volume = p
		e.setState(st)
	}
	return nil
}

// ---------- saves from an earlier emulator installation

// PreviousSaveDirs are save folders of emulators the user may have used before NEXUS, offered
// for a one-time copy into a NEXUS user (never moved or deleted).
func (e *Emulator) PreviousSaveDirs() []string {
	var out []string
	// the emulator imported from first, then any other emulator found on this PC
	imported := e.readImportRecord().Source
	srcs := e.LegacySources()
	sort.SliceStable(srcs, func(i, j int) bool { return srcs[i].ID == imported && srcs[j].ID != imported })
	for _, s := range srcs {
		if s.ID != "ryujinx" {
			out = append(out, filepath.Join(s.Path, "nand", "user", "save"))
		}
	}
	out = append(out, filepath.Join(e.app.Paths.Root, "Emulator", "Lichii", "user", "nand", "user", "save"),
		filepath.Join(e.DataDir(), "nand", "user", "save"))
	return out
}

func (e *Emulator) NandSaveDir() string {
	for _, d := range e.PreviousSaveDirs() {
		if dirHasFiles(d) {
			return d
		}
	}
	return ""
}

func (e *Emulator) ImportNandSaves(profileID string, overwrite bool) (int, error) {
	src := e.NandSaveDir()
	if src == "" {
		return 0, errors.New("No existing saves were found to import.")
	}
	dst := filepath.Join(e.app.Profiles.SaveDir(profileID), "user", "save")
	if !overwrite && dirHasFiles(dst) {
		conflict := ""
		_ = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(src, p)
				if fileExists(filepath.Join(dst, rel)) {
					conflict = rel
					return io.EOF
				}
			}
			return nil
		})
		if conflict != "" {
			return 0, fmt.Errorf("%w (%s). Confirm overwrite to replace this profile's copy.", errExists, conflict)
		}
	}
	if overwrite && dirHasFiles(dst) {
		bak := filepath.Join(e.app.Paths.Trash, "save-backups", profileID+"-"+time.Now().Format("20060102-150405"))
		if _, err := copyTree(dst, bak, false); err != nil {
			return 0, fmt.Errorf("backup before overwrite failed, nothing changed: %w", err)
		}
	}
	n, err := copyTree(src, dst, overwrite)
	if err == nil {
		_, _ = e.app.Saves.Organize(profileID) // into readable game folders (never overwriting)
	}
	return n, err
}

// Status summarises the engine for the UI.
func (e *Emulator) Status() map[string]any {
	checking := e.startupCheck.Load()
	ok, why := false, ""
	if !checking {
		ok, why = e.EngineAvailable()
	}
	return map[string]any{
		"installed": ok, "problem": why, "checking": checking, "dataDir": e.DataDir(), "keys": e.KeysPresent(),
		"firmware": e.FirmwarePresent(), "vcRuntime": vcRuntimeAvailable(e.Builds.ActiveDir()), "vulkan": vulkanLoaderPresent(), "state": e.State(),
		"inputProfiles": e.InputProfiles(), "volume": e.Volume(),
	}
}

// nexusLaunchError turns what the engine reports into a NEXUS message. The engine's own text
// (which may name upstream components) is kept for the technical details and the diagnostic log.
func nexusLaunchError(code, engineMsg string, started bool) *LaunchError {
	le := &LaunchError{Code: firstNonEmpty(code, "crash")}
	switch code {
	case "unsupported-file", "memory", "cpu", "gpu", "load":
		// these messages are written by the NEXUS engine itself
		le.Message = firstNonEmpty(engineMsg, "The game could not be started.")
	default:
		if started {
			le.Message = "The game stopped unexpectedly."
			le.Detail = "The NEXUS engine stopped while the game was running. Your play time up to this point was saved. Try again; if it keeps happening, check the game's settings (try Restore defaults) and the diagnostic log."
		} else {
			le.Message = "NEXUS could not start this game."
			le.Detail = "The NEXUS engine could not prepare the game. Check that your keys and firmware are installed (Settings › System files) and that the game file is complete. The diagnostic log has the technical details."
		}
	}
	return le
}

// writeDiagnostic writes a diagnostic log for a failed or crashed session and returns its path.
func (e *Emulator) writeDiagnostic(st RunState, le *LaunchError, tail string) string {
	dir := filepath.Join(e.app.Paths.Logs, "diagnostics")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	name := "session-" + time.Now().Format("20060102-150405") + ".txt"
	var b strings.Builder
	fmt.Fprintf(&b, "NEXUS %s diagnostic log\n%s\n\n", AppVersion, time.Now().Format(time.RFC1123))
	fmt.Fprintf(&b, "Game: %s (%s)\nUser: %s\nEngine: %s\n\n", st.Title, st.GameKey, st.ProfileID, e.Builds.ActiveVersionLabel())
	fmt.Fprintf(&b, "Shown to the user: %s\n%s\n\n%s\n", le.Message, le.Detail, le.Technical)
	if tail == "" {
		b.WriteString("\n(no error lines in the engine log)\n")
	}
	fmt.Fprintf(&b, "\nFull engine log: %s\n", filepath.Join(e.DataDir(), "log", "eden_log.txt"))
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		return ""
	}
	return p
}
