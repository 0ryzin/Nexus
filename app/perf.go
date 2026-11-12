package main

// NEXUS Performance Monitor: the metrics shown in the in-game overlay (overlay_windows.go) and
// in the preview in Settings. Every value is measured; what this PC cannot measure is reported as
// unavailable, never estimated:
//   - FPS: frames the engine presented in the last second (from its frame counter);
//     frame time: the average time per presented frame over that second.
//   - CPU utilisation: whole-system, from the OS's CPU times.
//   - GPU utilisation and GPU memory in use: Windows' GPU performance counters (all vendors).
//   - GPU temperature: NVIDIA cards through the driver's NVML library; others: unavailable.
//   - CPU temperature: unavailable (Windows offers no reliable sensor API to applications).
//   - RAM in use / total, and the configured memory speed (SMBIOS), which is a speed, not usage.
// Sampling happens only while the overlay or the preview is shown, at most twice a second.

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type PerfSample struct {
	At         int64    `json:"at"`
	FPS        *float64 `json:"fps"`         // presented frames per second (nil: no game)
	FrameMs    *float64 `json:"frameMs"`     // average frame time over the last second
	CPU        *float64 `json:"cpu"`         // % of all cores
	CPUTemp    *float64 `json:"cpuTemp"`     // °C
	GPU        *float64 `json:"gpu"`         // % (3D engine, busiest adapter)
	GPUTemp    *float64 `json:"gpuTemp"`     // °C
	VRAMUsedMB *float64 `json:"vramUsedMb"`  // dedicated GPU memory in use
	VRAMMB     *float64 `json:"vramMb"`      // dedicated GPU memory size
	RAMUsedMB  *float64 `json:"ramUsedMb"`   // system memory in use
	RAMMB      *float64 `json:"ramMb"`       // installed system memory
	RAMSpeed   *float64 `json:"ramSpeedMts"` // configured memory speed, MT/s
	Notes      []string `json:"notes,omitempty"`
}

var PerfMetrics = []EmuChoice{{"fps", "FPS"}, {"frametime", "Frame time"}, {"cpu", "CPU usage"}, {"cputemp", "CPU temperature"},
	{"gpu", "GPU usage"}, {"gputemp", "GPU temperature"}, {"vram", "GPU memory"}, {"ram", "RAM usage"}, {"ramspeed", "RAM speed"}}

type perfSampler struct {
	mu         sync.Mutex
	lastFrames uint64
	lastAt     time.Time
	fps        float64
	haveFPS    bool
	sys        sysSampler // platform part (perf_windows.go / perf_other.go)
	last       PerfSample
	lastTaken  time.Time
}

var perf = &perfSampler{}

func fptr(v float64) *float64 { return &v }

// Sample returns current metrics (cached for 400 ms so several viewers cost one measurement).
func (p *perfSampler) Sample(e *Emulator) PerfSample {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.lastTaken) < 400*time.Millisecond {
		return p.last
	}
	s := PerfSample{At: time.Now().UnixMilli()}
	// FPS from the engine's presented-frame counter
	if st := e.State(); st.Running && st.Phase == "playing" && !st.Suspended {
		if h := e.hostIfLoaded(); h != nil {
			es := h.Status()
			now := time.Now()
			if !p.lastAt.IsZero() && es.Frames >= p.lastFrames {
				dt := now.Sub(p.lastAt).Seconds()
				if dt > 0.2 {
					p.fps, p.haveFPS = float64(es.Frames-p.lastFrames)/dt, true
					p.lastFrames, p.lastAt = es.Frames, now
				}
			} else {
				p.lastFrames, p.lastAt = es.Frames, now
			}
			if p.haveFPS {
				s.FPS = fptr(p.fps)
				if p.fps > 0 {
					s.FrameMs = fptr(1000 / p.fps)
				}
			}
		}
	} else {
		p.lastAt, p.haveFPS = time.Time{}, false
	}
	p.sys.fill(&s)
	p.last, p.lastTaken = s, time.Now()
	return s
}

