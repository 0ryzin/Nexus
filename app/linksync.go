package main

// Synchronisation between linked NEXUS devices (EXPERIMENTAL, see devicelink.go).
//
// What is synchronised for a linked user:
//   - saves: every file in the user's save folder (Saves\<id> - <name>\...), by path;
//   - the profile: name, picture, colour, theme and controller layout (profile/meta.json, and
//     profile/avatar.* for a picture);
//   - play time: each device's own statistics travel as a snapshot and are added up for display
//     (no device ever overwrites another's play time).
// Never: games, updates/DLC, keys, firmware, engine or graphics settings (those depend on the PC).
//
// Each file is compared three ways — this device, the other device, and the version both had at
// the last successful synchronisation (the "base", kept per linked device):
//   - unchanged on one side → the changed side's version is copied over;
//   - changed on both sides → a conflict: nothing is overwritten, the other device's version is
//     kept next to ours in SyncConflicts\..., and the user chooses later;
//   - deleted on one side → not propagated (saves are never deleted by a synchronisation).
// Copying is safe: the data is checked against its SHA-256, the version it replaces is backed up
// to Archived\sync-backups\..., and the new file is written under a temporary name and then
// renamed, so an interrupted transfer never leaves a partial save in place.
// Saves are not touched while a game is running (the engine then owns them).

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxSyncFile = 512 << 20 // 512 MB: larger files are not saves

