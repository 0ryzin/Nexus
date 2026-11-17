package main

import "encoding/json"

// EngineHost is the NEXUS engine as seen from the NEXUS application: a library loaded into this
// process (nexus_engine.dll, see the engine's include/nexus_engine.h). NEXUS never starts an
// emulator executable; a game runs on threads the engine creates inside NEXUS.
type EngineHost interface {
	Info() (EngineBuildInfo, error)
	Start(req EngineRequest) error
	Status() EngineStatus
	Stop(force bool)
	SetVolume(percent int)
	Focus()
	// Suspend pauses the game and hides its window (NEXUS Home); false resumes it.
	Suspend(on bool)
	// Reveal shows the game window after its first frame (engine 0.4; no-op for older builds).
	Reveal()
	// Controllers lists connected game controllers (engine 0.4; error for older builds).
	Controllers() ([]PadInfo, error)
	// Background hides the game window and keeps the game running with its input blocked (NEXUS
	// Home); false shows it again. False is returned when the engine cannot (older builds).
	Background(on bool) bool
	// Screenshot asks the engine for a screenshot; Window is the game window's handle (0 if none).
	Screenshot() bool
	Window() uintptr
	// Amiibo places an amiibo dump on the virtual NFC reader ("" removes it): 0 placed,
	// 1 the game is not scanning, 2 unreadable file, 3 not an amiibo, -1 no game, -2 this
	// engine build cannot (engine 0.6).
	Amiibo(path string) int
}

// ShaderPreparer is engine 0.8's shader preparation (optional: older engine builds lack it).
// PrepareShaders builds a game's shader precache file from its own files while no game runs:
// 0 started, -1 busy (a game or a preparation runs), -2 bad request, -3 not in this build.
type ShaderPreparer interface {
	PrepareShaders(req PrepRequest) int
	CancelPrepare()
	// SkipPrecache: while the boot precache runs, start the game now (the rest is built when
	// the game first uses it).
	SkipPrecache()
}

// PrepRequest asks the engine to prepare a game's shaders (nexus_engine_prepare_shaders).
type PrepRequest struct {
	Game       string   `json:"game"`
	DataDir    string   `json:"dataDir"`
	ShaderDir  string   `json:"shaderDir,omitempty"` // shared shader cache (sharedcache.go)
	ConfigFile string   `json:"configFile"`
	TitleID    string   `json:"titleId"`
	Contents   []string `json:"contents,omitempty"`
	Force      bool     `json:"force,omitempty"`
}

// PrepStatus is the engine's report of the last shader preparation.
type PrepStatus struct {
	State    string  `json:"state"` // running | done | current | failed | stopped
	TitleID  string  `json:"titleId"`
	Stage    string  `json:"stage,omitempty"` // Reading game files | Extracting shaders | Converting | Writing
	Done     uint64  `json:"done"`
	Total    uint64  `json:"total"` // 0: unknown
	Archives uint64  `json:"archives,omitempty"`
	Programs uint64  `json:"programs,omitempty"` // compute programs in the precache file
	Skipped  uint64  `json:"skipped,omitempty"`
	Seconds  float64 `json:"seconds,omitempty"`
	Error    string  `json:"error,omitempty"`
}

// PadInfo is one connected game controller as the engine (SDL) sees it.
type PadInfo struct {
	GUID    string `json:"guid"`
	Port    int    `json:"port"`
	Name    string `json:"name"`
	Type    string `json:"type"` // xbox360 | xboxone | ps3 | ps4 | ps5 | switch-pro | joycon-* | standard
	Vendor  int    `json:"vendor"`
	Product int    `json:"product"`
	Path    string `json:"path,omitempty"`
	Serial  string `json:"serial,omitempty"`
}

// EnginePad is a controller chosen for a player (request "controllers").
type EnginePad struct {
	GUID string `json:"guid"`
	Port int    `json:"port"`
}

// EngineRequest is the JSON sent to nexus_engine_start.
type EngineRequest struct {
	Game        string  `json:"game"`
	DataDir     string  `json:"dataDir"`
	ShaderDir   string  `json:"shaderDir,omitempty"` // shared shader cache (sharedcache.go)
	TitleID     string  `json:"titleId,omitempty"`   // informative (the engine reads it from the game)
	ConfigFile  string  `json:"configFile"`
	Title       string  `json:"title"`
	Fullscreen  bool    `json:"fullscreen"`
	Volume      int     `json:"volume"`
	OwnerWindow uintptr `json:"ownerWindow"`
	// Contents are update and DLC containers for this game found in the user's game folders;
	// the engine registers them so the game starts with its latest update and its DLC.
	Contents []string `json:"contents,omitempty"`
	// AutoMapController maps player 1 to the first connected controller when the user has not
	// chosen a saved controller profile.
	AutoMapController bool `json:"autoMapController"`
	// ButtonMap / ControllerType: the NEXUS controller layout (controls.go) for this game.
	ButtonMap      map[string]string `json:"buttonMap,omitempty"`
	ControllerType string            `json:"controllerType,omitempty"`
	// NEXUS Home: keyboard key (SDL key name, "" = none; the engine leaves it to the game when a
	// game control uses it) and the controller's Guide/Home button
	// SaveFolders: for each game (base title ID), the folder of its account and device save
	// inside the user's save directory: {"0400…":{"account":"0400…-OOT","device":"0400…-OOT (device)"}}
	SaveFolders map[string]map[string]string `json:"saveFolders,omitempty"`
	HomeKey     string                       `json:"homeKey"`
	HomeButton  bool                         `json:"homeButton"`
	// F11 / F12 by default ("" = off); keys used by the game controls go to the game
	FullscreenKey string `json:"fullscreenKey"`
	ScreenshotKey string `json:"screenshotKey"`
	RecordKey     string `json:"recordKey"`     // presses are counted (recordPresses); NEXUS records
	CaptureButton bool   `json:"captureButton"` // controller capture/share button: screenshot
	ScreenshotDir string `json:"screenshotDir,omitempty"`
	// ScreenshotName starts the screenshot file name (the game's short name)
	ScreenshotName string `json:"screenshotName,omitempty"`
	// Controllers chosen for players 1, 2, … (empty: the first connected controller)
	Controllers []EnginePad `json:"controllers,omitempty"`
	// RevealOnRequest: after its first frame the game window waits for Reveal (NEXUS fades its
	// loading screen out first), at most RevealTimeoutMs; FadeInMs fades the window in
	RevealOnRequest bool `json:"revealOnRequest,omitempty"`
	RevealTimeoutMs int  `json:"revealTimeoutMs,omitempty"`
	FadeInMs        int  `json:"fadeInMs,omitempty"`
	// Embed: the game is drawn inside the NEXUS window (OwnerWindow) instead of a window of its
	// own (engine 0.6); NEXUS makes its own window full screen for F11.
	Embed bool `json:"embed,omitempty"`
}

