package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testZipFile is one file of a test archive.
type testZipFile struct {
	name, data string
}

func makeZip(t *testing.T, entries []testZipFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.data))
	}
	zw.Close()
	return buf.Bytes()
}

// ootEntries: a save shaped like the Ocarina of Time remake's under prefix.
func ootEntries(prefix, mark string) []testZipFile {
	var out []testZipFile
	for i := 0; i < 6; i++ {
		out = append(out, testZipFile{prefix + "/slot_0" + string(rune('0'+i)) + "/data.bin", mark + " slot"})
	}
	return append(out, testZipFile{prefix + "/account.sav", mark + " account"}, testZipFile{prefix + "/option.sav", mark + " option"})
}

func writeZipFile(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "Downloads", "my saves.zip")
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A ZIP made from another program's save folder, nested inside extra folders, with several
// games, a device save and a cache folder: every save is found and identified, the cache is not
// offered, only the selected saves are imported, and the archive is unchanged.
func TestZipImportNestedMultipleSelection(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	tom, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	base := "Backup of my PC/emulator/nand/user/save/0000000000000000/"
	var es []testZipFile
	es = append(es, ootEntries(base+testUID+"/0100AAAA00010000", "zip")...)
	es = append(es, testZipFile{base + testUID + "/0100BBBB00010000/progress.dat", "cloud"})
	es = append(es, testZipFile{base + "00000000000000000000000000000000/0100AAAA00010000/dev.bin", "device"})
	es = append(es, testZipFile{"Backup of my PC/emulator/nand/user/save/cache/0100AAAA00010000/shader.bin", "cache"})
	zp := writeZipFile(t, makeZip(t, es))
	before, _ := os.ReadFile(zp)

	insp, err := app.Saves.Inspect(zp, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(insp.Candidates) != 3 {
		t.Fatalf("%d candidates: %+v", len(insp.Candidates), insp.Candidates)
	}
	var picks []SaveImportPick
	for _, c := range insp.Candidates {
		if !c.Confident || c.TitleID == "" {
			t.Fatalf("not identified: %+v", c)
		}
		if strings.Contains(c.path, "cache") {
			t.Fatal("a cache folder was offered as a save")
		}
		if c.TitleID == "0100AAAA00010000" { // select only Alpha Quest (account + device), not the other game
			picks = append(picks, SaveImportPick{ID: c.ID, TitleID: c.TitleID, Device: c.Device})
		}
	}
	if _, err := app.Saves.Import(insp.Token, tom.ID, picks); err != nil {
		t.Fatal(err)
	}
	dir := app.Profiles.SaveDir(tom.ID)
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "slot_05", "data.bin")) != "zip slot" ||
		readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ (device)", "dev.bin")) != "device" {
		t.Fatal("selected saves not imported")
	}
	if entries, _ := filepath.Glob(filepath.Join(dir, "0100BBBB*")); len(entries) != 0 {
		t.Fatalf("an unselected save was imported: %v", entries)
	}
	if after, _ := os.ReadFile(zp); !bytes.Equal(before, after) {
		t.Fatal("the ZIP archive was changed")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.nexus-new")); len(left) != 0 {
		t.Fatalf("staging folders left: %v", left)
	}
}

// Existing saves are only replaced after confirmation, and backed up first.
func TestZipImportOverwriteNeedsConfirmationAndBackup(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	tom, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	dir := app.Profiles.SaveDir(tom.ID)
	writeOOTSave(t, filepath.Join(dir, "0100AAAA00010000-AQ"), "mine")
	data := makeZip(t, ootEntries("0100AAAA00010000", "zip"))
	insp, err := app.Saves.Inspect("", data, "saves.zip")
	if err != nil || len(insp.Candidates) != 1 {
		t.Fatal(err, insp)
	}
	pick := []SaveImportPick{{ID: insp.Candidates[0].ID, TitleID: "0100AAAA00010000"}}
	conf, err := app.Saves.Import(insp.Token, tom.ID, pick)
	if !errors.Is(err, errSaveConflict) || len(conf) != 1 || readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "account.sav")) != "mine account" {
		t.Fatalf("existing save not protected: %v %v", err, conf)
	}
	pick[0].Replace = true
	if _, err := app.Saves.Import(insp.Token, tom.ID, pick); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "account.sav")) != "zip account" {
		t.Fatal("not replaced")
	}
	if len(app.Saves.Backups(tom.ID, "0100AAAA00010000")) != 1 {
		t.Fatal("no backup before replacing")
	}
}

