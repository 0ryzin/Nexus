package main

// Game artwork across the user's devices (NEXUS Account, the "library" category). The library
// item of a game (account.go, syncLibrary) already carried the user's art choices (preference,
// fit) but not the pictures themselves, so a cover set on the PC had to be set again on the
// Steam Deck. Now:
//   - each custom picture (cover, Cards background, title logo) is stored once in a secret Gist of
//     the user's own GitHub account (like game saves, savesync.go), at most 8 MB;
//   - the game's library item names it by SHA-256 (artRef), so a device downloads a picture only
//     when its checksum changes, never at every sync, and an unchanged picture is never sent
//     again (System/art-sync.json remembers what was sent or received);
//   - a received picture is checked against its SHA-256 and replaces only the same game's custom
//     picture of the same kind (by Title ID); the replaced one goes to the game's backup folder.
//     Which device's choice wins is decided like any library item (newer change wins; changes on
//     both sides are a conflict the user resolves). A picture is never deleted by sync.
//   - offline: nothing is lost; pictures go out at the next sync that reaches GitHub.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxArtSync = 8 << 20

var artKinds = []string{"icon", "background", "logo"}

// artRef names a picture stored in the account.
type artRef struct {
	SHA256 string `json:"sha256"`
	Ext    string `json:"ext"`
	Size   int64  `json:"size"`
	Gist   string `json:"gist"`
	Raw    string `json:"raw"`
	// Set: when the picture was chosen on the device that sent it (Unix seconds; 0 from 0.8.x).
	// An older picture never replaces a newer one.
	Set int64 `json:"set,omitempty"`
}

type artSyncState struct {
	// Sent / Known: "<title id>/<kind>" -> the picture's reference as stored in the account
	Known map[string]artRef `json:"known"`
}

var artSyncMu sync.Mutex

func (a *App) artSyncPath() string { return filepath.Join(a.Paths.System, "art-sync.json") }

func (a *App) loadArtSync() artSyncState {
	st := artSyncState{Known: map[string]artRef{}}
	_ = readJSON(a.artSyncPath(), &st)
	if st.Known == nil {
		st.Known = map[string]artRef{}
	}
	return st
}

// customArtFile: the game's custom picture of a kind ("" when none).
func (a *App) customArtFile(key, kind string) string {
	gp := a.Library.GP.Get(key)
	name := map[string]string{"icon": gp.CustomIcon, "background": gp.CustomBackground, "logo": gp.CustomLogo}[kind]
	if name == "" {
		return ""
	}
	p := a.Library.GP.File(key, name)
	if !fileExists(p) {
		return ""
	}
	return p
}

// uploadCustomArt sends every custom picture that the account does not have yet (before the
// library items are built, so they can name it). Errors are returned; what was sent is kept.
func (a *App) uploadCustomArt(remote saveRemote) error {
	artSyncMu.Lock()
	defer artSyncMu.Unlock()
	st := a.loadArtSync()
	changed := false
	defer func() {
		if changed {
			_ = writeJSONAtomic(a.artSyncPath(), st)
		}
	}()
	entries, _ := os.ReadDir(a.Library.GP.root)
	for _, e := range entries {
		if !e.IsDir() || !tidKeyRe.MatchString(e.Name()) {
			continue
		}
		key := "t-" + strings.ToLower(e.Name())
		for _, kind := range artKinds {
			p := a.customArtFile(key, kind)
			if p == "" {
				continue
			}
			data, err := os.ReadFile(p)
			if err != nil || len(data) == 0 || len(data) > maxArtSync {
				continue
			}
			sum := sha256.Sum256(data)
			h := hex.EncodeToString(sum[:])
			id := strings.ToUpper(e.Name()) + "/" + kind
			if k := st.Known[id]; k.SHA256 == h && k.Raw != "" {
				continue // the account has this picture already
			}
			gist, raw, err := remote.putBlob(st.Known[id].Gist, "NEXUS artwork: "+id, data)
			if err != nil {
				return err
			}
			set := int64(0)
			if fi, err := os.Stat(p); err == nil {
				set = fi.ModTime().Unix()
			}
			st.Known[id] = artRef{SHA256: h, Ext: strings.ToLower(filepath.Ext(p)), Size: int64(len(data)), Gist: gist, Raw: raw, Set: set}
			changed = true
			a.Log.Printf("account: artwork %s sent (%d KB)", id, len(data)>>10)
		}
	}
	return nil
}

