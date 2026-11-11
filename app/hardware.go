package main

import (
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/cpu"
)

// Hardware is what NEXUS could detect about this PC. Empty strings and zero values mean
// "not detected"; the interface shows those as unavailable instead of guessing.
type Hardware struct {
	CPU      CPUInfo     `json:"cpu"`
	GPUs     []GPUInfo   `json:"gpus"`
	Memory   MemoryInfo  `json:"memory"`
	OS       OSInfo      `json:"os"`
	Displays []Display   `json:"displays"`
	Storage  []DiskInfo  `json:"storage"`
	Vulkan   VulkanInfo  `json:"vulkan"`
	Notes    []string    `json:"notes,omitempty"` // what could not be detected and why
	At       int64       `json:"at"`
	Source   string      `json:"source"` // "detected" or "override" (developer test profiles)
}

type CPUInfo struct {
	Vendor       string   `json:"vendor"`
	Model        string   `json:"model"`
	Cores        int      `json:"cores"`   // physical cores (0 = unknown)
	Threads      int      `json:"threads"` // logical processors
	Features     []string `json:"features"`
	BaseMHz      int      `json:"baseMhz,omitempty"`
	Architecture string   `json:"architecture"`
}

type GPUInfo struct {
	Vendor        string `json:"vendor"`
	Model         string `json:"model"`
	VendorID      uint32 `json:"vendorId"`
	DeviceID      uint32 `json:"deviceId"`
	DedicatedMB   int64  `json:"dedicatedMb"` // dedicated video memory (0 = none/unknown)
	SharedMB      int64  `json:"sharedMb"`
	Integrated    bool   `json:"integrated"`
	Kind          string `json:"kind"` // discrete | integrated | software | unknown
	Driver        string `json:"driver"`
	VulkanAPI     string `json:"vulkanApi,omitempty"` // highest Vulkan version the driver offers for this GPU
	VulkanDriver  string `json:"vulkanDriver,omitempty"`
	DirectX       string `json:"directX,omitempty"`
	Primary       bool   `json:"primary"`
}

type MemoryInfo struct {
	TotalMB     int64 `json:"totalMb"`
	AvailableMB int64 `json:"availableMb"`
	UsedMB      int64 `json:"usedMb"`
	LoadPercent int   `json:"loadPercent"`
}

type OSInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Arch    string `json:"arch"`
}

type Display struct {
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	RefreshHz int    `json:"refreshHz"`
	Primary   bool   `json:"primary"`
}

type DiskInfo struct {
	Label   string `json:"label"`
	Path    string `json:"path"`
	FreeMB  int64  `json:"freeMb"`
	TotalMB int64  `json:"totalMb"`
}

type VulkanInfo struct {
	Loader    bool   `json:"loader"`    // vulkan-1.dll present
	Instance  string `json:"instance"`  // loader / instance version
	Available bool   `json:"available"` // at least one Vulkan GPU
}

// x86 instruction-set extensions that matter for the engine's CPU JIT and its decoders.
func cpuFeatures() []string {
	var f []string
	add := func(ok bool, n string) {
		if ok {
			f = append(f, n)
		}
	}
	switch runtime.GOARCH {
	case "amd64", "386":
		add(cpu.X86.HasSSE41, "SSE4.1")
		add(cpu.X86.HasSSE42, "SSE4.2")
		add(cpu.X86.HasAVX, "AVX")
		add(cpu.X86.HasAVX2, "AVX2")
		add(cpu.X86.HasFMA, "FMA3")
		add(cpu.X86.HasBMI2, "BMI2")
		add(cpu.X86.HasAES, "AES-NI")
		add(cpu.X86.HasPCLMULQDQ, "CLMUL")
		add(cpu.X86.HasAVX512F, "AVX-512F")
		add(cpu.X86.HasAVX512VL, "AVX-512VL")
	case "arm64":
		add(cpu.ARM64.HasATOMICS, "LSE atomics")
		add(cpu.ARM64.HasAES, "AES")
		add(cpu.ARM64.HasSHA2, "SHA2")
	}
	return f
}

func (h *Hardware) PrimaryGPU() *GPUInfo {
	for i := range h.GPUs {
		if h.GPUs[i].Primary {
			return &h.GPUs[i]
		}
	}
	if len(h.GPUs) > 0 {
		return &h.GPUs[0]
	}
	return nil
}

func (h *Hardware) PrimaryDisplay() *Display {
	for i := range h.Displays {
		if h.Displays[i].Primary {
			return &h.Displays[i]
		}
	}
	if len(h.Displays) > 0 {
		return &h.Displays[0]
	}
	return nil
}

func gpuVendorName(id uint32) string {
	switch id {
	case 0x10DE:
		return "NVIDIA"
	case 0x1002, 0x1022:
		return "AMD"
	case 0x8086:
		return "Intel"
	case 0x5143:
		return "Qualcomm"
	case 0x1414:
		return "Microsoft"
	case 0x13B5:
		return "ARM"
	}
	return ""
}

func vkVersionString(v uint32) string {
	if v == 0 {
		return ""
	}
	return itoa(int(v>>22&0x7F)) + "." + itoa(int(v>>12&0x3FF)) + "." + itoa(int(v&0xFFF))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}

// ---------- cache

var hwMu sync.Mutex
var hwCache *Hardware

// DetectHardware returns the detected hardware (cached for a minute; refresh=true re-reads).
// NEXUS_HARDWARE=<file.json> replaces detection with a saved profile (developer testing of the
// optimiser on other machines' specifications); the result is then marked "override".
func (a *App) DetectHardware(refresh bool) Hardware {
	hwMu.Lock()
	defer hwMu.Unlock()
	if hwCache != nil && !refresh && time.Since(time.Unix(hwCache.At, 0)) < time.Minute {
		return *hwCache
	}
	var h Hardware
	if p := os.Getenv("NEXUS_HARDWARE"); p != "" {
		if data, err := os.ReadFile(p); err == nil && json.Unmarshal(data, &h) == nil {
			h.Source = "override"
		}
	}
	if h.Source == "" {
		h = detectHardware()
		h.Source = "detected"
		for _, p := range []struct{ label, path string }{{"NEXUS folder", a.Paths.Root}, {"NEXUS data", a.Paths.Data}} {
			free, total := diskSpace(p.path)
			if total > 0 {
				h.Storage = append(h.Storage, DiskInfo{Label: p.label, Path: p.path, FreeMB: int64(free >> 20), TotalMB: int64(total >> 20)})
			}
		}
	}
	if h.CPU.Threads == 0 {
		h.CPU.Threads = runtime.NumCPU()
	}
	if h.GPUs == nil {
		h.GPUs = []GPUInfo{}
	}
	if h.Displays == nil {
		h.Displays = []Display{}
	}
	h.At = time.Now().Unix()
	hwCache = &h
	return h
}
