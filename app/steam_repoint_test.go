package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRepointSteamEntryAndArtwork: after an update, NEXUS's existing Steam entry (made for the
// old AppImage) starts the new one with the same app ID; other entries are untouched; the old
// version's artwork NEXUS put there is replaced by the new artwork, the user's own image is not.
func TestRepointSteamEntryAndArtwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Steam entries are managed on Linux and the Steam Deck only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	oldImg := filepath.Join(home, "Applications", "NEXUS-0.8.1-Linux-amd64.AppImage")
	t.Setenv("APPIMAGE", filepath.Join(home, "Applications", "NEXUS-0.9.2-Linux-amd64.AppImage"))
	app, root := newTestApp(t, "")
	art := filepath.Join(root, "Resources", "Steam")
	os.MkdirAll(art, 0o755)
	for f := range steamArtFiles {
		os.WriteFile(filepath.Join(art, f), []byte("new "+f), 0o644)
	}
	cfg := filepath.Join(home, ".local", "share", "Steam", "userdata", "12345", "config")
	os.MkdirAll(filepath.Join(cfg, "grid"), 0o755)
	const nexusID, esID = 0x9000abcd, 0x8badf00d
	var file bytes.Buffer
	file.WriteString("\x00shortcuts\x00")
	file.Write(shortcutEntry(0, esID, "EmulationStation-DE", `"/home/deck/ES-DE.AppImage"`, `"/home/deck"`, ""))
	file.Write(shortcutEntry(1, nexusID, "NEXUS", `"`+oldImg+`"`, `"`+filepath.Dir(oldImg)+`"`, ""))
	file.WriteString("\x08\x08")
	orig := file.Bytes()
	os.WriteFile(filepath.Join(cfg, "shortcuts.vdf"), orig, 0o644)
	// grid: the 0.8 capsule (NEXUS's), the user's own hero, ES-DE's capsule
	grid := filepath.Join(cfg, "grid")
	old08 := []byte("old capsule")
	steamArtByNexus[sha256Hex(old08)] = true
	defer delete(steamArtByNexus, sha256Hex(old08))
	os.WriteFile(filepath.Join(grid, fmt.Sprintf("%dp.png", uint32(nexusID))), old08, 0o644)
	os.WriteFile(filepath.Join(grid, fmt.Sprintf("%d_hero.png", uint32(nexusID))), []byte("my hero"), 0o644)
	os.WriteFile(filepath.Join(grid, fmt.Sprintf("%dp.png", uint32(esID))), []byte("es-de"), 0o644)

	n, err := app.RepointSteamEntry()
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	now, _ := os.ReadFile(filepath.Join(cfg, "shortcuts.vdf"))
	scs := parseShortcuts(now)
	if len(scs) != 2 || scs[1].appID != nexusID || scs[1].exe != `"`+os.Getenv("APPIMAGE")+`"` || scs[0].exe != `"/home/deck/ES-DE.AppImage"` {
		t.Fatalf("%+v", scs)
	}
	es0 := bytes.Index(orig, []byte("EmulationStation"))
	if !bytes.Equal(now[:es0+200], orig[:es0+200]) {
		t.Fatal("the other entry changed")
	}
	if n, _ := app.RepointSteamEntry(); n != 0 {
		t.Fatal("repointed twice")
	}
	if _, err := app.InstallSteamArtwork(); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string { b, _ := os.ReadFile(filepath.Join(grid, name)); return string(b) }
	if read(fmt.Sprintf("%dp.png", uint32(nexusID))) != "new capsule.png" {
		t.Fatal("NEXUS's old capsule was not replaced")
	}
	if read(fmt.Sprintf("%d_hero.png", uint32(nexusID))) != "my hero" {
		t.Fatal("the user's own hero was replaced")
	}
	if read(fmt.Sprintf("%dp.png", uint32(esID))) != "es-de" {
		t.Fatal("another entry's artwork was touched")
	}
	// a later version's artwork replaces this one (recorded as NEXUS's)
	os.WriteFile(filepath.Join(art, "capsule.png"), []byte("newer capsule"), 0o644)
	app.InstallSteamArtwork()
	if read(fmt.Sprintf("%dp.png", uint32(nexusID))) != "newer capsule" {
		t.Fatal("artwork NEXUS wrote earlier was not updated")
	}
}
