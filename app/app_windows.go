//go:build windows

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

func init() { runtime.LockOSThread() } // all window/COM work happens on the main thread

var (
	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procUpdateWindow      = user32.NewProc("UpdateWindow")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
	procMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW   = user32.NewProc("GetMonitorInfoW")
	procGetWindowRect     = user32.NewProc("GetWindowRect")
	procLoadImageW        = user32.NewProc("LoadImageW")
	procLoadCursorW       = user32.NewProc("LoadCursorW")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procMessageBoxW       = user32.NewProc("MessageBoxW")
	procFindWindowW       = user32.NewProc("FindWindowW")
	procIsZoomed          = user32.NewProc("IsZoomed")

	gdi32                = windows.NewLazySystemDLL("gdi32.dll")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")

	shell32                  = windows.NewLazySystemDLL("shell32.dll")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	ole32                    = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	comdlg32                 = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsPopup            = 0x80000000
	wsVisible          = 0x10000000
	gwlStyle           = ^uintptr(15) // -16
	swpNoZOrder        = 0x0004
	swpFrameChanged    = 0x0020
	swpShowWindow      = 0x0040
	wmApp              = 0x8000
	className          = "NEXUSWindow"
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}
type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
}
type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}
type minMaxInfo struct {
	PtReserved, PtMaxSize, PtMaxPosition, PtMinTrackSize, PtMaxTrackSize point
}

type winUI struct {
	app        *App
	hwnd       uintptr
	chromium   *edge.Chromium
	fullscreen bool
	saved      rect
	mu         sync.Mutex
	queue      []func()
	shown      bool
}

var theUI *winUI

func (u *winUI) Native() bool { return true }

// WindowHandle is the NEXUS main window; the engine's game window is owned by it.
func (u *winUI) WindowHandle() uintptr { return u.hwnd }

// dispatch runs fn on the UI thread.
func (u *winUI) dispatch(fn func()) {
	u.mu.Lock()
	u.queue = append(u.queue, fn)
	u.mu.Unlock()
	procPostMessageW.Call(u.hwnd, wmApp+1, 0, 0)
}

func (u *winUI) dispatchWait(fn func()) {
	done := make(chan struct{})
	u.dispatch(func() { fn(); close(done) })
	<-done
}

func (u *winUI) IsFullscreen() bool { return u.fullscreen }

func (u *winUI) SetFullscreen(on bool) { u.dispatch(func() { u.applyFullscreen(on) }) }

func (u *winUI) applyFullscreen(on bool) {
	if on {
		if !u.fullscreen {
			procGetWindowRect.Call(u.hwnd, uintptr(unsafe.Pointer(&u.saved)))
		}
		mon, _, _ := procMonitorFromWindow.Call(u.hwnd, 2 /*MONITOR_DEFAULTTONEAREST*/)
		mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		procSetWindowLongPtrW.Call(u.hwnd, gwlStyle, wsPopup|wsVisible)
		r := mi.RcMonitor
		procSetWindowPos.Call(u.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpFrameChanged|swpShowWindow)
	} else {
		procSetWindowLongPtrW.Call(u.hwnd, gwlStyle, wsOverlappedWindow|wsVisible)
		r := u.saved
		if r.Right-r.Left < 400 {
			s := u.app.Settings.Get().Display
			cx, _, _ := procGetSystemMetrics.Call(0)
			cy, _, _ := procGetSystemMetrics.Call(1)
			w, h := int32(s.Width), int32(s.Height)
			r = rect{(int32(cx) - w) / 2, (int32(cy) - h) / 2, (int32(cx)+w)/2 + w%2, (int32(cy)+h)/2 + h%2}
		}
		procSetWindowPos.Call(u.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpFrameChanged|swpShowWindow)
	}
	u.fullscreen = on
	if u.chromium != nil {
		u.chromium.Resize()
	}
	u.app.Events.Emit("app.fullscreen", on)
}

func (u *winUI) Minimize() { u.dispatch(func() { procShowWindow.Call(u.hwnd, 6 /*SW_MINIMIZE*/) }) }

func (u *winUI) Restore() {
	u.dispatch(func() {
		procShowWindow.Call(u.hwnd, 9 /*SW_RESTORE*/)
		if u.fullscreen {
			u.applyFullscreen(true)
		}
		focusWindow(u.hwnd)
		if u.chromium != nil {
			u.chromium.Focus()
		}
	})
}

func (u *winUI) Quit() { procPostMessageW.Call(u.hwnd, 0x0010 /*WM_CLOSE*/, 0, 0) }

// ---------- native pickers ----------

type browseInfo struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
}

