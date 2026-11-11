package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// The NEXUS configuration system.
//
// Every setting has one NEXUS identity (id, section, label, values). The NEXUS engine declares
// which settings it applies and how a NEXUS value is written into the session configuration it
// reads at launch (EngineCaps / engineBinds).
//
// Stored values (global: settings.json "engine"; per game:
// GameProfiles/<id>/settings.json "settings") are always NEXUS values.

type NSetting struct {
	ID      string      `json:"id"`
	Section string      `json:"section"` // general | graphics | advanced | audio | controls | system
	Label   string      `json:"label"`
	Kind    string      `json:"kind"` // enum | bool | int | info
	Choices []EmuChoice `json:"choices,omitempty"`
	Min     int         `json:"min,omitempty"`
	Max     int         `json:"max,omitempty"`
	Step    int         `json:"step,omitempty"`
	Suffix  string      `json:"suffix,omitempty"`
	Presets []int       `json:"presets,omitempty"` // int: values offered in the picker (all steps when empty)
	Help    string      `json:"help,omitempty"`
}

var NexusSections = []EmuChoice{{"general", "General"}, {"graphics", "Graphics"}, {"advanced", "Advanced Graphics"},
	{"enhance", "Frame generation & DLSS"}, {"audio", "Audio"}, {"controls", "Controls"}, {"system", "System"}}

var languages = []EmuChoice{{"ja", "Japanese"}, {"en-US", "English (US)"}, {"fr", "French"}, {"de", "German"}, {"it", "Italian"},
	{"es", "Spanish"}, {"zh", "Chinese"}, {"ko", "Korean"}, {"nl", "Dutch"}, {"pt", "Portuguese"}, {"ru", "Russian"},
	{"zh-TW", "Taiwanese"}, {"en-GB", "English (UK)"}, {"fr-CA", "French (Canada)"}, {"es-419", "Spanish (Latin America)"},
	{"zh-Hans", "Chinese (Simplified)"}, {"zh-Hant", "Chinese (Traditional)"}, {"pt-BR", "Portuguese (Brazil)"}}

