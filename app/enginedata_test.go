package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if !st.Ready || !dirExists(filepath.Join(app.Emu.DataDir(), "keys")) {
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
