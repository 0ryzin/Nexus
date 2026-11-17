package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResetAllKeepsUserData: "Reset all settings" restores preferences only.
func TestResetAllKeepsUserData(t *testing.T) {
	a, _ := newTestApp(t, "")
	pr, _ := a.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	acct := filepath.Join(a.Paths.System, "Account", "session.bin")
	os.MkdirAll(filepath.Dir(acct), 0o700)
	os.WriteFile(acct, []byte("sign-in"), 0o600)
	a.Settings.Update(func(g *GlobalSettings) {
		g.Library.Dirs = []string{"D:\\Games"}
		g.LastProfileID = pr.ID
		g.Sound.Enabled = false
		g.Emulator.HomeKey = "F9"
		g.Engine["graphics.backend"] = "opengl"
	})
	a.Profiles.Mutate(pr.ID, func(p *Profile) {
		p.Theme.Mode, p.Theme.Wallpaper.File, p.Theme.Wallpaper.Brightness = "light", "mine.jpg", 0.9
		p.Home.Layout, p.Home.Favorites = "grid", []string{"g1"}
		p.Home.Folders = []HomeGroup{{ID: "f1", Name: "RPGs", Games: []string{"g2"}}}
		p.Controls.Mapping["menu"], p.Controls.PreferredPad = 8, "pad-key"
	})
	a.Settings.Update(resetGlobalSettings)
	p, _ := a.Profiles.Mutate(pr.ID, resetProfilePrefs)
	g := a.Settings.Get()
	if !g.Sound.Enabled || g.Emulator.HomeKey != "Shift+H" || g.Engine["graphics.backend"] != "" {
		t.Fatalf("not reset: %+v %v", g.Sound, g.Engine)
	}
	if len(g.Library.Dirs) != 1 || g.LastProfileID != pr.ID {
		t.Fatal("library folders or last user lost")
	}
	if p.Theme.Mode != "dark" || p.Theme.Wallpaper.Brightness != 0.55 || p.Theme.Wallpaper.File != "mine.jpg" {
		t.Fatalf("theme: %+v", p.Theme)
	}
	if p.Home.Layout != "classic" || len(p.Home.Favorites) != 1 || len(p.Home.Folders) != 1 {
		t.Fatalf("home: %+v", p.Home)
	}
	if p.Controls.Mapping["menu"] != 9 || p.Controls.PreferredPad != "pad-key" {
		t.Fatalf("controls: %+v", p.Controls)
	}
	if b, err := os.ReadFile(acct); err != nil || string(b) != "sign-in" {
		t.Fatal("the account sign-in was touched")
	}
}

// TestResetKeybindsOnly: restoring keybinds leaves other settings alone.
func TestResetKeybindsOnly(t *testing.T) {
	a, _ := newTestApp(t, "")
	a.Settings.Update(func(g *GlobalSettings) {
		g.Emulator.HomeKey, g.Emulator.ScreenshotKey, g.Recording.Key, g.Emulator.QuitCombo = "F9", "F8", "F7", false
		g.Sound.Volume, g.Emulator.FadeIn = 0.2, false
	})
	a.Settings.Update(resetKeybinds)
	g := a.Settings.Get()
	if g.Emulator.HomeKey != "Shift+H" || g.Emulator.ScreenshotKey != "Alt+F12" || g.Recording.Key != "F12" || !g.Emulator.QuitCombo {
		t.Fatalf("keys: %+v", g.Emulator)
	}
	if g.Sound.Volume != 0.2 || g.Emulator.FadeIn {
		t.Fatal("other settings changed")
	}
}
