package main

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestArtworkRealOrder: the order it happened for the user. Both devices were signed in and
// synchronised before the PC got its custom cover (through the interface, SetCustomArt); the
// game also has other per-game data on the Deck. The cover must still arrive on the Deck.
func TestArtworkRealOrder(t *testing.T) {
	t.Setenv("NEXUS_ACCOUNT_SERVER", "")
	f := &fakeGitHub{gists: map[string]map[string]string{}, desc: map[string]string{}, approved: map[string]bool{}}
	ts := httptest.NewServer(f)
	defer ts.Close()
	f.base = ts.URL
	oldW, oldA := githubWeb, githubAPI
	githubWeb, githubAPI = ts.URL, ts.URL
	defer func() { githubWeb, githubAPI = oldW, oldA }()

	pc, pcRoot := newTestApp(t, "")
	deck, deckRoot := newTestApp(t, "")
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	type f2 = struct {
		name string
		data []byte
	}
	nsp := buildPFS0([]f2{{"p.nca", buildNCA(t, k, 0, 0x0400486024DEC000, nil, nil, nil)}})
	for _, r := range []string{pcRoot, deckRoot} {
		os.WriteFile(filepath.Join(r, "Games", "oot.nsp"), nsp, 0o644)
	}
	pc.Library.Scan()
	deck.Library.Scan()
	key := "t-0400486024dec000"
	pc.Library.GP.Update(key, func(p *GameProfile) { p.ArtFit = "cover" })
	deck.Library.GP.Update(key, func(p *GameProfile) { p.Title = "Ocarina" })
	signInGitHub(t, pc, f, "PC")
	signInGitHub(t, deck, f, "Steam Deck")
	mustSync(t, pc)
	mustSync(t, deck)
	mustSync(t, pc)
	cover := append([]byte("\xff\xd8\xff"), bytes.Repeat([]byte("pc-cover"), 4000)...)
	if err := pc.Library.SetCustomArt(key, cover, ".jpg"); err != nil {
		t.Fatal(err)
	}
	mustSync(t, pc)
	mustSync(t, deck)
	got, _ := os.ReadFile(deck.customArtFile(key, "icon"))
	if !bytes.Equal(got, cover) {
		t.Fatalf("the Deck did not receive the cover (%d bytes); deck profile %+v", len(got), deck.Library.GP.Get(key))
	}
}

// TestArtworkNewerKept: a picture chosen on the Deck after the PC's was chosen is not replaced
// by the PC's older one, and reaches the PC instead.
func TestArtworkNewerKept(t *testing.T) {
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
	key := "t-0400486024dec000"
	pcCover := append([]byte("\\xff\\xd8\\xff"), bytes.Repeat([]byte("pc"), 3000)...)
	deckCover := append([]byte("\\xff\\xd8\\xff"), bytes.Repeat([]byte("deck"), 3000)...)
	pc.installSyncedArt(key, "icon", pcCover, ".jpg")
	old := time.Now().Add(-time.Hour)
	os.Chtimes(pc.customArtFile(key, "icon"), old, old)
	deck.installSyncedArt(key, "icon", deckCover, ".jpg")
	signInGitHub(t, pc, f, "PC")
	signInGitHub(t, deck, f, "Steam Deck")
	mustSync(t, pc)
	mustSync(t, deck)
	if got, _ := os.ReadFile(deck.customArtFile(key, "icon")); !bytes.Equal(got, deckCover) {
		t.Fatal("the Deck's newer cover was replaced by the PC's older one")
	}
	mustSync(t, deck)
	mustSync(t, pc)
	if got, _ := os.ReadFile(pc.customArtFile(key, "icon")); !bytes.Equal(got, deckCover) {
		t.Fatalf("the Deck's newer cover did not reach the PC (%d bytes)", len(got))
	}
}
