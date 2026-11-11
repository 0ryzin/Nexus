package main

// Media library: screenshots and recordings captured through NEXUS.
//
//	<data>\Media\Screenshots\   PNG screenshots (F12 or the controller's capture button in a game)
//	<data>\Media\Recordings\    recordings (any MP4/WebM/MKV/MOV placed here is shown too)
//
// The folders are the library: the Media tab lists what is on disk, so files added, renamed or
// removed in Explorer show up correctly. NEXUS remembers which game (and user) each capture
// belongs to in Media\.nexus-media.json; files NEXUS did not record are matched to a game by the
// short name at the start of their file name, when possible. Captures live in the user data
// folder, never in temporary folders, and updates of NEXUS never touch them. File names are
// unique (a number is added rather than replacing a file).

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const mediaIndexFile = ".nexus-media.json"

type MediaInfo struct {
	GameKey   string `json:"gameKey,omitempty"`
	TitleID   string `json:"titleId,omitempty"`
	Title     string `json:"title,omitempty"`
	ProfileID string `json:"profileId,omitempty"`
	Captured  int64  `json:"captured,omitempty"`
}

type MediaItem struct {
	MediaInfo
	ID       string `json:"id"`   // path relative to the Media folder, with forward slashes
	Type     string `json:"type"` // screenshot | recording
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	Modified int64  `json:"modified"`
	Ext      string `json:"ext"`
}

type MediaLibrary struct {
	app *App
	mu  sync.Mutex
}

var (
	mediaImageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true, ".gif": true}
	mediaVideoExts = map[string]bool{".mp4": true, ".webm": true, ".mkv": true, ".mov": true, ".m4v": true, ".avi": true}
)

func NewMediaLibrary(a *App) *MediaLibrary { return &MediaLibrary{app: a} }

func (m *MediaLibrary) Root() string           { return m.app.Paths.Media }
func (m *MediaLibrary) ScreenshotsDir() string { return filepath.Join(m.Root(), "Screenshots") }
func (m *MediaLibrary) RecordingsDir() string  { return filepath.Join(m.Root(), "Recordings") }

func (m *MediaLibrary) index() map[string]MediaInfo {
	idx := map[string]MediaInfo{}
	_ = readJSON(filepath.Join(m.Root(), mediaIndexFile), &idx)
	return idx
}

func (m *MediaLibrary) saveIndex(idx map[string]MediaInfo) {
	p := filepath.Join(m.Root(), mediaIndexFile)
	clearHidden(p)
	_ = writeJSONAtomic(p, idx)
	setHidden(p)
}

// rel turns a path inside the Media folder into an item ID ("" if it is outside).
func (m *MediaLibrary) rel(path string) string {
	r, err := filepath.Rel(m.Root(), path)
	if err != nil || strings.HasPrefix(r, "..") || filepath.IsAbs(r) {
		return ""
	}
	return filepath.ToSlash(r)
}

// Path resolves an item ID to its file (only files inside the Media folders).
func (m *MediaLibrary) Path(id string) (string, error) {
	id = filepath.ToSlash(id)
	if id == "" || strings.Contains(id, "..") || strings.Contains(id, ":") || strings.HasPrefix(id, "/") ||
		!(strings.HasPrefix(id, "Screenshots/") || strings.HasPrefix(id, "Recordings/")) {
		return "", errors.New("not a media file")
	}
	p := filepath.Join(m.Root(), filepath.FromSlash(id))
	if !fileExists(p) {
		return "", errors.New("This capture no longer exists. It may have been moved or deleted outside NEXUS.")
	}
	return p, nil
}

// Record remembers the game and user of a capture NEXUS just saved.
func (m *MediaLibrary) Record(path string, info MediaInfo) {
	id := m.rel(path)
	if id == "" {
		return
	}
	if info.Captured == 0 {
		info.Captured = time.Now().Unix()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.index()
	idx[id] = info
	m.saveIndex(idx)
}

// List returns the captures on disk, newest first. Index entries of files that are gone are
// dropped; files NEXUS did not record are matched to a game by their file name when possible.
func (m *MediaLibrary) List() []MediaItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.index()
	byShort := map[string]Game{}
	if m.app.Library != nil {
		for _, g := range m.app.Library.Games() {
			if g.Kind == "update" || g.Kind == "dlc" {
				continue
			}
			if sh := GameShortName(g.Title); sh != "" {
				byShort[strings.ToUpper(sh)] = g
			}
			byShort[strings.ToUpper(safeFolderName(g.Title))] = g
		}
	}
	out := []MediaItem{}
	seen := map[string]bool{}
	for _, dir := range []struct{ path, typ string }{{m.ScreenshotsDir(), "screenshot"}, {m.RecordingsDir(), "recording"}} {
		_ = filepath.WalkDir(dir.path, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if !(mediaImageExts[ext] || mediaVideoExts[ext]) || strings.HasSuffix(d.Name(), ".part") {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			id := m.rel(p)
			typ := dir.typ
			if mediaVideoExts[ext] {
				typ = "recording"
			} else {
				typ = "screenshot"
			}
			it := MediaItem{ID: id, Type: typ, Name: d.Name(), Bytes: fi.Size(), Modified: fi.ModTime().Unix(), Ext: strings.TrimPrefix(ext, ".")}
			if info, ok := idx[id]; ok {
				it.MediaInfo = info
			} else {
				// "<game> 2026-11-11 21-30-00.png": the part before the date names the game
				stem := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
				if i := strings.LastIndex(stem, " 20"); i > 0 {
					if g, ok := byShort[strings.ToUpper(strings.TrimSpace(stem[:i]))]; ok {
						it.GameKey, it.TitleID, it.Title = g.Key, g.TitleID, g.Title
					}
				}
			}
			if it.Captured == 0 {
				it.Captured = it.Modified
			}
			if it.GameKey != "" && it.Title == "" && m.app.Library != nil {
				if g, ok := m.app.Library.Get(it.GameKey); ok {
					it.Title = g.Title
				}
			}
			seen[id] = true
			out = append(out, it)
			return nil
		})
	}
	changed := false
	for id := range idx {
		if !seen[id] {
			delete(idx, id) // moved or deleted outside NEXUS
			changed = true
		}
	}
	if changed {
		m.saveIndex(idx)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Captured != out[j].Captured {
			return out[i].Captured > out[j].Captured
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// Delete moves a capture to the Recycle Bin (Windows) or deletes it.
func (m *MediaLibrary) Delete(id string) error {
	p, err := m.Path(id)
	if err != nil {
		return err
	}
	if err := recycleFile(p); err != nil {
		return err
	}
	m.mu.Lock()
	idx := m.index()
	delete(idx, filepath.ToSlash(id))
	m.saveIndex(idx)
	m.mu.Unlock()
	m.app.Log.Printf("media: deleted %s", id)
	return nil
}

// captureName is the start of a screenshot's file name: the game's short name, else its title.
func captureName(g *Game) string {
	if g == nil {
		return "NEXUS"
	}
	if sh := GameShortName(g.Title); sh != "" {
		return sh
	}
	if n := safeFolderName(g.Title); n != "" && !strings.HasPrefix(n, "_") {
		return n
	}
	return "NEXUS"
}