// hostIfLoaded returns the engine host without loading it.
func (e *Emulator) hostIfLoaded() EngineHost {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.host
}

// formatPerf renders the chosen metrics as one line; unavailable values read "N/A".
func formatPerf(s PerfSample, metrics []string, sep string) string {
	na := "N/A"
	f := func(v *float64, format string) string {
		if v == nil {
			return na
		}
		return fmt.Sprintf(format, *v)
	}
	var parts []string
	for _, m := range metrics {
		switch m {
		case "fps":
			parts = append(parts, "FPS "+f(s.FPS, "%.0f"))
		case "frametime":
			parts = append(parts, "Frame "+f(s.FrameMs, "%.1f ms"))
		case "cpu":
			parts = append(parts, "CPU "+f(s.CPU, "%.0f%%"))
		case "cputemp":
			parts = append(parts, "CPU "+f(s.CPUTemp, "%.0f °C"))
		case "gpu":
			parts = append(parts, "GPU "+f(s.GPU, "%.0f%%"))
		case "gputemp":
			parts = append(parts, "GPU "+f(s.GPUTemp, "%.0f °C"))
		case "vram":
			v := na
			if s.VRAMUsedMB != nil {
				v = fmt.Sprintf("%.1f", *s.VRAMUsedMB/1024)
				if s.VRAMMB != nil {
					v += fmt.Sprintf(" / %.0f", *s.VRAMMB/1024)
				}
				v += " GB"
			}
			parts = append(parts, "VRAM "+v)
		case "ram":
			v := na
			if s.RAMUsedMB != nil && s.RAMMB != nil {
				v = fmt.Sprintf("%.1f / %.0f GB", *s.RAMUsedMB/1024, *s.RAMMB/1024)
			}
			parts = append(parts, "RAM "+v)
		case "ramspeed":
			parts = append(parts, "RAM "+f(s.RAMSpeed, "%.0f MT/s"))
		}
	}
	return strings.Join(parts, sep)
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------- the in-game HUD's content and geometry (drawn by overlay_windows.go; shared so it is
// tested on every platform and shown the same way in the Settings preview)

// PerfSeg is one reading in the HUD: a short coloured label, the value, its unit.
type PerfSeg struct {
	Kind  string `json:"kind"` // fps | frame | cpu | gpu | mem | temp
	Label string `json:"label"`
	Value string `json:"value"`
	Unit  string `json:"unit,omitempty"`
}

// perfSegments: the chosen metrics, in order; temperatures are attached to their CPU/GPU reading
// when both are chosen (as one "CPU 42% 61°C" group). Unmeasurable values read "N/A".
func perfSegments(s PerfSample, metrics []string) []PerfSeg {
	num := func(v *float64, format string) string {
		if v == nil {
			return "N/A"
		}
		return fmt.Sprintf(format, *v)
	}
	unit := func(v *float64, u string) string {
		if v == nil {
			return ""
		}
		return u
	}
	has := map[string]bool{}
	for _, m := range metrics {
		has[m] = true
	}
	var out []PerfSeg
	for _, m := range metrics {
		switch m {
		case "fps":
			out = append(out, PerfSeg{"fps", "FPS", num(s.FPS, "%.0f"), ""})
		case "frametime":
			out = append(out, PerfSeg{"frame", "FRAME", num(s.FrameMs, "%.1f"), unit(s.FrameMs, "ms")})
		case "cpu":
			out = append(out, PerfSeg{"cpu", "CPU", num(s.CPU, "%.0f"), unit(s.CPU, "%")})
			if has["cputemp"] && s.CPUTemp != nil {
				out = append(out, PerfSeg{"temp", "", num(s.CPUTemp, "%.0f"), "°C"})
			}
		case "cputemp":
			if !has["cpu"] || s.CPUTemp == nil {
				out = append(out, PerfSeg{"temp", "CPU", num(s.CPUTemp, "%.0f"), unit(s.CPUTemp, "°C")})
			}
		case "gpu":
			out = append(out, PerfSeg{"gpu", "GPU", num(s.GPU, "%.0f"), unit(s.GPU, "%")})
			if has["gputemp"] && s.GPUTemp != nil {
				out = append(out, PerfSeg{"temp", "", num(s.GPUTemp, "%.0f"), "°C"})
			}
		case "gputemp":
			if !has["gpu"] || s.GPUTemp == nil {
				out = append(out, PerfSeg{"temp", "GPU", num(s.GPUTemp, "%.0f"), unit(s.GPUTemp, "°C")})
			}
		case "vram":
			v, u := "N/A", ""
			if s.VRAMUsedMB != nil {
				v, u = fmt.Sprintf("%.1f", *s.VRAMUsedMB/1024), "GB"
				if s.VRAMMB != nil {
					u = fmt.Sprintf("/ %.0f GB", *s.VRAMMB/1024)
				}
			}
			out = append(out, PerfSeg{"mem", "VRAM", v, u})
		case "ram":
			v, u := "N/A", ""
			if s.RAMUsedMB != nil && s.RAMMB != nil {
				v, u = fmt.Sprintf("%.1f", *s.RAMUsedMB/1024), fmt.Sprintf("/ %.0f GB", *s.RAMMB/1024)
			}
			out = append(out, PerfSeg{"mem", "RAM", v, u})
		case "ramspeed":
			out = append(out, PerfSeg{"mem", "RAM", num(s.RAMSpeed, "%.0f"), unit(s.RAMSpeed, "MT/s")})
		}
	}
	return out
}

// pxRect is a rectangle in screen pixels.
type pxRect struct{ L, T, R, B int32 }

func (r pxRect) W() int32 { return r.R - r.L }
func (r pxRect) H() int32 { return r.B - r.T }

// gameImageRect is where the game's picture is inside the area the game is drawn in: the engine
// keeps the aspect ratio (16:9 by default) and fills the rest with black bars, so the HUD sits
// on the picture, not on a bar. "stretch" fills the whole area.
func gameImageRect(area pxRect, aspect string) pxRect {
	ratios := map[string][2]int32{"16:9": {16, 9}, "4:3": {4, 3}, "21:9": {21, 9}, "16:10": {16, 10}}
	if aspect == "stretch" || area.W() <= 0 || area.H() <= 0 {
		return area
	}
	r, ok := ratios[aspect]
	if !ok {
		r = ratios["16:9"]
	}
	w, h := area.W(), area.H()
	if int64(w)*int64(r[1]) > int64(h)*int64(r[0]) { // wider than the picture: bars left and right
		iw := int32(int64(h) * int64(r[0]) / int64(r[1]))
		x := area.L + (w-iw)/2
		return pxRect{x, area.T, x + iw, area.B}
	}
	ih := int32(int64(w) * int64(r[1]) / int64(r[0])) // taller: bars top and bottom
	y := area.T + (h-ih)/2
	return pxRect{area.L, y, area.R, y + ih}
}

// hudBarHeight: the strip's height follows the picture's height (about 3.2 %, 22-48 px, like a
// handheld's overlay at 800p and a TV at 4K), times the user's size setting.
func hudBarHeight(img pxRect, scalePct int) int32 {
	base := clampF(float64(img.H())*0.032, 22, 48)
	return int32(base * clampF(float64(scalePct), 75, 200) / 100)
}

// hudAspect is the running game's aspect ratio setting (per game, else global, else 16:9).
func hudAspect(a *App, gameKey string) string {
	if v := a.Library.GP.Get(gameKey).Settings["graphics.aspect"]; v != "" {
		return v
	}
	if v := a.Settings.Get().Engine["graphics.aspect"]; v != "" {
		return v
	}
	return "16:9"
}
