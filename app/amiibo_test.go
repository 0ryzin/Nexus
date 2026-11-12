package main

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rawAmiiboDump builds the first pages of a raw (as read from a tag) NTAG215 dump: serial number
// with its checksums, lock bytes, capability container and the amiibo ID at 0x54. The encrypted
// areas are filler; NEXUS only reads the unencrypted header.
func rawAmiiboDump(id string, uid [7]byte) []byte {
	b := bytes.Repeat([]byte{0x5A}, 540)
	copy(b[0:3], uid[0:3])
	b[3] = 0x88 ^ uid[0] ^ uid[1] ^ uid[2]
	copy(b[4:8], uid[3:7])
	b[8] = uid[3] ^ uid[4] ^ uid[5] ^ uid[6]
	b[9] = 0x48
	b[10], b[11] = 0x0F, 0xE0
	copy(b[12:16], []byte{0xF1, 0x10, 0xFF, 0xEE})
	idb, _ := hex.DecodeString(id)
	copy(b[0x54:0x5C], idb)
	return b
}

// TestVirtualAmiiboImage: the image NEXUS creates has the structure the engine checks before it
// accepts a plain amiibo (NfcDevice/IsAmiiboValid: serial checksums, lock bytes, capability
// container, tag type, dynamic lock, CFG0/CFG1), the amiibo ID in the model info, and a fresh
// random serial number each time.
func TestVirtualAmiiboImage(t *testing.T) {
	const id = "0100000000040002"
	a, err := VirtualAmiiboImage(id)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := VirtualAmiiboImage(id)
	if len(a) != 540 || bytes.Equal(a[0x1D4:0x1DC], b[0x1D4:0x1DC]) {
		t.Fatal("size or serial number not random")
	}
	u := a[0x1D4:0x1DC]
	checks := []struct {
		name string
		ok   bool
	}{
		{"uid[0] NXP", u[0] == 0x04},
		{"BCC0", u[3] == 0x88^u[0]^u[1]^u[2]},
		{"BCC1", a[0] == u[4]^u[5]^u[6]^u[7]},
		{"static lock", a[2] == 0x0F && a[3] == 0xE0},
		{"CC", bytes.Equal(a[4:8], []byte{0xF1, 0x10, 0xFF, 0xEE})},
		{"tag type", a[0x1E3] == 0x02},
		{"amiibo id", hex.EncodeToString(a[0x1DC:0x1E4]) == id},
		{"dynamic lock", a[0x208] == 0x01 && a[0x209] == 0x00 && a[0x20A] == 0x0F},
		{"CFG0", bytes.Equal(a[0x20C:0x210], []byte{0, 0, 0, 4})},
		{"CFG1", a[0x210] == 0x5F},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("%s wrong", c.name)
		}
	}
	kind, aid, uid, err := ParseAmiiboDump(a)
	if err != nil || kind != "dump" || aid != id || len(uid) != 14 {
		t.Fatalf("parse own image: %s %s %s %v", kind, aid, uid, err)
	}
	for _, bad := range []string{"", "0100000000040001", "zz00000000040002", "01000000000400"} {
		if _, err := VirtualAmiiboImage(bad); err == nil {
			t.Errorf("accepted id %q", bad)
		}
	}
}

func TestParseAmiiboDump(t *testing.T) {
	raw := rawAmiiboDump("01c1000002410502", [7]byte{4, 1, 2, 3, 4, 5, 6})
	kind, aid, _, err := ParseAmiiboDump(raw)
	if err != nil || kind != "dump" || aid != "01c1000002410502" {
		t.Fatalf("raw: %s %s %v", kind, aid, err)
	}
	if _, _, _, err := ParseAmiiboDump(raw[:532]); err != nil {
		t.Fatal("532-byte dump refused")
	}
	if _, _, _, err := ParseAmiiboDump(append(append([]byte{}, raw...), make([]byte, 32)...)); err != nil {
		t.Fatal("572-byte dump refused")
	}
	if k, _, _, err := ParseAmiiboDump(make([]byte, 1024)); err != nil || k != "mifare" {
		t.Fatal("MIFARE dump refused")
	}
	damaged := append([]byte{}, raw...)
	damaged[3] ^= 1
	if _, _, _, err := ParseAmiiboDump(damaged); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("damaged serial accepted: %v", err)
	}
	for _, n := range []int{0, 100, 539, 2048} {
		if _, _, _, err := ParseAmiiboDump(make([]byte, n)); err == nil {
			t.Errorf("size %d accepted", n)
		}
	}
	if _, _, _, err := ParseAmiiboDump(make([]byte, 540)); err == nil {
		t.Error("zero-filled file accepted")
	}
}

