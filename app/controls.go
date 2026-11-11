package main

import (
	"errors"
	"regexp"
	"strings"
)

// Controller layouts for games.
//
// The engine maps player 1 to the first connected controller automatically, using the
// controller's standard layout (SDL game-controller database: Xbox, PlayStation, Switch Pro and
// most USB/Bluetooth pads). A layout says which physical button each Switch button uses; NEXUS
// passes it to the engine at every launch. Layouts are stored per user (their default for all
// games) and optionally per game. "Restore defaults" clears the custom buttons.
//
// Physical positions are named after the face of a standard gamepad, so they work for every
// controller type: east/south/north/west (face buttons), lshoulder/rshoulder, ltrigger/rtrigger,
// start/back, lstick/rstick.

type GameLayout struct {
	Base           string            `json:"base"`           // nintendo | xbox
	Buttons        map[string]string `json:"buttons"`        // custom: switch button -> physical position
	ControllerType string            `json:"controllerType"` // "" (automatic) | pro | dual_joycon | handheld
}

var SwitchButtons = []string{"a", "b", "x", "y", "l", "r", "zl", "zr", "plus", "minus", "lstick", "rstick"}
var PhysicalPositions = []string{"east", "south", "north", "west", "lshoulder", "rshoulder", "ltrigger", "rtrigger", "start", "back", "lstick", "rstick"}

// BaseLayouts: "nintendo" keeps the console's button positions (A on the right), "xbox" follows
// the letters printed on Xbox-style pads (A at the bottom).
var BaseLayouts = map[string]map[string]string{
	"nintendo": {"a": "east", "b": "south", "x": "north", "y": "west", "l": "lshoulder", "r": "rshoulder",
		"zl": "ltrigger", "zr": "rtrigger", "plus": "start", "minus": "back", "lstick": "lstick", "rstick": "rstick"},
	"xbox": {"a": "south", "b": "east", "x": "west", "y": "north", "l": "lshoulder", "r": "rshoulder",
		"zl": "ltrigger", "zr": "rtrigger", "plus": "start", "minus": "back", "lstick": "lstick", "rstick": "rstick"},
}

// GamePreset: games that expect a particular controller. Only facts that matter for the engine's
// emulated controller are listed; motion-only games are flagged honestly.
type GamePreset struct {
	re             *regexp.Regexp
	Name           string `json:"name"`
	ControllerType string `json:"controllerType"`
	Note           string `json:"note"`
}

var gamePresets = []GamePreset{
	{regexp.MustCompile(`(?i)^super mario party$`), "Super Mario Party", "dual_joycon",
		"This game only accepts Joy-Con controllers, so NEXUS presents your controller as a pair of Joy-Cons. Minigames that need motion controls cannot be played with a normal gamepad."},
	{regexp.MustCompile(`(?i)1-2-switch`), "1-2-Switch", "dual_joycon",
		"Needs Joy-Cons; NEXUS presents your controller as a pair of Joy-Cons. Most of its games rely on motion and HD rumble, which a normal gamepad cannot provide."},
	{regexp.MustCompile(`(?i)nintendo switch sports`), "Nintendo Switch Sports", "dual_joycon",
		"Needs Joy-Con motion controls; with a normal gamepad most sports cannot be played."},
	{regexp.MustCompile(`(?i)ring fit adventure`), "Ring Fit Adventure", "dual_joycon",
		"Needs the Ring-Con and Leg Strap accessories, which NEXUS cannot emulate."},
	{regexp.MustCompile(`(?i)labo`), "Nintendo Labo", "dual_joycon",
		"Needs Joy-Cons with the IR camera and the cardboard Toy-Con; not playable with a gamepad."},
}

func presetFor(title string) *GamePreset {
	for i := range gamePresets {
		if gamePresets[i].re.MatchString(strings.TrimSpace(title)) {
			return &gamePresets[i]
		}
	}
	return nil
}

func normalizeLayout(l GameLayout) (GameLayout, error) {
	if l.Base == "" {
		l.Base = "nintendo"
	}
	if _, ok := BaseLayouts[l.Base]; !ok {
		return l, errors.New("unknown base layout")
	}
	switch l.ControllerType {
	case "", "pro", "dual_joycon", "handheld":
	default:
		return l, errors.New("unknown controller type")
	}
	clean := map[string]string{}
	for k, v := range l.Buttons {
		if !contains(SwitchButtons, k) || !contains(PhysicalPositions, v) {
			return l, errors.New("unknown button " + k + "=" + v)
		}
		if BaseLayouts[l.Base][k] != v {
			clean[k] = v
		}
	}
	l.Buttons = clean
	return l, nil
}

// Resolve returns the complete button map (Switch button -> position) for the engine.
func (l GameLayout) Resolve() map[string]string {
	base := BaseLayouts[l.Base]
	if base == nil {
		base = BaseLayouts["nintendo"]
	}
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range l.Buttons {
		out[k] = v
	}
	return out
}

// EffectiveLayout: the game's own layout, else the user's; a game preset supplies the
// controller type when neither sets one.
func (e *Emulator) EffectiveLayout(g *Game, pr *Profile) (GameLayout, string, *GamePreset) {
	l, src := GameLayout{Base: "nintendo"}, "default"
	if pr != nil && (pr.Controls.GameLayout.Base != "" || len(pr.Controls.GameLayout.Buttons) > 0 || pr.Controls.GameLayout.ControllerType != "") {
		l, src = pr.Controls.GameLayout, "user"
	}
	if e.app.Controllers != nil {
		if cl := e.app.Controllers.Player1Layout(pr); cl != nil {
			l, src = *cl, "controller" // the player-1 controller's own layout
		}
	}
	var preset *GamePreset
	if g != nil {
		if gl := e.app.Library.GP.Get(g.Key).Controls; gl != nil {
			l, src = *gl, "game"
		}
		preset = presetFor(g.Title)
		if preset != nil && l.ControllerType == "" {
			l.ControllerType = preset.ControllerType
			if src != "game" {
				src = "preset"
			}
		}
	}
	if l.Base == "" {
		l.Base = "nintendo"
	}
	return l, src, preset
}
