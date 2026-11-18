//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func readNum(path string) (float64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return v, err == nil
}

var reCurClock = regexp.MustCompile(`(?m)^\d+:\s*(\d+)\s*[Mm][Hh]z\s*\*`)

// sysfsSample fills what Linux's sysfs offers (root is "/sys"; tests pass a fake tree).
func sysfsSample(p *PerfSample, root string) {
	// the GPU: the amdgpu card with the most video memory (on a Steam Deck the only one)
	var best string
	var bestVRAM float64
	cards, _ := filepath.Glob(filepath.Join(root, "class", "drm", "card[0-9]*", "device"))
	for _, dev := range cards {
		if strings.Contains(filepath.Base(filepath.Dir(dev)), "-") { // card0-eDP-1 connectors
			continue
		}
		if _, ok := readNum(filepath.Join(dev, "gpu_busy_percent")); !ok {
			continue
		}
		v, _ := readNum(filepath.Join(dev, "mem_info_vram_total"))
		if best == "" || v > bestVRAM {
			best, bestVRAM = dev, v
		}
	}
	if best != "" {
		if v, ok := readNum(filepath.Join(best, "gpu_busy_percent")); ok {
			p.GPU = fptr(v)
		}
		// video memory in use: dedicated VRAM, plus GTT (system memory the GPU uses) on an APU
		// like the Steam Deck's, where the dedicated part is only the BIOS carve-out
		used, okU := readNum(filepath.Join(best, "mem_info_vram_used"))
		tot, okT := readNum(filepath.Join(best, "mem_info_vram_total"))
		if okU && okT && tot > 0 {
			if gu, ok := readNum(filepath.Join(best, "mem_info_gtt_used")); ok && tot <= 4<<30 {
				used += gu
				if gt, ok := readNum(filepath.Join(best, "mem_info_gtt_total")); ok {
					tot += gt
				}
			}
			p.VRAMUsedMB, p.VRAMMB = fptr(used/(1<<20)), fptr(tot/(1<<20))
		}
		if b, err := os.ReadFile(filepath.Join(best, "pp_dpm_sclk")); err == nil {
			if m := reCurClock.FindSubmatch(b); m != nil {
				v, _ := strconv.ParseFloat(string(m[1]), 64)
				p.GPUMHz = fptr(v)
			}
		}
		hw, _ := filepath.Glob(filepath.Join(best, "hwmon", "hwmon*"))
		for _, h := range hw {
			if v, ok := readNum(filepath.Join(h, "temp1_input")); ok && v > 0 {
				p.GPUTemp = fptr(v / 1000)
			}
			if p.GPUMHz == nil {
				if v, ok := readNum(filepath.Join(h, "freq1_input")); ok && v > 0 {
					p.GPUMHz = fptr(v / 1e6)
				}
			}
			// amdgpu's power: the package on an APU (Steam Deck: the whole APU), the card otherwise
			for _, f := range []string{"power1_average", "power1_input"} {
				if v, ok := readNum(filepath.Join(h, f)); ok && v > 0 {
					p.PowerW = fptr(v / 1e6)
					break
				}
			}
		}
	}
	// the CPU's temperature: k10temp (AMD) or coretemp (Intel) hwmon; Tctl / Package first
	hwmons, _ := filepath.Glob(filepath.Join(root, "class", "hwmon", "hwmon*"))
	for _, h := range hwmons {
		name, _ := os.ReadFile(filepath.Join(h, "name"))
		switch strings.TrimSpace(string(name)) {
		case "k10temp", "zenpower", "coretemp":
			if v, ok := readNum(filepath.Join(h, "temp1_input")); ok && v > 0 {
				p.CPUTemp = fptr(v / 1000)
			}
		}
		if p.CPUTemp != nil {
			break
		}
	}
	// the CPU's clock: the average of the cores' current frequencies
	freqs, _ := filepath.Glob(filepath.Join(root, "devices", "system", "cpu", "cpu[0-9]*", "cpufreq", "scaling_cur_freq"))
	var sum float64
	n := 0
	for _, f := range freqs {
		if v, ok := readNum(f); ok && v > 0 {
			sum += v
			n++
		}
	}
	if n > 0 {
		p.CPUMHz = fptr(sum / float64(n) / 1000)
	}
	// the battery
	bats, _ := filepath.Glob(filepath.Join(root, "class", "power_supply", "BAT*"))
	for _, b := range bats {
		if v, ok := readNum(filepath.Join(b, "capacity")); ok {
			p.BatteryPct = fptr(v)
			st, _ := os.ReadFile(filepath.Join(b, "status"))
			p.Charging = strings.TrimSpace(string(st)) == "Charging"
			if v, ok := readNum(filepath.Join(b, "power_now")); ok && v > 0 {
				p.BatteryW = fptr(v / 1e6)
			} else if cur, ok := readNum(filepath.Join(b, "current_now")); ok && cur > 0 {
				if volt, ok := readNum(filepath.Join(b, "voltage_now")); ok && volt > 0 {
					p.BatteryW = fptr(cur * volt / 1e12)
				}
			}
			break
		}
	}
}
