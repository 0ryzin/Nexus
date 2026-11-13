//go:build windows

package main

// The in-game HUD: the performance strip (and the REC indicator) drawn over the game's picture.
//
// It behaves like part of the game, not like a desktop overlay:
//   - Its windows are *owned* by the window the game is drawn in (the NEXUS window when the game
//     is inside it, the game's own window otherwise) and are not topmost. Windows keeps owned
//     windows directly above their owner, moves them in the z-order with it, hides them when it
//     is minimised, and lets other programs cover them like the game itself.
//   - Position follows the game's picture: the client area of that window, minus the black bars
//     of the game's aspect ratio (gameImageRect). A WinEvent hook on this process reports every
//     move, resize, full-screen switch (F11) and minimise of that window, and the HUD is placed
//     again immediately - not only on the next twice-a-second refresh - so it never stays where
//     the window used to be (for example at the top of the monitor after leaving full screen).
//   - It is click-through and never takes focus, and does not touch the game's rendering or input.
//   - Drawing: a per-pixel-alpha bitmap (UpdateLayeredWindow): a translucent dark background with
//     opaque text, in the style of a handheld's performance strip (small coloured labels, large
//     values, small units), scaled with the picture's height.
// Ownership is set after the HUD windows exist (GWLP_HWNDPARENT), and Windows only stacks owned
// windows above their owner when the owner itself is re-stacked. Before 0.8.0 the strip could
// therefore stay below the game window until F11 re-stacked it (docs/REGRESSION_HISTORY.md), so
// the HUD now puts itself directly above its owner whenever it is shown, changes owner, or the
// owner moves, changes size or comes to the front (stack). The refresh timer runs only while a
// game is running; the emulator.state event starts it.
// A game in exclusive full screen covers every other window; the HUD needs borderless full screen
// (the default) or a window.

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ovUser                   = windows.NewLazySystemDLL("user32.dll")
	ovGdi                    = windows.NewLazySystemDLL("gdi32.dll")
	procOvRegisterClassEx    = ovUser.NewProc("RegisterClassExW")
	procOvCreateWindowEx     = ovUser.NewProc("CreateWindowExW")
	procOvDefWindowProc      = ovUser.NewProc("DefWindowProcW")
	procOvGetMessage         = ovUser.NewProc("GetMessageW")
	procOvTranslateMessage   = ovUser.NewProc("TranslateMessage")
	procOvDispatchMessage    = ovUser.NewProc("DispatchMessageW")
	procOvSetTimer           = ovUser.NewProc("SetTimer")
	procOvShowWindow         = ovUser.NewProc("ShowWindow")
	procOvIsIconic           = ovUser.NewProc("IsIconic")
	procOvIsWindowVisible    = ovUser.NewProc("IsWindowVisible")
	procOvGetClientRect      = ovUser.NewProc("GetClientRect")
	procOvClientToScreen     = ovUser.NewProc("ClientToScreen")
	procOvGetAncestor        = ovUser.NewProc("GetAncestor")
	procOvSetWindowLongPtr   = ovUser.NewProc("SetWindowLongPtrW")
	procOvUpdateLayered      = ovUser.NewProc("UpdateLayeredWindow")
	procOvSetWinEventHook    = ovUser.NewProc("SetWinEventHook")
	procOvGetDC              = ovUser.NewProc("GetDC")
	procOvReleaseDC          = ovUser.NewProc("ReleaseDC")
	procOvCreateCompatibleDC = ovGdi.NewProc("CreateCompatibleDC")
	procOvDeleteDC           = ovGdi.NewProc("DeleteDC")
	procOvCreateDIBSection   = ovGdi.NewProc("CreateDIBSection")
	procOvDeleteObject       = ovGdi.NewProc("DeleteObject")
	procOvSelectObject       = ovGdi.NewProc("SelectObject")
	procOvSetBkMode          = ovGdi.NewProc("SetBkMode")
	procOvSetTextColor       = ovGdi.NewProc("SetTextColor")
	procOvSetTextAlign       = ovGdi.NewProc("SetTextAlign")
	procOvCreateFont         = ovGdi.NewProc("CreateFontW")
	procOvTextOut            = ovGdi.NewProc("TextOutW")
	procOvGetTextExtentPoint = ovGdi.NewProc("GetTextExtentPoint32W")
	procOvGetCurrentThreadID = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	procOvSetWindowPos       = ovUser.NewProc("SetWindowPos")
	procOvGetWindow          = ovUser.NewProc("GetWindow")
	procOvKillTimer          = ovUser.NewProc("KillTimer")
	procOvPostMessage        = ovUser.NewProc("PostMessageW")
)

