package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTextureQuality: High by default; a game already played before 0.9.2 keeps Medium (what it
// ran with) as its own choice; the session's settings carry the engine's values.
func TestTextureQuality(t *testing.T) {
	a, _ := newTestApp(t, "")
	if v := a.Settings.Get().Engine["graphics.texture_quality"]; v != "high" {
		t.Fatalf("default %q", v)
	}
	played, fresh := "t-0100aaaa00010000", "t-0100bbbb00010000"
	a.Library.GP.Update(played, func(p *GameProfile) { p.LastPath = "/games/ffr.nsp" })
	os.Remove(filepath.Join(a.Paths.System, "texture-quality-0.9.2"))
	a.migrateTextureQuality()
	if v := a.Library.GP.Get(played).Settings["graphics.texture_quality"]; v != "medium" {
		t.Fatalf("played game: %q", v)
	}
	if _, set := a.Library.GP.Get(fresh).Settings["graphics.texture_quality"]; set {
		t.Fatal("a new game was pinned")
	}
	// once only: a later change by the user stays
	a.Library.GP.Update(played, func(p *GameProfile) { p.Settings["graphics.texture_quality"] = "low" })
	a.migrateTextureQuality()
	if v := a.Library.GP.Get(played).Settings["graphics.texture_quality"]; v != "low" {
		t.Fatalf("migration ran twice: %q", v)
	}
	m := engineBinds["graphics.texture_quality"]
	if m.key != "Renderer/astc_recompression" || m.vals["high"] != "0" || m.vals["medium"] != "2" || m.vals["low"] != "1" {
		t.Fatalf("engine mapping %+v", m)
	}
}
