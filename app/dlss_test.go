package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeStreamlineZip(t *testing.T) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range map[string]string{
		"license.txt": "Streamline MIT licence", "bin/x64/nvngx_dlss.license.txt": "DLSS licence",
		"bin/x64/sl.interposer.dll": strings.Repeat("I", 5000), "bin/x64/sl.common.dll": strings.Repeat("C", 3000),
		"bin/x64/sl.dlss.dll": strings.Repeat("D", 3000), "bin/x64/nvngx_dlss.dll": strings.Repeat("N", 200000),
		"docs/readme.md": "x",
	} {
		method := zip.Deflate
		if strings.HasSuffix(name, "nvngx_dlss.dll") {
			method = zip.Store // stored: the test can damage its bytes
		}
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		w.Write([]byte(body))
	}
	zw.Close()
	return b.Bytes()
}

// TestDLSSDownloadVerified: only an RTX card is offered DLSS; the licences are shown before
// anything is downloaded; only the missing files are fetched (with range requests), checked
// (CRC-32, NVIDIA signature) and installed; a damaged download installs nothing.
func TestDLSSDownloadVerified(t *testing.T) {
	data := fakeStreamlineZip(t)
	ranges := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			t.Error("whole-file download instead of range requests")
		}
		ranges++
		http.ServeContent(w, r, "sl.zip", time.Now(), bytes.NewReader(data))
	}))
	defer srv.Close()
	old := dlssMgr
	dlssMgr = &dlssManager{url: srv.URL, client: srv.Client()}
	defer func() { dlssMgr = old }()
	signed := true
	verifyNVIDIASigned = func(string) error {
		if !signed {
			return errors.New("not signed by NVIDIA")
		}
		return nil
	}
	defer func() { verifyNVIDIASigned = checkNVIDIASignature }()
	t.Setenv("ProgramData", t.TempDir())

	hw := filepath.Join(t.TempDir(), "hw.json")
	write := func(model string, vendor uint32) {
		d, _ := json.Marshal(Hardware{GPUs: []GPUInfo{{Vendor: "GPU", Model: model, VendorID: vendor, Primary: true, Kind: "discrete"}}})
		os.WriteFile(hw, d, 0o644)
	}
	write("AMD Radeon RX 7800 XT", vendorAMD)
	t.Setenv("NEXUS_HARDWARE", hw)
	app, _ := newTestApp(t, "")
	app.DetectHardware(true)
	if st := app.Emu.DLSSStatus(); st.Supported || st.Why == "" {
		t.Fatalf("AMD card offered DLSS: %+v", st)
	}
	if _, err := app.Emu.PrepareDLSS(); err == nil || ranges != 0 {
		t.Fatal("unsupported card must not download anything")
	}
	write("NVIDIA GeForce RTX 4070", vendorNVIDIA)
	app.DetectHardware(true)
	os.MkdirAll(app.Emu.streamlineDir(), 0o755)
	os.WriteFile(filepath.Join(app.Emu.streamlineDir(), "sl.common.dll"), []byte("already here"), 0o644)
	st := app.Emu.DLSSStatus()
	if !st.Supported || st.Ready || len(st.Missing) != 3 || contains(st.Missing, "sl.common.dll") {
		t.Fatalf("status %+v", st)
	}
	if err := app.Emu.InstallDLSS(false); err == nil {
		t.Fatal("installed without accepting the licences")
	}
	offer, err := app.Emu.PrepareDLSS()
	if err != nil || len(offer.Licences) != 2 || !strings.Contains(offer.Licences[1].Text, "DLSS licence") || len(offer.Files) != 3 {
		t.Fatalf("offer %+v %v", offer, err)
	}
	// a file failing the signature check is not installed
	signed = false
	app.Emu.InstallDLSS(true)
	j := waitDLSS(t, app)
	if j.OK || j.Error == "" || fileExists(filepath.Join(app.Emu.streamlineDir(), "sl.interposer.dll")) {
		t.Fatalf("unsigned file installed: %+v", j)
	}
	if m, _ := filepath.Glob(filepath.Join(app.Emu.streamlineDir(), "*.download")); len(m) != 0 {
		t.Fatal("partial file left behind")
	}
	signed = true
	app.Emu.InstallDLSS(true)
	if j := waitDLSS(t, app); !j.OK {
		t.Fatalf("install %+v", j)
	}
	if st := app.Emu.DLSSStatus(); !st.Ready || readFile(t, filepath.Join(st.Folder, "sl.common.dll")) != "already here" {
		t.Fatalf("after install %+v", st)
	}
	if len(readFile(t, filepath.Join(app.Emu.streamlineDir(), "nvngx_dlss.dll"))) != 200000 {
		t.Fatal("content")
	}
	// damaged download (CRC mismatch)
	os.Remove(filepath.Join(app.Emu.streamlineDir(), "nvngx_dlss.dll"))
	i := bytes.Index(data, []byte("NNNN"))
	data[i+10] = 'X'
	dlssMgr.entries = nil
	if _, err := app.Emu.PrepareDLSS(); err != nil {
		t.Fatal(err)
	}
	app.Emu.InstallDLSS(true)
	if j := waitDLSS(t, app); j.OK || !strings.Contains(j.Error, "integrity") || fileExists(filepath.Join(app.Emu.streamlineDir(), "nvngx_dlss.dll")) {
		t.Fatalf("damaged: %+v", j)
	}
}

func waitDLSS(t *testing.T, app *App) DLSSJob {
	for i := 0; i < 300; i++ {
		if st := app.Emu.DLSSStatus(); st.Job != nil && !st.Job.Running {
			return *st.Job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("download did not finish")
	return DLSSJob{}
}
