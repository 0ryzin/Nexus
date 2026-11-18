package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSmoothMotionSupport(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	dir := t.TempDir()
	old := smoothMotionLayerDirs
	smoothMotionLayerDirs = []string{dir}
	defer func() { smoothMotionLayerDirs = old }()
	rtx := Hardware{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Model: "GeForce RTX 4070", Driver: "nvidia"}}}
	if ok, why := smoothMotionSupport(rtx); ok || why == "" {
		t.Fatal("offered without the driver's layer")
	}
	os.WriteFile(filepath.Join(dir, "nvidia_layers.json"), []byte(`{"layer":{"name":"VK_LAYER_NV_present"}}`), 0o644)
	if ok, why := smoothMotionSupport(rtx); !ok {
		t.Fatal(why)
	}
	for _, h := range []Hardware{
		{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Model: "GeForce RTX 3080", Driver: "nvidia"}}},
		{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Model: "GeForce RTX 4070", Driver: "nouveau"}}},
		{GPUs: []GPUInfo{{VendorID: vendorAMD, Model: "AMD Custom GPU 0405 (Steam Deck)", Driver: "amdgpu"}}},
	} {
		if ok, _ := smoothMotionSupport(h); ok {
			t.Fatalf("offered on %+v", h.GPUs[0])
		}
	}
	if ok, _ := smoothMotionSupport(Hardware{GPUs: []GPUInfo{{VendorID: vendorNVIDIA, Model: "NVIDIA GeForce RTX 5090", Driver: "nvidia"}}}); !ok {
		t.Fatal("RTX 5090")
	}
	applySmoothMotion(true)
	if os.Getenv(smoothMotionEnv) != "1" {
		t.Fatal("not set")
	}
	applySmoothMotion(false)
	if _, set := os.LookupEnv(smoothMotionEnv); set {
		t.Fatal("not cleared")
	}
}
