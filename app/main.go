package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// startupBegan is when this NEXUS.exe started; startup log lines are timed from it.
var startupBegan = time.Now()

func main() {
	defer func() {
		// a panic in a GUI program has no console to print to: say so instead of vanishing
		if r := recover(); r != nil {
			fatalBox(fmt.Sprintf("NEXUS stopped while starting or running: %v\n\nThe log is nexus.log in the Logs folder of the NEXUS data folder.", r))
			panic(r)
		}
	}()
	if len(os.Args) >= 2 && os.Args[1] == "--apply-update" {
		os.Exit(applyUpdate(os.Args[2:])) // the new NEXUS.exe installing itself (updater.go)
	}
	root := resolveRoot()
	if len(os.Args) >= 3 && os.Args[1] == "--engine-check" {
		os.Exit(engineCheck(root, os.Args[2]))
	}
	if !singleInstance(root) {
		return
	}
	data, portable := resolveData(root)
	port, _ := strconv.Atoi(os.Getenv("NEXUS_PORT"))
	core, err := StartCore(CoreOptions{Root: root, Data: data, Portable: portable, Port: port, Args: os.Args})
	if err != nil {
		fatalBox(err.Error())
		os.Exit(1)
	}
	defer core.Close()
	app, url := core.App, core.URL
	app.Log.Printf("startup: core ready after %v (pid %d)", time.Since(startupBegan).Round(time.Millisecond), os.Getpid())
	go app.refreshSteamArtwork() // Linux: NEXUS's own Steam entries get this version's artwork
	runUI(app, url)
	// make sure a running game isn't orphaned silently
	if st := app.Emu.State(); st.Running {
		app.Log.Printf("NEXUS closed while %q was running; the emulator keeps running", st.Title)
		app.Emu.crash.end() // closed on purpose: not a crash
	}
	app.Log.Printf("NEXUS exited")
}

// CoreOptions: where NEXUS keeps its program and data, and the interface port (0 = any free).
type CoreOptions struct {
	Root, Data string
	Portable   bool
	Port       int
	Args       []string // command line (update hand-over flags)
	GamesDir   string   // the Games folder, if not <Root>\Games (Android: shared storage)
}

// Core is a running NEXUS: data, library, engine host and the local interface server. Every
// platform starts it the same way; only the window that shows the interface differs (WebView2
// on Windows, the Android app's WebView on Android).
type Core struct {
	App     *App
	Server  *Server
	URL     string
	closers []func()
}

func (c *Core) Close() {
	for i := len(c.closers) - 1; i >= 0; i-- {
		c.closers[i]()
	}
	c.App.Close()
}

// StartCore prepares NEXUS's data, starts the background work and the interface server.
func StartCore(o CoreOptions) (*Core, error) {
	paths := NewPathsSplit(o.Root, o.Data, o.Portable)
	if o.GamesDir != "" {
		paths.Games = o.GamesDir
	} else if os.Getenv("APPIMAGE") != "" && !o.Portable {
		// an AppImage runs from a read-only image mounted in a new place each time: the Games
		// folder lives with the user's data (~/.local/share/NEXUS/Games) instead
		paths.Games = filepath.Join(o.Data, "Games")
	}
	app, err := NewAppWithPaths(paths)
	if err != nil {
		return nil, fmt.Errorf("NEXUS could not start.\n\n%v", err)
	}
	c := &Core{App: app}
	app.installCrashCapture()
	// the last game session ended without NEXUS seeing it end: report it (crashreport.go). This
	// runs before the engine is loaded, while its log is still the crashed session's.
	app.setCrashNotice(app.checkLastSession())
	// a user's earlier private shader cache goes into the shared one (once; sharedcache.go)
	go app.Emu.shareUserCaches()
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

	// saves: one-time conversion to the 0.3.0 layout (with a backup first), then adopt saves an
	// older engine wrote in its own layout
	if err := app.Saves.Migrate(); err != nil {
		app.Log.Printf("saves: %v", err)
		app.saveMigrationErr = err.Error()
	}
	app.Saves.TidyAll()
	app.Library.RemoveImportedGameListCache()
	app.Recorder.RecoverInterrupted() // recordings cut off by a crash: kept, playable
	app.Emu.cleanDLSSPartials()       // an interrupted DLSS download leaves no half files
	// started by the updater (updater.go): use the engine that came with the update and report a
	// good start, or show why an update was rolled back
	for i := 1; i+1 < len(o.Args); i++ {
		switch o.Args[i] {
		case "--update-applied":
			app.preferBundledEngine()
			app.confirmUpdateStart(o.Args[i+1])
		case "--update-failed":
			app.updateFailed = o.Args[i+1]
			app.Log.Printf("update: rolled back (%s)", o.Args[i+1])
		}
	}

	// play time from a session interrupted by a crash of NEXUS or the PC
	if rec := app.Profiles.RecoverLiveSession(); rec != nil {
		app.Log.Printf("recovered an interrupted play session of %ds", rec.Seconds)
	}
	if app.Settings.Get().Library.ScanOnStart {
		go func() {
			if err := app.Library.Scan(); err != nil {
				app.Log.Printf("initial scan: %v", err)
			}
			app.markStartup("library scanned", fmt.Sprintf("%d games", len(app.Library.Games())))
			app.Saves.TidyAll() // names for games found by the scan
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
	// experimental: NEXUS Device Linking (off unless switched on, see features.go)
	if app.Feature("devicelink") {
		if l, err := NewDeviceLink(app); err != nil {
			app.Log.Printf("device link: %v", err)
		} else if err := l.Listen(linkDefaultPort); err != nil {
			app.Log.Printf("device link: %v", err)
		} else {
			app.Link = l
			c.closers = append(c.closers, func() { l.Close() })
		}
	}
	go app.Account.Run() // background synchronisation; idle unless signed in with sync on
	go app.Discord.Run() // Discord Rich Presence; nothing is sent unless it is turned on
	go func() {          // keys and firmware kept per computer user, whichever copy of NEXUS runs
		to, from, other := app.Emu.SyncSystemVault()
		if to+from > 0 || other != "" {
			app.Log.Printf("system files: %d saved to %s, %d restored from it%s", to, systemVaultDir(), from, map[bool]string{true: " (found in " + other + ")", false: ""}[other != ""])
			app.Events.Emit("engine.changed", nil)
		}
	}()
	c.closers = append(c.closers, app.Account.Close)
	srv := NewServer(app)
	url, err := srv.Start(o.Port)
	if err != nil {
		app.Close()
		return nil, fmt.Errorf("NEXUS could not start its local interface server: %v", err)
	}
	c.Server, c.URL = srv, url
	app.Log.Printf("UI server at %s", srv.ln.Addr())
	// Load and verify the bundled engine in the background while the interface starts, so the
	// first game launch does not wait for it and the engine status is ready on the home screen.
	app.Updater.CheckInBackground()
	app.StartPerfOverlay() // in-game performance bar (idle unless a game is in front and it is on)
	app.Emu.CheckEngineInBackground(func(ok bool, msg string) {
		if !ok {
			app.Log.Printf("engine: %s", msg)
		} else {
			app.Log.Printf("engine: ready (%s)", app.Emu.Builds.ActiveVersionLabel())
		}
		app.Events.Emit("engine.ready", map[string]any{"installed": ok, "problem": msg})
	})
	return c, nil
}
