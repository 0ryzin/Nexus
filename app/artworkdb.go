package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------- built-in database
//
// Resources/ArtworkDB/index.json ships with NEXUS and maps games to artwork
// files in the same folder. An entry is applied only when the game is
// identified with certainty:
//   - its Title ID is listed, or
//   - a trusted name (read from the game's own control data, or an online
//     name whose Title ID was verified) matches exactly, nothing in the name is
//     excluded, the publisher matches and it is not homebrew; or
//   - with no trusted name at all, the file name contains every required word.
// If a trusted name exists but doesn't match, the file name is never used.

type BuiltinArt struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	File        string   `json:"file"`
	TitleIDs    []string `json:"titleIds"`
	Names       []string `json:"names"`     // exact names after normalisation
	Exclude     []string `json:"exclude"`   // any of these words/phrases disqualifies
	Publisher   string   `json:"publisher"` // required (case-insensitive substring) when matching by name
	FilenameAll []string `json:"filenameAll"`
	Note        string   `json:"note"`
}

type builtinDB struct {
	once    sync.Once
	dir     string
	entries []BuiltinArt
}

func (l *Library) builtinDB() *builtinDB {
	l.bdbOnce.Do(func() {
		db := &builtinDB{dir: filepath.Join(l.app.Paths.Resources, "ArtworkDB")}
		var idx struct {
			Entries []BuiltinArt `json:"entries"`
		}
		if err := readJSON(filepath.Join(db.dir, "index.json"), &idx); err == nil {
			for _, e := range idx.Entries {
				if e.File != "" && fileExists(filepath.Join(db.dir, filepath.Base(e.File))) {
					db.entries = append(db.entries, e)
				}
			}
		}
		l.bdb = db
	})
	return l.bdb
}

func normName(s string) string { return strings.Join(nameTokens(s), " ") }

func containsPhrase(norm, phrase string) bool {
	p := normName(phrase)
	return p != "" && strings.Contains(" "+norm+" ", " "+p+" ")
}

// MatchBuiltin decides whether entry e applies to the game. reason explains
// the decision (shown in Game Information).
func MatchBuiltin(e BuiltinArt, g *Game, p GameProfile) (bool, string) {
	if g.Kind == "homebrew" || g.Kind == "update" || g.Kind == "dlc" {
		return false, "not a retail game"
	}
	for _, id := range e.TitleIDs {
		if g.TitleID != "" && strings.EqualFold(id, g.TitleID) {
			return true, "Title ID " + g.TitleID
		}
	}
	excluded := func(n string) bool {
		for _, x := range e.Exclude {
			if containsPhrase(n, x) {
				return true
			}
		}
		return false
	}
	// trusted names
	name, pub, src := "", "", ""
	switch {
	case g.EmbedName != "" && (g.IDSource == "nca" || g.IDSource == "ticket"):
		name, pub, src = g.EmbedName, g.Publisher, "the game's own data"
	case p.Online.Name != "" && g.TitleID != "" && g.IDSource != "filename":
		name, pub, src = p.Online.Name, p.Online.Publisher, "verified online metadata"
	}
	if name != "" {
		n := normName(name)
		hit := false
		for _, want := range e.Names {
			if n == normName(want) {
				hit = true
			}
		}
		if !hit || excluded(n) {
			return false, ""
		}
		if e.Publisher != "" && !strings.Contains(strings.ToLower(pub), strings.ToLower(e.Publisher)) {
			return false, "name matches but the publisher is not " + e.Publisher
		}
		if g.TitleID != "" && !(strings.HasPrefix(g.TitleID, "01") || strings.HasPrefix(g.TitleID, "04")) {
			return false, "not a retail Title ID"
		}
		return true, "name and publisher from " + src
	}
	// no trusted name: the file name must say it explicitly
	if len(e.FilenameAll) == 0 {
		return false, ""
	}
	fn := normName(g.FileTitle)
	if excluded(fn) {
		return false, ""
	}
	for _, w := range e.FilenameAll {
		if !containsPhrase(fn, w) {
			return false, ""
		}
	}
	return true, "file name"
}

