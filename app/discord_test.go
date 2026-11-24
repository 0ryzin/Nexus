//go:build !windows

package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// a stand-in Discord app: answers the handshake and records each SET_ACTIVITY
type fakeDiscordIPC struct {
	mu       sync.Mutex
	clientID string
	acts     []map[string]any // nil entry: the presence was cleared
}

func (f *fakeDiscordIPC) serve(t *testing.T, ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			for {
				var h [8]byte
				if _, err := io.ReadFull(c, h[:]); err != nil {
					return
				}
				body := make([]byte, binary.LittleEndian.Uint32(h[4:]))
				io.ReadFull(c, body)
				var m map[string]any
				json.Unmarshal(body, &m)
				f.mu.Lock()
				if binary.LittleEndian.Uint32(h[:4]) == 0 {
					f.clientID, _ = m["client_id"].(string)
				} else if args, ok := m["args"].(map[string]any); ok {
					act, _ := args["activity"].(map[string]any)
					f.acts = append(f.acts, act)
				}
				f.mu.Unlock()
				out, _ := json.Marshal(map[string]any{"evt": "READY", "cmd": "DISPATCH"})
				var oh [8]byte
				binary.LittleEndian.PutUint32(oh[:4], 1)
				binary.LittleEndian.PutUint32(oh[4:], uint32(len(out)))
				c.Write(append(oh[:], out...))
			}
		}(c)
	}
}

func TestDiscordPresence(t *testing.T) {
	dir, _ := os.MkdirTemp("/tmp", "dipc") // short path: unix socket names are limited
	defer os.RemoveAll(dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	ln, err := net.Listen("unix", filepath.Join(dir, "discord-ipc-0"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	f := &fakeDiscordIPC{}
	go f.serve(t, ln)

	a, _ := newTestApp(t, "")
	d := NewDiscord(a)
	setRunning := func(title string) {
		a.Emu.mu.Lock()
		a.Emu.state = RunState{Running: title != "", Phase: "playing", Title: title, Started: 1700000000}
		a.Emu.mu.Unlock()
	}
	count := func() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.acts) }

	// off by default: nothing is sent, even while a game runs
	setRunning("The Legend of Zelda: Ocarina of Time")
	d.Update()
	if count() != 0 || f.clientID != "" {
		t.Fatal("sent something while turned off")
	}
	// on, but no application ID: still nothing
	a.Settings.Update(func(s *GlobalSettings) { s.Discord.Enabled = true })
	d.Update()
	if count() != 0 {
		t.Fatal("sent without an application ID")
	}
	a.Settings.Update(func(s *GlobalSettings) { s.Discord.AppID = "123456789012345678" })
	d.Update()
	time.Sleep(50 * time.Millisecond)
	f.mu.Lock()
	if f.clientID != "123456789012345678" || len(f.acts) != 1 || f.acts[0]["details"] != "The Legend of Zelda: Ocarina of Time" {
		t.Fatalf("presence: id %q acts %v", f.clientID, f.acts)
	}
	f.mu.Unlock()
	// the game ends, idle off: the presence is cleared at once
	setRunning("")
	d.Update()
	time.Sleep(50 * time.Millisecond)
	f.mu.Lock()
	if len(f.acts) != 2 || f.acts[1] != nil {
		t.Fatalf("not cleared: %v", f.acts)
	}
	f.mu.Unlock()
	// turned off while a game runs: cleared and nothing more is sent
	setRunning("FINAL FANTASY RESONANCE")
	d.last = time.Time{}
	d.Update()
	a.Settings.Update(func(s *GlobalSettings) { s.Discord.Enabled = false })
	d.Update()
	time.Sleep(50 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.acts) != 4 || f.acts[2]["details"] != "FINAL FANTASY RESONANCE" || f.acts[3] != nil {
		t.Fatalf("turning it off: %v", f.acts)
	}
	if d.conn != nil {
		t.Fatal("connection kept while turned off")
	}
}
