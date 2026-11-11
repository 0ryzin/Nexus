package main

// NEXUS engine data: NEXUS creates and owns the engine's data folder (<data>\System\Engine) and
// its configuration. Nothing has to be set up in another emulator first.
//
// Data from emulators used before (Eden, Lichii, yuzu, Citron, Sudachi, Suyu; keys from Ryujinx)
// is never taken automatically. NEXUS detects it, explains what it would bring in, and lets the
// user import it or start fresh. An import:
//   - copies (never moves or links) keys, firmware, installed updates/DLC, mods, controller
//     profiles and the game-list icons; the source folders are never changed;
//   - never overwrites a file already in NEXUS; what NEXUS already had is backed up first;
//   - writes every file through a temporary name, so an interrupted import leaves no half files;
//   - is recorded in engine-import.json: an import that was cut off (NEXUS closed, PC turned off)
//     is detected at the next start and can be retried — retrying skips what was already copied.
// Saves are not part of this import: they are offered per user (User settings › Copy earlier
// saves), with their own backup.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type LegacyItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

type LegacySource struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Path  string       `json:"path"`
	Items []LegacyItem `json:"items"`
	Bytes int64        `json:"bytes"`
	Saves int          `json:"saves"` // save files found (offered per user, not imported here)
}

// EngineImportRecord is engine-import.json.
type EngineImportRecord struct {
	Decision string `json:"decision"`         // "" undecided | "imported" | "fresh"
	State    string `json:"state"`            // "" | "running" | "done" | "interrupted" | "error"
	Source   string `json:"source,omitempty"` // source id
	Started  int64  `json:"started,omitempty"`
	Finished int64  `json:"finished,omitempty"`
	Copied   int    `json:"copied,omitempty"`
	Skipped  int    `json:"skipped,omitempty"`
	Backup   string `json:"backup,omitempty"`
	Error    string `json:"error,omitempty"`
}

type EngineDataStatus struct {
	DataDir  string             `json:"dataDir"`
	Ready    bool               `json:"ready"` // folder and configuration created
	Record   EngineImportRecord `json:"record"`
	Sources  []LegacySource     `json:"sources"`
	Ask      bool               `json:"ask"` // NEXUS should offer the import now
	Running  bool               `json:"running"`
	Progress float64            `json:"progress"`
	Message  string             `json:"message,omitempty"`
}

// the parts of an emulator data folder NEXUS can bring in (relative to the source root)
var legacyItemDefs = []struct{ id, label, rel string }{
	{"keys", "Keys (prod.keys, title.keys)", "keys"},
	{"firmware", "Firmware", filepath.Join("nand", "system")},
	{"content", "Installed updates and DLC", filepath.Join("nand", "user", "Contents")},
	{"mods", "Mods and cheats", "load"},
	{"input", "Controller profiles", filepath.Join("config", "input")},
	{"icons", "Game-list icons", filepath.Join("cache", "game_list")},
}

var legacyEmulators = []struct{ id, name, dir string }{
	{"eden", "Eden", "eden"}, {"lichii", "Lichii", "lichii"}, {"yuzu", "yuzu", "yuzu"},
	{"citron", "Citron", "citron"}, {"sudachi", "Sudachi", "sudachi"}, {"suyu", "Suyu", "suyu"},
}

type engineDataJob struct {
	mu       sync.Mutex
	running  bool
	progress float64
	message  string
}

var edJob engineDataJob

func (e *Emulator) importRecordPath() string {
	return filepath.Join(e.app.Paths.System, "engine-import.json")
}

func (e *Emulator) readImportRecord() EngineImportRecord {
	var r EngineImportRecord
	_ = readJSON(e.importRecordPath(), &r)
	return r
}

func (e *Emulator) writeImportRecord(r EngineImportRecord) error {
	return writeJSONAtomic(e.importRecordPath(), r)
}

