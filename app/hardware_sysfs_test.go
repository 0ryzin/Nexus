//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeCard(t *testing.T, root, card, pci string, files map[string]string, driver string) {
	dev := filepath.Join(root, "devices", pci)
	os.MkdirAll(dev, 0o755)
	for k, v := range files {
		os.WriteFile(filepath.Join(dev, k), []byte(v+"\n"), 0o644)
	}
	if driver != "" {
		os.MkdirAll(filepath.Join(root, "drivers", driver), 0o755)
		os.Symlink(filepath.Join(root, "drivers", driver), filepath.Join(dev, "driver"))
	}
	os.MkdirAll(filepath.Join(root, "drm", card), 0o755)
	os.Symlink(dev, filepath.Join(root, "drm", card, "device"))
}

func fakeConnector(root, name, status, modes string) {
	d := filepath.Join(root, "drm", name)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "status"), []byte(status+"\n"), 0o644)
	os.WriteFile(filepath.Join(d, "modes"), []byte(modes), 0o644)
}

func TestSysfsSteamDeck(t *testing.T) {
	root := t.TempDir()
	fakeCard(t, root, "card0", "0000:04:00.0", map[string]string{"vendor": "0x1002", "device": "0x1435", "boot_vga": "1",
		"mem_info_vram_total": "1073741824", "mem_info_gtt_total": "8589934592"}, "amdgpu")
	fakeConnector(root, "card0-eDP-1", "connected", "800x1280\n")
	fakeConnector(root, "card0-DP-1", "disconnected", "")
	g := sysfsGPUs(filepath.Join(root, "drm"), nil)
	if len(g) != 1 || g[0].Model != "AMD Custom GPU 0932 (Steam Deck OLED)" || g[0].Kind != "integrated" || !g[0].Primary || g[0].Driver != "amdgpu" {
		t.Fatalf("%+v", g)
	}
	d := sysfsDisplays(filepath.Join(root, "drm"))
	if len(d) != 1 || d[0].Width != 1280 || d[0].Height != 800 || !d[0].Primary {
		t.Fatalf("%+v", d)
	}
	c := classifyDevice(Hardware{GPUs: g, Memory: MemoryInfo{TotalMB: 16 * 1024}})
	if c.GPUTier != 0 {
		t.Fatalf("Steam Deck tier %d", c.GPUTier)
	}
}

func TestSysfsDesktop(t *testing.T) {
	root := t.TempDir()
	fakeCard(t, root, "card0", "0000:00:02.0", map[string]string{"vendor": "0x8086", "device": "0x4680"}, "i915")
	fakeCard(t, root, "card1", "0000:01:00.0", map[string]string{"vendor": "0x10de", "device": "0x2786", "boot_vga": "1"}, "nvidia")
	fakeCard(t, root, "card2", "0000:03:00.0", map[string]string{"vendor": "0x1002", "device": "0x73df",
		"mem_info_vram_total": "12884901888", "mem_info_gtt_total": "8589934592"}, "amdgpu")
	fakeCard(t, root, "card3", "simple", map[string]string{"vendor": "0x1234", "device": "0x1111"}, "simpledrm")
	ids := func(v, d uint32) string { return map[uint32]string{0x2786: "GeForce RTX 4070"}[d] }
	g := sysfsGPUs(filepath.Join(root, "drm"), ids)
	if len(g) != 3 {
		t.Fatalf("%+v", g)
	}
	if g[0].Kind != "integrated" || g[1].Kind != "discrete" || !g[1].Primary || g[1].Model != "GeForce RTX 4070" || g[0].Primary {
		t.Fatalf("%+v", g)
	}
	if g[2].Kind != "discrete" || g[2].DedicatedMB != 12288 {
		t.Fatalf("%+v", g[2])
	}
	fakeConnector(root, "card1-HDMI-A-1", "connected", "2560x1440\n1920x1080\n")
	if d := sysfsDisplays(filepath.Join(root, "drm")); len(d) != 1 || d[0].Width != 2560 || !d[0].Primary {
		t.Fatalf("%+v", d)
	}
}
