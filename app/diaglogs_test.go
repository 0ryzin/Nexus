package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiagnosticsDeckToPC: the Deck sends its logs; the PC receives them at its next sync, in
// Logs/From other devices, checked and sanitised; the Deck does not receive its own.
func TestDiagnosticsDeckToPC(t *testing.T) {
	t.Setenv("NEXUS_ACCOUNT_SERVER", "")
	f := &fakeGitHub{gists: map[string]map[string]string{}, desc: map[string]string{}, approved: map[string]bool{}}
	ts := httptest.NewServer(f)
	defer ts.Close()
	f.base = ts.URL
	oldW, oldA := githubWeb, githubAPI
	githubWeb, githubAPI = ts.URL, ts.URL
	defer func() { githubWeb, githubAPI = oldW, oldA }()

	pc, _ := newTestApp(t, "")
	deck, _ := newTestApp(t, "")
	signInGitHub(t, pc, f, "PC")
	signInGitHub(t, deck, f, "Steam Deck")
	mustSync(t, pc)
	mustSync(t, deck)
	os.MkdirAll(filepath.Join(deck.Emu.DataDir(), "log"), 0o755)
	os.WriteFile(filepath.Join(deck.Emu.DataDir(), "log", "eden_log.txt"), []byte("[ 1.0] Audio <Info> Auto-selecting the cubeb backend\n[ 2.0] Render.Vulkan <Critical> Device loss occurred!\nprod.keys loaded from /secret\n"), 0o644)
	os.WriteFile(filepath.Join(deck.Paths.Logs, "nexus.log"), []byte("engine: started\naccount: token abc123 refreshed\n"), 0o644)
	e, err := deck.SendDiagnostics("OoT: no sound")
	if err != nil || e.Size == 0 || e.SHA256 == "" {
		t.Fatalf("%+v %v", e, err)
	}
	if got, _ := deck.FetchDiagnostics(); len(got) != 0 {
		t.Fatal("the Deck received its own logs")
	}
	mustSync(t, pc)
	if got, err := pc.FetchDiagnostics(); err != nil {
		t.Fatalf("fetch: %v %v", got, err)
	}
	dirs, _ := filepath.Glob(filepath.Join(pc.Paths.Logs, "From other devices", "Steam Deck *"))
	if len(dirs) != 1 {
		t.Fatalf("received: %v", dirs)
	}
	eng, _ := os.ReadFile(filepath.Join(dirs[0], "engine-eden_log.txt"))
	nx, _ := os.ReadFile(filepath.Join(dirs[0], "nexus.log"))
	about, _ := os.ReadFile(filepath.Join(dirs[0], "about.txt"))
	if !strings.Contains(string(eng), "Device loss occurred") || strings.Contains(string(eng), "prod.keys") {
		t.Fatalf("engine log: %s", eng)
	}
	if !strings.Contains(string(nx), "engine: started") || strings.Contains(string(nx), "abc123") {
		t.Fatalf("NEXUS log: %s", nx)
	}
	if !strings.Contains(string(about), "OoT: no sound") {
		t.Fatalf("about: %s", about)
	}
	// nothing new: nothing received again
	if got, _ := pc.FetchDiagnostics(); len(got) != 0 {
		t.Fatalf("received twice: %v", got)
	}
}
