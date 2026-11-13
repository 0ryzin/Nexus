package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// "Optimise for my device": hardware-based recommendations for the settings the NEXUS engine
// really applies (see EngineCaps). It is an informed starting point, not a benchmark: NEXUS
// does not promise a frame rate it has not measured.

type OptProfile string

const (
	OptPerformance OptProfile = "performance"
	OptBalanced    OptProfile = "balanced"
	OptQuality     OptProfile = "quality"
)

// DeviceClass is how NEXUS rates this PC for Switch emulation.
type DeviceClass struct {
	GPUTier   int      `json:"gpuTier"` // 0 basic … 4 enthusiast
	GPULabel  string   `json:"gpuLabel"`
	CPUTier   int      `json:"cpuTier"` // 0 weak, 1 capable, 2 strong
	CPULabel  string   `json:"cpuLabel"`
	LowMemory bool     `json:"lowMemory"`
	Vulkan    bool     `json:"vulkan"`
	Display   string   `json:"display"`
	Reasons   []string `json:"reasons"`
}

type OptChange struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	From   string `json:"from"` // current effective value ("" = engine default)
	To     string `json:"to"`   // recommended value
	FromL  string `json:"fromLabel"`
	ToL    string `json:"toLabel"`
	Reason string `json:"reason"`
	Same   bool   `json:"same"` // already set like this
}

type OptPlan struct {
	Profile  OptProfile  `json:"profile"`
	Scope    string      `json:"scope"` // global | game
	GameKey  string      `json:"gameKey,omitempty"`
	Game     string      `json:"game,omitempty"`
	Demand   string      `json:"demand"` // standard | heavy | light
	Class    DeviceClass `json:"class"`
	Changes  []OptChange `json:"changes"`
	Skipped  []string    `json:"skipped,omitempty"` // settings that cannot be set per game, etc.
	Notes    []string    `json:"notes"`
	Hardware Hardware    `json:"hardware"`
}

// ---------- classification

var (
	reModernIGPU = regexp.MustCompile(`(?i)radeon\s*(7[0-9]0m|8[0-9]0m|6[0-9]0m)|iris\s*xe|arc\s*(graphics|1[2-9]|1[0-9]0v)|radeon\s*890m|adreno`)
	reHeavyGame  = regexp.MustCompile(`(?i)tears of the kingdom|breath of the wild|xenoblade|pok[eé]mon (scarlet|violet|legends)|metroid prime|the witcher|doom|bayonetta 3|fire emblem engage|super mario bros\. wonder|kirby and the forgotten land|monster hunter|hogwarts|mortal kombat|astral chain|triangle strategy`)
)

func classifyDevice(h Hardware) DeviceClass {
	c := DeviceClass{}
	g := h.PrimaryGPU()
	// prefer a discrete GPU if the PC has one
	for i := range h.GPUs {
		if h.GPUs[i].Kind == "discrete" && (g == nil || g.Kind != "discrete" || h.GPUs[i].DedicatedMB > g.DedicatedMB) {
			g = &h.GPUs[i]
		}
	}
	switch {
	case g == nil:
		c.GPUTier, c.GPULabel = 1, "unknown graphics"
		c.Reasons = append(c.Reasons, "The graphics card could not be detected, so NEXUS assumes an entry-level GPU.")
	case g.Kind == "discrete" || (g.Kind == "unknown" && g.DedicatedMB > 1024):
		vram := g.DedicatedMB
		switch {
		case vram >= 12*1024:
			c.GPUTier = 4
		case vram >= 8*1024:
			c.GPUTier = 3
		case vram >= 4*1024:
			c.GPUTier = 2
		default:
			c.GPUTier = 1
		}
		c.GPULabel = fmt.Sprintf("%s, %d GB video memory", g.Model, (vram+512)/1024)
		c.Reasons = append(c.Reasons, fmt.Sprintf("Dedicated graphics: %s.", c.GPULabel))
	default: // integrated
		c.GPUTier = 0
		if reModernIGPU.MatchString(g.Model) {
			c.GPUTier = 1
		}
		c.GPULabel = g.Model + " (integrated)"
		c.Reasons = append(c.Reasons, "Integrated graphics share the system memory, so NEXUS keeps the rendering load moderate.")
	}
	c.Vulkan = h.Vulkan.Available || (g != nil && g.VulkanAPI != "")
	if g != nil && g.VulkanAPI == "" && !h.Vulkan.Available && h.Source == "detected" && len(h.GPUs) > 0 {
		c.Reasons = append(c.Reasons, "No Vulkan support was found, so OpenGL is recommended.")
	}
	threads := h.CPU.Threads
	avx2 := false
	for _, f := range h.CPU.Features {
		avx2 = avx2 || f == "AVX2"
	}
	switch {
	case threads >= 8 && avx2:
		c.CPUTier, c.CPULabel = 2, "strong"
	case threads >= 4:
		c.CPUTier, c.CPULabel = 1, "capable"
	default:
		c.CPUTier, c.CPULabel = 0, "limited"
	}
	c.Reasons = append(c.Reasons, fmt.Sprintf("Processor: %d threads%s (%s for the engine's CPU emulation).", threads, map[bool]string{true: ", AVX2", false: ""}[avx2], c.CPULabel))
	if h.Memory.TotalMB > 0 && h.Memory.TotalMB < 12*1024 {
		c.LowMemory = true
		c.Reasons = append(c.Reasons, fmt.Sprintf("%d GB of RAM: NEXUS uses conservative video-memory settings.", (h.Memory.TotalMB+512)/1024))
	}
	if d := h.PrimaryDisplay(); d != nil && d.Height > 0 {
		c.Display = fmt.Sprintf("%d × %d at %d Hz", d.Width, d.Height, d.RefreshHz)
	}
	return c
}

