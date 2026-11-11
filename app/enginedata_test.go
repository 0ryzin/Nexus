package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitImport(t *testing.T, app *App) EngineDataStatus {
	t.Helper()
	for i := 0; i < 200; i++ {
		st := app.Emu.EngineDataStatus()
		if !st.Running && st.Record.State != "running" {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("import did not finish")
	return EngineDataStatus{}
}

func legacyEden(t *testing.T, appdata string) string {
	eden := filepath.Join(appdata, "eden")
	for p, data := range map[string]string{
		"keys/prod.keys": "header_key = 00\n", "keys/title.keys": "01 = 02\n",
		"nand/system/Contents/registered/a.nca": "nca", "nand/user/Contents/registered/u.nca": "update",
		"load/0100AAAA00010000/mod/romfs/x": "mod", "config/input/pad.ini": "[x]",
		"nand/user/save/0000000000000000/s.bin": "save",
		"config/qt-config.ini":                  "eden's own config, never imported",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(eden, p)), 0o755)
		os.WriteFile(filepath.Join(eden, p), []byte(data), 0o644)
	}
	return eden
}

// A fresh NEXUS creates its engine folder and configuration itself; with no other emulator on
// the PC there is nothing to ask.
func TestEngineDataFreshInstall(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	data := t.TempDir()
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), data, false))
	if err := app.Emu.EnsureEngineData(); err != nil {
		t.Fatal(err)
	}
	st := app.Emu.EngineDataStatus()
	if !st.Ready || st.Ask || len(st.Sources) != 0 || !dirExists(filepath.Join(app.Emu.DataDir(), "keys")) {
		t.Fatalf("fresh: %+v", st)
	}
	// restart: same state, configuration kept
	cfg, _ := os.ReadFile(app.Emu.ConfigPath())
	app2, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), data, false))
	app2.Emu.EnsureEngineData()
	cfg2, _ := os.ReadFile(app2.Emu.ConfigPath())
	if string(cfg) != string(cfg2) || !app2.Emu.EngineDataStatus().Ready {
		t.Fatal("configuration must survive a restart unchanged")
	}
}

// Another emulator's data is detected, offered, imported by copying, never overwrites NEXUS's
// own files (which are backed up), and leaves the source untouched.
func TestEngineDataLegacyImport(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	eden := legacyEden(t, appdata)
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), t.TempDir(), false))
	app.Emu.EnsureEngineData()
	// NEXUS already has its own prod.keys: it must be kept
	os.WriteFile(filepath.Join(app.Emu.DataDir(), "keys", "prod.keys"), []byte("nexus's own"), 0o644)

	st := app.Emu.EngineDataStatus()
	if !st.Ask || len(st.Sources) != 1 || st.Sources[0].ID != "eden" || st.Sources[0].Saves != 1 {
		t.Fatalf("detection: %+v", st)
	}
	if err := app.Emu.ImportLegacy("eden"); err != nil {
		t.Fatal(err)
	}
	st = waitImport(t, app)
	if st.Record.State != "done" || st.Record.Decision != "imported" || st.Ask || st.Record.Copied != 5 || st.Record.Skipped != 1 {
		t.Fatalf("import: %+v", st.Record)
	}
	dd := app.Emu.DataDir()
	if b, _ := os.ReadFile(filepath.Join(dd, "keys", "prod.keys")); string(b) != "nexus's own" {
		t.Fatal("an existing NEXUS file was overwritten")
	}
	if b, _ := os.ReadFile(filepath.Join(st.Record.Backup, "keys", "prod.keys")); string(b) != "nexus's own" {
		t.Fatal("NEXUS's existing keys were not backed up")
	}
	for _, p := range []string{"keys/title.keys", "nand/system/Contents/registered/a.nca", "nand/user/Contents/registered/u.nca", "load/0100AAAA00010000/mod/romfs/x", "config/input/pad.ini"} {
		if !fileExists(filepath.Join(dd, p)) {
			t.Errorf("%s not imported", p)
		}
	}
	if fileExists(filepath.Join(dd, "config", "qt-config.ini")) || fileExists(filepath.Join(dd, "nand", "user", "save", "0000000000000000", "s.bin")) {
		t.Fatal("only the listed parts are imported (saves go per user)")
	}
	if n, _ := countFiles(eden); n != 8 {
		t.Fatalf("the source must be untouched (%d files)", n)
	}
	if app.Emu.NandSaveDir() != filepath.Join(eden, "nand", "user", "save") {
		t.Fatal("the earlier saves must still be offered per user")
	}
}

