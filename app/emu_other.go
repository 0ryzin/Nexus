//go:build !windows

package main

import (
	"os"
	"os/exec"
)

// Non-Windows builds exist for development and automated testing only.

type procWindow struct {
	HWND    uintptr
	Title   string
	Visible bool
}

func windowsForPID(pid int) []procWindow {
	// Test hook: a fake emulator can report its "window title" via a file.
	if p := os.Getenv("NEXUS_FAKE_TITLE_FILE"); p != "" {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			return []procWindow{{HWND: uintptr(pid), Title: string(b), Visible: true}}
		}
	}
	return nil
}
func closeWindow(hwnd uintptr) {
	if p, err := os.FindProcess(int(hwnd)); err == nil {
		_ = p.Signal(os.Interrupt)
	}
}
func focusWindow(uintptr)             {}
func windowTrackingUnavailable() bool { return os.Getenv("NEXUS_FAKE_TITLE_FILE") == "" }
func prepareCmd(*exec.Cmd)            {}
func vcRuntimeInstalled() bool        { return true }
func vcRuntimeAvailable(string) bool  { return true }
func vulkanLoaderPresent() bool       { return true }
func quitComboPressed() bool          { return false }
func xinputConnected() []int          { return nil }
func powerInfo() (bool, int, bool)    { return false, 0, false }
func guidePressed() bool              { return false }
