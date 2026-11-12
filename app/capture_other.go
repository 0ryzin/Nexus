//go:build !windows

package main

// Recording uses Windows APIs; other platforms get their own capture backend when they are ported
// (docs/ARCHITECTURE.md). The fake backend below is only used by tests and the browser preview.
func newCaptureBackend(a *App) captureBackend {
	if fakeEngineEnabled() {
		return &fakeCapture{}
	}
	return nil
}