func gameDemand(g *Game, compat CompatView) (string, string) {
	if g == nil {
		return "standard", ""
	}
	if reHeavyGame.MatchString(g.Title) {
		return "heavy", g.Title + " is known to be demanding, so NEXUS recommends one step less rendering load than for typical games."
	}
	if compat.Crashes > 0 {
		return "stability", "This game has stopped unexpectedly in NEXUS before, so the recommendation favours accuracy and stability."
	}
	return "standard", ""
}

// ---------- recommendation

func scaleStep(scale string, d int) string {
	steps := []string{"0.5", "0.75", "1", "1.25", "1.5", "2", "3", "4"}
	i := 2
	for k, s := range steps {
		if s == scale {
			i = k
		}
	}
	i = max(0, min(len(steps)-1, i+d))
	return steps[i]
}

// recommend returns NEXUS setting values (id -> value) with a reason for each.
func recommend(h Hardware, c DeviceClass, p OptProfile, demand string) (map[string]string, map[string]string) {
	v, why := map[string]string{}, map[string]string{}
	set := func(id, val, reason string) { v[id], why[id] = val, reason }
	t := c.GPUTier
	dispH := 1080
	refresh := 60
	if d := h.PrimaryDisplay(); d != nil && d.Height > 0 {
		dispH, refresh = d.Height, d.RefreshHz
	}

	// Graphics API
	if c.Vulkan {
		set("graphics.backend", "vulkan", "Vulkan is the engine's fastest renderer and your GPU supports it.")
	} else {
		set("graphics.backend", "opengl", "No Vulkan support was detected; OpenGL works on all supported drivers.")
	}

	// Console mode and resolution scaling (docked = 1080p base, handheld = 720p)
	docked := "docked"
	if p == OptPerformance && t == 0 {
		docked = "handheld"
	}
	set("system.docked", docked, map[string]string{
		"docked":   "Docked mode renders at the game's full 1080p base resolution.",
		"handheld": "Handheld mode renders at 720p, which weak integrated graphics handle much better.",
	}[docked])
	base := map[OptProfile][]string{
		OptPerformance: {"0.75", "1", "1", "1", "1.5"},
		OptBalanced:    {"1", "1", "1.5", "2", "2"},
		OptQuality:     {"1", "1.25", "1.5", "2", "3"},
	}[p][t]
	scale := base
	if demand == "heavy" && p != OptQuality {
		scale = scaleStep(scale, -1)
	}
	// no point rendering far above the display: cap at about 1.5x the display height
	baseH := 1080
	if docked == "handheld" {
		baseH = 720
	}
	for scale != "0.5" && float64(baseH)*atofSafe(scale) > float64(dispH)*1.5+1 && !(p == OptQuality && t >= 4) {
		scale = scaleStep(scale, -1)
	}
	render := int(float64(baseH) * atofSafe(scale))
	set("graphics.scale", scale, fmt.Sprintf("Renders at about %dp for a %dp display with %s graphics.", render, dispH, map[int]string{0: "basic", 1: "entry-level", 2: "mid-range", 3: "high-end", 4: "very high-end"}[t]))

	// Upscaling / downscaling filter
	switch {
	case render > dispH:
		set("adv.filter", "area", "The image is rendered above your display resolution; area filtering downsamples it cleanly.")
	case render < dispH && p == OptPerformance:
		set("adv.filter", "fsr1", "AMD FSR 1 upscales the lower render resolution sharply at almost no cost.")
	case render < dispH:
		set("adv.filter", "fsr3", "AMD FSR 3 gives the sharpest upscale to your display resolution.")
	default:
		set("adv.filter", "bilinear", "The render resolution matches your display, so a simple filter is enough.")
	}

	// Anti-aliasing: cheap FXAA when rendering at or below native, none when supersampling
	switch {
	case render > dispH:
		set("graphics.aa", "none", "Rendering above your display resolution already smooths edges.")
	case p == OptQuality && t >= 2:
		set("graphics.aa", "smaa", "SMAA smooths edges well and your GPU has headroom for it.")
	case p == OptPerformance && t == 0:
		set("graphics.aa", "none", "Anti-aliasing is skipped to keep the frame rate up on this GPU.")
	default:
		set("graphics.aa", "fxaa", "FXAA smooths edges at very little cost.")
	}

	// Anisotropic filtering
	af := map[OptProfile][]string{
		OptPerformance: {"default", "default", "4", "8", "8"},
		OptBalanced:    {"default", "4", "8", "16", "16"},
		OptQuality:     {"4", "8", "16", "16", "16"},
	}[p][t]
	set("graphics.anisotropy", af, "Sharper textures at angles; its cost depends mostly on the GPU's memory bandwidth.")

	// VSync / frame pacing
	switch {
	case p == OptPerformance:
		set("graphics.vsync", "mailbox", "Mailbox avoids tearing without the extra input delay of standard VSync.")
	case refresh > 60:
		set("graphics.vsync", "mailbox", fmt.Sprintf("Your %d Hz display is faster than the games' 30/60 fps; mailbox presents frames without waiting.", refresh))
	default:
		set("graphics.vsync", "fifo", "Standard VSync gives the most even frame pacing on a 60 Hz display.")
	}

	// Shaders
	set("adv.shader_cache", "true", "Compiled shaders are kept on disk, so stutter happens only the first time.")
	if p == OptQuality && t >= 3 && c.CPUTier >= 2 && demand != "heavy" {
		set("adv.async_shaders", "false", "Your CPU compiles shaders quickly, so they are built before drawing (no brief visual glitches).")
	} else {
		set("adv.async_shaders", "true", "Shaders are compiled in the background, which avoids stutter.")
	}

	// GPU accuracy and flushing
	if (p == OptQuality && t >= 3) || demand == "stability" {
		set("adv.gpu_accuracy", "high", "High GPU accuracy fixes rare rendering errors at a performance cost.")
	} else {
		set("adv.gpu_accuracy", "low", "Low GPU accuracy is faster and correct for most games.")
	}
	if p == OptQuality && t >= 3 {
		set("adv.reactive_flushing", "true", "Improves accuracy of some effects; your GPU can afford it.")
	} else {
		set("adv.reactive_flushing", "false", "Off for speed; it is only needed by a few games.")
	}

	// Textures and video memory
	if t == 0 {
		set("adv.astc", "cpu_async", "Decoding ASTC textures on the CPU in the background saves the integrated GPU's limited memory.")
	} else {
		set("adv.astc", "gpu", "Your GPU decodes ASTC textures fastest.")
	}
	g := h.PrimaryGPU()
	if c.LowMemory || t <= 1 || (g != nil && g.DedicatedMB > 0 && g.DedicatedMB < 6*1024) {
		set("adv.vram", "conservative", "Conservative video-memory use avoids running out of memory.")
	} else {
		set("adv.vram", "aggressive", "Plenty of video memory: keeping more textures resident reduces stutter.")
	}

	// CPU emulation
	if h.CPU.Threads >= 4 {
		set("adv.multicore", "true", "Multicore emulation uses several CPU threads and is much faster.")
	} else {
		set("adv.multicore", "false", "With fewer than 4 threads, single-core emulation avoids contention.")
	}
	if demand == "stability" {
		set("adv.cpu_accuracy", "accurate", "Accurate CPU emulation is safest for a game that has crashed before.")
	} else {
		set("adv.cpu_accuracy", "auto", "Automatic chooses safe optimisations per game.")
	}
	set("adv.speed_limit_on", "true", "Keeps games at their intended speed.")
	return v, why
}

