package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The System, Core and Memory layout settings reach the session configuration with the engine's
// own keys and values; a game's own value overrides the global one only for that game.
func TestAdvancedEmulationSettings(t *testing.T) {
	app, _ := newTestApp(t, "")
	hw := filepath.Join(t.TempDir(), "hw.json")
	os.WriteFile(hw, []byte(`{"memory":{"totalMb":32768}}`), 0o644)
	t.Setenv("NEXUS_HARDWARE", hw)
	hwCache = nil
	t.Cleanup(func() { hwCache = nil })
	app.Library.Scan()
	app.Settings.Update(func(s *GlobalSettings) {
		s.Engine["system.cpu_clock"] = "boost"
		s.Engine["system.gpu_clock"] = "normal"
		s.Engine["adv.multicore"] = "false"
		s.Engine["core.sync_speed"] = "true"
		s.Engine["adv.speed_limit_on"] = "true"
		s.Engine["adv.speed_limit"] = "200"
		s.Engine["memory.layout"] = "6"
	})
	g, _ := app.Library.Get("t-0100aaaa00010000")
	read := func() string {
		p, err := app.Emu.writeSessionConfig(&g, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		return string(b)
	}
	ini := read()
	for _, want := range []string{"fast_cpu_time=1", "fast_gpu_time=0", "use_multi_core=false", "sync_core_speed=true", "speed_limit=200", "memory_layout_mode=1"} {
		if !strings.Contains(ini, want) {
			t.Errorf("global: %s missing", want)
		}
	}
	// per game: overclock, 8 GB and a custom 135% speed for this game only
	app.Library.GP.Update(g.Key, func(p *GameProfile) {
		p.Settings = map[string]string{"system.cpu_clock": "overclock", "memory.layout": "8", "adv.speed_limit": "135"}
	})
	ini = read()
	for _, want := range []string{"fast_cpu_time=2", "memory_layout_mode=2", "speed_limit=135", "fast_gpu_time=0"} {
		if !strings.Contains(ini, want) {
			t.Errorf("per game: %s missing\n%s", want, ini)
		}
	}
	// another game keeps the global values
	g2, _ := app.Library.Get("t-0100bbbb00010000")
	p2, err := app.Emu.writeSessionConfig(&g2, nil)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(p2)
	if !strings.Contains(string(b2), "memory_layout_mode=1") || !strings.Contains(string(b2), "fast_cpu_time=1") {
		t.Fatalf("other game affected:\n%s", b2)
	}
	if !validNexusValue("adv.speed_limit", "135") || validNexusValue("adv.speed_limit", "5") || validNexusValue("adv.speed_limit", "2000") {
		t.Fatal("custom speed limit validation")
	}
}

// A layout larger than the PC can back is refused before the engine starts.
func TestMemoryLayoutNeedsHostRAM(t *testing.T) {
	app, _ := newTestApp(t, "")
	hw := filepath.Join(t.TempDir(), "hw.json")
	os.WriteFile(hw, []byte(`{"memory":{"totalMb":12000}}`), 0o644)
	t.Setenv("NEXUS_HARDWARE", hw)
	hwCache = nil
	t.Cleanup(func() { hwCache = nil })
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	for layout, ok := range map[string]bool{"4": true, "6": true, "8": true, "10": false, "12": false} {
		app.Settings.Update(func(s *GlobalSettings) { s.Engine["memory.layout"] = layout })
		_, err := app.Emu.writeSessionConfig(&g, nil)
		if (err == nil) != ok {
			t.Errorf("%s GB on a 12 GB PC: err=%v", layout, err)
		}
		if err != nil && !strings.Contains(err.Error(), "smaller memory layout") {
			t.Errorf("unhelpful: %v", err)
		}
	}
}
