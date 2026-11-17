package main

// Controller management: NEXUS remembers every game controller that has been connected (by its
// serial number when the controller reports one, otherwise by its model GUID and position), with
// a name, its type and an optional button layout of its own. Each NEXUS user chooses which
// controllers they play with, in player order (player 1 first); with nothing chosen, the first
// connected controller is player 1, as before.
//
// Detection goes through the NEXUS engine (SDL, the same layer that drives controllers in games),
// so what NEXUS lists is exactly what a game can use.

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type KnownPad struct {
	Key       string      `json:"key"`
	GUID      string      `json:"guid"`
	Port      int         `json:"port"`
	Serial    string      `json:"serial,omitempty"`
	Path      string      `json:"path,omitempty"`
	Name      string      `json:"name"`
	Nickname  string      `json:"nickname,omitempty"`
	Type      string      `json:"type"`
	Vendor    int         `json:"vendor"`
	Product   int         `json:"product"`
	FirstSeen int64       `json:"firstSeen"`
	LastSeen  int64       `json:"lastSeen"`
	Layout    *GameLayout `json:"layout,omitempty"` // this controller's own layout (player 1)
}

type PadView struct {
	KnownPad
	Label     string `json:"label"` // the model in words (padmodels.go)
	Model     string `json:"model"`
	Family    string `json:"family"`    // button prompts: xbox | playstation | nintendo | steam | generic
	Connected bool   `json:"connected"` // connected now
	Player    int    `json:"player"`    // 1… in the user's order, 0 = not chosen
}

type controllerDoc struct {
	Known    map[string]*KnownPad `json:"known"`
	Profiles map[string][]string  `json:"profiles"` // user ID -> controller keys in player order
}

type ControllerStore struct {
	app  *App
	path string
	mu   sync.Mutex
	doc  controllerDoc
	last []PadInfo // last detection
	at   time.Time
	err  string
}

func NewControllerStore(a *App) *ControllerStore {
	c := &ControllerStore{app: a, path: filepath.Join(a.Paths.Settings, "controllers.json")}
	_ = readJSON(c.path, &c.doc)
	if c.doc.Known == nil {
		c.doc.Known = map[string]*KnownPad{}
	}
	if c.doc.Profiles == nil {
		c.doc.Profiles = map[string][]string{}
	}
	return c
}

func (c *ControllerStore) saveLocked() { _ = writeJSONAtomic(c.path, c.doc) }

// padKey: the serial number identifies one controller; without one, model and position do.
func padKey(p PadInfo) string {
	if s := strings.TrimSpace(p.Serial); s != "" && s != "000000000000" {
		return "sn:" + strings.ToLower(s) + ":" + fmt.Sprintf("%04x%04x", p.Vendor, p.Product)
	}
	return fmt.Sprintf("guid:%s:%d", strings.ToLower(p.GUID), p.Port)
}

func padLabel(typ string, vendor, product int) string {
	return identifyPad(vendor, product, typ, "", "").Name
}

// Detect asks the engine for the connected controllers and remembers new ones. During a game the
// last detection is returned (the engine is busy with the controllers then).
func (c *ControllerStore) Detect() ([]PadInfo, error) {
	if st := c.app.Emu.State(); st.Running {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.last, nil
	}
	host, err := c.app.Emu.Host()
	var pads []PadInfo
	if err == nil {
		pads, err = host.Controllers()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.err = err.Error()
		return c.last, err
	}
	c.err, c.last, c.at = "", pads, time.Now()
	now := time.Now().Unix()
	changed := false
	for _, p := range pads {
		k := padKey(p)
		kp := c.doc.Known[k]
		if kp == nil {
			kp = &KnownPad{Key: k, FirstSeen: now}
			c.doc.Known[k] = kp
			c.app.Log.Printf("controllers: new controller %q (%s)", p.Name, p.Type)
		}
		kp.GUID, kp.Port, kp.Serial, kp.Name, kp.Type, kp.Vendor, kp.Product, kp.LastSeen = p.GUID, p.Port, p.Serial, p.Name, p.Type, p.Vendor, p.Product, now
		kp.Path = p.Path
		changed = true
	}
	if changed {
		c.saveLocked()
	}
	return pads, nil
}

