package main

// NVIDIA Smooth Motion on Linux: the NVIDIA driver's own frame generation for Vulkan programs
// (driver 575 and later, GeForce RTX 40 and 50 series). The driver's implicit Vulkan layer
// VK_LAYER_NV_present turns it on for a process that has NVPRESENT_ENABLE_SMOOTH_MOTION=1 in its
// environment when it creates its Vulkan instance. The NEXUS engine runs inside the NEXUS
// process, so NEXUS sets the variable for a session that asks for it, and clears it otherwise.
//
// NEXUS cannot see whether the driver applies it (the layer does not report to the program), so
// it says "requested", never "running". It replaces the built-in frame generation for that
// session: NVIDIA says the two must not be combined. On Windows Smooth Motion is turned on in
// the NVIDIA app, outside NEXUS.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const smoothMotionEnv = "NVPRESENT_ENABLE_SMOOTH_MOTION"

var smoothMotionModel = regexp.MustCompile(`(?i)\bRTX\s*(PRO\s*)?[45]0[0-9]0\b|\bRTX\s*(PRO\s*)?[0-9]{4}\s*Blackwell`)

// smoothMotionLayerDirs: where the NVIDIA driver installs its implicit Vulkan layer manifests.
var smoothMotionLayerDirs = []string{"/usr/share/vulkan/implicit_layer.d", "/etc/vulkan/implicit_layer.d", "/usr/local/share/vulkan/implicit_layer.d"}

func smoothMotionLayerInstalled() bool {
	for _, d := range smoothMotionLayerDirs {
		m, _ := filepath.Glob(filepath.Join(d, "*.json"))
		for _, p := range m {
			if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), "VK_LAYER_NV_present") {
				return true
			}
		}
	}
	return false
}

// smoothMotionSupport: whether this PC can ask the driver for Smooth Motion, and if not, why.
func smoothMotionSupport(h Hardware) (bool, string) {
	if runtime.GOOS != "linux" {
		return false, "On Windows, NVIDIA Smooth Motion is turned on in the NVIDIA app, not in NEXUS."
	}
	var nv *GPUInfo
	for i := range h.GPUs {
		if h.GPUs[i].VendorID == vendorNVIDIA {
			nv = &h.GPUs[i]
		}
	}
	switch {
	case nv == nil:
		return false, "NVIDIA Smooth Motion needs an NVIDIA GeForce RTX 40 or 50 series graphics card."
	case nv.Driver != "nvidia":
		return false, "NVIDIA Smooth Motion needs NVIDIA's own Linux driver (575 or newer), not " + firstNonEmpty(nv.Driver, "this driver") + "."
	case !smoothMotionModel.MatchString(nv.Model):
		return false, "NVIDIA Smooth Motion needs a GeForce RTX 40 or 50 series graphics card (" + nv.Model + ")."
	case !smoothMotionLayerInstalled():
		return false, "This NVIDIA driver has no Smooth Motion layer (VK_LAYER_NV_present); it needs driver 575 or newer."
	}
	return true, ""
}

// applySmoothMotion sets or clears the driver's variable for the session about to start.
func applySmoothMotion(on bool) {
	if on {
		_ = os.Setenv(smoothMotionEnv, "1")
	} else {
		_ = os.Unsetenv(smoothMotionEnv)
	}
}
