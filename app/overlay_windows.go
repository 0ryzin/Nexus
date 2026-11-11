//go:build windows

package main

// The in-game performance overlay: a thin, semi-transparent bar at the top or bottom of the
// screen the game is on. It is a separate layered, topmost, click-through window that never takes
// focus, so it does not touch the game's rendering or input. It is shown only while a game is in
// front and the overlay is switched on for that game, and updates twice a second.
// Limitation: a game in *exclusive* fullscreen (Settings › Emulation › Fullscreen behaviour)
// covers every other window, so the overlay shows with borderless fullscreen (the default) and
// windowed games.

import (
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ovUser                     = windows.NewLazySystemDLL("user32.dll")
	ovGdi                      = windows.NewLazySystemDLL("gdi32.dll")
	procOvRegisterClassEx      = ovUser.NewProc("RegisterClassExW")
	procOvCreateWindowEx       = ovUser.NewProc("CreateWindowExW")
	procOvDefWindowProc        = ovUser.NewProc("DefWindowProcW")
	procOvGetMessage           = ovUser.NewProc("GetMessageW")
	procOvTranslateMessage     = ovUser.NewProc("TranslateMessage")
	procOvDispatchMessage      = ovUser.NewProc("DispatchMessageW")
	procOvSetTimer             = ovUser.NewProc("SetTimer")
	procOvShowWindow           = ovUser.NewProc("ShowWindow")
	procOvSetWindowPos         = ovUser.NewProc("SetWindowPos")
	procOvSetLayeredAttributes = ovUser.NewProc("SetLayeredWindowAttributes")
	procOvInvalidateRect       = ovUser.NewProc("InvalidateRect")
	procOvBeginPaint           = ovUser.NewProc("BeginPaint")
	procOvEndPaint             = ovUser.NewProc("EndPaint")
	procOvFillRect             = ovUser.NewProc("FillRect")
	procOvDrawText             = ovUser.NewProc("DrawTextW")
	procOvMonitorFromWindow    = ovUser.NewProc("MonitorFromWindow")
	procOvGetMonitorInfo       = ovUser.NewProc("GetMonitorInfoW")
	procOvCreateSolidBrush     = ovGdi.NewProc("CreateSolidBrush")
	procOvDeleteObject         = ovGdi.NewProc("DeleteObject")
	procOvSetBkMode            = ovGdi.NewProc("SetBkMode")
	procOvSetTextColor         = ovGdi.NewProc("SetTextColor")
	procOvCreateFont           = ovGdi.NewProc("CreateFontW")
	procOvSelectObject         = ovGdi.NewProc("SelectObject")
)

const (
	wsExLayered      = 0x00080000
	wsExTransparent  = 0x00000020
	wsExTopmost      = 0x00000008
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	wmTimer          = 0x0113
	wmPaint          = 0x000F
	swHide           = 0
	swShowNoActivate = 4
	lwaAlpha         = 0x2
	swpNoActivate    = 0x0010
	hwndTopmost      = ^uintptr(0) // (HWND)-1
)

type ovRect struct{ L, T, R, B int32 }
type ovMonitorInfo struct {
	Size          uint32
	Monitor, Work ovRect
	Flags         uint32
}
type ovPaint struct {
	HDC      uintptr
	Erase    int32
	Rc       ovRect
	Restore  int32
	IncUpd   int32
	Reserved [32]byte
}
type ovMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}
type ovClass struct {
	Size                                     uint32
	Style                                    uint32
	WndProc                                  uintptr
	ClsExtra, WndExtra                       int32
	Instance, Icon, Cursor, Background, Menu uintptr
	ClassName                                *uint16
	IconSm                                   uintptr
}

type perfOverlay struct {
	app     *App
	mu      sync.Mutex
	hwnd    uintptr
	text    string
	dark    bool
	shown   bool
	height  int32
	started bool
}

var overlay = &perfOverlay{}

// StartPerfOverlay starts the overlay's own UI thread (once). Cheap while no game is in front.
func (a *App) StartPerfOverlay() {
	overlay.mu.Lock()
	defer overlay.mu.Unlock()
	if overlay.started {
		return
	}
	overlay.started, overlay.app = true, a
	go overlay.loop()
}