var NexusSettings = []NSetting{
	// Graphics
	{ID: "graphics.backend", Section: "graphics", Label: "Graphics backend", Kind: "enum",
		Choices: []EmuChoice{{"vulkan", "Vulkan"}, {"opengl", "OpenGL"}, {"opengl_glasm", "OpenGL (GLASM)"}, {"opengl_spirv", "OpenGL (SPIR-V)"}}},
	{ID: "graphics.resolution", Section: "graphics", Label: "Resolution", Kind: "info",
		Help: "The resolution the game is rendered at. It follows from the console mode (720p handheld, 1080p docked) and resolution scaling."},
	{ID: "graphics.scale", Section: "graphics", Label: "Resolution scaling", Kind: "enum",
		Choices: []EmuChoice{{"0.5", "0.5×"}, {"0.75", "0.75×"}, {"1", "1× native"}, {"1.25", "1.25×"}, {"1.5", "1.5×"}, {"2", "2×"}, {"3", "3×"}, {"4", "4×"}}},
	{ID: "graphics.aspect", Section: "graphics", Label: "Aspect ratio", Kind: "enum",
		Choices: []EmuChoice{{"16:9", "16:9"}, {"4:3", "4:3"}, {"16:10", "16:10"}, {"21:9", "21:9"}, {"stretch", "Stretch to window"}}},
	{ID: "graphics.vsync", Section: "graphics", Label: "VSync", Kind: "enum",
		Help:    "On: frames are shown in step with your display's refresh (no tearing). Off: shown immediately (lowest latency, may tear). Mailbox: no tearing with lower latency where the driver supports it.",
		Choices: []EmuChoice{{"fifo", "On"}, {"relaxed", "On (relaxed)"}, {"mailbox", "Mailbox"}, {"off", "Off (unlimited)"}}},
	{ID: "graphics.fps_limit", Section: "graphics", Label: "Frame rate limit (Switch 2 games)", Kind: "int", Min: 30, Max: 240, Step: 1, Suffix: " fps",
		Presets: []int{30, 40, 45, 50, 60, 72, 75, 80, 90, 100, 110, 120, 144, 160, 165, 170, 180, 200, 240},
		Help:    "The highest frame rate Switch 2 games may render at (30–240). Switch 1 games run at their own rate (usually 30 or 60) whatever is set here — a higher limit never speeds a game up. Frame generation adds frames on top."},
	{ID: "graphics.fps_match_display", Section: "graphics", Label: "Match my display’s refresh rate", Kind: "bool",
		Help: "Sets the frame rate limit above to the refresh rate of your display when the game starts."},
	{ID: "graphics.fullscreen", Section: "graphics", Label: "Fullscreen behaviour", Kind: "enum",
		Choices: []EmuChoice{{"borderless", "Borderless window"}, {"exclusive", "Exclusive fullscreen"}}},
	{ID: "graphics.aa", Section: "graphics", Label: "Anti-aliasing", Kind: "enum",
		Choices: []EmuChoice{{"none", "Off"}, {"fxaa", "FXAA"}, {"smaa", "SMAA"}}},
	{ID: "graphics.anisotropy", Section: "graphics", Label: "Anisotropic filtering", Kind: "enum",
		Choices: []EmuChoice{{"auto", "Automatic"}, {"default", "Game default"}, {"2", "2×"}, {"4", "4×"}, {"8", "8×"}, {"16", "16×"}}},
	// Advanced graphics
	{ID: "adv.shader_cache", Section: "advanced", Label: "Disk shader cache", Kind: "bool", Help: "Keeps compiled shaders between sessions to reduce stutter."},
	{ID: "adv.async_shaders", Section: "advanced", Label: "Asynchronous shader building", Kind: "bool", Help: "Less stutter while shaders compile; may cause brief visual glitches."},
	{ID: "adv.gpu_accuracy", Section: "advanced", Label: "GPU accuracy", Kind: "enum", Choices: []EmuChoice{{"low", "Low (faster)"}, {"high", "High (more accurate)"}}},
	{ID: "adv.filter", Section: "advanced", Label: "Texture filtering / upscaling filter", Kind: "enum",
		Choices: []EmuChoice{{"fsr3", "AMD FSR 3"}, {"fsr1", "AMD FSR 1"}, {"bilinear", "Bilinear"}, {"bicubic", "Bicubic"}, {"lanczos", "Lanczos"}, {"scaleforce", "ScaleForce"}, {"area", "Area"}, {"nearest", "Nearest neighbour"}}},
	{ID: "adv.reactive_flushing", Section: "advanced", Label: "Reactive flushing", Kind: "bool", Help: "Improves accuracy in some games at a performance cost."},
	{ID: "adv.astc", Section: "advanced", Label: "ASTC texture decoding", Kind: "enum", Choices: []EmuChoice{{"gpu", "GPU"}, {"cpu", "CPU"}, {"cpu_async", "CPU (asynchronous)"}}},
	{ID: "adv.vram", Section: "advanced", Label: "Video memory usage", Kind: "enum", Choices: []EmuChoice{{"conservative", "Conservative"}, {"aggressive", "Aggressive"}}},
	{ID: "adv.nvdec", Section: "advanced", Label: "Video decoding", Kind: "enum", Choices: []EmuChoice{{"gpu", "GPU"}, {"cpu", "CPU"}, {"off", "Off"}}},
	{ID: "adv.cpu_accuracy", Section: "advanced", Label: "CPU accuracy", Kind: "enum", Choices: []EmuChoice{{"auto", "Automatic"}, {"accurate", "Accurate"}, {"unsafe", "Unsafe (faster)"}, {"paranoid", "Paranoid (slowest)"}}},
	{ID: "adv.multicore", Section: "advanced", Label: "Multicore CPU emulation", Kind: "bool"},
	{ID: "adv.speed_limit_on", Section: "advanced", Label: "Limit emulation speed", Kind: "bool",
		Help: "On: games run at real-time speed. Off (uncapped): the game's own simulation runs as fast as your PC allows — the game itself speeds up; this is not the same as a higher frame rate."},
	{ID: "adv.speed_limit", Section: "advanced", Label: "Speed limit", Kind: "int", Min: 25, Max: 400, Step: 25, Suffix: "%"},
	// Frame generation & upscaler replacement (enhancements.go decides what this PC can use)
	{ID: "enhance.framegen", Section: "enhance", Label: "Frame generation", Kind: "bool",
		Help: "Adds generated frames between the game's own frames for smoother motion. Vulkan only. Adds a little input latency; best when the game already runs at a steady frame rate."},
	{ID: "enhance.framegen_type", Section: "enhance", Label: "Frame generation method", Kind: "enum",
		Choices: []EmuChoice{{"fsr3", "AMD FSR 3 (built in)"}, {"lsfg", "Lossless Scaling (your own copy)"}}},
	{ID: "enhance.framegen_mult", Section: "enhance", Label: "Generated frames", Kind: "enum",
		Choices: []EmuChoice{{"2", "2× (one generated frame)"}, {"3", "3×"}, {"4", "4×"}}},
	{ID: "enhance.framegen_target", Section: "enhance", Label: "Target frame rate", Kind: "enum",
		Choices: []EmuChoice{{"0", "Automatic"}, {"60", "60 fps"}, {"90", "90 fps"}, {"120", "120 fps"}, {"144", "144 fps"}, {"165", "165 fps"}, {"240", "240 fps"}},
		Help:    "Automatic follows the multiplier; a target caps the output at your display's refresh rate."},
	{ID: "enhance.dlss", Section: "enhance", Label: "NVIDIA DLSS instead of the game's DLSS", Kind: "bool",
		Help: "Switch 2 games that use the console's DLSS get your graphics card's NVIDIA DLSS instead. Other games are not affected."},
	{ID: "enhance.dlss_quality", Section: "enhance", Label: "DLSS quality", Kind: "enum",
		Choices: []EmuChoice{{"auto", "Automatic"}, {"native", "Native (DLAA)"}, {"quality", "Quality"}, {"balanced", "Balanced"}, {"performance", "Performance"}, {"ultra", "Ultra Performance"}}},
	{ID: "enhance.dlss_model", Section: "enhance", Label: "DLSS model", Kind: "enum",
		Choices: []EmuChoice{{"m", "Preset M (recommended)"}, {"default", "NVIDIA default"}, {"k", "Preset K"}, {"l", "Preset L"}, {"j", "Preset J"}, {"f", "Preset F"}, {"e", "Preset E"}}},
	{ID: "enhance.fsr4", Section: "enhance", Label: "AMD FSR 4 instead of FSR 3.1", Kind: "bool",
		Help: "Where the engine replaces a game's upscaler with FSR, use FSR 4 (AMD's driver provides it)."},
	// Audio
	{ID: "audio.backend", Section: "audio", Label: "Audio output", Kind: "enum",
		Choices: []EmuChoice{{"auto", "Automatic"}, {"cubeb", "Cubeb"}, {"sdl", "SDL"}, {"none", "No audio"}}},
	{ID: "audio.channels", Section: "audio", Label: "Sound mode", Kind: "enum", Choices: []EmuChoice{{"mono", "Mono"}, {"stereo", "Stereo"}, {"surround", "Surround"}}},
	{ID: "audio.quality", Section: "audio", Label: "Audio quality", Kind: "info",
		Help: "The console's audio is reproduced at its native 48 kHz; the engine has no separate quality setting."},
	{ID: "audio.volume", Section: "audio", Label: "Volume", Kind: "int", Min: 0, Max: 200, Step: 1, Suffix: "%",
		Help: "0% is silent, 100% is the game's own level, up to 200% boosts quiet games. Above 100% a soft limiter keeps loud passages from clipping."},
	{ID: "audio.mute", Section: "audio", Label: "Mute", Kind: "bool"},
	// Controls
	{ID: "controls.profile", Section: "controls", Label: "Controller profile (player 1)", Kind: "enum"},
	{ID: "controls.mapping", Section: "controls", Label: "Keyboard, mouse and controller mapping", Kind: "info"},
	{ID: "controls.sensitivity", Section: "controls", Label: "Stick sensitivity and dead zone", Kind: "info"},
	// System
	{ID: "system.firmware", Section: "system", Label: "Firmware", Kind: "info"},
	{ID: "system.docked", Section: "system", Label: "Console mode", Kind: "enum", Choices: []EmuChoice{{"docked", "Docked (TV)"}, {"handheld", "Handheld"}}},
	{ID: "system.language", Section: "system", Label: "System language", Kind: "enum", Choices: languages},
	{ID: "system.region", Section: "system", Label: "Console region", Kind: "enum",
		Choices: []EmuChoice{{"japan", "Japan"}, {"usa", "USA"}, {"europe", "Europe"}, {"australia", "Australia"}, {"china", "China"}, {"korea", "Korea"}, {"taiwan", "Taiwan"}}},
}

