package main

import (
	"archive/zip"
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func waitShaderJob(t *testing.T, app *App) *ShaderJob {
	t.Helper()
	for i := 0; i < 600; i++ {
		if j := app.Emu.shaderJobs.get(); j != nil && j.State != "running" {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the shader job did not finish")
	return nil
}

// TestShaderExportZipAndReimport: Export Shader Data writes a ZIP (manifest + pack, nothing else)
// into the chosen folder; Import from a ZIP file on another device adds every record (15,000
// programs here), a second import adds nothing and backs up first; a ZIP for another game, a
// damaged one and the source file itself are handled safely.
func TestShaderExportZipAndReimport(t *testing.T) {
	pc, _ := newTestApp(t, "")
	g := shaderGame(t, pc)
	var recs [][]byte
	for i := 0; i < 15000; i++ {
		recs = append(recs, v2rec(v2KindProgram, uint64(i+1), "program "+strconv.Itoa(i)))
	}
	recs = append(recs, v2rec(v2KindGraphics, 1, "pipeline"), v2rec(8, 5, "driver binary"))
	writeLog(t, pc.Emu.ShaderLogPath(g.TitleID), recs...)
	os.WriteFile(pc.Emu.ShaderLogPath(g.TitleID)+".lock", nil, 0o644)

	dest := t.TempDir()
	if _, err := pc.Emu.StartShaderExport(g, dest); err != nil {
		t.Fatal(err)
	}
	j := waitShaderJob(t, pc)
	if j.State != "done" || j.Export == nil || j.Export.Programs != 15000 || j.Export.Graphics != 1 {
		t.Fatalf("export: %+v", j)
	}
	if filepath.Dir(j.Export.Path) != dest || !strings.HasSuffix(j.Export.Path, "["+strings.ToUpper(g.TitleID)+"].zip") {
		t.Fatalf("export path %s", j.Export.Path)
	}
	zr, err := zip.OpenReader(j.Export.Path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	zr.Close()
	if len(names) != 2 || names[0] != shaderExportManifest || !strings.HasPrefix(names[1], "shaders/") || !strings.HasSuffix(names[1], ".nxshaders") {
		t.Fatalf("ZIP contents %v", names)
	}
	src, _ := os.ReadFile(j.Export.Path)

	// another device: everything arrives
	deck, _ := newTestApp(t, "")
	dg := shaderGame(t, deck)
	if _, err := deck.Emu.StartShaderZipImport(dg, j.Export.Path); err != nil {
		t.Fatal(err)
	}
	ij := waitShaderJob(t, deck)
	if ij.State != "done" || ij.Zip == nil || ij.Zip.Added != 15001 || ij.Zip.Usable != 1 || ij.Backup != "" {
		t.Fatalf("import: %+v %+v", ij, ij.Zip)
	}
	if got := liveOf(t, deck.Emu.ShaderLogPath(dg.TitleID)); len(got) != 15001 || got[rk{1, 1}] != "program 0" {
		t.Fatalf("imported %d records", len(got))
	}
	// again: nothing new, the existing data backed up first
	deck.Emu.StartShaderZipImport(dg, j.Export.Path)
	ij = waitShaderJob(t, deck)
	if ij.State != "done" || ij.Zip.Added != 0 || ij.Backup == "" || !fileExists(ij.Backup) {
		t.Fatalf("second import: %+v", ij)
	}
	if after, _ := os.ReadFile(j.Export.Path); !bytes.Equal(after, src) {
		t.Fatal("the source ZIP was changed")
	}

	// the export of another game is refused before anything changes
	other := *dg
	other.TitleID, other.Title = "0100AAAA00020000", "b"
	before, _ := os.ReadFile(deck.Emu.ShaderLogPath(dg.TitleID))
	deck.Emu.StartShaderZipImport(&other, j.Export.Path)
	ij = waitShaderJob(t, deck)
	if ij.State != "failed" || !strings.Contains(ij.Error, "not b's") {
		t.Fatalf("another game's export: %+v", ij)
	}
	if after, _ := os.ReadFile(deck.Emu.ShaderLogPath(dg.TitleID)); !bytes.Equal(before, after) {
		t.Fatal("a refused import changed the cache")
	}

	// a damaged pack (checksum) is refused whole
	zr2, _ := zip.OpenReader(j.Export.Path)
	files := map[string][]byte{}
	for _, f := range zr2.File {
		b, _ := readZipEntry(f, 1<<30)
		files[f.Name] = b
	}
	zr2.Close()
	for n, b := range files {
		if strings.HasSuffix(n, ".nxshaders") {
			b[len(b)-3] ^= 0xff
		}
	}
	bad := zipOf(t, files)
	deck.Emu.StartShaderZipImport(dg, bad)
	ij = waitShaderJob(t, deck)
	if ij.State != "failed" || !strings.Contains(ij.Error, "checksum") {
		t.Fatalf("damaged: %+v", ij)
	}

	// a ZIP of loose files: lock files, path escapes and other games are skipped with reasons
	loose := zipOf(t, map[string][]byte{"cache.lock": []byte("x"), "../evil.bin": []byte("yuzucach"), "0100AAAA00020000/x.bin": []byte("yuzucach0000")})
	deck.Emu.StartShaderZipImport(dg, loose)
	ij = waitShaderJob(t, deck)
	if ij.State != "done" || ij.Zip.Usable != 0 || len(ij.Zip.Files) != 3 {
		t.Fatalf("loose: %+v", ij.Zip)
	}
	for _, f := range ij.Zip.Files {
		if f.Action != "skipped" || f.Reason == "" {
			t.Fatalf("%+v", f)
		}
	}
	// one job at a time
	if _, err := deck.Emu.startShaderJob("export", dg, func(*ShaderJob, func(string)) error { time.Sleep(200 * time.Millisecond); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := deck.Emu.StartShaderExport(dg, t.TempDir()); err == nil {
		t.Fatal("a second job must wait")
	}
	waitShaderJob(t, deck)
}

// TestShaderFromLinkedDevice: Import Shader Data › From another device asks the linked PC for its
// pack directly; a PC without the game, and no Device Linking, are explained.
func TestShaderFromLinkedDevice(t *testing.T) {
	linkApp := func(app *App, name string) (*DeviceLink, string) {
		l, err := NewDeviceLink(app)
		if err != nil {
			t.Fatal(err)
		}
		l.id.DeviceName = name
		if err := l.Listen(0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Close)
		app.Link = l
		return l, net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Port))
	}
	pc, _ := newTestApp(t, "")
	deck, _ := newTestApp(t, "")
	g, dg := shaderGame(t, pc), shaderGame(t, deck)
	if _, err := deck.Emu.StartShaderDeviceImport(dg, "x"); err == nil || !strings.Contains(err.Error(), "Device Linking is off") {
		t.Fatalf("no linking: %v", err)
	}
	writeLog(t, pc.Emu.ShaderLogPath(g.TitleID), v2rec(v2KindProgram, 1, "p1"), v2rec(v2KindGraphics, 2, "g2"))
	pcl, pcAddr := linkApp(pc, "My Windows PC")
	deckl, _ := linkApp(deck, "Steam Deck")
	tom, _ := pc.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	code, _, _ := pcl.StartPairing(tom.ID)
	approveNext(&linkPeer{app: pc, link: pcl}, true)
	d, err := deckl.PairWith(pcAddr, UserID(tom.ID), code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deck.Emu.StartShaderDeviceImport(dg, d.DeviceID); err != nil {
		t.Fatal(err)
	}
	j := waitShaderJob(t, deck)
	if j.State != "done" || j.Pack == nil || j.Pack.Added != 2 || j.Device != "My Windows PC" {
		t.Fatalf("%+v %+v", j, j.Pack)
	}
	if got := liveOf(t, deck.Emu.ShaderLogPath(dg.TitleID)); got[rk{1, 1}] != "p1" {
		t.Fatalf("%v", got)
	}
	// a game the PC does not have: its answer is shown
	other := *dg
	other.TitleID = "0100FFFF00010000"
	deck.Emu.StartShaderDeviceImport(&other, d.DeviceID)
	j = waitShaderJob(t, deck)
	if j.State != "failed" || !strings.Contains(j.Error, "not in the library there") {
		t.Fatalf("%+v", j)
	}
	// the PC is closed: unreachable, said so
	pcl.Close()
	deck.Emu.StartShaderDeviceImport(dg, d.DeviceID)
	j = waitShaderJob(t, deck)
	if j.State != "failed" || !strings.Contains(j.Error, "cannot be reached") {
		t.Fatalf("closed: %+v", j)
	}
}

// TestMakeShaderFixture (tools/uitest/shader_data.py): NEXUS_SHADER_FIXTURE=<folder> gets a real
// Export Shader Data ZIP for Starfall Rally (0100AAAA00010000, 1,200 programs, 2 pipelines).
func TestMakeShaderFixture(t *testing.T) {
	dir := os.Getenv("NEXUS_SHADER_FIXTURE")
	if dir == "" {
		t.Skip("NEXUS_SHADER_FIXTURE not set")
	}
	app, _ := newTestApp(t, "")
	g := shaderGame(t, app)
	var recs [][]byte
	for i := 0; i < 1200; i++ {
		recs = append(recs, v2rec(v2KindProgram, uint64(i+1), "program "+strconv.Itoa(i)))
	}
	recs = append(recs, v2rec(v2KindGraphics, 1, "pipeline 1"), v2rec(v2KindCompute, 2, "pipeline 2"))
	writeLog(t, app.Emu.ShaderLogPath(g.TitleID), recs...)
	ok, err := app.Emu.ExportShaderZip(g, dir)
	if err != nil {
		t.Fatal(err)
	}
	os.Rename(ok.Path, filepath.Join(dir, "starfall-shaders.zip"))
}
