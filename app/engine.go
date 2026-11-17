package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	// Background: while suspended, the game keeps running (muted, no input) behind NEXUS
	Background bool `json:"background,omitempty"`
	// Fullscreen: the game window is fullscreen (F11 switches)
	Fullscreen bool `json:"fullscreen,omitempty"`
	// Stage, while starting: preparing (NEXUS writes the session) | loading (the engine reads the
	// game) | starting (the game runs, first frame not shown yet). Shown on the loading screen.
	Stage string `json:"stage,omitempty"`
	// LoadDone/LoadTotal: measured progress of the current stage (shaders), when the engine has it
	LoadDone  uint64 `json:"loadDone,omitempty"`
	LoadTotal uint64 `json:"loadTotal,omitempty"`
	// Prep: while Stage is "prepare", the shader preparation's own stage and counts
	Prep *PrepStatus `json:"prep,omitempty"`
	// Shaders: while starting, the engine's shader status (cleared when the game plays; the
	// performance bar samples it from then on, so state events do not follow every build)
	Shaders *ShaderStatus `json:"shaders,omitempty"`
	// HomeKeyConflict: the Home key is one of the game's controls, so it goes to the game
	HomeKeyConflict bool `json:"homeKeyConflict,omitempty"`
	// Embedded: the game is drawn inside the NEXUS window (engine 0.6)
	Embedded bool `json:"embedded,omitempty"`
	// Amiibo: the game is "scanning" for an amiibo, or one is "placed" (engine 0.6)
	Amiibo string `json:"amiibo,omitempty"`
	// KeyboardMoves: game buttons moved off NEXUS shortcut keys in the default keyboard layout
	KeyboardMoves string `json:"keyboardMoves,omitempty"`
}

const engineID = "nexus"

type Emulator struct {
	app  *App
	mu   sync.Mutex
	prep shaderPrep // shader preparation (shaderprep.go)

	state   RunState
	host    EngineHost
	hostErr error
	hostDir string

	// restoreFullscreen: the NEXUS window's full-screen state before an embedded game started
	restoreFullscreen *bool

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
	// shader preparation (shaderprep.go): with it on, the engine builds the compute programs found
	// in the game's files before the game starts (precache_shaders: 0 Off, 2 Always)
	ini.SetSetting("Renderer", "precache_shaders", map[bool]string{true: "2", false: "0"}[s.Emulator.PrepareShaders])
	// "Match my display": the Switch 2 frame-rate limit follows the refresh rate of the display
	match := s.Engine["graphics.fps_match_display"]
	if g != nil {
		if v, ok := gameNexusSettings(e.app.Library.GP.Get(g.Key))["graphics.fps_match_display"]; ok {
			match = v
		}
	}
	if match == "true" {
		if hz := primaryRefresh(e.app.DetectHardware(false)); hz > 0 {
			ini.SetSetting("Renderer", "nx2_frame_rate_limit", strconv.Itoa(int(clampF(float64(hz), 30, 240))))
		}
	}
	// the graphics card (gpu_select.go): by default the fastest one, not whatever the Vulkan
	// loader happens to list first
	pref := firstNonEmpty(s.Engine["graphics.gpu"], gpuAuto)
	if g != nil {
		if v, ok := gameNexusSettings(e.app.Library.GP.Get(g.Key))["graphics.gpu"]; ok && v != "" {
			pref = v
		}
	}
	if err := e.checkMemoryLayout(g, s); err != nil {
		return "", err
	}
	if devs, err := vulkanDevices(); err == nil {
		if idx, why := ChooseVulkanDevice(devs, pref); idx >= 0 {
			ini.SetSetting("Renderer", "vulkan_device", strconv.Itoa(idx))
			e.app.Log.Printf("graphics: %s (Vulkan device %d of %d, %s)", devs[idx].Name, idx, len(devs), why)
		}
	}
	if g != nil && g.TitleID != "" {
		writeDisabledAddOns(ini, g.TitleID, e.app.Content.DisabledMods(g.TitleID)) // mods switched off for this game
	}
	if pr != nil { // every user has their own saves (saves.go)
		dir := e.app.Profiles.SaveDir(pr.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("cannot create the user's save folder: %w", err)
		}
		abs, _ := filepath.Abs(dir)
		ini.Set("Data%20Storage", "save_directory", iniPath(abs))
	}
	return e.SessionPath(), ini.Save(e.SessionPath())
}

