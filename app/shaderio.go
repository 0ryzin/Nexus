package main

// Import Shader Data / Export Shader Data from a game's menu (docs/SHADERS.md).
//
//   - Export writes "<title> shaders [<title id>].zip" into a folder the user picks: the game's
//     portable shader data as a NEXUS shader pack (shadercache.go, NXSHPACK: program, pipeline and
//     session records only, never driver-compiled code) and nexus-shader-export.json, which names
//     the game, its title ID, the format, the exporting device and NEXUS version and each file's
//     SHA-256. Nothing else (no lock or temporary files, no driver caches).
//   - Import from a ZIP: the archive is read where it is (no upload through the interface), by
//     ImportShaderZip (shaderzip.go), which recognises each file's format from its own header and
//     converts only what the NEXUS engine can use. A NEXUS export's manifest must name this game.
//   - Import from another device: the other NEXUS device sends its shader pack for this title
//     directly over Device Linking (devicelink.go: paired devices, mutual TLS on the local
//     network). The pack holds the game's shader code, so it never goes through the NEXUS Account
//     (GitHub).
//
// Every import first copies the game's current shader log to a backup next to it, runs in the
// background (one job at a time, progress as events) and reports what was added, what was already
// there and what was skipped and why. The source file is never changed or deleted.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	shaderExportManifest = "nexus-shader-export.json"
	shaderExportKind     = "nexus-shader-export"
	maxShaderZipTotal    = 8 << 30 // uncompressed bytes read from one archive
	maxShaderZipFiles    = 200000
	shaderBackupsKept    = 3
)

// ShaderExportManifest is nexus-shader-export.json.
type ShaderExportManifest struct {
	Kind     string               `json:"kind"`
	Version  int                  `json:"version"`
	Title    string               `json:"title"`
	TitleID  string               `json:"titleId"`
	Emulator string               `json:"emulator"` // "NEXUS Engine"
	Format   string               `json:"format"`   // "NEXUS shader pack (shader cache v2 records)"
	Device   string               `json:"device"`
	Nexus    string               `json:"nexus"`
	Created  int64                `json:"created"`
	Pack     ShaderPackInfo       `json:"pack"`
	Files    []ShaderExportedFile `json:"files"`
}

type ShaderExportedFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// ShaderJob is the one import or export running (or the last one).
type ShaderJob struct {
	ID       string              `json:"id"`
	Kind     string              `json:"kind"` // export | import-zip | import-device
	GameKey  string              `json:"gameKey"`
	Title    string              `json:"title"`
	State    string              `json:"state"` // running | done | failed
	Step     string              `json:"step"`
	Error    string              `json:"error,omitempty"`
	Started  int64               `json:"started"`
	Finished int64               `json:"finished,omitempty"`
	Export   *ShaderExportOK     `json:"export,omitempty"`
	Zip      *ShaderZipResult    `json:"zip,omitempty"`
	Pack     *ShaderImportResult `json:"pack,omitempty"`
	Backup   string              `json:"backup,omitempty"`
	Device   string              `json:"device,omitempty"`
}

type ShaderExportOK struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Programs int    `json:"programs"`
	Graphics int    `json:"graphics"`
	Compute  int    `json:"compute"`
}

type shaderJobs struct {
	mu  sync.Mutex
	cur *ShaderJob
}

func (s *shaderJobs) get() *ShaderJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return nil
	}
	c := *s.cur
	return &c
}

// start runs fn in the background as the one shader job; refused while another runs.
func (e *Emulator) startShaderJob(kind string, g *Game, fn func(j *ShaderJob, step func(string)) error) (*ShaderJob, error) {
	s := &e.shaderJobs
	s.mu.Lock()
	if s.cur != nil && s.cur.State == "running" {
		s.mu.Unlock()
		return nil, errors.New("Another shader import or export is still running; wait for it to finish.")
	}
	j := &ShaderJob{ID: newID(), Kind: kind, GameKey: g.Key, Title: g.Title, State: "running", Started: nowUnix()}
	s.cur = j
	s.mu.Unlock()
	emit := func() {
		if c := s.get(); c != nil {
			e.app.Events.Emit("shaders.job", c)
		}
	}
	step := func(t string) {
		s.mu.Lock()
		j.Step = t
		s.mu.Unlock()
		emit()
	}
	go func() {
		err := fn(j, step)
		s.mu.Lock()
		j.Finished = nowUnix()
		if err != nil {
			j.State, j.Error = "failed", err.Error()
		} else {
			j.State = "done"
		}
		s.mu.Unlock()
		if err != nil {
			e.app.Log.Printf("shaders: %s for %s failed: %v", kind, g.TitleID, err)
		}
		emit()
	}()
	c := *j
	return &c, nil
}

