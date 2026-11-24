package main

import (
	"os"
	"sync"
)

// Joy-Con Compatibility (Settings › Controls). Enabled (the default, as before 0.9.3): the
// engine's own Joy-Con driver opens Joy-Cons directly (Lichii's enable_joycon_driver). Disabled:
// that driver is off and SDL's direct Joy-Con support (HIDAPI) is off for the engine too, so a
// Joy-Con reaches the game only as the software the user runs presents it (Steam Input on a
// Steam Deck, joycond on Linux, a Joy-Con tool on Windows) and never twice; the NEXUS menus
// ignore single Joy-Cons seen directly (web/app.js Pad). Nothing outside NEXUS is stopped,
// changed or even looked for.

const sdlJoyConHint = "SDL_JOYSTICK_HIDAPI_JOY_CONS"

var joyConEnv struct {
	sync.Mutex
	saved, had bool // the user's own value of the variable, if any, before NEXUS changed it
	value      string
	set        bool // NEXUS set it
}

// joyConCompatOff: the setting is Disabled.
func (a *App) joyConCompatOff() bool {
	return a.Settings.Get().Engine["controls.joycon_compat"] == "disabled"
}

// applyJoyConCompat sets the engine's SDL hint through the environment (a hint the engine sets
// with normal priority does not override one from the environment). SDL 3 reads the process
// environment once, when it is first used, so this runs at startup before the engine is loaded
// (and again before each game, for an engine loaded later); the
// user's own value of the variable is kept and put back when the setting is Enabled again.
func (a *App) applyJoyConCompat() {
	joyConEnv.Lock()
	defer joyConEnv.Unlock()
	if !joyConEnv.saved {
		joyConEnv.value, joyConEnv.had = os.LookupEnv(sdlJoyConHint)
		joyConEnv.saved = true
	}
	if a.joyConCompatOff() {
		if !joyConEnv.set {
			a.Log.Printf("controllers: Joy-Con Compatibility disabled: the engine leaves Joy-Cons to other software")
		}
		os.Setenv(sdlJoyConHint, "0")
		joyConEnv.set = true
		return
	}
	if joyConEnv.set {
		if joyConEnv.had {
			os.Setenv(sdlJoyConHint, joyConEnv.value)
		} else {
			os.Unsetenv(sdlJoyConHint)
		}
		joyConEnv.set = false
	}
}