// TestAmiiboLibrary: import copies (the original is untouched), duplicates are refused, entries
// are renamed and assigned to a user, survive a restart, and removing one deletes only NEXUS's copy.
func TestAmiiboLibrary(t *testing.T) {
	app, root := newTestApp(t, "")
	src := filepath.Join(root, "mario.bin")
	raw := rawAmiiboDump("0000000000000002", [7]byte{4, 9, 9, 9, 1, 2, 3})
	os.WriteFile(src, raw, 0o644)
	a, err := app.Amiibo.Import(src, "")
	if err != nil || a.Name != "mario" || a.Kind != "dump" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := app.Amiibo.Import(src, ""); err == nil {
		t.Fatal("duplicate imported")
	}
	notAmiibo := filepath.Join(root, "notes.txt")
	os.WriteFile(notAmiibo, []byte("hello"), 0o644)
	if _, err := app.Amiibo.Import(notAmiibo, ""); err == nil {
		t.Fatal("text file imported")
	}
	p, _ := app.Profiles.Create("Kai", Avatar{Kind: "glyph"})
	if _, err := app.Amiibo.Update(a.ID, func(x *Amiibo) { x.Name, x.Owner = "My Mario", p.ID }); err != nil {
		t.Fatal(err)
	}
	v, err := app.Amiibo.CreateVirtual("0x0100000000040002", "Link", "Zelda", p.ID)
	if err != nil || v.Kind != "virtual" || v.Series != "Zelda" {
		t.Fatalf("%+v %v", v, err)
	}
	// restart
	again := NewAmiiboStore(app, app.Amiibo.dir)
	if l := again.List(); len(l) != 2 || l[0].Name != "Link" || l[1].Name != "My Mario" || l[1].Owner != p.ID {
		t.Fatalf("after restart: %+v", l)
	}
	if err := app.Amiibo.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	if fileExists(app.Amiibo.Path(a)) {
		t.Fatal("copy left behind")
	}
	if b, _ := os.ReadFile(src); !bytes.Equal(b, raw) {
		t.Fatal("original changed")
	}
}

func TestAmiiboKeys(t *testing.T) {
	app, root := newTestApp(t, "")
	bad := filepath.Join(root, "k.bin")
	os.WriteFile(bad, make([]byte, 80), 0o644)
	if err := app.Amiibo.InstallKeys(bad); err == nil || app.Amiibo.KeysInstalled() {
		t.Fatal("wrong-size keys accepted")
	}
	good := filepath.Join(root, "key_retail.bin")
	os.WriteFile(good, make([]byte, 160), 0o644)
	if err := app.Amiibo.InstallKeys(good); err != nil || !app.Amiibo.KeysInstalled() {
		t.Fatalf("keys not installed: %v", err)
	}
	if filepath.Dir(app.Amiibo.keysPath()) != app.Emu.KeysDir() {
		t.Fatal("keys not next to prod.keys")
	}
}

// TestAmiiboInGame: NEXUS reports when the game scans, and placing an amiibo hands its file to
// the engine; placing while the game is not scanning gives a clear message.
func TestAmiiboInGame(t *testing.T) {
	app, root := newTestApp(t, "")
	if err := app.Amiibo.Place("x"); err == nil {
		t.Fatal("placed with no game running")
	}
	src := filepath.Join(root, "a.bin")
	os.WriteFile(src, rawAmiiboDump("0000000000000002", [7]byte{4, 7, 7, 7, 1, 1, 1}), 0o644)
	am, _ := app.Amiibo.Import(src, "")
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	_, f := playing(t, app)
	if err := app.Amiibo.Place(am.ID); err == nil || !strings.Contains(err.Error(), "not scanning") {
		t.Fatalf("placed while not scanning: %v", err)
	}
	f.Scan()
	if waitEvent(t, ev, "amiibo.scanning") == nil {
		t.Fatal("no scanning event")
	}
	for i := 0; i < 100 && app.Emu.State().Amiibo != "scanning"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if err := app.Amiibo.Place(am.ID); err != nil {
		t.Fatal(err)
	}
	if f.Status().Amiibo != "placed" {
		t.Fatal("engine did not get the amiibo")
	}
	if err := app.Amiibo.Place(""); err != nil || f.Status().Amiibo != "scanning" {
		t.Fatal("could not take the amiibo away")
	}
	app.Emu.Stop(true)
}
