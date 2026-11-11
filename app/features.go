package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Experimental features are off in every normal NEXUS and invisible in its interface. A developer
// or tester switches one on with the environment variable NEXUS_EXPERIMENTAL (comma-separated
// names) or a line in <data>\System\experimental-features.txt.
//
//	devicelink — NEXUS Device Linking and local synchronisation (devicelink.go, linksync.go)
func (a *App) Feature(name string) bool {
	for _, f := range strings.Split(os.Getenv("NEXUS_EXPERIMENTAL"), ",") {
		if strings.EqualFold(strings.TrimSpace(f), name) {
			return true
		}
	}
	data, err := os.ReadFile(filepath.Join(a.Paths.System, "experimental-features.txt"))
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.EqualFold(strings.TrimSpace(l), name) {
			return true
		}
	}
	return false
}