const (
	wsExLayered      = 0x00080000
	wsExTransparent  = 0x00000020
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	wmTimer          = 0x0113
	swHide           = 0
	swShowNoActivate = 4
	gwlpHwndParent   = ^uintptr(7) // -8
	ulwAlpha         = 2
	evLocationChange = 0x800B
	evMinimizeStart  = 0x0016
	evMinimizeEnd    = 0x0017
	evForeground     = 0x0003
	winEventOutOfCtx = 0
	wmHUDState       = 0x0400 + 1 // WM_USER+1: the emulator's state changed
	hudTimerMs       = 500
)

type ovRect = pxRect

// hudHost is the window the game is drawn in, and the area of it the game fills (screen
// coordinates); ok=false when it is not visible (minimised, hidden, no game window yet).
func hudHost(a *App) (host uintptr, area ovRect, ok bool) {
	host = a.UI.WindowHandle()
	if st := a.Emu.State(); !st.Embedded {
		if g := a.Emu.GameWindow(); g != 0 {
			host = g
		}
	}
	if host == 0 {
		return 0, ovRect{}, false
	}
	top, _, _ := procOvGetAncestor.Call(host, 2 /*GA_ROOT*/)
	if top == 0 {
		top = host
	}
	if ic, _, _ := procOvIsIconic.Call(top); ic != 0 {
		return top, ovRect{}, false
	}
	if vis, _, _ := procOvIsWindowVisible.Call(host); vis == 0 {
		return top, ovRect{}, false
	}
	var r ovRect
	procOvGetClientRect.Call(host, uintptr(unsafe.Pointer(&r)))
	pt := [2]int32{0, 0}
	procOvClientToScreen.Call(host, uintptr(unsafe.Pointer(&pt[0])))
	r.L, r.T, r.R, r.B = pt[0], pt[1], pt[0]+r.R, pt[1]+r.B
	return top, r, r.R > r.L && r.B > r.T
}

// gameArea is kept for the recorder and other callers: the game's area on screen.
func gameArea(a *App) (ovRect, bool) {
	_, r, ok := hudHost(a)
	return r, ok
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
type ovBitmapInfo struct {
	Size                       uint32
	Width, Height              int32
	Planes, BitCount           uint16
	Compression, SizeImage     uint32
	XPels, YPels, Used, Import int32
}

type hudWin struct {
	hwnd    uintptr
	owner   uintptr
	shown   bool
	restack bool // owner changed or just shown: put it above the owner after drawing
}

type perfOverlay struct {
	app     *App
	mu      sync.Mutex
	bar     hudWin
	rec     hudWin
	thread  uintptr
	segs    []PerfSeg
	host    uintptr // the window the game is drawn in, and its top-level window
	top     uintptr
	started bool
	ticking bool // the refresh timer is running (only while a game runs)
}

var overlay = &perfOverlay{}

// StartPerfOverlay starts the HUD's own UI thread (once). Cheap while no game is in front.
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
	name, _ := windows.UTF16PtrFromString("NEXUSGameHUD")
	wc := ovClass{WndProc: windows.NewCallback(o.wndProc), ClassName: name}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procOvRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	mk := func(title string) uintptr {
		t, _ := windows.UTF16PtrFromString(title)
		h, _, _ := procOvCreateWindowEx.Call(wsExLayered|wsExTransparent|wsExToolWindow|wsExNoActivate,
			uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(t)), wsPopup, 0, 0, 10, 10, 0, 0, 0, 0)
		return h
	}
	o.bar.hwnd, o.rec.hwnd = mk("NEXUS performance"), mk("NEXUS recording")
	if o.bar.hwnd == 0 {
		return
	}
	o.thread, _, _ = procOvGetCurrentThreadID.Call()
	// moves, resizes (F11), minimise/restore and activation of this process's windows
	cb := windows.NewCallback(o.winEvent)
	pid := uintptr(os.Getpid())
	procOvSetWinEventHook.Call(evLocationChange, evLocationChange, 0, cb, pid, 0, winEventOutOfCtx)
	procOvSetWinEventHook.Call(evMinimizeStart, evMinimizeEnd, 0, cb, pid, 0, winEventOutOfCtx)
	procOvSetWinEventHook.Call(evForeground, evForeground, 0, cb, 0, 0, winEventOutOfCtx)
	o.armTimer(o.app.Emu.State().Running)
	go o.followEmulator()
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

