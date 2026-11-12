package main

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

// fakeCapture stands in for the capture module in NEXUS's tests and the browser preview: it
// writes a small file where the recording would be, with the same .part / rename behaviour.
type fakeCapture struct {
	mu      sync.Mutex
	path    string
	started time.Time
	state   string
	failNow bool
}

func (f *fakeCapture) Caps() (CaptureCaps, error) {
	return CaptureCaps{WindowCapture: true, ProcessAudio: true, Encoders: []EncoderCap{
		{Name: "NVIDIA H.264 Encoder MFT", Vendor: "nvidia", Hardware: true, Codec: "h264"},
		{Name: "NVIDIA HEVC Encoder MFT", Vendor: "nvidia", Hardware: true, Codec: "hevc"},
		{Name: "H264 Encoder MFT", Vendor: "microsoft", Codec: "h264"}}}, nil
}

func (f *fakeCapture) Start(settings string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == "recording" {
		return errors.New("already recording")
	}
	for _, l := range strings.Split(settings, "\n") {
		if v, ok := strings.CutPrefix(l, "path="); ok {
			f.path = v
		}
	}
	if f.path == "" {
		return errors.New("no file name")
	}
	if err := os.WriteFile(f.path+".part", []byte("\x00\x00\x00\x1cftypiso6fake mp4"), 0o644); err != nil {
		return err
	}
	f.state, f.started = "recording", time.Now()
	return nil
}

func (f *fakeCapture) Status() CaptureStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := CaptureStatus{State: f.state, Encoder: "NVIDIA H.264 Encoder MFT", GPU: true, GPUConvert: true, Audio: "process", Path: f.path, Width: 1920, Height: 1080}
	if st.State == "" {
		st.State = "idle"
	}
	if f.state == "recording" {
		st.Seconds = time.Since(f.started).Seconds()
		st.Frames = uint64(st.Seconds * 60)
		if f.failNow {
			st.State, st.Error = "error", "encode video: the device was removed"
		}
	}
	return st
}

func (f *fakeCapture) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != "recording" {
		return nil
	}
	f.state = "idle"
	if f.failNow {
		return errors.New("encode video: the device was removed")
	}
	return os.Rename(f.path+".part", f.path)
}
