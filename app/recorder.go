package main

// Screen recording (NEXUS 0.4.0).
//
// F12 while playing starts or stops a recording of the game window; Alt+F12 takes a screenshot
// (both changeable in Settings › Recording). The engine only reports the key presses; recording
// itself is done by the NEXUS capture module (nexus_capture.dll, capture/ in the repository):
// Windows Graphics Capture of the game window only (never the NEXUS interface), GPU colour
// conversion, hardware H.264/HEVC encoding where the graphics driver offers it (NVIDIA NVENC,
// AMD, Intel Quick Sync), software encoding otherwise, the game's own sound and optionally the
// microphone. Files are fragmented MP4 written as "<name>.mp4.part" until the recording stops, so
// a crash still leaves a playable file, which NEXUS renames at the next start.
//
// This file is the platform-independent part; capture_windows.go loads the capture module.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RecordingSettings are in Settings › Recording.
type RecordingSettings struct {
	Enabled    bool   `json:"enabled"`
	Key        string `json:"key"`        // start/stop while playing ("" = no shortcut)
	Resolution string `json:"resolution"` // native | 720p | 1080p | 1440p | 2160p
	FPS        int    `json:"fps"`        // 30 | 60
	Quality    string `json:"quality"`    // standard | high | max | custom
	Bitrate    int    `json:"bitrate"`    // kbit/s, for custom quality
	Codec      string `json:"codec"`      // h264 | hevc
	Audio      bool   `json:"audio"`      // the game's sound
	Mic        bool   `json:"mic"`
	Hardware   bool   `json:"hardware"` // allow hardware encoders
	Folder     string `json:"folder"`   // "" = Media\Recordings
}

func DefaultRecordingSettings() RecordingSettings {
	return RecordingSettings{Enabled: true, Key: "F12", Resolution: "1080p", FPS: 60, Quality: "high", Bitrate: 12000,
		Codec: "h264", Audio: true, Hardware: true}
}

// CaptureCaps is what the capture module finds on this PC.
type CaptureCaps struct {
	Available     bool         `json:"available"`
	Reason        string       `json:"reason,omitempty"`
	Encoders      []EncoderCap `json:"encoders"`
	WindowCapture bool         `json:"windowCapture"`
	ProcessAudio  bool         `json:"processAudio"`
	Hardware      bool         `json:"hardware"` // a hardware H.264 encoder exists
	HardwareHEVC  bool         `json:"hardwareHevc"`
	Vendor        string       `json:"vendor,omitempty"` // of the hardware encoder
	// what NEXUS offers on this PC (software encoding is limited to keep games smooth)
	Resolutions []string `json:"resolutions"`
	FPS         []int    `json:"fps"`
}

type EncoderCap struct {
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	Hardware bool   `json:"hardware"`
	Codec    string `json:"codec"`
}

