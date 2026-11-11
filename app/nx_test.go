package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func aesECBEncrypt(key, data []byte) []byte {
	c, _ := aes.NewCipher(key)
	out := make([]byte, len(data))
	for i := 0; i < len(data); i += 16 {
		c.Encrypt(out[i:i+16], data[i:i+16])
	}
	return out
}

func buildRomFS(files map[string][]byte) []byte {
	// minimal RomFS: header, (empty dir tables), file meta table, file data
	var meta, data bytes.Buffer
	names := []string{"control.nacp", "icon_AmericanEnglish.dat"}
	for i, n := range names {
		d := files[n]
		e := make([]byte, 0x20)
		binary.LittleEndian.PutUint32(e[0:], 0)          // parent = root
		binary.LittleEndian.PutUint32(e[4:], 0xFFFFFFFF) // sibling
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
		_ = i
	}
	hdr := make([]byte, 0x50)
	binary.LittleEndian.PutUint64(hdr[0:], 0x50)
	metaOff := uint64(0x50)
	binary.LittleEndian.PutUint64(hdr[0x38:], metaOff)
	binary.LittleEndian.PutUint64(hdr[0x40:], uint64(meta.Len()))
	dataOff := (metaOff + uint64(meta.Len()) + 0xF) &^ 0xF
	binary.LittleEndian.PutUint64(hdr[0x48:], dataOff)
	out := make([]byte, dataOff)
	copy(out, hdr)
	copy(out[metaOff:], meta.Bytes())
	return append(out, data.Bytes()...)
}

type testKeys struct {
	header, kak, kek []byte
}

func buildNCA(t *testing.T, k testKeys, contentType byte, programID uint64, romfs []byte, rights []byte, titleKey []byte) []byte {
	const sectionStart = 0xC00
	const levelOff = 0x400
	secKey := bytes.Repeat([]byte{0x5A}, 16)
	if rights != nil {
		secKey = titleKey
	}
	section := make([]byte, levelOff)
	section = append(section, romfs...)
	for len(section)%0x200 != 0 {
		section = append(section, 0)
	}
	ctrHi := []byte{1, 2, 3, 4, 5, 6, 7, 8} // as used in the counter (big-endian)
	if romfs != nil {
		block, _ := aes.NewCipher(secKey)
		iv := make([]byte, 16)
		copy(iv, ctrHi)
		binary.BigEndian.PutUint64(iv[8:], uint64(sectionStart>>4))
		cipher.NewCTR(block, iv).XORKeyStream(section, section)
	}
	h := make([]byte, 0xC00)
	copy(h[0x200:], "NCA3")
	h[0x205] = contentType
	binary.LittleEndian.PutUint64(h[0x210:], programID)
	if rights != nil {
		copy(h[0x230:], rights)
	}
	binary.LittleEndian.PutUint32(h[0x240:], sectionStart/0x200)
	binary.LittleEndian.PutUint32(h[0x244:], uint32((sectionStart+len(section))/0x200))
	keyArea := make([]byte, 0x40)
	copy(keyArea[0x20:], secKey)
	copy(h[0x300:], aesECBEncrypt(k.kak, keyArea))
	fs := h[0x400:0x600]
	fs[0x2], fs[0x3], fs[0x4] = 0, 3, 3
	copy(fs[0x8:], "IVFC")
	binary.LittleEndian.PutUint64(fs[0x90:], levelOff)
	for j := 0; j < 8; j++ {
		fs[0x140+7-j] = ctrHi[j]
	}
	if err := nintendoXTS(k.header, h, 0, false); err != nil {
		t.Fatal(err)
	}
	return append(h, section...)
}