// shaderStatusChanged: whether the loading screen's shader numbers differ.
func shaderStatusChanged(a, b *ShaderStatus) bool {
	if (a == nil) != (b == nil) {
		return true
	}
	if a == nil {
		return false
	}
	an, _ := a.Building()
	bn, _ := b.Building()
	return a.FromCache != b.FromCache || a.Compiled != b.Compiled || an != bn
}

// switch2SessionSummary is one log line with what a Switch 2 session was started with: the
// renderer settings that most affect loading (docs/COMPATIBILITY.md, Ocarina of Time) and
// the update/DLC files passed to the engine. Nothing personal is logged: file names only.
func switch2SessionSummary(g *Game, cfg string, contents []string) string {
	ini, _ := LoadIni(cfg)
	get := func(k string) string {
		if ini == nil {
			return "?"
		}
		if v, ok := ini.GetSetting("Renderer", k); ok {
			return v
		}
		return "default"
	}
	names := make([]string, 0, len(contents))
	for _, c := range contents {
		names = append(names, filepath.Base(c))
	}
	return fmt.Sprintf("engine: Switch 2 session %s: gpu_accuracy=%s reactive_flushing=%s vram=%s resolution=%s async_shaders=%s; %d add-on file(s) %v",
		g.TitleID, get("gpu_accuracy"), get("use_reactive_flushing"), get("vram_usage_mode"),
		get("resolution_setup"), get("use_asynchronous_shaders"), len(names), names)
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
	out = append(out, e.app.Content.EnabledUpdateFiles(g.TitleID)...) // Game Updates added in NEXUS
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
		Contents: e.extraContents(g), AutoMapController: e.inputProfileFor(g, pr) == "",
		HomeKey: s.Emulator.HomeKey, HomeButton: s.Emulator.HomeButton,
		FullscreenKey: s.Emulator.FullscreenKey, ScreenshotKey: s.Emulator.ScreenshotKey, CaptureButton: s.Emulator.CaptureButton,
		RecordKey:     recordKey(s),
		ScreenshotDir: e.app.Media.ScreenshotsDir(), ScreenshotName: captureName(g),
		RevealOnRequest: true, RevealTimeoutMs: 1500, Controllers: e.app.Controllers.ForSession(pr)}
	if isSwitch2Title(g.TitleID) {
		e.app.Log.Print(switch2SessionSummary(g, cfg, req.Contents))
	}
	// one NEXUS window: the game is drawn inside it (Settings › Game sessions › Game window)
	req.Embed = s.Emulator.GameWindow != "separate" && e.app.UI.Native() && req.OwnerWindow != 0
	if req.Embed {
		req.Fullscreen = false
		wasFull := e.app.UI.IsFullscreen()
		e.mu.Lock()
		e.restoreFullscreen = &wasFull
		e.mu.Unlock()
		if s.Emulator.Fullscreen && !wasFull {
			e.app.UI.SetFullscreen(true) // games start full screen: the NEXUS window does
		}
	}
	if s.Emulator.FadeIn {
		req.FadeInMs = 300
	}
	if req.AutoMapController {
		layout, _, _ := e.EffectiveLayout(g, pr)
		req.ButtonMap, req.ControllerType = layout.Resolve(), layout.ControllerType
	}
	_ = os.MkdirAll(e.DataDir(), 0o755)
	e.app.Log.Printf("engine: start %s (%s)", g.Title, g.Path)
	if pr != nil {
		// the engine reads and writes each game's save directly in the user's game folders
		e.app.Saves.Tidy(profileID)
		req.SaveFolders = e.app.Saves.SaveFolders(profileID)
	}
	st := RunState{Running: true, Phase: "starting", Stage: "preparing", Started: nowUnix(), ProfileID: profileID, Mode: "game",
		GameKey: g.Key, Title: g.Title, Emulator: engineID, EmuName: "NEXUS", Volume: vol}
	// shader preparation first (shaderprep.go), when on and the engine build can
	if sp, ok := host.(ShaderPreparer); ok && s.Emulator.PrepareShaders && g.TitleID != "" && e.PreparingKey() == "" {
		st.Stage = "prepare"
		e.setState(st)
		go e.prepareThenStart(host, sp, g, req, st)
		return nil
	}
	if err := e.startEngine(host, req, st); err != nil {
		return &LaunchError{Code: "start-failed", Message: "The NEXUS engine could not start the game.", Detail: err.Error()}
	}
	return nil
}

