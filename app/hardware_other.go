//go:build !windows

package main

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Non-Windows detection: /proc for CPU and memory, /etc/os-release; on Linux the graphics cards
// and displays from sysfs (hardware_sysfs.go).
func detectHardware() Hardware {
	h := Hardware{}
	h.CPU.Architecture = runtime.GOARCH
	h.CPU.Threads = runtime.NumCPU()
	h.CPU.Features = cpuFeatures()
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		sc := bufio.NewScanner(f)
		cores := map[string]bool{}
		phys := ""
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), ":")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			switch k {
			case "vendor_id":
				h.CPU.Vendor = map[string]string{"GenuineIntel": "Intel", "AuthenticAMD": "AMD"}[v]
				if h.CPU.Vendor == "" {
					h.CPU.Vendor = v
				}
			case "model name":
				h.CPU.Model = v
			case "physical id":
				phys = v
			case "core id":
				cores[phys+"/"+v] = true
			case "cpu MHz":
				if mhz, err := strconv.ParseFloat(v, 64); err == nil && h.CPU.BaseMHz == 0 {
					h.CPU.BaseMHz = int(mhz)
				}
			}
		}
		f.Close()
		h.CPU.Cores = len(cores)
	}
	if f, err := os.Open("/proc/meminfo"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) < 2 {
				continue
			}
			kb, _ := strconv.ParseInt(fs[1], 10, 64)
			switch fs[0] {
			case "MemTotal:":
				h.Memory.TotalMB = kb >> 10
			case "MemAvailable:":
				h.Memory.AvailableMB = kb >> 10
			}
		}
		f.Close()
		if h.Memory.TotalMB > 0 {
			h.Memory.UsedMB = h.Memory.TotalMB - h.Memory.AvailableMB
			h.Memory.LoadPercent = int(h.Memory.UsedMB * 100 / h.Memory.TotalMB)
		}
	}
	h.OS.Arch = runtime.GOARCH
	h.OS.Name = runtime.GOOS
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
				h.OS.Name = strings.Trim(v, `"`)
			}
		}
	}
	if runtime.GOOS == "linux" {
		h.GPUs = sysfsGPUs("/sys/class/drm", pciIDs())
		h.Displays = sysfsDisplays("/sys/class/drm")
		h.Vulkan = sysfsVulkan(h.GPUs)
		if len(h.GPUs) == 0 {
			h.Notes = append(h.Notes, "No graphics card was found in /sys/class/drm.")
		}
		return h
	}
	h.Notes = append(h.Notes, "Graphics card and display details are read through Windows (DXGI, Vulkan, display settings) and are not available on this system.")
	return h
}
