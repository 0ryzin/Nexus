package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withHardware(t *testing.T, h Hardware) {
	p := filepath.Join(t.TempDir(), "hw.json")
	data, _ := json.Marshal(h)
	os.WriteFile(p, data, 0o644)
	t.Setenv("NEXUS_HARDWARE", p)
	hwMu.Lock()
	hwCache = nil
	hwMu.Unlock()
}

func TestGPUFeatureDetection(t *testing.T) {
	cases := []struct {
		g          GPUInfo
		dlss, fsr4 bool
	}{
		{GPUInfo{VendorID: vendorNVIDIA, Model: "NVIDIA GeForce RTX 4070"}, true, false},
		{GPUInfo{VendorID: vendorNVIDIA, Model: "NVIDIA RTX A2000"}, true, false},
		{GPUInfo{VendorID: vendorNVIDIA, Model: "NVIDIA GeForce GTX 1080 Ti"}, false, false},
		{GPUInfo{VendorID: vendorNVIDIA, Model: "NVIDIA GeForce MX450"}, false, false},
		{GPUInfo{VendorID: vendorAMD, Model: "AMD Radeon RX 9070 XT"}, false, true},
		{GPUInfo{VendorID: vendorAMD, Model: "AMD Radeon RX 7900 XTX"}, false, false},
		{GPUInfo{VendorID: 0x8086, Model: "Intel(R) Arc(TM) A770 Graphics"}, false, false},
	}
	for _, c := range cases {
		if dlssCapable(c.g) != c.dlss || fsr4Capable(c.g) != c.fsr4 {
			t.Errorf("%s: dlss=%v fsr4=%v", c.g.Model, dlssCapable(c.g), fsr4Capable(c.g))
		}
	}
}

func TestEnhancementSettings(t *testing.T) {
	withWindowsUpscalers(t)
	app, _ := testNewApp(t, t.TempDir())
	// a GTX card: DLSS not offered, frame generation (FSR 3) offered, Lossless needs its file
	withHardware(t, Hardware{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Vendor: "NVIDIA", Model: "NVIDIA GeForce GTX 1660", Kind: "discrete", Primary: true, VulkanAPI: "1.3"}}, Vulkan: VulkanInfo{Available: true}})
	caps := app.Emu.hardwareCaps()
	if caps["enhance.dlss"].Available || caps["enhance.dlss"].Note == "" {
		t.Fatalf("DLSS must not be offered on a GTX card: %+v", caps["enhance.dlss"])
	}
	if !caps["enhance.framegen"].Available || len(caps["enhance.framegen_type"].Choices) != 1 {
		t.Fatalf("FSR 3 frame generation only, until Lossless.dll is there: %+v", caps["enhance.framegen_type"])
	}
	// an RTX card: DLSS offered (Streamline still missing -> not ready)
	withHardware(t, Hardware{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Vendor: "NVIDIA", Model: "NVIDIA GeForce RTX 3060", Kind: "discrete", Primary: true, VulkanAPI: "1.3"}}, Vulkan: VulkanInfo{Available: true}})
	en := app.Emu.Enhancements()
	if !en.DLSS.Supported || en.DLSS.Ready || !app.Emu.hardwareCaps()["enhance.dlss"].Available {
		t.Fatalf("RTX: %+v", en.DLSS)
	}
	os.MkdirAll(app.Emu.streamlineDir(), 0o755)
	os.WriteFile(filepath.Join(app.Emu.streamlineDir(), "sl.interposer.dll"), []byte("x"), 0o644)
	if en := app.Emu.Enhancements(); en.DLSS.Ready {
		t.Fatal("the plugins and the DLSS model are still missing")
	}
	for _, f := range []string{"sl.common.dll", "sl.dlss.dll", "nvngx_dlss.dll"} {
		os.WriteFile(filepath.Join(app.Emu.streamlineDir(), f), []byte("x"), 0o644)
	}
	if en := app.Emu.Enhancements(); !en.DLSS.Ready {
		t.Fatalf("Streamline present: %+v", en.DLSS)
	}
	// Lossless.dll setup (lossless_test.go has the details)
	src := filepath.Join(t.TempDir(), "Lossless.dll")
	os.WriteFile(src, goodLossless(1), 0o644)
	if _, err := app.Emu.SetupLossless(filepath.Join(t.TempDir(), "other.dll"), false); err == nil {
		t.Fatal("a file that is not there must be refused")
	}
	if _, err := app.Emu.SetupLossless(src, false); err != nil || !app.Emu.Enhancements().Lossless.Ready {
		t.Fatalf("import: %v", err)
	}
	if c := app.Emu.hardwareCaps()["enhance.framegen_type"]; len(c.Choices) != 2 {
		t.Fatalf("both methods after import: %+v", c)
	}
	// the settings reach the engine configuration
	out := engineProject(map[string]string{"enhance.framegen": "true", "enhance.framegen_type": "lsfg", "enhance.framegen_mult": "3",
		"enhance.framegen_target": "120", "enhance.dlss": "true", "enhance.dlss_quality": "performance", "enhance.dlss_model": "k", "enhance.fsr4": "false"}, true)
	want := map[string]string{"Renderer/frame_gen": "true", "Renderer/frame_gen_type": "0", "Renderer/frame_gen_multiplier": "3",
		"Renderer/frame_gen_target_rate": "120", "Renderer/nvidia_dlss": "true", "Renderer/nvidia_dlss_quality": "4", "Renderer/nvidia_dlss_preset": "5", "Renderer/amd_fsr4": "false"}
	for k, v := range want {
		if out[k] != v {
			t.Errorf("%s = %q, want %q", k, out[k], v)
		}
	}
}

// withWindowsUpscalers: the test covers DLSS / FSR 4, which NEXUS offers on Windows only.
func withWindowsUpscalers(t *testing.T) {
	old := windowsUpscalerLibs
	windowsUpscalerLibs = true
	t.Cleanup(func() { windowsUpscalerLibs = old })
}

func TestUpscalersWindowsOnly(t *testing.T) {
	old := windowsUpscalerLibs
	windowsUpscalerLibs = false
	defer func() { windowsUpscalerLibs = old }()
	app, _ := testNewApp(t, t.TempDir())
	withHardware(t, Hardware{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Vendor: "NVIDIA", Model: "NVIDIA GeForce RTX 4070", Kind: "discrete", Primary: true, VulkanAPI: "1.3"}}, Vulkan: VulkanInfo{Available: true}})
	en := app.Emu.Enhancements()
	if en.DLSS.Supported || en.FSR4.Supported || !strings.Contains(en.DLSS.Why, "Windows only") {
		t.Fatalf("%+v %+v", en.DLSS, en.FSR4)
	}
}
