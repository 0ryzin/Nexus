package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// yaz0Literal encodes data using only literal runs (valid Yaz0).
func yaz0Literal(data []byte) []byte {
	out := []byte("Yaz0")
	sz := make([]byte, 4)
	binary.BigEndian.PutUint32(sz, uint32(len(data)))
	out = append(out, sz...)
	out = append(out, make([]byte, 8)...)
	for i := 0; i < len(data); i += 8 {
		out = append(out, 0xFF)
		end := min(i+8, len(data))
		out = append(out, data[i:end]...)
	}
	return out
}

func TestYaz0BackReference(t *testing.T) {
	// "abc" literal then copy 6 bytes from distance 3 -> "abcabcabc"
	in := []byte("Yaz0\x00\x00\x00\x09\x00\x00\x00\x00\x00\x00\x00\x00")
	in = append(in, 0xE0, 'a', 'b', 'c', 0x40, 0x02)
	out, err := DecompressYaz0(in)
	if err != nil || string(out) != "abcabcabc" {
		t.Fatalf("got %q %v", out, err)
	}
}

// buildRomFSTree builds a RomFS with top-level directories (proper child/sibling links).
func buildRomFSTree(dirs map[string]map[string][]byte) []byte {
	names := make([]string, 0, len(dirs))
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)
	const none = 0xFFFFFFFF
	pad4 := func(b *bytes.Buffer) {
		for b.Len()%4 != 0 {
			b.WriteByte(0)
		}
	}
	// dir offsets: root at 0 (0x18 bytes), then each dir
	dirOff := map[string]uint32{}
	off := uint32(0x18)
	for _, d := range names {
		dirOff[d] = off
		off += uint32((0x18 + len(d) + 3) &^ 3)
	}
	var dm, fm, data bytes.Buffer
	firstFile := map[string]uint32{}
	type fe struct {
		dir, name string
		d         []byte
	}
	var files []fe
	for _, d := range names {
		fns := make([]string, 0)
		for n := range dirs[d] {
			fns = append(fns, n)
		}
		sort.Strings(fns)
		for _, n := range fns {
			files = append(files, fe{d, n, dirs[d][n]})
		}
	}
	fileOffs := make([]uint32, len(files))
	o := uint32(0)
	for i, f := range files {
		fileOffs[i] = o
		o += uint32((0x20 + len(f.name) + 3) &^ 3)
	}
	for i, f := range files {
		if _, ok := firstFile[f.dir]; !ok {
			firstFile[f.dir] = fileOffs[i]
		}
		next := uint32(none)
		if i+1 < len(files) && files[i+1].dir == f.dir {
			next = fileOffs[i+1]
		}
		e := make([]byte, 0x20)
		binary.LittleEndian.PutUint32(e[0:], dirOff[f.dir])
		binary.LittleEndian.PutUint32(e[4:], next)
		binary.LittleEndian.PutUint64(e[8:], uint64(data.Len()))
		binary.LittleEndian.PutUint64(e[16:], uint64(len(f.d)))
		binary.LittleEndian.PutUint32(e[24:], none)
		binary.LittleEndian.PutUint32(e[28:], uint32(len(f.name)))
		fm.Write(e)
		fm.WriteString(f.name)
		pad4(&fm)
		data.Write(f.d)
		for data.Len()%16 != 0 {
			data.WriteByte(0)
		}
	}
	root := make([]byte, 0x18)
	binary.LittleEndian.PutUint32(root[4:], none)
	if len(names) > 0 {
		binary.LittleEndian.PutUint32(root[8:], dirOff[names[0]])
	} else {
		binary.LittleEndian.PutUint32(root[8:], none)
	}
	binary.LittleEndian.PutUint32(root[12:], none)
	binary.LittleEndian.PutUint32(root[16:], none)
	dm.Write(root)
	for i, d := range names {
		e := make([]byte, 0x18)
		sib := uint32(none)
		if i+1 < len(names) {
			sib = dirOff[names[i+1]]
		}
		binary.LittleEndian.PutUint32(e[4:], sib)
		binary.LittleEndian.PutUint32(e[8:], none)
		ff, ok := firstFile[d]
		if !ok {
			ff = none
		}
		binary.LittleEndian.PutUint32(e[12:], ff)
		binary.LittleEndian.PutUint32(e[16:], none)
		binary.LittleEndian.PutUint32(e[20:], uint32(len(d)))
		dm.Write(e)
		dm.WriteString(d)
		pad4(&dm)
	}
	hdr := make([]byte, 0x50)
	binary.LittleEndian.PutUint64(hdr[0:], 0x50)
	binary.LittleEndian.PutUint64(hdr[0x18:], 0x50)
	binary.LittleEndian.PutUint64(hdr[0x20:], uint64(dm.Len()))
	fmo := uint64(0x50 + dm.Len())
	binary.LittleEndian.PutUint64(hdr[0x38:], fmo)
	binary.LittleEndian.PutUint64(hdr[0x40:], uint64(fm.Len()))
	do := (fmo + uint64(fm.Len()) + 15) &^ 15
	binary.LittleEndian.PutUint64(hdr[0x48:], do)
	out := make([]byte, do)
	copy(out, hdr)
	copy(out[0x50:], dm.Bytes())
	copy(out[fmo:], fm.Bytes())
	return append(out, data.Bytes()...)
}

