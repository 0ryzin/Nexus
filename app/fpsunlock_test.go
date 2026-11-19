package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFPSUnlock: only what the engine can do is offered, per game; choices reach the session's
// settings; Game default writes nothing; Ocarina needs its built-in FPS Unlock.
func TestFPSUnlock(t *testing.T) {
	app, root := newTestApp(t, "")
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	type f = struct {
		name string
		data []byte
	}
	for n, id := range map[string]uint64{"oot.nsp": 0x0400486024DEC000, "mkw.nsp": 0x0400C3F00006E000, "other.nsp": 0x0400AAAA00010000} {
		os.WriteFile(filepath.Join(root, "Games", n), buildPFS0([]f{{"p.nca", buildNCA(t, k, 0, id, nil, nil, nil)}}), 0o644)
	}
	app.Library.Scan()
	key := func(id string) string {
		for _, g := range app.Library.Games() {
			if strings.EqualFold(g.TitleID, id) {
				return g.Key
			}
		}
		t.Fatalf("no game %s", id)
		return ""
	}
	oot, mkw, other, s1 := key("0400486024DEC000"), key("0400C3F00006E000"), key("0400AAAA00010000"), key("0100AAAA00010000")

	v, err := app.FPSView(oot)
	if err != nil || v.Status != "supported" || len(v.Choices) != 6 {
		t.Fatalf("oot %+v %v", v, err)
	}
	if _, err := app.SetFPS(oot, "120"); err != nil {
		t.Fatal(err)
	}
	ini := &IniFile{}
	g, _ := app.Library.Get(oot)
	app.Emu.applyFPS(ini, &g)
	if got := iget(ini, "Renderer", "nx2_frame_rate_limit"); got != "120" {
		t.Fatalf("oot limit %q", got)
	}
	// FPS Unlock off: nothing but the default, and the stored choice is not applied
	if err := app.Content.SetModEnabled(oot, fpsUnlockMod, false); err != nil {
		t.Fatal(err)
	}
	v, _ = app.FPSView(oot)
	if v.Status != "unavailable" || v.Fix != "enable-builtin" || v.Current != "" {
		t.Fatalf("oot without unlock %+v", v)
	}
	ini = &IniFile{}
	app.Emu.applyFPS(ini, &g)
	if iget(ini, "Renderer", "nx2_frame_rate_limit") != "" {
		t.Fatal("applied without FPS Unlock")
	}

	if _, err := app.SetFPS(mkw, "dynamic"); err != nil {
		t.Fatal(err)
	}
	ini = &IniFile{}
	gm, _ := app.Library.Get(mkw)
	app.Emu.applyFPS(ini, &gm)
	if iget(ini, "Renderer", "mkw_dynamic_fps") != "true" || iget(ini, "Renderer", "mkw_60fps") != "false" {
		t.Fatal("mkw mode not applied")
	}
	if _, err := app.SetFPS(mkw, ""); err != nil {
		t.Fatal(err)
	}
	ini = &IniFile{}
	app.Emu.applyFPS(ini, &gm)
	if iget(ini, "Renderer", "mkw_dynamic_fps") != "" {
		t.Fatal("game default wrote settings")
	}

	if v, _ := app.FPSView(other); v.Status != "unavailable" || len(v.Choices) != 1 {
		t.Fatalf("ordinary Switch 2 game %+v", v)
	}
	if _, err := app.SetFPS(other, "60"); err == nil {
		t.Fatal("60 accepted for a game without support")
	}
	if v, _ := app.FPSView(s1); v.Status != "unavailable" || !strings.Contains(v.Summary, "does not download") {
		t.Fatalf("switch 1 %+v", v)
	}
	src := t.TempDir()
	writeSave(t, filepath.Join(src, "60fps", "exefs", "AAAA.ips"), "PATCHEOF")
	if _, err := app.Content.AddMod(s1, filepath.Join(src, "60fps"), nil, ""); err != nil {
		t.Fatal(err)
	}
	if v, _ := app.FPSView(s1); v.Status != "experimental" || !strings.Contains(v.Summary, "60fps") {
		t.Fatalf("switch 1 with a mod %+v", v)
	}
}

func iget(f *IniFile, sec, k string) string { v, _ := f.Get(sec, k); return v }