// EnsureEngineData creates the engine's data folder, its standard sub-folders and the NEXUS
// engine configuration if they are missing. Called at every start; it never changes existing
// files. It also notices an import that was cut off.
func (e *Emulator) EnsureEngineData() error {
	dd := e.DataDir()
	for _, sub := range []string{"", "keys", filepath.Join("nand", "system"), filepath.Join("nand", "user"), "sdmc", "load", "log",
		"config", "dump", "shader", "screenshots", "lossless", "streamline"} {
		if err := os.MkdirAll(filepath.Join(dd, sub), 0o755); err != nil {
			return fmt.Errorf("the NEXUS engine data folder could not be created (%s): %w", dd, err)
		}
	}
	if !fileExists(e.ConfigPath()) {
		ini, err := LoadIni(e.ConfigPath())
		if err != nil {
			return err
		}
		seedNewConfig(ini)
		if err := ini.Save(e.ConfigPath()); err != nil {
			return fmt.Errorf("the NEXUS engine configuration could not be created: %w", err)
		}
		e.app.Log.Printf("engine data: created the NEXUS engine configuration in %s", e.ConfigDir())
	}
	r := e.readImportRecord()
	// NEXUS 2.x imported automatically and left this marker: that counts as imported
	if r.Decision == "" && fileExists(filepath.Join(dd, ".imported-previous-data")) {
		r = EngineImportRecord{Decision: "imported", State: "done", Source: "eden"}
		_ = e.writeImportRecord(r)
	}
	if r.State == "running" {
		edJob.mu.Lock()
		running := edJob.running
		edJob.mu.Unlock()
		if !running {
			r.State = "interrupted"
			_ = e.writeImportRecord(r)
			e.app.Log.Printf("engine data: the import from %s was interrupted; it can be retried", r.Source)
		}
	}
	return nil
}

func appDataDir() string { return os.Getenv("APPDATA") }

// LegacySources lists the other emulators' data found on this PC and what NEXUS could import.
func (e *Emulator) LegacySources() []LegacySource {
	ad := appDataDir()
	if ad == "" {
		return nil
	}
	var out []LegacySource
	for _, em := range legacyEmulators {
		root := filepath.Join(ad, em.dir)
		if !dirExists(root) || sameDir(root, e.DataDir()) {
			continue
		}
		src := LegacySource{ID: em.id, Name: em.name, Path: root}
		for _, it := range legacyItemDefs {
			n, b := countFiles(filepath.Join(root, it.rel))
			if n > 0 {
				src.Items = append(src.Items, LegacyItem{ID: it.id, Label: it.label, Files: n, Bytes: b})
				src.Bytes += b
			}
		}
		src.Saves, _ = countFiles(filepath.Join(root, "nand", "user", "save"))
		if len(src.Items) > 0 || src.Saves > 0 {
			out = append(out, src)
		}
	}
	// Ryujinx keeps its keys in its own layout; only the keys can be used by NEXUS
	ryu := filepath.Join(ad, "Ryujinx", "system")
	if n, b := countKeyFiles(ryu); n > 0 {
		out = append(out, LegacySource{ID: "ryujinx", Name: "Ryujinx", Path: filepath.Join(ad, "Ryujinx"), Bytes: b,
			Items: []LegacyItem{{ID: "keys", Label: "Keys (prod.keys, title.keys)", Files: n, Bytes: b}}})
	}
	return out
}

func sameDir(a, b string) bool {
	x, _ := filepath.Abs(a)
	y, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(x), filepath.Clean(y))
}

func countFiles(dir string) (int, int64) {
	n, b := 0, int64(0)
	if !dirExists(dir) {
		return 0, 0
	}
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			n++
			b += info.Size()
		}
		return nil
	})
	return n, b
}

func countKeyFiles(dir string) (int, int64) {
	n, b := 0, int64(0)
	for _, f := range []string{"prod.keys", "title.keys"} {
		if st, err := os.Stat(filepath.Join(dir, f)); err == nil && st.Mode().IsRegular() {
			n++
			b += st.Size()
		}
	}
	return n, b
}

// EngineDataStatus is what the NEXUS setup and Settings › System files show.
func (e *Emulator) EngineDataStatus() EngineDataStatus {
	r := e.readImportRecord()
	st := EngineDataStatus{DataDir: e.DataDir(), Ready: fileExists(e.ConfigPath()), Record: r, Sources: e.LegacySources()}
	if st.Sources == nil {
		st.Sources = []LegacySource{}
	}
	edJob.mu.Lock()
	st.Running, st.Progress, st.Message = edJob.running, edJob.progress, edJob.message
	edJob.mu.Unlock()
	st.Ask = !st.Running && len(st.Sources) > 0 && (r.Decision == "" || r.State == "interrupted" || r.State == "error")
	return st
}

// StartFresh records that the user does not want to import anything. Nothing is deleted.
func (e *Emulator) StartFresh() error {
	r := e.readImportRecord()
	if r.State == "running" {
		return errors.New("An import is running. Wait for it to finish.")
	}
	r.Decision, r.State, r.Error = "fresh", "done", ""
	r.Finished = time.Now().Unix()
	e.app.Log.Printf("engine data: starting fresh (nothing imported; other emulators' data left as it is)")
	return e.writeImportRecord(r)
}