// writeFakeFirmware creates prod.keys and a split-layout NAND avatar archive
// (system key area, chara + bg folders of Yaz0 RGBA images) under root.
func writeFakeFirmware(t *testing.T, root string, chars map[string][]byte, bgs map[string][]byte) {
	user := filepath.Join(root, "System", "Engine") // NEXUS engine data (portable test layout)
	os.MkdirAll(filepath.Join(user, "keys"), 0o755)
	k := testKeys{header: bytes.Repeat([]byte{0x21}, 32), kak: bytes.Repeat([]byte{0x31}, 16), kek: bytes.Repeat([]byte{0x41}, 16)}
	os.WriteFile(filepath.Join(user, "keys", "prod.keys"), []byte("header_key = "+hexs(k.header)+"\nkey_area_key_system_00 = "+hexs(k.kak)+"\n"), 0o644)
	enc := func(m map[string][]byte) map[string][]byte {
		o := map[string][]byte{}
		for n, px := range m {
			o[n+".szs"] = yaz0Literal(px)
		}
		return o
	}
	tree := map[string]map[string][]byte{"chara": enc(chars)}
	if len(bgs) > 0 {
		tree["bg"] = enc(bgs)
	}
	tree["chara"]["readme.txt"] = []byte("not an image")
	nca := buildNCA(t, k, 4, avatarArchiveID, buildRomFSTree(tree), nil, nil)
	hdr := nca[:0xC00]
	nintendoXTS(k.header, hdr, 0, true)
	hdr[0x207] = 2 // key area: system
	nintendoXTS(k.header, hdr, 0, false)
	reg := filepath.Join(user, "nand", "system", "Contents", "registered")
	os.MkdirAll(filepath.Join(reg, "aaaa.nca"), 0o755)
	os.WriteFile(filepath.Join(reg, "aaaa.nca", "00"), nca, 0o644) // split layout
	os.WriteFile(filepath.Join(reg, "other.nca"), make([]byte, 0x1000), 0o644)
}

func TestFirmwareAvatarExtraction(t *testing.T) {
	root := t.TempDir()
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	if st := app.Avatars.Ensure(false); st.State != "no-firmware" {
		t.Fatalf("expected no-firmware, got %+v", st)
	}
	pix := make([]byte, 256*256*4)
	for i := range pix {
		pix[i] = byte(i % 251)
	}
	writeFakeFirmware(t, root, map[string][]byte{"chr_alpha_00": pix, "chr_alpha_01": pix, "chr_beta_00": pix}, map[string][]byte{"bg_00": pix})
	st := app.Avatars.Ensure(false)
	if st.State != "ready" || st.Count != 4 {
		t.Fatalf("provision: %+v", st)
	}
	f, _ := os.Open(filepath.Join(app.avatarDir(), "characters", "chr_alpha_00.png"))
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != 256 {
		t.Fatalf("png: %v", err)
	}
	cats := app.Avatars.Categories()
	if len(cats) != 3 || cats[0].ID != "firmware" || len(cats[0].Characters) != 3 || len(cats[0].Backgrounds) != 1 || cats[1].Name != "Alpha" {
		t.Fatalf("categories: %+v", cats)
	}
	// unchanged firmware -> no re-extraction; deleted cache -> re-extracted
	if st2 := app.Avatars.Ensure(false); st2.At != st.At {
		t.Fatal("should not re-extract unchanged firmware")
	}
	os.RemoveAll(app.avatarDir())
	if st3 := app.Avatars.Ensure(false); st3.State != "ready" || len(app.Avatars.List()) != 3 {
		t.Fatalf("cache not rebuilt: %+v", st3)
	}
}

func hexs(b []byte) string {
	const d = "0123456789abcdef"
	o := make([]byte, 0, len(b)*2)
	for _, c := range b {
		o = append(o, d[c>>4], d[c&15])
	}
	return string(o)
}