func (u *winUI) PickFolder(title string) (string, error) {
	var result string
	var err error
	u.dispatchWait(func() {
		procCoInitializeEx.Call(0, 2 /*COINIT_APARTMENTTHREADED*/)
		name := make([]uint16, 260)
		t, _ := windows.UTF16PtrFromString(title)
		bi := browseInfo{HwndOwner: u.hwnd, PszDisplayName: &name[0], LpszTitle: t, UlFlags: 0x0001 | 0x0040 /*RETURNONLYFSDIRS|NEWDIALOGSTYLE*/}
		pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
		if pidl == 0 {
			err = errors.New("cancelled")
			return
		}
		defer procCoTaskMemFree.Call(pidl)
		buf := make([]uint16, 1024)
		if r, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
			err = errors.New("That location is not a normal folder.")
			return
		}
		result = windows.UTF16ToString(buf)
	})
	return result, err
}

type openFileNameW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

func (u *winUI) PickFile(title, filter string) (string, error) {
	var result string
	var err error
	u.dispatchWait(func() {
		buf := make([]uint16, 1024)
		f := utf16z(strings.ReplaceAll(filter, "|", "\x00") + "\x00")
		t, _ := windows.UTF16PtrFromString(title)
		ofn := openFileNameW{LpstrFilter: &f[0], LpstrFile: &buf[0], NMaxFile: 1024, LpstrTitle: t,
			HwndOwner: u.hwnd, Flags: 0x00001000 | 0x00000800 | 0x00080000 /*FILEMUSTEXIST|PATHMUSTEXIST|EXPLORER*/}
		ofn.LStructSize = uint32(unsafe.Sizeof(ofn))
		if r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
			err = errors.New("cancelled")
			return
		}
		result = windows.UTF16ToString(buf)
	})
	return result, err
}

func utf16z(s string) []uint16 {
	out := make([]uint16, 0, len(s)+1)
	for _, r := range s {
		out = append(out, uint16(r))
	}
	return append(out, 0)
}

// ---------- window procedure ----------

func wndProc(hwnd, m, wp, lp uintptr) uintptr {
	u := theUI
	if u == nil || u.hwnd == 0 {
		r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
		return r
	}
	switch m {
	case 0x0005: // WM_SIZE
		if u.chromium != nil {
			u.chromium.Resize()
		}
	case 0x0003, 0x0216: // WM_MOVE, WM_MOVING
		if u.chromium != nil {
			_ = u.chromium.NotifyParentWindowPositionChanged()
		}
	case 0x0006: // WM_ACTIVATE
		if wp&0xFFFF != 0 && u.chromium != nil {
			u.chromium.Focus()
		}
		u.app.Events.Emit("app.focus", wp&0xFFFF != 0)
	case 0x0024: // WM_GETMINMAXINFO
		mmi := (*minMaxInfo)(unsafe.Pointer(lp))
		mmi.PtMinTrackSize = point{960, 540}
		return 0
	case 0x0010: // WM_CLOSE
		procDestroyWindow.Call(hwnd)
		return 0
	case 0x0002: // WM_DESTROY
		procPostQuitMessage.Call(0)
		return 0
	case wmApp + 1:
		u.mu.Lock()
		q := u.queue
		u.queue = nil
		u.mu.Unlock()
		for _, fn := range q {
			fn()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

func messageBox(text, caption string, flags uintptr) int {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(caption)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return int(r)
}

func fatalBox(text string) { messageBox(text, "NEXUS", 0x10 /*MB_ICONERROR*/) }

// singleInstance returns false if NEXUS is already running from this folder
// (and brings the existing window to the front).
func singleInstance(root string) bool {
	h := sha1.Sum([]byte(strings.ToLower(root)))
	name, _ := windows.UTF16PtrFromString("Local\\NEXUS-" + hex.EncodeToString(h[:8]))
	_, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		cn, _ := windows.UTF16PtrFromString(className)
		if w, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cn)), 0); w != 0 {
			procShowWindow.Call(w, 9)
			procSetForegroundWindow.Call(w)
		}
		return false
	}
	return true
}

