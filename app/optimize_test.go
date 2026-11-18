package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var (
	hwWeakLaptop = Hardware{Source: "override",
		CPU:      CPUInfo{Vendor: "Intel", Model: "Intel Core i3-1005G1", Cores: 2, Threads: 4, Features: []string{"SSE4.2", "AVX", "AVX2"}},
		GPUs:     []GPUInfo{{Vendor: "Intel", Model: "Intel(R) UHD Graphics", VendorID: 0x8086, DedicatedMB: 128, Kind: "integrated", Integrated: true, VulkanAPI: "1.3.215", Primary: true}},
		Memory:   MemoryInfo{TotalMB: 8 * 1024, AvailableMB: 3 * 1024},
		Displays: []Display{{Width: 1366, Height: 768, RefreshHz: 60, Primary: true}},
		Vulkan:   VulkanInfo{Loader: true, Available: true}}
	hwMidDesktop = Hardware{Source: "override",
		CPU:      CPUInfo{Vendor: "AMD", Model: "AMD Ryzen 5 3600", Cores: 6, Threads: 12, Features: []string{"AVX", "AVX2"}},
		GPUs:     []GPUInfo{{Vendor: "NVIDIA", Model: "NVIDIA GeForce GTX 1660 SUPER", VendorID: 0x10DE, DedicatedMB: 6 * 1024, Kind: "discrete", VulkanAPI: "1.3.277", Primary: true}},
		Memory:   MemoryInfo{TotalMB: 16 * 1024, AvailableMB: 9 * 1024},
		Displays: []Display{{Width: 1920, Height: 1080, RefreshHz: 144, Primary: true}},
		Vulkan:   VulkanInfo{Loader: true, Available: true}}
	hwHighEnd = Hardware{Source: "override",
		CPU:      CPUInfo{Vendor: "AMD", Model: "AMD Ryzen 7 7800X3D", Cores: 8, Threads: 16, Features: []string{"AVX", "AVX2", "AVX-512F"}},
		GPUs:     []GPUInfo{{Vendor: "NVIDIA", Model: "NVIDIA GeForce RTX 4080", VendorID: 0x10DE, DedicatedMB: 16 * 1024, Kind: "discrete", VulkanAPI: "1.3.277", Primary: true}},
		Memory:   MemoryInfo{TotalMB: 32 * 1024},
		Displays: []Display{{Width: 3840, Height: 2160, RefreshHz: 60, Primary: true}},
		Vulkan:   VulkanInfo{Loader: true, Available: true}}
	hwNoVulkan = Hardware{Source: "override",
		CPU:  CPUInfo{Threads: 8, Features: []string{"AVX2"}},
		GPUs: []GPUInfo{{Vendor: "AMD", Model: "AMD Radeon HD 7850", VendorID: 0x1002, DedicatedMB: 2048, Kind: "discrete", Primary: true}}}
)

func useHardware(t *testing.T, h Hardware) {
	p := filepath.Join(t.TempDir(), "hw.json")
	b, _ := json.Marshal(h)
	os.WriteFile(p, b, 0o644)
	t.Setenv("NEXUS_HARDWARE", p)
	hwMu.Lock()
	hwCache = nil
	hwMu.Unlock()
}

