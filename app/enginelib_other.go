//go:build !windows

package main

// engineLibName is the NEXUS engine library in an engine folder (Linux; Android installs its
// own copy as libnexus_engine.so, enginehost_android.go).
const engineLibName = "nexus_engine.so"
