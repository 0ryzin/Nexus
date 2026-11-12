package main

// Which graphics card the engine renders on.
//
// The engine renders on the Vulkan physical device whose index is in its "vulkan_device"
// setting (0 by default). On PCs with two graphics processors (most gaming laptops: an Intel or
// AMD integrated GPU plus an NVIDIA or AMD discrete GPU) the Vulkan loader often lists the
// integrated GPU first, so index 0 means the slow GPU. Emulator programs avoid this by exporting
// NvOptimusEnablement / AmdPowerXpressRequestHighPerformance from their .exe, which the drivers
// read from the program that starts; NEXUS loads the engine into NEXUS.exe, so those exports in
// the engine library have no effect.
//
// NEXUS therefore chooses the device itself: it lists the Vulkan devices in its own process
// (the same loader and the same order the engine sees, vulkan_windows.go), picks one by the
// user's preference and writes its index into the session configuration.

import (
	"sort"
	"strings"
)

// VulkanDevice is one physical device as the Vulkan loader lists it.
type VulkanDevice struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Type     string `json:"type"` // discrete | integrated | virtual | cpu | other
	VendorID uint32 `json:"vendorId"`
	DeviceID uint32 `json:"deviceId"`
	API      string `json:"api"`     // "1.3.280"
	LocalMB  uint64 `json:"localMb"` // device-local memory
}

// vulkanDevices is listVulkanDevices (vulkan_windows.go); tests replace it.
var vulkanDevices = listVulkanDevices

// GPU preferences (setting graphics.gpu).
const (
	gpuAuto       = "auto"       // the fastest card: discrete before integrated
	gpuIntegrated = "integrated" // the integrated GPU (longer battery life)
)

// ChooseVulkanDevice returns the device to render on and why, or -1 when there is nothing to
// choose (one device or none: the engine's own default is right).
func ChooseVulkanDevice(devs []VulkanDevice, pref string) (int, string) {
	usable := make([]VulkanDevice, 0, len(devs))
	for _, d := range devs {
		if d.Type != "cpu" { // software rasterisers (llvmpipe, SwiftShader) are never chosen
			usable = append(usable, d)
		}
	}
	if len(usable) == 0 {
		return -1, ""
	}
	if len(usable) == 1 && len(devs) == 1 {
		return -1, ""
	}
	// a specific card the user chose, by name
	if pref != "" && pref != gpuAuto && pref != gpuIntegrated {
		for _, d := range usable {
			if strings.EqualFold(d.Name, pref) {
				return d.Index, "chosen in settings"
			}
		}
		pref = gpuAuto // that card is gone (new PC, new driver): fall back
	}
	rank := map[string]int{"discrete": 0, "integrated": 1, "virtual": 2, "other": 3}
	if pref == gpuIntegrated {
		rank = map[string]int{"integrated": 0, "discrete": 1, "virtual": 2, "other": 3}
	}
	sort.SliceStable(usable, func(i, j int) bool {
		ri, rj := rank[usable[i].Type], rank[usable[j].Type]
		if ri != rj {
			return ri < rj
		}
		return usable[i].LocalMB > usable[j].LocalMB // the larger card of the same kind
	})
	best := usable[0]
	why := "the fastest graphics card"
	if pref == gpuIntegrated && best.Type == "integrated" {
		why = "integrated graphics, as set"
	}
	return best.Index, why
}
