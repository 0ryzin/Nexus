package main

// Engine option catalogue and helpers shared by the NEXUS engine host. Option keys and enum
// numbers were verified against the engine source (src/common/settings.h, settings_enums.h);
// the engine reads them from the session configuration NEXUS writes before each launch.

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// EmuOption describes one engine configuration key NEXUS manages.
type EmuOption struct {
	ID      string      `json:"id"` // Section/key
	Label   string      `json:"label"`
	Group   string      `json:"group"`
	Kind    string      `json:"kind"` // enum | bool | int
	Choices []EmuChoice `json:"choices,omitempty"`
	Min     int         `json:"min,omitempty"`
	Max     int         `json:"max,omitempty"`
	Step    int         `json:"step,omitempty"`
	Suffix  string      `json:"suffix,omitempty"`
	Help    string      `json:"help,omitempty"`
	PerGame bool        `json:"perGame"` // an engine SwitchableSetting: can differ per game
}

type EmuChoice struct {
	V string `json:"v"`
	L string `json:"l"`
}

var EmuOptions = []EmuOption{
	{ID: "Renderer/backend", Label: "Graphics API", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"1", "Vulkan"}, {"0", "OpenGL (GLSL)"}, {"3", "OpenGL (GLASM)"}, {"4", "OpenGL (SPIR-V)"}}},
	{ID: "Renderer/resolution_setup", Label: "Resolution scaling", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"1", "0.5× (360p/540p)"}, {"2", "0.75×"}, {"3", "1× native (720p/1080p)"}, {"4", "1.25×"}, {"5", "1.5×"}, {"6", "2× (1440p/2160p)"}, {"7", "3×"}, {"8", "4×"}}},
	{ID: "Renderer/use_vsync", Label: "V-Sync", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"2", "On (FIFO)"}, {"3", "Relaxed (FIFO relaxed)"}, {"1", "Mailbox"}, {"0", "Off (immediate)"}}},
	{ID: "Renderer/fullscreen_mode", Label: "Fullscreen mode", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Borderless window"}, {"1", "Exclusive fullscreen"}}},
	{ID: "Renderer/aspect_ratio", Label: "Aspect ratio", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "16:9"}, {"1", "4:3"}, {"2", "21:9"}, {"3", "16:10"}, {"4", "Stretch to window"}}},
	{ID: "Renderer/scaling_filter", Label: "Upscaling filter", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"15", "AMD FSR 3"}, {"6", "AMD FSR 1"}, {"1", "Bilinear"}, {"2", "Bicubic"}, {"4", "Lanczos"}, {"5", "ScaleForce"}, {"7", "Area"}, {"0", "Nearest neighbour"}}},
	{ID: "Renderer/anti_aliasing", Label: "Anti-aliasing", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "None"}, {"1", "FXAA"}, {"2", "SMAA"}}},
	{ID: "Renderer/max_anisotropy", Label: "Anisotropic filtering", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Automatic"}, {"1", "Game default"}, {"2", "2×"}, {"3", "4×"}, {"4", "8×"}, {"5", "16×"}}},
	{ID: "Renderer/gpu_accuracy", Label: "GPU accuracy", Group: "Performance", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Low (faster)"}, {"1", "High (more accurate)"}}},
	{ID: "Renderer/use_disk_shader_cache", Label: "Disk shader cache", Group: "Performance", Kind: "bool", PerGame: true},
	{ID: "Renderer/use_asynchronous_shaders", Label: "Asynchronous shader building", Group: "Performance", Kind: "bool", PerGame: true,
		Help: "Reduces stutter while shaders compile; may cause brief visual glitches."},
	{ID: "Renderer/use_reactive_flushing", Label: "Reactive flushing", Group: "Performance", Kind: "bool", PerGame: true,
		Help: "Improves accuracy in some games at a performance cost."},
	{ID: "Renderer/accelerate_astc", Label: "ASTC texture decoding", Group: "Performance", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"1", "GPU"}, {"0", "CPU"}, {"2", "CPU (asynchronous)"}}},
	{ID: "Renderer/vram_usage_mode", Label: "VRAM usage", Group: "Performance", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Conservative"}, {"1", "Aggressive"}}},
	{ID: "Renderer/nvdec_emulation", Label: "Video decoding (NVDEC)", Group: "Performance", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"2", "GPU"}, {"1", "CPU"}, {"0", "Off"}}},
	{ID: "Cpu/cpu_accuracy", Label: "CPU accuracy", Group: "Performance", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Auto"}, {"1", "Accurate"}, {"2", "Unsafe"}, {"3", "Paranoid"}}},
	{ID: "Core/use_multi_core", Label: "Multicore CPU emulation", Group: "Performance", Kind: "bool", PerGame: true},
	{ID: "Core/use_speed_limit", Label: "Limit emulation speed", Group: "Performance", Kind: "bool", PerGame: true},
	{ID: "Core/speed_limit", Label: "Speed limit", Group: "Performance", Kind: "int", Min: 25, Max: 400, Step: 25, Suffix: "%", PerGame: true},
	{ID: "System/use_docked_mode", Label: "Console mode", Group: "System", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"1", "Docked (TV)"}, {"0", "Handheld"}}},
	{ID: "System/sound_index", Label: "Sound output", Group: "Audio", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"1", "Stereo"}, {"0", "Mono"}, {"2", "Surround"}}},
	{ID: "Audio/volume", Label: "Volume", Group: "Audio", Kind: "int", Min: 0, Max: 200, Step: 10, Suffix: "%", PerGame: true},
	{ID: "Audio/audio_muted", Label: "Mute", Group: "Audio", Kind: "bool"},
	{ID: "Audio/output_engine", Label: "Audio output", Group: "Audio", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Automatic"}, {"1", "Cubeb"}, {"2", "SDL"}, {"3", "No audio"}}},
	// frame generation and upscaler replacement (settings.h: frame_gen*, nvidia_dlss*, amd_fsr4)
	{ID: "Renderer/frame_gen", Label: "Frame generation", Group: "Graphics", Kind: "bool", PerGame: true},
	{ID: "Renderer/frame_gen_type", Label: "Frame generation method", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"1", "AMD FSR 3"}, {"0", "Lossless Scaling"}}},
	{ID: "Renderer/frame_gen_multiplier", Label: "Generated frames", Group: "Graphics", Kind: "int", Min: 2, Max: 4, Step: 1, Suffix: "×", PerGame: true},
	{ID: "Renderer/frame_gen_target_rate", Label: "Frame generation target", Group: "Graphics", Kind: "int", Min: 0, Max: 240, Step: 1, Suffix: " fps", PerGame: true},
	{ID: "Renderer/nvidia_dlss", Label: "NVIDIA DLSS in place of the game's DLSS", Group: "Graphics", Kind: "bool", PerGame: true},
	{ID: "Renderer/nvidia_dlss_quality", Label: "DLSS quality", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Automatic"}, {"1", "Native (DLAA)"}, {"2", "Quality"}, {"3", "Balanced"}, {"4", "Performance"}, {"5", "Ultra Performance"}}},
	{ID: "Renderer/nvidia_dlss_preset", Label: "DLSS model", Group: "Graphics", Kind: "enum", PerGame: true,
		Choices: []EmuChoice{{"0", "Preset M"}, {"1", "NVIDIA default"}, {"2", "Preset E"}, {"3", "Preset F"}, {"4", "Preset J"}, {"5", "Preset K"}, {"6", "Preset L"}}},
	{ID: "Renderer/amd_fsr4", Label: "AMD FSR 4 in place of FSR 3.1", Group: "Graphics", Kind: "bool", PerGame: true},
	{ID: "System/language_index", Label: "System language", Group: "System", Kind: "int", Min: 0, Max: 17, Step: 1, PerGame: true},
	{ID: "System/region_index", Label: "Console region", Group: "System", Kind: "int", Min: 0, Max: 6, Step: 1, PerGame: true},
}