// An import cut off by a crash is recognised at the next start and can be retried; the retry
// skips what was already copied and finishes.
func TestEngineDataInterruptedImport(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	legacyEden(t, appdata)
	data := t.TempDir()
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), data, false))
	app.Emu.EnsureEngineData()
	// simulate a crash: one file copied, the record still says "running", a temp file left over
	dd := app.Emu.DataDir()
	copyFileTmpRename(filepath.Join(appdata, "eden", "keys", "prod.keys"), filepath.Join(dd, "keys", "prod.keys"))
	os.WriteFile(filepath.Join(dd, "keys", "title.keys.nexus-tmp"), []byte("half"), 0o644)
	app.Emu.writeImportRecord(EngineImportRecord{Decision: "imported", State: "running", Source: "eden", Started: 1})

	app2, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), data, false))
	app2.Emu.EnsureEngineData()
	st := app2.Emu.EngineDataStatus()
	if st.Record.State != "interrupted" || !st.Ask {
		t.Fatalf("interrupted import not detected: %+v", st.Record)
	}
	if err := app2.Emu.ImportLegacy("eden"); err != nil {
		t.Fatal(err)
	}
	st = waitImport(t, app2)
	if st.Record.State != "done" || st.Record.Skipped != 1 || st.Record.Copied != 5 {
		t.Fatalf("retry: %+v", st.Record)
	}
	if b, _ := os.ReadFile(filepath.Join(dd, "keys", "title.keys")); string(b) != "01 = 02\n" {
		t.Fatal("the half-written file must be replaced by a complete copy")
	}
}

// Starting fresh imports nothing, deletes nothing, and is remembered across restarts.
func TestEngineDataStartFresh(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	eden := legacyEden(t, appdata)
	data := t.TempDir()
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), data, false))
	app.Emu.EnsureEngineData()
	if err := app.Emu.StartFresh(); err != nil {
		t.Fatal(err)
	}
	app2, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), data, false))
	app2.Emu.EnsureEngineData()
	st := app2.Emu.EngineDataStatus()
	if st.Ask || st.Record.Decision != "fresh" || app2.Emu.KeysPresent() {
		t.Fatalf("fresh: %+v", st.Record)
	}
	if n, _ := countFiles(eden); n != 8 {
		t.Fatal("other emulators' data must be left alone")
	}
	// the user can still import later
	if err := app2.Emu.ImportLegacy("eden"); err != nil {
		t.Fatal(err)
	}
	if st := waitImport(t, app2); st.Record.State != "done" || !app2.Emu.KeysPresent() {
		t.Fatalf("later import: %+v", st.Record)
	}
}

// NEXUS 2.x imported automatically; such an installation is not asked again.
func TestEngineDataEarlierAutomaticImport(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	legacyEden(t, appdata)
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), t.TempDir(), false))
	os.MkdirAll(app.Emu.DataDir(), 0o755)
	os.WriteFile(filepath.Join(app.Emu.DataDir(), ".imported-previous-data"), []byte("x"), 0o644)
	app.Emu.EnsureEngineData()
	if st := app.Emu.EngineDataStatus(); st.Ask || st.Record.Decision != "imported" {
		t.Fatalf("%+v", st.Record)
	}
}
