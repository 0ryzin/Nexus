//go:build !windows

package main

// Graphics cards and displays on Linux (PCs and the Steam Deck), read from sysfs: no Vulkan or
// X11 calls, so detection costs a few file reads and works before any window exists.
//
//   /sys/class/drm/cardN/device/{vendor,device,boot_vga,mem_info_vram_total}, driver symlink
//   /sys/class/drm/cardN-<connector>/{status,modes}
//   Vulkan: the loader (libvulkan.so.1) and an ICD manifest for the card's driver
//   Names: the Steam Deck's APUs by id, otherwise pci.ids when the system has it

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func readHex(p string) uint32 {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(string(b)), "0x"), 16, 32)
	return uint32(v)
}

func readInt(p string) int64 {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return v
}

// sysfsGPUs lists the cards under root (normally /sys/class/drm).
func sysfsGPUs(root string, ids func(ven, dev uint32) string) []GPUInfo {
	cards, _ := filepath.Glob(filepath.Join(root, "card[0-9]*"))
	sort.Strings(cards)
	var out []GPUInfo
	seen := map[string]bool{}
	for _, c := range cards {
		if strings.Contains(filepath.Base(c), "-") { // a connector, not a card
			continue
		}
		dev := filepath.Join(c, "device")
		real, _ := filepath.EvalSymlinks(dev)
		if real == "" || seen[real] {
			continue
		}
		seen[real] = true
		ven, did := readHex(filepath.Join(dev, "vendor")), readHex(filepath.Join(dev, "device"))
		if ven == 0 {
			continue
		}
		drv := ""
		if l, err := os.Readlink(filepath.Join(dev, "driver")); err == nil {
			drv = filepath.Base(l)
		}
		if virtualDisplayDrivers[drv] {
			continue // firmware framebuffers and virtual machines' display adapters run no games
		}
		g := GPUInfo{VendorID: ven, DeviceID: did, Driver: drv}
		g.Vendor = map[uint32]string{vendorNVIDIA: "NVIDIA", vendorAMD: "AMD", 0x8086: "Intel"}[ven]
		if g.Vendor == "" {
			g.Vendor = fmt.Sprintf("0x%04x", ven)
		}
		g.Model = steamDeckGPUs[did]
		if g.Model == "" && ids != nil {
			g.Model = ids(ven, did)
		}
		if g.Model == "" {
			g.Model = fmt.Sprintf("%s graphics (%04x:%04x)", g.Vendor, ven, did)
		}
		vram := readInt(filepath.Join(dev, "mem_info_vram_total")) >> 20
		gtt := readInt(filepath.Join(dev, "mem_info_gtt_total")) >> 20
		switch {
		case ven == vendorNVIDIA:
			g.Kind = "discrete"
		case ven == vendorAMD && vram > 0:
			// an APU has a small carve-out (the Deck: 1 GB) and shares system memory through GTT
			if vram <= 2048 && gtt > vram || steamDeckGPUs[did] != "" {
				g.Kind = "integrated"
				g.SharedMB = gtt
			} else {
				g.Kind = "discrete"
				g.DedicatedMB = vram
			}
		case ven == 0x8086:
			g.Kind = "integrated"
			if drv == "xe" || strings.Contains(strings.ToLower(g.Model), "arc a") || strings.Contains(strings.ToLower(g.Model), "arc b") {
				g.Kind = "discrete"
			}
		default:
			g.Kind = "unknown"
		}
		g.Integrated = g.Kind == "integrated"
		g.Primary = readInt(filepath.Join(dev, "boot_vga")) == 1
		out = append(out, g)
	}
	if len(out) > 0 {
		primary := false
		for _, g := range out {
			primary = primary || g.Primary
		}
		if !primary {
			out[0].Primary = true
		}
	}
	return out
}

