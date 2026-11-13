package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSwitch2LowRAMWarning: a Switch 2 game on a PC with less than 16 GB of RAM starts with a
// warning (the engine forces 12 GB of emulated memory); Switch 1 games get none.
func TestSwitch2LowRAMWarning(t *testing.T) {
	useHardware(t, Hardware{Source: "override", Memory: MemoryInfo{TotalMB: 12000}})
	app, _ := newTestApp(t, "")
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	s := app.Settings.Get()
	if err := app.Emu.checkMemoryLayout(&Game{Title: "Alpha", TitleID: "0100AAAA00010000"}, s); err != nil {
		t.Fatal(err)
	}
	if err := app.Emu.checkMemoryLayout(&Game{Title: "Ocarina", TitleID: "0400486024DEC000"}, s); err != nil {
		t.Fatal(err)
	}
	if e := waitEvent(t, ev, "launch.warning"); e == nil {
		t.Fatal("no warning for a Switch 2 game on 12 GB")
	}
	// a 32 GB PC gets none
	useHardware(t, Hardware{Source: "override", Memory: MemoryInfo{TotalMB: 32 * 1024}})
	if err := app.Emu.checkMemoryLayout(&Game{Title: "Ocarina", TitleID: "0400486024DEC000"}, s); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-ev:
		if strings.Contains(string(m), `"launch.warning"`) {
			t.Fatal("warning on a 32 GB PC")
		}
	case <-time.After(300 * time.Millisecond):
	}
}

// TestSwitch2OptimiserLimits: the optimiser never recommends settings for a Switch 2 game that
// differ from the engine's tested configuration in the ways linked to stuck loading (reactive
// flushing off) or memory pressure (aggressive VRAM, scale above 1x), even on a high-end PC.
func TestSwitch2OptimiserLimits(t *testing.T) {
	for _, hw := range []Hardware{hwMidDesktop, hwHighEnd, hwWeakLaptop} {
		for _, p := range []OptProfile{OptPerformance, OptBalanced, OptQuality} {
			v, why := recommend(hw, classifyDevice(hw), p, "standard")
			switch2Limits(v, why)
			if v["adv.reactive_flushing"] != "true" || v["adv.vram"] != "conservative" || atofSafe(v["graphics.scale"]) > 1 {
				t.Errorf("%s/%s: %v", hw.GPUs[0].Model, p, v)
			}
			if v["adv.filter"] == "area" && hw.Displays[0].Height >= 1080 && v["system.docked"] == "docked" {
				t.Errorf("%s/%s: area filter without supersampling", hw.GPUs[0].Model, p)
			}
			for id := range v {
				if why[id] == "" {
					t.Errorf("%s has no reason", id)
				}
			}
		}
	}
	for id, want := range map[string]bool{"0400486024DEC000": true, "0400486024dec000": true, "0100AAAA00010000": false, "04": false, "": false} {
		if isSwitch2Title(id) != want {
			t.Errorf("isSwitch2Title(%q) != %v", id, want)
		}
	}
}

// TestSwitch2SessionSummary: the launch log line names the loading-related renderer settings
// and the add-on files by name only.
func TestSwitch2SessionSummary(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.ini")
	os.WriteFile(p, []byte("[Renderer]\nuse_reactive_flushing\\default=false\nuse_reactive_flushing=false\nvram_usage_mode\\default=false\nvram_usage_mode=1\n"), 0o644)
	s := switch2SessionSummary(&Game{TitleID: "0400486024DEC000"}, p, []string{filepath.Join("C:", "Games", "OoT Update v1.0.1.nsp")})
	for _, want := range []string{"0400486024DEC000", "reactive_flushing=false", "vram=1", "gpu_accuracy=default", "1 add-on file(s) [OoT Update v1.0.1.nsp]"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary %q lacks %q", s, want)
		}
	}
}
