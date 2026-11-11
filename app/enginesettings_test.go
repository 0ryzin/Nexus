package main

import (
	"strings"
	"testing"
)

func sp(s string) *string { return &s }

// TestNexusSettingsReachTheEngine: global and per-game NEXUS values become the engine's own
// configuration keys in the session file it reads at launch; other games keep the global values.
func TestNexusSettingsReachTheEngine(t *testing.T) {
	app, root := newTestApp(t, "")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	other, _ := app.Library.Get("t-0100aaaa00020000")
	if err := app.Emu.SetGlobal(map[string]*string{"graphics.backend": sp("opengl"), "graphics.aa": sp("fxaa"), "audio.volume": sp("180")}); err != nil {
		t.Fatal(err)
	}
	if err := app.Emu.SetGameSettings(g, map[string]*string{"graphics.scale": sp("0.5"), "system.language": sp("fr"), "audio.volume": sp("40")}, nil); err != nil {
		t.Fatal(err)
	}
	if err := app.Emu.SetGameSettings(g, map[string]*string{"audio.mute": sp("true")}, nil); err == nil {
		t.Fatal("mute is global-only in the engine; per-game must be refused")
	}
	if err := app.Emu.SetGameSettings(g, map[string]*string{"graphics.scale": sp("7")}, nil); err == nil {
		t.Fatal("invalid value accepted")
	}
	if err := app.Emu.SetGlobal(map[string]*string{"audio.volume": sp("201")}); err == nil {
		t.Fatal("volume above 200% accepted")
	}
	chk := func(sec, key, want string) {
		t.Helper()
		ini, _ := LoadIni(app.Emu.SessionPath())
		if v, _ := ini.GetSetting(sec, key); v != want {
			t.Fatalf("%s/%s = %q, want %q", sec, key, v, want)
		}
	}
	if _, err := app.Emu.writeSessionConfig(&g, nil); err != nil {
		t.Fatal(err)
	}
	chk("Renderer", "resolution_setup", "1")
	chk("System", "language_index", "2")
	chk("Audio", "volume", "40")
	chk("Renderer", "backend", "0")
	chk("Renderer", "anti_aliasing", "1")
	if _, err := app.Emu.writeSessionConfig(&other, nil); err != nil {
		t.Fatal(err)
	}
	chk("Renderer", "resolution_setup", "") // engine default, not game A's 0.5x
	chk("Audio", "volume", "180")
	chk("Renderer", "backend", "0")
	ev := app.Emu.EngineValues()
	if ev["graphics.backend"] != "opengl" || ev["graphics.aa"] != "fxaa" {
		t.Fatalf("reverse %v", ev)
	}
	app.Emu.SetGameSettings(g, map[string]*string{"graphics.vsync": sp("off")}, nil)
	app2, _ := NewApp(root)
	if v := app2.Emu.GameSettingsView(g).Overrides["graphics.vsync"]; v != "off" {
		t.Fatal("per-game settings lost after restart")
	}
	if app2.Settings.Get().Engine["graphics.backend"] != "opengl" {
		t.Fatal("global settings lost after restart")
	}
}

func TestSettingsMigrationAndCaps(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	app.Library.GP.Update(g.Key, func(p *GameProfile) {
		p.Emu = map[string]string{"Renderer/resolution_setup": "6", "Renderer/use_vsync": "0"}
		p.Ryu = map[string]string{"docked_mode": "false", "graphics_backend": "OpenGl"}
	})
	v := app.Emu.GameSettingsView(g)
	if v.Overrides["graphics.scale"] != "2" || v.Overrides["graphics.vsync"] != "off" || v.Overrides["system.docked"] != "handheld" || v.Overrides["graphics.backend"] != "opengl" {
		t.Fatalf("migration %v", v.Overrides)
	}
	if len(v.Engines) != 1 || v.Engines[0].ID != "nexus" || v.Effective != "nexus" {
		t.Fatalf("only the NEXUS engine may be offered: %+v", v.Engines)
	}
	c := EngineCaps()
	for _, s := range NexusSettings {
		if s.Kind == "enum" && c[s.ID].Available && len(c[s.ID].Choices) != len(s.Choices) && s.ID != "controls.profile" {
			t.Errorf("%s offers choices the engine does not apply: %v vs %d", s.ID, c[s.ID].Choices, len(s.Choices))
		}
		if !c[s.ID].Available && s.Kind != "info" {
			t.Errorf("%s is shown but not applied by the engine", s.ID)
		}
	}
	if c["audio.mute"].PerGame || !c["audio.mute"].Available {
		t.Fatal("mute is global-only")
	}
	if c["controls.mapping"].Available || c["controls.mapping"].Note == "" {
		t.Fatal("mapping must be shown as unavailable, not faked")
	}
	if s := nsetting("audio.volume"); s.Max != 200 || !strings.Contains(s.Help, "200%") {
		t.Fatal("volume range must be 0-200%")
	}
	if resolutionText("2", "docked") != "3840 × 2160 (2×, docked)" || resolutionText("0.5", "handheld") != "640 × 360 (0.5×, handheld)" {
		t.Fatal(resolutionText("2", "docked"), resolutionText("0.5", "handheld"))
	}
	app.Settings.Update(func(s *GlobalSettings) { s.Emulator.Overrides = map[string]string{"Renderer/backend": "0"} })
	s2, _ := LoadSettings(app.Paths)
	if s2.Get().Engine["graphics.backend"] != "opengl" || len(s2.Get().Emulator.Overrides) != 0 {
		t.Fatal("global migration")
	}
}
