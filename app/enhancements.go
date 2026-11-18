package main

// Frame generation and upscaler replacement (DLSS / FSR 4).
//
// The NEXUS engine (Lichii lineage) already contains these features; NEXUS exposes them as
// settings and decides, from the detected graphics card, which ones this PC can use:
//
//   - Frame generation (Vulkan only): the engine's own optical-flow frame interpolation (vendor-neutral Vulkan compute, not AMD FidelityFX code), usable
//     on any Vulkan GPU, or Lossless Scaling's frame generation, which needs the user's own copy of
//     Lossless.dll (from the Lossless Scaling app) in <engine data>\lossless.
//   - NVIDIA DLSS in place of a game's own DLSS (Switch 2 games that use DLSS): needs a GeForce RTX
//     (or RTX professional) card and NVIDIA's Streamline loader (sl.interposer.dll + plugins) in
//     <engine data>\streamline or <NEXUS>\streamline. The DLSS model itself may come from the NVIDIA
//     app's downloads, which the engine finds on its own.
//   - AMD FSR 4 in place of FSR 3.1 where FSR replaces a game's upscaler: needs an RDNA 4 card
//     (Radeon RX 9000 series) and AMD's driver.
//
// Nothing of NVIDIA's, AMD's or Lossless Scaling's is shipped with NEXUS.

import (
	"path/filepath"
	"regexp"
	"strings"
)

const (
	vendorNVIDIA = 0x10DE
	vendorAMD    = 0x1002
)

type FeatureState struct {
	Supported bool   `json:"supported"`        // this PC's graphics card can use it
	Ready     bool   `json:"ready"`            // everything it needs is in place
	Why       string `json:"why,omitempty"`    // why not, or what is missing
	Detail    string `json:"detail,omitempty"` // e.g. where files were found
}

type Enhancements struct {
	GPU         string       `json:"gpu"`
	FrameGen    FeatureState `json:"frameGen"`
	Lossless    FeatureState `json:"lossless"`
	DLSS        FeatureState `json:"dlss"`
	FSR4        FeatureState `json:"fsr4"`
	LosslessDir string       `json:"losslessDir"`
	DLSSDir     string       `json:"dlssDir"`
}

var rtxModel = regexp.MustCompile(`(?i)\bRTX\b`)
var rdna4Model = regexp.MustCompile(`(?i)\bRX\s*9\d{3}\b|Radeon AI PRO R97`)

// dlssCapable: DLSS runs on NVIDIA's RTX cards (tensor cores): GeForce RTX 20 series and newer,
// and the RTX professional cards. GTX, MX and older cards cannot run it.
func dlssCapable(g GPUInfo) bool {
	return g.VendorID == vendorNVIDIA && rtxModel.MatchString(g.Model)
}

// fsr4Capable: FSR 4 needs AMD's RDNA 4 (Radeon RX 9000 series).
func fsr4Capable(g GPUInfo) bool {
	return g.VendorID == vendorAMD && rdna4Model.MatchString(g.Model)
}

// gameGPU is the card games run on: the primary one, else the first discrete one, else any.
func gameGPU(h Hardware) *GPUInfo {
	for i := range h.GPUs {
		if h.GPUs[i].Primary && h.GPUs[i].Kind != "software" {
			return &h.GPUs[i]
		}
	}
	for i := range h.GPUs {
		if h.GPUs[i].Kind == "discrete" {
			return &h.GPUs[i]
		}
	}
	for i := range h.GPUs {
		if h.GPUs[i].Kind != "software" {
			return &h.GPUs[i]
		}
	}
	return nil
}

func (e *Emulator) losslessDir() string   { return filepath.Join(e.DataDir(), "lossless") }
func (e *Emulator) streamlineDir() string { return filepath.Join(e.DataDir(), "streamline") }