// startEngine starts the session in the engine and follows it.
func (e *Emulator) startEngine(host EngineHost, req EngineRequest, st RunState) error {
	if err := host.Start(req); err != nil {
		return err
	}
	e.app.Profiles.RecordLaunch(st.ProfileID, st.GameKey, st.Title)
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
	homeSeen := uint64(0)
	lastHome := time.Time{}
	shotsSeen := uint64(0)
	recordSeen := uint64(0)
	fullscreenSeen := uint64(0)
	var bgSince time.Time // when the game started running in the background
	revealAsked := time.Time{}
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
			stage := map[string]string{"starting": "loading", "loading": "loading", "running": "starting"}[es.State]
			if es.LoadStage == "shaders" || es.LoadStage == "precache" {
				stage = es.LoadStage
			}
			if stage != "" && (stage != st.Stage || es.LoadDone != st.LoadDone || es.LoadTotal != st.LoadTotal || shaderStatusChanged(st.Shaders, es.Shaders)) {
				st.Stage, st.LoadDone, st.LoadTotal, st.Shaders = stage, es.LoadDone, es.LoadTotal, es.Shaders
				e.setState(st)
			}
		}
		if es.HomeKeyConflict != st.HomeKeyConflict {
			st.HomeKeyConflict = es.HomeKeyConflict
			e.setState(st)
		}
		switch es.State {
		case "running":
			if es.Frames > 0 && st.Phase != "playing" {
				if !sawGame {
					sessionStart = nowUnix()
				}
				sawGame = true
				playStart = time.Now()
				st.Phase, st.Stage, st.Shaders = "playing", "", nil
				e.setState(st)
				checkpoint()
			}
		case "stopping", "stopped", "error", "idle":
			final = es
		}
		// a game forgotten in the background is paused after a while (Settings › Game sessions)
		if es.State == "running" && es.Suspended && es.Background {
			if bgSince.IsZero() {
				bgSince = time.Now()
			} else if lim := backgroundLimit(e.app); lim > 0 && time.Since(bgSince) >= lim {
				host.Suspend(true) // engine: from running in the background to paused
				host.SetVolume(st.Volume)
				bgSince = time.Time{}
				st.Background = false
				e.setState(st)
				e.app.Log.Printf("emulator: %s paused after %s in the background", st.Title, lim)
				e.app.Events.Emit("emulator.autopaused", map[string]any{"title": st.Title, "minutes": int(lim.Minutes())})
			}
		} else {
			bgSince = time.Time{}
		}
		if es.State == "running" && es.Suspended != st.Suspended {
			// NEXUS Home while playing: time spent suspended is not play time
			if es.Suspended && st.Phase == "playing" {
				played += time.Since(playStart)
			} else if !es.Suspended {
				playStart = time.Now()
			}
			st.Suspended, st.Background = es.Suspended, es.Background
			e.setState(st)
			if st.Suspended {
				e.app.UI.Restore()
				e.app.Events.Emit("emulator.home", st)
			}
		}
		// NEXUS Home: one press while playing suspends the game and shows NEXUS Home — it never
		// switches back (Resume Game does that), so presses seen twice (the engine's count and
		// the Xbox Guide button read here through XInput, or a held button) cannot flicker
		// between the game and NEXUS. While NEXUS is showing, a press just goes to the Home screen.
		g := guidePressed() && s.Emulator.HomeButton
		home := (g && !guideWas) || es.HomePresses > homeSeen
		homeSeen, guideWas = es.HomePresses, g
		if home && st.Phase == "playing" && time.Since(lastHome) > 500*time.Millisecond {
			lastHome = time.Now()
			if !st.Suspended && !es.Suspended {
				go func() { _ = e.Suspend(true) }()
			} else {
				e.app.Events.Emit("input.home", nil)
			}
		}
		// the first frame is ready: NEXUS fades its loading screen out, then the game appears
		// (the interface asks for the reveal; this is the safety net if it does not)
		if es.Ready && !es.Shown {
			if revealAsked.IsZero() {
				revealAsked = time.Now()
				e.app.Events.Emit("launch.reveal", map[string]any{"gameKey": st.GameKey})
			} else if time.Since(revealAsked) > 900*time.Millisecond {
				host.Reveal()
			}
		}
		if es.Fullscreen != st.Fullscreen {
			st.Fullscreen = es.Fullscreen
			e.setState(st)
		}
		// engine 0.6: drawn inside the NEXUS window; F11 makes the NEXUS window full screen
		if es.Embedded != st.Embedded || es.Amiibo != st.Amiibo || es.KeyboardMoves != st.KeyboardMoves {
			scanning := es.Amiibo == "scanning" && st.Amiibo != "scanning"
			st.Embedded, st.Amiibo, st.KeyboardMoves = es.Embedded, es.Amiibo, es.KeyboardMoves
			e.setState(st)
			if scanning {
				e.app.Events.Emit("amiibo.scanning", map[string]any{"title": st.Title})
			}
		}
		for fullscreenSeen < es.FullscreenPresses { // every press counts, also several between two checks
			fullscreenSeen++
			toggleUIFullscreen(e.app.UI)
		}
		// the recording key (F12) in the game window: NEXUS records
		if es.RecordPresses > recordSeen {
			recordSeen = es.RecordPresses
			go func() {
				if err := e.app.Recorder.Toggle(); err != nil {
					e.app.Events.Emit("recording.failed", map[string]any{"error": err.Error()})
				}
			}()
		}
		// screenshots saved by the engine (Alt+F12 / capture button)
		if es.Screenshots > shotsSeen {
			shotsSeen = es.Screenshots
			if es.LastScreenshot != "" {
				e.app.Media.Record(es.LastScreenshot, MediaInfo{GameKey: st.GameKey, TitleID: e.app.Library.TitleIDOf(st.GameKey), Title: st.Title, ProfileID: st.ProfileID})
				e.app.Events.Emit("media.added", map[string]any{"type": "screenshot", "title": st.Title, "id": e.app.Media.rel(es.LastScreenshot)})
			}
		}
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
	if e.app.Recorder.Active() { // the game closed while recording: keep what was recorded
		if _, err := e.app.Recorder.Stop(); err != nil {
			e.app.Events.Emit("recording.failed", map[string]any{"error": err.Error()})
		}
	}
	// wait for the engine to finish shutting the game down
	for i := 0; i < 400 && final.State == "stopping"; i++ {
		time.Sleep(50 * time.Millisecond)
		final = host.Status()
	}
	// the NEXUS window goes back to how it was before the game (full screen or not)
	e.mu.Lock()
	restore := e.restoreFullscreen
	e.restoreFullscreen = nil
	e.mu.Unlock()
	if restore != nil && e.app.UI.IsFullscreen() != *restore {
		e.app.UI.SetFullscreen(*restore)
	}
	secs := int64(played.Seconds())
	crashed := final.State == "error"
	if secs > 0 {
		e.app.Profiles.AddSession(st.ProfileID, st.GameKey, st.Title, SessionRec{Start: sessionStart,
			End: sessionStart + secs, Seconds: secs, Emulator: engineID, Crashed: crashed})
	}
	e.app.Profiles.Checkpoint(nil)
	e.setState(RunState{Volume: e.Volume()}) // session over: allows the save organiser to run
	e.app.Saves.Tidy(st.ProfileID)           // saves an older engine wrote in its own layout
	if e.app.Account != nil {
		e.app.Account.Kick() // the game closed: play time and changes go to the account now
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
	if st := e.State(); st.Stage == "prepare" && e.cancelPrep() {
		return nil // the launch ends when the preparation has stopped (prepareThenStart)
	}
	h, err := e.Host()
	if err != nil {
		return err
	}
	h.Stop(force)
	return nil
}

