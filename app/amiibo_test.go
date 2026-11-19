package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
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

// TestAmiiboCreator: since 0.8.1 no amiibo is created (no generator, not even by copying a
// virtual one); entries made by earlier versions keep working: details, edit (data unchanged),
// duplicate of a dump (exact copy; the original unchanged), picture, and removal of everything.
func TestAmiiboCreator(t *testing.T) {
	app, root := newTestApp(t, "")
	p, _ := app.Profiles.Create("Kai", Avatar{Kind: "glyph"})
	if _, err := app.Amiibo.CreateVirtualWith("0100000000040002", AmiiboDetails{Name: "x"}); !errors.Is(err, errVirtualAmiiboRetired) {
		t.Fatalf("an amiibo was generated: %v", err)
	}
	v, err := app.Amiibo.createVirtualLegacy("0100000000040002", AmiiboDetails{Name: "My Link", Character: "Link", Series: "The Legend of Zelda", GameSeries: "The Legend of Zelda", Type: "Figure", Notes: "for Tears of the Kingdom", Owner: p.ID})
	if err != nil || v.Kind != "virtual" || v.Character != "Link" || v.Notes == "" || v.Owner != p.ID {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := app.Amiibo.createVirtualLegacy("0100000000040002", AmiiboDetails{Name: strings.Repeat("x", 41)}); err == nil {
		t.Fatal("name too long accepted")
	}
	before, _ := os.ReadFile(app.Amiibo.Path(v))
	e, err := app.Amiibo.Edit(v.ID, AmiiboDetails{Name: "Hero", Character: "Link", Type: "Card", Notes: "edited"})
	if err != nil || e.Name != "Hero" || e.Type != "Card" || e.Owner != "" || e.AmiiboID != v.AmiiboID || e.UID != v.UID {
		t.Fatalf("edit %+v %v", e, err)
	}
	if after, _ := os.ReadFile(app.Amiibo.Path(v)); !bytes.Equal(before, after) {
		t.Fatal("editing changed the amiibo data")
	}
	if _, err := app.Amiibo.Duplicate(v.ID); !errors.Is(err, errVirtualAmiiboRetired) {
		t.Fatalf("copying a virtual amiibo generated a new one: %v", err)
	}
	src := filepath.Join(root, "dump.bin")
	raw := rawAmiiboDump("0000000000000002", [7]byte{4, 3, 3, 3, 4, 4, 4})
	os.WriteFile(src, raw, 0o644)
	dump, _ := app.Amiibo.Import(src, "")
	dd, err := app.Amiibo.Duplicate(dump.ID)
	if err != nil || dd.UID != dump.UID || dd.Kind != "dump" {
		t.Fatalf("dump duplicate %+v %v", dd, err)
	}
	if b, _ := os.ReadFile(app.Amiibo.Path(dd)); !bytes.Equal(b, raw) {
		t.Fatal("dump copy differs")
	}
	// picture: only real images
	if _, err := app.Amiibo.SetArt(v.ID, []byte("not a picture")); err == nil {
		t.Fatal("text accepted as a picture")
	}
	if a, err := app.Amiibo.SetArt(v.ID, jpegBytes(64, color.RGBA{1, 2, 3, 255})); err != nil || !strings.HasSuffix(a.Art, ".jpg") || !fileExists(app.Amiibo.ArtPath(v.ID)) {
		t.Fatalf("art %v", err)
	}
	art := app.Amiibo.ArtPath(v.ID)
	app.Amiibo.Remove(v.ID)
	if fileExists(art) || fileExists(app.Amiibo.Path(v)) {
		t.Fatal("remove left files")
	}
	if b, _ := os.ReadFile(src); !bytes.Equal(b, raw) {
		t.Fatal("the user's original dump changed")
	}
}

// TestAmiiboLookup: searches the open database by name, character or series (a stand-in server
// with AmiiboAPI's answer format), and handles "nothing found" and an unreachable service.
func TestAmiiboLookup(t *testing.T) {
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		if r.URL.Query().Get("name") == "nothing" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"code":404,"error":"Not Found"}`)
			return
		}
		fmt.Fprint(w, `{"amiibo":[{"amiiboSeries":"Super Smash Bros.","character":"Link","gameSeries":"The Legend of Zelda","head":"01000000","name":"Link","tail":"00040002","type":"Figure"}]}`)
	}))
	defer ts.Close()
	old := amiiboAPIBases
	amiiboAPIBases = []string{ts.URL + "/api/amiibo/"}
	defer func() { amiiboAPIBases = old }()
	l, err := LookupAmiibo("Link", "character")
	if err != nil || len(l) != 1 || l[0].ID() != "0100000000040002" || l[0].Character != "Link" || got != "character=Link" {
		t.Fatalf("%+v %v %q", l, err, got)
	}
	if LookupAmiibo("Smash", "series"); got != "amiiboSeries=Smash" {
		t.Fatal(got)
	}
	if LookupAmiibo("0100000000040002", ""); got != "head=01000000&tail=00040002" {
		t.Fatal(got)
	}
	if l, err := LookupAmiibo("nothing", ""); err != nil || len(l) != 0 {
		t.Fatal("not found", err)
	}
	amiiboAPIBases = []string{"http://127.0.0.1:1/api/amiibo/"}
	if _, err := LookupAmiibo("Link", ""); err == nil || !strings.Contains(err.Error(), "could not be reached") {
		t.Fatal(err)
	}
}

// TestAmiiboInGameVirtual: a NEXUS virtual amiibo is handed to the engine like a dump; a missing
// or damaged data file is reported instead of being passed on.
func TestAmiiboInGameVirtual(t *testing.T) {
	app, _ := newTestApp(t, "")
	v, _ := app.Amiibo.CreateVirtual("0100000000040002", "Link", "", "")
	broken, _ := app.Amiibo.CreateVirtual("0000000000000002", "Mario", "", "")
	_, f := playing(t, app)
	f.Scan()
	for i := 0; i < 100 && app.Emu.State().Amiibo != "scanning"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	os.WriteFile(app.Amiibo.Path(broken), []byte("damaged"), 0o644)
	if err := app.Amiibo.Place(broken.ID); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("damaged entry passed on: %v", err)
	}
	if f.Status().Amiibo != "scanning" {
		t.Fatal("the game received something")
	}
	if err := app.Amiibo.Place(v.ID); err != nil || f.Status().Amiibo != "placed" {
		t.Fatalf("virtual amiibo not placed: %v", err)
	}
	app.Emu.Stop(true)
}
