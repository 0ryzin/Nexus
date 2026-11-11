//go:build !windows

package main

import (
	"errors"
	"os"
)

// Non-Windows builds of NEXUS exist for development and automated tests only. The engine
// library is built for Windows; here a stand-in engine (fakeEngine) is used when
// NEXUS_FAKE_ENGINE=1, so the host side (sessions, play time, volume, errors) can be tested.
func loadEngineHost(dir string) (EngineHost, error) {
	if fakeEngineEnabled() {
		return newFakeEngine(), nil
	}
	return nil, errors.New("the NEXUS engine library is only available for Windows")
}

func fakeEngineEnabled() bool { return os.Getenv("NEXUS_FAKE_ENGINE") == "1" }