// A damaged entry in the second selected save: nothing is imported, the first game's existing
// save is untouched, no staging folder is left.
func TestZipImportAllOrNothing(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	tom, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	dir := app.Profiles.SaveDir(tom.ID)
	writeOOTSave(t, filepath.Join(dir, "0100AAAA00010000-AQ"), "mine")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range ootEntries("0100AAAA00010000", "zip") {
		w, _ := zw.Create(e.name)
		w.Write([]byte(e.data))
	}
	// stored entry whose CRC does not match its data
	payload := []byte("corrupted progress")
	h := &zip.FileHeader{Name: "0100BBBB00010000/progress.dat", Method: zip.Store, CRC32: crc32.ChecksumIEEE(payload) ^ 0xffff,
		CompressedSize64: uint64(len(payload)), UncompressedSize64: uint64(len(payload))}
	w, err := zw.CreateRaw(h)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(payload)
	zw.Close()

	insp, err := app.Saves.Inspect("", buf.Bytes(), "saves.zip")
	if err != nil || len(insp.Candidates) != 2 {
		t.Fatal(err, insp)
	}
	var picks []SaveImportPick
	for _, c := range insp.Candidates {
		picks = append(picks, SaveImportPick{ID: c.ID, TitleID: c.TitleID, Replace: true})
	}
	_, err = app.Saves.Import(insp.Token, tom.ID, picks)
	if err == nil || !strings.Contains(err.Error(), "Nothing was imported") {
		t.Fatalf("damaged archive imported: %v", err)
	}
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "account.sav")) != "mine account" {
		t.Fatal("an existing save was changed by a failed import")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.nexus-*")); len(left) != 0 {
		t.Fatalf("leftovers: %v", left)
	}
	if len(app.Saves.Backups(tom.ID, "0100AAAA00010000")) != 0 {
		t.Fatal("backup made although nothing was imported")
	}
}

// Malicious or broken archives are refused before anything is extracted.
func TestZipImportRefusesUnsafeArchives(t *testing.T) {
	app, _ := newTestApp(t, "")
	raw := func(h *zip.FileHeader, data []byte) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.CreateRaw(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
		zw.Close()
		return buf.Bytes()
	}
	symlink := func() []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		h := &zip.FileHeader{Name: "0100AAAA00010000/link"}
		h.SetMode(os.ModeSymlink | 0o777)
		w, _ := zw.CreateHeader(h)
		w.Write([]byte("/etc/passwd"))
		zw.Close()
		return buf.Bytes()
	}
	cases := map[string][]byte{
		"traversal":     makeZip(t, []testZipFile{{"0100AAAA00010000/../../../evil.txt", "x"}}),
		"backslash":     makeZip(t, []testZipFile{{"0100AAAA00010000\\..\\..\\evil.txt", "x"}}),
		"absolute":      makeZip(t, []testZipFile{{"/etc/0100AAAA00010000/x", "x"}}),
		"drive":         makeZip(t, []testZipFile{{"C:/Windows/0100AAAA00010000/x", "x"}}),
		"reserved name": makeZip(t, []testZipFile{{"0100AAAA00010000/CON.txt", "x"}}),
		"duplicate":     makeZip(t, []testZipFile{{"0100AAAA00010000/a.sav", "1"}, {"0100AAAA00010000/A.sav", "2"}}),
		"symlink":       symlink(),
		"encrypted":     raw(&zip.FileHeader{Name: "0100AAAA00010000/a.sav", Method: zip.Store, Flags: 0x1, CompressedSize64: 1, UncompressedSize64: 1}, []byte("x")),
		"zip bomb":      raw(&zip.FileHeader{Name: "0100AAAA00010000/a.sav", Method: zip.Deflate, CompressedSize64: 2000, UncompressedSize64: 1 << 31}, make([]byte, 2000)),
		"not a zip":     []byte("this is not a zip archive at all, just text"),
		"truncated":     makeZip(t, ootEntries("0100AAAA00010000", "zip"))[:120],
	}
	outside := filepath.Join(filepath.Dir(app.Paths.Root), "evil.txt")
	for name, data := range cases {
		insp, err := app.Saves.Inspect("", data, name+".zip")
		if err == nil {
			t.Errorf("%s: accepted (%d candidates)", name, len(insp.Candidates))
			continue
		}
		if len(err.Error()) < 30 {
			t.Errorf("%s: unhelpful message %q", name, err)
		}
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a file was written outside NEXUS")
	}
}

// An archive with save files but no IDs: one save whose game the user chooses, with help text.
func TestZipImportUnidentifiedExplains(t *testing.T) {
	app, _ := newTestApp(t, "")
	insp, err := app.Saves.Inspect("", makeZip(t, []testZipFile{{"my save/progress.dat", "x"}}), "loose.zip")
	if err != nil {
		t.Fatal(err)
	}
	if len(insp.Candidates) != 1 || insp.Candidates[0].TitleID != "" || !strings.Contains(insp.Note, "16-character ID") {
		t.Fatalf("%+v", insp)
	}
}

func TestCleanZipName(t *testing.T) {
	ok := map[string]string{"a/b.sav": "a/b.sav", "a\\b.sav": "a/b.sav", "dir/": "dir", "0100AAAA00010000/slot_00/data.bin": "0100AAAA00010000/slot_00/data.bin"}
	for in, want := range ok {
		if got, err := cleanZipName(in); err != nil || got != want {
			t.Errorf("%q -> %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"../a", "a/../../b", "/a", "C:/a", "a/./b", "a//b", "a/nul", "a/lpt9.txt", "a/b.", "a/b ", "a/b?c", "a/\x01b"} {
		if _, err := cleanZipName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
