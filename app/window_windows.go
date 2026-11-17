//go:build windows

package main

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// How the NEXUS window is shown. See docs/REGRESSION_HISTORY.md ("NEXUS started without a
// window"): the window used to be put into full screen by writing WS_VISIBLE into its style
// before it had ever been shown. Windows then treats it as already visible, so the
// SWP_SHOWWINDOW that followed did nothing and the window never appeared, while WebView2 ran
// behind it (the boot sound played). The rules now:
//   - the style's WS_VISIBLE bit is never written by hand; it always comes from ShowWindow;
//   - the window is created hidden, put into its start-up layout while hidden, then shown once;
//   - after the interface has loaded, and when NEXUS is started again, the window is checked
//     and brought back if it is hidden, minimised or off every monitor.

var (
	procMonitorFromRect = user32.NewProc("MonitorFromRect")
)

const (
	wmNexusActivate = wmApp + 2 // a second NEXUS.exe asks this one to show itself
	swShow          = 5
	swRestore       = 9
	swShowDefault   = 10
)

// windowStyleFor is the window style for full screen or a normal window. The WS_VISIBLE bit
// is carried over from the current style: setting it here would mark a hidden window visible
// without showing it.
func windowStyleFor(current uintptr, fullscreen bool) uintptr {
	vis := current & wsVisible
	if fullscreen {
		return wsPopup | vis
	}
	return wsOverlappedWindow | vis
}

// windowState is what the startup log records about the window.
type windowState struct {
	Visible, Minimised, OnMonitor bool
	Rect                          rect
}

func readWindowState(hwnd uintptr) windowState {
	var s windowState
	v, _, _ := procIsWindowVisible.Call(hwnd)
	ic, _, _ := procIsIconic.Call(hwnd)
	s.Visible, s.Minimised = v != 0, ic != 0
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&s.Rect)))
	mon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&s.Rect)), 0 /*MONITOR_DEFAULTTONULL*/)
	s.OnMonitor = mon != 0
	return s
}

// showMainWindow shows the window for the first time. SW_SHOWDEFAULT lets Windows apply how
// NEXUS was started (a shortcut set to "Minimised" stays minimised, in the taskbar).
func (u *winUI) showMainWindow() {
	t0 := time.Now()
	procShowWindow.Call(u.hwnd, swShowDefault)
	if v, _, _ := procIsWindowVisible.Call(u.hwnd); v == 0 {
		procShowWindow.Call(u.hwnd, swShow)
	}
	t1 := time.Now()
	focusWindow(u.hwnd)
	u.shown = true
	// the two steps timed separately: the user's log showed 0.15-1.6 s between "window created"
	// and "window shown", and which of them takes it decides the fix
	u.app.markStartup("window shown", fmt.Sprintf("%+v, full screen %v; showing took %v, bringing to the front %v", readWindowState(u.hwnd), u.fullscreen,
		t1.Sub(t0).Round(time.Millisecond), time.Since(t1).Round(time.Millisecond)))
}

// ensureWindowVisible brings the window back when it is hidden, minimised or entirely off the
// monitors (a monitor was unplugged or the resolution changed). reason goes to the log.
func (u *winUI) ensureWindowVisible(reason string, activate bool) {
	s := readWindowState(u.hwnd)
	fixed := false
	if !s.Visible {
		procShowWindow.Call(u.hwnd, swShow)
		fixed = true
	}
	if s.Minimised && activate {
		procShowWindow.Call(u.hwnd, swRestore)
		fixed = true
	}
	if !s.OnMonitor {
		if u.fullscreen {
			u.applyFullscreen(true) // re-fits to the nearest monitor
		} else {
			u.centreOnPrimary()
		}
		fixed = true
	}
	if activate {
		focusWindow(u.hwnd)
		if u.chromium != nil {
			u.chromium.Focus()
		}
	}
	if fixed {
		u.app.Log.Printf("startup: window was %+v (%s); shown again: %+v", s, reason, readWindowState(u.hwnd))
	}
}

func (u *winUI) centreOnPrimary() {
	var r rect
	procGetWindowRect.Call(u.hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := r.Right-r.Left, r.Bottom-r.Top
	cx, _, _ := procGetSystemMetrics.Call(0)
	cy, _, _ := procGetSystemMetrics.Call(1)
	if w > int32(cx) || w < 400 {
		w = int32(cx)
	}
	if h > int32(cy) || h < 300 {
		h = int32(cy)
	}
	procSetWindowPos.Call(u.hwnd, 0, uintptr((int32(cx)-w)/2), uintptr((int32(cy)-h)/2), uintptr(w), uintptr(h), swpNoZOrder)
}

// activateRunningInstance is what a second NEXUS.exe does: it lets the running one take the
// foreground and asks it to show itself (it restores its own full screen state). The running
// NEXUS may still be starting, before its window exists, so it waits for the window.
func activateRunningInstance(wait time.Duration) bool {
	cn, _ := windows.UTF16PtrFromString(className)
	deadline := time.Now().Add(wait)
	for {
		if w, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cn)), 0); w != 0 {
			var pid uint32
			procGetWindowThreadProcessId.Call(w, uintptr(unsafe.Pointer(&pid)))
			if pid != 0 {
				procAllowSetForegroundWindow.Call(uintptr(pid))
			}
			procPostMessageW.Call(w, wmNexusActivate, 0, 0)
			procSetForegroundWindow.Call(w)
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}