// isSwitch2Title reports whether a title ID belongs to a Switch 2 program (04xxxxxxxxxxxxxx).
func isSwitch2Title(id string) bool {
	return len(id) == 16 && strings.HasPrefix(strings.ToUpper(id), "04")
}

// switch2Limits keeps a recommendation for a Switch 2 game on the configuration the engine
// (Lichii build 177) ships and tests with. Switch 2 games already render at up to 4K and use
// 12 GB of emulated memory, and some wait on GPU read-backs while loading: turning reactive
// flushing off, keeping more textures resident or rendering above 1x trades stability for
// little gain there. See docs/COMPATIBILITY.md (Ocarina of Time) and docs/PERFORMANCE.md.
func switch2Limits(v, why map[string]string) {
	v["adv.reactive_flushing"] = "true"
	why["adv.reactive_flushing"] = "Switch 2 games can wait on the GPU while loading; reactive flushing is the engine's tested default."
	v["adv.vram"] = "conservative"
	why["adv.vram"] = "Switch 2 games use 12 GB of emulated memory; conservative video memory leaves room on 16 GB PCs."
	if atofSafe(v["graphics.scale"]) > 1 {
		v["graphics.scale"] = "1"
		why["graphics.scale"] = "Switch 2 games already render at up to 4K docked, so no extra scaling is applied."
		if v["adv.filter"] == "area" {
			v["adv.filter"] = "bilinear"
			why["adv.filter"] = "No resolution scaling, so a simple filter is enough."
		}
		if v["graphics.aa"] == "none" {
			v["graphics.aa"] = "fxaa"
			why["graphics.aa"] = "FXAA smooths edges at very little cost."
		}
	}
}

