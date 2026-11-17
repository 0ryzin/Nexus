package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLosslessDLL builds a minimal PE file with the RCDATA resources the engine reads from
// Lossless.dll. ids: the resource ids to include; payload: the bytes of each (SPIR-V or not).
func fakeLosslessDLL(ids []uint32, spirv bool, tag byte) []byte {
	le := binary.LittleEndian
	const rsrcRVA, rsrcRaw = 0x1000, 0x400
	// resource tree, offsets relative to the start of the resource section
	n := uint32(len(ids))
	rootOff, typeOff := uint32(0), uint32(16+8)
	langOff := typeOff + 16 + 8*n
	leafOff := langOff + n*(16+8)
	dataOff := leafOff + n*16
	res := make([]byte, dataOff+n*16)
	le.PutUint16(res[rootOff+14:], 1)
	le.PutUint32(res[rootOff+16:], 10)
	le.PutUint32(res[rootOff+20:], 0x80000000|typeOff)
	le.PutUint16(res[typeOff+14:], uint16(n))
	for i, id := range ids {
		ui := uint32(i)
		le.PutUint32(res[typeOff+16+ui*8:], id)
		lo := langOff + ui*(16+8)
		le.PutUint32(res[typeOff+16+ui*8+4:], 0x80000000|lo)
		le.PutUint16(res[lo+14:], 1)
		le.PutUint32(res[lo+16:], 0x409)
		lf := leafOff + ui*16
		le.PutUint32(res[lo+20:], lf)
		d := dataOff + ui*16
		le.PutUint32(res[lf:], rsrcRVA+d)
		le.PutUint32(res[lf+4:], 16)
		if spirv {
			le.PutUint32(res[d:], 0x07230203)
		}
		res[d+8] = tag
	}
	img := make([]byte, rsrcRaw+len(res))
	img[0], img[1] = 'M', 'Z'
	le.PutUint32(img[0x3C:], 0x40)
	copy(img[0x40:], "PE\x00\x00")
	coff := 0x44
	le.PutUint16(img[coff:], 0x8664)
	le.PutUint16(img[coff+2:], 1)
	le.PutUint16(img[coff+16:], 240)
	opt := coff + 20
	le.PutUint16(img[opt:], 0x20B)
	le.PutUint32(img[opt+112+16:], rsrcRVA)
	le.PutUint32(img[opt+112+20:], uint32(len(res)))
	sec := opt + 240
	copy(img[sec:], ".rsrc")
	le.PutUint32(img[sec+8:], uint32(len(res)))
	le.PutUint32(img[sec+12:], rsrcRVA)
	le.PutUint32(img[sec+16:], uint32(len(res)))
	le.PutUint32(img[sec+20:], rsrcRaw)
	copy(img[rsrcRaw:], res)
	return img
}

func goodLossless(tag byte) []byte { return fakeLosslessDLL(losslessRequired(), true, tag) }

func TestLosslessDLLCheck(t *testing.T) {
	if m, n, p := checkLosslessImage(goodLossless(1)); p != "" || m != "x64" || n != len(losslessRequired()) {
		t.Fatalf("good DLL: %s %d %q", m, n, p)
	}
	if _, _, p := checkLosslessImage([]byte("not a dll at all")); !strings.Contains(p, "not a Windows DLL") {
		t.Fatalf("text file: %q", p)
	}
	if _, _, p := checkLosslessImage(fakeLosslessDLL([]uint32{1, 2}, true, 0)); !strings.Contains(p, "does not contain") {
		t.Fatalf("other DLL: %q", p)
	}
	if _, n, p := checkLosslessImage(fakeLosslessDLL(losslessRequired()[:10], true, 0)); !strings.Contains(p, "missing") || n != 10 {
		t.Fatalf("partial DLL: %d %q", n, p)
	}
	if _, _, p := checkLosslessImage(fakeLosslessDLL(losslessRequired(), false, 0)); p == "" {
		t.Fatal("resources that are not SPIR-V accepted")
	}
}