// List: connected and remembered controllers, from the point of view of a user.
func (c *ControllerStore) List(profileID string) ([]PadView, string) {
	pads, _ := c.Detect()
	c.mu.Lock()
	defer c.mu.Unlock()
	connected := map[string]bool{}
	for _, p := range pads {
		connected[padKey(p)] = true
	}
	order := c.doc.Profiles[profileID]
	out := []PadView{}
	for k, kp := range c.doc.Known {
		pm := identifyPad(kp.Vendor, kp.Product, kp.Type, kp.Path, kp.Name)
		v := PadView{KnownPad: *kp, Label: pm.Name, Model: pm.Model, Family: pm.Family, Connected: connected[k]}
		for i, ok := range order {
			if ok == k {
				v.Player = i + 1
			}
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Player > 0) != (b.Player > 0) {
			return a.Player > 0
		}
		if a.Player != b.Player {
			return a.Player < b.Player
		}
		if a.Connected != b.Connected {
			return a.Connected
		}
		return a.LastSeen > b.LastSeen
	})
	return out, c.err
}

// SetPlayers sets a user's controllers in player order (only remembered controllers).
func (c *ControllerStore) SetPlayers(profileID string, keys []string) error {
	if _, ok := c.app.Profiles.Get(profileID); !ok {
		return errors.New("User not found.")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, k := range keys {
		if c.doc.Known[k] == nil || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	if len(out) > 8 {
		out = out[:8]
	}
	c.doc.Profiles[profileID] = out
	c.saveLocked()
	return nil
}

// Forget removes a remembered controller (and its layout) everywhere.
func (c *ControllerStore) Forget(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.doc.Known[key] == nil {
		return errors.New("That controller is not remembered.")
	}
	delete(c.doc.Known, key)
	for id, keys := range c.doc.Profiles {
		var keep []string
		for _, k := range keys {
			if k != key {
				keep = append(keep, k)
			}
		}
		c.doc.Profiles[id] = keep
	}
	c.saveLocked()
	return nil
}

// Update sets a controller's nickname and/or its own layout (nil layout clears it).
func (c *ControllerStore) Update(key string, nickname *string, layout *GameLayout, clearLayout bool) (KnownPad, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	kp := c.doc.Known[key]
	if kp == nil {
		return KnownPad{}, errors.New("That controller is not remembered.")
	}
	if nickname != nil {
		n := strings.TrimSpace(*nickname)
		if len([]rune(n)) > 30 {
			return *kp, errors.New("Names can be up to 30 characters.")
		}
		kp.Nickname = n
	}
	if clearLayout {
		kp.Layout = nil
	} else if layout != nil {
		l, err := normalizeLayout(*layout)
		if err != nil {
			return *kp, err
		}
		kp.Layout = &l
	}
	c.saveLocked()
	return *kp, nil
}

// ForSession: the user's chosen controllers that are connected, in player order, for the engine.
func (c *ControllerStore) ForSession(pr *Profile) []EnginePad {
	if pr == nil {
		return nil
	}
	pads, _ := c.Detect()
	c.mu.Lock()
	defer c.mu.Unlock()
	now := map[string]PadInfo{}
	for _, p := range pads {
		now[padKey(p)] = p
	}
	var out []EnginePad
	for _, k := range c.doc.Profiles[pr.ID] {
		if p, ok := now[k]; ok {
			out = append(out, EnginePad{GUID: p.GUID, Port: p.Port})
		}
	}
	return out
}

// Player1Layout: the own layout of the user's player-1 controller, if it is connected and has one.
func (c *ControllerStore) Player1Layout(pr *Profile) *GameLayout {
	if pr == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := c.doc.Profiles[pr.ID]
	connected := map[string]bool{}
	for _, p := range c.last {
		connected[padKey(p)] = true
	}
	for _, k := range keys {
		if connected[k] {
			if kp := c.doc.Known[k]; kp != nil && kp.Layout != nil {
				l := *kp.Layout
				return &l
			}
			return nil
		}
	}
	return nil
}

// RemoveProfile forgets a deleted user's choice.
func (c *ControllerStore) RemoveProfile(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.doc.Profiles[id]; ok {
		delete(c.doc.Profiles, id)
		c.saveLocked()
	}
}
