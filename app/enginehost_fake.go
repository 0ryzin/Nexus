package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
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
	reveal  bool
	readyAt time.Time
}

func newFakeEngine() *fakeEngine { return &fakeEngine{st: EngineStatus{State: "idle"}, volume: 100} }

func (f *fakeEngine) Info() (EngineBuildInfo, error) {
	return EngineBuildInfo{Name: "NEXUS Engine", Version: "test stand-in (not an emulator)", API: engineAPIVersion,
		Upstream:  map[string]string{"project": "development stand-in"},
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
	fakeLast = f
	f.stop = make(chan bool, 1)
	go f.run(req)
	return nil
}

func (f *fakeEngine) run(req EngineRequest) {
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(req.Game, "bad") {
		f.set(func(s *EngineStatus) {
			*s = EngineStatus{State: "error", Code: "load", Message: "The game could not be loaded."}
		})
		return
	}
	if _, err := os.Stat(req.ConfigFile); err != nil {
		f.set(func(s *EngineStatus) {
			*s = EngineStatus{State: "error", Code: "config", Message: "session configuration missing"}
		})
		return
	}
	// development: a loading phase (NEXUS_FAKE_LOAD) and an engine exception with upstream wording
	if d, err := time.ParseDuration(os.Getenv("NEXUS_FAKE_LOAD")); err == nil && d > 0 {
		// like the real engine: "starting" while loading, the second half with measured shader progress
		f.set(func(s *EngineStatus) { s.State = "starting" })
		time.Sleep(d / 2)
		for i := uint64(0); i <= 20; i++ {
			f.set(func(s *EngineStatus) { s.LoadStage, s.LoadDone, s.LoadTotal = "shaders", i*50, 1000 })
			time.Sleep(d / 40)
		}
		f.set(func(s *EngineStatus) { s.LoadStage, s.LoadDone, s.LoadTotal = "", 0, 0 })
	}
	fakeLast = f
	if strings.Contains(req.Game, "exception") || os.Getenv("NEXUS_FAKE_FAIL") == "exception" {
		f.set(func(s *EngineStatus) {
			*s = EngineStatus{State: "error", Code: "exception", Message: "Legacy emulator data or an unfinished import was found. Open the Lichii GUI to import it or explicitly start fresh before using eden-cli. No configuration was created."}
		})
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
			if f.st.Suspended && !f.st.Background { // paused (running in the background keeps going)
				f.mu.Unlock()
				continue
			}
			f.st.Frames += 3
			f.samples = toneBuffer(f.volume)
			// like engine 0.4: ready with the first frame, shown at once or after Reveal
			if !f.st.Ready {
				f.st.Ready, f.readyAt = true, time.Now()
			}
			timeout := time.Duration(max(f.req.RevealTimeoutMs, 0)) * time.Millisecond
			if !f.st.Shown && (!f.req.RevealOnRequest || f.reveal || time.Since(f.readyAt) > timeout) {
				f.st.Shown = true
				f.st.Fullscreen = f.req.Fullscreen
			}
			f.mu.Unlock()
			if strings.Contains(req.Game, "crash") && f.Status().Frames > 15 {
				f.set(func(s *EngineStatus) {
					s.State = "error"
					s.Code = "crash"
					s.Message = "The game stopped unexpectedly."
				})
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
		f.st.Background = false
	}
	f.mu.Unlock()
}

// Background: like engine 0.5, hidden but running (frames keep coming), input blocked.
func (f *fakeEngine) Background(on bool) bool {
	f.mu.Lock()
	if f.st.State == "running" {
		f.st.Suspended, f.st.Background = on, on
	}
	f.mu.Unlock()
	return true
}

func (f *fakeEngine) Screenshot() bool { return f.PressScreenshot() == nil }

func (f *fakeEngine) Window() uintptr {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.st.Shown {
		return 0x1234
	}
	return 0
}

// PressRecord: like the record key (F12) in the game window.
func (f *fakeEngine) PressRecord() {
	f.set(func(s *EngineStatus) {
		if !s.Suspended {
			s.RecordPresses++
		}
	})
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

// fakeLast is the stand-in engine of the current session (development: simulate a Home press).
var fakeLast *fakeEngine

func (f *fakeEngine) PressHome() {
	f.set(func(s *EngineStatus) {
		if !s.Suspended { // like the engine: presses while NEXUS is showing are not counted
			s.HomePresses++
		}
	})
}

func (f *fakeEngine) Reveal() {
	f.mu.Lock()
	f.reveal = true
	f.mu.Unlock()
}

// Controllers: NEXUS_FAKE_PADS (JSON) or an Xbox and a DualSense controller.
func (f *fakeEngine) Controllers() ([]PadInfo, error) {
	if v := os.Getenv("NEXUS_FAKE_PADS"); v != "" {
		var out []PadInfo
		err := json.Unmarshal([]byte(v), &out)
		return out, err
	}
	return []PadInfo{
		{GUID: "030000005e040000e002000000007801", Name: "Xbox Wireless Controller", Type: "xboxone", Vendor: 0x045e, Product: 0x02e0, Serial: "XB-0001"},
		{GUID: "050000004c050000e60c000000006801", Name: "DualSense Wireless Controller", Type: "ps5", Vendor: 0x054c, Product: 0x0ce6, Serial: "a4:ae:12:00:00:01"},
	}, nil
}

// PressScreenshot: like the engine's F12 / capture button, a PNG with a unique name.
func (f *fakeEngine) PressScreenshot() error {
	f.mu.Lock()
	req, shown := f.req, f.st.Shown
	f.mu.Unlock()
	if req.ScreenshotDir == "" || !shown {
		return errors.New("no screenshot folder or game window")
	}
	_ = os.MkdirAll(req.ScreenshotDir, 0o755)
	stamp := time.Now().Format("2006-01-02 15-04-05")
	name := req.ScreenshotName + " " + stamp + ".png"
	path := filepath.Join(req.ScreenshotDir, name)
	for n := 2; fileExists(path); n++ {
		path = filepath.Join(req.ScreenshotDir, fmt.Sprintf("%s %s (%d).png", req.ScreenshotName, stamp, n))
	}
	if err := os.WriteFile(path, fakePNG(), 0o644); err != nil {
		return err
	}
	f.set(func(s *EngineStatus) { s.Screenshots++; s.LastScreenshot = path })
	return nil
}

// ToggleFullscreen: like F11.
func (f *fakeEngine) ToggleFullscreen() {
	f.set(func(s *EngineStatus) { s.Fullscreen = !s.Fullscreen })
}

func fakePNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / 320), uint8(y * 255 / 180), 160, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
