package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	// NEXUS sets up the engine's data folder and configuration itself (as at every start)
	if err := app.Emu.EnsureEngineData(); err != nil {
		res["error"] = err.Error()
		return write(1)
	}
	res["engineConfig"] = fileExists(app.Emu.ConfigPath())
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
	// --probe-start: let the engine prepare a session for a game file that does not exist. The
	// engine creates its configuration in NEXUS's folder first and only then fails to load the
	// game, so this proves a session can be prepared (for example, that no other emulator's data
	// on this PC blocks it) without needing a game.
	if len(os.Args) > 3 && os.Args[3] == "--probe-start" {
		cfg, err := app.Emu.writeSessionConfig(nil, nil)
		if err != nil {
			res["error"] = "the session configuration could not be written: " + err.Error()
			return write(1)
		}
		// no GPU is needed: the probe session uses the engine's null renderer
		if ini, err := LoadIni(cfg); err == nil {
			ini.SetSetting("Renderer", "backend", "2")
			_ = ini.Save(cfg)
		}
		req := EngineRequest{Game: filepath.Join(os.TempDir(), "nexus-probe-missing-game.nsp"), DataDir: app.Emu.DataDir(), ConfigFile: cfg, Title: "NEXUS probe"}
		if err := h.Start(req); err != nil {
			res["error"] = "the engine refused the session: " + err.Error()
			return write(1)
		}
		var st EngineStatus
		for i := 0; i < 600; i++ {
			st = h.Status()
			if st.State == "error" || st.State == "stopped" || st.State == "idle" && i > 4 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		h.Stop(true)
		res["probe"] = st
		if strings.Contains(st.Message, "configuration") || strings.Contains(st.Message, "import") || strings.Contains(st.Message, "Legacy") {
			res["error"] = "the engine could not prepare a session: " + st.Message
			return write(1)
		}
	}
	res["ready"] = true
	return write(0)
}
