//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// steamRoots: Steam's install folders from the registry (wherever Steam was installed), then the
// usual folders.
func steamRoots() []string {
	var out []string
	for _, k := range []struct {
		root       registry.Key
		path, name string
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath"},
	} {
		if key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE); err == nil {
			if v, _, err := key.GetStringValue(k.name); err == nil && v != "" {
				out = append(out, filepath.Clean(filepath.FromSlash(v)))
			}
			key.Close()
		}
	}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if p := os.Getenv(env); p != "" {
			out = append(out, filepath.Join(p, "Steam"))
		}
	}
	var have []string
	for _, p := range sortedUnique(out) {
		if dirExists(p) {
			have = append(have, p)
		}
	}
	return have
}
