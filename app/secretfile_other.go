//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

// storeSecretFile keeps a secret (the NEXUS account sign-in token) in a file only the current
// user can read (0600 in a 0700 folder), the usual practice on Linux, Steam Deck and macOS
// where no common secret store is guaranteed. The token is revocable from any other device.
func storeSecretFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	_ = os.Chmod(filepath.Dir(path), 0o700)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadSecretFile(path string) ([]byte, error) { return os.ReadFile(path) }

const secretStorageName = "a file only your user account can read"