// backupShaderLog copies the game's shader log (if any) to "<log>.before-import-<time>.bak" and
// keeps the newest few. Returns the backup's path ("" when the game had no shader data yet).
func (e *Emulator) backupShaderLog(titleID string) (string, error) {
	src := e.ShaderLogPath(titleID)
	if !fileExists(src) {
		return "", nil
	}
	dst := src + ".before-import-" + time.Now().Format("20060102-150405") + ".bak"
	if err := copyFileOverwrite(src, dst); err != nil {
		return "", fmt.Errorf("the current shader data could not be backed up, so nothing was imported: %w", err)
	}
	old, _ := filepath.Glob(src + ".before-import-*.bak")
	sort.Strings(old)
	for len(old) > shaderBackupsKept {
		os.Remove(old[0])
		old = old[1:]
	}
	return dst, nil
}

// RestoreShaderBackup puts back the game's shader data from the newest backup made before an
// import (0.9.3: an import could leave a game with none of its shaders built at start). The data
// it replaces is backed up first ("<log>.before-restore-<time>.bak").
func (e *Emulator) RestoreShaderBackup(g *Game) (restored string, records int, err error) {
	if err := e.shaderFilesBusy(); err != nil {
		return "", 0, err
	}
	src := e.ShaderLogPath(g.TitleID)
	baks, _ := filepath.Glob(src + ".before-import-*.bak")
	if len(baks) == 0 {
		return "", 0, errors.New("There is no backup from before an import for this game.")
	}
	sort.Strings(baks)
	bak := baks[len(baks)-1]
	b, err := os.ReadFile(bak)
	if err != nil {
		return "", 0, err
	}
	if _, _, _, err := parseV2Log(b); err != nil {
		return "", 0, fmt.Errorf("the backup %s cannot be read (%v); nothing was changed", filepath.Base(bak), err)
	}
	unlock, err := lockWait(src+".lock", 5*time.Second)
	if err != nil {
		return "", 0, err
	}
	defer unlock()
	if fileExists(src) {
		if err := copyFileOverwrite(src, src+".before-restore-"+time.Now().Format("20060102-150405")+".bak"); err != nil {
			return "", 0, fmt.Errorf("the current shader data could not be backed up, so nothing was restored: %w", err)
		}
	}
	tmp := src + ".nexus-restore"
	if err := os.WriteFile(tmp, b, 0o666); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	if err := os.Rename(tmp, src); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	n := countShaderRecords(src)
	e.app.Log.Printf("shaders: restored %s from %s (%d shader records)", g.TitleID, filepath.Base(bak), n)
	return filepath.Base(bak), n, nil
}

// ExportShaderZip writes the game's export ZIP into dir and returns its path.
func (e *Emulator) ExportShaderZip(g *Game, dir string) (ShaderExportOK, error) {
	var ok ShaderExportOK
	var pack bytes.Buffer
	info, err := e.ExportShaders(g, &pack)
	if err != nil {
		return ok, err
	}
	packName := shaderPackFileName(g.Title, info.TitleID)
	sum := sha256.Sum256(pack.Bytes())
	man := ShaderExportManifest{Kind: shaderExportKind, Version: 1, Title: g.Title, TitleID: info.TitleID,
		Emulator: "NEXUS Engine", Format: "NEXUS shader pack (shader cache v2 records)", Device: info.Device,
		Nexus: AppVersion, Created: info.Created, Pack: info,
		Files: []ShaderExportedFile{{Name: "shaders/" + packName, Size: int64(pack.Len()), SHA256: hex.EncodeToString(sum[:])}}}
	manJSON, _ := json.MarshalIndent(man, "", "  ")
	name := strings.TrimSuffix(packName, ".nxshaders") + ".zip"
	dst := filepath.Join(dir, name)
	tmp := dst + ".nexus-part"
	f, err := os.Create(tmp)
	if err != nil {
		return ok, fmt.Errorf("NEXUS cannot write to that folder: %w", err)
	}
	zw := zip.NewWriter(f)
	write := func(n string, b []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: time.Unix(info.Created, 0)})
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	err = write(shaderExportManifest, manJSON)
	if err == nil {
		err = write("shaders/"+packName, pack.Bytes())
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return ok, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return ok, err
	}
	fi, _ := os.Stat(dst)
	ok = ShaderExportOK{Path: dst, Programs: info.Programs, Graphics: info.Graphics, Compute: info.Compute}
	if fi != nil {
		ok.Bytes = fi.Size()
	}
	e.app.Log.Printf("shaders: exported %s to a ZIP (%d bytes)", info.TitleID, ok.Bytes)
	return ok, nil
}

