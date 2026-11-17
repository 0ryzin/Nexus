//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

// defaultSharedShaderDir: none outside Windows (NEXUS_SHARED_SHADER_DIR can name one).
func defaultSharedShaderDir() string { return "" }

func prepareSharedDir(d string) error {
	if err := os.MkdirAll(d, 0o777); err != nil {
		return err
	}
	probe := filepath.Join(d, ".nexus-write-test")
	if err := os.WriteFile(probe, nil, 0o666); err != nil {
		return err
	}
	return os.Remove(probe)
}
