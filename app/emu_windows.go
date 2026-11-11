//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procGetWindow                = user32.NewProc("GetWindow")

	xinputDLL      *windows.LazyDLL
	procXInputGet  *windows.LazyProc
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	procGetPowerSt = kernel32.NewProc("GetSystemPowerStatus")
)

type procWindow struct {
	HWND    uintptr
	Title   string
	Visible bool
}

var enumCallback = syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
	ctx := (*enumCtx)(unsafe.Pointer(lparam))
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != ctx.pid {
		return 1
	}
	// only top-level, unowned windows
	if owner, _, _ := procGetWindow.Call(hwnd, 4 /*GW_OWNER*/); owner != 0 {
		return 1
	}
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	ctx.out = append(ctx.out, procWindow{HWND: hwnd, Title: windows.UTF16ToString(buf), Visible: vis != 0})
	return 1
})

type enumCtx struct {
	pid uint32
	out []procWindow
}

func windowsForPID(pid int) []procWindow {
	ctx := &enumCtx{pid: uint32(pid)}
	procEnumWindows.Call(enumCallback, uintptr(unsafe.Pointer(ctx)))
	return ctx.out
}

func closeWindow(hwnd uintptr) {
	procPostMessageW.Call(hwnd, 0x0010 /*WM_CLOSE*/, 0, 0)
}

var (
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procKeybdEvent          = user32.NewProc("keybd_event")
	procGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")
)

// focusWindow brings hwnd to the foreground even when another process owns the
// foreground (Windows' focus-stealing rules otherwise just flash the taskbar).
func focusWindow(hwnd uintptr) {
	if ic, _, _ := procIsIconic.Call(hwnd); ic != 0 {
		procShowWindow.Call(hwnd, 9 /*SW_RESTORE*/)
	}
	if r, _, _ := procSetForegroundWindow.Call(hwnd); r != 0 {
		return
	}
	fg, _, _ := procGetForegroundWindow.Call()
	fgThread, _, _ := procGetWindowThreadProcessId.Call(fg, 0)
	cur, _, _ := procGetCurrentThreadId.Call()
	if fgThread != 0 && fgThread != cur {
		procAttachThreadInput.Call(fgThread, cur, 1)
		defer procAttachThreadInput.Call(fgThread, cur, 0)
	}
	// a synthetic ALT tap marks this process as having received input
	procKeybdEvent.Call(0x12, 0, 0, 0)
	procKeybdEvent.Call(0x12, 0, 2 /*KEYEVENTF_KEYUP*/, 0)
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
}

func windowTrackingUnavailable() bool { return false }

func prepareCmd(cmd *exec.Cmd) {
	// Let the emulator take focus from NEXUS.
	procAllowSetForegroundWindow.Call(uintptr(0xFFFFFFFF)) // ASFW_ANY
	cmd.SysProcAttr = &syscall.SysProcAttr{}
}

// vcRuntimeInstalled checks for the MSVC 2015-2022 x64 runtime DLLs in Windows.
func vcRuntimeInstalled() bool { return vcRuntimeAvailable("") }

// vcRuntimeAvailable: the MSVC 2015-2022 x64 runtime is installed in Windows, or shipped
// app-locally with the engine (Engine\Libraries), which Microsoft permits for these files.
func vcRuntimeAvailable(engineDir string) bool {
	dirs := []string{filepath.Join(os.Getenv("SystemRoot"), "System32")}
	if engineDir != "" {
		dirs = append(dirs, filepath.Join(engineDir, "Libraries"), engineDir)
	}
	for _, d := range dirs {
		ok := true
		for _, dll := range []string{"vcruntime140.dll", "vcruntime140_1.dll", "msvcp140.dll"} {
			ok = ok && fileExists(filepath.Join(d, dll))
		}
		if ok {
			return true
		}
	}
	return false
}

// vulkanLoaderPresent: vulkan-1.dll is installed by every current GPU driver.
func vulkanLoaderPresent() bool {
	return fileExists(filepath.Join(os.Getenv("SystemRoot"), "System32", "vulkan-1.dll"))
}

// ---------- XInput (in-game quit shortcut + controller info) ----------

type xinputGamepad struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

type xinputState struct {
	Packet  uint32
	Gamepad xinputGamepad
}

func loadXInput() bool {
	if procXInputGet != nil {
		return procXInputGet.Find() == nil
	}
	for _, name := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
		d := windows.NewLazySystemDLL(name)
		if d.Load() == nil {
			xinputDLL = d
			procXInputGet = d.NewProc("XInputGetState")
			return procXInputGet.Find() == nil
		}
	}
	return false
}

// quitComboPressed reports whether Back+Start (View+Menu) is held on any XInput pad.
func quitComboPressed() bool {
	if !loadXInput() {
		return false
	}
	for i := uintptr(0); i < 4; i++ {
		var st xinputState
		r, _, _ := procXInputGet.Call(i, uintptr(unsafe.Pointer(&st)))
		if r == 0 && st.Gamepad.Buttons&0x0030 == 0x0030 { // START 0x10 | BACK 0x20
			return true
		}
	}
	return false
}

func xinputConnected() []int {
	var out []int
	if !loadXInput() {
		return out
	}
	for i := uintptr(0); i < 4; i++ {
		var st xinputState
		if r, _, _ := procXInputGet.Call(i, uintptr(unsafe.Pointer(&st))); r == 0 {
			out = append(out, int(i))
		}
	}
	return out
}

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// powerInfo returns (hasBattery, percent, charging).
func powerInfo() (bool, int, bool) {
	var s systemPowerStatus
	if r, _, _ := procGetPowerSt.Call(uintptr(unsafe.Pointer(&s))); r == 0 {
		return false, 0, false
	}
	if s.BatteryFlag == 128 || s.BatteryFlag == 255 || s.BatteryLifePercent == 255 {
		return false, 0, s.ACLineStatus == 1
	}
	return true, int(s.BatteryLifePercent), s.ACLineStatus == 1
}

func diskSpace(path string) (free, total uint64) {
	p, _ := windows.UTF16PtrFromString(path)
	var avail, tot, totFree uint64
	if windows.GetDiskFreeSpaceEx(p, &avail, &tot, &totFree) != nil {
		return 0, 0
	}
	return avail, tot
}

// guidePressed reports whether the Guide/Home (Xbox) button is held on any XInput pad. The public
// XInputGetState hides that button; xinput1_4.dll exports XInputGetStateEx as ordinal 100, which
// reports it as 0x0400. Returns false where that export is unavailable.
var (
	guideOnce sync.Once
	guideProc uintptr
)

func guidePressed() bool {
	guideOnce.Do(func() {
		h, err := windows.LoadLibraryEx("xinput1_4.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
		if err != nil {
			return
		}
		if p, err := windows.GetProcAddressByOrdinal(h, 100); err == nil {
			guideProc = p
		}
	})
	if guideProc == 0 {
		return false
	}
	for i := uintptr(0); i < 4; i++ {
		var st xinputState
		r, _, _ := syscall.SyscallN(guideProc, i, uintptr(unsafe.Pointer(&st)))
		if r == 0 && st.Gamepad.Buttons&0x0400 != 0 {
			return true
		}
	}
	return false
}
