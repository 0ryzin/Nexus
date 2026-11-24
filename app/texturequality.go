package main

// Texture quality (engine setting Renderer/astc_recompression; Lichii calls it "ASTC Recompression
// Method": Uncompressed (Best quality) / BC3 (Medium quality) / BC1 (Low quality)).
//
// Before 0.9.2 NEXUS did not set it, so every game ran with the engine's default, BC3 (Medium).
// From 0.9.2 the default is High. Games already played on this device keep what they ran with
// (Medium, as a per-game choice the user can change), so a working game such as Final Fantasy
// Resonance on the Steam Deck is not changed behind the user's back.

import (
	"os"
	"path/filepath"
)

func (a *App) migrateTextureQuality() {
	mark := filepath.Join(a.Paths.System, "texture-quality-0.9.2")
	if fileExists(mark) || a.Library == nil {
		return
	}
	entries, _ := os.ReadDir(a.Library.GP.root)
	kept := 0
	for _, e := range entries {
		if !e.IsDir() || !tidKeyRe.MatchString(e.Name()) {
			continue
		}
		key := "t-" + lowerASCII(e.Name())
		gp := a.Library.GP.Get(key)
		if gp.LastPath == "" { // never started on this device
			continue
		}
		if _, set := gp.Settings["graphics.texture_quality"]; set {
			continue
		}
		_, _ = a.Library.GP.Update(key, func(p *GameProfile) {
			if p.Settings == nil {
				p.Settings = map[string]string{}
			}
			p.Settings["graphics.texture_quality"] = "medium"
		})
		kept++
	}
	_ = os.MkdirAll(filepath.Dir(mark), 0o755)
	_ = os.WriteFile(mark, []byte("games already played keep Medium textures\n"), 0o644)
	if kept > 0 {
		a.Log.Printf("texture quality: %d game(s) played before 0.9.2 keep Medium (what they ran with); new games use High", kept)
	}
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
