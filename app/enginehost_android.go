//go:build android && cgo

package main

import (
	"errors"
	"os"
	"path/filepath"
)

// androidLibDir is the app's native library folder (ApplicationInfo.nativeLibraryDir), where
// Android installs libnexus_engine.so from the APK. Set by nativeStart.
var androidLibDir string

// On Android the engine is a shared library in the app's native library folder, loaded with the
// same C interface as on Windows (enginehost_dl.go). dir (an engine build folder) is used first
// when it contains a library, so engine builds can be installed beside the bundled one later.
func loadEngineHost(dir string) (EngineHost, error) {
	if fakeEngineEnabled() {
		return newFakeEngine(), nil
	}
	for _, d := range []string{dir, androidLibDir} {
		if d == "" {
			continue
		}
		p := filepath.Join(d, "libnexus_engine.so")
		if _, err := os.Stat(p); err == nil {
			return openDLEngine(p)
		}
	}
	return nil, errors.New("this development build of NEXUS for Android does not include the NEXUS Engine yet")
}

func fakeEngineEnabled() bool { return os.Getenv("NEXUS_FAKE_ENGINE") == "1" }

// androidEngineBundled reports whether the APK contains the engine library.
func androidEngineBundled() bool {
	if androidLibDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(androidLibDir, "libnexus_engine.so"))
	return err == nil
}
