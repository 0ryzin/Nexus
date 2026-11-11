package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func releaseZip(t *testing.T, version string, files map[string]string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	base := map[string]string{"NEXUS/NEXUS.exe": "MZ new nexus", "NEXUS/Engine/nexus_engine.dll": "engine", "NEXUS/Engine/engine.json": "{}",
		"NEXUS/RELEASE.json": `{"version":"` + version + `"}`, "NEXUS/Games/README.txt": "put games here", "NEXUS/Resources/x.txt": "new res"}
	for k, v := range files {
		base[k] = v
	}
	for k, v := range base {
		w, _ := zw.Create(k)
		w.Write([]byte(v))
	}
	zw.Close()
	return b.Bytes()
}

// TestUpdateCheckDownloadVerify: a newer release is found and downloaded; a download that does
// not match the published checksum is discarded; a private release page asks for a token.
func TestUpdateCheckDownloadVerify(t *testing.T) {
	app, _ := newTestApp(t, "")
	old := AppVersion
	AppVersion = "0.2.0"
	defer func() { AppVersion = old }()
	data := releaseZip(t, "0.3.0", nil)
	sum := sha256.Sum256(data)
	published := hex.EncodeToString(sum[:])
	private := false
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case private:
			http.Error(w, "Not Found", 404)
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":"v0.3.0","name":"NEXUS 0.3.0","body":"notes","html_url":"x","assets":[
			  {"name":"NEXUS-0.3.0-Windows-x64.zip","url":"%[1]s/asset/zip","size":%[2]d},
			  {"name":"NEXUS-0.3.0-Windows-x64.zip.sha256","url":"%[1]s/asset/sum","size":90}]}`, srv.URL, len(data))
		case r.URL.Path == "/asset/sum":
			fmt.Fprintf(w, "%s  NEXUS-0.3.0-Windows-x64.zip\n", published)
		case r.URL.Path == "/asset/zip":
			http.ServeContent(w, r, "z.zip", time.Now(), bytes.NewReader(data))
		}
	}))
	defer srv.Close()
	app.Updater.api = srv.URL
	info := app.Updater.Check()
	if !info.Available || info.Latest != "0.3.0" || info.Notes != "notes" || info.Size != int64(len(data)) {
		t.Fatalf("%+v", info)
	}
	// a damaged download is refused
	published = strings.Repeat("0", 64)
	app.Updater.Download()
	j := waitUpdate(t, app)
	if j.State != "error" || !strings.Contains(j.Error, "checksum") {
		t.Fatalf("bad checksum: %+v", j)
	}
	if fileExists(filepath.Join(app.Updater.stagingDir(), "0.3.0", "NEXUS-0.3.0-Windows-x64.zip")) {
		t.Fatal("unverified download kept")
	}
	published = hex.EncodeToString(sum[:])
	app.Updater.Download()
	if j := waitUpdate(t, app); j.State != "ready" {
		t.Fatalf("download: %+v", j)
	}
	if !fileExists(filepath.Join(app.Updater.stagingDir(), "0.3.0", "extracted", "NEXUS", "NEXUS.exe")) {
		t.Fatal("not extracted")
	}
	// a package for another version or without the engine is refused
	bad := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(bad, releaseZip(t, "0.2.9", nil), 0o644)
	if err := extractRelease(bad, t.TempDir(), "0.3.0"); err == nil {
		t.Fatal("wrong version accepted")
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create("NEXUS/../evil.exe")
	w.Write([]byte("x"))
	zw.Close()
	os.WriteFile(bad, b.Bytes(), 0o644)
	if err := extractRelease(bad, t.TempDir(), "0.3.0"); err == nil {
		t.Fatal("path escape accepted")
	}
	private = true
	if info := app.Updater.Check(); info.Available || !info.NeedsToken {
		t.Fatalf("private: %+v", info)
	}
}

func waitUpdate(t *testing.T, app *App) UpdateJob {
	for i := 0; i < 300; i++ {
		if j := app.Updater.Job(); j != nil && (j.State == "ready" || j.State == "error") {
			return *j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("download did not finish")
	return UpdateJob{}
}

// TestApplyUpdateKeepsDataAndRollsBack: program files are replaced, user data and games are not;
// a new version that does not start is rolled back to the old files.
func TestApplyUpdateKeepsDataAndRollsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts as stand-ins for NEXUS.exe")
	}
	for _, good := range []bool{true, false} {
		root, src := t.TempDir(), t.TempDir()
		write := func(p, s string, mode os.FileMode) {
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte(s), mode)
		}
		write(filepath.Join(root, "NEXUS.exe"), "#!/bin/sh\nexit 0\n", 0o755)
		write(filepath.Join(root, "Engine", "nexus_engine.dll"), "old engine", 0o644)
		write(filepath.Join(root, "Resources", "x.txt"), "old res", 0o644)
		write(filepath.Join(root, "Games", "My Game.nsp"), "my game", 0o644)
		write(filepath.Join(root, "System", "UserData", "Saves", "s.sav"), "save", 0o644)
		write(filepath.Join(root, "portable.txt"), "", 0o644)
		newExe := "#!/bin/sh\n[ \"$1\" = --update-applied ] && echo ok > \"$2\"\nexit 0\n"
		if !good {
			newExe = "#!/bin/sh\nexit 3\n"
		}
		write(filepath.Join(src, "NEXUS.exe"), newExe, 0o755)
		write(filepath.Join(src, "Engine", "nexus_engine.dll"), "new engine", 0o644)
		write(filepath.Join(src, "Resources", "x.txt"), "new res", 0o644)
		write(filepath.Join(src, "Games", "README.txt"), "readme", 0o644)
		write(filepath.Join(src, "Games", "My Game.nsp"), "MUST NOT REPLACE", 0o644)
		write(filepath.Join(src, "System", "evil.txt"), "must not appear", 0o644)
		code := applyUpdate([]string{src, root, "999999", "0.2.0"})
		if readFile(t, filepath.Join(root, "Games", "My Game.nsp")) != "my game" || readFile(t, filepath.Join(root, "System", "UserData", "Saves", "s.sav")) != "save" ||
			fileExists(filepath.Join(root, "System", "evil.txt")) || !fileExists(filepath.Join(root, "portable.txt")) {
			t.Fatalf("good=%v: user data touched", good)
		}
		if good {
			if code != 0 || readFile(t, filepath.Join(root, "Engine", "nexus_engine.dll")) != "new engine" || readFile(t, filepath.Join(root, "Resources", "x.txt")) != "new res" ||
				readFile(t, filepath.Join(root, ".nexus-previous", "0.2.0", "Engine", "nexus_engine.dll")) != "old engine" || !fileExists(filepath.Join(root, "Games", "README.txt")) {
				t.Fatalf("update not applied (code %d)", code)
			}
		} else {
			if code == 0 || readFile(t, filepath.Join(root, "Engine", "nexus_engine.dll")) != "old engine" || readFile(t, filepath.Join(root, "Resources", "x.txt")) != "old res" ||
				!strings.Contains(readFile(t, filepath.Join(root, "NEXUS.exe")), "exit 0") {
				t.Fatalf("failed update not rolled back (code %d)", code)
			}
		}
	}
}
