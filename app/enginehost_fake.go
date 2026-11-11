package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

// fakeEngine stands in for nexus_engine.dll in development/tests (non-Windows only). It follows
// the same state machine as the real engine and renders a test tone through the engine's gain
// curve so the volume path can be checked end to end. It never runs a game.
type fakeEngine struct {
	mu      sync.Mutex
	st      EngineStatus
	req     EngineRequest
	stop    chan bool
	volume  int
	samples []int16 // last audio buffer "played"
}

func newFakeEngine() *fakeEngine { return &fakeEngine{st: EngineStatus{State: "idle"}, volume: 100} }

func (f *fakeEngine) Info() (EngineBuildInfo, error) {
	return EngineBuildInfo{Name: "NEXUS Engine", Version: "test stand-in (not an emulator)", API: engineAPIVersion,
		Upstream: map[string]string{"project": "development stand-in"},
		Systems:   []EngineSystem{{ID: "switch1", Name: "Nintendo Switch", Status: "core"}, {ID: "switch2", Name: "Nintendo Switch 2", Status: "experimental"}},
		Renderers: []string{"null"}, Components: []string{"test tone audio through the engine gain curve"}}, nil
}

// fake behaviour comes from the game file name: "crash" -> error at load, "bad" -> load error
func (f *fakeEngine) Start(req EngineRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.st.State == "starting" || f.st.State == "running" || f.st.State == "stopping" {
		return errors.New("a game is already running")
	}
	if req.Game == "" || req.ConfigFile == "" || req.DataDir == "" {
		return errors.New("the engine refused the launch request")
	}
	f.req, f.volume = req, req.Volume
	f.st = EngineStatus{State: "starting", Volume: req.Volume}
	f.stop = make(chan bool, 1)
	go f.run(req)
	return nil
}

func (f *fakeEngine) run(req EngineRequest) {
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(req.Game, "bad") {
		f.set(func(s *EngineStatus) { *s = EngineStatus{State: "error", Code: "load", Message: "The game could not be loaded."} })
		return
	}
	if _, err := os.Stat(req.ConfigFile); err != nil {
		f.set(func(s *EngineStatus) { *s = EngineStatus{State: "error", Code: "config", Message: "session configuration missing"} })
		return
	}
	sys := "switch1"
	if strings.Contains(req.Game, "nx2") {
		sys = "switch2"
	}
	f.set(func(s *EngineStatus) { s.State = "running"; s.System = sys; s.ProgramID = "0100000000010000" })
	dur := 2 * time.Second
	if d := os.Getenv("NEXUS_FAKE_PLAY"); d != "" {
		if v, err := time.ParseDuration(d); err == nil {
			dur = v
		}
	}
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	end := time.After(dur)
	for {
		select {
		case <-f.stop:
			f.set(func(s *EngineStatus) { s.State = "stopped" })
			return
		case <-end:
			f.set(func(s *EngineStatus) { s.State = "stopped" }) // game exited itself
			return
		case <-t.C:
			f.mu.Lock()
			if f.st.Suspended {
				f.mu.Unlock()
				continue
			}
			f.st.Frames += 3
			f.samples = toneBuffer(f.volume)
			f.mu.Unlock()
			if strings.Contains(req.Game, "crash") && f.Status().Frames > 15 {
				f.set(func(s *EngineStatus) { s.State = "error"; s.Code = "crash"; s.Message = "The game stopped unexpectedly." })
				return
			}
		}
	}
}

// softLimit is the engine's limiter (src/audio_core/sink/nexus_gain.h): unchanged below a knee
// at 80 % of full scale, then a tanh curve that approaches but never exceeds full scale.
func softLimit(x float64) float64 {
	knee, fs := 0.8*32767.0, 32767.0
	if a := math.Abs(x); a > knee {
		return math.Copysign(knee+(fs-knee)*math.Tanh((a-knee)/(fs-knee)), x)
	}
	return x
}

// toneBuffer: a 440 Hz tone at 75 % of full scale through the engine's gain curve.
func toneBuffer(volume int) []int16 {
	g := math.Max(0, math.Min(200, float64(volume))) / 100
	out := make([]int16, 480)
	for i := range out {
		x := 0.75 * 32767 * math.Sin(2*math.Pi*440*float64(i)/48000) * g
		if g > 1 {
			x = softLimit(x)
		}
		out[i] = int16(math.Round(math.Max(-32768, math.Min(32767, x))))
	}
	return out
}

func (f *fakeEngine) set(fn func(*EngineStatus)) {
	f.mu.Lock()
	fn(&f.st)
	f.mu.Unlock()
}

func (f *fakeEngine) Status() EngineStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.st
	s.Volume = f.volume
	return s
}

func (f *fakeEngine) Stop(force bool) {
	f.mu.Lock()
	ch := f.stop
	f.mu.Unlock()
	if ch != nil {
		select {
		case ch <- force:
		default:
		}
	}
}

func (f *fakeEngine) SetVolume(p int) {
	f.mu.Lock()
	f.volume = max(0, min(200, p))
	f.mu.Unlock()
}
func (f *fakeEngine) Focus() {}

// Suspend: like the engine, frames stop advancing while suspended and the session stays loaded.
func (f *fakeEngine) Suspend(on bool) {
	f.mu.Lock()
	if f.st.State == "running" {
		f.st.Suspended = on
	}
	f.mu.Unlock()
}

// Peak of the last played buffer (tests / dev UI).
func (f *fakeEngine) Peak() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := 0
	for _, s := range f.samples {
		if v := int(s); v > p {
			p = v
		} else if -v > p {
			p = -v
		}
	}
	return p
}

var _ = json.Marshal
