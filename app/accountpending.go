package main

import (
	"sync"
	"time"
)

// pendingCache keeps Status from waiting on the count of local changes. Working that out reads
// every synchronised item (profiles, saves, artwork) and could wait behind a synchronisation
// that was sending pictures; on Tom's PC (0.8.1–0.9.2) that left the account page blank and
// the account indicator on Home missing while a long synchronisation ran.
type pendingCache struct {
	mu      sync.Mutex
	n       int
	known   bool
	running chan struct{} // closed when the count being worked out is ready
}

// pendingWait: how long Status waits for a fresh count before answering with the last one.
var pendingWait = 150 * time.Millisecond

func (ac *Account) pendingCount() int {
	c := &ac.pend
	c.mu.Lock()
	done := c.running
	if done == nil {
		done = make(chan struct{})
		c.running = done
		go func() {
			n := len(ac.pendingKeys())
			c.mu.Lock()
			changed := !c.known || c.n != n
			c.n, c.known, c.running = n, true, nil
			c.mu.Unlock()
			close(done)
			if changed && ac.app != nil {
				ac.changed() // the interface asks again and gets the new count at once
			}
		}()
	}
	c.mu.Unlock()
	select {
	case <-done:
	case <-time.After(pendingWait):
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
