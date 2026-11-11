package main

import (
	"encoding/json"
	"os"
)

// engineCheck is `NEXUS.exe --engine-check <result.json>`: it finds the engine exactly the way
// NEXUS does when it starts (the Engine folder next to NEXUS.exe), verifies every file against
// engine.json, loads nexus_engine.dll into this process and asks it for its build information.
// The result is written as JSON; the exit code is 0 only if the engine is ready to run games.
// Used by the release build to test the extracted ZIP, and by people diagnosing an install.
// User data is not touched: a throw-away data folder is used unless NEXUS_DATA is set.
func engineCheck(root, out string) int {
	res := map[string]any{"root": root, "ready": false}
	write := func(code int) int {
		data, _ := json.MarshalIndent(res, "", "  ")
		_ = os.WriteFile(out, data, 0o644)
		return code
	}
	data := os.Getenv("NEXUS_DATA")
	if data == "" {
		tmp, err := os.MkdirTemp("", "nexus-engine-check-")
		if err != nil {
			res["error"] = err.Error()
			return write(2)
		}
		defer os.RemoveAll(tmp)
		data = tmp
	}
	app, err := NewAppWithPaths(NewPathsSplit(root, data, false))
	if err != nil {
		res["error"] = err.Error()
		return write(2)
	}
	defer app.Close()
	res["bundledPresent"] = app.Emu.Builds.BundledPresent()
	res["version"] = app.Emu.Builds.ActiveVersionLabel()
	ok, msg := app.Emu.EngineAvailable()
	if !ok {
		res["error"] = msg
		return write(1)
	}
	h, err := app.Emu.Host()
	if err != nil {
		res["error"] = err.Error()
		return write(1)
	}
	info, err := h.Info()
	if err != nil {
		res["error"] = "the engine loaded but did not report its build information: " + err.Error()
		return write(1)
	}
	if info.API != engineAPIVersion {
		res["error"] = "the engine uses a different interface version"
		res["info"] = info
		return write(1)
	}
	res["info"] = info
	res["status"] = h.Status()
	res["ready"] = true
	return write(0)
}