type SyncFile struct {
	Path  string `json:"path"` // slash-separated: "saves/...", "profile/meta.json", "profile/avatar.png"
	Hash  string `json:"hash"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

// ProfileExport is the part of a profile that follows the user to other devices.
type ProfileExport struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	CreatedAt int64    `json:"createdAt"`
	Avatar    Avatar   `json:"avatar"`
	Theme     Theme    `json:"theme"`
	Controls  Controls `json:"controls"`
}

func profileExport(p Profile) ProfileExport {
	av := p.Avatar
	av.Rev = 0 // revision numbers are local
	th := p.Theme
	th.Wallpaper = Wallpaper{} // wallpapers are files of this PC
	return ProfileExport{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt, Avatar: av, Theme: th, Controls: p.Controls}
}

// ImportLinked creates (or updates) a user that was linked from another device, keeping its id.
func (st *ProfileStore) ImportLinked(x ProfileExport) error {
	if len(x.ID) != 12 {
		return errors.New("invalid user")
	}
	if _, err := validName(x.Name); err != nil {
		return err
	}
	st.mu.Lock()
	for _, p := range st.Profiles {
		if p.ID == x.ID {
			st.mu.Unlock()
			return st.applyExport(x)
		}
		if strings.EqualFold(p.Name, x.Name) {
			st.mu.Unlock()
			return fmt.Errorf("A different user called %q already exists on this device. Rename one of them, then link again.", x.Name)
		}
	}
	if len(st.Profiles) >= MaxProfiles {
		st.mu.Unlock()
		return fmt.Errorf("NEXUS supports up to %d users. Delete a user on this device to make room.", MaxProfiles)
	}
	pr := &Profile{ID: x.ID, Name: x.Name, CreatedAt: x.CreatedAt, Avatar: x.Avatar, Theme: x.Theme, Controls: x.Controls, Sort: "recent"}
	if pr.Avatar.Kind == "image" {
		pr.Avatar = Avatar{Kind: "preset", Preset: "star", Color: "#0a84ff"} // the picture arrives with the first sync
	}
	st.normalise(pr)
	_ = os.MkdirAll(st.profileDir(pr.ID), 0o755)
	_ = os.MkdirAll(st.saveDirNamed(pr.ID, pr.Name), 0o755)
	st.Profiles = append(st.Profiles, pr)
	err := st.saveLocked()
	st.mu.Unlock()
	return err
}

func (st *ProfileStore) applyExport(x ProfileExport) error {
	cur, ok := st.Get(x.ID)
	if !ok {
		return errors.New("user not found")
	}
	av := x.Avatar
	if av.Kind == "image" {
		av.File, av.Rev = cur.Avatar.File, cur.Avatar.Rev
		if av.File == "" {
			av = cur.Avatar
		}
	}
	th := x.Theme
	th.Wallpaper = cur.Theme.Wallpaper
	patch, _ := json.Marshal(map[string]any{"name": x.Name, "avatar": av, "theme": th, "controls": x.Controls})
	_, err := st.Update(x.ID, patch)
	return err
}

// ---------------------------------------------------------------- manifests

func fileHash(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// syncRoots maps manifest prefixes to folders of this device for a user.
func (a *App) syncLocalPath(account, rel string) (string, error) {
	rel = filepath.ToSlash(rel)
	if strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") || strings.Contains(rel, ":") || strings.Contains(rel, "\\") {
		return "", errors.New("invalid path")
	}
	switch {
	case strings.HasPrefix(rel, "saves/"):
		return filepath.Join(a.Profiles.SaveDir(account), filepath.FromSlash(strings.TrimPrefix(rel, "saves/"))), nil
	case rel == "profile/meta.json":
		return "", nil // virtual
	case strings.HasPrefix(rel, "profile/avatar."):
		return filepath.Join(a.Profiles.profileDir(account), strings.TrimPrefix(rel, "profile/")), nil
	}
	return "", errors.New("not a synchronised path")
}

func profileMeta(p Profile) []byte {
	data, _ := json.Marshal(profileExport(p))
	return data
}

// SyncManifest lists what this device has for the user.
func (a *App) SyncManifest(account string) ([]SyncFile, error) {
	p, ok := a.Profiles.Get(account)
	if !ok {
		return nil, errors.New("user not found")
	}
	var out []SyncFile
	meta := profileMeta(p)
	h := sha256.Sum256(meta)
	out = append(out, SyncFile{Path: "profile/meta.json", Hash: hex.EncodeToString(h[:]), Size: int64(len(meta))})
	if ap := a.Profiles.AvatarPath(account); ap != "" {
		if hs, n, err := fileHash(ap); err == nil {
			out = append(out, SyncFile{Path: "profile/" + filepath.Base(ap), Hash: hs, Size: n})
		}
	}
	root := a.Profiles.SaveDir(account)
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxSyncFile || strings.HasSuffix(p, ".nexus-tmp") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		hs, n, err := fileHash(p)
		if err != nil {
			return nil
		}
		out = append(out, SyncFile{Path: "saves/" + filepath.ToSlash(rel), Hash: hs, Size: n, MTime: info.ModTime().Unix()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

func (a *App) readSyncFile(account, rel string) ([]byte, error) {
	if rel == "profile/meta.json" {
		p, ok := a.Profiles.Get(account)
		if !ok {
			return nil, errors.New("user not found")
		}
		return profileMeta(p), nil
	}
	lp, err := a.syncLocalPath(account, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(lp)
}

// writeSyncFile replaces (or creates) one synchronised file: verified, backed up, atomic.
func (a *App) writeSyncFile(account, rel string, data []byte, wantHash, backupDir string) error {
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != wantHash {
		return errors.New("the data arrived damaged (checksum mismatch); nothing was changed")
	}
	if rel == "profile/meta.json" {
		var x ProfileExport
		if err := json.Unmarshal(data, &x); err != nil || x.ID != account {
			return errors.New("invalid profile data")
		}
		return a.Profiles.applyExport(x)
	}
	lp, err := a.syncLocalPath(account, rel)
	if err != nil {
		return err
	}
	if fileExists(lp) {
		rp, _ := filepath.Rel(a.Paths.Data, lp)
		if err := backupTree(lp, filepath.Join(backupDir, rp)); err != nil {
			return fmt.Errorf("the backup failed, so nothing was replaced: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		return err
	}
	tmp := lp + ".nexus-tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, lp); err != nil {
		os.Remove(tmp)
		return err
	}
	if strings.HasPrefix(rel, "profile/avatar.") {
		patch, _ := json.Marshal(map[string]any{"avatar": map[string]any{"kind": "image", "file": filepath.Base(lp), "rev": time.Now().UnixMilli()}})
		_, _ = a.Profiles.Update(account, patch)
	}
	return nil
}

// ---------------------------------------------------------------- planning

type SyncAction struct {
	Path   string `json:"path"`
	Action string `json:"action"` // pull | push | conflict
	Local  string `json:"local,omitempty"`
	Remote string `json:"remote,omitempty"`
}

// planSync decides per file what to do, from this device's files, the other device's files and
// the hashes both had at the last synchronisation.
func planSync(local, remote []SyncFile, base map[string]string) []SyncAction {
	l, r := map[string]SyncFile{}, map[string]SyncFile{}
	for _, f := range local {
		l[f.Path] = f
	}
	for _, f := range remote {
		r[f.Path] = f
	}
	paths := map[string]bool{}
	for p := range l {
		paths[p] = true
	}
	for p := range r {
		paths[p] = true
	}
	var out []SyncAction
	for p := range paths {
		lf, inL := l[p]
		rf, inR := r[p]
		b := base[p]
		switch {
		case inL && inR && lf.Hash == rf.Hash:
			continue
		case inR && !inL:
			if b == "" || b != rf.Hash { // new there (or changed there after we deleted): bring it
				out = append(out, SyncAction{Path: p, Action: "pull", Remote: rf.Hash})
			}
		case inL && !inR:
			if b == "" || b != lf.Hash {
				out = append(out, SyncAction{Path: p, Action: "push", Local: lf.Hash})
			}
		case lf.Hash == b:
			out = append(out, SyncAction{Path: p, Action: "pull", Local: lf.Hash, Remote: rf.Hash})
		case rf.Hash == b:
			out = append(out, SyncAction{Path: p, Action: "push", Local: lf.Hash, Remote: rf.Hash})
		default:
			out = append(out, SyncAction{Path: p, Action: "conflict", Local: lf.Hash, Remote: rf.Hash})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ---------------------------------------------------------------- state per linked device

type syncState struct {
	Base map[string]map[string]string `json:"base"` // account -> path -> hash
}

func (l *DeviceLink) statePath(deviceID string) string {
	return filepath.Join(l.app.linkDir(), "sync-"+safeFolderName(deviceID)+".json")
}

func (l *DeviceLink) loadState(deviceID string) syncState {
	var s syncState
	_ = readJSON(l.statePath(deviceID), &s)
	if s.Base == nil {
		s.Base = map[string]map[string]string{}
	}
	return s
}

func (l *DeviceLink) saveState(deviceID string, s syncState) {
	_ = writeJSONAtomic(l.statePath(deviceID), s)
}

type SyncConflict struct {
	ID       string `json:"id"`
	Account  string `json:"account"`
	Path     string `json:"path"`
	Device   string `json:"device"`
	DeviceID string `json:"deviceId"`
	Remote   string `json:"remote"` // hash of the other device's version
	Theirs   string `json:"theirs"` // where the other device's version is kept
	Detected int64  `json:"detected"`
}

func (l *DeviceLink) conflictsPath() string { return filepath.Join(l.app.linkDir(), "conflicts.json") }

func (l *DeviceLink) Conflicts() []SyncConflict {
	var c []SyncConflict
	_ = readJSON(l.conflictsPath(), &c)
	if c == nil {
		c = []SyncConflict{}
	}
	return c
}

func (l *DeviceLink) addConflict(c SyncConflict) {
	list := l.Conflicts()
	for i := range list {
		if list[i].Account == c.Account && list[i].Path == c.Path && list[i].Device == c.Device {
			list[i] = c
			_ = writeJSONAtomic(l.conflictsPath(), list)
			return
		}
	}
	_ = writeJSONAtomic(l.conflictsPath(), append(list, c))
}

// ResolveConflict: keep "mine" (the other version stays archived) or "theirs" (ours is backed up
// and replaced by the other device's version).
func (l *DeviceLink) ResolveConflict(id, keep string) error {
	if l.app.Emu.State().Running {
		return errors.New("Close the game first.")
	}
	list := l.Conflicts()
	for i, c := range list {
		if c.ID != id {
			continue
		}
		if keep == "theirs" {
			data, err := os.ReadFile(c.Theirs)
			if err != nil {
				return err
			}
			h := sha256.Sum256(data)
			if err := l.app.writeSyncFile(c.Account, c.Path, data, hex.EncodeToString(h[:]), l.backupDir()); err != nil {
				return err
			}
		} else if keep == "mine" {
			// ours stays; the next synchronisation from here sends it to the other device
			st := l.loadState(c.DeviceID)
			if st.Base[c.Account] == nil {
				st.Base[c.Account] = map[string]string{}
			}
			st.Base[c.Account][c.Path] = c.Remote
			l.saveState(c.DeviceID, st)
		} else {
			return errors.New("choose mine or theirs")
		}
		_ = writeJSONAtomic(l.conflictsPath(), append(list[:i], list[i+1:]...))
		return nil
	}
	return errors.New("That conflict is already resolved.")
}

func (l *DeviceLink) backupDir() string {
	return filepath.Join(l.app.Paths.Trash, "sync-backups", time.Now().Format("20060102-150405"))
}

// ---------------------------------------------------------------- the exchange

type SyncResult struct {
	Device    string `json:"device"`
	Pulled    int    `json:"pulled"`
	Pushed    int    `json:"pushed"`
	Conflicts int    `json:"conflicts"`
	Error     string `json:"error,omitempty"`
}

// SyncWith synchronises every shared user with a linked device (this device asks, the other
// answers). Saves are left alone while a game runs on either device.
func (l *DeviceLink) SyncWith(deviceID, addr string) (SyncResult, error) {
	l.syncMu.Lock()
	defer l.syncMu.Unlock()
	res := SyncResult{Device: deviceID}
	if l.app.Emu.State().Running {
		return res, errors.New("A game is running; NEXUS synchronises after it is closed.")
	}
	l.mu.Lock()
	var dev *TrustedDevice
	for i := range l.devices {
		if l.devices[i].DeviceID == deviceID {
			d := l.devices[i]
			dev = &d
		}
	}
	l.mu.Unlock()
	if dev == nil {
		return res, errors.New("That device is not linked.")
	}
	addr = firstNonEmpty(addr, dev.Addr)
	fail := func(err error) (SyncResult, error) {
		l.noteSync(deviceID, addr, err)
		res.Error = err.Error()
		return res, err
	}
	conn, err := l.dialPeer(*dev, addr)
	if err != nil {
		return fail(err)
	}
	defer conn.Close()
	w := wire{bufio.NewReader(conn), conn}
	if err := w.send(linkMsg{T: "hello", Proto: linkProto, Device: l.id.DeviceID}); err != nil {
		return fail(err)
	}
	var hello linkMsg
	if err := w.recv(&hello, 30*time.Second); err != nil {
		return fail(err)
	}
	if hello.T != "hello" {
		return fail(fmt.Errorf("the other device refused: %s", hello.Err))
	}
	if hello.Proto != linkProto {
		return fail(errors.New("the other device runs a NEXUS version with a different synchronisation format; update both"))
	}
	st := l.loadState(deviceID)
	backup := l.backupDir()
	for _, account := range dev.Accounts {
		if _, ok := l.app.Profiles.Get(account); !ok {
			continue
		}
		var m linkMsg
		if err := w.send(linkMsg{T: "manifest", Account: account}); err != nil {
			return fail(err)
		}
		if err := w.recv(&m, 60*time.Second); err != nil || m.T != "manifest" {
			return fail(errors.New("the other device did not send its file list"))
		}
		var remote []SyncFile
		_ = json.Unmarshal(m.Data, &remote)
		local, err := l.app.SyncManifest(account)
		if err != nil {
			return fail(err)
		}
		base := st.Base[account]
		if base == nil {
			base = map[string]string{}
		}
		// files both devices already have in the same version become the common base
		rh := map[string]string{}
		for _, f := range remote {
			rh[f.Path] = f.Hash
		}
		for _, f := range local {
			if rh[f.Path] == f.Hash {
				base[f.Path] = f.Hash
			}
		}
		for _, act := range planSync(local, remote, base) {
			switch act.Action {
			case "pull":
				data, err := l.fetch(w, account, act.Path, act.Remote)
				if err != nil {
					return fail(err)
				}
				if err := l.app.writeSyncFile(account, act.Path, data, act.Remote, backup); err != nil {
					return fail(err)
				}
				base[act.Path] = act.Remote
				res.Pulled++
			case "push":
				data, err := l.app.readSyncFile(account, act.Path)
				if err != nil {
					return fail(err)
				}
				var r linkMsg
				payload, _ := json.Marshal(map[string]string{"path": act.Path, "hash": act.Local, "base": base[act.Path], "data": b64.EncodeToString(data)})
				if err := w.send(linkMsg{T: "put", Account: account, Data: payload}); err != nil {
					return fail(err)
				}
				if err := w.recv(&r, 120*time.Second); err != nil {
					return fail(err)
				}
				if r.T == "conflict" {
					res.Conflicts++ // the other device kept both; its user chooses there
					continue
				}
				if r.T != "ok" {
					return fail(fmt.Errorf("the other device could not store %s: %s", act.Path, r.Err))
				}
				base[act.Path] = act.Local
				res.Pushed++
			case "conflict":
				data, err := l.fetch(w, account, act.Path, act.Remote)
				if err != nil {
					return fail(err)
				}
				keep := filepath.Join(l.app.Paths.Data, "SyncConflicts", account, time.Now().Format("20060102-150405")+" from "+safeFolderName(dev.Name), filepath.FromSlash(act.Path))
				if err := os.MkdirAll(filepath.Dir(keep), 0o755); err == nil {
					_ = os.WriteFile(keep, data, 0o644)
				}
				l.addConflict(SyncConflict{ID: newID(), Account: account, Path: act.Path, Device: dev.Name, DeviceID: dev.DeviceID, Remote: act.Remote, Theirs: keep, Detected: nowUnix()})
				res.Conflicts++
			}
		}
		// play time: send ours, receive theirs (each device's own numbers, added up for display)
		if err := l.exchangeStats(w, account); err != nil {
			return fail(err)
		}
		st.Base[account] = base
	}
	_ = w.send(linkMsg{T: "bye"})
	l.saveState(deviceID, st)
	l.noteSync(deviceID, addr, nil)
	l.app.Log.Printf("device link: synchronised with %s: %d in, %d out, %d conflicts", dev.Name, res.Pulled, res.Pushed, res.Conflicts)
	l.app.Events.Emit("link.changed", nil)
	return res, nil
}

func (l *DeviceLink) fetch(w wire, account, path, want string) ([]byte, error) {
	if err := w.send(linkMsg{T: "get", Account: account, Data: mustJSON(map[string]string{"path": path})}); err != nil {
		return nil, err
	}
	var r linkMsg
	if err := w.recv(&r, 120*time.Second); err != nil {
		return nil, err
	}
	if r.T != "file" {
		return nil, fmt.Errorf("the other device could not send %s", path)
	}
	var s string
	_ = json.Unmarshal(r.Data, &s)
	data, err := b64.DecodeString(s)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != want {
		return nil, errors.New("a file arrived damaged (checksum mismatch); nothing was changed")
	}
	return data, nil
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func (l *DeviceLink) noteSync(deviceID, addr string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.devices {
		if l.devices[i].DeviceID == deviceID {
			if err != nil {
				l.devices[i].LastError = err.Error()
			} else {
				l.devices[i].LastError, l.devices[i].LastSync = "", nowUnix()
				if addr != "" {
					l.devices[i].Addr = addr
				}
			}
		}
	}
	_ = l.saveDevicesLocked()
}

// serveSync answers a linked device's synchronisation.
func (l *DeviceLink) serveSync(w wire, dev TrustedDevice, hello linkMsg) {
	if err := w.send(linkMsg{T: "hello", Proto: linkProto, Device: l.id.DeviceID}); err != nil {
		return
	}
	st := l.loadState(dev.DeviceID)
	backup := l.backupDir()
	defer l.saveState(dev.DeviceID, st)
	for {
		var m linkMsg
		if err := w.recv(&m, 5*time.Minute); err != nil || m.T == "bye" {
			if m.T == "bye" {
				l.noteSync(dev.DeviceID, "", nil)
				l.app.Events.Emit("link.changed", nil)
			}
			return
		}
		if !contains(dev.Accounts, m.Account) {
			_ = w.send(linkMsg{T: "err", Err: "user not shared with this device"})
			return
		}
		if st.Base[m.Account] == nil {
			st.Base[m.Account] = map[string]string{}
		}
		base := st.Base[m.Account]
		switch m.T {
		case "manifest":
			files, err := l.app.SyncManifest(m.Account)
			if err != nil {
				_ = w.send(linkMsg{T: "err", Err: err.Error()})
				return
			}
			_ = w.send(linkMsg{T: "manifest", Data: mustJSON(files)})
		case "get":
			var a struct{ Path string }
			_ = json.Unmarshal(m.Data, &a)
			data, err := l.app.readSyncFile(m.Account, a.Path)
			if err != nil {
				_ = w.send(linkMsg{T: "err", Err: "not available"})
				continue
			}
			// the asking device takes our version: that is now the common base
			h := sha256.Sum256(data)
			base[a.Path] = hex.EncodeToString(h[:])
			_ = w.send(linkMsg{T: "file", Data: mustJSON(b64.EncodeToString(data))})
		case "put":
			var a struct{ Path, Hash, Base, Data string }
			_ = json.Unmarshal(m.Data, &a)
			data, err := b64.DecodeString(a.Data)
			if err != nil {
				_ = w.send(linkMsg{T: "err", Err: "bad data"})
				continue
			}
			if l.app.Emu.State().Running {
				_ = w.send(linkMsg{T: "err", Err: "a game is running on this device"})
				continue
			}
			// our current version must be the one the other device based its change on
			cur, _ := l.app.readSyncFile(m.Account, a.Path)
			curHash := ""
			if cur != nil {
				h := sha256.Sum256(cur)
				curHash = hex.EncodeToString(h[:])
			}
			if curHash != "" && curHash != a.Base && curHash != a.Hash {
				keep := filepath.Join(l.app.Paths.Data, "SyncConflicts", m.Account, time.Now().Format("20060102-150405")+" from "+safeFolderName(dev.Name), filepath.FromSlash(a.Path))
				if err := os.MkdirAll(filepath.Dir(keep), 0o755); err == nil {
					_ = os.WriteFile(keep, data, 0o644)
				}
				l.addConflict(SyncConflict{ID: newID(), Account: m.Account, Path: a.Path, Device: dev.Name, DeviceID: dev.DeviceID, Remote: a.Hash, Theirs: keep, Detected: nowUnix()})
				_ = w.send(linkMsg{T: "conflict"})
				continue
			}
			if err := l.app.writeSyncFile(m.Account, a.Path, data, a.Hash, backup); err != nil {
				_ = w.send(linkMsg{T: "err", Err: err.Error()})
				continue
			}
			base[a.Path] = a.Hash
			_ = w.send(linkMsg{T: "ok"})
		case "stats":
			l.receiveStats(m)
			_ = w.send(linkMsg{T: "stats", Data: mustJSON(l.statsBundle(m.Account))})
		}
	}
}

// ---------------------------------------------------------------- play time

type statsSnapshot struct {
	Device  string       `json:"device"`
	Name    string       `json:"name"`
	Updated int64        `json:"updated"`
	Stats   ProfileStats `json:"stats"`
}

func (l *DeviceLink) remoteStatsDir(account string) string {
	return filepath.Join(l.app.Profiles.profileDir(account), "stats-devices")
}

// statsBundle: our own statistics plus every other device's snapshot we hold.
func (l *DeviceLink) statsBundle(account string) []statsSnapshot {
	out := []statsSnapshot{{Device: l.id.DeviceID, Name: l.id.DeviceName, Updated: nowUnix(), Stats: l.app.Profiles.LocalStats(account)}}
	entries, _ := os.ReadDir(l.remoteStatsDir(account))
	for _, e := range entries {
		var s statsSnapshot
		if readJSON(filepath.Join(l.remoteStatsDir(account), e.Name()), &s) == nil && s.Device != l.id.DeviceID {
			out = append(out, s)
		}
	}
	return out
}

func (l *DeviceLink) receiveStats(m linkMsg) {
	var in []statsSnapshot
	if json.Unmarshal(m.Data, &in) != nil {
		return
	}
	_ = os.MkdirAll(l.remoteStatsDir(m.Account), 0o755)
	for _, s := range in {
		if s.Device == "" || s.Device == l.id.DeviceID {
			continue
		}
		p := filepath.Join(l.remoteStatsDir(m.Account), safeFolderName(s.Device)+".json")
		var cur statsSnapshot
		if readJSON(p, &cur) == nil && cur.Updated > s.Updated {
			continue
		}
		_ = writeJSONAtomic(p, s)
	}
	l.app.Profiles.dropStatsCache(m.Account)
}

func (l *DeviceLink) exchangeStats(w wire, account string) error {
	if err := w.send(linkMsg{T: "stats", Account: account, Data: mustJSON(l.statsBundle(account))}); err != nil {
		return err
	}
	var r linkMsg
	if err := w.recv(&r, 60*time.Second); err != nil || r.T != "stats" {
		return errors.New("the other device did not send its play time")
	}
	r.Account = account
	l.receiveStats(r)
	return nil
}
