package main

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipBytes(t *testing.T, files map[string]string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, c := range files {
		w, _ := z.Create(n)
		w.Write([]byte(c))
	}
	z.Close()
	return b.Bytes()
}

func TestGameBananaInstall(t *testing.T) {
	good := zipBytes(t, map[string]string{"Better Text/romfs/text.bin": "hello"})
	evil := zipBytes(t, map[string]string{"../../escape/romfs/x.bin": "x"})
	sum := func(b []byte) string { s := md5.Sum(b); return hex.EncodeToString(s[:]) }
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := srv.URL
		switch {
		case strings.HasPrefix(r.URL.Path, "/apiv11/Util/Game/NameMatch"):
			json.NewEncoder(w).Encode(map[string]any{"_aRecords": []map[string]any{{"_idRow": 9, "_sName": "Something Else"}, {"_idRow": 7, "_sName": "Alpha Quest"}}})
		case strings.HasPrefix(r.URL.Path, "/apiv11/Game/7/Subfeed"):
			json.NewEncoder(w).Encode(map[string]any{"_aMetadata": map[string]any{"_bIsComplete": true}, "_aRecords": []map[string]any{
				{"_idRow": 100, "_sName": "Better Text", "_sModelName": "Mod", "_aSubmitter": map[string]any{"_sName": "modder"}},
				{"_idRow": 5, "_sName": "A question", "_sModelName": "Question"}}})
		case r.URL.Path == "/apiv11/Mod/100/ProfilePage":
			json.NewEncoder(w).Encode(map[string]any{"_idRow": 100, "_sName": "Better Text", "_sText": "<b>Nice</b> mod<br>line 2", "_aFiles": []map[string]any{
				{"_idRow": 1, "_sFile": "better.zip", "_nFilesize": len(good), "_sMd5Checksum": sum(good), "_sDownloadUrl": base + "/dl/1"},
				{"_idRow": 2, "_sFile": "bad-md5.zip", "_nFilesize": len(good), "_sMd5Checksum": strings.Repeat("0", 32), "_sDownloadUrl": base + "/dl/1"},
				{"_idRow": 3, "_sFile": "mod.7z", "_nFilesize": 10, "_sDownloadUrl": base + "/dl/3"},
				{"_idRow": 4, "_sFile": "evil.zip", "_nFilesize": len(evil), "_sDownloadUrl": base + "/dl/4"},
				{"_idRow": 5, "_sFile": "elsewhere.zip", "_nFilesize": 10, "_sDownloadUrl": base + "/dl/5"},
				{"_idRow": 6, "_sFile": "http.zip", "_nFilesize": 10, "_sDownloadUrl": "http://gamebanana.com/dl/6"},
			}})
		case r.URL.Path == "/dl/1":
			w.Write(good)
		case r.URL.Path == "/dl/4":
			w.Write(evil)
		case r.URL.Path == "/dl/5":
			http.Redirect(w, r, "https://evil.example.org/x.zip", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	oldAPI, oldHost := gbAPI, gbTestHost
	gbAPI, gbTestHost = srv.URL+"/apiv11", u.Host
	defer func() { gbAPI, gbTestHost = oldAPI, oldHost }()

	a, _ := newTestApp(t, "")
	a.Library.Scan()
	g, _ := a.Library.Get("t-0100aaaa00010000")
	a.GameBanana.client.Transport = srv.Client().Transport

	res, err := a.GameBanana.Search(g.Title, "", 1)
	if err != nil || res.GameID != 7 || len(res.Mods) != 1 || res.Mods[0].Name != "Better Text" {
		t.Fatalf("search %+v %v", res, err)
	}
	page, err := a.GameBanana.Mod(100)
	if err != nil || len(page.Files) != 6 || page.Description != "Nice mod\nline 2" || page.Files[2].Zip {
		t.Fatalf("mod page %+v %v", page, err)
	}
	for _, c := range []struct {
		file int64
		want string
	}{{2, "checksum"}, {3, "not a ZIP"}, {4, "not safe"}, {5, "not GameBanana"}, {6, "not on GameBanana"}} {
		if _, err := a.GameBanana.Install(g.Key, 100, c.file); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("file %d: %v (want %q)", c.file, err, c.want)
		}
	}
	if _, err := os.Stat(filepath.Join(a.Paths.Data, "escape")); err == nil {
		t.Fatal("a path outside the mod was written")
	}
	added, err := a.GameBanana.Install(g.Key, 100, 1)
	if err != nil || len(added) != 1 {
		t.Fatalf("install: %v %v", added, err)
	}
	dir := filepath.Join(a.Content.modsDir(g.TitleID), added[0])
	if b, _ := os.ReadFile(filepath.Join(dir, "romfs", "text.bin")); string(b) != "hello" {
		t.Fatalf("installed files in %s", dir)
	}
	var src map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, ".nexus-source.json"))
	if json.Unmarshal(b, &src) != nil || fmt.Sprint(src["modId"]) != "100" {
		t.Fatalf("source record %s", b)
	}
	if _, err := a.GameBanana.Install(g.Key, 100, 1); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("installed twice: %v", err)
	}
}