func buildPFS0(files []struct {
	name string
	data []byte
}) []byte {
	var strs bytes.Buffer
	var entries bytes.Buffer
	var data bytes.Buffer
	for _, f := range files {
		e := make([]byte, 0x18)
		binary.LittleEndian.PutUint64(e[0:], uint64(data.Len()))
		binary.LittleEndian.PutUint64(e[8:], uint64(len(f.data)))
		binary.LittleEndian.PutUint32(e[16:], uint32(strs.Len()))
		entries.Write(e)
		strs.WriteString(f.name)
		strs.WriteByte(0)
		data.Write(f.data)
	}
	for strs.Len()%0x20 != 0 {
		strs.WriteByte(0)
	}
	h := make([]byte, 0x10)
	copy(h, "PFS0")
	binary.LittleEndian.PutUint32(h[4:], uint32(len(files)))
	binary.LittleEndian.PutUint32(h[8:], uint32(strs.Len()))
	out := append(h, entries.Bytes()...)
	out = append(out, strs.Bytes()...)
	return append(out, data.Bytes()...)
}

func writeKeys(t *testing.T, dir string, k testKeys) string {
	p := filepath.Join(dir, "prod.keys")
	content := fmt.Sprintf("header_key = %s\nkey_area_key_application_00 = %s\ntitlekek_00 = %s\n",
		hex.EncodeToString(k.header), hex.EncodeToString(k.kak), hex.EncodeToString(k.kek))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func makeNACP(name, version string) []byte {
	n := make([]byte, 0x4000)
	copy(n[0:], name)
	copy(n[0x200:], "Test Publisher")
	copy(n[0x3060:], version)
	return n
}

func TestIdentifyNSPWithKeys(t *testing.T) {
	dir := t.TempDir()
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	keysPath := writeKeys(t, dir, k)
	icon := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{7}, 500)...)
	romfs := buildRomFS(map[string][]byte{"control.nacp": makeNACP("Super Test Adventure", "1.2.0"), "icon_AmericanEnglish.dat": icon})
	const id = 0x0100ABCD12340000
	program := buildNCA(t, k, 0, id, nil, nil, nil)
	control := buildNCA(t, k, 2, id, romfs, nil, nil)
	nsp := buildPFS0([]struct {
		name string
		data []byte
	}{{"aaaa.nca", program}, {"bbbb.nca", control}})
	path := filepath.Join(dir, "Whatever [v0].nsp")
	os.WriteFile(path, nsp, 0o644)

	info, err := IdentifyFile(path, LoadKeys(keysPath))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if info.TitleID != "0100ABCD12340000" || info.Kind != "base" || info.Source != "nca" {
		t.Fatalf("bad id: %+v", info)
	}
	if info.Name != "Super Test Adventure" || info.Version != "1.2.0" {
		t.Fatalf("bad nacp: %q %q", info.Name, info.Version)
	}
	if !bytes.Equal(info.Icon, icon) {
		t.Fatalf("icon mismatch (%d bytes)", len(info.Icon))
	}
}