func splitOpt(id string) (string, string) {
	sec, key, _ := strings.Cut(id, "/")
	return sec, key
}

func validOverride(id, v string) bool {
	for _, o := range EmuOptions {
		if o.ID != id {
			continue
		}
		switch o.Kind {
		case "bool":
			return v == "true" || v == "false"
		case "int":
			n, err := strconv.Atoi(v)
			return err == nil && n >= o.Min && n <= o.Max
		case "enum":
			for _, c := range o.Choices {
				if c.V == v {
					return true
				}
			}
		}
	}
	return false
}

// seedNewConfig applies the first-run defaults the engine's upstream frontend writes into a
// brand-new configuration (frontend_common/config.cpp), so a new NEXUS install starts with them.
func seedNewConfig(ini *IniFile) {
	ini.SetSetting("Renderer", "scaling_filter", "15")
	ini.SetSetting("Renderer", "gpu_accuracy", "0")
	ini.SetSetting("Renderer", "nvdec_emulation", "2")
	ini.SetSetting("Renderer", "async_presentation", "true")
	ini.SetSetting("Renderer", "use_asynchronous_shaders", "true")
}

type SaveTitle struct {
	TitleID string `json:"titleId"`
	User    string `json:"user"`
	Bytes   int64  `json:"bytes"`
	Files   int    `json:"files"`
}

// ListSaves lists save folders found under <root>/user/save/0000000000000000/<user>/<titleid>.
func ListSaves(root string) []SaveTitle {
	out := []SaveTitle{}
	// emulated NAND layouts: user/save/0000000000000000/<user>/<title> and user/save/account/<uuid>/<title>
	for _, sub := range []string{"0000000000000000", "account"} {
		base := filepath.Join(root, "user", "save", sub)
		users, _ := os.ReadDir(base)
		for _, u := range users {
			if !u.IsDir() {
				continue
			}
			titles, _ := os.ReadDir(filepath.Join(base, u.Name()))
			for _, t := range titles {
				if !t.IsDir() {
					continue
				}
				b, n := dirSize(filepath.Join(base, u.Name(), t.Name()))
				out = append(out, SaveTitle{TitleID: strings.ToUpper(t.Name()), User: u.Name(), Bytes: b, Files: n})
			}
		}
	}
	return out
}

type LaunchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	// Technical: the engine's own message and last log lines, shown only on request
	Technical string `json:"technical,omitempty"`
	// Log: the diagnostic file written for this failure
	Log string `json:"log,omitempty"`
}

func (le *LaunchError) Error() string { return le.Message }

func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ' ' && !inQ:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func tailFile(path string, n int, keep func(string) bool) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > 256<<10 {
		data = data[len(data)-256<<10:]
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || (keep != nil && !keep(l)) {
			continue
		}
		if len(l) > 220 {
			l = l[:220] + "…"
		}
		out = append([]string{l}, out...)
	}
	return strings.Join(out, "\n")
}

func cleanOldLogs(dir, prefix string, keep int) {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, en := range entries {
		if strings.HasPrefix(en.Name(), prefix) {
			names = append(names, en.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// Stop asks the emulator to close gracefully; force=true kills it.

func optionByID(id string) *EmuOption {
	for i := range EmuOptions {
		if EmuOptions[i].ID == id {
			return &EmuOptions[i]
		}
	}
	return nil
}
