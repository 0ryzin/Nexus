//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Development builds for other systems keep no secrets (tokens need Windows' DPAPI).
func protectSecret(data []byte) ([]byte, error) {
	return nil, errors.New("storing an access token needs Windows")
}

func unprotectSecret(data []byte) ([]byte, error) { return nil, errors.New("not available") }

func waitForExit(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p, err := os.FindProcess(pid)
		if err != nil || p.Signal(syscall.Signal(0)) != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
