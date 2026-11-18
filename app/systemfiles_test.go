package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// buildRomFSRoot builds a RomFS with the given files in its root directory.
func buildRomFSRoot(files map[string][]byte) []byte {
	var meta, data bytes.Buffer
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		d := files[n]
		e := make([]byte, 0x20)
		binary.LittleEndian.PutUint32(e[4:], 0xFFFFFFFF)
		binary.LittleEndian.PutUint64(e[8:], uint64(data.Len()))
		binary.LittleEndian.PutUint64(e[16:], uint64(len(d)))
		binary.LittleEndian.PutUint32(e[24:], 0xFFFFFFFF)
		binary.LittleEndian.PutUint32(e[28:], uint32(len(n)))
		meta.Write(e)
		meta.WriteString(n)
		for meta.Len()%4 != 0 {
			meta.WriteByte(0)
		}
		data.Write(d)
		for data.Len()%16 != 0 {
			data.WriteByte(0)
		}
	}
	hdr := make([]byte, 0x50)
	binary.LittleEndian.PutUint64(hdr[0:], 0x50)
	binary.LittleEndian.PutUint64(hdr[0x38:], 0x50)
	binary.LittleEndian.PutUint64(hdr[0x40:], uint64(meta.Len()))
	dataOff := (0x50 + uint64(meta.Len()) + 0xF) &^ 0xF
	binary.LittleEndian.PutUint64(hdr[0x48:], dataOff)
	out := make([]byte, dataOff)
	copy(out, hdr)
	copy(out[0x50:], meta.Bytes())
	return append(out, data.Bytes()...)
}

// systemVersionFile builds nn::settings::system::FirmwareVersion (0x100 bytes).
func systemVersionFile(major, minor, micro byte, display string) []byte {
	b := make([]byte, 0x100)
	b[0], b[1], b[2] = major, minor, micro
	b[4], b[5] = 1, 0
	copy(b[0x08:], "NX")
	copy(b[0x68:], display)
	copy(b[0x80:], "NintendoSDK Firmware for NX "+display+"-1.0")
	return b
}

// writeTestFirmware writes a synthetic firmware dump (not Nintendo's data):
// a SystemVersion archive, an avatar archive and some filler contents.
func writeTestFirmware(t *testing.T, dir string, k testKeys, display string, withVersion bool, filler int) {
	os.MkdirAll(dir, 0o755)
	id := 0
	name := func() string { id++; return fmt.Sprintf("%032x.nca", 0xabc000+id) }
	if withVersion {
		var maj, min, mic byte
		fmt.Sscanf(display, "%d.%d.%d", &maj, &min, &mic)
		rom := buildRomFSRoot(map[string][]byte{"file": systemVersionFile(maj, min, mic, display)})
		os.WriteFile(filepath.Join(dir, name()), buildNCA(t, k, 4, systemVersionID, rom, nil, nil), 0o644)
	}
	tree := map[string]map[string][]byte{"chara": {"chr_test.szs": yaz0Literal(bytes.Repeat([]byte{200, 30, 30, 255}, 256*256))}}
	os.WriteFile(filepath.Join(dir, name()), buildNCA(t, k, 4, avatarArchiveID, buildRomFSTree(tree), nil, nil), 0o644)
	for i := 0; i < filler; i++ {
		os.WriteFile(filepath.Join(dir, name()), buildNCA(t, k, 0, 0x0100000000001000+uint64(i), nil, nil, nil), 0o644)
	}
}

func zipFirmwareDir(t *testing.T, dir, dst string) {
	f, _ := os.Create(dst)
	zw := zip.NewWriter(f)
	entries, _ := os.ReadDir(dir)
	for i, e := range entries {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		name := "Firmware 19.0.1/" + e.Name()
		if i == 0 { // also exercise the split "<id>.nca/00" layout
			name = "Firmware 19.0.1/" + e.Name() + "/00"
		}
		w, _ := zw.Create(name)
		w.Write(data)
	}
	zw.Close()
	f.Close()
}