// winEvent: the game's window moved, changed size or was minimised: place the HUD again now.
func (o *perfOverlay) winEvent(hook, event, hwnd, idObject, idChild, thread, t uintptr) uintptr {
	if int32(idObject) != 0 /*OBJID_WINDOW*/ || hwnd == o.bar.hwnd || hwnd == o.rec.hwnd {
		return 0
	}
	switch event {
	case evLocationChange: // only the game's window matters (not every control inside NEXUS)
		if (hwnd == o.host || hwnd == o.top) && (o.bar.shown || o.rec.shown) {
			o.update(false)
			o.stackAll()
		}
	default: // minimise/restore, a window came to the front
		o.update(false)
		o.stackAll()
	}
	return 0
}

// followEmulator wakes the HUD thread when a game starts, stops, pauses or resumes, so it does
// not need a timer while NEXUS sits on Home.
func (o *perfOverlay) followEmulator() {
	ch := o.app.Events.Subscribe()
	for m := range ch {
		if bytes.Contains(m, []byte(`"type":"emulator.state"`)) || bytes.Contains(m, []byte(`"type":"emulator.exit"`)) {
			procOvPostMessage.Call(o.bar.hwnd, wmHUDState, 0, 0)
		}
	}
}

// armTimer starts the twice-a-second refresh while a game runs and stops it otherwise.
func (o *perfOverlay) armTimer(running bool) {
	switch {
	case running && !o.ticking:
		procOvSetTimer.Call(o.bar.hwnd, 1, hudTimerMs, 0)
		o.ticking = true
	case !running && o.ticking:
		procOvKillTimer.Call(o.bar.hwnd, 1)
		o.ticking = false
	}
}

func (o *perfOverlay) wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmTimer:
		o.update(true)
		if !o.app.Emu.State().Running {
			o.armTimer(false)
		}
		return 0
	case wmHUDState:
		o.armTimer(o.app.Emu.State().Running)
		o.update(true)
		o.stackAll()
		return 0
	}
	r, _, _ := procOvDefWindowProc.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

// stack puts a shown HUD window directly above its owner in the z-order: just below whatever
// window is above the owner, so it never covers another program that is in front of the game.
func (o *perfOverlay) stack(w *hudWin) {
	if !w.shown || w.owner == 0 {
		return
	}
	above, _, _ := procOvGetWindow.Call(w.owner, 3 /*GW_HWNDPREV*/)
	for above == o.bar.hwnd || above == o.rec.hwnd { // HUD windows already there
		if above == w.hwnd {
			return
		}
		above, _, _ = procOvGetWindow.Call(above, 3)
	}
	// above == 0 (HWND_TOP) when the owner is the top window
	procOvSetWindowPos.Call(w.hwnd, above, 0, 0, 0, 0,
		0x0001|0x0002|0x0010 /*SWP_NOSIZE|SWP_NOMOVE|SWP_NOACTIVATE*/)
}

func (o *perfOverlay) stackAll() {
	o.stack(&o.bar)
	o.stack(&o.rec)
}

func (o *perfOverlay) hide(w *hudWin) {
	if w.shown {
		procOvShowWindow.Call(w.hwnd, swHide)
		w.shown = false
	}
}

// own makes the HUD window belong to the window the game is drawn in.
func (o *perfOverlay) own(w *hudWin, owner uintptr) {
	if w.owner != owner {
		procOvSetWindowLongPtr.Call(w.hwnd, gwlpHwndParent, owner)
		w.owner = owner
		w.restack = true
	}
}

