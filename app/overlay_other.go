//go:build !windows

package main

// The in-game overlay is a Windows window; development builds elsewhere only have the preview in
// Settings.
func (a *App) StartPerfOverlay() {}
