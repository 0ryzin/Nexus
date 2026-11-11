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