// Suspend shows NEXUS Home while the game stays loaded (on=true), or returns to the game.
// With "Keep the game running" (the default) the game goes on behind NEXUS, muted and without
// input; otherwise, or with an engine that cannot, it is paused.
func (e *Emulator) Suspend(on bool) error {
	st := e.State()
	if !st.Running {
		return errors.New("Nothing is running.")
	}
	h, err := e.Host()
	if err != nil {
		return err
	}
	if on {
		if e.app.Settings.Get().Emulator.HomeAction != "pause" && h.Background(true) {
			h.SetVolume(0) // muted while NEXUS is showing
			st.Background = true
		} else {
			h.Suspend(true)
			st.Background = false
		}
		e.setState(st)
		return nil
	}
	if st.Background {
		h.Background(false)
		h.SetVolume(st.Volume)
		st.Background = false
		e.setState(st)
	} else {
		h.Suspend(false)
	}
	h.Focus()
	return nil
}

// testBackgroundLimit shortens the background limit in tests.
var testBackgroundLimit time.Duration

func backgroundLimit(a *App) time.Duration {
	if testBackgroundLimit > 0 {
		return testBackgroundLimit
	}
	return a.Settings.Get().Emulator.BackgroundLimit()
}

// GameWindow is the running game's window handle (0 if none).
func (e *Emulator) GameWindow() uintptr {
	if h, err := e.Host(); err == nil && e.State().Running {
		if e.State().Embedded {
			// the game fills the NEXUS window's client area; screen capture records top-level
			// windows, so the NEXUS window is recorded (its client area: the game)
			return e.app.UI.WindowHandle()
		}
		return h.Window()
	}
	return 0
}