// checkShaderZip refuses archives NEXUS will not read before anything is changed: too many files,
// too large in total, or a NEXUS export for another game (or with a damaged file).
func checkShaderZip(zipPath string, g *Game) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("This file is not a ZIP archive NEXUS can read (%v).", err)
	}
	defer zr.Close()
	if len(zr.File) > maxShaderZipFiles {
		return fmt.Errorf("The archive has %d files; NEXUS reads at most %d.", len(zr.File), maxShaderZipFiles)
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
	}
	if total > maxShaderZipTotal {
		return fmt.Errorf("The archive unpacks to %.1f GB; NEXUS reads at most %d GB of shader data at once.", float64(total)/(1<<30), maxShaderZipTotal>>30)
	}
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(strings.ReplaceAll(f.Name, "\\", "/")), shaderExportManifest) {
			b, err := readZipEntry(f, 1<<20)
			var m ShaderExportManifest
			if err != nil || json.Unmarshal(b, &m) != nil || m.Kind != shaderExportKind {
				return errors.New("The archive's NEXUS export description is damaged; nothing was imported.")
			}
			if !strings.EqualFold(m.TitleID, g.TitleID) {
				return fmt.Errorf("This is %s's shader data (%s), not %s's. Import it from that game's menu.", firstNonEmpty(m.Title, "another game"), m.TitleID, g.Title)
			}
			for _, x := range m.Files {
				for _, zf := range zr.File {
					if zf.Name != x.Name {
						continue
					}
					b, err := readZipEntry(zf, maxShaderPackSize)
					sum := sha256.Sum256(b)
					if err != nil || hex.EncodeToString(sum[:]) != x.SHA256 {
						return fmt.Errorf("%s in the archive is damaged or incomplete (its checksum does not match); copy the ZIP again.", x.Name)
					}
				}
			}
		}
	}
	return nil
}

// StartShaderExport exports in the background into dir.
func (e *Emulator) StartShaderExport(g *Game, dir string) (*ShaderJob, error) {
	return e.startShaderJob("export", g, func(j *ShaderJob, step func(string)) error {
		step("Packing the shader data")
		ok, err := e.ExportShaderZip(g, dir)
		if err != nil {
			return err
		}
		e.shaderJobs.mu.Lock()
		j.Export = &ok
		e.shaderJobs.mu.Unlock()
		return nil
	})
}

// StartShaderZipImport imports a ZIP in the background (the file stays where it is, unchanged).
func (e *Emulator) StartShaderZipImport(g *Game, zipPath string) (*ShaderJob, error) {
	if err := e.shaderFilesBusy(); err != nil {
		return nil, err
	}
	return e.startShaderJob("import-zip", g, func(j *ShaderJob, step func(string)) error {
		step("Checking the archive")
		if err := checkShaderZip(zipPath, g); err != nil {
			return err
		}
		step("Backing up this game's shader data")
		bak, err := e.backupShaderLog(g.TitleID)
		if err != nil {
			return err
		}
		step("Reading and converting the shader caches")
		res, err := e.ImportShaderZip(zipPath, g)
		e.shaderJobs.mu.Lock()
		j.Backup, j.Zip = bak, &res
		e.shaderJobs.mu.Unlock()
		return err
	})
}

