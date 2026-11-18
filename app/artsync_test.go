package main

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestArtworkPCToDeckAndBack: a cover and a Cards background set on the PC appear on the Deck
// (by Title ID, even before the game is in the Deck's library), are not sent again unchanged, a
// change on the Deck comes back, and the PC keeps its old picture as a backup.
func TestArtworkPCToDeckAndBack(t *testing.T) {
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
	cover := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("pc-cover"), 4000)...)
	bg := append([]byte("\xff\xd8\xff"), bytes.Repeat([]byte("pc-bg"), 9000)...)
	if err := pc.installSyncedArt(key, "icon", cover, ".png"); err != nil {
		t.Fatal(err)
	}
	pc.installSyncedArt(key, "background", bg, ".jpg")
	signInGitHub(t, pc, f, "PC")
	signInGitHub(t, deck, f, "Steam Deck")
	mustSync(t, pc)
	gistsAfterFirst := len(f.gists)
	mustSync(t, deck)
	got, _ := os.ReadFile(deck.customArtFile(key, "icon"))
	gotBG, _ := os.ReadFile(deck.customArtFile(key, "background"))
	if !bytes.Equal(got, cover) || !bytes.Equal(gotBG, bg) {
		t.Fatalf("the Deck did not receive the pictures (%d, %d bytes)", len(got), len(gotBG))
	}
	if gp := deck.Library.GP.Get(key); gp.ArtPreference != "custom" {
		t.Fatalf("art preference %q", gp.ArtPreference)
	}
	// unchanged: nothing sent again
	mustSync(t, pc)
	mustSync(t, deck)
	if len(f.gists) != gistsAfterFirst {
		t.Fatalf("pictures sent again: %d gists, was %d", len(f.gists), gistsAfterFirst)
	}
	// a new cover on the Deck comes back to the PC; the PC's old one is kept
	newCover := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("deck-cover"), 3000)...)
	deck.installSyncedArt(key, "icon", newCover, ".png")
	mustSync(t, deck)
	mustSync(t, pc)
	back, _ := os.ReadFile(pc.customArtFile(key, "icon"))
	if !bytes.Equal(back, newCover) {
		t.Fatalf("the PC did not receive the Deck's cover (%d bytes)", len(back))
	}
	if b, _ := filepath.Glob(pc.Library.GP.File(key, "backup-icon-*")); len(b) != 1 {
		t.Fatalf("the PC's old cover was not kept: %v", b)
	}
	// the background was not touched by the cover change
	if gotBG, _ := os.ReadFile(pc.customArtFile(key, "background")); !bytes.Equal(gotBG, bg) {
		t.Fatal("background changed")
	}
}
