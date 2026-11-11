package main

// NEXUS Device Linking — EXPERIMENTAL, disabled unless the "devicelink" feature is switched on
// (features.go). Nothing of it is visible or listening in a normal NEXUS.
//
// A NEXUS user ("account") can be linked to other devices running NEXUS. The device that already
// has the user approves every new device; afterwards the devices synchronise that user's saves,
// profile, controller layout and play time directly over the local network (linksync.go).
//
// Security model
//   - Every device has its own Ed25519 identity key (never leaves the device).
//   - Pairing: the trusted device shows a one-time code (8 characters, ~40 bits), valid for
//     5 minutes, usable once, locked after 5 wrong attempts. The new device enters the User ID and
//     the code. SPAKE2 (linkcrypto.go) proves both sides know the code without revealing it; a
//     wrong code never reaches the user, and nobody can test codes offline. Only then does the
//     trusted device ask its user to Approve or Decline, naming the new device and its address.
//   - The User ID alone grants nothing: it only says which user a code belongs to.
//   - After approval the devices exchange their identity keys inside the SPAKE2-protected channel.
//     Every later connection is mutual TLS 1.3 with both keys pinned; removing a device revokes it
//     immediately (its key is no longer accepted).
//   - Only the local network is used; nothing is exposed to the internet, no account password
//     exists, and no secret is shared between devices. NEXUS Cloud is designed separately
//     (docs/NEXUS_CLOUD.md) and not implemented.
//   - Never synchronised: games, updates/DLC, keys, firmware.

import (
	"bufio"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	linkProto        = 1
	linkDefaultPort  = 47913 // TCP (TLS), and UDP for discovery on the local network
	pairCodeLife     = 5 * time.Minute
	pairMaxAttempts  = 5
	pairApprovalWait = 2 * time.Minute
	codeAlphabet     = "23456789ABCDEFGHJKMNPQRSTUVWXYZ" // no 0/O, 1/I/L
)

type linkIdentity struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Seed       []byte `json:"seed"` // Ed25519 private key seed
	Created    int64  `json:"created"`
}

type TrustedDevice struct {
	DeviceID  string   `json:"deviceId"`
	Name      string   `json:"name"`
	PublicKey []byte   `json:"publicKey"`
	Accounts  []string `json:"accounts"` // NEXUS user (profile) ids shared with this device
	Role      string   `json:"role"`     // "approved-by-us" | "approved-us"
	Added     int64    `json:"added"`
	Addr      string   `json:"addr,omitempty"` // last known address (host:port)
	LastSync  int64    `json:"lastSync,omitempty"`
	LastError string   `json:"lastError,omitempty"`
}

type pairSession struct {
	Account  string
	Code     string
	Expires  time.Time
	Attempts int
	Used     bool
	busy     bool
}

// PairRequest is shown on the trusted device for approval.
type PairRequest struct {
	ID       string `json:"id"`
	Account  string `json:"account"`
	Device   string `json:"device"`
	Addr     string `json:"addr"`
	decision chan bool
}

type DeviceLink struct {
	app      *App
	mu       sync.Mutex
	id       linkIdentity
	priv     ed25519.PrivateKey
	devices  []TrustedDevice
	sessions map[string]*pairSession // by account
	requests map[string]*PairRequest
	ln       net.Listener
	udp      *net.UDPConn
	Port     int
	syncMu   sync.Mutex // one synchronisation at a time
}

func (a *App) linkDir() string { return filepath.Join(a.Paths.System, "DeviceLink") }

// UserID is how a NEXUS user is named when linking: NX-XXXX-XXXX-XXXX (from the profile id).
func UserID(profileID string) string {
	s := strings.ToUpper(profileID)
	for len(s) < 12 {
		s = "0" + s
	}
	return "NX-" + s[0:4] + "-" + s[4:8] + "-" + s[8:12]
}

func profileIDFromUserID(uid string) string {
	s := strings.ToLower(strings.NewReplacer("NX-", "", "nx-", "", "-", "", " ", "").Replace(strings.TrimSpace(uid)))
	return s
}

func normalizeCode(c string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(c)))
}

func newPairCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i := range b {
		out[i] = codeAlphabet[int(b[i])%len(codeAlphabet)] // 31 symbols: bias < 1%, irrelevant with 5 attempts
	}
	return string(out[:4]) + "-" + string(out[4:]), nil
}

