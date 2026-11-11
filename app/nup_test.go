package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildNup writes a minimal NUP package (no game data) with the given packages' metadata.
func buildNup(t *testing.T, pkgs []NupPackage) []byte {
	t.Helper()
	le := binary.LittleEndian
	tlv := func(typ uint16, v []byte) []byte {
		b := make([]byte, 8, 8+len(v))
		le.PutUint16(b, typ)
		le.PutUint32(b[4:], uint32(len(v)))
		return append(b, v...)
	}
	var metas [][]byte
	for _, p := range pkgs {
		var m []byte
		if p.Name != "" {
			m = append(m, tlv(nupMetaTitle, []byte(p.Name))...)
		}
		if p.Publisher != "" {
			m = append(m, tlv(nupMetaPub, []byte(p.Publisher))...)
		}
		if p.DisplayVersion != "" {
			m = append(m, tlv(nupMetaVersion, []byte(p.DisplayVersion))...)
		}
		if p.Icon != nil {
			m = append(m, tlv(nupMetaIcon, p.Icon)...)
		}
		for lang, ic := range p.LocalIcons {
			rec := append([]byte{byte(len(lang))}, lang...)
			n := make([]byte, 4)
			le.PutUint32(n, uint32(len(ic)))
			rec = append(append(rec, n...), ic...)
			m = append(m, tlv(nupMetaLIcons, rec)...)
		}
		metas = append(metas, m)
	}
	toc := make([]byte, nupTocHdrSize+len(pkgs)*nupPackageSize+0x18) // + one dir record
	le.PutUint32(toc[0:], 0x01434F54)
	le.PutUint32(toc[4:], uint32(len(pkgs)))
	le.PutUint64(toc[8:], 1)
	le.PutUint64(toc[24:], uint64(len(toc)))
	tocOff := int64(nupSuperSize)
	blobOff := (tocOff + int64(len(toc)) + 15) &^ 15
	metaOff := blobOff
	for i, p := range pkgs {
		r := toc[nupTocHdrSize+i*nupPackageSize:]
		le.PutUint64(r[0:], p.TitleID)
		le.PutUint32(r[0x14:], p.Version)
		r[0x18] = map[string]byte{"base": 0, "update": 1, "dlc": 2}[p.Kind]
		le.PutUint64(r[0x20:], uint64(metaOff))
		le.PutUint32(r[0x28:], uint32(len(metas[i])))
		metaOff += int64(len(metas[i]))
	}
	sb := make([]byte, nupSuperSize)
	copy(sb, "NUP\x01")
	le.PutUint16(sb[4:], nupSuperSize)
	le.PutUint64(sb[8:], uint64(tocOff))
	le.PutUint64(sb[16:], uint64(len(toc)))
	sum := sha256.Sum256(toc)
	copy(sb[24:], sum[:])
	le.PutUint64(sb[56:], uint64(blobOff))
	le.PutUint32(sb[72:], uint32(len(pkgs)))
	var buf bytes.Buffer
	buf.Write(sb)
	buf.Write(toc)
	buf.Write(make([]byte, blobOff-int64(buf.Len())))
	for _, m := range metas {
		buf.Write(m)
	}
	out := buf.Bytes()
	le.PutUint64(out[64:], uint64(len(out)))
	return out
}

var fakeJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{1}, 40)...)

// TestNupIdentifiesNameAndIcon: a Switch 2 game in a NUP package gets its title, version and its
// own icon from the package metadata (this is how the Ocarina of Time remake's icon is found).
func TestNupIdentifiesNameAndIcon(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{2}, 30)...)
	data := buildNup(t, []NupPackage{
		{TitleID: 0x0400486024DEC000, Kind: "base", Name: "The Legend of Zelda: Ocarina of Time", Publisher: "Nintendo", DisplayVersion: "1.0.0",
			LocalIcons: map[string][]byte{"AmericanEnglish": png}},
		{TitleID: 0x0400486024DEC800, Kind: "update", Version: 65536, DisplayVersion: "1.0.1"},
	})
	p := filepath.Join(t.TempDir(), "Ocarina.nup")
	os.WriteFile(p, data, 0o644)
	info, err := IdentifyFile(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.TitleID != "0400486024DEC000" || info.Kind != "base" || info.Name != "The Legend of Zelda: Ocarina of Time" || info.Version != "1.0.1" || info.Source != "nup" {
		t.Fatalf("%+v", info)
	}
	if !bytes.Equal(info.Icon, png) {
		t.Fatal("icon not taken from the package")
	}
	// damaged table of contents: rejected, not misread
	data[nupSuperSize+40] ^= 0xFF
	os.WriteFile(p, data, 0o644)
	if _, err := IdentifyFile(p, nil); err == nil {
		t.Fatal("damaged NUP accepted")
	}
}

// TestLibraryShowsNupIcon: the library scan stores the NUP icon as the game's embedded artwork.
func TestLibraryShowsNupIcon(t *testing.T) {
	app, root := newTestApp(t, "")
	data := buildNup(t, []NupPackage{{TitleID: 0x0400486024DEC000, Kind: "base", Name: "Ocarina", Icon: fakeJPEG}})
	os.WriteFile(filepath.Join(root, "Games", "Ocarina.nup"), data, 0o644)
	if err := app.Library.Scan(); err != nil {
		t.Fatal(err)
	}
	g, ok := app.Library.Get("t-0400486024dec000")
	if !ok {
		var keys []string
		for _, g := range app.Library.Games() {
			keys = append(keys, g.Key)
		}
		t.Fatalf("game not found: %v", keys)
	}
	if g.Title != "Ocarina" || !g.HasEmbed || g.ArtSource == "" {
		t.Fatalf("%+v", g)
	}
}
