package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestManageModsDetails: information from nexus-mod.json and from the mod itself, build checks
// once the game's build is known, conflicts between switched-on mods, separate mods per game,
// and persistence (a new App on the same data sees the same state).
func TestManageModsDetails(t *testing.T) {
	app, root := newTestApp(t, "")
	app.Library.Scan()
	key, other := "t-0100aaaa00010000", "t-0100aaaa00020000"
	src := t.TempDir()
	writeSave(t, filepath.Join(src, "HD Textures", "romfs", "tex", "a.bntx"), "a")
	writeSave(t, filepath.Join(src, "HD Textures", "romfs", "tex", "b.bntx"), "b")
	writeSave(t, filepath.Join(src, "HD Textures", "nexus-mod.json"), `{"name":"HD Textures","version":"1.2","author":"someone","category":"visual","description":"Sharper textures."}`)
	writeSave(t, filepath.Join(src, "Retexture", "romfs", "tex", "a.bntx"), "other a")
	writeSave(t, filepath.Join(src, "60fps", "exefs", "E0F3CCFAF50065AE4EA01EB5AEB13B1A45DA3348.ips"), "PATCHEOF")
	writeSave(t, filepath.Join(src, "Old60", "Old60.pchtxt"), "@nsobid-1111222233334444555566667777888899990000\n@enabled\n")
	for _, n := range []string{"HD Textures", "Retexture", "60fps", "Old60"} {
		if _, err := app.Content.AddMod(key, filepath.Join(src, n), nil, ""); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	v, err := app.Content.ModsView(key)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ModDetail{}
	for _, m := range v.Mods {
		by[m.Name] = m
	}
	hd := by["HD Textures"]
	if hd.Meta.Version != "1.2" || hd.Meta.Author != "someone" || hd.Files != 2 || hd.Compat.State != "any" {
		t.Fatalf("HD Textures %+v", hd)
	}
	if !strings.Contains(strings.Join(hd.Conflicts, ","), "Retexture") || !strings.Contains(strings.Join(by["Retexture"].Conflicts, ","), "HD Textures") {
		t.Fatalf("conflict not found: %v / %v", hd.Conflicts, by["Retexture"].Conflicts)
	}
	if by["60fps"].Compat.State != "unknown" || len(by["60fps"].BuildIDs) != 1 {
		t.Fatalf("before the build is known: %+v", by["60fps"])
	}
	// a switched-off mod has no conflicts
	app.Content.SetModEnabled(key, "Retexture", false)
	// the engine log of a session tells NEXUS the build and what applied
	logPath := filepath.Join(t.TempDir(), "eden_log.txt")
	os.WriteFile(logPath, []byte(strings.Join([]string{
		"[  27.0] Loader <Info> core\\file_sys\\patch_manager.cpp:689:PatchExeFS: Patching ExeFS for title_id=0100AAAA00010000",
		"[  28.3] Loader <Info> core\\file_sys\\patch_manager.cpp:1118:HasNSOPatch: Querying NSO patch existence for build_id=E0F3CCFAF50065AE4EA01EB5AEB13B1A45DA3348, name=main",
		"[  28.3] Loader <Info> core\\file_sys\\patch_manager.cpp:954:PatchNSO:     - Applying IPS patch from mod \"60fps\"",
		"[  28.4] Loader <Info> core\\file_sys\\patch_manager.cpp:1253:PatchRomFS:     RomFS: LayeredFS mod HD Textures",
	}, "\n")), 0o644)
	app.Content.LearnFromEngineLog("0100AAAA00020000", logPath) // another game's log: ignored
	if v, _ := app.Content.ModsView(key); v.BuildID != "" {
		t.Fatal("learnt from another game's log")
	}
	app.Content.LearnFromEngineLog("0100AAAA00010000", logPath)
	v, _ = app.Content.ModsView(key)
	by = map[string]ModDetail{}
	for _, m := range v.Mods {
		by[m.Name] = m
	}
	if v.BuildID != "E0F3CCFAF50065AE4EA01EB5AEB13B1A45DA3348" {
		t.Fatalf("build %q", v.BuildID)
	}
	if by["60fps"].Compat.State != "ok" || by["60fps"].Applied != "applied" || by["HD Textures"].Applied != "applied" {
		t.Fatalf("60fps %+v / HD %+v", by["60fps"], by["HD Textures"])
	}
	if by["Old60"].Compat.State != "mismatch" {
		t.Fatalf("Old60 %+v", by["Old60"].Compat)
	}
	if len(by["HD Textures"].Conflicts) != 0 {
		t.Fatalf("conflict with a switched-off mod: %v", by["HD Textures"].Conflicts)
	}
	// the other game has none of these
	if ov, _ := app.Content.ModsView(other); len(ov.Mods) != 0 || ov.BuildID != "" {
		t.Fatalf("other game sees %+v", ov)
	}
	// persistence: a fresh NEXUS on the same data
	app2, err := testNewApp(t, root)
	if err != nil {
		t.Fatal(err)
	}
	app2.Library.Scan()
	v2, _ := app2.Content.ModsView(key)
	n := 0
	for _, m := range v2.Mods {
		if m.Name == "Retexture" && !m.Enabled {
			n++
		}
	}
	if n != 1 || v2.BuildID == "" {
		t.Fatalf("after restart: %+v", v2)
	}
	// removing
	if err := app.Content.RemoveMod(key, "Old60"); err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Content.ModsView(key); len(v.Mods) != 3 {
		t.Fatalf("after removal %d mods", len(v.Mods))
	}
}

// TestBuiltinModsOnlyForTheirGame: the engine's built-in patches appear only for the game they
// are made for, switch through the disabled add-ons, report missing requirements, and cannot be
// removed.
func TestBuiltinModsOnlyForTheirGame(t *testing.T) {
	saved := builtinMods
	defer func() { builtinMods = saved }()
	builtinMods = append(append([]builtinMod{}, saved...),
		builtinMod{Name: "Big Res (built-in)", TitleID: "0100AAAA00010000", BuildID: "AB", Requires: []string{"Pool (built-in)"}},
		builtinMod{Name: "Pool (built-in)", TitleID: "0100AAAA00010000", BuildID: "AB"})
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	key := "t-0100aaaa00010000"
	v, _ := app.Content.ModsView(key)
	if len(v.Builtin) != 2 || !v.Builtin[0].Enabled {
		t.Fatalf("built-ins %+v", v.Builtin)
	}
	if ov, _ := app.Content.ModsView("t-0100aaaa00020000"); len(ov.Builtin) != 0 {
		t.Fatal("built-in offered for another game")
	}
	if err := app.Content.SetModEnabled(key, "Pool (built-in)", false); err != nil {
		t.Fatal(err)
	}
	v, _ = app.Content.ModsView(key)
	if v.Builtin[0].Name != "Big Res (built-in)" || strings.Join(v.Builtin[0].Missing, ",") != "Pool (built-in)" {
		t.Fatalf("missing requirement not shown: %+v", v.Builtin[0])
	}
	g, _ := app.Library.Get(key)
	app.Emu.writeSessionConfig(&g, nil)
	ini, _ := LoadIni(app.Emu.SessionPath())
	if v, _ := ini.Get("DisabledAddOns", `1\disabled\1\d`); v != "Pool (built-in)" {
		t.Fatalf("built-in not passed as disabled: %q", v)
	}
	if err := app.Content.RemoveMod(key, "Pool (built-in)"); err == nil {
		t.Fatal("built-in removed")
	}
	// Ocarina of Time's are listed for it only
	if len(builtinFor("0400486024DEC000")) != 4 || len(builtinFor("0100AAAA00010000")) != 2 {
		t.Fatal("built-in table")
	}
}

// TestModArchiveSafety: archives with paths that leave the mod folder are refused and leave
// nothing behind.
func TestModArchiveSafety(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	key := "t-0100aaaa00010000"
	for _, bad := range []string{"Evil/romfs/../../../../escape.txt", "/abs/romfs/x", "Evil/romfs/..\\..\\x"} {
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		w, _ := zw.Create("Evil/romfs/ok.bin")
		w.Write([]byte("ok"))
		w, _ = zw.Create(bad)
		w.Write([]byte("bad"))
		zw.Close()
		_, err := app.Content.AddMod(key, "", b.Bytes(), "Evil.zip")
		if err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if fileExists(filepath.Join(app.Emu.DataDir(), "escape.txt")) || fileExists(filepath.Join(app.Emu.DataDir(), "load", "escape.txt")) {
		t.Fatal("a file escaped the mod folder")
	}
	if v, _ := app.Content.ModsView(key); len(v.Mods) != 0 {
		t.Fatalf("a half-installed mod is listed: %+v", v.Mods)
	}
}
