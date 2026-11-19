package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestSaveDeleteStaysDeleted: the 0.8.0 bug. Deleting a save on the Deck removed the folder, but
// the next synchronisation saw "not here, in the account" and installed the account's copy again,
// so the game still offered Continue. Now: the deletion goes to the account, the save never comes
// back on this device, the PC asks before deleting its copy, only the chosen user is affected,
// the device save goes too when asked, the engine-layout copy goes, and a new save after the
// deletion synchronises normally again.
func TestSaveDeleteStaysDeleted(t *testing.T) {
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
	ann, _ := pc.Profiles.Create("Ann", Avatar{Kind: "preset", Preset: "star", Color: "#00ff00"})
	const ffr = "0100A5B00BDC6000"
	putSaveFiles(t, pc, tom.ID, ffr+"-FFR", map[string]string{"save00.dat": "tom chapter 4"})
	putSaveFiles(t, pc, tom.ID, ffr+"-FFR (device)", map[string]string{"system.dat": "tom device"})
	putSaveFiles(t, pc, ann.ID, ffr+"-FFR", map[string]string{"save00.dat": "ann chapter 1"})
	signInGitHub(t, pc, f, "Tom's PC")
	mustSync(t, pc)
	signInGitHub(t, deck, f, "Steam Deck")
	mustSync(t, deck)
	dTom, dAnn := onlyProfile(t, deck, "Tom"), onlyProfile(t, deck, "Ann")
	dir := deck.Saves.ProfileDir(dTom)
	acc, dev := gameDir(dir, ffr, false), gameDir(dir, ffr, true)
	if acc == "" || dev == "" {
		t.Fatal("deck did not receive Tom's saves")
	}
	// a stale copy in the engine's own layout inside Tom's folder (Tidy would move it back)
	stale := filepath.Join(dir, "user", "save", "0000000000000000", "00000000000000000000000000000001", ffr)
	os.MkdirAll(stale, 0o755)
	os.WriteFile(filepath.Join(stale, "save00.dat"), []byte("old engine copy"), 0o644)

	if _, err := deck.Saves.DeleteSave(dTom, acc, true); err != nil {
		t.Fatal(err)
	}
	if dirExists(filepath.Join(dir, acc)) || dirExists(filepath.Join(dir, dev)) || dirExists(stale) {
		t.Fatal("save, device save or engine copy still on disk after Delete")
	}
	if b := deck.Saves.Backups(dTom, ffr); len(b) < 2 {
		t.Fatalf("no backups before deleting: %+v", b)
	}
	// the bug: synchronisation (and listing, which tidies) must not bring it back
	for i := 0; i < 2; i++ {
		mustSync(t, deck)
		if _, err := deck.Saves.Player(dTom); err != nil {
			t.Fatal(err)
		}
		if gameDir(dir, ffr, false) != "" || gameDir(dir, ffr, true) != "" {
			t.Fatal("deleted save came back after synchronising")
		}
	}
	if getSaveFiles(t, deck, dAnn, ffr)["save00.dat"] != "ann chapter 1" {
		t.Fatal("Ann's save was touched")
	}
	idx, err := saveRemote{ac: deck.Account}.index()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range idx {
		if m.Deleted {
			n++
			if m.Raw != "" || m.DeviceName != "Steam Deck" {
				t.Fatalf("deletion record: %+v", m)
			}
		}
	}
	if n != 2 {
		t.Fatalf("want 2 deletion records in the account, got %d: %+v", n, idx)
	}

	// the PC: never deleted without asking
	mustSync(t, pc)
	if getSaveFiles(t, pc, tom.ID, ffr)["save00.dat"] != "tom chapter 4" {
		t.Fatal("PC deleted its copy without asking")
	}
	st := pc.SaveSync.Status()
	if len(st.Conflicts) != 2 || !st.Conflicts[0].Remote.Deleted {
		t.Fatalf("PC should ask about the two deleted saves: %+v", st.Conflicts)
	}
	for _, c := range st.Conflicts {
		if err := pc.SaveSync.Resolve(c.Key, "remote"); err != nil {
			t.Fatal(err)
		}
	}
	if gameDir(pc.Saves.ProfileDir(tom.ID), ffr, false) != "" || gameDir(pc.Saves.ProfileDir(tom.ID), ffr, true) != "" {
		t.Fatal("PC kept the save after agreeing to delete it")
	}
	if getSaveFiles(t, pc, ann.ID, ffr)["save00.dat"] != "ann chapter 1" {
		t.Fatal("Ann's save on the PC was touched")
	}

	// a new game on the deck after the deletion synchronises normally
	putSaveFiles(t, deck, dTom, ffr+"-FFR", map[string]string{"save00.dat": "tom new game"})
	mustSync(t, deck)
	mustSync(t, pc)
	if getSaveFiles(t, pc, tom.ID, ffr)["save00.dat"] != "tom new game" {
		t.Fatalf("new save after deletion did not arrive: %v", getSaveFiles(t, pc, tom.ID, ffr))
	}
	if st := pc.SaveSync.Status(); len(st.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", st.Conflicts)
	}
}

// TestSaveDeleteOtherCopyNotSynced: other copies are local only.
func TestSaveDeleteOtherCopyNotSynced(t *testing.T) {
	a, _ := newTestApp(t, "")
	p, _ := a.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	putSaveFiles(t, a, p.ID, "0100A5B00BDC6000-FFR (other copy 1)", map[string]string{"a": "b"})
	if _, err := a.Saves.Delete(p.ID, "0100A5B00BDC6000-FFR (other copy 1)"); err != nil {
		t.Fatal(err)
	}
	a.SaveSync.mu.Lock()
	n := len(a.SaveSync.st.Deleted)
	a.SaveSync.mu.Unlock()
	if n != 0 {
		t.Fatal("deleting an other copy must not send a deletion to the account")
	}
}
