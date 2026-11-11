package main

import (
	"bytes"
	"crypto/hmac"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSpake2(t *testing.T) {
	a, _ := newSpake(true, "ABCD-2345", "acc", "devA", "devB")
	b, _ := newSpake(false, "abcd 2345", "acc", "devA", "devB")
	ka, maA, mbA, err := a.finish(b.msg)
	kb, maB, mbB, err2 := b.finish(a.msg)
	if err != nil || err2 != nil || !bytes.Equal(ka, kb) || !hmac.Equal(maA, maB) || !hmac.Equal(mbA, mbB) {
		t.Fatal("same code must give the same key and confirmations")
	}
	c, _ := newSpake(true, "ABCD-2346", "acc", "devA", "devB")
	d, _ := newSpake(false, "ABCD-2345", "acc", "devA", "devB")
	kc, mac1, _, _ := c.finish(d.msg)
	kd, mac2, _, _ := d.finish(c.msg)
	if bytes.Equal(kc, kd) || hmac.Equal(mac1, mac2) {
		t.Fatal("a wrong code must not agree")
	}
	// the code is bound to the user: the right code for another user fails
	e, _ := newSpake(true, "ABCD-2345", "other", "devA", "devB")
	f, _ := newSpake(false, "ABCD-2345", "acc", "devA", "devB")
	_, m3, _, _ := e.finish(f.msg)
	_, m4, _, _ := f.finish(e.msg)
	if hmac.Equal(m3, m4) {
		t.Fatal("codes must be bound to the user")
	}
	if _, _, _, err := a.finish([]byte{4, 1, 2, 3}); err == nil {
		t.Fatal("an invalid point must be refused")
	}
	box, _ := sealBox(ka, []byte("hello"), []byte("ad"))
	if p, err := openBox(kb, box, []byte("ad")); err != nil || string(p) != "hello" {
		t.Fatal("seal/open")
	}
	if _, err := openBox(kb, box, []byte("other")); err == nil {
		t.Fatal("wrong associated data must fail")
	}
}

func TestPairCodes(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, _ := newPairCode()
		if len(c) != 9 || c[4] != '-' || strings.ContainsAny(c, "01ILO") || seen[c] {
			t.Fatalf("bad or repeated code %q", c)
		}
		seen[c] = true
	}
	if UserID("8e0a6fc92d8e") != "NX-8E0A-6FC9-2D8E" || profileIDFromUserID("nx-8e0a-6fc9-2d8e") != "8e0a6fc92d8e" {
		t.Fatal("user id format")
	}
}

type linkPeer struct {
	app  *App
	link *DeviceLink
	addr string
}

func newLinkPeer(t *testing.T, name string) *linkPeer {
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), t.TempDir(), false))
	l, err := NewDeviceLink(app)
	if err != nil {
		t.Fatal(err)
	}
	l.id.DeviceName = name
	if err := l.Listen(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	app.Link = l
	return &linkPeer{app, l, net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Port))}
}