// builtinArt returns the built-in artwork for g, if one applies.
func (l *Library) builtinArt(g *Game, p GameProfile) string {
	db := l.builtinDB()
	for _, e := range db.entries {
		if ok, why := MatchBuiltin(e, g, p); ok {
			g.BuiltinArt, g.BuiltinWhy = e.ID, why
			return filepath.Join(db.dir, filepath.Base(e.File))
		}
	}
	g.BuiltinArt, g.BuiltinWhy = "", ""
	return ""
}

// ---------------------------------------------------------------- downloaded artwork database
//
// Every image downloaded for a Title ID is also kept in
// <data>\Artwork\Database\<TitleID>.<ext> with its metadata, so it stays
// available offline, is never downloaded twice (even after a game file moves,
// a second copy appears, or its GameProfiles folder is reset) and can be
// browsed or cleared as one database. Artwork you chose yourself is never
// stored here or replaced.

type ArtDBRecord struct {
	TitleID   string `json:"titleId"`
	Name      string `json:"name,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	Released  string `json:"released,omitempty"`
	File      string `json:"file"`
	Source    string `json:"source"`
	Fetched   int64  `json:"fetched"`
	Bytes     int64  `json:"bytes"`
}

type artCache struct {
	mu  sync.Mutex
	dir string
	idx map[string]ArtDBRecord
}

func (l *Library) artCache() *artCache {
	l.acOnce.Do(func() {
		c := &artCache{dir: filepath.Join(l.app.Paths.Artwork, "Database"), idx: map[string]ArtDBRecord{}}
		_ = readJSON(filepath.Join(c.dir, "index.json"), &c.idx)
		if c.idx == nil {
			c.idx = map[string]ArtDBRecord{}
		}
		l.ac = c
	})
	return l.ac
}

func (c *artCache) Get(tid string) (ArtDBRecord, []byte, bool) {
	c.mu.Lock()
	r, ok := c.idx[strings.ToUpper(tid)]
	c.mu.Unlock()
	if !ok {
		return r, nil, false
	}
	data, err := os.ReadFile(filepath.Join(c.dir, r.File))
	if err != nil {
		return r, nil, false
	}
	return r, data, true
}

func (c *artCache) Put(r ArtDBRecord, data []byte, ext string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = os.MkdirAll(c.dir, 0o755)
	r.TitleID = strings.ToUpper(r.TitleID)
	r.File = r.TitleID + ext
	r.Bytes = int64(len(data))
	if r.Fetched == 0 {
		r.Fetched = time.Now().Unix()
	}
	if err := os.WriteFile(filepath.Join(c.dir, r.File), data, 0o644); err != nil {
		return
	}
	c.idx[r.TitleID] = r
	_ = writeJSONAtomic(filepath.Join(c.dir, "index.json"), c.idx)
}

func (c *artCache) Stats() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	var bytes int64
	for _, r := range c.idx {
		bytes += r.Bytes
	}
	return map[string]any{"count": len(c.idx), "bytes": bytes, "dir": c.dir}
}

// Clear empties the downloaded-artwork database (built-in and user-chosen
// artwork are not affected). Games keep their current copy until refreshed.
func (c *artCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.idx = map[string]ArtDBRecord{}
	if err := os.RemoveAll(c.dir); err != nil {
		return err
	}
	return nil
}

// BuiltinInfo describes the built-in database (Settings › Game library).
func (l *Library) BuiltinInfo() []map[string]any {
	db := l.builtinDB()
	out := []map[string]any{}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range db.entries {
		var games []string
		for _, g := range l.byKey {
			if g.BuiltinArt == e.ID {
				games = append(games, g.Title)
			}
		}
		out = append(out, map[string]any{"id": e.ID, "title": e.Title, "note": e.Note, "matched": games})
	}
	return out
}
