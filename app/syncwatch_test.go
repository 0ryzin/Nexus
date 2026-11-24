package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncWatchdog(t *testing.T) {
	a, _ := newTestApp(t, "")
	old := syncStuckAfter
	syncStuckAfter = 50 * time.Millisecond
	defer func() { syncStuckAfter = old }()
	done := make(chan struct{})
	a.syncWatchdog("account", time.Now(), done)
	m, _ := filepath.Glob(filepath.Join(a.Paths.Logs, "sync-stuck-*.txt"))
	if len(m) != 1 {
		t.Fatalf("no details written: %v", m)
	}
	b, _ := os.ReadFile(m[0])
	if !strings.Contains(string(b), "goroutine") || !strings.Contains(string(b), "syncWatchdog") {
		t.Fatal("no stacks in the details")
	}
	close(done)
}