func TestRecommendationsFollowHardware(t *testing.T) {
	type want map[string]string
	cases := []struct {
		name string
		hw   Hardware
		p    OptProfile
		w    want
	}{
		{"weak laptop, performance", hwWeakLaptop, OptPerformance, want{"system.docked": "handheld", "graphics.scale": "0.75", "adv.astc": "cpu_async", "adv.vram": "conservative", "graphics.aa": "none", "adv.filter": "fsr1"}},
		{"weak laptop, balanced", hwWeakLaptop, OptBalanced, want{"system.docked": "docked", "graphics.scale": "1", "adv.gpu_accuracy": "low", "adv.multicore": "true"}},
		{"mid desktop, balanced", hwMidDesktop, OptBalanced, want{"graphics.scale": "1.5", "graphics.aa": "none", "adv.filter": "area", "graphics.vsync": "mailbox", "adv.vram": "aggressive", "graphics.anisotropy": "8"}},
		{"high end 4K, quality", hwHighEnd, OptQuality, want{"graphics.scale": "3", "adv.gpu_accuracy": "high", "adv.async_shaders": "false", "graphics.anisotropy": "16", "adv.reactive_flushing": "true"}},
		{"high end 4K, performance", hwHighEnd, OptPerformance, want{"graphics.scale": "1.5", "adv.gpu_accuracy": "low", "adv.async_shaders": "true"}},
		{"no vulkan", hwNoVulkan, OptBalanced, want{"graphics.backend": "opengl"}},
	}
	for _, c := range cases {
		cl := classifyDevice(c.hw)
		v, why := recommend(c.hw, cl, c.p, "standard")
		for id, val := range v {
			if !validNexusValue(id, val) {
				t.Errorf("%s: %s=%q is not a valid NEXUS value", c.name, id, val)
			}
			if !EngineCaps()[id].Available {
				t.Errorf("%s: %s is not applied by the engine", c.name, id)
			}
			if why[id] == "" {
				t.Errorf("%s: %s has no reason", c.name, id)
			}
		}
		for id, val := range c.w {
			if v[id] != val {
				t.Errorf("%s: %s = %q, want %q", c.name, id, v[id], val)
			}
		}
	}
	// a known demanding game gets one step less load than a typical game
	cl := classifyDevice(hwMidDesktop)
	heavy, _ := recommend(hwMidDesktop, cl, OptBalanced, "heavy")
	if heavy["graphics.scale"] != "1.25" {
		t.Fatalf("heavy game scale %s", heavy["graphics.scale"])
	}
	// performance never chooses a higher load than balanced, balanced never above quality
	rank := map[string]int{"0.5": 0, "0.75": 1, "1": 2, "1.25": 3, "1.5": 4, "2": 5, "3": 6, "4": 7}
	for _, h := range []Hardware{hwWeakLaptop, hwMidDesktop, hwHighEnd} {
		cl := classifyDevice(h)
		pv, _ := recommend(h, cl, OptPerformance, "standard")
		bv, _ := recommend(h, cl, OptBalanced, "standard")
		qv, _ := recommend(h, cl, OptQuality, "standard")
		if rank[pv["graphics.scale"]] > rank[bv["graphics.scale"]] || rank[bv["graphics.scale"]] > rank[qv["graphics.scale"]] {
			t.Fatalf("%s: scales %s/%s/%s", h.CPU.Model, pv["graphics.scale"], bv["graphics.scale"], qv["graphics.scale"])
		}
	}
}

