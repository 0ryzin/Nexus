package main

import (
	"archive/zip"
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func putSaveFiles(t *testing.T, a *App, profileID, folder string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(a.Saves.ProfileDir(profileID), folder)
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func getSaveFiles(t *testing.T, a *App, profileID, titleID string) map[string]string {
	t.Helper()
	dir := a.Saves.ProfileDir(profileID)
	name := gameDir(dir, titleID, false)
	if name == "" {
		return nil
	}
	out := map[string]string{}
	root := filepath.Join(dir, name)
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			rel, _ := filepath.Rel(root, p)
			b, _ := os.ReadFile(p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

func onlyProfile(t *testing.T, a *App, name string) string {
	t.Helper()
	for _, p := range a.Profiles.List() {
		if p.Name == name {
			return p.ID
		}
	}
	t.Fatalf("no user %s: %+v", name, a.Profiles.List())
	return ""
}

// TestSaveSyncWindowsToDeck: a save made on a PC arrives on a Steam Deck signed in to the same
// account (with every file and its exact content), changes go both ways, a save changed on both
// is not overwritten until the user chooses, unchanged saves are not sent again, offline changes
// wait, and replaced saves are backed up first.
func TestSaveSyncWindowsToDeck(t *testing.T) {
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
	const oot = "0400486024DEC000"
	slots := map[string]string{"slot_00/save.dat": "tom slot 0 v1", "slot_01/save.dat": "tom slot 1 v1", "option.sav": "options"}
	putSaveFiles(t, pc, tom.ID, oot+"-OOT", slots)
	putSaveFiles(t, pc, ann.ID, oot+"-OOT", map[string]string{"slot_00/save.dat": "ann's own game"})
	putSaveFiles(t, pc, tom.ID, oot+"-OOT (device)", map[string]string{"device.sav": "device data"})
	// engine bookkeeping is not a save
	putSaveFiles(t, pc, tom.ID, oot+"-OOT", map[string]string{".yuzu_save_size": "x"})

	signInGitHub(t, pc, f, "Tom's PC")
	mustSync(t, pc)
	st := pc.SaveSync.Status()
	if st.State != "done" || st.Sent != 3 || len(st.Conflicts) != 0 {
		t.Fatalf("PC after first sync: %+v", st)
	}

	// the Steam Deck: no users, no saves; signing in brings both
	signInGitHub(t, deck, f, "Steam Deck")
	mustSync(t, deck)
	dTom, dAnn := onlyProfile(t, deck, "Tom"), onlyProfile(t, deck, "Ann")
	got := getSaveFiles(t, deck, dTom, oot)
	for k, v := range slots {
		if got[k] != v {
			t.Fatalf("Tom's save on the deck: %v", got)
		}
	}
	if getSaveFiles(t, deck, dAnn, oot)["slot_00/save.dat"] != "ann's own game" {
		t.Fatal("Ann's save mixed up or missing on the deck")
	}
	if name := gameDir(deck.Saves.ProfileDir(dTom), oot, false); name != oot+"-OOT" {
		t.Fatalf("folder name on the deck: %q", name)
	}
	if name := gameDir(deck.Saves.ProfileDir(dTom), oot, true); name == "" {
		t.Fatal("device save missing on the deck")
	}
	if st := deck.SaveSync.Status(); st.Received != 3 || st.Pending != 0 {
		t.Fatalf("deck: %+v", st)
	}

	// nothing changed: nothing is sent again
	saveRevs := func() int {
		n := 0
		for p := range f.raw {
			if strings.HasSuffix(p, "/"+saveBlobFile) {
				n++
			}
		}
		return n
	}
	before := saveRevs()
	mustSync(t, pc)
	mustSync(t, deck)
	if saveRevs() != before {
		t.Fatal("an unchanged save was sent again")
	}

	// played on the deck: the PC receives it, its own copy backed up first
	putSaveFiles(t, deck, dTom, gameDir(deck.Saves.ProfileDir(dTom), oot, false), map[string]string{"slot_00/save.dat": "tom slot 0 v2 (deck)"})
	mustSync(t, deck)
	mustSync(t, pc)
	if got := getSaveFiles(t, pc, tom.ID, oot); got["slot_00/save.dat"] != "tom slot 0 v2 (deck)" || got["slot_01/save.dat"] != "tom slot 1 v1" {
		t.Fatalf("deck's save on the PC: %v", got)
	}
	if b := pc.Saves.Backups(tom.ID, oot); len(b) == 0 || b[0].Reason != "before-sync" {
		t.Fatalf("no backup before replacing the PC's save: %+v", b)
	}

	// changed on both: neither is overwritten; the user chooses
	putSaveFiles(t, pc, tom.ID, gameDir(pc.Saves.ProfileDir(tom.ID), oot, false), map[string]string{"slot_00/save.dat": "pc v3"})
	putSaveFiles(t, deck, dTom, gameDir(deck.Saves.ProfileDir(dTom), oot, false), map[string]string{"slot_00/save.dat": "deck v3"})
	mustSync(t, deck)
	mustSync(t, pc)
	if st := pc.SaveSync.Status(); len(st.Conflicts) != 1 || st.Conflicts[0].Remote.DeviceName != "Steam Deck" {
		t.Fatalf("conflict on the PC: %+v", st)
	}
	if getSaveFiles(t, pc, tom.ID, oot)["slot_00/save.dat"] != "pc v3" || getSaveFiles(t, deck, dTom, oot)["slot_00/save.dat"] != "deck v3" {
		t.Fatal("a conflicting save was overwritten")
	}
	key := pc.SaveSync.Status().Conflicts[0].Key
	if err := pc.SaveSync.Resolve(key, "remote"); err != nil {
		t.Fatal(err)
	}
	if getSaveFiles(t, pc, tom.ID, oot)["slot_00/save.dat"] != "deck v3" || len(pc.SaveSync.Status().Conflicts) != 0 {
		t.Fatal("choosing the deck's save")
	}
	found := false
	for _, b := range pc.Saves.Backups(tom.ID, oot) {
		found = found || b.Reason == "before-sync"
	}
	if !found {
		t.Fatal("the PC's own version was not backed up")
	}

	// offline: the change waits, nothing is lost, and goes out when the connection is back
	putSaveFiles(t, deck, dTom, gameDir(deck.Saves.ProfileDir(dTom), oot, false), map[string]string{"slot_00/save.dat": "deck v4 offline"})
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
	deck.Account.Sync()
	if getSaveFiles(t, deck, dTom, oot)["slot_00/save.dat"] != "deck v4 offline" {
		t.Fatal("offline: the local save changed")
	}
	f.mu.Lock()
	f.down = false
	f.mu.Unlock()
	mustSync(t, deck)
	mustSync(t, pc)
	if getSaveFiles(t, pc, tom.ID, oot)["slot_00/save.dat"] != "deck v4 offline" {
		t.Fatal("the offline change did not arrive after reconnecting")
	}
	if getSaveFiles(t, pc, ann.ID, oot)["slot_00/save.dat"] != "ann's own game" {
		t.Fatal("another user's save of the same game changed")
	}
}

func TestUnzipSafeRefusesEscapes(t *testing.T) {
	for _, name := range []string{"../evil", "/abs", "a/../../evil", "dir\\..\\evil"} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create(name)
		w.Write([]byte("x"))
		zw.Close()
		dir := t.TempDir()
		if err := unzipSafe(buf.Bytes(), dir, 1<<20); err == nil {
			t.Fatalf("%q was extracted", name)
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("big")
	w.Write(bytes.Repeat([]byte("a"), 4096))
	zw.Close()
	if err := unzipSafe(buf.Bytes(), t.TempDir(), 1000); err == nil {
		t.Fatal("size limit not applied")
	}
}