// Screenshot of the running game (also while it runs behind NEXUS).
func (e *Emulator) Screenshot() error {
	st := e.State()
	if !st.Running || st.Phase != "playing" {
		return errNoGameWindow
	}
	h, err := e.Host()
	if err != nil {
		return err
	}
	if !h.Screenshot() {
		return errors.New("This engine build cannot take screenshots on request.")
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

// primaryRefresh: the refresh rate of the main display (0 if unknown).
func primaryRefresh(h Hardware) int {
	if d := h.PrimaryDisplay(); d != nil {
		return d.RefreshHz
	}
	return 0
}

// recordKey is the recording shortcut the engine reports ("" when recording is off).
func recordKey(s GlobalSettings) string {
	if !s.Recording.Enabled {
		return ""
	}
	return s.Recording.Key
}

// checkMemoryLayout refuses a memory layout (memory.layout) the PC cannot back: the engine
// reserves the whole emulated DRAM in host memory, and the system, NEXUS and the graphics driver
// need about 4 GB besides. Switch 2 games always use 12 GB (the engine decides that itself).
func (e *Emulator) checkMemoryLayout(g *Game, s GlobalSettings) error {
	layout := s.Engine["memory.layout"]
	if g != nil {
		if v, ok := gameNexusSettings(e.app.Library.GP.Get(g.Key))["memory.layout"]; ok && v != "" {
			layout = v
		}
	}
	gb, _ := strconv.Atoi(layout)
	// Switch 2 programs always get the 12 GB layout from the engine. Like Lichii (#1478), warn on
	// PCs with less than 16 GB of RAM - the game may run out of memory - but let it start.
	if g != nil && isSwitch2Title(g.TitleID) {
		if total := e.app.DetectHardware(false).Memory.TotalMB; total > 0 && total < 16*1024-768 {
			e.app.Log.Printf("engine: Switch 2 game on a PC with %d MB of RAM", total)
			e.app.Events.Emit("launch.warning", map[string]any{"message": fmt.Sprintf(
				"%s is a Switch 2 game: it uses 12 GB of emulated memory, and this PC has %.0f GB of RAM. It may stop or close when memory runs out; close other programs first.",
				g.Title, float64(total)/1024)})
		}
	}
	if gb <= 4 {
		return nil
	}
	total := e.app.DetectHardware(false).Memory.TotalMB
	if total <= 0 {
		return nil // unknown: the engine reports a failed allocation itself
	}
	if need := int64(gb+4) * 1024; total < need-512 { // installed RAM shows a little under its size
		return fmt.Errorf("This game is set to %d GB of emulated memory, which needs a PC with at least %d GB of RAM; this PC has %.0f GB. Choose a smaller memory layout in the game's settings (Memory layout) or in Emulation settings.",
			gb, gb+4, float64(total)/1024)
	}
	if gb >= 10 {
		e.app.Log.Printf("engine: experimental %d GB memory layout", gb)
	}
	return nil
}

// toggleUIFullscreen flips the NEXUS window's full screen atomically where the window supports it.
func toggleUIFullscreen(ui UIHost) {
	if t, ok := ui.(interface{ ToggleFullscreen() bool }); ok {
		t.ToggleFullscreen()
		return
	}
	ui.SetFullscreen(!ui.IsFullscreen())
}