func (o *perfOverlay) loop() {
	runtime.LockOSThread()
	name, _ := windows.UTF16PtrFromString("NEXUSPerfOverlay")
	wc := ovClass{WndProc: windows.NewCallback(o.wndProc), ClassName: name}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procOvRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := windows.UTF16PtrFromString("NEXUS performance")
	hwnd, _, _ := procOvCreateWindowEx.Call(wsExLayered|wsExTransparent|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 10, 10, 0, 0, 0, 0)
	if hwnd == 0 {
		return
	}
	o.hwnd = hwnd
	procOvSetTimer.Call(hwnd, 1, 500, 0)
	var m ovMsg
	for {
		r, _, _ := procOvGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procOvTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procOvDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (o *perfOverlay) wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmTimer:
		o.tick()
		return 0
	case wmPaint:
		o.paint(hwnd)
		return 0
	}
	r, _, _ := procOvDefWindowProc.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func (o *perfOverlay) tick() {
	a := o.app
	cfg := a.Settings.Get().PerfMonitor
	st := a.Emu.State()
	want := st.Running && st.Phase == "playing" && !st.Suspended && cfg.For(st.GameKey)
	if !want {
		if o.shown {
			procOvShowWindow.Call(o.hwnd, swHide)
			o.shown = false
		}
		return
	}
	s := perf.Sample(a.Emu)
	o.text = formatPerf(s, cfg.Metrics, "    ")
	if p, ok := a.Profiles.Get(st.ProfileID); ok {
		o.dark = p.Theme.Mode != "light"
	}
	// strip at the top or bottom of the monitor NEXUS (and so the game) is on
	var mi ovMonitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	mon, _, _ := procOvMonitorFromWindow.Call(a.UI.WindowHandle(), 2 /*NEAREST*/)
	procOvGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	scale := clampF(float64(cfg.Scale), 75, 200) / 100
	o.height = int32(30 * scale)
	w := mi.Monitor.R - mi.Monitor.L
	y := mi.Monitor.T
	if cfg.Position == "bottom" {
		y = mi.Monitor.B - o.height
	}
	alpha := uintptr(clampF(float64(cfg.Opacity), 30, 100) * 255 / 100)
	procOvSetLayeredAttributes.Call(o.hwnd, 0, alpha, lwaAlpha)
	procOvSetWindowPos.Call(o.hwnd, hwndTopmost, uintptr(mi.Monitor.L), uintptr(y), uintptr(w), uintptr(o.height), swpNoActivate)
	if !o.shown {
		procOvShowWindow.Call(o.hwnd, swShowNoActivate)
		o.shown = true
	}
	procOvInvalidateRect.Call(o.hwnd, 0, 1)
}

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

func (o *perfOverlay) paint(hwnd uintptr) {
	var ps ovPaint
	hdc, _, _ := procOvBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procOvEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	bg, fg := rgb(22, 23, 26), rgb(244, 245, 247)
	if !o.dark {
		bg, fg = rgb(244, 245, 247), rgb(28, 29, 32)
	}
	brush, _, _ := procOvCreateSolidBrush.Call(bg)
	procOvFillRect.Call(hdc, uintptr(unsafe.Pointer(&ps.Rc)), brush)
	procOvDeleteObject.Call(brush)
	face, _ := windows.UTF16PtrFromString("Segoe UI Semibold")
	font, _, _ := procOvCreateFont.Call(uintptr(-int32(float64(o.height)*0.55)), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5 /*CLEARTYPE*/, 0, uintptr(unsafe.Pointer(face)))
	old, _, _ := procOvSelectObject.Call(hdc, font)
	procOvSetBkMode.Call(hdc, 1 /*TRANSPARENT*/)
	procOvSetTextColor.Call(hdc, fg)
	text, _ := windows.UTF16FromString(o.text)
	procOvDrawText.Call(hdc, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1), uintptr(unsafe.Pointer(&ps.Rc)), 0x1|0x4|0x20 /*CENTER|VCENTER|SINGLELINE*/)
	procOvSelectObject.Call(hdc, old)
	procOvDeleteObject.Call(font)
}