// CaptureStatus mirrors nxcap_status.
type CaptureStatus struct {
	State      string  `json:"state"`
	Seconds    float64 `json:"seconds"`
	Frames     uint64  `json:"frames"`
	Dropped    uint64  `json:"dropped"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Encoder    string  `json:"encoder"`
	GPU        bool    `json:"gpu"`
	GPUConvert bool    `json:"gpuConvert"`
	Audio      string  `json:"audio"`
	Mic        bool    `json:"mic"`
	Path       string  `json:"path"`
	Error      string  `json:"error"`
}

// captureBackend is the capture module (capture_windows.go) or nothing (other platforms).
type captureBackend interface {
	Caps() (CaptureCaps, error)
	Start(settings string) error
	Status() CaptureStatus
	Stop() error
}

// Recorder runs one recording at a time.
type Recorder struct {
	app     *App
	backend captureBackend
	mu      sync.Mutex
	active  bool
	started time.Time
	path    string
	info    MediaInfo
	last    CaptureStatus
	caps    *CaptureCaps
}

func NewRecorder(a *App) *Recorder { return &Recorder{app: a, backend: newCaptureBackend(a)} }

// Caps: what this PC can record (cached; the encoder list does not change while NEXUS runs).
func (r *Recorder) Caps() CaptureCaps {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.caps != nil {
		return *r.caps
	}
	c := CaptureCaps{Resolutions: []string{"native", "720p", "1080p"}, FPS: []int{30, 60}}
	if r.backend == nil {
		c.Reason = "Recording is not available on this platform yet."
		r.caps = &c
		return c
	}
	got, err := r.backend.Caps()
	if err != nil {
		c.Reason = "The NEXUS capture module could not be loaded: " + err.Error() + ". Extract the complete NEXUS ZIP again."
		r.caps = &c
		return c
	}
	got.Available = got.WindowCapture
	if !got.WindowCapture {
		got.Reason = "This version of Windows cannot capture a window (Windows 10 version 1903 or later is needed)."
	}
	for _, e := range got.Encoders {
		if e.Hardware && e.Codec == "h264" && !got.Hardware {
			got.Hardware, got.Vendor = true, e.Vendor
		}
		if e.Hardware && e.Codec == "hevc" {
			got.HardwareHEVC = true
		}
	}
	// without a hardware encoder the CPU encodes; above 1080p or 60 fps that costs the game too
	// much, so it is not offered
	got.Resolutions = []string{"native", "720p", "1080p"}
	got.FPS = []int{30, 60}
	if got.Hardware {
		got.Resolutions = append(got.Resolutions, "1440p", "2160p")
	}
	r.caps = &got
	return got
}

// effective settings for this PC (never more than it can encode)
func (r *Recorder) settingsFor(c CaptureCaps) RecordingSettings {
	s := r.app.Settings.Get().Recording
	if !contains(c.Resolutions, s.Resolution) {
		s.Resolution = "1080p"
	}
	if s.FPS != 30 && s.FPS != 60 {
		s.FPS = 60
	}
	if s.Codec == "hevc" && !c.HardwareHEVC {
		s.Codec = "h264"
	}
	if !c.Hardware && s.Resolution == "native" {
		s.Resolution = "1080p" // native could be 4K: too much for software encoding
	}
	return s
}

func resolutionSize(res string) (int, int) {
	switch res {
	case "720p":
		return 1280, 720
	case "1080p":
		return 1920, 1080
	case "1440p":
		return 2560, 1440
	case "2160p":
		return 3840, 2160
	}
	return 0, 0
}

// RecordingBitrate (kbit/s) for a resolution, frame rate and quality.
func RecordingBitrate(s RecordingSettings) int {
	if s.Quality == "custom" && s.Bitrate >= 1000 {
		return min(s.Bitrate, 150000)
	}
	w, h := resolutionSize(s.Resolution)
	if w == 0 {
		w, h = 1920, 1080
	}
	// about 0.1 bit per pixel per frame at 60 fps for "high" (H.264)
	base := float64(w*h*s.FPS) * 0.1 / 1000
	switch s.Quality {
	case "standard":
		base *= 0.65
	case "max":
		base *= 1.6
	}
	if s.Codec == "hevc" {
		base *= 0.7
	}
	return int(clampF(base, 2500, 120000))
}

func (r *Recorder) Active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// Elapsed is how long the current recording has been running.
func (r *Recorder) Elapsed() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return 0
	}
	return time.Since(r.started)
}

// Toggle starts or stops recording the running game.
func (r *Recorder) Toggle() error {
	if r.Active() {
		_, err := r.Stop()
		return err
	}
	return r.Start()
}

var errNoGameWindow = errors.New("No game is running.")

// Start records the running game's window.
func (r *Recorder) Start() error {
	st := r.app.Emu.State()
	if !st.Running || st.Phase != "playing" {
		return errNoGameWindow
	}
	cfg := r.app.Settings.Get().Recording
	if !cfg.Enabled {
		return errors.New("Recording is turned off in Settings › Recording.")
	}
	caps := r.Caps()
	if !caps.Available {
		return errors.New(caps.Reason)
	}
	hwnd := r.app.Emu.GameWindow()
	if hwnd == 0 {
		return errors.New("The game window is not ready yet.")
	}
	s := r.settingsFor(caps)
	dir := s.Folder
	if dir == "" {
		dir = r.app.Media.RecordingsDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("the recordings folder cannot be created: %w", err)
	}
	var g *Game
	if gg, ok := r.app.Library.Get(st.GameKey); ok {
		g = &gg
	}
	path := uniqueMediaPath(dir, captureName(g), ".mp4")
	w, h := resolutionSize(s.Resolution)
	lines := []string{
		fmt.Sprintf("hwnd=%d", hwnd), "path=" + path,
		fmt.Sprintf("width=%d", w), fmt.Sprintf("height=%d", h), fmt.Sprintf("fps=%d", s.FPS),
		fmt.Sprintf("bitrate=%d", RecordingBitrate(s)), "codec=" + s.Codec,
		fmt.Sprintf("audio=%d", b2i(s.Audio)), fmt.Sprintf("mic=%d", b2i(s.Mic)),
		fmt.Sprintf("hardware=%d", b2i(s.Hardware)), fmt.Sprintf("pid=%d", os.Getpid()),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active {
		return nil
	}
	if err := r.backend.Start(strings.Join(lines, "\n") + "\n"); err != nil {
		r.app.Log.Printf("recording: could not start: %v", err)
		return fmt.Errorf("Recording could not start: %v", err)
	}
	r.active, r.started, r.path = true, time.Now(), path
	r.info = MediaInfo{GameKey: st.GameKey, Title: st.Title, ProfileID: st.ProfileID, Captured: time.Now().Unix()}
	if g != nil {
		r.info.TitleID = g.TitleID
	}
	r.app.Log.Printf("recording: started %s (%s, %d fps, %d kbit/s)", filepath.Base(path), s.Resolution, s.FPS, RecordingBitrate(s))
	r.app.Events.Emit("recording.started", map[string]any{"title": st.Title})
	go r.watch(path)
	return nil
}

// watch reports a recording that stopped by itself (an encoder error, the window closed).
func (r *Recorder) watch(path string) {
	for {
		time.Sleep(time.Second)
		r.mu.Lock()
		if !r.active || r.path != path {
			r.mu.Unlock()
			return
		}
		st := r.backend.Status()
		r.last = st
		r.mu.Unlock()
		if st.State == "error" {
			msg := st.Error
			if _, err := r.Stop(); err != nil && msg == "" {
				msg = err.Error()
			}
			r.app.Events.Emit("recording.failed", map[string]any{"error": "Recording stopped: " + msg})
			return
		}
	}
}

// Stop finishes the recording and adds it to the Media library.
func (r *Recorder) Stop() (MediaItem, error) {
	r.mu.Lock()
	if !r.active {
		r.mu.Unlock()
		return MediaItem{}, nil
	}
	err := r.backend.Stop()
	st := r.backend.Status()
	path, info := r.path, r.info
	if st.Seconds <= 0 {
		st.Seconds = time.Since(r.started).Seconds()
	}
	r.active, r.last = false, st
	r.mu.Unlock()
	if err != nil {
		r.app.Log.Printf("recording: %v", err)
		// a partial file is kept (it plays up to where it stopped)
		recoverPartial(path)
		return MediaItem{}, fmt.Errorf("The recording could not be finished: %v", err)
	}
	r.app.Media.Record(path, info)
	r.app.Log.Printf("recording: saved %s (%.0f s, %d frames, %d dropped, %s, audio %s)", filepath.Base(path), st.Seconds, st.Frames, st.Dropped, st.Encoder, st.Audio)
	it := MediaItem{ID: r.app.Media.rel(path), Type: "recording", Name: filepath.Base(path), MediaInfo: info}
	r.app.Events.Emit("recording.saved", map[string]any{"title": info.Title, "seconds": st.Seconds, "id": it.ID})
	return it, nil
}

// Status for the interface.
func (r *Recorder) Status() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]any{"active": r.active}
	if r.active {
		out["seconds"] = time.Since(r.started).Seconds()
		out["detail"] = r.last
	}
	return out
}

// RecoverInterrupted renames recordings cut off by a crash ("x.mp4.part") to "x (interrupted).mp4"
// (fragmented MP4 plays up to the last complete fragment).
func (r *Recorder) RecoverInterrupted() {
	dirs := []string{r.app.Media.RecordingsDir()}
	if f := r.app.Settings.Get().Recording.Folder; f != "" {
		dirs = append(dirs, f)
	}
	for _, d := range dirs {
		parts, _ := filepath.Glob(filepath.Join(d, "*.mp4.part"))
		for _, p := range parts {
			if q := recoverPartial(p[:len(p)-len(".part")]); q != "" {
				r.app.Log.Printf("recording: kept an interrupted recording as %s", filepath.Base(q))
			}
		}
	}
}

func recoverPartial(path string) string {
	part := path + ".part"
	fi, err := os.Stat(part)
	if err != nil {
		return ""
	}
	if fi.Size() == 0 {
		_ = os.Remove(part)
		return ""
	}
	base := strings.TrimSuffix(path, ".mp4") + " (interrupted)"
	q := uniqueFile(base, ".mp4")
	if os.Rename(part, q) != nil {
		return ""
	}
	return q
}

// uniqueMediaPath: "<name> 2026-11-12 10-30-00.mp4" (a number is added if it exists).
func uniqueMediaPath(dir, name, ext string) string {
	return uniqueFile(filepath.Join(dir, name+" "+time.Now().Format("2006-01-02 15-04-05")), ext)
}

func uniqueFile(base, ext string) string {
	p := base + ext
	for i := 2; fileExists(p) || fileExists(p+".part"); i++ {
		p = fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
	return p
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseCaps(data []byte) (CaptureCaps, error) {
	var c CaptureCaps
	err := json.Unmarshal(data, &c)
	return c, err
}
