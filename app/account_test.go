package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nexus/accountd"
)

// flakyService is the NEXUS account service (accountd) that can be "unplugged".
type flakyService struct {
	h    http.Handler
	down atomic.Bool
}

func (f *flakyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.down.Load() {
		if hj, ok := w.(http.Hijacker); ok {
			if c, _, err := hj.Hijack(); err == nil {
				c.Close()
				return
			}
		}
	}
	f.h.ServeHTTP(w, r)
}

func startAccountService(t *testing.T) (*flakyService, string) {
	t.Helper()
	s, err := accountd.New(accountd.Config{DataDir: t.TempDir(), Iterations: 2000, Logger: log.New(io.Discard, "", 0), RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	f := &flakyService{h: s}
	ts := httptest.NewServer(f)
	t.Cleanup(ts.Close)
	return f, ts.URL
}

func mustSync(t *testing.T, a *App) SyncSummary {
	t.Helper()
	sum, err := a.Account.Sync()
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	return sum
}

func TestAccountUnavailableWithoutService(t *testing.T) {
	t.Setenv("NEXUS_ACCOUNT_SERVER", "")
	app, _ := newTestApp(t, "")
	st := app.Account.Status()
	if st.Available || st.State != "unavailable" || st.SignedIn {
		t.Fatalf("%+v", st)
	}
	if _, err := app.Account.Register("a@b.org", "long password", "A", ""); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("pretended to register: %v", err)
	}
	for _, bad := range []string{"http://accounts.example.org", "ftp://x", "https://user:pw@x.org"} {
		if err := app.Account.SetServer(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if err := app.Account.SetServer("https://accounts.example.org/"); err != nil || app.Account.Server() != "https://accounts.example.org" {
		t.Fatal(err)
	}
}

// TestAccountTwoDevices: create an account on "Tom's PC", link a "Steam Deck" with a one-time
// code, synchronise users, library names, preferences and amiibo, keep offline changes,
// handle a conflict deliberately, and sign out a removed device - with two real NEXUS app
// instances and the real account service.
func TestAccountTwoDevices(t *testing.T) {
	svc, url := startAccountService(t)
	t.Setenv("NEXUS_ACCOUNT_SERVER", url)
	pc, pcRoot := newTestApp(t, "")
	deck, deckRoot := newTestApp(t, "")

	// --- the PC: data, then a new account
	tom, _ := pc.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	pc.Profiles.Update(tom.ID, []byte(`{"theme":{"accent":"#00aa55"}}`))
	link, _ := pc.Amiibo.CreateVirtual("0100000000040002", "Link", "The Legend of Zelda", tom.ID)
	dumpPath := filepath.Join(pcRoot, "mario.bin")
	os.WriteFile(dumpPath, rawAmiiboDump("0000000000000002", [7]byte{4, 1, 1, 1, 2, 2, 2}), 0o644)
	mario, _ := pc.Amiibo.Import(dumpPath, tom.ID)
	pc.Library.GP.Update("t-0100aaaa00010000", func(p *GameProfile) { p.Title = "Alpha Quest (my copy)" })
	pc.Settings.Update(func(s *GlobalSettings) {
		s.Emulator.HomeKey = "F1"
		s.Engine["adv.speed_limit"] = "150"
	})
	code, err := pc.Account.Register("tom@example.org", "correct horse battery", "Tom", "Tom's PC")
	if err != nil || code == "" {
		t.Fatalf("register: %v", err)
	}
	st := pc.Account.Status()
	if !st.SignedIn || st.Email != "tom@example.org" || st.DeviceName != "Tom's PC" || !st.SyncOn {
		t.Fatalf("%+v", st)
	}
	// the sign-in token is not in the readable account file
	acc, _ := os.ReadFile(filepath.Join(pc.Paths.System, "Account", "account.json"))
	if bytes.Contains(acc, []byte(pc.Account.token)) || len(pc.Account.token) < 20 {
		t.Fatal("token stored in account.json")
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(pc.Account.tokenPath()); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("token file permissions: %v %v", fi.Mode(), err)
		}
	}
	if sum := mustSync(t, pc); sum.Uploaded < 4 {
		t.Fatalf("uploaded %+v", sum)
	}
	if st := pc.Account.Status(); st.State != "synced" || st.Pending != 0 || st.LastSync == 0 {
		t.Fatalf("after sync: %+v", st)
	}

	// --- the Steam Deck already has a local "Tom" (never synchronised): link with a code
	deckTom, _ := deck.Profiles.Create("tom", Avatar{Kind: "preset", Preset: "heart", Color: "#0000ff"})
	pcode, _, err := pc.Account.LinkCode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deck.Account.ClaimCode("WRONG-CODE", "Steam Deck"); err == nil {
		t.Fatal("wrong code accepted")
	}
	if _, err := deck.Account.ClaimCode(strings.ToLower(pcode), "Steam Deck"); err != nil {
		t.Fatal(err)
	}
	if s, _ := deck.Account.LinkStatus(); s != "pending" || deck.Account.Status().SignedIn {
		t.Fatal("signed in before approval")
	}
	reqs, _ := pc.Account.LinkRequests()
	if len(reqs) != 1 || reqs[0].Name != "Steam Deck" {
		t.Fatalf("%+v", reqs)
	}
	if err := pc.Account.DecideLink(reqs[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if s, err := deck.Account.LinkStatus(); s != "approved" || err != nil {
		t.Fatalf("%s %v", s, err)
	}
	if st := deck.Account.Status(); !st.SignedIn || st.Email != "tom@example.org" || len(st.Devices) != 2 {
		t.Fatalf("deck: %+v", st)
	}
	if sum := mustSync(t, deck); sum.Downloaded < 4 {
		t.Fatalf("deck downloaded %+v", sum)
	}
	// one Tom, not two: the Deck's own "tom" is the account's Tom, and differs -> a conflict
	if n := len(deck.Profiles.List()); n != 1 {
		t.Fatalf("%d users on the deck", n)
	}
	cs := deck.Account.Status().Conflicts
	if len(cs) != 1 || !strings.Contains(strings.ToLower(cs[0].Title), "tom") {
		t.Fatalf("conflicts %+v", cs)
	}
	if err := deck.Account.ResolveConflict(cs[0].ID, "other"); err != nil {
		t.Fatal(err)
	}
	if p, _ := deck.Profiles.Get(deckTom.ID); p.Name != "Tom" || p.Theme.Accent != "#00aa55" {
		t.Fatalf("deck user after choosing the PC's version: %+v", p)
	}
	// library name, preferences; emulation settings stay (off by default)
	if g := deck.Library.GP.Get("t-0100aaaa00010000"); g.Title != "Alpha Quest (my copy)" {
		t.Fatalf("library %q", g.Title)
	}
	if s := deck.Settings.Get(); s.Emulator.HomeKey != "F1" || s.Engine["adv.speed_limit"] == "150" {
		t.Fatalf("prefs: home %q speed %q", s.Emulator.HomeKey, s.Engine["adv.speed_limit"])
	}
	// amiibo: the virtual one with its data and owner; the dump's details only
	dl, ok := deck.Amiibo.Get(link.ID)
	if !ok || !dl.Available() || dl.Owner != deckTom.ID || dl.Series != "The Legend of Zelda" {
		t.Fatalf("virtual amiibo on the deck: %+v", dl)
	}
	pcImg, _ := os.ReadFile(pc.Amiibo.Path(link))
	deckImg, _ := os.ReadFile(deck.Amiibo.Path(dl))
	if !bytes.Equal(pcImg, deckImg) {
		t.Fatal("virtual amiibo data differs")
	}
	dm, ok := deck.Amiibo.Get(mario.ID)
	if !ok || dm.Available() || dm.Origin != "Tom's PC" {
		t.Fatalf("dump on the deck: %+v", dm)
	}
	if files, _ := filepath.Glob(filepath.Join(deck.Amiibo.dir, mario.ID+"*")); len(files) != 0 {
		t.Fatal("the imported dump was copied to the other device")
	}
	_, f := playing(t, deck)
	f.Scan()
	if err := deck.Amiibo.Place(mario.ID); err == nil || !strings.Contains(err.Error(), "Tom's PC") {
		t.Fatalf("placed a dump that is not here: %v", err)
	}
	deck.Emu.Stop(true)
	waitUntil(t, "the deck's game to end", func() bool { return !deck.Emu.State().Running })
	time.Sleep(100 * time.Millisecond) // the session record is written after the state changes
	// a second synchronisation changes nothing and duplicates nothing
	if sum := mustSync(t, deck); sum.Downloaded+sum.Uploaded != 1 { // the resolved user goes up once
		t.Fatalf("second sync %+v", sum)
	}
	mustSync(t, pc)
	// (the deck's playing() helper created "Kai", which now exists on both)
	names := func(a *App) string {
		var n []string
		for _, p := range a.Profiles.List() {
			n = append(n, p.Name)
		}
		sort.Strings(n)
		return strings.Join(n, ",")
	}
	if names(pc) != "Kai,Tom" || names(deck) != "Kai,Tom" || len(pc.Amiibo.List()) != 2 || len(deck.Amiibo.List()) != 2 {
		t.Logf("pc %s deck %s", names(pc), names(deck))
		t.Fatal("duplicates after synchronising")
	}
	if sum := mustSync(t, pc); sum.Downloaded+sum.Uploaded != 0 {
		t.Fatalf("no-op sync did something: %+v", sum)
	}

	// --- offline: changes stay pending and go up when the service is back
	svc.down.Store(true)
	deck.Amiibo.Update(link.ID, func(x *Amiibo) { x.Name = "Link (Deck)" })
	if _, err := deck.Account.Sync(); err == nil {
		t.Fatal("sync succeeded while offline")
	}
	if st := deck.Account.Status(); st.State != "offline" || st.Pending != 1 {
		t.Fatalf("offline status %+v", st)
	}
	svc.down.Store(false)
	if sum := mustSync(t, deck); sum.Uploaded != 1 {
		t.Fatalf("%+v", sum)
	}
	mustSync(t, pc)
	if a, _ := pc.Amiibo.Get(link.ID); a.Name != "Link (Deck)" {
		t.Fatalf("pc did not receive the offline change: %q", a.Name)
	}

	// --- both devices change the same amiibo: a conflict, not "last upload wins"
	pc.Amiibo.Update(link.ID, func(x *Amiibo) { x.Name = "Link PC" })
	deck.Amiibo.Update(link.ID, func(x *Amiibo) { x.Name = "Link Deck" })
	mustSync(t, pc)
	sum := mustSync(t, deck)
	cs = deck.Account.Status().Conflicts
	if sum.Conflicts != 1 || len(cs) != 1 || deck.Account.Status().State != "conflict" {
		t.Fatalf("no conflict: %+v %+v", sum, cs)
	}
	if a, _ := deck.Amiibo.Get(link.ID); a.Name != "Link Deck" {
		t.Fatal("conflict overwrote the local change")
	}
	if err := deck.Account.ResolveConflict(cs[0].ID, "this"); err != nil {
		t.Fatal(err)
	}
	mustSync(t, deck)
	mustSync(t, pc)
	if a, _ := pc.Amiibo.Get(link.ID); a.Name != "Link Deck" {
		t.Fatalf("chosen version did not reach the PC: %q", a.Name)
	}

	// --- deleting a virtual amiibo reaches the other device
	pc.Amiibo.Remove(link.ID)
	mustSync(t, pc)
	mustSync(t, deck)
	if _, ok := deck.Amiibo.Get(link.ID); ok {
		t.Fatal("deletion not synchronised")
	}

	// --- emulation settings when switched on
	on := true
	deck.Account.SetSync(&on, map[string]bool{"emulation": true})
	pc.Account.SetSync(&on, map[string]bool{"emulation": true})
	mustSync(t, pc)
	mustSync(t, deck)
	if deck.Settings.Get().Engine["adv.speed_limit"] != "150" {
		t.Fatal("emulation settings not synchronised when switched on")
	}

	// --- remove the deck from the PC: the deck is signed out, its data stays
	var deckID string
	for _, d := range pc.Account.Status().Devices {
		if !d.Current {
			deckID = d.ID
		}
	}
	if err := pc.Account.RemoveDevice(deckID); err != nil {
		t.Fatal(err)
	}
	if _, err := deck.Account.Sync(); err == nil {
		t.Fatal("removed device still synchronises")
	}
	if st := deck.Account.Status(); st.SignedIn || st.Ended == "" || st.State != "signedout" {
		t.Fatalf("deck after removal: %+v", st)
	}
	if names(deck) != "Kai,Tom" || fileExists(deck.Account.tokenPath()) {
		t.Fatal("local data lost or token kept after removal")
	}
	_ = deckRoot

	// --- sign out and in again (the session persists across a restart)
	if err := pc.Account.SignOut(); err != nil || pc.Account.Status().SignedIn {
		t.Fatal("sign out", err)
	}
	if err := pc.Account.SignIn("tom@example.org", "wrong password", ""); err == nil {
		t.Fatal("wrong password accepted")
	}
	if err := pc.Account.SignIn("tom@example.org", "correct horse battery", ""); err != nil {
		t.Fatal(err)
	}
	again := NewAccount(pc)
	if st := again.Status(); !st.SignedIn || again.token != pc.Account.token {
		t.Fatal("sign-in lost after a restart")
	}
}
