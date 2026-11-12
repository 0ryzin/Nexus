//go:build windows

package main

import "os"

// storeSecretFile keeps a secret (the NEXUS account sign-in token) encrypted with DPAPI for the
// current Windows user: copying the file to another PC or user account does not reveal it.
func storeSecretFile(path string, data []byte) error {
	enc, err := protectSecret(data)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, enc)
}

func loadSecretFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return unprotectSecret(b)
}

const secretStorageName = "Windows data protection (only your Windows account can read it)"