// update places (and with sample=true, refreshes) the strip and the REC indicator.
func (o *perfOverlay) update(sample bool) {
	a := o.app
	st := a.Emu.State()
	cfg := a.Settings.Get().PerfMonitor
	owner, area, visible := hudHost(a)
	o.top = owner
	o.host = a.UI.WindowHandle()
	if !st.Embedded {
		if g := a.Emu.GameWindow(); g != 0 {
			o.host = g
		}
	}
	playing := st.Running && st.Phase == "playing" && !st.Suspended && visible
	img := gameImageRect(area, hudAspect(a, st.GameKey))
	barH := int32(0)

	if playing && cfg.For(st.GameKey) {
		if sample || o.segs == nil {
			o.segs = perfSegments(perf.Sample(a.Emu), cfg.Metrics)
		}
		segs := o.segs
		if cfg.Layout == "fps" {
			segs = nil
			for _, s := range o.segs {
				if s.Kind == "fps" || s.Kind == "frame" {
					segs = append(segs, s)
				}
			}
			if len(segs) == 0 {
				segs = perfSegments(perf.Sample(a.Emu), []string{"fps"})
			}
		}
		barH = hudBarHeight(img, cfg.Scale)
		x, y, w := img.L, img.T, img.W()
		if cfg.Position == "bottom" {
			y = img.B - barH
		}
		bgAlpha := byte(clampF(float64(cfg.Opacity), 30, 100) * 255 / 100)
		o.own(&o.bar, owner)
		if cfg.Layout == "fps" { // compact: a small box in the picture's corner
			margin := barH / 3
			x, y = img.L+margin, y+margin
			w = 0 // fit the text
		}
		o.draw(&o.bar, x, y, w, barH, bgAlpha, segs, false)
		if cfg.Layout == "fps" {
			barH += barH / 3
		}
	} else {
		o.hide(&o.bar)
		o.segs = nil
	}

	if a.Recorder != nil && a.Recorder.Active() && playing {
		d := int(a.Recorder.Elapsed().Seconds())
		t := fmt.Sprintf("%02d:%02d", d/60, d%60)
		if d >= 3600 {
			t = fmt.Sprintf("%d:%02d:%02d", d/3600, d/60%60, d%60)
		}
		h := hudBarHeight(img, 100)
		margin := h / 2
		y := img.T + margin
		if cfg.Position != "bottom" {
			y += barH
		}
		o.own(&o.rec, owner)
		o.draw(&o.rec, img.R-margin, y, -1, h, 215, []PerfSeg{{Kind: "rec", Label: "●", Value: "REC", Unit: t}}, true)
	} else {
		o.hide(&o.rec)
	}
}

// HUD colours (NEXUS's own palette): labels by kind, values white, units grey.
var hudLabelColour = map[string]uintptr{"fps": rgb(100, 210, 255), "frame": rgb(100, 210, 255), "cpu": rgb(10, 132, 255),
	"gpu": rgb(48, 209, 88), "mem": rgb(191, 90, 242), "temp": rgb(255, 159, 10), "rec": rgb(255, 59, 48)}

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

func newHUDFont(px int32, weight int) uintptr {
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	f, _, _ := procOvCreateFont.Call(uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 4 /*ANTIALIASED*/, 0, uintptr(unsafe.Pointer(face)))
	return f
}

func textWidth(dc uintptr, s string) int32 {
	if s == "" {
		return 0
	}
	u, _ := windows.UTF16FromString(s)
	var sz [2]int32
	procOvGetTextExtentPoint.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz[0])))
	return sz[0]
}

func textOut(dc uintptr, x, y int32, s string, col uintptr) {
	if s == "" {
		return
	}
	u, _ := windows.UTF16FromString(s)
	procOvSetTextColor.Call(dc, col)
	procOvTextOut.Call(dc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1))
}