func TestOptimiseApplyRestoreAndPerGame(t *testing.T) {
	app, root := newTestApp(t, "")
	useHardware(t, hwMidDesktop)
	app.Library.Scan()
	a, _ := app.Library.Get("t-0100aaaa00010000")
	b, _ := app.Library.Get("t-0100aaaa00020000")
	app.Emu.SetGlobal(map[string]*string{"graphics.scale": sp("0.5"), "graphics.aa": sp("smaa")})
	app.Emu.SetGameSettings(b, map[string]*string{"graphics.scale": sp("4")}, nil)

	plan, err := app.Emu.Plan(OptBalanced, "")
	if err != nil || plan.Class.GPUTier != 2 || len(plan.Changes) == 0 {
		t.Fatalf("plan %+v %v", plan.Class, err)
	}
	var scale *OptChange
	for i := range plan.Changes {
		if plan.Changes[i].ID == "graphics.scale" {
			scale = &plan.Changes[i]
		}
	}
	if scale == nil || scale.From != "0.5" || scale.To != "1.5" || scale.Same {
		t.Fatalf("the plan must show what changes: %+v", scale)
	}
	if _, err := app.Emu.ApplyPlan(OptBalanced, "", nil); err != nil {
		t.Fatal(err)
	}
	g := app.Emu.GlobalNexusSettings()
	if g["graphics.scale"] != "1.5" || g["graphics.aa"] != "none" || g["graphics.backend"] != "vulkan" {
		t.Fatalf("global not applied: %v", g)
	}
	// the engine really receives it: session config for game A
	app.Emu.writeSessionConfig(&a, nil)
	ini, _ := LoadIni(app.Emu.SessionPath())
	if v, _ := ini.GetSetting("Renderer", "resolution_setup"); v != "5" { // 1.5x
		t.Fatalf("engine resolution_setup %q", v)
	}
	// game B kept its own 4x override
	if v := gameNexusSettings(app.Library.GP.Get(b.Key))["graphics.scale"]; v != "4" {
		t.Fatalf("global optimisation overwrote game B's setting: %q", v)
	}
	// persists across restart
	app2, _ := testNewApp(t, root)
	if app2.Emu.GlobalNexusSettings()["graphics.scale"] != "1.5" || app2.Emu.OptBackupInfo("") == nil {
		t.Fatal("optimisation not persisted")
	}
	// restore puts the previous values back, including "not set"
	if err := app2.Emu.RestoreOptimisation(""); err != nil {
		t.Fatal(err)
	}
	g = app2.Emu.GlobalNexusSettings()
	if g["graphics.scale"] != "0.5" || g["graphics.aa"] != "smaa" {
		t.Fatalf("restore: %v", g)
	}
	if _, set := g["graphics.backend"]; set {
		t.Fatal("a setting that was not set before must be unset again")
	}
	if app2.Emu.RestoreOptimisation("") == nil {
		t.Fatal("nothing left to restore")
	}

	// per game: only game A changes, only per-game settings, game B untouched
	app2.Library.Scan()
	beforeB := gameNexusSettings(app2.Library.GP.Get(b.Key))
	gp, err := app2.Emu.ApplyPlan(OptPerformance, a.Key, []string{"graphics.scale", "graphics.anisotropy"})
	if err != nil {
		t.Fatal(err)
	}
	oa := gameNexusSettings(app2.Library.GP.Get(a.Key))
	if oa["graphics.scale"] != "1" || oa["graphics.anisotropy"] != "4" || len(oa) != 2 {
		t.Fatalf("game A overrides %v (only the chosen settings)", oa)
	}
	if ob := gameNexusSettings(app2.Library.GP.Get(b.Key)); ob["graphics.scale"] != beforeB["graphics.scale"] || len(ob) != len(beforeB) {
		t.Fatalf("game B changed: %v", ob)
	}
	if gs := app2.Emu.GlobalNexusSettings(); gs["graphics.scale"] != "0.5" {
		t.Fatal("per-game optimisation changed the global settings")
	}
	for _, c := range gp.Changes {
		if !EngineCaps()[c.ID].PerGame {
			t.Fatalf("%s is not per-game but was offered for one game", c.ID)
		}
	}
	if err := app2.Emu.RestoreOptimisation(a.Key); err != nil || len(gameNexusSettings(app2.Library.GP.Get(a.Key))) != 0 {
		t.Fatalf("per-game restore %v", err)
	}
}

func TestHardwareDetectionHonest(t *testing.T) {
	app, _ := testNewApp(t, t.TempDir())
	t.Setenv("NEXUS_HARDWARE", "")
	hwMu.Lock()
	hwCache = nil
	hwMu.Unlock()
	h := app.DetectHardware(true)
	if h.Source != "detected" || h.CPU.Threads <= 0 || h.Memory.TotalMB <= 0 || len(h.Storage) == 0 {
		t.Fatalf("basic detection failed: %+v", h)
	}
	// nothing invented for what this system cannot report (a CI machine has no usable GPU; on
	// Linux a GPU is listed only from sysfs, with its PCI ids and driver)
	for _, g := range h.GPUs {
		if runtime.GOOS == "windows" || g.VendorID == 0 || g.Driver == "" || virtualDisplayDrivers[g.Driver] {
			t.Fatalf("GPU must be reported as not detected here, not guessed: %+v", h)
		}
	}
	if len(h.GPUs) > 0 {
		return // a real graphics card on this machine
	}
	if len(h.Notes) == 0 {
		t.Fatalf("an undetected GPU must be explained: %+v", h)
	}
	c := classifyDevice(h)
	if c.GPUTier != 1 || len(c.Reasons) == 0 {
		t.Fatalf("unknown GPU must be treated cautiously and explained: %+v", c)
	}
}