// sysfsDisplays lists connected displays with their preferred (first) mode. A portrait panel
// (the Steam Deck's, 800x1280) is reported the way it is used, landscape.
func sysfsDisplays(root string) []Display {
	conns, _ := filepath.Glob(filepath.Join(root, "card[0-9]*-*"))
	sort.Strings(conns)
	var out []Display
	for _, c := range conns {
		st, _ := os.ReadFile(filepath.Join(c, "status"))
		if strings.TrimSpace(string(st)) != "connected" {
			continue
		}
		modes, _ := os.ReadFile(filepath.Join(c, "modes"))
		first, _, _ := strings.Cut(strings.TrimSpace(string(modes)), "\n")
		var w, h int
		if _, err := fmt.Sscanf(first, "%dx%d", &w, &h); err != nil || w == 0 {
			continue
		}
		name := strings.SplitN(filepath.Base(c), "-", 2)[1]
		internal := strings.HasPrefix(name, "eDP") || strings.HasPrefix(name, "DSI") || strings.HasPrefix(name, "LVDS")
		if internal && h > w {
			w, h = h, w
		}
		out = append(out, Display{Name: name, Width: w, Height: h, Primary: internal})
	}
	if len(out) > 0 {
		primary := false
		for _, d := range out {
			primary = primary || d.Primary
		}
		if !primary {
			out[0].Primary = true
		}
	}
	return out
}

// sysfsVulkan: the Vulkan loader is installed and a driver manifest serves one of the cards.
func sysfsVulkan(gpus []GPUInfo) VulkanInfo {
	v := VulkanInfo{}
	for _, d := range []string{"/usr/lib/x86_64-linux-gnu", "/usr/lib/aarch64-linux-gnu", "/usr/lib64", "/usr/lib", "/lib/x86_64-linux-gnu", "/lib/aarch64-linux-gnu"} {
		if fileExists(filepath.Join(d, "libvulkan.so.1")) {
			v.Loader = true
			break
		}
	}
	var icds []string
	for _, d := range []string{"/usr/share/vulkan/icd.d", "/etc/vulkan/icd.d", "/usr/local/share/vulkan/icd.d"} {
		m, _ := filepath.Glob(filepath.Join(d, "*.json"))
		icds = append(icds, m...)
	}
	want := map[string]string{"amdgpu": "radeon", "radeon": "radeon", "i915": "intel", "xe": "intel", "nvidia": "nvidia", "nouveau": "nouveau", "msm": "freedreno", "msm_drm": "freedreno", "panfrost": "panfrost", "asahi": "asahi"}
	for _, g := range gpus {
		key := want[g.Driver]
		for _, p := range icds {
			if key != "" && strings.Contains(strings.ToLower(filepath.Base(p)), key) {
				v.Available = v.Loader
			}
		}
	}
	return v
}

// pciIDs looks names up in the system's pci.ids (hwdata), when it has one.
func pciIDs() func(ven, dev uint32) string {
	var path string
	for _, p := range []string{"/usr/share/hwdata/pci.ids", "/usr/share/misc/pci.ids", "/usr/share/pci.ids"} {
		if fileExists(p) {
			path = p
			break
		}
	}
	if path == "" {
		return nil
	}
	return func(ven, dev uint32) string {
		f, err := os.Open(path)
		if err != nil {
			return ""
		}
		defer f.Close()
		vs, ds := fmt.Sprintf("%04x", ven), fmt.Sprintf("\t%04x", dev)
		in := false
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			l := sc.Text()
			if l == "" || l[0] == '#' {
				continue
			}
			if l[0] != '\t' {
				if in {
					return ""
				}
				in = strings.HasPrefix(l, vs+"  ")
				continue
			}
			if in && strings.HasPrefix(l, ds+"  ") {
				name := strings.TrimSpace(l[len(ds)+2:])
				// "Navi 22 [Radeon RX 6700/6700 XT/6750 XT / 6800M/6850M XT]" -> the marketing name
				if i, j := strings.Index(name, "["), strings.LastIndex(name, "]"); i >= 0 && j > i {
					name = name[i+1 : j]
				}
				return name
			}
		}
		return ""
	}
}