// NewDeviceLink loads (or creates) this device's identity. It does not listen yet.
func NewDeviceLink(a *App) (*DeviceLink, error) {
	l := &DeviceLink{app: a, sessions: map[string]*pairSession{}, requests: map[string]*PairRequest{}}
	if err := os.MkdirAll(a.linkDir(), 0o700); err != nil {
		return nil, err
	}
	idPath := filepath.Join(a.linkDir(), "identity.json")
	if err := readJSON(idPath, &l.id); err != nil || len(l.id.Seed) != ed25519.SeedSize {
		seed := make([]byte, ed25519.SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return nil, err
		}
		host, _ := os.Hostname()
		l.id = linkIdentity{DeviceID: newID() + newID(), DeviceName: firstNonEmpty(host, "NEXUS device"), Seed: seed, Created: nowUnix()}
		data, _ := json.MarshalIndent(l.id, "", "  ")
		if err := os.WriteFile(idPath, data, 0o600); err != nil {
			return nil, err
		}
	}
	l.priv = ed25519.NewKeyFromSeed(l.id.Seed)
	_ = readJSON(filepath.Join(a.linkDir(), "devices.json"), &l.devices)
	return l, nil
}

func (l *DeviceLink) saveDevicesLocked() error {
	return writeJSONAtomic(filepath.Join(l.app.linkDir(), "devices.json"), l.devices)
}

func (l *DeviceLink) Devices() []TrustedDevice {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]TrustedDevice{}, l.devices...)
}

func (l *DeviceLink) deviceByKey(pub ed25519.PublicKey) *TrustedDevice {
	for i := range l.devices {
		if hmac.Equal(l.devices[i].PublicKey, pub) {
			return &l.devices[i]
		}
	}
	return nil
}

// RemoveDevice revokes a linked device: its key is no longer accepted, here or by any
// synchronisation started from here.
func (l *DeviceLink) RemoveDevice(deviceID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, d := range l.devices {
		if d.DeviceID == deviceID {
			l.devices = append(l.devices[:i], l.devices[i+1:]...)
			l.app.Log.Printf("device link: %s (%s) removed", d.Name, d.DeviceID)
			return l.saveDevicesLocked()
		}
	}
	return errors.New("That device is not linked.")
}

// StartPairing creates a one-time code for linking the user to a new device.
func (l *DeviceLink) StartPairing(profileID string) (string, time.Time, error) {
	if _, ok := l.app.Profiles.Get(profileID); !ok {
		return "", time.Time{}, errors.New("User not found.")
	}
	code, err := newPairCode()
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().Add(pairCodeLife)
	l.mu.Lock()
	l.sessions[profileID] = &pairSession{Account: profileID, Code: code, Expires: exp}
	l.mu.Unlock()
	return code, exp, nil
}

func (l *DeviceLink) CancelPairing(profileID string) {
	l.mu.Lock()
	delete(l.sessions, profileID)
	l.mu.Unlock()
}

// PendingRequests: new devices waiting for approval on this device.
func (l *DeviceLink) PendingRequests() []PairRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []PairRequest{}
	for _, r := range l.requests {
		out = append(out, PairRequest{ID: r.ID, Account: r.Account, Device: r.Device, Addr: r.Addr})
	}
	return out
}

func (l *DeviceLink) Decide(requestID string, approve bool) error {
	l.mu.Lock()
	r := l.requests[requestID]
	l.mu.Unlock()
	if r == nil {
		return errors.New("That request is no longer waiting.")
	}
	select {
	case r.decision <- approve:
	default:
	}
	return nil
}

// ---------------------------------------------------------------- networking

func (l *DeviceLink) tlsServerConfig() (*tls.Config, error) {
	cert, err := deviceCert(l.priv, l.id.DeviceName)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, ClientAuth: tls.RequestClientCert}, nil
}

// Listen starts the link service on the local network (port 0: any free port, for tests).
func (l *DeviceLink) Listen(port int) error {
	cfg, err := l.tlsServerConfig()
	if err != nil {
		return err
	}
	ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), cfg)
	if err != nil {
		return err
	}
	l.ln = ln
	l.Port = ln.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go l.serve(c)
		}
	}()
	if u, err := net.ListenUDP("udp4", &net.UDPAddr{Port: l.Port}); err == nil {
		l.udp = u
		go l.answerDiscovery()
	}
	l.app.Log.Printf("device link: listening on port %d (experimental)", l.Port)
	return nil
}

