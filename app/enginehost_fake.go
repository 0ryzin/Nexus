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
	// shader preparation (ShaderPreparer): titles whose precache file was "built", and a skip
	prepared  map[string]bool
	precached map[string]bool // boot precache finished (the real engine's marker)
	prepStop  chan bool
	skip      bool
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
	f.st = EngineStatus{State: "starting", Volume: req.Volume, Prepare: f.st.Prepare}
	f.skip = false
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
	// like the real engine: the boot precache before the game starts, when the session config asks
	// for it (precache_shaders = 2) and the title was prepared; "Start now" skips the rest
	if cfg, _ := os.ReadFile(req.ConfigFile); bytes.Contains(cfg, []byte("precache_shaders=2")) && f.wasPrepared(req) && !f.wasPrecached(req) {
		const total = 120
		finished := true
		for i := uint64(0); i <= total; i += 8 {
			f.mu.Lock()
			skip := f.skip
			f.mu.Unlock()
			if skip {
				finished = false
				break
			}
			f.set(func(s *EngineStatus) {
				s.State, s.LoadStage, s.LoadDone, s.LoadTotal = "starting", "precache", i, total
			})
			time.Sleep(60 * time.Millisecond)
		}
		if finished {
			f.mu.Lock()
			if f.precached == nil {
				f.precached = map[string]bool{}
			}
			f.precached[req.Game] = true
			f.mu.Unlock()
		}
		f.set(func(s *EngineStatus) { s.LoadStage, s.LoadDone, s.LoadTotal = "", 0, 0 })
	}
	// development: a loading phase (NEXUS_FAKE_LOAD) and an engine exception with upstream wording
	if d, err := time.ParseDuration(os.Getenv("NEXUS_FAKE_LOAD")); err == nil && d > 0 {
		// like the real engine: "starting" while loading, the second half with measured shader progress
		f.set(func(s *EngineStatus) { s.State = "starting" })
		time.Sleep(d / 2)
		for i := uint64(0); i <= 20; i++ {
			f.set(func(s *EngineStatus) {
				s.LoadStage, s.LoadDone, s.LoadTotal = "shaders", i*50, 1000
				s.Shaders = fakeShaders(i*50, 0, 0, 0)
			})
			time.Sleep(d / 40)
		}
		f.set(func(s *EngineStatus) { s.LoadStage, s.LoadDone, s.LoadTotal = "", 0, 0 })
	}
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
	playStart := time.Now()
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
			// like the engine: shaders compiled during the first seconds of play, then ready
			el := time.Since(playStart).Seconds()
			cached := uint64(0)
			if f.st.Shaders != nil {
				cached = f.st.Shaders.FromCache
			}
			f.st.Shaders = fakeShaders(cached, uint64(min(el, 5)*8), uint64(max(0, 10-el*3)), uint64(max(0, 30-el*10)))
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
	if s.Prepare != nil { // a copy: the preparation timer updates f.st.Prepare in place
		p := *s.Prepare
		s.Prepare = &p
	}
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

// Amiibo: the stand-in game scans for an amiibo while NEXUS_FAKE_AMIIBO=1 (tests set
// st.Amiibo = "scanning"); a placed dump is checked like the engine does (size).
func (f *fakeEngine) Amiibo(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.st.State != "running" {
		return -1
	}
	if path == "" {
		if f.st.Amiibo == "placed" {
			f.st.Amiibo = "scanning"
		}
		return 0
	}
	if f.st.Amiibo != "scanning" {
		return 1
	}
	fi, err := os.Stat(path)
	if err != nil {
		return 2
	}
	switch fi.Size() {
	case 532, 540, 572, 1024:
	default:
		return 3
	}
	f.st.Amiibo = "placed"
	return 0
}

// Scan makes the stand-in game ask for an amiibo (tests).
func (f *fakeEngine) Scan() {
	f.mu.Lock()
	f.st.Amiibo = "scanning"
	f.mu.Unlock()
}

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
	if f.req.Embed && f.req.OwnerWindow != 0 {
		f.st.Embedded = true // like the engine: drawn inside the NEXUS window once revealed
	}
	f.mu.Unlock()
}

// PressFullscreen: F11 in an embedded game is counted for NEXUS (engine 0.6).
func (f *fakeEngine) PressFullscreen() {
	f.mu.Lock()
	f.st.FullscreenPresses++
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

func fakeShaders(cached, compiled, building, background uint64) *ShaderStatus {
	zero := uint64(0)
	q, r, bq := building+background, min(building, 2), background
	q -= min(q, r)
	return &ShaderStatus{FromCache: cached, Compiled: compiled, Queued: &q, Running: &r, BackgroundQueued: &bq, Failed: &zero}
}

// ---- ShaderPreparer: the real engine's stages and states, on a short timer

func (f *fakeEngine) wasPrecached(req EngineRequest) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.precached[req.Game]
}

func (f *fakeEngine) wasPrepared(req EngineRequest) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prepared[req.Game]
}

func (f *fakeEngine) PrepareShaders(req PrepRequest) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.st.State == "starting" || f.st.State == "running" || f.st.State == "stopping" ||
		f.st.Prepare != nil && f.st.Prepare.State == "running" {
		return -1
	}
	if req.Game == "" || req.TitleID == "" || req.TitleID == "0" || req.DataDir == "" || req.ConfigFile == "" {
		return -2
	}
	if f.prepared == nil {
		f.prepared = map[string]bool{}
	}
	if f.prepared[req.Game] && !req.Force {
		f.st.Prepare = &PrepStatus{State: "current", TitleID: req.TitleID, Programs: 96}
		return 0
	}
	f.st.Prepare = &PrepStatus{State: "running", TitleID: req.TitleID}
	f.prepStop = make(chan bool, 1)
	stop := f.prepStop
	go func() {
		begin := time.Now()
		stages := []struct {
			name  string
			total uint64
		}{{"Reading game files", 0}, {"Extracting shaders", 12}, {"Converting", 96}, {"Writing", 1}}
		for _, st := range stages {
			steps := max(st.total, 1)
			for i := uint64(0); i <= steps; i += max(steps/6, 1) {
				select {
				case <-stop:
					f.set(func(s *EngineStatus) { s.Prepare.State, s.Prepare.Stage = "stopped", "" })
					return
				default:
				}
				f.set(func(s *EngineStatus) {
					s.Prepare.Stage, s.Prepare.Done, s.Prepare.Total = st.name, min(i, st.total), st.total
				})
				time.Sleep(time.Duration(400/(steps/max(steps/6, 1)+1)) * time.Millisecond) // each stage ~0.4 s (real ones take seconds)
			}
		}
		f.mu.Lock()
		f.prepared[req.Game] = true
		delete(f.precached, req.Game) // a new precache file: its marker no longer matches
		f.mu.Unlock()
		f.set(func(s *EngineStatus) {
			s.Prepare = &PrepStatus{State: "done", TitleID: req.TitleID, Archives: 12, Programs: 96, Skipped: 1, Seconds: time.Since(begin).Seconds()}
		})
	}()
	return 0
}

func (f *fakeEngine) CancelPrepare() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.prepStop != nil {
		select {
		case f.prepStop <- true:
		default:
		}
	}
}

func (f *fakeEngine) SkipPrecache() {
	f.mu.Lock()
	f.skip = true
	f.mu.Unlock()
}
