//go:build !windows

package main

import "errors"

// Development builds for other systems cannot check Windows code signatures: nothing is accepted.
func checkNVIDIASignature(path string) error {
	return errors.New("code signatures can only be checked on Windows")
}