func (l *DeviceLink) Close() {
	if l.ln != nil {
		l.ln.Close()
	}
	if l.udp != nil {
		l.udp.Close()
	}
}

// discovery: "NEXUS-LINK? <profile id>" -> "NEXUS-LINK! <profile id> <port>" from the device that
// has an active pairing code (or already shares the user). Only the local network segment.
func (l *DeviceLink) answerDiscovery() {
	buf := make([]byte, 256)
	for {
		n, from, err := l.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		f := strings.Fields(string(buf[:n]))
		if len(f) != 2 || f[0] != "NEXUS-LINK?" {
			continue
		}
		l.mu.Lock()
		_, pairing := l.sessions[f[1]]
		l.mu.Unlock()
		if pairing {
			_, _ = l.udp.WriteToUDP([]byte(fmt.Sprintf("NEXUS-LINK! %s %d", f[1], l.Port)), from)
		}
	}
}

// Discover looks for the device that is showing a pairing code for this user.
func Discover(profileID string, port int, wait time.Duration) (string, error) {
	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return "", err
	}
	defer c.Close()
	msg := []byte("NEXUS-LINK? " + profileID)
	for _, to := range []string{"255.255.255.255", "127.0.0.1"} {
		_, _ = c.WriteToUDP(msg, &net.UDPAddr{IP: net.ParseIP(to), Port: port})
	}
	_ = c.SetReadDeadline(time.Now().Add(wait))
	buf := make([]byte, 256)
	for {
		n, from, err := c.ReadFromUDP(buf)
		if err != nil {
			return "", errors.New("No NEXUS device showing a link code for this User ID was found on your network.")
		}
		f := strings.Fields(string(buf[:n]))
		if len(f) == 3 && f[0] == "NEXUS-LINK!" && f[1] == profileID {
			return net.JoinHostPort(from.IP.String(), f[2]), nil
		}
	}
}

type wire struct {
	r *bufio.Reader
	c net.Conn
}

func (w wire) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_ = w.c.SetWriteDeadline(time.Now().Add(60 * time.Second))
	_, err = w.c.Write(append(data, '\n'))
	return err
}

func (w wire) recv(v any, timeout time.Duration) error {
	_ = w.c.SetReadDeadline(time.Now().Add(timeout))
	line, err := w.r.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}

