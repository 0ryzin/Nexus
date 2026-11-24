//go:build !windows

package main

// runXInputAssist: Windows only (XInput); elsewhere the browser's Gamepad API is used alone.
func (a *App) runXInputAssist() {}
