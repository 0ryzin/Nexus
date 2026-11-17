package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v2rec frames a record like the engine (the 8-byte payload hash is a stand-in: NEXUS copies it
// and never checks it; the engine does when it loads the log).
func v2rec(kind uint16, key uint64, payload string) []byte {
	h := make([]byte, v2RecordHeader)
	binary.LittleEndian.PutUint32(h, v2RecordMagic)
	binary.LittleEndian.PutUint16(h[4:], kind)
	binary.LittleEndian.PutUint32(h[8:], uint32(len(payload)))
	binary.LittleEndian.PutUint64(h[12:], key)
	binary.LittleEndian.PutUint32(h[20:], crc32.ChecksumIEEE(h[:20]))
	out := append(h, payload...)
	out = append(out, []byte("HASHHASH")...)
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	return out
}

func tomb(kind uint16, key uint64) []byte {
	return v2rec(v2KindTombstone, key, string([]byte{byte(kind), byte(kind >> 8)}))
}

func writeLog(t *testing.T, path string, recs ...[]byte) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	b := v2Header(1)
	for _, r := range recs {
		b = append(b, r...)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func liveOf(t *testing.T, path string) map[rk]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, recs, end, err := parseV2Log(b)
	if err != nil || end != len(b) {
		t.Fatalf("log not clean: %v end %d of %d", err, end, len(b))
	}
	live, _, _ := liveRecords(b, recs)
	out := map[rk]string{}
	for k, r := range live {
		size := binary.LittleEndian.Uint32(b[r.off+8:])
		out[k] = string(b[r.off+v2RecordHeader : r.off+v2RecordHeader+int(size)])
	}
	return out
}

func shaderGame(t *testing.T, app *App) *Game {
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	return &g
}

// TestShaderPackRoundTrip: a pack carries the live portable records only (no driver data, nothing
// a tombstone removed, the latest version of each), and imports into an empty device whole.
func TestShaderPackRoundTrip(t *testing.T) {
	pc, _ := newTestApp(t, "")
	g := shaderGame(t, pc)
	writeLog(t, pc.Emu.ShaderLogPath(g.TitleID),
		v2rec(v2KindProgram, 1, "prog-1"), v2rec(v2KindGraphics, 2, "gfx-old"), v2rec(v2KindGraphics, 2, "gfx-new"),
		v2rec(v2KindCompute, 3, "comp"), v2rec(v2KindUsed, 9, "session"), v2rec(7, 4, "device identity"),
		v2rec(8, 5, "driver binary"), v2rec(v2KindGraphics, 6, "removed"), tomb(v2KindGraphics, 6))
	var pack bytes.Buffer
	info, err := pc.Emu.ExportShaders(g, &pack)
	if err != nil {
		t.Fatal(err)
	}
	if info.Programs != 1 || info.Graphics != 1 || info.Compute != 1 || info.Sessions != 1 {
		t.Fatalf("%+v", info)
	}
	deck, _ := newTestApp(t, "")
	dg := shaderGame(t, deck)
	res, err := deck.Emu.ImportShaders(bytes.NewReader(pack.Bytes()), func(string) *Game { return dg })
	if err != nil || res.Added != 4 || res.Backup != "" {
		t.Fatalf("%+v %v", res, err)
	}
	got := liveOf(t, deck.Emu.ShaderLogPath(dg.TitleID))
	want := map[rk]string{{1, 1}: "prog-1", {2, 2}: "gfx-new", {3, 3}: "comp", {5, 9}: "session"}
	if len(got) != len(want) {
		t.Fatalf("imported %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%v: %q, want %q", k, got[k], v)
		}
	}
	// the same pack again adds nothing
	res, _ = deck.Emu.ImportShaders(bytes.NewReader(pack.Bytes()), func(string) *Game { return dg })
	if res.Added != 0 || res.Present != 4 {
		t.Fatalf("second import: %+v", res)
	}
}

// TestShaderImportNeverOverrides: an import only adds what this device lacks; a record this device
// has (even a different version) is kept, one it removed stays removed, a cut-off tail is dropped,
// and the old log is kept as a backup.
func TestShaderImportNeverOverrides(t *testing.T) {
	src, _ := newTestApp(t, "")
	sg := shaderGame(t, src)
	writeLog(t, src.Emu.ShaderLogPath(sg.TitleID),
		v2rec(v2KindGraphics, 1, "theirs"), v2rec(v2KindGraphics, 2, "new to me"), v2rec(v2KindCompute, 3, "I removed this"))
	var pack bytes.Buffer
	if _, err := src.Emu.ExportShaders(sg, &pack); err != nil {
		t.Fatal(err)
	}
	dst, _ := newTestApp(t, "")
	dg := shaderGame(t, dst)
	path := dst.Emu.ShaderLogPath(dg.TitleID)
	writeLog(t, path, v2rec(v2KindGraphics, 1, "mine"), v2rec(v2KindCompute, 3, "x"), tomb(v2KindCompute, 3))
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.Write(v2rec(v2KindGraphics, 7, "cut off")[:20]) // a torn tail, as after a crash
	f.Close()
	before, _ := os.ReadFile(path)
	res, err := dst.Emu.ImportShaders(bytes.NewReader(pack.Bytes()), func(string) *Game { return dg })
	if err != nil || res.Added != 1 || res.Present != 1 || res.Removed != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	got := liveOf(t, path)
	if got[rk{2, 1}] != "mine" || got[rk{2, 2}] != "new to me" || got[rk{3, 3}] != "" {
		t.Fatalf("after import: %v", got)
	}
	// appended, never rewritten: what was there (torn tail included) is unchanged in front
	if b, _ := os.ReadFile(path); !bytes.HasPrefix(b, before) {
		t.Fatal("the previous log was rewritten")
	}
}

// TestShaderPackRejected: damaged, incomplete or foreign packs, a game not in the library and a
// running game change nothing.
func TestShaderPackRejected(t *testing.T) {
	app, _ := newTestApp(t, "")
	g := shaderGame(t, app)
	writeLog(t, app.Emu.ShaderLogPath(g.TitleID), v2rec(v2KindProgram, 1, "p"), v2rec(v2KindGraphics, 2, "g"))
	var pack bytes.Buffer
	if _, err := app.Emu.ExportShaders(g, &pack); err != nil {
		t.Fatal(err)
	}
	other, _ := newTestApp(t, "")
	og := shaderGame(t, other)
	path := other.Emu.ShaderLogPath(og.TitleID)
	writeLog(t, path, v2rec(v2KindProgram, 5, "local"))
	before, _ := os.ReadFile(path)
	find := func(string) *Game { return og }
	damaged := bytes.Clone(pack.Bytes())
	damaged[len(damaged)-3] ^= 0xff
	cases := map[string][]byte{
		"checksum":   damaged,
		"incomplete": pack.Bytes()[:pack.Len()-16],
		"not a pack": []byte("PK\x03\x04 a zip file"),
	}
	for name, b := range cases {
		if _, err := other.Emu.ImportShaders(bytes.NewReader(b), find); err == nil {
			t.Errorf("%s accepted", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
	if _, err := other.Emu.ImportShaders(bytes.NewReader(pack.Bytes()), func(string) *Game { return nil }); err == nil || !strings.Contains(err.Error(), "not in your library") {
		t.Errorf("unknown game: %v", err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("a rejected pack changed the shader cache")
	}
	if _, err := app.Emu.ExportShaders(&Game{Title: "x", TitleID: "0100AAAA00099999"}, &bytes.Buffer{}); err == nil {
		t.Error("export of a game with no shader data")
	}
}