type linkMsg struct {
	T       string          `json:"t"`
	Err     string          `json:"e,omitempty"`
	Proto   int             `json:"proto,omitempty"`
	Account string          `json:"account,omitempty"`
	Device  string          `json:"device,omitempty"`
	Name    string          `json:"name,omitempty"`
	Share   []byte          `json:"share,omitempty"`
	MAC     []byte          `json:"mac,omitempty"`
	Box     []byte          `json:"box,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (l *DeviceLink) serve(c net.Conn) {
	defer c.Close()
	tc := c.(*tls.Conn)
	_ = tc.SetDeadline(time.Now().Add(30 * time.Second))
	if err := tc.Handshake(); err != nil {
		return
	}
	_ = tc.SetDeadline(time.Time{})
	w := wire{bufio.NewReader(c), c}
	var first linkMsg
	if err := w.recv(&first, 30*time.Second); err != nil {
		return
	}
	switch first.T {
	case "pair1":
		l.servePair(w, first, c.RemoteAddr().String())
	case "hello":
		pub, err := peerKey(tc.ConnectionState())
		l.mu.Lock()
		var dev *TrustedDevice
		if err == nil {
			dev = l.deviceByKey(pub)
		}
		var d TrustedDevice
		if dev != nil {
			d = *dev
		}
		l.mu.Unlock()
		if dev == nil {
			_ = w.send(linkMsg{T: "err", Err: "not-linked"})
			return
		}
		l.serveSync(w, d, first)
	}
}

// servePair: the trusted device's side of pairing.
func (l *DeviceLink) servePair(w wire, m linkMsg, addr string) {
	generic := func() { _ = w.send(linkMsg{T: "err", Err: "pairing-failed"}) }
	l.mu.Lock()
	s := l.sessions[m.Account]
	ok := s != nil && !s.Used && !s.busy && time.Now().Before(s.Expires) && s.Attempts < pairMaxAttempts
	if ok {
		s.busy = true
	}
	l.mu.Unlock()
	if !ok {
		generic()
		return
	}
	defer func() { l.mu.Lock(); s.busy = false; l.mu.Unlock() }()
	sp, err := newSpake(false, s.Code, m.Account, m.Device, l.id.DeviceID)
	if err != nil {
		generic()
		return
	}
	key, macA, macB, err := sp.finish(m.Share)
	if err != nil {
		generic()
		return
	}
	if err := w.send(linkMsg{T: "pair2", Device: l.id.DeviceID, Name: l.id.DeviceName, Share: sp.msg, MAC: macB}); err != nil {
		return
	}
	var m3 linkMsg
	if err := w.recv(&m3, 30*time.Second); err != nil || m3.T != "pair3" || !hmac.Equal(m3.MAC, macA) {
		l.mu.Lock()
		s.Attempts++
		if s.Attempts >= pairMaxAttempts && l.sessions[m.Account] == s {
			delete(l.sessions, m.Account)
			l.app.Log.Printf("device link: too many wrong codes for %s; the code was cancelled", UserID(m.Account))
		}
		l.mu.Unlock()
		_ = w.send(linkMsg{T: "err", Err: "wrong-code"})
		return
	}
	// the code was right: now the user of this device decides
	req := &PairRequest{ID: newID(), Account: m.Account, Device: firstNonEmpty(m.Name, "Unknown device"), Addr: addrHost(addr), decision: make(chan bool, 1)}
	l.mu.Lock()
	l.requests[req.ID] = req
	l.mu.Unlock()
	l.app.Events.Emit("link.request", map[string]any{"id": req.ID, "account": req.Account, "device": req.Device, "addr": req.Addr})
	var approved bool
	select {
	case approved = <-req.decision:
	case <-time.After(pairApprovalWait):
	}
	l.mu.Lock()
	delete(l.requests, req.ID)
	l.mu.Unlock()
	if !approved {
		_ = w.send(linkMsg{T: "err", Err: "declined"})
		return
	}
	pr, _ := l.app.Profiles.Get(m.Account)
	payload, _ := json.Marshal(map[string]any{"publicKey": l.priv.Public(), "profile": profileExport(pr)})
	box, err := sealBox(key, payload, []byte("pair4"))
	if err != nil {
		return
	}
	if err := w.send(linkMsg{T: "pair4", Box: box}); err != nil {
		return
	}
	var m5 linkMsg
	if err := w.recv(&m5, 30*time.Second); err != nil || m5.T != "pair5" {
		return
	}
	plain, err := openBox(key, m5.Box, []byte("pair5"))
	var their struct{ PublicKey []byte }
	if err != nil || json.Unmarshal(plain, &their) != nil || len(their.PublicKey) != ed25519.PublicKeySize {
		return
	}
	l.mu.Lock()
	s.Used = true
	if l.sessions[m.Account] == s {
		delete(l.sessions, m.Account)
	}
	l.addDeviceLocked(TrustedDevice{DeviceID: m.Device, Name: req.Device, PublicKey: their.PublicKey, Accounts: []string{m.Account},
		Role: "approved-by-us", Added: nowUnix()})
	l.mu.Unlock()
	_ = w.send(linkMsg{T: "ok"})
	l.app.Log.Printf("device link: %s linked to %s", req.Device, UserID(m.Account))
	l.app.Events.Emit("link.changed", nil)
}

func (l *DeviceLink) addDeviceLocked(d TrustedDevice) {
	for i := range l.devices {
		if l.devices[i].DeviceID == d.DeviceID {
			for _, a := range d.Accounts {
				if !contains(l.devices[i].Accounts, a) {
					l.devices[i].Accounts = append(l.devices[i].Accounts, a)
				}
			}
			l.devices[i].PublicKey, l.devices[i].Name = d.PublicKey, d.Name
			if d.Addr != "" {
				l.devices[i].Addr = d.Addr
			}
			_ = l.saveDevicesLocked()
			return
		}
	}
	l.devices = append(l.devices, d)
	_ = l.saveDevicesLocked()
}

func addrHost(addr string) string {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return h
}

// PairWith is the new device's side: link the user with userID from the device at addr using the
// code shown there. On success the user exists on this device and is synchronised.
func (l *DeviceLink) PairWith(addr, userID, code string) (TrustedDevice, error) {
	account := profileIDFromUserID(userID)
	if len(account) != 12 {
		return TrustedDevice{}, errors.New("That is not a NEXUS User ID (it looks like NX-XXXX-XXXX-XXXX).")
	}
	eph, err := ephemeralCert()
	if err != nil {
		return TrustedDevice{}, err
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr,
		&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{eph}}) //nolint:gosec // authenticated by SPAKE2, see above
	if err != nil {
		return TrustedDevice{}, fmt.Errorf("The other device could not be reached: %w", err)
	}
	defer conn.Close()
	w := wire{bufio.NewReader(conn), conn}
	sp, err := newSpake(true, code, account, l.id.DeviceID, "")
	if err != nil {
		return TrustedDevice{}, err
	}
	if err := w.send(linkMsg{T: "pair1", Proto: linkProto, Account: account, Device: l.id.DeviceID, Name: l.id.DeviceName, Share: sp.msg}); err != nil {
		return TrustedDevice{}, err
	}
	var m2 linkMsg
	if err := w.recv(&m2, 30*time.Second); err != nil {
		return TrustedDevice{}, err
	}
	if m2.T != "pair2" {
		return TrustedDevice{}, errors.New("The other device has no link code for this User ID, or it has expired. Create a new code there and try again.")
	}
	sp.idB = m2.Device
	key, macA, macB, err := sp.finish(m2.Share)
	if err != nil || !hmac.Equal(m2.MAC, macB) {
		_ = w.send(linkMsg{T: "pair3", MAC: make([]byte, 32)}) // counts as a wrong attempt there
		var ack linkMsg
		_ = w.recv(&ack, 10*time.Second)
		return TrustedDevice{}, errors.New("The code is not correct. Check it and try again.")
	}
	if err := w.send(linkMsg{T: "pair3", MAC: macA}); err != nil {
		return TrustedDevice{}, err
	}
	var m4 linkMsg
	if err := w.recv(&m4, pairApprovalWait+30*time.Second); err != nil {
		return TrustedDevice{}, err
	}
	if m4.T != "pair4" {
		if m4.Err == "declined" {
			return TrustedDevice{}, errors.New("The link was declined (or not approved in time) on the other device.")
		}
		return TrustedDevice{}, errors.New("The code is not correct. Check it and try again.")
	}
	plain, err := openBox(key, m4.Box, []byte("pair4"))
	var theirs struct {
		PublicKey []byte
		Profile   ProfileExport
	}
	if err != nil || json.Unmarshal(plain, &theirs) != nil || len(theirs.PublicKey) != ed25519.PublicKeySize {
		return TrustedDevice{}, errors.New("The link could not be completed.")
	}
	mine, _ := json.Marshal(map[string]any{"publicKey": l.priv.Public()})
	box, _ := sealBox(key, mine, []byte("pair5"))
	if err := w.send(linkMsg{T: "pair5", Box: box}); err != nil {
		return TrustedDevice{}, err
	}
	var ok linkMsg
	if err := w.recv(&ok, 30*time.Second); err != nil || ok.T != "ok" {
		return TrustedDevice{}, errors.New("The link could not be completed.")
	}
	if err := l.app.Profiles.ImportLinked(theirs.Profile); err != nil {
		return TrustedDevice{}, err
	}
	d := TrustedDevice{DeviceID: m2.Device, Name: m2.Name, PublicKey: theirs.PublicKey, Accounts: []string{account}, Role: "approved-us", Added: nowUnix(), Addr: addr}
	l.mu.Lock()
	l.addDeviceLocked(d)
	l.mu.Unlock()
	l.app.Log.Printf("device link: linked %s from %s", UserID(account), m2.Name)
	l.app.Events.Emit("link.changed", nil)
	return d, nil
}

// dialPeer opens a pinned mutual-TLS connection to a linked device.
func (l *DeviceLink) dialPeer(d TrustedDevice, addr string) (*tls.Conn, error) {
	cert, err := deviceCert(l.priv, l.id.DeviceName)
	if err != nil {
		return nil, err
	}
	want := ed25519.PublicKey(d.PublicKey)
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true, //nolint:gosec // pinned below
		VerifyConnection: func(cs tls.ConnectionState) error {
			k, err := peerKey(cs)
			if err != nil || !hmac.Equal(k, want) {
				return errors.New("this is not the linked device (its identity key does not match)")
			}
			return nil
		}}
	return tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, cfg)
}

var b64 = base64.StdEncoding

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
