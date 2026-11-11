//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Development host: serves the UI and waits. Open the printed URL in a browser.
type devUI struct {
	noopUI
	quit chan struct{}
	once sync.Once
	fs   bool
}

func (d *devUI) Quit()                 { d.once.Do(func() { close(d.quit) }) }
func (d *devUI) SetFullscreen(on bool) { d.fs = on }
func (d *devUI) IsFullscreen() bool    { return d.fs }

func singleInstance(string) bool { return true }
func fatalBox(text string)       { fmt.Fprintln(os.Stderr, text) }

func runUI(a *App, url string) {
	d := &devUI{quit: make(chan struct{})}
	a.UI = d
	fmt.Println("NEXUS UI:", url)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-d.quit:
	}
}

func diskSpace(path string) (free, total uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}

func setHidden(string)   {}
func clearHidden(string) {}