func TestIdentifyTitleKeyCrypto(t *testing.T) {
	dir := t.TempDir()
	k := testKeys{header: bytes.Repeat([]byte{0x99}, 32), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	keysPath := writeKeys(t, dir, k)
	titleKey := bytes.Repeat([]byte{0x77}, 16)
	rights, _ := hex.DecodeString("0100ABCD123408000000000000000000")
	romfs := buildRomFS(map[string][]byte{"control.nacp": makeNACP("Patch Name", "2.0"), "icon_AmericanEnglish.dat": []byte("JPEGDATA")})
	control := buildNCA(t, k, 2, 0x0100ABCD12340800, romfs, rights, titleKey)
	tik := make([]byte, 0x2C0)
	copy(tik[0x180:], aesECBEncrypt(k.kek, titleKey))
	copy(tik[0x2A0:], rights)
	nsp := buildPFS0([]struct {
		name string
		data []byte
	}{{"0100abcd123408000000000000000000.tik", tik}, {"cc.nca", control}})
	path := filepath.Join(dir, "upd.nsp")
	os.WriteFile(path, nsp, 0o644)
	info, err := IdentifyFile(path, LoadKeys(keysPath))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if info.Kind != "update" || info.TitleID != "0100ABCD12340000" || info.FullID != "0100ABCD12340800" {
		t.Fatalf("bad: %+v", info)
	}
	if info.Name != "Patch Name" {
		t.Fatalf("title key decrypt failed: %q", info.Name)
	}
}

func TestIdentifyWithoutKeysUsesTicketAndFilename(t *testing.T) {
	dir := t.TempDir()
	tik := make([]byte, 0x2C0)
	rights, _ := hex.DecodeString("01007EF00011E0000000000000000000")
	copy(tik[0x2A0:], rights)
	nsp := buildPFS0([]struct {
		name string
		data []byte
	}{{"x.tik", tik}, {"y.nca", make([]byte, 0x1000)}})
	p := filepath.Join(dir, "My Game.nsp")
	os.WriteFile(p, nsp, 0o644)
	info, _ := IdentifyFile(p, KeySet{})
	if info.TitleID != "01007EF00011E000" || info.Source != "ticket" {
		t.Fatalf("ticket id: %+v", info)
	}
	p2 := filepath.Join(dir, "Cool Game [0100AAAABBBB0000][v65536] (USA).xci")
	os.WriteFile(p2, []byte("junk"), 0o644)
	info2, _ := IdentifyFile(p2, KeySet{})
	if info2.TitleID != "0100AAAABBBB0000" || info2.Version != "v65536" {
		t.Fatalf("filename id: %+v", info2)
	}
	if CleanTitle(p2) != "Cool Game" {
		t.Fatalf("clean title: %q", CleanTitle(p2))
	}
}

func TestIdentifyXCI(t *testing.T) {
	dir := t.TempDir()
	k := testKeys{header: bytes.Repeat([]byte{0x12}, 32), kak: bytes.Repeat([]byte{0x34}, 16), kek: bytes.Repeat([]byte{0x56}, 16)}
	keysPath := writeKeys(t, dir, k)
	romfs := buildRomFS(map[string][]byte{"control.nacp": makeNACP("Cart Game", "1.0"), "icon_AmericanEnglish.dat": []byte("ICON")})
	control := buildNCA(t, k, 2, 0x0100111122220000, romfs, nil, nil)
	// HFS0 with one entry (reuse PFS0 builder layout but HFS0 entries are 0x40)
	hfs := func(files []struct {
		name string
		data []byte
	}) []byte {
		var strs, entries, data bytes.Buffer
		for _, f := range files {
			e := make([]byte, 0x40)
			binary.LittleEndian.PutUint64(e[0:], uint64(data.Len()))
			binary.LittleEndian.PutUint64(e[8:], uint64(len(f.data)))
			binary.LittleEndian.PutUint32(e[16:], uint32(strs.Len()))
			entries.Write(e)
			strs.WriteString(f.name)
			strs.WriteByte(0)
			data.Write(f.data)
		}
		h := make([]byte, 0x10)
		copy(h, "HFS0")
		binary.LittleEndian.PutUint32(h[4:], uint32(len(files)))
		binary.LittleEndian.PutUint32(h[8:], uint32(strs.Len()))
		out := append(h, entries.Bytes()...)
		out = append(out, strs.Bytes()...)
		return append(out, data.Bytes()...)
	}
	secure := hfs([]struct {
		name string
		data []byte
	}{{"abc.nca", control}})
	root := hfs([]struct {
		name string
		data []byte
	}{{"update", hfs([]struct {
		name string
		data []byte
	}{{"u.nca", make([]byte, 16)}})}, {"secure", secure}})
	xci := make([]byte, 0xF000)
	copy(xci[0x100:], "HEAD")
	binary.LittleEndian.PutUint64(xci[0x130:], 0xF000)
	xci = append(xci, root...)
	p := filepath.Join(dir, "cart.xci")
	os.WriteFile(p, xci, 0o644)
	info, err := IdentifyFile(p, LoadKeys(keysPath))
	if err != nil {
		t.Fatal(err)
	}
	if info.TitleID != "0100111122220000" || info.Name != "Cart Game" {
		t.Fatalf("xci: %+v", info)
	}
}

func TestNRO(t *testing.T) {
	dir := t.TempDir()
	nro := make([]byte, 0x100)
	copy(nro[0x10:], "NRO0")
	binary.LittleEndian.PutUint32(nro[0x18:], 0x100)
	icon := []byte{0xFF, 0xD8, 1, 2, 3}
	nacp := makeNACP("Homebrew Menu", "2.4")
	aset := make([]byte, 0x38)
	copy(aset, "ASET")
	binary.LittleEndian.PutUint64(aset[8:], 0x38)
	binary.LittleEndian.PutUint64(aset[16:], uint64(len(icon)))
	binary.LittleEndian.PutUint64(aset[24:], uint64(0x38+len(icon)))
	binary.LittleEndian.PutUint64(aset[32:], uint64(len(nacp)))
	nro = append(nro, aset...)
	nro = append(nro, icon...)
	nro = append(nro, nacp...)
	p := filepath.Join(dir, "hbmenu.nro")
	os.WriteFile(p, nro, 0o644)
	info, err := IdentifyFile(p, nil)
	if err != nil || info.Name != "Homebrew Menu" || !bytes.Equal(info.Icon, icon) || info.Kind != "homebrew" {
		t.Fatalf("nro: %v %+v", err, info)
	}
}

func TestIniEditing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "qt-config.ini")
	orig := "[Renderer]\r\nbackend\\default=true\r\nbackend=1\r\n\r\n[Data%20Storage]\r\nnand_directory=C:/x/nand/\r\n\r\n[UI]\r\nfoo=bar\r\n"
	os.WriteFile(p, []byte(orig), 0o644)
	ini, _ := LoadIni(p)
	ini.Set("Data%20Storage", "save_directory", iniPath(`C:\NEXUS\System\Saves\abc`))
	ini.SetSetting("Renderer", "resolution_setup", "6")
	ini.SetSetting("Core", "use_multi_core", "true")
	ini.Save(p)
	b, _ := os.ReadFile(p)
	s := string(b)
	for _, want := range []string{"save_directory=C:/NEXUS/System/Saves/abc\r\n", "resolution_setup\\default=false\r\nresolution_setup=6", "[Core]\r\nuse_multi_core\\default=false\r\nuse_multi_core=true", "[UI]\r\nfoo=bar"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	ini2, _ := LoadIni(p)
	if v, ok := ini2.GetSetting("Renderer", "resolution_setup"); !ok || v != "6" {
		t.Fatal("GetSetting failed")
	}
	if _, ok := ini2.GetSetting("Renderer", "backend"); ok {
		t.Fatal("default value should report not-set")
	}
	if iniPath(`D:\Games, Mine`) != `"D:/Games, Mine"` {
		t.Fatalf("quoting: %s", iniPath(`D:\Games, Mine`))
	}
}

func TestXTSRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{1, 2, 3, 4}, 8)
	data := bytes.Repeat([]byte("0123456789abcdef"), 64)
	orig := append([]byte(nil), data...)
	nintendoXTS(key, data, 3, false)
	if bytes.Equal(data, orig) {
		t.Fatal("not encrypted")
	}
	nintendoXTS(key, data, 3, true)
	if !bytes.Equal(data, orig) {
		t.Fatal("round trip failed")
	}
}

func TestXTSKnownVector(t *testing.T) {
	// IEEE P1619 XTS-AES-128 vector 1 (sector 0 is identical for either tweak endianness)
	data := make([]byte, 0x200)
	nintendoXTS(make([]byte, 32), data, 0, false)
	want, _ := hex.DecodeString("917cf69ebd68b2ec9b9fe9a3eadda692cd43d2f59598ed858c02c2652fbf922e")
	if !bytes.Equal(data[:32], want) {
		t.Fatalf("xts mismatch: %x", data[:32])
	}
}