// EngineStatus is the JSON returned by nexus_engine_status.
// ShaderStatus: the pipelines this session used and how they got ready, and the builds in
// flight. Queued/Running/BackgroundQueued/Failed are nil when the engine cannot tell.
type ShaderStatus struct {
	FromCache          uint64  `json:"fromCache"`    // built from the disk cache at boot
	FromPrebuild       uint64  `json:"fromPrebuild"` // built from the disk cache in the background
	Compiled           uint64  `json:"compiled"`     // compiled when first used (can stutter)
	ProgramsReused     uint64  `json:"programsReused"`
	ProgramsTranslated uint64  `json:"programsTranslated"`
	Queued             *uint64 `json:"queued,omitempty"`
	Running            *uint64 `json:"running,omitempty"`
	BackgroundQueued   *uint64 `json:"backgroundQueued,omitempty"`
	Failed             *uint64 `json:"failed,omitempty"`
}

// Building is the number of builds the game is waiting for or will soon need (not the
// background prebuild of cached pipelines); ok=false when the engine cannot tell.
func (s *ShaderStatus) Building() (n uint64, ok bool) {
	if s == nil || s.Queued == nil || s.Running == nil {
		return 0, false
	}
	n = *s.Queued + *s.Running
	if s.BackgroundQueued != nil {
		n -= min(n, *s.BackgroundQueued)
	}
	return n, true
}

type EngineStatus struct {
	State     string `json:"state"` // idle | starting | running | stopping | stopped | error
	Frames    uint64 `json:"frames"`
	ProgramID string `json:"programId,omitempty"`
	System    string `json:"system,omitempty"` // switch1 | switch2 (from the program's NPDM)
	Suspended bool   `json:"suspended"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	Volume    int    `json:"volume"`
	// Shown: the game window is visible (the engine shows it with the first frame)
	Shown bool `json:"shown,omitempty"`
	// HomePresses counts NEXUS Home requests from the game window (key or controller button)
	HomePresses     uint64 `json:"homePresses,omitempty"`
	HomeKeyConflict bool   `json:"homeKeyConflict,omitempty"`
	// real loading progress: loadStage "preparing" | "shaders", loadDone of loadTotal
	LoadStage string `json:"loadStage,omitempty"`
	LoadDone  uint64 `json:"loadDone,omitempty"`
	LoadTotal uint64 `json:"loadTotal,omitempty"`
	// engine 0.4
	Ready              bool   `json:"ready,omitempty"` // first frame drawn (window may wait for Reveal)
	Fullscreen         bool   `json:"fullscreen,omitempty"`
	Screenshots        uint64 `json:"screenshots,omitempty"`
	ScreenshotFailures uint64 `json:"screenshotFailures,omitempty"`
	LastScreenshot     string `json:"lastScreenshot,omitempty"`
	// engine 0.5
	RecordPresses uint64 `json:"recordPresses,omitempty"`
	Background    bool   `json:"background,omitempty"`
	// engine 0.6
	Embedded          bool   `json:"embedded,omitempty"`          // drawn inside the NEXUS window
	FullscreenPresses uint64 `json:"fullscreenPresses,omitempty"` // F11 while embedded
	Amiibo            string `json:"amiibo,omitempty"`            // none | scanning | placed
	// engine 0.8: shaders and pipelines of this session (nexus_shader_status.h)
	Shaders *ShaderStatus `json:"shaders,omitempty"`
	// engine 0.8: the last shader preparation (nexus_engine_prepare_shaders)
	Prepare       *PrepStatus `json:"prepare,omitempty"`
	KeyboardMoves string      `json:"keyboardMoves,omitempty"` // "R H -> E": default keys freed for shortcuts
}

// EngineBuildInfo is nexus_engine_info plus what NEXUS knows about the installed build.
type EngineBuildInfo struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	API        int               `json:"api"`
	Upstream   map[string]string `json:"upstream,omitempty"`
	Systems    []EngineSystem    `json:"systems,omitempty"`
	Renderers  []string          `json:"renderers,omitempty"`
	Components []string          `json:"components,omitempty"`
}

type EngineSystem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

const engineAPIVersion = 2

func parseEngineInfo(s string) (EngineBuildInfo, error) {
	var i EngineBuildInfo
	err := json.Unmarshal([]byte(s), &i)
	return i, err
}

// allowFakeEngine is set only by NEXUS's tests (see fixture_test.go).
var allowFakeEngine bool
