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
}

// EngineRequest is the JSON sent to nexus_engine_start.
type EngineRequest struct {
	Game        string  `json:"game"`
	DataDir     string  `json:"dataDir"`
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
	HomeKey    string `json:"homeKey"`
	HomeButton bool   `json:"homeButton"`
}

// EngineStatus is the JSON returned by nexus_engine_status.
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
