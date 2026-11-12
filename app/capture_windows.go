//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

// The NEXUS capture module next to NEXUS.exe (see recorder.go and capture/ in the repository).
type dllCapture struct {
	once                               sync.Once
	err                                error
	caps, start, status, stop, version *syscall.Proc
}

func newCaptureBackend(a *App) captureBackend {
	if fakeEngineEnabled() {
		return &fakeCapture{} // NEXUS's own tests
	}
	return &dllCapture{}
}

func (c *dllCapture) load() error {
	c.once.Do(func() {
		exe, _ := os.Executable()
		p := filepath.Join(filepath.Dir(exe), "nexus_capture.dll")
		d, err := syscall.LoadDLL(p)
		if err != nil {
			c.err = err
			return
		}
		for _, x := range []struct {
			name string
			dst  **syscall.Proc
		}{{"nxcap_api_version", &c.version}, {"nxcap_capabilities", &c.caps}, {"nxcap_start", &c.start}, {"nxcap_status", &c.status}, {"nxcap_stop", &c.stop}} {
			if *x.dst, err = d.FindProc(x.name); err != nil {
				c.err = err
				return
			}
		}
		if v, _, _ := c.version.Call(); int32(v) != 1 {
			c.err = errors.New("the capture module has another interface version")
		}
	})
	return c.err
}

func callJSON(p *syscall.Proc) []byte {
	buf := make([]byte, 64<<10)
	n, _, _ := p.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if int(int32(n)) >= len(buf) {
		buf = make([]byte, int(int32(n))+1)
		p.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	}
	return []byte(cstr(uintptr(unsafe.Pointer(&buf[0]))))
}

func (c *dllCapture) Caps() (CaptureCaps, error) {
	if err := c.load(); err != nil {
		return CaptureCaps{}, err
	}
	return parseCaps(callJSON(c.caps))
}

func (c *dllCapture) Start(settings string) error {
	if err := c.load(); err != nil {
		return err
	}
	s := append([]byte(settings), 0)
	errBuf := make([]byte, 2048)
	r, _, _ := c.start.Call(uintptr(unsafe.Pointer(&s[0])), uintptr(unsafe.Pointer(&errBuf[0])), uintptr(len(errBuf)))
	if int32(r) != 0 {
		return errors.New(cstr(uintptr(unsafe.Pointer(&errBuf[0]))))
	}
	return nil
}

func (c *dllCapture) Status() CaptureStatus {
	var st CaptureStatus
	if c.load() == nil {
		_ = json.Unmarshal(callJSON(c.status), &st)
	}
	return st
}

func (c *dllCapture) Stop() error {
	if err := c.load(); err != nil {
		return err
	}
	errBuf := make([]byte, 2048)
	r, _, _ := c.stop.Call(uintptr(unsafe.Pointer(&errBuf[0])), uintptr(len(errBuf)))
	if int32(r) != 0 {
		return errors.New(cstr(uintptr(unsafe.Pointer(&errBuf[0]))))
	}
	return nil
}
