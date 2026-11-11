package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testNSP(t *testing.T, path string, id uint64, version string) {
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	type f = struct {
		name string
		data []byte
	}
	rom := buildRomFS(map[string][]byte{"control.nacp": makeNACP("Alpha Quest", version)})
	os.WriteFile(path, buildPFS0([]f{{"p.nca", buildNCA(t, k, 0, id, nil, nil, nil)}, {"c.nca", buildNCA(t, k, 2, id, rom, nil, nil)}}), 0o644)
}

// TestGameUpdatesChecked: only a real update for this game is accepted (by its contents, not
// its name); duplicates are refused; enabled updates reach the engine, disabled ones do not.
func TestGameUpdatesChecked(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	key := "t-0100aaaa00010000"
	src := t.TempDir()
	cases := []struct {
		file string
		id   uint64
		want string
	}{
		{"base-named-update.nsp", 0x0100AAAA00010000, "a game, not an update"},
		{"dlc.nsp", 0x0100AAAA00011001, "DLC"},
		{"other-game-update.nsp", 0x0100AAAA00020800, "another game"},
	}
	for _, c := range cases {
		p := filepath.Join(src, c.file)
		testNSP(t, p, c.id, "1.0")
		if _, err := app.Content.AddUpdate(key, p); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: %v", c.file, err)
		}
	}
	os.WriteFile(filepath.Join(src, "junk.nsp"), []byte("not a package"), 0o644)
	if _, err := app.Content.AddUpdate(key, filepath.Join(src, "junk.nsp")); err == nil {
		t.Fatal("junk accepted")
	}
	up := filepath.Join(src, "Alpha Quest v1.2 [0100AAAA00010800].nsp")
	testNSP(t, up, 0x0100AAAA00010800, "1.2.0")
	u, err := app.Content.AddUpdate(key, up)
	if err != nil || u.Version != "1.2.0" || u.FullID != "0100AAAA00010800" {
		t.Fatalf("%+v %v", u, err)
	}
	var list []GameUpdate
	for i := 0; i < 200; i++ {
		if list, _ = app.Content.Updates(key); len(list) == 1 && app.Content.Busy(key) == "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(list) != 1 || !list[0].Enabled || !fileExists(filepath.Join(app.Content.updatesDir("0100AAAA00010000"), list[0].File)) {
		t.Fatalf("list %+v", list)
	}
	if _, err := app.Content.AddUpdate(key, up); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatal("duplicate accepted")
	}
	g, _ := app.Library.Get(key)
	if !containsSuffix(app.Emu.extraContents(&g), list[0].File) {
		t.Fatal("enabled update not given to the engine")
	}
	app.Content.SetUpdateEnabled(key, list[0].File, false)
	if containsSuffix(app.Emu.extraContents(&g), list[0].File) {
		t.Fatal("disabled update given to the engine")
	}
	if err := app.Content.RemoveUpdate(key, list[0].File); err != nil || fileExists(filepath.Join(app.Content.updatesDir("0100AAAA00010000"), list[0].File)) || !fileExists(up) {
		t.Fatal("remove (the original package must stay)")
	}
}

func containsSuffix(l []string, s string) bool {
	for _, v := range l {
		if strings.HasSuffix(v, s) {
			return true
		}
	}
	return false
}

// TestGameModsChecked: mods are found in their usual layouts, installed for this game only,
// never overwritten, and disabled ones are passed to the engine.
func TestGameModsChecked(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	key := "t-0100aaaa00010000"
	src := t.TempDir()
	writeSave(t, filepath.Join(src, "Big Font", "romfs", "font.bin"), "font")
	if added, err := app.Content.AddMod(key, filepath.Join(src, "Big Font"), nil, ""); err != nil || len(added) != 1 {
		t.Fatalf("%v %v", added, err)
	}
	if readFile(t, filepath.Join(app.Emu.DataDir(), "load", "0100AAAA00010000", "Big Font", "romfs", "font.bin")) != "font" {
		t.Fatal("not installed where the engine loads mods")
	}
	if _, err := app.Content.AddMod(key, filepath.Join(src, "Big Font"), nil, ""); err == nil {
		t.Fatal("installed over an existing mod")
	}
	writeSave(t, filepath.Join(src, "wrong", "0100AAAA00020000", "romfs", "x"), "x")
	if _, err := app.Content.AddMod(key, filepath.Join(src, "wrong"), nil, ""); err == nil || !strings.Contains(err.Error(), "another game") {
		t.Fatalf("other game's mod: %v", err)
	}
	writeSave(t, filepath.Join(src, "notamod", "readme.txt"), "x")
	if _, err := app.Content.AddMod(key, filepath.Join(src, "notamod"), nil, ""); err == nil {
		t.Fatal("non-mod accepted")
	}
	// ZIP in the atmosphere layout, and loose exefs patches
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create("atmosphere/contents/0100AAAA00010000/romfs/a.bin")
	w.Write([]byte("zip mod"))
	zw.Close()
	if added, err := app.Content.AddMod(key, "", b.Bytes(), "60 FPS.zip"); err != nil || added[0] != "60 FPS" {
		t.Fatalf("%v %v", added, err)
	}
	writeSave(t, filepath.Join(src, "Patches", "0123456789ABCDEF.pchtxt"), "@nsobid")
	app.Content.AddMod(key, filepath.Join(src, "Patches"), nil, "")
	if !fileExists(filepath.Join(app.Emu.DataDir(), "load", "0100AAAA00010000", "Patches", "exefs", "0123456789ABCDEF.pchtxt")) {
		t.Fatal("loose patches go under exefs")
	}
	mods, _ := app.Content.Mods(key)
	if len(mods) != 3 {
		t.Fatalf("mods %+v", mods)
	}
	app.Content.SetModEnabled(key, "60 FPS", false)
	g, _ := app.Library.Get(key)
	app.Emu.writeSessionConfig(&g, nil)
	ini, _ := LoadIni(app.Emu.SessionPath())
	if v, _ := ini.Get("DisabledAddOns", `1\disabled\1\d`); v != "60 FPS" {
		t.Fatalf("disabled mod not passed: %q", v)
	}
	if v, _ := ini.Get("DisabledAddOns", `1\title_id`); v != "72245241159155712" {
		t.Fatalf("title id %q", v)
	}
	other, _ := app.Library.Get("t-0100aaaa00020000")
	app.Emu.writeSessionConfig(&other, nil)
	ini, _ = LoadIni(app.Emu.SessionPath())
	if _, ok := ini.Get("DisabledAddOns", "size"); ok {
		t.Fatal("another game's session got this game's disabled mods")
	}
}