// ImportLegacy copies the chosen source into the NEXUS engine data folder in the background.
// Progress is reported with "enginedata.job" events.
func (e *Emulator) ImportLegacy(sourceID string) error {
	if s := e.State(); s.Running {
		return errors.New("Close the game first.")
	}
	var src *LegacySource
	for _, s := range e.LegacySources() {
		if s.ID == sourceID {
			s := s
			src = &s
		}
	}
	if src == nil {
		return errors.New("That emulator's data was not found any more.")
	}
	edJob.mu.Lock()
	if edJob.running {
		edJob.mu.Unlock()
		return errors.New("An import is already running.")
	}
	edJob.running, edJob.progress, edJob.message = true, 0, "Preparing…"
	edJob.mu.Unlock()

	r := EngineImportRecord{Decision: "imported", State: "running", Source: src.ID, Started: time.Now().Unix()}
	if err := e.writeImportRecord(r); err != nil {
		edJob.mu.Lock()
		edJob.running = false
		edJob.mu.Unlock()
		return err
	}
	go e.runImport(*src, r)
	return nil
}

func (e *Emulator) setJob(p float64, msg string) {
	edJob.mu.Lock()
	edJob.progress, edJob.message = p, msg
	edJob.mu.Unlock()
	e.app.Events.Emit("enginedata.job", map[string]any{"progress": p, "message": msg, "running": true})
}

func (e *Emulator) runImport(src LegacySource, r EngineImportRecord) {
	finish := func(err error) {
		r.Finished = time.Now().Unix()
		if err != nil {
			r.State, r.Error = "error", err.Error()
			e.app.Log.Printf("engine data: import from %s failed: %v", src.Name, err)
		} else {
			r.State, r.Error = "done", ""
			e.app.Log.Printf("engine data: imported %d files from %s (%d already in NEXUS, kept)", r.Copied, src.Path, r.Skipped)
		}
		_ = e.writeImportRecord(r)
		edJob.mu.Lock()
		edJob.running = false
		edJob.mu.Unlock()
		e.app.Events.Emit("enginedata.job", map[string]any{"running": false, "done": err == nil, "error": r.Error, "copied": r.Copied, "skipped": r.Skipped})
		e.app.Events.Emit("engine.changed", nil)
	}
	dd := e.DataDir()

	// what to copy: (source file, destination file)
	type pair struct{ from, to string }
	var files []pair
	add := func(srcDir, dstDir string) {
		_ = filepath.Walk(srcDir, func(p string, info os.FileInfo, err error) error {
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			rel, _ := filepath.Rel(srcDir, p)
			files = append(files, pair{p, filepath.Join(dstDir, rel)})
			return nil
		})
	}
	if src.ID == "ryujinx" {
		for _, f := range []string{"prod.keys", "title.keys"} {
			if p := filepath.Join(src.Path, "system", f); fileExists(p) {
				files = append(files, pair{p, filepath.Join(dd, "keys", f)})
			}
		}
	} else {
		for _, it := range legacyItemDefs {
			add(filepath.Join(src.Path, it.rel), filepath.Join(dd, it.rel))
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].to < files[j].to })

	// back up what NEXUS already has in the places the import writes to (it is never
	// overwritten, but the user's existing NEXUS keys/config are kept safe regardless)
	backup := filepath.Join(e.app.Paths.Trash, "engine-data-backup-"+time.Now().Format("20060102-150405"))
	backedUp := 0
	for _, rel := range []string{"keys", filepath.Join("config", "input")} {
		from := filepath.Join(dd, rel)
		if n, _ := countFiles(from); n == 0 {
			continue
		}
		if err := backupTree(from, filepath.Join(backup, rel)); err != nil {
			finish(fmt.Errorf("the backup of the existing NEXUS engine data failed, so nothing was imported: %w", err))
			return
		}
		backedUp++
	}
	if backedUp > 0 {
		r.Backup = backup
		_ = e.writeImportRecord(r)
	}

	for i, f := range files {
		if i%25 == 0 {
			e.setJob(float64(i)/float64(max(1, len(files))), fmt.Sprintf("Copying from %s… %d of %d files", src.Name, i, len(files)))
		}
		if fileExists(f.to) {
			r.Skipped++
			continue
		}
		if err := copyFileTmpRename(f.from, f.to); err != nil {
			finish(fmt.Errorf("%s could not be copied: %w", filepath.Base(f.from), err))
			return
		}
		r.Copied++
		if r.Copied%200 == 0 {
			_ = e.writeImportRecord(r) // progress survives a crash; a retry skips what is there
		}
	}
	finish(nil)
}

// copyFileTmpRename copies through a temporary file in the destination folder, so a crash never
// leaves a partly written file under the final name.
func copyFileTmpRename(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := to + ".nexus-tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if st, err := in.Stat(); err == nil {
		_ = os.Chtimes(tmp, st.ModTime(), st.ModTime())
	}
	return os.Rename(tmp, to)
}

func backupTree(from, to string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFileTmpRename(p, filepath.Join(to, rel))
	})
}
