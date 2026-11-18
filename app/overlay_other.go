//go:build !windows && !(linux && !android && webkitgtk)

package main

// The in-game overlay is a Windows window; development builds elsewhere only have the preview in
// Settings.
func (a *App) StartPerfOverlay() {}

func perfOverlayNote() string {
	return "The in-game strip is drawn on Windows, Linux and the Steam Deck; this build has no window of its own."
}
