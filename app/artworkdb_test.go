package main

import (
	"bytes"
	"encoding/json"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func installArtworkDB(t *testing.T, root string) {
	dst := filepath.Join(root, "Resources", "ArtworkDB")
	os.MkdirAll(dst, 0o755)
	for _, f := range []string{"index.json", "zelda-ocarina-of-time-remake.png"} {
		data, err := os.ReadFile(filepath.Join("resources", "ArtworkDB", f))
		if os.IsNotExist(err) && strings.HasSuffix(f, ".png") {
			// the supplied artwork is kept out of the public source; the matching is still tested
			t.Skip("the built-in artwork image is not in this source tree (personal builds only)")
		}
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dst, f), data, 0o644)
	}
}

func nacpWith(name, publisher string) []byte {
	n := makeNACP(name, "1.0.0")
	copy(n[0x200:0x300], make([]byte, 0x100))
	copy(n[0x200:], publisher)
	return n
}

func TestOcarinaRemakeArtwork(t *testing.T) {
	app, root := newTestApp(t, "")
	installArtworkDB(t, root)
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	games := filepath.Join(root, "Games")
	type f = struct {
		name string
		data []byte
	}
	nsp := func(file, name, pub string, id uint64) {
		rom := buildRomFS(map[string][]byte{"control.nacp": nacpWith(name, pub), "icon_AmericanEnglish.dat": jpegBytes(256, color.RGBA{1, 2, 3, 255})})
		os.WriteFile(filepath.Join(games, file), buildPFS0([]f{{"p.nca", buildNCA(t, k, 0, id, nil, nil, nil)}, {"c.nca", buildNCA(t, k, 2, id, rom, nil, nil)}}), 0o644)
	}
	// should match
	nsp("oot-remake.nsp", "The Legend of Zelda: Ocarina of Time", "Nintendo", 0x0400486024DEC000)
	os.WriteFile(filepath.Join(games, "The Legend of Zelda Ocarina of Time Remake [v0].xci"), []byte("unreadable"), 0o644)
	// should NOT match
	nsp("oot-fanport.nsp", "The Legend of Zelda: Ocarina of Time", "Harbour Masters", 0x0100BBBB00010000)
	nsp("oot3d.nsp", "The Legend of Zelda: Ocarina of Time 3D", "Nintendo", 0x0100BBBB00020000)
	nsp("mm.nsp", "The Legend of Zelda: Majora's Mask 3D", "Nintendo", 0x0100BBBB00030000)
	nsp("botw-misnamed Ocarina of Time Remake.nsp", "The Legend of Zelda: Breath of the Wild", "Nintendo", 0x0100BBBB00040000)
	nsp("mq.nsp", "The Legend of Zelda: Ocarina of Time Master Quest", "Nintendo", 0x0100BBBB00050000)
	os.WriteFile(filepath.Join(games, "Ocarina of Time.xci"), []byte("unreadable, no remake keyword"), 0o644)
	os.WriteFile(filepath.Join(games, "Ocarina of Time 3D Remake.xci"), []byte("unreadable"), 0o644)
	if err := app.Library.Scan(); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"oot-remake.nsp": true, "The Legend of Zelda Ocarina of Time Remake [v0].xci": true}
	n := 0
	for _, g := range app.Library.Games() {
		base := filepath.Base(g.Path)
		got := g.ArtSource == "builtin"
		if strings.HasPrefix(base, "a.") || strings.HasPrefix(base, "b.") || strings.HasPrefix(base, "Charlie") {
			continue
		}
		n++
		if got != want[base] {
			t.Errorf("%s (%q, publisher %q): builtin=%v, want %v", base, g.Title, g.Publisher, got, want[base])
		}
	}
	if n != 9 {
		t.Fatalf("expected 9 test games, saw %d", n)
	}
	// the served image is exactly the supplied file
	g, _ := app.Library.Get("t-0400486024dec000")
	if g.BuiltinArt != "zelda-oot-remake-switch2" {
		t.Fatalf("builtin id %q", g.BuiltinArt)
	}
	served, _ := os.ReadFile(app.Library.ArtPath(g.Key, ""))
	orig, _ := os.ReadFile(filepath.Join("resources", "ArtworkDB", "zelda-ocarina-of-time-remake.png"))
	if !bytes.Equal(served, orig) {
		t.Fatal("built-in artwork is not the supplied image")
	}
	// artwork the user picked always wins
	app.Library.SetCustomArt(g.Key, jpegBytes(300, color.RGBA{9, 9, 9, 255}), ".jpg")
	g, _ = app.Library.Get(g.Key)
	if g.ArtSource != "custom" {
		t.Fatalf("custom art replaced: %s", g.ArtSource)
	}
}

func TestArtworkDatabaseOffline(t *testing.T) {
	var hits int32
	inner := fakeNlib(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if strings.HasSuffix(r.URL.Path, "0100AAAA00020000") {
			json.NewEncoder(w).Encode(map[string]any{"id": "0100AAAA00020000", "name": "Bravo Island Deluxe", "icon": "x", "publisher": "Bravo Soft", "releaseDate": 20240101})
			return
		}
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	defer inner.Close()
	app, root := newTestApp(t, srv.URL)
	app.Library.Scan()
	waitOnline(app)
	g, _ := app.Library.Get("t-0100aaaa00020000")
	if g.ArtSource != "online" {
		t.Fatalf("no online art: %s %s", g.ArtSource, g.OnlineErr)
	}
	p := app.Library.GP.Get(g.Key)
	if p.Online.Publisher != "Bravo Soft" || p.Online.Released != "20240101" {
		t.Fatalf("metadata %+v", p.Online)
	}
	rec, _, ok := app.Library.artCache().Get("0100AAAA00020000")
	if !ok || rec.Name != "Bravo Island Deluxe" {
		t.Fatal("not stored in the artwork database")
	}
	before := atomic.LoadInt32(&hits)
	// the game's own folder is lost (or the file moved): artwork comes back without the network
	srv.Close()
	os.RemoveAll(app.Library.GP.Dir(g.Key))
	app2, _ := NewApp(root)
	app2.Settings.Update(func(s *GlobalSettings) { s.Library.NlibURL = "http://127.0.0.1:1" })
	app2.Library.Scan()
	waitOnline(app2)
	g2, _ := app2.Library.Get(g.Key)
	if g2.ArtSource != "online" || atomic.LoadInt32(&hits) != before {
		t.Fatalf("offline restore failed: %s err=%s hits %d->%d", g2.ArtSource, g2.OnlineErr, before, hits)
	}
	if !strings.Contains(app2.Library.GP.Get(g.Key).Online.Source, "local database") {
		t.Fatal("source should say local database")
	}
}
