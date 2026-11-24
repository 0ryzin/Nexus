package main

import (
	"os"
	"time"
	"unsafe"
)

// XInput D-pad assist (0.9.3, docs: "D-pad can't navigate", Tom's Windows PC). The menus read
// controllers through WebView2's Gamepad API. Where that delivers the controller but not its
// D-pad, NEXUS reads the D-pad of XInput controllers itself and sends the changes to the
// interface ("pad.dpad"), which uses them only while the browser has never reported a D-pad
// press (web/app.js XAssist), so a press is never acted on twice. Only while NEXUS's own window
// is in front and no game is shown: the game has the controller then.
func (a *App) runXInputAssist() {
	if !loadXInput() {
		return
	}
	me := uint32(os.Getpid())
	var last uint16
	t := time.NewTicker(16 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		var mask uint16
		if a.nexusInFront(me) {
			for i := uintptr(0); i < 4; i++ {
				var st xinputState
				if r, _, _ := procXInputGet.Call(i, uintptr(unsafe.Pointer(&st))); r == 0 {
					mask |= st.Gamepad.Buttons & 0x000F // DPAD_UP 1, DOWN 2, LEFT 4, RIGHT 8
				}
			}
		}
		if mask != last {
			last = mask
			a.Events.Emit("pad.dpad", map[string]any{"up": mask&1 != 0, "down": mask&2 != 0, "left": mask&4 != 0, "right": mask&8 != 0})
		}
	}
}

// nexusInFront: the foreground window is NEXUS's and no game is in front.
func (a *App) nexusInFront(me uint32) bool {
	if a.Emu != nil {
		if st := a.Emu.State(); st.Running && !st.Suspended {
			return false
		}
	}
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(fg, uintptr(unsafe.Pointer(&pid)))
	return pid == me
}