// Enhancements reports what this PC can use and what is still missing.
func (e *Emulator) Enhancements() Enhancements {
	h := e.app.DetectHardware(false)
	out := Enhancements{LosslessDir: e.losslessDir(), DLSSDir: e.streamlineDir()}
	var dlss, fsr4 bool
	var any *GPUInfo
	for i := range h.GPUs {
		g := h.GPUs[i]
		dlss = dlss || dlssCapable(g)
		fsr4 = fsr4 || fsr4Capable(g)
	}
	any = gameGPU(h)
	if any != nil {
		out.GPU = strings.TrimSpace(any.Vendor + " " + any.Model)
		if strings.HasPrefix(strings.ToLower(any.Model), strings.ToLower(any.Vendor)) {
			out.GPU = any.Model
		}
	}
	gpuName := out.GPU
	if gpuName == "" {
		gpuName = "no graphics card was detected"
	}

	// built-in frame generation: any Vulkan-capable GPU
	vulkan := h.Vulkan.Available
	for _, g := range h.GPUs {
		vulkan = vulkan || g.VulkanAPI != ""
	}
	out.FrameGen = FeatureState{Supported: vulkan, Ready: vulkan}
	if !vulkan {
		out.FrameGen.Why = "Frame generation needs a graphics card with Vulkan support (" + gpuName + ")."
	}

	// Lossless Scaling frame generation: the user's own Lossless.dll
	ll := filepath.Join(e.losslessDir(), "Lossless.dll")
	out.Lossless = FeatureState{Supported: vulkan, Ready: vulkan && fileExists(ll)}
	switch {
	case !vulkan:
		out.Lossless.Why = out.FrameGen.Why
	case !fileExists(ll):
		out.Lossless.Why = "Needs Lossless.dll from your own copy of Lossless Scaling (Steam). Use “Set up Lossless Scaling”."
	default:
		out.Lossless.Detail = ll
	}

	// DLSS in place of a game's DLSS
	out.DLSS = FeatureState{Supported: dlss}
	if !dlss {
		out.DLSS.Why = "DLSS needs an NVIDIA GeForce RTX graphics card (" + gpuName + ")."
	} else if missing := e.dlssMissing(); len(missing) > 0 {
		out.DLSS.Why = "Needs NVIDIA's DLSS files (" + strings.Join(missing, ", ") + "). NEXUS can download them from NVIDIA in Frame generation & DLSS."
	} else {
		out.DLSS.Ready, out.DLSS.Detail = true, e.streamlineDir()
	}

	out.FSR4 = FeatureState{Supported: fsr4, Ready: fsr4}
	if !fsr4 {
		out.FSR4.Why = "FSR 4 needs an AMD Radeon RX 9000 series graphics card (" + gpuName + ")."
	}
	return out
}

// findStreamline: where the engine will find sl.interposer.dll (same search order as the engine).
func (e *Emulator) findStreamline() string {
	for _, d := range []string{e.streamlineDir(), filepath.Join(e.app.Paths.Root, "streamline")} {
		if fileExists(filepath.Join(d, "sl.interposer.dll")) {
			return d
		}
	}
	return ""
}

// applyHardwareCaps marks the settings this PC cannot use as not available, with the reason.
func (e *Emulator) applyHardwareCaps(caps map[string]Support) {
	en := e.Enhancements()
	off := func(ids []string, why string) {
		for _, id := range ids {
			c := caps[id]
			c.Available, c.Note = false, why
			caps[id] = c
		}
	}
	if !en.FrameGen.Supported {
		off([]string{"enhance.framegen", "enhance.framegen_type", "enhance.framegen_mult", "enhance.framegen_target"}, en.FrameGen.Why)
	} else if !en.Lossless.Ready {
		c := caps["enhance.framegen_type"]
		c.Choices = []string{"fsr3"}
		c.Note = "Lossless Scaling needs your own Lossless.dll (see above)."
		caps["enhance.framegen_type"] = c
	}
	if !en.DLSS.Supported {
		off([]string{"enhance.dlss", "enhance.dlss_quality", "enhance.dlss_model"}, en.DLSS.Why)
	}
	if !en.FSR4.Supported {
		off([]string{"enhance.fsr4"}, en.FSR4.Why)
	}
}