func nsetting(id string) *NSetting {
	for i := range NexusSettings {
		if NexusSettings[i].ID == id {
			return &NexusSettings[i]
		}
	}
	return nil
}

func validNexusValue(id, v string) bool {
	s := nsetting(id)
	if s == nil {
		return false
	}
	switch s.Kind {
	case "bool":
		return v == "true" || v == "false"
	case "int":
		n, err := strconv.Atoi(v)
		return err == nil && n >= s.Min && n <= s.Max
	case "enum":
		for _, c := range s.Choices {
			if c.V == v {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------- engine keys

// engineBind maps a NEXUS setting to an engine configuration key. Keys and enum
// numbers were checked against the engine source (src/common/settings.h,
// settings_enums.h; enums are stored as their underlying integer).
type engineBind struct {
	key  string
	vals map[string]string // nil: value is written as-is (bool, int)
}

func seq(keys ...string) map[string]string {
	m := map[string]string{}
	for i, k := range keys {
		if k != "" {
			m[k] = strconv.Itoa(i)
		}
	}
	return m
}

var engineBinds = map[string]engineBind{
	"graphics.backend":        {"Renderer/backend", map[string]string{"opengl": "0", "vulkan": "1", "opengl_glasm": "3", "opengl_spirv": "4"}},
	"graphics.scale":          {"Renderer/resolution_setup", map[string]string{"0.5": "1", "0.75": "2", "1": "3", "1.25": "4", "1.5": "5", "2": "6", "3": "7", "4": "8"}},
	"graphics.aspect":         {"Renderer/aspect_ratio", map[string]string{"16:9": "0", "4:3": "1", "21:9": "2", "16:10": "3", "stretch": "4"}},
	"graphics.vsync":          {"Renderer/use_vsync", map[string]string{"off": "0", "mailbox": "1", "fifo": "2", "relaxed": "3"}},
	"graphics.fullscreen":     {"Renderer/fullscreen_mode", map[string]string{"borderless": "0", "exclusive": "1"}},
	"graphics.aa":             {"Renderer/anti_aliasing", map[string]string{"none": "0", "fxaa": "1", "smaa": "2"}},
	"graphics.anisotropy":     {"Renderer/max_anisotropy", map[string]string{"auto": "0", "default": "1", "2": "2", "4": "3", "8": "4", "16": "5"}},
	"adv.shader_cache":        {"Renderer/use_disk_shader_cache", nil},
	"adv.async_shaders":       {"Renderer/use_asynchronous_shaders", nil},
	"adv.gpu_accuracy":        {"Renderer/gpu_accuracy", map[string]string{"low": "0", "high": "1"}},
	"adv.filter":              {"Renderer/scaling_filter", map[string]string{"nearest": "0", "bilinear": "1", "bicubic": "2", "lanczos": "4", "scaleforce": "5", "fsr1": "6", "area": "7", "fsr3": "15"}},
	"adv.reactive_flushing":   {"Renderer/use_reactive_flushing", nil},
	"adv.astc":                {"Renderer/accelerate_astc", map[string]string{"cpu": "0", "gpu": "1", "cpu_async": "2"}},
	"adv.vram":                {"Renderer/vram_usage_mode", map[string]string{"conservative": "0", "aggressive": "1"}},
	"adv.nvdec":               {"Renderer/nvdec_emulation", map[string]string{"off": "0", "cpu": "1", "gpu": "2"}},
	"adv.cpu_accuracy":        {"Cpu/cpu_accuracy", map[string]string{"auto": "0", "accurate": "1", "unsafe": "2", "paranoid": "3"}},
	"adv.multicore":           {"Core/use_multi_core", nil},
	"adv.speed_limit_on":      {"Core/use_speed_limit", nil},
	"adv.speed_limit":         {"Core/speed_limit", nil},
	"graphics.fps_limit":      {"Renderer/nx2_frame_rate_limit", nil},
	"enhance.framegen":        {"Renderer/frame_gen", nil},
	"enhance.framegen_type":   {"Renderer/frame_gen_type", map[string]string{"lsfg": "0", "fsr3": "1"}},
	"enhance.framegen_mult":   {"Renderer/frame_gen_multiplier", map[string]string{"2": "2", "3": "3", "4": "4"}},
	"enhance.framegen_target": {"Renderer/frame_gen_target_rate", map[string]string{"0": "0", "60": "60", "90": "90", "120": "120", "144": "144", "165": "165", "240": "240"}},
	"enhance.dlss":            {"Renderer/nvidia_dlss", nil},
	"enhance.dlss_quality":    {"Renderer/nvidia_dlss_quality", map[string]string{"auto": "0", "native": "1", "quality": "2", "balanced": "3", "performance": "4", "ultra": "5"}},
	"enhance.dlss_model":      {"Renderer/nvidia_dlss_preset", map[string]string{"m": "0", "default": "1", "e": "2", "f": "3", "j": "4", "k": "5", "l": "6"}},
	"enhance.fsr4":            {"Renderer/amd_fsr4", nil},
	"audio.backend":           {"Audio/output_engine", map[string]string{"auto": "0", "cubeb": "1", "sdl": "2", "none": "3"}},
	"audio.channels":          {"System/sound_index", map[string]string{"mono": "0", "stereo": "1", "surround": "2"}},
	"audio.volume":            {"Audio/volume", nil},
	"audio.mute":              {"Audio/audio_muted", nil},
	"system.docked":           {"System/use_docked_mode", map[string]string{"handheld": "0", "docked": "1"}},
	"system.language":         {"System/language_index", seq("ja", "en-US", "fr", "de", "it", "es", "zh", "ko", "nl", "pt", "ru", "zh-TW", "en-GB", "fr-CA", "es-419", "zh-Hans", "zh-Hant", "pt-BR")},
	"system.region":           {"System/region_index", seq("japan", "usa", "europe", "australia", "china", "korea", "taiwan")},
}

// engineValue converts one NEXUS value; ok=false if the engine can't express it.
func engineValue(id, v string) (key, val string, ok bool) {
	b, found := engineBinds[id]
	if !found {
		return "", "", false
	}
	if b.vals == nil {
		return b.key, v, validOverride(b.key, v)
	}
	val, ok = b.vals[v]
	return b.key, val, ok
}

// engineProject turns NEXUS values into engine option values.
func engineProject(m map[string]string, perGameOnly bool) map[string]string {
	out := map[string]string{}
	for id, v := range m {
		key, val, ok := engineValue(id, v)
		if !ok {
			continue
		}
		if o := optionByID(key); o == nil || (perGameOnly && !o.PerGame) {
			continue
		}
		out[key] = val
	}
	return out
}

// engineReverse reads NEXUS values back from engine option values.
func engineReverse(vals map[string]string) map[string]string {
	out := map[string]string{}
	for id, b := range engineBinds {
		v, ok := vals[b.key]
		if !ok {
			continue
		}
		if b.vals == nil {
			out[id] = v
			continue
		}
		for nv, lv := range b.vals {
			if lv == v {
				out[id] = nv
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- capabilities

// Support says how an engine handles one NEXUS setting.
type Support struct {
	Available bool     `json:"available"`
	PerGame   bool     `json:"perGame"`
	Choices   []string `json:"choices,omitempty"` // supported values (enum); empty = all
	Note      string   `json:"note,omitempty"`
}

// EngineCaps says, for every NEXUS setting, whether the NEXUS engine applies it and whether it
// can differ per game. Only settings the engine really reads are offered.
func EngineCaps() map[string]Support {
	caps := map[string]Support{}
	for _, s := range NexusSettings {
		var sup Support
		if b, ok := engineBinds[s.ID]; ok {
			o := optionByID(b.key)
			sup = Support{Available: true, PerGame: o != nil && o.PerGame}
			if b.vals != nil {
				for _, c := range s.Choices {
					if _, ok := b.vals[c.V]; ok {
						sup.Choices = append(sup.Choices, c.V)
					}
				}
			}
			if !sup.PerGame {
				sup.Note = "This applies to all games."
			}
		}
		switch s.ID {
		case "graphics.resolution", "system.firmware":
			sup = Support{Available: true}
		case "audio.quality":
			sup = Support{Note: s.Help}
		case "controls.profile":
			sup = Support{Available: true, PerGame: true}
		case "graphics.fps_match_display": // applied by NEXUS when the session starts
			sup = Support{Available: true, PerGame: true}
		case "controls.mapping":
			sup.Note = "Buttons are set in Controllers (your layout for all games) or in a game's Controls page (its own layout)."
		case "controls.sensitivity":
			sup.Note = "Not adjustable from NEXUS yet; the engine's stick range and dead zone are used."
		}
		if !sup.Available && sup.Note == "" {
			sup.Note = "Not supported by the NEXUS engine."
		}
		caps[s.ID] = sup
	}
	return caps
}

// ---------------------------------------------------------------- storage

// EngineValues reads the values in the engine configuration NEXUS last wrote, as NEXUS values.
func (e *Emulator) EngineValues() map[string]string { return engineReverse(e.CurrentEmuValues()) }

func (e *Emulator) GlobalNexusSettings() map[string]string {
	out := map[string]string{}
	for k, v := range e.app.Settings.Get().Engine {
		out[k] = v
	}
	return out
}

// SetGlobal changes global NEXUS settings (nil = back to the engine's value).
func (e *Emulator) SetGlobal(changes map[string]*string) error {
	for id, v := range changes {
		s := nsetting(id)
		if s == nil || s.Kind == "info" || id == "controls.profile" {
			return fmt.Errorf("unknown setting %s", id)
		}
		if v != nil && !validNexusValue(id, *v) {
			return fmt.Errorf("invalid value for %s", s.Label)
		}
	}
	_, err := e.app.Settings.Update(func(g *GlobalSettings) {
		if g.Engine == nil {
			g.Engine = map[string]string{}
		}
		for id, v := range changes {
			if v == nil {
				delete(g.Engine, id)
			} else {
				g.Engine[id] = *v
			}
		}
	})
	return err
}

// SetGameSettings changes a game's overrides (nil = use global).
func (e *Emulator) SetGameSettings(g Game, changes map[string]*string, input *string) error {
	for id, v := range changes {
		s := nsetting(id)
		if s == nil || s.Kind == "info" || id == "controls.profile" {
			return fmt.Errorf("unknown setting %s", id)
		}
		if v != nil && !validNexusValue(id, *v) {
			return fmt.Errorf("invalid value for %s", s.Label)
		}
		if v != nil {
			if c := EngineCaps()[id]; !c.Available || !c.PerGame {
				return fmt.Errorf("%s cannot be set for a single game", s.Label)
			}
		}
	}
	if input != nil && *input != "" {
		found := false
		for _, ip := range e.InputProfiles() {
			found = found || ip == *input
		}
		if !found {
			return fmt.Errorf("controller profile %q does not exist", *input)
		}
	}
	_, err := e.app.Library.GP.Update(g.Key, func(p *GameProfile) {
		migrateGameSettings(p)
		if p.Settings == nil {
			p.Settings = map[string]string{}
		}
		for id, v := range changes {
			if v == nil {
				delete(p.Settings, id)
			} else {
				p.Settings[id] = *v
			}
		}
		if input != nil {
			p.InputProfile = *input
		}
		if p.TitleID == "" {
			p.TitleID = g.TitleID
		}
	})
	if err != nil {
		return err
	}
	e.app.Library.redecorate(g.Key)
	return nil // applied through the session configuration at the next launch
}

// migrateGameSettings moves 1.2–1.5 engine-specific overrides into NEXUS values.
func migrateGameSettings(p *GameProfile) {
	if len(p.Emu) == 0 && len(p.Ryu) == 0 {
		return
	}
	if p.Settings == nil {
		p.Settings = map[string]string{}
	}
	for id, v := range engineReverse(p.Emu) {
		if _, set := p.Settings[id]; !set {
			p.Settings[id] = v
		}
	}
	for k, v := range p.Ryu {
		if id, nv, ok := legacyRyuValue(k, v); ok {
			if _, set := p.Settings[id]; !set {
				p.Settings[id] = nv
			}
		}
	}
	// keep only engine keys that have no NEXUS equivalent
	rest := map[string]string{}
	mapped := map[string]bool{}
	for _, b := range engineBinds {
		mapped[b.key] = true
	}
	for k, v := range p.Emu {
		if !mapped[k] {
			rest[k] = v
		}
	}
	p.Emu = rest
	p.Ryu = nil
}

// migrateGlobalSettings does the same for the global engine overrides (NEXUS 1.2-1.5).
func migrateGlobalSettings(g *GlobalSettings) bool {
	if len(g.Emulator.Overrides) == 0 {
		return false
	}
	if g.Engine == nil {
		g.Engine = map[string]string{}
	}
	for id, v := range engineReverse(g.Emulator.Overrides) {
		if _, set := g.Engine[id]; !set {
			g.Engine[id] = v
		}
	}
	g.Emulator.Overrides = map[string]string{}
	return true
}

// gameNexusSettings returns a game's overrides (migrated view).
func gameNexusSettings(p GameProfile) map[string]string {
	q := p
	q.Settings = map[string]string{}
	for k, v := range p.Settings {
		q.Settings[k] = v
	}
	migrateGameSettings(&q)
	return q.Settings
}

var errNoConfig = errors.New("no configuration yet")

func resolutionText(scale, docked string) string {
	f, err := strconv.ParseFloat(scale, 64)
	if err != nil {
		f = 1
	}
	h := 1080.0
	mode := "docked"
	if docked == "handheld" {
		h, mode = 720, "handheld"
	}
	w := h * 16 / 9
	return fmt.Sprintf("%d × %d (%s, %s)", int(w*f+0.5), int(h*f+0.5), strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", f), "0"), ".0")+"×", mode)
}

// legacyRyuValue converts a per-game value saved by NEXUS 1.2-1.6 for the former second engine
// into a NEXUS setting, so those per-game choices survive the move to the NEXUS engine.
func legacyRyuValue(k, v string) (id, val string, ok bool) {
	switch k {
	case "res_scale":
		id, val = "graphics.scale", v
	case "graphics_backend":
		id, val = "graphics.backend", map[string]string{"Vulkan": "vulkan", "OpenGl": "opengl"}[v]
	case "aspect_ratio":
		id, val = "graphics.aspect", map[string]string{"Fixed16x9": "16:9", "Fixed4x3": "4:3", "Fixed16x10": "16:10", "Fixed21x9": "21:9", "Stretched": "stretch"}[v]
	case "max_anisotropy":
		id, val = "graphics.anisotropy", map[string]string{"-1": "auto", "2": "2", "4": "4", "8": "8", "16": "16"}[v]
	case "docked_mode":
		id, val = "system.docked", map[string]string{"true": "docked", "false": "handheld"}[v]
	case "audio_volume":
		id, val = "audio.volume", v
	}
	return id, val, id != "" && validNexusValue(id, val)
}
