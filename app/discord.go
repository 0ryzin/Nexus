package main

// Discord Rich Presence (0.9.3). While a game runs in NEXUS, Discord shows "Playing NEXUS" with
// the game's real title and the time played; optionally "In NEXUS" while no game runs. Off by
// default; one switch turns it off entirely (the presence is cleared at once).
//
// It talks to the Discord app on this computer only (its local IPC: a named pipe on Windows, a
// socket on Linux, including Flatpak and Snap Discord on a Steam Deck), sends nothing else and
// reads nothing. It sets the presence of NEXUS's own Discord application only: Discord keeps
// detecting games started outside NEXUS by itself, and NEXUS clears its presence when the game
// ends, so nothing of it is left over another game.
//
// Discord limits (docs.discord.com, Rich Presence): the top line is the application's registered
// name (not settable); images are either one of up to 300 assets uploaded to the application or a
// public https URL, so the game's local artwork cannot be shown. NEXUS uses the asset "nexus" if
// the application has one, and text only otherwise. Updates are limited (one per 15 s here).

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// discordAppID: the NEXUS Discord application, set at build time (-ldflags -X) or in settings.
// An application ID is public (it is in every client that uses it); it is not a secret.
var discordAppID = ""

type discordPresence struct {
	Details, State string
	Start          int64
}

type Discord struct {
	app     *App
	mu      sync.Mutex
	conn    io.ReadWriteCloser
	connID  string
	shown   *discordPresence
	last    time.Time
	nonce   int
	failing time.Time // the last failed connection: tried again after a while
	stop    chan struct{}
}

func NewDiscord(a *App) *Discord { return &Discord{app: a, stop: make(chan struct{})} }

func (d *Discord) appID() string {
	if id := strings.TrimSpace(d.app.Settings.Get().Discord.AppID); id != "" {
		return id
	}
	if id := os.Getenv("NEXUS_DISCORD_APP_ID"); id != "" {
		return id
	}
	return discordAppID
}

// want is what should be shown now (nil: nothing).
func (d *Discord) want() *discordPresence {
	set := d.app.Settings.Get().Discord
	if !set.Enabled || d.appID() == "" {
		return nil
	}
	if d.app.Emu != nil {
		if st := d.app.Emu.State(); st.Running && st.Title != "" {
			return &discordPresence{Details: discordText(st.Title), State: "Playing", Start: st.Started}
		}
	}
	if set.ShowIdle {
		return &discordPresence{Details: "In NEXUS"}
	}
	return nil
}

// discordText keeps a field within Discord's 2-128 characters.
func discordText(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 128 {
		s = string(r[:127]) + "…"
	}
	for len([]rune(s)) < 2 {
		s += " "
	}
	return s
}

// Run keeps the presence in step every few seconds until Stop.
func (d *Discord) Run() {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		d.Update()
		select {
		case <-t.C:
		case <-d.stop:
			d.clear()
			return
		}
	}
}

func (d *Discord) Stop() {
	select {
	case <-d.stop:
	default:
		close(d.stop)
	}
}

// Update sends the presence when it changed (at most every 15 s, except clearing, at once).
func (d *Discord) Update() {
	w := d.want()
	d.mu.Lock()
	defer d.mu.Unlock()
	if samePresence(w, d.shown) {
		return
	}
	if w == nil {
		d.clearLocked()
		return
	}
	if time.Since(d.last) < 15*time.Second && d.shown != nil {
		return // Discord's rate limit; the next tick sends it
	}
	if err := d.sendLocked(w); err != nil {
		d.closeLocked()
		return
	}
	d.shown, d.last = w, time.Now()
}

func samePresence(a, b *discordPresence) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (d *Discord) clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clearLocked()
}

// clearLocked removes NEXUS's presence and closes the connection: with no connection Discord
// shows nothing of NEXUS's application.
func (d *Discord) clearLocked() {
	if d.conn != nil && d.shown != nil {
		_ = d.frame(1, map[string]any{"cmd": "SET_ACTIVITY", "args": map[string]any{"pid": os.Getpid()}, "nonce": d.nextNonce()})
	}
	d.shown = nil
	d.closeLocked()
}

func (d *Discord) closeLocked() {
	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
	}
}

func (d *Discord) nextNonce() string { d.nonce++; return fmt.Sprintf("nexus-%d", d.nonce) }

func (d *Discord) sendLocked(p *discordPresence) error {
	id := d.appID()
	if d.conn != nil && d.connID != id {
		d.closeLocked()
	}
	if d.conn == nil {
		if time.Since(d.failing) < 30*time.Second {
			return errors.New("discord not reachable")
		}
		c, err := dialDiscord()
		if err != nil {
			d.failing = time.Now()
			return err
		}
		d.conn, d.connID = c, id
		if err := d.frame(0, map[string]any{"v": 1, "client_id": id}); err != nil {
			d.failing = time.Now()
			return err
		}
		if _, _, err := d.read(); err != nil { // READY (or an error for a wrong ID)
			d.failing = time.Now()
			return err
		}
		d.app.Log.Printf("discord: connected")
	}
	act := map[string]any{"details": p.Details, "assets": map[string]any{"large_image": "nexus", "large_text": "NEXUS"}}
	if p.State != "" {
		act["state"] = p.State
	}
	if p.Start > 0 {
		act["timestamps"] = map[string]any{"start": p.Start}
	}
	if err := d.frame(1, map[string]any{"cmd": "SET_ACTIVITY", "args": map[string]any{"pid": os.Getpid(), "activity": act}, "nonce": d.nextNonce()}); err != nil {
		return err
	}
	_, body, err := d.read()
	if err != nil {
		return err
	}
	var r struct {
		Evt  string `json:"evt"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) == nil && r.Evt == "ERROR" {
		d.app.Log.Printf("discord: presence not accepted: %s", r.Data.Message)
		return errors.New(r.Data.Message)
	}
	return nil
}

// frame: Discord IPC framing, opcode and length (little-endian uint32) then JSON.
func (d *Discord) frame(op uint32, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, op)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(b)))
	buf.Write(b)
	_, err = d.conn.Write(buf.Bytes())
	return err
}

func (d *Discord) read() (uint32, []byte, error) {
	if dl, ok := d.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = dl.SetReadDeadline(time.Now().Add(5 * time.Second))
	}
	var h [8]byte
	if _, err := io.ReadFull(d.conn, h[:]); err != nil {
		return 0, nil, err
	}
	n := binary.LittleEndian.Uint32(h[4:])
	if n > 1<<20 {
		return 0, nil, errors.New("discord: oversized message")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(d.conn, body); err != nil {
		return 0, nil, err
	}
	op := binary.LittleEndian.Uint32(h[:4])
	if op == 2 { // CLOSE: a wrong application ID, for example
		return op, body, fmt.Errorf("discord closed the connection: %s", body)
	}
	return op, body, nil
}