// TestLosslessSetup: Steam found through its library list on another drive, the app found by its
// manifest, checked, copied once; a repeat copies nothing; a different version needs Update; a
// folder chosen by the user (Steam root or the app folder) works the same.
func TestLosslessSetup(t *testing.T) {
	app, _ := newTestApp(t, "")
	withHardware(t, Hardware{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Vendor: "NVIDIA", Model: "NVIDIA GeForce RTX 3060", Kind: "discrete", Primary: true, VulkanAPI: "1.3"}}, Vulkan: VulkanInfo{Available: true}})
	steam := filepath.Join(t.TempDir(), "C", "Program Files (x86)", "Steam")
	lib := filepath.Join(t.TempDir(), "D", "SteamLibrary")
	os.MkdirAll(filepath.Join(steam, "steamapps"), 0o755)
	os.WriteFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf"), []byte("\"libraryfolders\"\n{\n \"0\" { \"path\" \""+strings.ReplaceAll(steam, `\`, `\\`)+"\" }\n \"1\" { \"path\" \""+strings.ReplaceAll(lib, `\`, `\\`)+"\" }\n}\n"), 0o644)
	os.MkdirAll(filepath.Join(lib, "steamapps", "common", "LS"), 0o755)
	os.WriteFile(filepath.Join(lib, "steamapps", "appmanifest_993090.acf"), []byte(`"AppState" { "appid" "993090" "installdir" "LS" }`), 0o644)
	dll := filepath.Join(lib, "steamapps", "common", "LS", "Lossless.dll")
	saved := steamRootsFn
	defer func() { steamRootsFn = saved }()
	steamRootsFn = func() []string { return nil }
	if st := app.Emu.LosslessStatus(""); st.State != "notfound" || st.Message != "Lossless Scaling files not found." {
		t.Fatalf("no Steam: %+v", st)
	}
	steamRootsFn = func() []string { return []string{steam} }
	os.WriteFile(dll, []byte("broken"), 0o644)
	if st := app.Emu.LosslessStatus(""); st.State != "invalid" {
		t.Fatalf("broken DLL: %+v", st)
	}
	if _, err := app.Emu.SetupLossless("", false); err == nil || fileExists(app.Emu.losslessInstalledPath()) {
		t.Fatal("a broken DLL was copied")
	}
	os.WriteFile(dll, goodLossless(1), 0o644)
	st := app.Emu.LosslessStatus("")
	if st.State != "found" || st.Message != "Lossless Scaling detected." || st.Found.Path != dll || len(st.Libraries) != 2 {
		t.Fatalf("found: %+v", st)
	}
	os.MkdirAll(app.Emu.losslessDir(), 0o755)
	os.WriteFile(filepath.Join(app.Emu.losslessDir(), "lsfg_spirv.cache"), []byte("old"), 0o644)
	if st, err := app.Emu.SetupLossless("", false); err != nil || st.State != "ready" || st.Message != "Lossless Scaling integration ready." {
		t.Fatalf("setup: %v %+v", err, st)
	}
	if fileExists(filepath.Join(app.Emu.losslessDir(), "lsfg_spirv.cache")) {
		t.Fatal("the old shader translation was kept")
	}
	fi1, _ := os.Stat(app.Emu.losslessInstalledPath())
	// a repeat copies nothing
	if _, err := app.Emu.SetupLossless("", false); err != nil {
		t.Fatal(err)
	}
	fi2, _ := os.Stat(app.Emu.losslessInstalledPath())
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("copied again")
	}
	// a different version in Steam: shown as an update, not copied without asking
	os.WriteFile(dll, goodLossless(2), 0o644)
	if st := app.Emu.LosslessStatus(""); st.State != "update" {
		t.Fatalf("update: %+v", st)
	}
	if _, err := app.Emu.SetupLossless("", false); err == nil {
		t.Fatal("a working DLL was replaced without asking")
	}
	if st, err := app.Emu.SetupLossless("", true); err != nil || st.State != "ready" {
		t.Fatalf("update: %v %+v", err, st)
	}
	// the user points at Steam itself, or the app folder
	steamRootsFn = func() []string { return nil }
	if st := app.Emu.LosslessStatus(steam); st.Found == nil || st.Found.Path != dll {
		t.Fatalf("Steam folder chosen: %+v", st)
	}
	if st := app.Emu.LosslessStatus(filepath.Dir(dll)); st.Found == nil || st.State != "ready" {
		t.Fatalf("app folder chosen: %+v", st)
	}
	if st := app.Emu.LosslessStatus(t.TempDir()); st.State != "ready" || st.Found != nil {
		t.Fatalf("empty folder chosen (installed copy still works): %+v", st)
	}
}
