package main

import (
	"archive/zip"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipOf(t *testing.T, files map[string][]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "shaders.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for n, b := range files {
		w, _ := zw.Create(n)
		w.Write(b)
	}
	zw.Close()
	f.Close()
	return p
}

func legacyCache(ver uint32) []byte {
	b := []byte(legacyCacheMagic)
	v := make([]byte, 4)
	binary.LittleEndian.PutUint32(v, ver)
	return append(append(b, v...), []byte("records...")...)
}

// TestImportShaderZip: v2 logs are merged (in nested folders too), engine-compatible legacy caches
// are installed for the engine's own migration only where the game has no log, and every other
// format is reported, not installed; nothing for another game is used.
func TestImportShaderZip(t *testing.T) {
	app, _ := newTestApp(t, "")
	g := shaderGame(t, app)
	v2 := append(v2Header(1), append(v2rec(v2KindProgram, 1, "prog"), v2rec(v2KindGraphics, 2, "pipe")...)...)
	drv := append([]byte(driverCacheMagic), 1, 0, 0, 0, 9, 9, 9, 9)
	files := map[string][]byte{
		"shaders/" + strings.ToLower(g.TitleID) + "/vulkan.fork-v54.bin": legacyCache(54),
		"shaders/" + g.TitleID + "/vulkan.bin":                           legacyCache(18),
		"pc/vulkan_pipelines.bin":                                        drv,
		"ryujinx/" + g.TitleID + "/cache/shader/guest.toc":               []byte("TOC"),
		"x/lichii-cache.v2.bin.lock":                                     []byte{},
		"0100BBBB00010000/lichii-cache.v2.bin":                           v2,
		"readme.txt":                                                     []byte("hello world, not a cache"),
	}
	// 1. no log yet: the legacy v54 cache is installed for the engine to convert
	res, err := app.Emu.ImportShaderZip(zipOf(t, files), g)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ShaderZipFile{}
	for _, f := range res.Files {
		by[f.Name] = f
	}
	dir := filepath.Dir(app.Emu.ShaderLogPath(g.TitleID))
	if by["shaders/"+strings.ToLower(g.TitleID)+"/vulkan.fork-v54.bin"].Action != "installed" || !fileExists(filepath.Join(dir, "vulkan.fork-v54.bin")) {
		t.Fatalf("v54 not installed: %+v", res.Files)
	}
	for _, n := range []string{"shaders/" + g.TitleID + "/vulkan.bin", "pc/vulkan_pipelines.bin", "ryujinx/" + g.TitleID + "/cache/shader/guest.toc", "x/lichii-cache.v2.bin.lock", "0100BBBB00010000/lichii-cache.v2.bin", "readme.txt"} {
		if f := by[n]; f.Action != "skipped" || f.Reason == "" {
			t.Fatalf("%s: %+v", n, f)
		}
	}
	if fileExists(app.Emu.ShaderLogPath(g.TitleID)) {
		t.Fatal("another game's log was used")
	}
	// 2. a v2 log for this game in a nested folder: its records are added; the legacy cache now
	//    cannot be converted (the game has a log)
	os.Remove(filepath.Join(dir, "vulkan.fork-v54.bin"))
	res, err = app.Emu.ImportShaderZip(zipOf(t, map[string][]byte{"deep/a/b/" + g.TitleID + "/lichii-cache.v2.bin": v2, "vulkan.fork-v53.bin": legacyCache(53)}), g)
	if err != nil || res.Added != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if live := liveOf(t, app.Emu.ShaderLogPath(g.TitleID)); len(live) != 2 {
		t.Fatalf("%v", live)
	}
	for _, f := range res.Files {
		if f.Name == "vulkan.fork-v53.bin" && (f.Action != "skipped" || fileExists(filepath.Join(dir, "vulkan.fork-v53.bin"))) {
			t.Fatalf("legacy cache installed next to a log: %+v", f)
		}
	}
	// 3. again: nothing new, nothing replaced
	res, _ = app.Emu.ImportShaderZip(zipOf(t, map[string][]byte{g.TitleID + "/lichii-cache.v2.bin": v2}), g)
	if res.Added != 0 {
		t.Fatalf("added again: %+v", res)
	}
	// 4. not a ZIP, and a path escaping the archive
	if _, err := app.Emu.ImportShaderZip(filepath.Join(t.TempDir(), "missing.zip"), g); err == nil {
		t.Fatal("a missing file was accepted")
	}
	res, _ = app.Emu.ImportShaderZip(zipOf(t, map[string][]byte{"../evil/lichii-cache.v2.bin": v2}), g)
	if res.Files[0].Action != "skipped" {
		t.Fatal("unsafe path used")
	}
}
