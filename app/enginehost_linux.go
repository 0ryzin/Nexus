//go:build linux && !android && cgo

package main

import (
	"errors"
	"os"
	"path/filepath"
)

// On Linux (PCs and the Steam Deck) the engine is Engine/nexus_engine.so, loaded with the same C
// interface as on Windows (enginehost_dl.go). The libraries it needs are next to it (its RUNPATH
// is $ORIGIN, set when NEXUS is packaged). The stand-in engine is used for development and tests.
func loadEngineHost(dir string) (EngineHost, error) {
	if fakeEngineEnabled() {
		return newFakeEngine(), nil
	}
	p := filepath.Join(dir, engineLibName)
	if _, err := os.Stat(p); err != nil {
		return nil, errors.New("the NEXUS engine is missing from the NEXUS folder (Engine/nexus_engine.so). Extract the complete NEXUS download again, keeping all folders together")
	}
	return openDLEngine(p)
}

func fakeEngineEnabled() bool { return os.Getenv("NEXUS_FAKE_ENGINE") == "1" }
