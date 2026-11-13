//go:build windows

package main

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// testWindow creates a hidden top-level window of class cls with the default window procedure.
func testWindow(t *testing.T, cls string) uintptr {
	t.Helper()
	var hinst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinst)
	cn, _ := windows.UTF16PtrFromString(cls)
	wc := wndClassExW{LpfnWndProc: procDefWindowProcW.Addr(), HInstance: hinst, LpszClassName: cn}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := windows.UTF16PtrFromString("NEXUS window test")
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cn)), uintptr(unsafe.Pointer(title)), wsOverlappedWindow,
		100, 100, 1280, 760, 0, 0, uintptr(hinst), 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowEx: %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(hwnd) })
	return hwnd
}

// TestStartFullscreenWindowIsShown is the regression test for "NEXUS started without a window":
// putting a never-shown window into full screen must not mark it visible by hand, and showing
// it afterwards must make it visible.
func TestStartFullscreenWindowIsShown(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app, _ := newTestApp(t, "")
	u := &winUI{app: app, hwnd: testWindow(t, "NEXUSWindowTest1")}

	u.fsWant = true
	u.applyFullscreen(true)
	if s := readWindowState(u.hwnd); s.Visible {
		t.Fatal("full screen set WS_VISIBLE on a window that was never shown (the invisible-window bug)")
	}
	style, _, _ := procGetWindowLongPtrW.Call(u.hwnd, gwlStyle)
	if style&wsPopup == 0 {
		t.Fatalf("not in full screen style: %#x", style)
	}
	u.showMainWindow()
	if s := readWindowState(u.hwnd); !s.Visible || !s.OnMonitor {
		t.Fatalf("after showing: %+v", s)
	}
	// leaving full screen keeps it visible and windowed
	u.fsWant = false
	u.applyFullscreen(false)
	style, _, _ = procGetWindowLongPtrW.Call(u.hwnd, gwlStyle)
	if s := readWindowState(u.hwnd); !s.Visible || style&wsPopup != 0 {
		t.Fatalf("after leaving full screen: %+v style %#x", s, style)
	}
}

// TestEnsureWindowVisibleRecovers: a hidden, minimised or off-screen window is brought back.
func TestEnsureWindowVisibleRecovers(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app, _ := newTestApp(t, "")
	u := &winUI{app: app, hwnd: testWindow(t, "NEXUSWindowTest2")}
	u.showMainWindow()

	procShowWindow.Call(u.hwnd, 0 /*SW_HIDE*/)
	u.ensureWindowVisible("test: hidden", false)
	if !readWindowState(u.hwnd).Visible {
		t.Fatal("hidden window not shown again")
	}
	off := int32(-30000)
	procSetWindowPos.Call(u.hwnd, 0, uintptr(off), uintptr(off), 1280, 760, swpNoZOrder)
	if readWindowState(u.hwnd).OnMonitor {
		t.Skip("this desktop has a monitor at -30000; cannot test off-screen recovery")
	}
	u.ensureWindowVisible("test: off screen", false)
	if s := readWindowState(u.hwnd); !s.OnMonitor {
		t.Fatalf("off-screen window not brought back: %+v", s)
	}
	procShowWindow.Call(u.hwnd, 6 /*SW_MINIMIZE*/)
	u.ensureWindowVisible("test: minimised, started again", true)
	if s := readWindowState(u.hwnd); s.Minimised || !s.Visible {
		t.Fatalf("minimised window not restored when NEXUS was started again: %+v", s)
	}
}

// TestSecondInstanceFindsWindow: a second NEXUS.exe finds the running window (and gives up,
// rather than hang, when there is none).
func TestSecondInstanceFindsWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if activateRunningInstance(0) {
		t.Skip("a NEXUS window is open on this desktop")
	}
	testWindow(t, className)
	if !activateRunningInstance(0) {
		t.Fatal("running NEXUS window not found")
	}
}

func TestWindowStyleFor(t *testing.T) {
	if windowStyleFor(0, true) != wsPopup || windowStyleFor(0, false) != wsOverlappedWindow {
		t.Fatal("hidden window got WS_VISIBLE")
	}
	if windowStyleFor(wsVisible|wsOverlappedWindow, true) != wsPopup|wsVisible {
		t.Fatal("visible window lost WS_VISIBLE")
	}
}

// TestHUDStacksAboveItsOwner is the regression test for "the performance bar only appeared after
// F11": a HUD window whose owner is set after creation is put directly above the owner, and not
// above another window that is in front of the owner.
func TestHUDStacksAboveItsOwner(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app, _ := newTestApp(t, "")
	owner := testWindow(t, "NEXUSWindowTest3")
	other := testWindow(t, "NEXUSWindowTest4")
	hud := testWindow(t, "NEXUSWindowTest5")
	for _, w := range []uintptr{hud, owner, other} { // hud first: it starts below the owner
		procShowWindow.Call(w, swShowNoActivate)
	}
	procSetWindowPos.Call(other, 0, 0, 0, 0, 0, 0x0001|0x0002|0x0010)
	procSetWindowPos.Call(hud, 1 /*HWND_BOTTOM*/, 0, 0, 0, 0, 0x0001|0x0002|0x0010)
	prev := func(w uintptr) uintptr { p, _, _ := procOvGetWindow.Call(w, 3); return p }
	if prev(owner) == hud {
		t.Fatal("test setup: hud already above the owner")
	}

	o := &perfOverlay{app: app}
	o.bar = hudWin{hwnd: hud, shown: true}
	o.own(&o.bar, owner)
	o.stack(&o.bar)
	if prev(owner) != hud {
		t.Fatalf("HUD not directly above its owner (window above the owner: %#x, hud %#x)", prev(owner), hud)
	}
	// the other window was in front of the owner and stays in front of the HUD too
	for w := hud; w != 0; w = prev(w) {
		if w == other {
			return
		}
	}
	t.Fatal("HUD was put above a window that was in front of the game")
}