// artRefsFor: the references of a game's custom pictures that the account has (for its item).
func (a *App) artRefsFor(tid string) map[string]artRef {
	artSyncMu.Lock()
	st := a.loadArtSync()
	artSyncMu.Unlock()
	key := "t-" + strings.ToLower(tid)
	out := map[string]artRef{}
	for _, kind := range artKinds {
		p := a.customArtFile(key, kind)
		if p == "" {
			continue
		}
		k := st.Known[strings.ToUpper(tid)+"/"+kind]
		if k.Raw == "" {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) == k.SHA256 {
				out[kind] = k
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyArtRefs installs the pictures a received library item names, where this device's
// picture differs. The game need not be in the library yet: the picture waits in its profile.
func (a *App) applyArtRefs(remote saveRemote, tid string, refs map[string]artRef) {
	if len(refs) == 0 {
		return
	}
	key := "t-" + strings.ToLower(tid)
	for kind, ref := range refs {
		if !strings.Contains(" icon background logo ", " "+kind+" ") || ref.Raw == "" || ref.Size > maxArtSync {
			continue
		}
		ext := strings.ToLower(ref.Ext)
		if !strings.Contains(" .png .jpg .jpeg .webp .gif ", " "+ext+" ") {
			continue
		}
		if p := a.customArtFile(key, kind); p != "" {
			if data, err := os.ReadFile(p); err == nil {
				sum := sha256.Sum256(data)
				if hex.EncodeToString(sum[:]) == ref.SHA256 {
					continue // the same picture is here
				}
			}
			// a picture chosen here after the received one was chosen stays (it will be sent)
			if fi, err := os.Stat(p); err == nil && ref.Set != 0 && fi.ModTime().Unix() > ref.Set {
				a.Log.Printf("account: artwork %s/%s: this device's picture is newer; kept", tid, kind)
				continue
			}
		}
		raw, err := remote.download(ref.Raw, maxArtSync*2)
		if err != nil {
			a.Log.Printf("account: artwork %s/%s: %v", tid, kind, err)
			continue
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		sum := sha256.Sum256(data)
		if err != nil || hex.EncodeToString(sum[:]) != ref.SHA256 {
			a.Log.Printf("account: artwork %s/%s: the received picture does not match its checksum; ignored", tid, kind)
			continue
		}
		if err := a.installSyncedArt(key, kind, data, ext, ref.Set); err != nil {
			a.Log.Printf("account: artwork %s/%s: %v", tid, kind, err)
			continue
		}
		artSyncMu.Lock()
		st := a.loadArtSync()
		st.Known[strings.ToUpper(tid)+"/"+kind] = ref
		_ = writeJSONAtomic(a.artSyncPath(), st)
		artSyncMu.Unlock()
		a.Log.Printf("account: artwork %s/%s received (%d KB)", tid, kind, len(data)>>10)
	}
}

// installSyncedArt writes a received picture as the game's custom picture of that kind; the
// picture it replaces is kept in the game's profile folder (backup-<kind>-<time>.<ext>).
func (a *App) installSyncedArt(key, kind string, data []byte, ext string, setAt ...int64) error {
	set := int64(0)
	if len(setAt) > 0 {
		set = setAt[0]
	}
	gp := a.Library.GP
	if err := os.MkdirAll(gp.Dir(key), 0o755); err != nil {
		return err
	}
	if old := a.customArtFile(key, kind); old != "" {
		bak := gp.File(key, fmt.Sprintf("backup-%s-%s%s", kind, time.Now().Format("20060102-150405"), filepath.Ext(old)))
		_ = os.Rename(old, bak)
	}
	base := map[string]string{"icon": "custom-icon", "background": "custom-background", "logo": "custom-logo"}[kind]
	name := base + ext
	if err := writeFileAtomic(gp.File(key, name), data); err != nil {
		return err
	}
	if set != 0 {
		t := time.Unix(set, 0)
		_ = os.Chtimes(gp.File(key, name), t, t)
	}
	if _, err := gp.Update(key, func(p *GameProfile) {
		switch kind {
		case "icon":
			p.CustomIcon = name
			if p.ArtPreference == "" || p.ArtPreference == "auto" {
				p.ArtPreference = "custom"
			}
		case "background":
			p.CustomBackground = name
		case "logo":
			p.CustomLogo = name
		}
	}); err != nil {
		return err
	}
	if a.Library.has(key) {
		a.Library.redecorate(key)
	}
	return nil
}

// libraryArtRefs: the picture references a library item names (nil when none or unreadable).
func libraryArtRefs(data []byte) map[string]artRef {
	var x syncLibrary
	if json.Unmarshal(data, &x) != nil {
		return nil
	}
	return x.Art
}