func waitSysJob(t *testing.T, app *App) *SysJob {
	for i := 0; i < 400; i++ {
		if j := app.Sys.Job(); j != nil && j.Done {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("system file job did not finish")
	return nil
}

func TestSystemFilesKeys(t *testing.T) {
	app, root := newTestApp(t, "")
	app.Sys.noBackground = true
	in := t.TempDir()
	bad := map[string]string{
		"garbage.keys":  "this is not a key file\nhello world\n",
		"nohdr.keys":    "key_area_key_application_00 = " + strings.Repeat("33", 16) + "\n",
		"shorthdr.keys": "header_key = 1122\nkey_area_key_application_00 = " + strings.Repeat("33", 16) + "\n",
		"nokak.keys":    "header_key = " + strings.Repeat("11", 32) + "\n",
	}
	for n, c := range bad {
		os.WriteFile(filepath.Join(in, n), []byte(c), 0o644)
		if _, err := app.Sys.InstallKeys(filepath.Join(in, n), ""); err == nil {
			t.Fatalf("%s accepted", n)
		} else {
			t.Logf("%s: %v", n, err)
		}
	}
	if fileExists(app.Sys.ProdKeys()) {
		t.Fatal("invalid keys must not be installed")
	}
	if _, err := app.Sys.InstallKeys(filepath.Join(in, "missing.keys"), ""); err == nil {
		t.Fatal("missing file accepted")
	}
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	good := filepath.Join(in, "prod.keys")
	os.WriteFile(good, []byte(fmt.Sprintf("; my keys\nheader_key = %s\nkey_area_key_application_00 = %s\ntitlekek_00 = %s\nmaster_key_00 = %s\nmaster_key_12 = %s\n",
		hex.EncodeToString(k.header), hex.EncodeToString(k.kak), hex.EncodeToString(k.kek), strings.Repeat("ab", 16), strings.Repeat("cd", 16))), 0o644)
	// the engine data folder already has a different prod.keys: it must be backed up
	engKeys := filepath.Join(root, "System", "Engine", "keys", "prod.keys")
	os.MkdirAll(filepath.Dir(engKeys), 0o755)
	os.WriteFile(engKeys, []byte("header_key = "+strings.Repeat("99", 32)+"\n"), 0o644)
	ki, err := app.Sys.InstallKeys(good, "")
	if err != nil {
		t.Fatal(err)
	}
	if !ki.Valid || ki.Count != 5 || ki.MasterKeyMax != 0x12 || ki.SupportsUpTo != "19.0.0" || ki.KeyAreaKeys != 1 {
		t.Fatalf("%+v", ki)
	}
	if !sameFile(good, engKeys) || engKeys != filepath.Join(app.Emu.KeysDir(), "prod.keys") {
		t.Fatal("keys not installed into the NEXUS engine")
	}
	backups, _ := filepath.Glob(engKeys + ".nexus-backup-*")
	if len(backups) != 1 {
		t.Fatal("previous engine keys not backed up")
	}
	st := app.Sys.Status()
	for _, e := range st["engines"].([]EngineSystemState) {
		if e.Keys != "match" {
			t.Fatalf("engine %s keys %s", e.Engine, e.Keys)
		}
	}
	// replacing keys archives the old file
	if _, err := app.Sys.InstallKeys(good, ""); err != nil {
		t.Fatal(err)
	}
	arch, _ := filepath.Glob(filepath.Join(app.Paths.Trash, "system-files-*", "keys", "prod.keys"))
	if len(arch) == 0 {
		t.Fatal("old keys not archived")
	}
}

func TestSystemFilesFirmware(t *testing.T) {
	app, root := newTestApp(t, "")
	app.Sys.noBackground = true
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	keys := writeKeys(t, t.TempDir(), k)
	fwDir := filepath.Join(t.TempDir(), "fw")
	writeTestFirmware(t, fwDir, k, "19.0.1", true, 6)

	// firmware before keys: refused with clear instructions
	if err := app.Sys.InstallFirmware(fwDir); err == nil || !strings.Contains(err.Error(), "prod.keys") {
		t.Fatalf("expected keys first: %v", err)
	}
	if _, err := app.Sys.InstallKeys(keys, ""); err != nil {
		t.Fatal(err)
	}
	// incomplete firmware (no SystemVersion): rejected, nothing installed
	badDir := filepath.Join(t.TempDir(), "bad")
	writeTestFirmware(t, badDir, k, "", false, 3)
	if err := app.Sys.InstallFirmware(badDir); err != nil {
		t.Fatal(err)
	}
	if j := waitSysJob(t, app); j.Error == "" || !strings.Contains(j.Error, "SystemVersion") {
		t.Fatalf("incomplete firmware accepted: %+v", j)
	}
	if dirHasFiles(app.Sys.Registered()) {
		t.Fatal("rejected firmware was installed")
	}
	// firmware from a different console's keys: rejected
	other := testKeys{header: bytes.Repeat([]byte{0x55}, 32), kak: bytes.Repeat([]byte{0x66}, 16), kek: k.kek}
	otherDir := filepath.Join(t.TempDir(), "other")
	writeTestFirmware(t, otherDir, other, "18.0.0", true, 2)
	app.Sys.InstallFirmware(otherDir)
	if j := waitSysJob(t, app); j.Error == "" {
		t.Fatal("undecryptable firmware accepted")
	}
	// a non-firmware folder
	if err := app.Sys.InstallFirmware(t.TempDir()); err == nil {
		t.Fatal("empty folder accepted")
	}
	// valid firmware as a zip (with one split entry)
	zipPath := filepath.Join(t.TempDir(), "fw.zip")
	zipFirmwareDir(t, fwDir, zipPath)
	if err := app.Sys.InstallFirmware(zipPath); err != nil {
		t.Fatal(err)
	}
	j := waitSysJob(t, app)
	if j.Error != "" {
		t.Fatal(j.Error)
	}
	fi, state := app.Sys.FirmwareStatus()
	if state != "ok" || fi.Version != "19.0.1" || fi.Numeric != "19.0.1" || fi.NCAs != 8 || fi.Platform != "NX" || fi.InstalledAt == 0 {
		t.Fatalf("%s %+v", state, fi)
	}
	// the NEXUS engine received it in its layout and reports the same version
	engReg := filepath.Join(root, "System", "Engine", "nand", "system", "Contents", "registered")
	le, _ := os.ReadDir(engReg)
	if len(le) != 8 || le[0].IsDir() || engReg != app.Emu.FirmwareDir() || !app.Emu.FirmwarePresent() {
		t.Fatalf("engine layout wrong: %d", len(le))
	}
	for _, e := range app.Sys.EngineStates(fi) {
		if e.Firmware != "19.0.1" || e.FWState != "match" {
			t.Fatalf("engine %s: %+v", e.Engine, e)
		}
	}
	// keys now verified against the firmware; foreign keys are refused
	if ki := app.Sys.KeysStatus(); ki.Verified != "decrypts the installed firmware" {
		t.Fatalf("verified=%q", ki.Verified)
	}
	foreign := writeKeys(t, t.TempDir(), other)
	if _, err := app.Sys.InstallKeys(foreign, ""); err == nil || !strings.Contains(err.Error(), "cannot decrypt") {
		t.Fatalf("foreign keys accepted: %v", err)
	}
	// profile icons come from the NEXUS firmware copy
	if st := app.Avatars.Ensure(true); st.State != "ready" || st.Count != 1 {
		t.Fatalf("avatars %+v", st)
	}
	// update to a newer firmware from a folder: old one archived
	newDir := filepath.Join(t.TempDir(), "fw20")
	writeTestFirmware(t, newDir, k, "20.1.0", true, 4)
	app.Sys.InstallFirmware(newDir)
	if j := waitSysJob(t, app); j.Error != "" {
		t.Fatal(j.Error)
	}
	if fi, _ := app.Sys.FirmwareStatus(); fi.Version != "20.1.0" {
		t.Fatal("update not applied")
	}
	if arch, _ := filepath.Glob(filepath.Join(app.Paths.Trash, "system-files-*", "firmware", "Firmware")); len(arch) == 0 {
		t.Fatal("previous firmware not archived")
	}
	// removal archives, never deletes
	dst, err := app.Sys.Remove("firmware", true)
	if err != nil || !dirExists(filepath.Join(dst, "Firmware")) || dirHasFiles(engReg) {
		t.Fatalf("remove: %v %s", err, dst)
	}
	if _, st := app.Sys.FirmwareStatus(); st != "not-installed" {
		t.Fatal(st)
	}
}

// TestTitleKeysNeverBreakProdKeys: the Steam Deck report. Installing title.keys alone used to
// move the installed prod.keys to the archive and then copy from where it had been: "not
// accepted", and prod.keys gone. Each file is now checked and installed on its own.
func TestTitleKeysNeverBreakProdKeys(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Sys.noBackground = true
	in := t.TempDir()
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	prod := filepath.Join(in, "prod.keys")
	os.WriteFile(prod, []byte(fmt.Sprintf("header_key = %s\nkey_area_key_application_00 = %s\ntitlekek_00 = %s\nmaster_key_00 = %s\n",
		hex.EncodeToString(k.header), hex.EncodeToString(k.kak), hex.EncodeToString(k.kek), strings.Repeat("ab", 16))), 0o644)
	if _, err := app.Sys.InstallKeys(prod, ""); err != nil {
		t.Fatal(err)
	}
	installed, _ := os.ReadFile(app.Sys.ProdKeys())
	intact := func(step string) {
		t.Helper()
		now, err := os.ReadFile(app.Sys.ProdKeys())
		if err != nil || !bytes.Equal(now, installed) || !app.Sys.KeysStatus().Valid {
			t.Fatalf("%s: prod.keys changed or lost (%v)", step, err)
		}
	}
	// invalid title keys: refused, nothing changed
	bad := filepath.Join(in, "bad-title.keys")
	os.WriteFile(bad, []byte("hello\nworld\n"), 0o644)
	if _, err := app.Sys.InstallTitleKeys(bad); err == nil {
		t.Fatal("invalid title.keys accepted")
	}
	intact("invalid title.keys")
	if fileExists(app.Sys.TitleKeys()) {
		t.Fatal("an invalid title.keys was installed")
	}
	// prod.keys chosen by mistake as title.keys: refused with a clear reason
	if _, err := app.Sys.InstallTitleKeys(prod); err == nil || !strings.Contains(err.Error(), "not a title.keys") {
		t.Fatalf("prod.keys as title.keys: %v", err)
	}
	intact("prod.keys as title.keys")
	// valid title keys: installed, prod.keys untouched
	good := filepath.Join(in, "title.keys")
	os.WriteFile(good, []byte(strings.Repeat("01", 16)+" = "+strings.Repeat("02", 16)+"\r\n"+strings.Repeat("03", 16)+"="+strings.Repeat("04", 16)+"\n"), 0o644)
	ki, err := app.Sys.InstallTitleKeys(good)
	if err != nil || ki.TitleKeys != 2 {
		t.Fatalf("%+v %v", ki, err)
	}
	intact("valid title.keys")
	// replacing title keys keeps a copy of the old ones; prod.keys still untouched
	if _, err := app.Sys.InstallTitleKeys(good); err != nil {
		t.Fatal(err)
	}
	if arch, _ := filepath.Glob(filepath.Join(app.Paths.Trash, "system-files-*", "title-keys", "title.keys")); len(arch) == 0 {
		t.Fatal("old title.keys not kept")
	}
	intact("title.keys replaced")
	// the old route (prod.keys reinstalled from its own installed path, with title keys) works
	if _, err := app.Sys.InstallKeys(app.Sys.ProdKeys(), good); err != nil {
		t.Fatal(err)
	}
	intact("prod.keys from its installed path")
}
