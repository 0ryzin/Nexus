package main

// Where NEXUS is running, from the environment: Steam's Gaming Mode, a Steam Deck, Steam.
// Used by the Linux window (app_linux.go) and the interface (/api/platform: on-screen keyboard).

import (
	"os"
	"runtime"
	"strings"
)

// gamingMode: NEXUS runs inside Steam's Gaming Mode (gamescope), e.g. on a Steam Deck.
func gamingMode() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("SteamGamepadUI") == "1" || os.Getenv("SteamDeck") == "1" && os.Getenv("XDG_CURRENT_DESKTOP") != "KDE" {
		return true
	}
	return strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gamescope") || os.Getenv("GAMESCOPE_WAYLAND_DISPLAY") != ""
}

// dmiRoot: where the firmware's machine description is (tests point it elsewhere).
var dmiRoot = "/sys/devices/virtual/dmi/id"

// steamDeck: this machine is a Steam Deck (Valve "Jupiter" LCD or "Galileo" OLED), in either
// mode. Desktop Mode has no physical keyboard either, so NEXUS treats both alike for typing.
func steamDeck() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	vendor, _ := os.ReadFile(dmiRoot + "/board_vendor")
	product, _ := os.ReadFile(dmiRoot + "/product_name")
	v, p := strings.TrimSpace(string(vendor)), strings.TrimSpace(string(product))
	return strings.EqualFold(v, "Valve") && (p == "Jupiter" || p == "Galileo")
}

// steamRunning: the Steam client runs for this user (its keyboard can be asked for).
func steamRunning() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{home + "/.steam/steam.pid", home + "/.local/share/Steam/steam.pid"} {
		if b, err := os.ReadFile(p); err == nil {
			if pid := strings.TrimSpace(string(b)); pid != "" && dirExists("/proc/"+pid) { // /proc/<pid> is a folder (fileExists was always false)
				return true
			}
		}
	}
	return false
}

// hostEnv is the environment for programs NEXUS starts on the desktop (xdg-open, dialogs,
// Steam's keyboard). Inside the AppImage it drops the settings that point the bundled GTK,
// GStreamer and GLib at the image (AppRun), which would break the desktop's own programs.
func hostEnv() []string {
	env := os.Environ()
	if os.Getenv("NEXUS_APPIMAGE_ENV") != "1" {
		return env
	}
	drop := []string{"GST_", "GTK_", "GDK_", "GI_TYPELIB_PATH=", "GSETTINGS_SCHEMA_DIR=", "GIO_", "APPDIR=", "NEXUS_APPIMAGE_ENV=", "NEXUS_HOST_", "XDG_DATA_DIRS="}
	out := env[:0:0]
	for _, kv := range env {
		keep := true
		for _, p := range drop {
			if strings.HasPrefix(kv, p) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, kv)
		}
	}
	if v := os.Getenv("NEXUS_HOST_XDG_DATA_DIRS"); v != "" {
		out = append(out, "XDG_DATA_DIRS="+v)
	}
	return out
}
