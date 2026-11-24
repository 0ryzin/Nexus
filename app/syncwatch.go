package main

// syncWatchdog: a synchronisation that has not finished after two minutes is written down with
// what every part of NEXUS is doing at that moment (goroutine stacks: function names only, no
// file contents, keys or sign-ins), so a stuck sync can be diagnosed from the log folder.
// 0.8.1 on the owner's PC started a sync that never finished and left nothing in the log.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"
)

var syncStuckAfter = 2 * time.Minute

func (a *App) syncWatchdog(what string, started time.Time, done <-chan struct{}) {
	select {
	case <-done:
		return
	case <-time.After(syncStuckAfter):
	}
	name := filepath.Join(a.Paths.Logs, fmt.Sprintf("sync-stuck-%s.txt", started.Format("20060102-150405")))
	f, err := os.Create(name)
	if err != nil {
		a.Log.Printf("%s: synchronisation still running after %s (could not write details: %v)", what, syncStuckAfter, err)
		return
	}
	fmt.Fprintf(f, "NEXUS %s: %s synchronisation started %s and has not finished after %s.\n\n", AppVersion, what, started.Format(time.RFC3339), syncStuckAfter)
	_ = pprof.Lookup("goroutine").WriteTo(f, 2)
	f.Close()
	a.Log.Printf("%s: synchronisation still running after %s; what NEXUS was doing is in %s", what, syncStuckAfter, name)
}
