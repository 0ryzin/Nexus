//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

// steamRoots: Steam's usual folders on Linux (Lossless Scaling itself is a Windows app; Proton
// installs keep it in the same library layout).
func steamRoots() []string {
	home, _ := os.UserHomeDir()
	var have []string
	for _, p := range []string{filepath.Join(home, ".steam", "steam"), filepath.Join(home, ".local", "share", "Steam")} {
		if dirExists(p) {
			have = append(have, p)
		}
	}
	return sortedUnique(have)
}
