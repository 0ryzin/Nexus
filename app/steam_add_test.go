//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestAddNexusToSteam: existing shortcuts stay byte for byte, NEXUS is added once with its app
// ID, a backup is kept, artwork goes to that app ID, and a running Steam stops it.
func TestAddNexusToSteam(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPIMAGE", filepath.Join(home, "Applications", "NEXUS-0.8.0-Linux-amd64.AppImage"))
	app, root := newTestApp(t, "")
	art := filepath.Join(root, "Resources", "Steam")
	os.MkdirAll(art, 0o755)
	for _, f := range []string{"capsule.png", "wide.png", "hero.png", "logo.png", "icon.png"} {
		os.WriteFile(filepath.Join(art, f), []byte("png "+f), 0o644)
	}
	cfg := filepath.Join(home, ".local", "share", "Steam", "userdata", "12345", "config")
	os.MkdirAll(cfg, 0o755)
	os.Symlink(filepath.Join(home, ".local", "share", "Steam"), filepath.Join(home, ".steam-link"))
	var file bytes.Buffer
	file.WriteString("\x00shortcuts\x00")
	file.Write(shortcutEntry(0, 0x8badf00d, "EmulationStation-DE", `"/home/deck/ES-DE.AppImage"`, `"/home/deck"`, ""))
	file.Write(shortcutEntry(1, 0x8000beef, "Heroic", `"/usr/bin/heroic"`, `"/usr/bin"`, ""))
	file.WriteString("\x08\x08")
	orig := file.Bytes()
	os.WriteFile(filepath.Join(cfg, "shortcuts.vdf"), orig, 0o644)

	// Steam running: refused, nothing changed
	os.WriteFile(filepath.Join(home, ".local", "share", "Steam", "steam.pid"), []byte(fmt.Sprint(os.Getpid())), 0o644)
	if _, err := app.AddNexusToSteam(); err == nil {
		t.Fatal("added while Steam runs")
	}
	os.Remove(filepath.Join(home, ".local", "share", "Steam", "steam.pid"))

	res, err := app.AddNexusToSteam()
	if err != nil || len(res.Added) != 1 || res.Added[0] != "12345" {
		t.Fatalf("%+v %v", res, err)
	}
	now, _ := os.ReadFile(filepath.Join(cfg, "shortcuts.vdf"))
	if !bytes.Equal(now[:len(orig)-2], orig[:len(orig)-2]) {
		t.Fatal("existing shortcuts changed")
	}
	scs := parseShortcuts(now)
	if len(scs) != 3 || scs[2].name != "NEXUS" || scs[2].appID != res.AppID || scs[2].exe != `"`+os.Getenv("APPIMAGE")+`"` {
		t.Fatalf("%+v", scs)
	}
	if res.AppID&0x80000000 == 0 || shortcutsCount(now) != 3 {
		t.Fatalf("app ID %x, count %d", res.AppID, shortcutsCount(now))
	}
	if b, _ := filepath.Glob(filepath.Join(cfg, "shortcuts.vdf.nexus-backup-*")); len(b) != 1 {
		t.Fatal("no backup")
	}
	if !fileExists(filepath.Join(cfg, "grid", fmt.Sprintf("%dp.png", res.AppID))) || res.Art != 1 {
		t.Fatalf("artwork: %+v", res)
	}
	// again: already there, nothing added
	res2, err := app.AddNexusToSteam()
	again, _ := os.ReadFile(filepath.Join(cfg, "shortcuts.vdf"))
	if err != nil || len(res2.Added) != 0 || len(res2.Already) != 1 || !bytes.Equal(again, now) {
		t.Fatalf("second add: %+v %v", res2, err)
	}
	// a user with no list yet gets a new one
	cfg2 := filepath.Join(home, ".local", "share", "Steam", "userdata", "777", "config")
	os.MkdirAll(cfg2, 0o755)
	if res3, err := app.AddNexusToSteam(); err != nil || len(res3.Added) != 1 || len(parseShortcuts(mustRead(t, filepath.Join(cfg2, "shortcuts.vdf")))) != 1 {
		t.Fatalf("new list: %+v %v", res3, err)
	}
}

func mustRead(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