func atofSafe(s string) float64 {
	var f float64
	_, err := fmt.Sscanf(s, "%g", &f)
	if err != nil || f <= 0 {
		return 1
	}
	return f
}

// ---------- plan, apply, restore

// Plan builds the recommendation for global settings (gameKey == "") or for one game.
func (e *Emulator) Plan(p OptProfile, gameKey string) (OptPlan, error) {
	if p != OptPerformance && p != OptBalanced && p != OptQuality {
		return OptPlan{}, errors.New("profile must be performance, balanced or quality")
	}
	h := e.app.DetectHardware(false)
	c := classifyDevice(h)
	plan := OptPlan{Profile: p, Scope: "global", Class: c, Hardware: h, Demand: "standard"}
	var game *Game
	if gameKey != "" {
		g, ok := e.app.Library.Get(gameKey)
		if !ok {
			return plan, errors.New("Game not found.")
		}
		game = &g
		plan.Scope, plan.GameKey, plan.Game = "game", g.Key, g.Title
		d, note := gameDemand(game, e.app.Compat.Get(g.Key))
		plan.Demand = d
		if note != "" {
			plan.Notes = append(plan.Notes, note)
		}
	}
	vals, why := recommend(h, c, p, plan.Demand)
	if game != nil && isSwitch2Title(game.TitleID) {
		switch2Limits(vals, why)
		plan.Notes = append(plan.Notes, game.Title+" is a Switch 2 game, so NEXUS keeps the settings the engine is tested with: reactive flushing on, conservative video memory and no resolution scaling above 1x.")
	}
	caps := EngineCaps()
	global := e.GlobalNexusSettings()
	engineNow := e.EngineValues()
	var over map[string]string
	if game != nil {
		over = gameNexusSettings(e.app.Library.GP.Get(game.Key))
	}
	ids := make([]string, 0, len(vals))
	for id := range vals {
		ids = append(ids, id)
	}
	order := map[string]int{}
	for i, s := range NexusSettings {
		order[s.ID] = i
	}
	sort.Slice(ids, func(i, j int) bool { return order[ids[i]] < order[ids[j]] })
	for _, id := range ids {
		st := nsetting(id)
		cap := caps[id]
		if st == nil || !cap.Available || !validNexusValue(id, vals[id]) {
			continue // only settings the engine really applies
		}
		if len(cap.Choices) > 0 && !contains(cap.Choices, vals[id]) {
			continue
		}
		if game != nil && !cap.PerGame {
			plan.Skipped = append(plan.Skipped, st.Label+" applies to all games (change it in the global optimisation).")
			continue
		}
		cur := global[id]
		if cur == "" {
			cur = engineNow[id]
		}
		if game != nil {
			if v, ok := over[id]; ok {
				cur = v
			}
		}
		ch := OptChange{ID: id, Label: st.Label, From: cur, To: vals[id], FromL: valueLabel(st, cur), ToL: valueLabel(st, vals[id]), Reason: why[id]}
		ch.Same = cur == vals[id]
		plan.Changes = append(plan.Changes, ch)
	}
	plan.Notes = append(plan.Notes, "These settings are chosen from your hardware, not measured in the game. Frame rates depend on the game; try it and adjust if needed.")
	return plan, nil
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func valueLabel(st *NSetting, v string) string {
	if v == "" {
		return "Engine default"
	}
	switch st.Kind {
	case "bool":
		if v == "true" {
			return "On"
		}
		return "Off"
	case "enum":
		for _, c := range st.Choices {
			if c.V == v {
				return c.L
			}
		}
	}
	return v + st.Suffix
}

// optimiser backups: the values in place before each optimisation, so they can be restored.
type optBackup struct {
	At       int64              `json:"at"`
	Profile  OptProfile         `json:"profile"`
	Previous map[string]*string `json:"previous"` // nil = was not set
}

type optStore struct {
	Global *optBackup            `json:"global,omitempty"`
	Games  map[string]*optBackup `json:"games,omitempty"`
}

var optMu sync.Mutex

func (e *Emulator) optPath() string { return filepath.Join(e.app.Paths.Settings, "optimiser.json") }

func (e *Emulator) loadOpt() optStore {
	var st optStore
	_ = readJSON(e.optPath(), &st)
	if st.Games == nil {
		st.Games = map[string]*optBackup{}
	}
	return st
}

// ApplyPlan applies the recommendation (only the listed ids; empty = all changes) and keeps the
// previous values for Restore. Per-game plans change only that game's own settings.
func (e *Emulator) ApplyPlan(p OptProfile, gameKey string, only []string) (OptPlan, error) {
	if e.State().Running {
		return OptPlan{}, errors.New("Close the running game before changing its settings.")
	}
	plan, err := e.Plan(p, gameKey)
	if err != nil {
		return plan, err
	}
	want := map[string]bool{}
	for _, id := range only {
		want[id] = true
	}
	changes := map[string]*string{}
	prev := map[string]*string{}
	var current map[string]string
	if gameKey == "" {
		current = e.GlobalNexusSettings()
	} else {
		current = gameNexusSettings(e.app.Library.GP.Get(gameKey))
	}
	for _, ch := range plan.Changes {
		if ch.Same || (len(want) > 0 && !want[ch.ID]) {
			continue
		}
		to := ch.To
		changes[ch.ID] = &to
		if v, ok := current[ch.ID]; ok {
			vv := v
			prev[ch.ID] = &vv
		} else {
			prev[ch.ID] = nil
		}
	}
	if len(changes) == 0 {
		return plan, nil
	}
	if gameKey == "" {
		err = e.SetGlobal(changes)
	} else {
		g, _ := e.app.Library.Get(gameKey)
		err = e.SetGameSettings(g, changes, nil)
	}
	if err != nil {
		return plan, err
	}
	optMu.Lock()
	st := e.loadOpt()
	b := &optBackup{At: time.Now().Unix(), Profile: p, Previous: prev}
	if gameKey == "" {
		st.Global = b
	} else {
		st.Games[gameKey] = b
	}
	err = writeJSONAtomic(e.optPath(), st)
	optMu.Unlock()
	e.app.Log.Printf("optimiser: %s profile applied to %s (%d settings)", p, map[bool]string{true: "global settings", false: gameKey}[gameKey == ""], len(changes))
	return e.Plan(p, gameKey)
}

// OptBackupInfo says whether settings from before the last optimisation can be restored.
func (e *Emulator) OptBackupInfo(gameKey string) *optBackup {
	optMu.Lock()
	defer optMu.Unlock()
	st := e.loadOpt()
	if gameKey == "" {
		return st.Global
	}
	return st.Games[gameKey]
}

// RestoreOptimisation puts back the values from before the last optimisation.
func (e *Emulator) RestoreOptimisation(gameKey string) error {
	if e.State().Running {
		return errors.New("Close the running game first.")
	}
	optMu.Lock()
	st := e.loadOpt()
	b := st.Global
	if gameKey != "" {
		b = st.Games[gameKey]
	}
	optMu.Unlock()
	if b == nil {
		return errors.New("There are no earlier settings to restore.")
	}
	var err error
	if gameKey == "" {
		err = e.SetGlobal(b.Previous)
	} else {
		g, ok := e.app.Library.Get(gameKey)
		if !ok {
			return errors.New("Game not found.")
		}
		err = e.SetGameSettings(g, b.Previous, nil)
	}
	if err != nil {
		return err
	}
	optMu.Lock()
	st = e.loadOpt()
	if gameKey == "" {
		st.Global = nil
	} else {
		delete(st.Games, gameKey)
	}
	err = writeJSONAtomic(e.optPath(), st)
	optMu.Unlock()
	return err
}
