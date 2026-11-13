package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSystemVault: keys and firmware installed in one copy of NEXUS are available to a new copy
// extracted elsewhere (portable or not); newer files are never replaced by older ones.
func TestSystemVault(t *testing.T) {
	t.Setenv("NEXUS_SYSTEM_FILES", filepath.Join(t.TempDir(), "vault"))
	first, _ := newTestApp(t, "")
	os.RemoveAll(first.Emu.KeysDir())
	os.MkdirAll(first.Emu.KeysDir(), 0o755)
	os.WriteFile(filepath.Join(first.Emu.KeysDir(), "prod.keys"), []byte("header_key = 00"), 0o644)
	os.MkdirAll(first.Emu.FirmwareDir(), 0o755)
	os.WriteFile(filepath.Join(first.Emu.FirmwareDir(), "a.nca"), []byte("fw"), 0o644)
	if to, _, _ := first.Emu.SyncSystemVault(); to != 2 {
		t.Fatalf("saved %d", to)
	}
	second, _ := newTestApp(t, "") // a new version extracted elsewhere
	os.RemoveAll(second.Emu.KeysDir())
	if second.Emu.KeysPresent() {
		t.Fatal("test setup")
	}
	if _, from, _ := second.Emu.SyncSystemVault(); from != 2 || !second.Emu.KeysPresent() || !second.Emu.FirmwarePresent() {
		t.Fatalf("restored %d", from)
	}
	// newer keys in the second copy go to the vault; the first copy's older ones never replace them
	p := filepath.Join(second.Emu.KeysDir(), "prod.keys")
	os.WriteFile(p, []byte("header_key = 11"), 0o644)
	os.Chtimes(p, time.Now(), time.Now().Add(time.Hour))
	second.Emu.SyncSystemVault()
	first.Emu.SyncSystemVault()
	if b, _ := os.ReadFile(filepath.Join(first.Emu.KeysDir(), "prod.keys")); string(b) != "header_key = 11" {
		t.Fatalf("first copy has %q", b)
	}
}

// TestSystemVaultFindsEarlierCopy: an empty vault takes keys from an earlier portable copy next
// to this one.
func TestSystemVaultFindsEarlierCopy(t *testing.T) {
	t.Setenv("NEXUS_SYSTEM_FILES", filepath.Join(t.TempDir(), "vault"))
	app, root := newTestApp(t, "")
	os.RemoveAll(app.Emu.KeysDir())
	old := filepath.Join(filepath.Dir(root), "NEXUS-0.4.0", "System", "Engine", "keys")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "prod.keys"), []byte("k"), 0o644)
	defer os.RemoveAll(filepath.Join(filepath.Dir(root), "NEXUS-0.4.0"))
	if _, _, found := app.Emu.SyncSystemVault(); found == "" || !app.Emu.KeysPresent() {
		t.Fatalf("not found (%q)", found)
	}
}