// draw renders segments into a per-pixel-alpha bitmap and shows it at (x, y). w>0: that width
// (text left-aligned); w==0: fitted to the text; w<0: fitted, with x as the right edge.
func (o *perfOverlay) draw(win *hudWin, x, y, w, h int32, bgAlpha byte, segs []PerfSeg, pill bool) {
	screen, _, _ := procOvGetDC.Call(0)
	dc, _, _ := procOvCreateCompatibleDC.Call(screen)
	procOvReleaseDC.Call(0, screen)
	defer procOvDeleteDC.Call(dc)
	labelF, valueF, unitF := newHUDFont(h*40/100, 700), newHUDFont(h*58/100, 700), newHUDFont(h*40/100, 600)
	defer procOvDeleteObject.Call(labelF)
	defer procOvDeleteObject.Call(valueF)
	defer procOvDeleteObject.Call(unitF)
	pad, gapLV, gapSeg := h*45/100, h*18/100, h*80/100

	// measure
	width := pad
	type pos struct{ l, v, u int32 }
	at := make([]pos, len(segs))
	for i, s := range segs {
		if i > 0 {
			width += gapSeg
			if s.Label == "" { // a temperature joined to the reading before it
				width -= gapSeg / 2
			}
		}
		procOvSelectObject.Call(dc, labelF)
		at[i].l = width
		if lw := textWidth(dc, s.Label); lw > 0 {
			width += lw + gapLV
		}
		procOvSelectObject.Call(dc, valueF)
		at[i].v = width
		width += textWidth(dc, s.Value)
		procOvSelectObject.Call(dc, unitF)
		at[i].u = width + h*8/100
		if uw := textWidth(dc, s.Unit); uw > 0 {
			width += h*8/100 + uw
		}
	}
	width += pad
	switch {
	case w < 0:
		x -= width
		w = width
	case w == 0:
		w = width
	}
	if w < 8 || h < 8 {
		o.hide(win)
		return
	}
	// bitmap (top-down BGRA)
	bi := ovBitmapInfo{Width: w, Height: -h, Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := procOvCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return
	}
	defer procOvDeleteObject.Call(bmp)
	oldBmp, _, _ := procOvSelectObject.Call(dc, bmp)
	defer procOvSelectObject.Call(dc, oldBmp)
	px := unsafe.Slice((*uint32)(bits), int(w)*int(h))
	const bg = 0x000E0F12 // BGR of the background (14,15,18)
	for i := range px {
		px[i] = bg
	}
	procOvSetBkMode.Call(dc, 1 /*TRANSPARENT*/)
	procOvSetTextAlign.Call(dc, 24 /*TA_BASELINE*/)
	base := h * 69 / 100
	for i, s := range segs {
		procOvSelectObject.Call(dc, labelF)
		textOut(dc, at[i].l, base, s.Label, hudLabelColour[s.Kind])
		procOvSelectObject.Call(dc, valueF)
		textOut(dc, at[i].v, base, s.Value, rgb(244, 245, 247))
		procOvSelectObject.Call(dc, unitF)
		textOut(dc, at[i].u, base, s.Unit, rgb(152, 156, 164))
	}
	// alpha: background translucent, text opaque; rounded ends for the pill and the compact box
	radius := int32(0)
	if pill || w < 2*h*6 {
		radius = h / 2
	}
	for yy := int32(0); yy < h; yy++ {
		for xx := int32(0); xx < w; xx++ {
			i := yy*w + xx
			c := px[i] & 0xFFFFFF
			a := uint32(255)
			if c == bg {
				a = uint32(bgAlpha)
			}
			if radius > 0 && outsideRound(xx, yy, w, h, radius) {
				a = 0
			}
			r, g, b := (c>>16)&0xFF*a/255, (c>>8)&0xFF*a/255, c&0xFF*a/255
			px[i] = a<<24 | r<<16 | g<<8 | b
		}
	}
	pt := [2]int32{x, y}
	size := [2]int32{w, h}
	src := [2]int32{0, 0}
	blend := uint32(0x01FF0000) // AC_SRC_OVER, 0, 255, AC_SRC_ALPHA
	procOvUpdateLayered.Call(win.hwnd, 0, uintptr(unsafe.Pointer(&pt[0])), uintptr(unsafe.Pointer(&size[0])), dc,
		uintptr(unsafe.Pointer(&src[0])), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	// Windows hides owned windows with their owner (minimise) and shows them again itself, so
	// whether it is on screen is asked, not remembered
	if vis, _, _ := procOvIsWindowVisible.Call(win.hwnd); vis == 0 || !win.shown {
		procOvShowWindow.Call(win.hwnd, swShowNoActivate)
		win.shown = true
		win.restack = true
	}
	if win.restack {
		win.restack = false
		o.stack(win)
	}
}

func outsideRound(x, y, w, h, r int32) bool {
	cx, cy := int32(-1), int32(-1)
	switch {
	case x < r && y < r:
		cx, cy = r, r
	case x >= w-r && y < r:
		cx, cy = w-r-1, r
	case x < r && y >= h-r:
		cx, cy = r, h-r-1
	case x >= w-r && y >= h-r:
		cx, cy = w-r-1, h-r-1
	default:
		return false
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy > r*r
}