// approveNext approves (or declines) the next request that reaches the trusted device.
func approveNext(p *linkPeer, approve bool) {
	go func() {
		for i := 0; i < 200; i++ {
			for _, r := range p.link.PendingRequests() {
				_ = p.link.Decide(r.ID, approve)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

func TestDeviceLinkingEndToEnd(t *testing.T) {
	pc := newLinkPeer(t, "My Windows PC")
	deck := newLinkPeer(t, "Steam Deck")
	tom, _ := pc.app.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	pc.app.Profiles.Update(tom.ID, []byte(`{"theme":{"accent":"#30d158"}}`))
	uid := UserID(tom.ID)

	// no code yet: refused, and the User ID alone is useless
	if _, err := deck.link.PairWith(pc.addr, uid, "AAAA-BBBB"); err == nil {
		t.Fatal("pairing without a code shown on the PC must fail")
	}
	code, _, _ := pc.link.StartPairing(tom.ID)

	// wrong codes: refused before anybody is asked; the fifth locks the code
	for i := 0; i < pairMaxAttempts; i++ {
		if _, err := deck.link.PairWith(pc.addr, uid, "ZZZZ-ZZZZ"); err == nil {
			t.Fatal("a wrong code must fail")
		}
	}
	if len(pc.link.PendingRequests()) != 0 {
		t.Fatal("a wrong code must never reach the user for approval")
	}
	if _, err := deck.link.PairWith(pc.addr, uid, code); err == nil {
		t.Fatal("after too many wrong codes the code must be cancelled")
	}

		// declined
	code, _, _ = pc.link.StartPairing(tom.ID)
	approveNext(pc, false)
	if _, err := deck.link.PairWith(pc.addr, uid, code); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("declined: %v", err)
	}

	// approved
	code, _, _ = pc.link.StartPairing(tom.ID)
	approveNext(pc, true)
	d, err := deck.link.PairWith(pc.addr, uid, code)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := deck.app.Profiles.Get(tom.ID); !ok || got.Name != "Tom" || got.Theme.Accent != "#30d158" {
		t.Fatalf("the user must exist on the new device with the same id: %+v", got)
	}
	if len(pc.link.Devices()) != 1 || pc.link.Devices()[0].Name != "Steam Deck" {
		t.Fatal("the PC must list the deck")
	}
	// the code is single-use
	if _, err := deck.link.PairWith(pc.addr, uid, code); err == nil {
		t.Fatal("a used code must not work again")
	}

	// ---- synchronisation: a save made on the PC reaches the deck
	pcSave := filepath.Join(pc.app.Profiles.SaveDir(tom.ID), "OOT - Ocarina of Time", "0000000000000001", "save.bin")
	os.MkdirAll(filepath.Dir(pcSave), 0o755)
	os.WriteFile(pcSave, []byte("slot 1: Kokiri Forest"), 0o644)
	pc.app.Profiles.AddSession(tom.ID, "t-oot", "Ocarina of Time", SessionRec{Start: 1000, End: 1600, Seconds: 600, Emulator: engineID})
	res, err := deck.link.SyncWith(d.DeviceID, pc.addr)
	if err != nil || res.Pulled < 1 || res.Conflicts != 0 {
		t.Fatalf("first sync: %+v %v", res, err)
	}
	deckSave := filepath.Join(deck.app.Profiles.SaveDir(tom.ID), "OOT - Ocarina of Time", "0000000000000001", "save.bin")
	if b, _ := os.ReadFile(deckSave); string(b) != "slot 1: Kokiri Forest" {
		t.Fatal("the save did not arrive")
	}
	if g := deck.app.Profiles.Stats(tom.ID).Games["t-oot"]; g == nil || g.Seconds != 600 {
		t.Fatal("play time from the PC must show on the deck")
	}

	// played on the deck: the newer save goes back to the PC (backed up there first)
	os.WriteFile(deckSave, []byte("slot 1: Death Mountain"), 0o644)
	deck.app.Profiles.AddSession(tom.ID, "t-oot", "Ocarina of Time", SessionRec{Start: 5000, End: 5300, Seconds: 300, Emulator: engineID})
	res, err = deck.link.SyncWith(d.DeviceID, pc.addr)
	if err != nil || res.Pushed != 1 || res.Conflicts != 0 {
		t.Fatalf("second sync: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(pcSave); string(b) != "slot 1: Death Mountain" {
		t.Fatal("the newer save must reach the PC")
	}
	if !backupHas(t, pc.app, "slot 1: Kokiri Forest") {
		t.Fatal("the replaced save must be backed up")
	}
	if g := pc.app.Profiles.Stats(tom.ID).Games["t-oot"]; g == nil || g.Seconds != 900 {
		t.Fatalf("play time adds up across devices: %+v", g)
	}

	// both changed: nothing overwritten, both kept, the user chooses
	os.WriteFile(pcSave, []byte("PC: Water Temple"), 0o644)
	os.WriteFile(deckSave, []byte("Deck: Fire Temple"), 0o644)
	res, err = deck.link.SyncWith(d.DeviceID, pc.addr)
	if err != nil || res.Conflicts != 1 {
		t.Fatalf("conflict: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(pcSave); string(b) != "PC: Water Temple" {
		t.Fatal("a conflict must not overwrite the PC's save")
	}
	if b, _ := os.ReadFile(deckSave); string(b) != "Deck: Fire Temple" {
		t.Fatal("a conflict must not overwrite the deck's save")
	}
	cs := deck.link.Conflicts()
	if len(cs) != 1 {
		t.Fatalf("conflicts: %+v", cs)
	}
	if b, _ := os.ReadFile(cs[0].Theirs); string(b) != "PC: Water Temple" {
		t.Fatal("the other device's version must be kept")
	}
	if err := deck.link.ResolveConflict(cs[0].ID, "theirs"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(deckSave); string(b) != "PC: Water Temple" || !backupHas(t, deck.app, "Deck: Fire Temple") {
		t.Fatal("choosing the other version replaces ours, after a backup")
	}
	if res, err := deck.link.SyncWith(d.DeviceID, pc.addr); err != nil || res.Conflicts != 0 || res.Pulled+res.Pushed != 0 {
		t.Fatalf("after resolving, the devices agree: %+v %v", res, err)
	}

	// profile changes travel too
	deck.app.Profiles.Update(tom.ID, []byte(`{"name":"Tommy"}`))
	if _, err := deck.link.SyncWith(d.DeviceID, pc.addr); err != nil {
		t.Fatal(err)
	}
	if p, _ := pc.app.Profiles.Get(tom.ID); p.Name != "Tommy" {
		t.Fatalf("profile name: %q", p.Name)
	}

	// revoked: the PC no longer accepts the deck
	if err := pc.link.RemoveDevice(d.DeviceID); err == nil {
		t.Fatal("the PC knows the deck under the deck's id")
	}
	if err := pc.link.RemoveDevice(pc.link.Devices()[0].DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := deck.link.SyncWith(d.DeviceID, pc.addr); err == nil {
		t.Fatal("a removed device must be refused")
	}
}

func backupHas(t *testing.T, app *App, content string) bool {
	found := false
	filepath.Walk(filepath.Join(app.Paths.Trash, "sync-backups"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); string(b) == content {
				found = true
			}
		}
		return nil
	})
	return found
}

func TestSyncPlanning(t *testing.T) {
	f := func(p, h string) SyncFile { return SyncFile{Path: p, Hash: h} }
	acts := planSync(
		[]SyncFile{f("same", "1"), f("mine-changed", "2"), f("theirs-changed", "1"), f("both", "x"), f("only-here", "1"), f("deleted-there", "1")},
		[]SyncFile{f("same", "1"), f("mine-changed", "1"), f("theirs-changed", "3"), f("both", "y"), f("only-there", "1")},
		map[string]string{"mine-changed": "1", "theirs-changed": "1", "both": "1", "deleted-there": "1"})
	got := map[string]string{}
	for _, a := range acts {
		got[a.Path] = a.Action
	}
	want := map[string]string{"mine-changed": "push", "theirs-changed": "pull", "both": "conflict", "only-here": "push", "only-there": "pull"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q want %q", k, got[k], v)
		}
	}
	if _, ok := got["same"]; ok {
		t.Error("identical files need nothing")
	}
	if _, ok := got["deleted-there"]; ok {
		t.Error("a deletion is never propagated (saves are not deleted by a synchronisation)")
	}
}

func TestSyncFileSafety(t *testing.T) {
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), t.TempDir(), false))
	p, _ := app.Profiles.Create("Ann", Avatar{})
	if err := app.writeSyncFile(p.ID, "saves/../../evil", []byte("x"), "", t.TempDir()); err == nil {
		t.Fatal("paths outside the save folder must be refused")
	}
	if err := app.writeSyncFile(p.ID, "saves/a/b.bin", []byte("data"), "0000", t.TempDir()); err == nil {
		t.Fatal("data that does not match its checksum must be refused")
	}
	if fileExists(filepath.Join(app.Profiles.SaveDir(p.ID), "a", "b.bin")) {
		t.Fatal("nothing may be written for damaged data")
	}
	if _, err := app.syncLocalPath(p.ID, "keys/prod.keys"); err == nil {
		t.Fatal("only saves and the profile are synchronised")
	}
}

func TestFeatureFlagOff(t *testing.T) {
	app, _ := testNewApp(t, t.TempDir())
	t.Setenv("NEXUS_EXPERIMENTAL", "")
	if app.Feature("devicelink") {
		t.Fatal("experimental features are off by default")
	}
	t.Setenv("NEXUS_EXPERIMENTAL", "other, devicelink")
	if !app.Feature("devicelink") {
		t.Fatal("the flag switches it on")
	}
}
