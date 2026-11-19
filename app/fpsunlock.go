package main

// Per-game frame rate (FPS unlock), not frame generation: the rate the game itself runs at.
//
// Only what the engine can really do is offered (docs/research/FPS_UNLOCK.md):
//   - Switch 2 games start a frame on the engine's VSync event (Renderer/nx2_frame_rate_limit,
//     30-240). A game whose step is fixed to 1/60 s runs FASTER than real time above 60 and slower
//     below, so for an ordinary Switch 2 game only its own rate is offered.
//   - Ocarina of Time (0400486024DEC000, build E0F3CCFA…): the engine's built-in "FPS Unlock"
//     patch (Dark's dynamic step plus a tail for >60) keeps real speed at any limit, so 30-240 are
//     offered while that built-in is on and matches the game's build. Above 60 is experimental.
//   - Mario Kart World: the engine's mode patch (loader/nso.cpp): 30, 60, or dynamic 60/30.
//   - Hyrule Warriors: Age of Imprisonment: 30 always, or dynamic 60/30 (nx2_frame_interval.h).
//   - Switch 1 games run at their own rate. Raising it needs a game-specific frame-rate mod for
//     the exact game version. NEXUS never downloads or installs one; with a frame-rate mod
//     installed and on, the mod decides the rate and NEXUS says so.
// The choice is stored per game (GameProfile.Settings["fps.target"]); "" is the game's default.

import (
	"errors"
	"strconv"
	"strings"
)

const (
	fpsOcarinaTitle = "0400486024DEC000"
	fpsMKWTitle     = "0400C3F00006E000"
	fpsHyruleTitle  = "04003F40232DE000"
	fpsUnlockMod    = "FPS Unlock (built-in)"
)

type FPSChoice struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Status string `json:"status"` // supported | experimental | unavailable
	Note   string `json:"note,omitempty"`
}

type FPSView struct {
	Game        string      `json:"game"`
	Status      string      `json:"status"` // supported | experimental | unavailable
	Summary     string      `json:"summary"`
	Requirement string      `json:"requirement,omitempty"`
	Fix         string      `json:"fix,omitempty"` // "enable-builtin": the requirement can be switched on
	Choices     []FPSChoice `json:"choices"`
	Current     string      `json:"current"`
	Warning     string      `json:"warning,omitempty"`
}

func titleIs(g *Game, id string) bool {
	return g != nil && strings.EqualFold(baseTitleOf(strings.ToUpper(g.TitleID)), id)
}

