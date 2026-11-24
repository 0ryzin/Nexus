package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 0.9.3: importing a 15,000-shader cache left a game with about 5,000 shaders built at start
// going to 0. The archive's lists of the pipelines its sessions used were added; the engine keeps
// the 16 newest sessions and deletes older lists at the next start, so the other device's newer
// sessions pushed out every list of this device's own. A game that has its own lists keeps them;
// the archive's programs and pipelines are still added, and the counts are real.
func TestImportKeepsThisDevicesUsedLists(t *testing.T) {
	app, _ := newTestApp(t, "")
	g := shaderGame(t, app)
	logPath := app.Emu.ShaderLogPath(g.TitleID)
	writeLog(t, logPath, v2rec(v2KindProgram, 1, "mine"), v2rec(v2KindGraphics, 2, "my pipeline"), v2rec(v2KindUsed, 100, "my session"))
	var other []byte
	other = append(other, v2Header(2)...)
	for i := uint64(0); i < 20; i++ { // twenty newer sessions from the other device
		other = append(other, v2rec(v2KindUsed, 1000+i, "their session")...)
	}
	other = append(other, v2rec(v2KindGraphics, 3, "their pipeline")...)
	other = append(other, v2rec(v2KindProgram, 4, "their program")...)
	res, err := app.Emu.ImportShaderZip(zipOf(t, map[string][]byte{"lichii-cache.v2.bin": other}), g)
	if err != nil {
		t.Fatal(err)
	}
	live := liveOf(t, logPath)
	if live[rk{v2KindUsed, 100}] != "my session" {
		t.Fatal("this device's used list is gone")
	}
	for i := uint64(0); i < 20; i++ {
		if _, ok := live[rk{v2KindUsed, 1000 + i}]; ok {
			t.Fatal("another device's used list was added to a game that has its own")
		}
	}
	if live[rk{v2KindGraphics, 3}] != "their pipeline" || live[rk{v2KindProgram, 4}] != "their program" {
		t.Fatal("the archive's shaders were not added")
	}
	if res.Before != 2 || res.After != 4 || res.Added != 2 || res.Files[0].UsedKept != 20 {
		t.Fatalf("counts: before %d after %d added %d usedKept %d", res.Before, res.After, res.Added, res.Files[0].UsedKept)
	}

	// a game with no shader data yet takes the lists too (they say what to build at start)
	g2 := *g
	g2.TitleID = "0100CCCC00010000"
	if _, err := app.Emu.ImportShaderZip(zipOf(t, map[string][]byte{"lichii-cache.v2.bin": other}), &g2); err != nil {
		t.Fatal(err)
	}
	if _, ok := liveOf(t, app.Emu.ShaderLogPath(g2.TitleID))[rk{v2KindUsed, 1000}]; !ok {
		t.Fatal("a new game did not get the archive's used lists")
	}
}

func TestRestoreShaderBackup(t *testing.T) {
	app, _ := newTestApp(t, "")
	g := shaderGame(t, app)
	logPath := app.Emu.ShaderLogPath(g.TitleID)
	if _, _, err := app.Emu.RestoreShaderBackup(g); err == nil {
		t.Fatal("restored without a backup")
	}
	writeLog(t, logPath, v2rec(v2KindProgram, 1, "before"), v2rec(v2KindGraphics, 2, "before"))
	if _, err := app.Emu.backupShaderLog(g.TitleID); err != nil {
		t.Fatal(err)
	}
	writeLog(t, logPath, v2rec(v2KindProgram, 9, "after a bad import"))
	name, n, err := app.Emu.RestoreShaderBackup(g)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || liveOf(t, logPath)[rk{v2KindProgram, 1}] != "before" {
		t.Fatalf("restored %s: %d records", name, n)
	}
	keep, _ := filepath.Glob(logPath + ".before-restore-*.bak")
	if len(keep) != 1 {
		t.Fatal("the replaced shader data was not backed up")
	}
	if b, _ := os.ReadFile(keep[0]); len(b) == 0 {
		t.Fatal("empty backup")
	}
}