// StartShaderDeviceImport fetches the game's shader pack from a linked device and imports it.
func (e *Emulator) StartShaderDeviceImport(g *Game, deviceID string) (*ShaderJob, error) {
	l := e.app.Link
	if l == nil {
		return nil, errors.New("Device Linking is off on this device, so it cannot reach your other devices. Use Import from a ZIP file instead (Export Shader Data on the other device makes one).")
	}
	if err := e.shaderFilesBusy(); err != nil {
		return nil, err
	}
	var dev *TrustedDevice
	for _, d := range l.Devices() {
		if d.DeviceID == deviceID {
			d := d
			dev = &d
		}
	}
	if dev == nil {
		return nil, errors.New("That device is not linked to this one.")
	}
	return e.startShaderJob("import-device", g, func(j *ShaderJob, step func(string)) error {
		e.shaderJobs.mu.Lock()
		j.Device = dev.Name
		e.shaderJobs.mu.Unlock()
		step("Asking " + dev.Name + " for the shader data")
		pack, err := l.FetchShaderPack(*dev, g.TitleID)
		if err != nil {
			return err
		}
		step("Backing up this game's shader data")
		bak, err := e.backupShaderLog(g.TitleID)
		if err != nil {
			return err
		}
		step("Adding the shader data")
		res, err := e.ImportShaders(bytes.NewReader(pack), func(id string) *Game {
			if strings.EqualFold(id, g.TitleID) {
				return g
			}
			return nil
		})
		e.shaderJobs.mu.Lock()
		j.Backup, j.Pack = bak, &res
		e.shaderJobs.mu.Unlock()
		return err
	})
}

// ---------- Device Linking: the shader pack of one title from a linked device

// FetchShaderPack asks a linked device for its shader pack of titleID.
func (l *DeviceLink) FetchShaderPack(dev TrustedDevice, titleID string) ([]byte, error) {
	conn, err := l.dialPeer(dev, dev.Addr)
	if err != nil {
		return nil, fmt.Errorf("%s cannot be reached: make sure NEXUS is open on it and both devices are on the same network (%v).", dev.Name, err)
	}
	defer conn.Close()
	w := wire{bufio.NewReader(conn), conn}
	if err := w.send(linkMsg{T: "hello", Proto: linkProto, Device: l.id.DeviceID}); err != nil {
		return nil, err
	}
	var hello linkMsg
	if err := w.recv(&hello, 30*time.Second); err != nil || hello.T != "hello" {
		return nil, fmt.Errorf("%s did not answer.", dev.Name)
	}
	if hello.Proto != linkProto {
		return nil, errors.New("the other device runs a NEXUS version with a different link format; update both")
	}
	account := ""
	if len(dev.Accounts) > 0 {
		account = dev.Accounts[0]
	}
	if err := w.send(linkMsg{T: "shaders", Account: account, Data: mustJSON(map[string]string{"titleId": strings.ToUpper(titleID)})}); err != nil {
		return nil, err
	}
	var m linkMsg
	if err := w.recv(&m, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("%s stopped answering: %v", dev.Name, err)
	}
	_ = w.send(linkMsg{T: "bye"})
	if m.T == "err" {
		return nil, fmt.Errorf("%s: %s", dev.Name, m.Err)
	}
	if m.T != "shaders" {
		return nil, fmt.Errorf("%s sent something unexpected.", dev.Name)
	}
	var s string
	if err := json.Unmarshal(m.Data, &s); err != nil {
		return nil, err
	}
	return b64.DecodeString(s)
}

// serveShaderPack answers a linked device's "shaders" request with this device's pack.
func (l *DeviceLink) serveShaderPack(w wire, m linkMsg) {
	var a struct {
		TitleID string `json:"titleId"`
	}
	_ = json.Unmarshal(m.Data, &a)
	var g *Game
	for _, x := range l.app.Library.Games() {
		if strings.EqualFold(x.TitleID, a.TitleID) {
			x := x
			g = &x
			break
		}
	}
	if g == nil {
		_ = w.send(linkMsg{T: "err", Err: "this game is not in the library there"})
		return
	}
	var buf bytes.Buffer
	if _, err := l.app.Emu.ExportShaders(g, &buf); err != nil {
		_ = w.send(linkMsg{T: "err", Err: err.Error()})
		return
	}
	_ = w.send(linkMsg{T: "shaders", Data: mustJSON(b64.EncodeToString(buf.Bytes()))})
}