// FPSView: what can be chosen for this game, and why.
func (a *App) FPSView(key string) (FPSView, error) {
	game, ok := a.Library.Get(key)
	if !ok {
		return FPSView{}, errors.New("Game not found.")
	}
	g := &game
	v := FPSView{Game: g.Title, Current: gameNexusSettings(a.Library.GP.Get(key))["fps.target"]}
	def := FPSChoice{Value: "", Label: "Game default", Status: "supported", Note: "The game's own frame rate. Always safe."}
	steamDeck := isSteamDeckHW(a.DetectHardware(false))
	switch {
	case titleIs(g, fpsOcarinaTitle):
		mods, _ := a.Content.ModsView(key)
		var unlock *ModDetail
		for i := range mods.Builtin {
			if mods.Builtin[i].Name == fpsUnlockMod {
				unlock = &mods.Builtin[i]
			}
		}
		if unlock == nil || !unlock.Enabled {
			v.Status, v.Requirement, v.Fix = "unavailable", "Needs the engine's built-in FPS Unlock patch (Add-Ons), which is switched off for this game. It changes nothing on disk; switch it on to choose a frame rate.", "enable-builtin"
			v.Summary = "Ocarina of Time keeps real speed at other frame rates only with FPS Unlock."
			v.Choices = []FPSChoice{def}
			break
		}
		if unlock.Compat.State == "mismatch" {
			v.Status, v.Requirement = "unavailable", "FPS Unlock is made for one version of the game, and this copy is another ("+unlock.Compat.Note+")."
			v.Summary = "Not available for this game version."
			v.Choices = []FPSChoice{def}
			break
		}
		v.Status = "supported"
		v.Summary = "With the engine's built-in FPS Unlock the game keeps its real speed at any of these rates."
		if unlock.Compat.State == "unknown" {
			v.Summary += " NEXUS checks the game version the first time it is played."
		}
		v.Choices = []FPSChoice{def,
			{Value: "30", Label: "30 fps", Status: "supported", Note: "Steadier on slower hardware."},
			{Value: "60", Label: "60 fps", Status: "supported"},
			{Value: "90", Label: "90 fps", Status: "experimental", Note: "Above 60 the patch also re-times sound; reported problems were long-session sound drop-outs."},
			{Value: "120", Label: "120 fps", Status: "experimental", Note: "As 90."},
			{Value: "max", Label: "Uncapped (240 fps, the engine's highest)", Status: "experimental", Note: "As 90; frame pacing depends on your display."},
		}
	case titleIs(g, fpsMKWTitle):
		v.Status, v.Summary = "supported", "The engine switches Mario Kart World's own frame-rate mode (no files changed)."
		v.Choices = []FPSChoice{def,
			{Value: "30", Label: "30 fps (the game's 30 fps mode)", Status: "supported"},
			{Value: "60", Label: "60 fps (falls back to 30 while 60 can't be held)", Status: "supported"},
			{Value: "dynamic", Label: "Dynamic 60/30 at real speed", Status: "supported"},
		}
	case titleIs(g, fpsHyruleTitle):
		v.Status, v.Summary = "supported", "The engine switches the game's own frame interval (no files changed)."
		v.Choices = []FPSChoice{def,
			{Value: "30", Label: "30 fps always", Status: "supported"},
			{Value: "dynamic", Label: "Dynamic: 30 at real speed while 60 can't be held", Status: "supported"},
		}
	case isSwitch2Title(g.TitleID):
		v.Status = "unavailable"
		v.Summary = "This game advances one fixed step per frame: at a higher frame rate it would run faster than real time, at a lower one slower. NEXUS knows no patch for it, so only its own rate is offered."
		v.Choices = []FPSChoice{def}
	default: // Switch 1
		v.Choices = []FPSChoice{def}
		mods, _ := a.Content.ModsView(key)
		var fpsMods []string
		for _, m := range mods.Mods {
			n := strings.ToLower(m.Name + " " + m.Meta.Name + " " + m.Meta.Category)
			if m.Enabled && (m.Meta.Category == "fps" || strings.Contains(n, "fps") || strings.Contains(n, "frame rate") || strings.Contains(n, "framerate")) {
				fpsMods = append(fpsMods, m.Name)
			}
		}
		if len(fpsMods) > 0 {
			v.Status = "experimental"
			v.Summary = "Your frame-rate mod (" + strings.Join(fpsMods, ", ") + ") sets this game's rate; NEXUS cannot check what it does. If the game runs too fast or too slow, switch the mod off in Add-Ons."
			break
		}
		v.Status = "unavailable"
		v.Summary = "Switch 1 games run at their own frame rate (often 30). Raising it needs a frame-rate mod made for this exact game and version; the emulator's limit alone cannot do it. NEXUS does not download or install patches: add one you trust in Add-Ons and NEXUS will show it here."
	}
	if steamDeck && v.Status != "unavailable" {
		v.Warning = "A higher limit is a ceiling, not a promise: the Steam Deck reaches it only where the game is light enough. The performance strip shows the real rate."
	}
	if !fpsChoiceAllowed(v, v.Current) {
		v.Current = ""
	}
	return v, nil
}

func fpsChoiceAllowed(v FPSView, val string) bool {
	for _, c := range v.Choices {
		if c.Value == val && c.Status != "unavailable" {
			return true
		}
	}
	return false
}

// SetFPS stores a game's choice ("" returns to the game's default).
func (a *App) SetFPS(key, val string) (FPSView, error) {
	v, err := a.FPSView(key)
	if err != nil {
		return v, err
	}
	if !fpsChoiceAllowed(v, val) {
		return v, errors.New("That frame rate is not available for this game.")
	}
	_, _ = a.Library.GP.Update(key, func(p *GameProfile) {
		if p.Settings == nil {
			p.Settings = map[string]string{}
		}
		if val == "" {
			delete(p.Settings, "fps.target")
		} else {
			p.Settings["fps.target"] = val
		}
	})
	return a.FPSView(key)
}

// applyFPS writes the game's frame-rate choice into the session's settings (after the global
// settings, so the per-game choice wins). Nothing is written for the game default.
func (e *Emulator) applyFPS(ini *IniFile, g *Game) {
	if g == nil {
		return
	}
	v, err := e.app.FPSView(g.Key)
	if err != nil || v.Current == "" {
		return
	}
	set := func(k, val string) { ini.SetSetting("Renderer", k, val) }
	switch {
	case titleIs(g, fpsOcarinaTitle):
		n := v.Current
		if n == "max" {
			n = "240"
		}
		if _, err := strconv.Atoi(n); err == nil {
			set("nx2_frame_rate_limit", n)
		}
	case titleIs(g, fpsMKWTitle):
		set("mkw_30fps", strconv.FormatBool(v.Current == "30"))
		set("mkw_60fps", strconv.FormatBool(v.Current == "60"))
		set("mkw_dynamic_fps", strconv.FormatBool(v.Current == "dynamic"))
	case titleIs(g, fpsHyruleTitle):
		set("hyrule_30fps", strconv.FormatBool(v.Current == "30"))
		set("hyrule_dynamic_fps", strconv.FormatBool(v.Current == "dynamic"))
	}
	e.app.Log.Printf("engine: frame rate for %s: %s (per-game choice)", g.Title, v.Current)
}
