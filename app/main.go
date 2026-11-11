package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	root := resolveRoot()
	if len(os.Args) >= 3 && os.Args[1] == "--engine-check" {
		os.Exit(engineCheck(root, os.Args[2]))
	}
	if !singleInstance(root) {
		return
	}
	data, portable := resolveData(root)
	app, err := NewAppWithPaths(NewPathsSplit(root, data, portable))
	if err != nil {
		fatalBox(fmt.Sprintf("NEXUS could not start.\n\n%v", err))
		os.Exit(1)
	}
	defer app.Close()
	app.migrateToDataDir()
	// the engine's data folder and configuration belong to NEXUS: create them if missing (other
	// emulators' data is only imported when the user chooses to, see enginedata.go)
	if err := app.Emu.EnsureEngineData(); err != nil {
		app.Log.Printf("engine data: %v", err)
	}
	for _, w := range app.Sys.EnsureEngine() {
		app.Log.Printf("system files: %s", w)
	}
	if len(app.migrated) > 0 {
		// reload stores that may have been created empty before migration
		if s, err := LoadSettings(app.Paths); err == nil {
			app.Settings = s
		}
		if pr, err := LoadProfiles(app.Paths); err == nil {
			app.Profiles = pr
		}
		app.Library = NewLibrary(app)
	}

	// readable save folders: brings back folders of a crashed session, migrates old "<id>" folders
	app.Saves.OrganizeAll()

	// play time from a session interrupted by a crash of NEXUS or the PC
	if rec := app.Profiles.RecoverLiveSession(); rec != nil {
		app.Log.Printf("recovered an interrupted play session of %ds", rec.Seconds)
	}
	if app.Settings.Get().Library.ScanOnStart {
		go func() {
			if err := app.Library.Scan(); err != nil {
				app.Log.Printf("initial scan: %v", err)
			}
			app.Saves.OrganizeAll() // names for games found by the scan
		}()
	}
	// Profile icons: extract the console's own icons from the user's firmware
	// automatically (same source as Eden/Lichii's avatar picker), and keep
	// checking until firmware + keys are available.
	go func() {
		for i := 0; ; i++ {
			st := app.Avatars.Ensure(false)
			if st.State == "ready" && i > 0 {
				// still re-check occasionally in case the firmware is updated
				time.Sleep(5 * time.Minute)
			} else {
				time.Sleep(20 * time.Second)
			}
		}
	}()
	port, _ := strconv.Atoi(os.Getenv("NEXUS_PORT"))
	srv := NewServer(app)
	url, err := srv.Start(port)
	if err != nil {
		fatalBox("NEXUS could not start its local interface server: " + err.Error())
		os.Exit(1)
	}
	app.Log.Printf("UI server at %s", srv.ln.Addr())
	// Load and verify the bundled engine in the background while the interface starts, so the
	// first game launch does not wait for it and the engine status is ready on the home screen.
	app.Emu.CheckEngineInBackground(func(ok bool, msg string) {
		if !ok {
			app.Log.Printf("engine: %s", msg)
		} else {
			app.Log.Printf("engine: ready (%s)", app.Emu.Builds.ActiveVersionLabel())
		}
		app.Events.Emit("engine.ready", map[string]any{"installed": ok, "problem": msg})
	})
	runUI(app, url)
	// make sure a running game isn't orphaned silently
	if st := app.Emu.State(); st.Running {
		app.Log.Printf("NEXUS closed while %q was running; the emulator keeps running", st.Title)
	}
	app.Log.Printf("NEXUS exited")
}
