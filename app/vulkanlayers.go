package main

import (
	"path/filepath"
	"strings"
)

// Implicit Vulkan layers are installed by other programs (overlays, recorders, mod managers) and
// load into every Vulkan application, including the NEXUS engine. An outdated or faulty layer
// can crash games or break rendering even though the graphics driver is fine; emulator projects
// have traced crashes to such layers (an outdated Overwolf layer is the documented case). NEXUS
// lists the enabled implicit layers in its hardware report and warns about known problem ones.

// VulkanLayer is one enabled implicit layer (its manifest file).
type VulkanLayer struct {
	Manifest string `json:"manifest"`
	Problem  string `json:"problem,omitempty"`
}

// knownBadLayers: a part of the manifest path (lower case) and what to tell the user.
var knownBadLayers = []struct{ match, problem string }{
	{"overwolf", "Overwolf's Vulkan layer is known to make Vulkan games crash or render incorrectly. Update or uninstall Overwolf if games fail to start."},
}

func assessVulkanLayers(manifests []string) []VulkanLayer {
	var out []VulkanLayer
	for _, m := range manifests {
		l := VulkanLayer{Manifest: m}
		low := strings.ToLower(filepath.ToSlash(m))
		for _, b := range knownBadLayers {
			if strings.Contains(low, b.match) {
				l.Problem = b.problem
			}
		}
		out = append(out, l)
	}
	return out
}
