package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tom's Steam Deck has more progress than his PC (Ocarina of Time, Final Fantasy Resonance).
// When both changed the same save, nothing is overwritten; the choice shows each device's play
// time and which copy is further ahead; using the Deck's backs up the PC's first.
func TestSaveConflictShowsWhichIsAhead(t *testing.T) {
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
	tom, _ := pc.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	const oot = "0400486024DEC000"
	putSaveFiles(t, pc, tom.ID, oot+"-OOT", map[string]string{"slot_00/save.dat": "start"})
	signInGitHub(t, pc, f, "Tom's PC")
	mustSync(t, pc)
	signInGitHub(t, deck, f, "Steam Deck")
	mustSync(t, deck)
	dTom := onlyProfile(t, deck, "Tom")

	// the Deck plays ten hours, the PC one, both save (the PC earlier)
	gk := "t-" + strings.ToLower(oot)
	now := time.Now().Unix()
	deck.Profiles.AddSession(dTom, gk, "Ocarina of Time", SessionRec{Start: now - 36000, End: now - 60, Seconds: 36000, Emulator: "lichii"})
	pc.Profiles.AddSession(tom.ID, gk, "Ocarina of Time", SessionRec{Start: now - 7200, End: now - 3600, Seconds: 3600, Emulator: "lichii"})
	pcDir := putSaveFiles(t, pc, tom.ID, oot+"-OOT", map[string]string{"slot_00/save.dat": "pc: Kokiri Forest"})
	old := time.Unix(now-3600, 0)
	_ = filepath.Walk(pcDir, func(p string, _ os.FileInfo, _ error) error { return os.Chtimes(p, old, old) })
	putSaveFiles(t, deck, dTom, oot+"-OOT", map[string]string{"slot_00/save.dat": "deck: Water Temple"})
	mustSync(t, deck)
	mustSync(t, pc)

	if got := getSaveFiles(t, pc, tom.ID, oot)["slot_00/save.dat"]; got != "pc: Kokiri Forest" {
		t.Fatalf("PC's save replaced without a choice: %q", got)
	}
	st := pc.SaveSync.Status()
	if len(st.Conflicts) != 1 {
		t.Fatalf("conflicts on the PC: %+v", st.Conflicts)
	}
	c := st.Conflicts[0]
	if c.LocalPlay == nil || c.RemotePlay == nil || c.LocalPlay.Seconds != 3600 || c.RemotePlay.Seconds != 36000 {
		t.Fatalf("play times: local %+v remote %+v", c.LocalPlay, c.RemotePlay)
	}
	if c.Ahead != "remote" {
		t.Fatalf("ahead = %q, want the Deck's (remote)", c.Ahead)
	}
	if err := pc.SaveSync.Resolve(c.Key, "remote"); err != nil {
		t.Fatal(err)
	}
	if got := getSaveFiles(t, pc, tom.ID, oot)["slot_00/save.dat"]; got != "deck: Water Temple" {
		t.Fatalf("after choosing the Deck's: %q", got)
	}
	found := false
	_ = filepath.Walk(pc.Paths.Data, func(p string, _ os.FileInfo, _ error) error {
		if strings.Contains(p, "before-sync") {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("no backup of the PC's save before it was replaced")
	}
}

func TestAheadOfNeedsBothSignals(t *testing.T) {
	c := &SaveSyncConflict{LocalModified: 200, Remote: SaveMeta{Modified: 100}, LocalPlay: &conflictPlay{Seconds: 10}, RemotePlay: &conflictPlay{Seconds: 99}}
	if got := aheadOf(c); got != "" { // more play on the Deck but saved later here: no suggestion
		t.Fatalf("ahead = %q", got)
	}
	c.RemotePlay = nil
	if got := aheadOf(c); got != "" {
		t.Fatalf("unknown play time: ahead = %q", got)
	}
}