// runUI creates the NEXUS window with an embedded WebView2 and blocks until it closes.
func runUI(a *App, url string) {
	settings := a.Settings.Get()
	// Allow the boot sound to play without a click, and keep the UI smooth.
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--autoplay-policy=no-user-gesture-required --disable-features=msSmartScreenProtection")
	u := &winUI{app: a, fullscreen: false}
	theUI = u
	a.UI = u

	var hinst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinst)
	icon, _, _ := procLoadImageW.Call(uintptr(hinst), 2 /*icon group id from rsrc (manifest is 1)*/, 1 /*IMAGE_ICON*/, 0, 0, 0x40 /*LR_DEFAULTSIZE*/)
	cursor, _, _ := procLoadCursorW.Call(0, 32512 /*IDC_ARROW*/)
	brush, _, _ := procCreateSolidBrush.Call(0x00141210) // BGR of #101214
	cn, _ := windows.UTF16PtrFromString(className)
	wc := wndClassExW{LpfnWndProc: syscall.NewCallback(wndProc), HInstance: hinst, HIcon: windows.Handle(icon),
		HIconSm: windows.Handle(icon), HCursor: windows.Handle(cursor), HbrBackground: windows.Handle(brush), LpszClassName: cn}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	w, h := settings.Display.Width, settings.Display.Height
	if w < 960 || h < 540 {
		w, h = 1280, 760
	}
	cxr, _, _ := procGetSystemMetrics.Call(0)
	cyr, _, _ := procGetSystemMetrics.Call(1)
	cx, cy := int(cxr), int(cyr)
	if w > cx {
		w = cx
	}
	if h > cy {
		h = cy
	}
	title, _ := windows.UTF16PtrFromString("NEXUS")
	u.hwnd, _, _ = procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cn)), uintptr(unsafe.Pointer(title)), wsOverlappedWindow,
		uintptr((cx-w)/2), uintptr((cy-h)/2), uintptr(w), uintptr(h), 0, 0, uintptr(hinst), 0)
	if u.hwnd == 0 {
		fatalBox("NEXUS could not create its window.")
		return
	}
	a.UI = u
	if settings.Display.Fullscreen {
		u.applyFullscreen(true)
	} else {
		procShowWindow.Call(u.hwnd, 5 /*SW_SHOW*/)
	}
	procUpdateWindow.Call(u.hwnd)

	chromium := edge.NewChromium()
	chromium.DataPath = a.Paths.WebView
	chromium.MessageCallback = func(string) {}
	chromium.NavigationCompletedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {}
	chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)
	chromium.AcceleratorKeyCallback = func(vk uint) bool {
		if vk == 0x7A { // F11
			go func() {
				on := !u.fullscreen
				u.SetFullscreen(on)
				_, _ = a.Settings.Update(func(g *GlobalSettings) { g.Display.Fullscreen = on })
			}()
			return true
		}
		return false
	}
	if !chromium.Embed(u.hwnd) {
		a.Log.Printf("WebView2 unavailable; falling back to Edge app mode")
		procDestroyWindow.Call(u.hwnd)
		a.UI = noopUI{}
		fallbackBrowser(a, url)
		return
	}
	u.chromium = chromium
	if s, err := chromium.GetSettings(); err == nil {
		dev := os.Getenv("NEXUS_DEVTOOLS") == "1"
		_ = s.PutAreDefaultContextMenusEnabled(dev)
		_ = s.PutAreDevToolsEnabled(dev)
		_ = s.PutIsZoomControlEnabled(false)
		_ = s.PutIsStatusBarEnabled(false)
		_ = s.PutAreBrowserAcceleratorKeysEnabled(dev)
	}
	if c := chromium.GetController(); c != nil {
		if c2 := c.GetICoreWebView2Controller2(); c2 != nil {
			_ = c2.PutDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 255, R: 0x10, G: 0x12, B: 0x14})
		}
	}
	chromium.Resize()
	chromium.Navigate(url)
	chromium.Focus()

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// fallbackBrowser shows NEXUS in Microsoft Edge's app mode when the WebView2
// runtime is missing, and waits for the user to quit from the UI.
func fallbackBrowser(a *App, url string) {
	quit := make(chan struct{})
	a.UI = &fallbackUI{quit: quit}
	edgePaths := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	for _, p := range edgePaths {
		if fileExists(p) {
			args := []string{"--app=" + url, "--user-data-dir=" + filepath.Join(a.Paths.System, "EdgeProfile"),
				"--autoplay-policy=no-user-gesture-required", "--no-first-run"}
			if a.Settings.Get().Display.Fullscreen {
				args = append(args, "--start-fullscreen")
			}
			if err := exec.Command(p, args...).Start(); err == nil {
				<-quit
				return
			}
		}
	}
	messageBox(fmt.Sprintf("NEXUS needs the Microsoft Edge WebView2 Runtime, which is built into Windows 10 and 11.\n\n"+
		"Install it from https://go.microsoft.com/fwlink/p/?LinkId=2124703 and start NEXUS again.\n\nNEXUS is available at:\n%s", url), "NEXUS", 0x30)
}

type fallbackUI struct {
	noopUI
	quit chan struct{}
	once sync.Once
}

func (f *fallbackUI) Quit() { f.once.Do(func() { close(f.quit) }) }

// setHidden marks NEXUS's internal files in user-visible folders as hidden.
func setHidden(path string) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	if a, err := windows.GetFileAttributes(p); err == nil {
		_ = windows.SetFileAttributes(p, a|windows.FILE_ATTRIBUTE_HIDDEN)
	}
}

func clearHidden(path string) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	if a, err := windows.GetFileAttributes(p); err == nil && a&windows.FILE_ATTRIBUTE_HIDDEN != 0 {
		_ = windows.SetFileAttributes(p, a&^windows.FILE_ATTRIBUTE_HIDDEN)
	}
}
