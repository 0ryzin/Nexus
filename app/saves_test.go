package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testUID = "1122334455667788AABBCCDDEEFF0011"

func writeSave(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("missing %s", p)
	}
	return string(b)
}

func TestSaveFolderNames(t *testing.T) {
	cases := map[string]string{
		"The Legend of Zelda: Breath of the Wild": "BOTW - The Legend of Zelda Breath of the Wild",
		"The Legend of Zelda: Ocarina of Time":    "OOT - The Legend of Zelda Ocarina of Time",
		"Some Indie Game":                         "Some Indie Game",
		"What? <Game>|*":                          "What Game",
		"CON":                                     "_CON",
		"Trailing dots...":                        "Trailing dots",
	}
	for in, want := range cases {
		if got := GameFolderName(in, "0100000000000000"); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	if got := GameFolderName("", "0100abcd00000000"); got != "Unknown game 0100ABCD00000000" {
		t.Errorf("unknown game folder %q", got)
	}
}

func TestSaveOrganisationLifecycle(t *testing.T) {
	app, root := newTestApp(t, "")
	app.Library.Scan()
	p, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	saves := app.Paths.Saves
	// NEXUS 2.0 layout: Saves/<id>/user/save/... with title-ID folders
	newDir := app.Profiles.SaveDir(p.ID)
	os.RemoveAll(newDir)
	old := filepath.Join(saves, p.ID)
	eng := filepath.Join(old, "user", "save", "0000000000000000")
	writeSave(t, filepath.Join(eng, testUID, "0100AAAA00010000", "slot1.sav"), "alpha progress")
	os.MkdirAll(filepath.Join(eng, testUID, "0100AAAA00020000"), 0o755) // empty: never played
	writeSave(t, filepath.Join(eng, "00000000000000000000000000000000", "0100AAAA00010000", "device.dat"), "shared")
	writeSave(t, filepath.Join(eng, testUID, "0100FFFF00010000", "x.sav"), "not in library")
	writeSave(t, filepath.Join(old, "user", "save", "cache", "0100AAAA00010000", "c.bin"), "cache")

	app.Saves.OrganizeAll()
	dir := app.Profiles.SaveDir(p.ID)
	if filepath.Base(dir) != p.ID+" - Tom" || dirExists(old) {
		t.Fatalf("profile folder %q (old still there: %v)", dir, dirExists(old))
	}
	if readFile(t, filepath.Join(dir, "Alpha Quest", "slot1.sav")) != "alpha progress" {
		t.Fatal("game save not in its readable folder")
	}
	if readFile(t, filepath.Join(dir, "Alpha Quest (shared device save)", "device.dat")) != "shared" {
		t.Fatal("device save not organised")
	}
	if readFile(t, filepath.Join(dir, "Unknown game 0100FFFF00010000", "x.sav")) != "not in library" {
		t.Fatal("save of a game not in the library must still be kept and named")
	}
	if !fileExists(filepath.Join(dir, "user", "save", "cache", "0100AAAA00010000", "c.bin")) {
		t.Fatal("engine cache must stay where the engine keeps it")
	}
	if dirExists(filepath.Join(dir, "Bravo Island Deluxe")) || dirExists(filepath.Join(dir, "b")) {
		t.Fatal("an empty engine folder must not become a game folder")
	}
	list := app.Saves.List(p.ID)
	if len(list) != 3 || list[0].TitleID == "" {
		t.Fatalf("list %+v", list)
	}

	// a game session: folders go to the engine paths and come back afterwards
	t.Setenv("NEXUS_FAKE_PLAY", "600ms")
	g, _ := app.Library.Get("t-0100aaaa00010000")
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	enginePath := filepath.Join(dir, "user", "save", "0000000000000000", testUID, "0100AAAA00010000", "slot1.sav")
	if !fileExists(enginePath) || dirExists(filepath.Join(dir, "Alpha Quest")) {
		t.Fatal("during the game the engine must find the save at its title-ID path")
	}
	ini, _ := LoadIni(app.Emu.SessionPath())
	if v, _ := ini.Get("Data%20Storage", "save_directory"); !strings.Contains(v, p.ID+" - Tom") {
		t.Fatalf("engine save directory %q", v)
	}
	// the game writes a new save
	writeSave(t, filepath.Join(dir, "user", "save", "0000000000000000", testUID, "0100AAAA00010000", "slot2.sav"), "new")
	waitIdle(t, app)
	time.Sleep(50 * time.Millisecond)
	if readFile(t, filepath.Join(dir, "Alpha Quest", "slot2.sav")) != "new" || fileExists(enginePath) {
		t.Fatal("after the game the save must be back in its readable folder")
	}

	// renaming the user renames the folder; the profile ID stays at the front
	app.Profiles.Update(p.ID, []byte(`{"name":"Tom: Main?"}`))
	app.Saves.SyncProfileName(p.ID)
	dir2 := app.Profiles.SaveDir(p.ID)
	if filepath.Base(dir2) != p.ID+" - Tom Main" || readFile(t, filepath.Join(dir2, "Alpha Quest", "slot1.sav")) != "alpha progress" {
		t.Fatalf("rename: %s", dir2)
	}

	// crash during a session: next start brings the folders back
	app.Saves.Assemble(p.ID)
	if dirExists(filepath.Join(dir2, "Alpha Quest")) {
		t.Fatal("assemble did not move the folder")
	}
	app2, _ := NewApp(root)
	app2.Library.Scan()
	app2.Saves.OrganizeAll()
	if readFile(t, filepath.Join(app2.Profiles.SaveDir(p.ID), "Alpha Quest", "slot1.sav")) != "alpha progress" {
		t.Fatal("crash recovery")
	}
}

func TestSaveDropInExportImportNoOverwrite(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	p, _ := app.Profiles.Create("Ana", Avatar{Kind: "glyph"})
	app.Saves.EnsureGameFolders(p.ID)
	dir := app.Profiles.SaveDir(p.ID)
	if !dirExists(filepath.Join(dir, "Alpha Quest")) || !dirExists(filepath.Join(dir, "Charlie Racing")) {
		entries, _ := os.ReadDir(dir)
		t.Fatalf("library game folders not created: %v", entries)
	}
	// the user drops a save into the empty folder; the engine already has a console user
	writeSave(t, filepath.Join(dir, "Alpha Quest", "dropped.sav"), "from my other PC")
	pd := make([]byte, 0x650)
	lo, hi := uint64(0x8877665544332211), uint64(0x1100FFEEDDCCBBAA)
	binary.LittleEndian.PutUint64(pd[0x10:], lo)
	binary.LittleEndian.PutUint64(pd[0x18:], hi)
	writeSave(t, filepath.Join(app.Emu.DataDir(), "nand", "system", "save", "8000000000000010", "su", "avators", "profiles.dat"), string(pd))
	if n, _ := app.Saves.Assemble(p.ID); n != 1 {
		t.Fatalf("placed %d", n)
	}
	// the engine names a console user's folder %016X%016X of the UUID's high and low halves
	if readFile(t, filepath.Join(dir, "user", "save", "0000000000000000", "1100FFEEDDCCBBAA8877665544332211", "0100AAAA00010000", "dropped.sav")) != "from my other PC" {
		t.Fatal("dropped save not placed at the console user's path")
	}
	app.Saves.Organize(p.ID)

	// export, then import into another user: added beside existing folders, nothing replaced
	var buf bytes.Buffer
	if err := app.Saves.Export(p.ID, &buf); err != nil {
		t.Fatal(err)
	}
	q, _ := app.Profiles.Create("Ben", Avatar{Kind: "glyph"})
	qdir := app.Profiles.SaveDir(q.ID)
	writeSave(t, filepath.Join(qdir, "Alpha Quest", "mine.sav"), "ben's own")
	if n, err := app.Saves.ImportZip(q.ID, buf.Bytes()); err != nil || n == 0 {
		t.Fatalf("import %d %v", n, err)
	}
	if readFile(t, filepath.Join(qdir, "Alpha Quest", "mine.sav")) != "ben's own" {
		t.Fatal("import overwrote an existing save")
	}
	if readFile(t, filepath.Join(qdir, "Alpha Quest (2)", "dropped.sav")) != "from my other PC" {
		t.Fatal("imported save missing")
	}
	if fileExists(filepath.Join(qdir, "Alpha Quest (2)", ".nexus-saves.json")) {
		t.Fatal("mapping file must not land inside a game folder")
	}
	// conflict: readable folder has data AND the engine has a save for the same game
	writeSave(t, filepath.Join(qdir, "user", "save", "0000000000000000", testUID, "0100AAAA00010000", "engine.sav"), "engine copy")
	app.Saves.Organize(q.ID)
	found := false
	for _, f := range app.Saves.List(q.ID) {
		if fileExists(filepath.Join(f.Path, "engine.sav")) {
			found = true
		}
	}
	if !found || readFile(t, filepath.Join(qdir, "Alpha Quest", "mine.sav")) != "ben's own" {
		t.Fatal("both copies must be kept")
	}
}
