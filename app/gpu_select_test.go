package main

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

func TestChooseVulkanDevice(t *testing.T) {
	intel := VulkanDevice{Index: 0, Name: "Intel(R) UHD Graphics 630", Type: "integrated", LocalMB: 128}
	nv := VulkanDevice{Index: 1, Name: "NVIDIA GeForce RTX 4060 Laptop GPU", Type: "discrete", LocalMB: 8192}
	soft := VulkanDevice{Index: 0, Name: "llvmpipe", Type: "cpu"}
	cases := []struct {
		name string
		devs []VulkanDevice
		pref string
		want int
	}{
		{"laptop: integrated listed first", []VulkanDevice{intel, nv}, "auto", 1},
		{"empty preference = auto", []VulkanDevice{intel, nv}, "", 1},
		{"battery", []VulkanDevice{intel, nv}, "integrated", 0},
		{"one GPU: engine default", []VulkanDevice{{Index: 0, Name: "Radeon", Type: "discrete"}}, "auto", -1},
		{"software rasteriser first", []VulkanDevice{soft, {Index: 1, Name: "Radeon RX 7600", Type: "discrete"}}, "auto", 1},
		{"named card", []VulkanDevice{intel, nv}, "intel(r) uhd graphics 630", 0},
		{"named card gone", []VulkanDevice{intel, nv}, "Old Card", 1},
		{"two discrete: larger", []VulkanDevice{{Index: 0, Type: "discrete", LocalMB: 4096}, {Index: 1, Type: "discrete", LocalMB: 16384}}, "auto", 1},
		{"nothing", nil, "auto", -1},
	}
	for _, c := range cases {
		if got, _ := ChooseVulkanDevice(c.devs, c.pref); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

func TestParseVulkanDevice(t *testing.T) {
	p := make([]byte, 1024)
	m := make([]byte, 600)
	le := binary.LittleEndian
	le.PutUint32(p[0:], 1<<22|3<<12|280)
	le.PutUint32(p[8:], 0x10DE)
	le.PutUint32(p[12:], 0x28E0)
	le.PutUint32(p[16:], 2)
	copy(p[20:], "NVIDIA GeForce RTX 4060 Laptop GPU")
	le.PutUint32(m[260:], 2)      // two heaps
	le.PutUint64(m[264:], 8<<30)  // 8 GB device local
	le.PutUint32(m[264+8:], 1)    //   DEVICE_LOCAL
	le.PutUint64(m[280:], 16<<30) // 16 GB system memory
	d := parseVulkanDevice(3, p, m)
	if d.Index != 3 || d.Type != "discrete" || d.API != "1.3.280" || d.VendorID != 0x10DE || d.LocalMB != 8192 || d.Name != "NVIDIA GeForce RTX 4060 Laptop GPU" {
		t.Fatalf("%+v", d)
	}
}

// The chosen device's index is written into the session configuration the engine reads.
func TestSessionConfigChoosesGPU(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	old := vulkanDevices
	defer func() { vulkanDevices = old }()
	vulkanDevices = func() ([]VulkanDevice, error) {
		return []VulkanDevice{{Index: 0, Name: "Intel", Type: "integrated"}, {Index: 1, Name: "NVIDIA", Type: "discrete", LocalMB: 8192}}, nil
	}
	g, _ := app.Library.Get("t-0100aaaa00010000")
	path, err := app.Emu.writeSessionConfig(&g, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "vulkan_device=1") {
		t.Fatalf("vulkan_device not set:\n%s", b)
	}
	app.Settings.Update(func(s *GlobalSettings) { s.Engine["graphics.gpu"] = "integrated" })
	path, _ = app.Emu.writeSessionConfig(&g, nil)
	b, _ = os.ReadFile(path)
	if !strings.Contains(string(b), "vulkan_device=0") {
		t.Fatalf("integrated preference ignored:\n%s", b)
	}
}

func TestAssessVulkanLayers(t *testing.T) {
	ls := assessVulkanLayers([]string{`C:\Program Files\NVIDIA Corporation\nvoglv64.json`, `C:\Program Files (x86)\Overwolf\0.250.0.4\ow-vulkan-overlay64.json`})
	if len(ls) != 2 || ls[0].Problem != "" || !strings.Contains(ls[1].Problem, "Overwolf") {
		t.Fatalf("%+v", ls)
	}
}
