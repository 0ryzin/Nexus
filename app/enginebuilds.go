package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// EngineBuilds manages the NEXUS engine (nexus_engine.dll plus the libraries it needs).
//
// The engine ships inside the NEXUS folder: <NEXUS>/Engine holds nexus_engine.dll, engine.json
// (version, source commit, SHA-256 of every file) and Libraries/ with its dependencies. It is
// found relative to NEXUS.exe, so the extracted folder works wherever it is placed, and it is
// replaced together with NEXUS when NEXUS is updated. Nothing has to be installed.
//
// For developers and testers, other engine builds can be added: they are kept in the user data
// folder (<data>/EngineBuilds/<version>), never overwrite the bundled engine, and can be switched
// back to the bundled engine at any time. Every file is checked against its manifest before use.

type EngineManifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	API          int               `json:"api"`
	Commit       string            `json:"commit"`
	UpstreamBase string            `json:"upstreamBase"`
	Files        map[string]string `json:"files"`
}

type EngineBuild struct {
	Version   string         `json:"version"`
	Dir       string         `json:"dir"`
	Active    bool           `json:"active"`
	Installed int64          `json:"installed"`
	Manifest  EngineManifest `json:"manifest"`
	Problem   string         `json:"problem,omitempty"`
	Bundled   bool           `json:"bundled,omitempty"` // the engine that ships inside the NEXUS folder
}

const bundledVersion = "bundled"

type EngineBuilds struct {
	e  *Emulator
	mu sync.Mutex
	// verified caches successful verifications this run (dir -> manifest stamp)
	verified map[string]string
	// Job reports an install in progress
	job struct {
		Active   bool    `json:"active"`
		Message  string  `json:"message"`
		Error    string  `json:"error,omitempty"`
		Progress float64 `json:"progress"`
	}
}

func NewEngineBuilds(e *Emulator) *EngineBuilds { return &EngineBuilds{e: e} }

// Root is the engine bundled with NEXUS: <folder of NEXUS.exe>/Engine.
func (b *EngineBuilds) Root() string        { return filepath.Join(b.e.app.Paths.Root, "Engine") }
func (b *EngineBuilds) BuildsDir() string   { return filepath.Join(b.e.app.Paths.System, "EngineBuilds") }
func (b *EngineBuilds) currentFile() string { return filepath.Join(b.BuildsDir(), "current.txt") }

// BundledPresent: the NEXUS folder contains an engine.
func (b *EngineBuilds) BundledPresent() bool {
	return fileExists(filepath.Join(b.Root(), "engine.json")) && fileExists(filepath.Join(b.Root(), "nexus_engine.dll"))
}

// selection is the side build the user switched to ("" = the bundled engine).
func (b *EngineBuilds) selection() string {
	data, err := os.ReadFile(b.currentFile())
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(data))
	if !safeVersion.MatchString(v) || v == bundledVersion || !fileExists(filepath.Join(b.BuildsDir(), v, "nexus_engine.dll")) {
		return ""
	}
	return v
}

var safeVersion = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)

// activeVersion names the engine NEXUS uses: a side build's version, or "bundled".
func (b *EngineBuilds) activeVersion() string {
	if v := b.selection(); v != "" {
		return v
	}
	if b.BundledPresent() {
		return bundledVersion
	}
	return ""
}

// ActiveVersionLabel is the version number shown to people.
func (b *EngineBuilds) ActiveVersionLabel() string {
	v := b.activeVersion()
	if v == bundledVersion {
		if m, err := readManifest(b.Root()); err == nil {
			return m.Version
		}
	}
	return v
}

// ActiveDir is the folder of the engine in use ("" when none).
func (b *EngineBuilds) ActiveDir() string {
	switch v := b.activeVersion(); v {
	case "":
		return ""
	case bundledVersion:
		return b.Root()
	default:
		return filepath.Join(b.BuildsDir(), v)
	}
}

func (b *EngineBuilds) HasActive() bool {
	d := b.ActiveDir()
	return d != "" && fileExists(filepath.Join(d, "nexus_engine.dll"))
}


// VerifyActive checks the engine in use against its manifest once per run.
func (b *EngineBuilds) VerifyActive() error {
	dir := b.ActiveDir()
	if dir == "" {
		return errors.New("no engine")
	}
	fi, err := os.Stat(filepath.Join(dir, "engine.json"))
	if err != nil {
		return errors.New("engine.json is missing: the NEXUS folder is incomplete. Extract the whole NEXUS ZIP again.")
	}
	stamp := fmt.Sprint(fi.Size(), fi.ModTime().UnixNano())
	b.mu.Lock()
	ok := b.verified[dir] == stamp
	b.mu.Unlock()
	if ok {
		return nil
	}
	if _, err := verifyBuild(dir); err != nil {
		return err
	}
	b.mu.Lock()
	if b.verified == nil {
		b.verified = map[string]string{}
	}
	b.verified[dir] = stamp
	b.mu.Unlock()
	return nil
}

func readManifest(dir string) (EngineManifest, error) {
	var m EngineManifest
	data, err := os.ReadFile(filepath.Join(dir, "engine.json"))
	if err != nil {
		return m, errors.New("engine.json is missing: this is not a NEXUS engine build")
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("engine.json is damaged: %w", err)
	}
	return m, nil
}

// verifyBuild checks the manifest, the API version and every file's SHA-256.
func verifyBuild(dir string) (EngineManifest, error) {
	m, err := readManifest(dir)
	if err != nil {
		return m, err
	}
	if !safeVersion.MatchString(m.Version) {
		return m, fmt.Errorf("engine.json has an invalid version %q", m.Version)
	}
	if m.API != engineAPIVersion {
		return m, fmt.Errorf("this engine build uses interface version %d; this NEXUS needs version %d", m.API, engineAPIVersion)
	}
	if _, ok := m.Files["nexus_engine.dll"]; !ok {
		return m, errors.New("the build does not contain nexus_engine.dll")
	}
	for name, want := range m.Files {
		if strings.ContainsAny(name, `\:`) || strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			return m, fmt.Errorf("engine.json lists an invalid file name %q", name)
		}
		p := filepath.Join(dir, filepath.FromSlash(name))
		got, err := sha256File(p)
		if err != nil {
			return m, fmt.Errorf("%s is missing from the build", name)
		}
		if !strings.EqualFold(got, want) {
			return m, fmt.Errorf("%s is damaged (checksum mismatch)", name)
		}
		if strings.HasSuffix(strings.ToLower(name), ".dll") {
			if err := checkPE(p); err != nil {
				return m, fmt.Errorf("%s: %v", name, err)
			}
		}
	}
	return m, nil
}

// List returns installed builds, newest first.
func (b *EngineBuilds) List() []EngineBuild {
	active := b.activeVersion()
	entries, _ := os.ReadDir(b.BuildsDir())
	var out []EngineBuild
	if b.BundledPresent() {
		eb := EngineBuild{Version: bundledVersion, Dir: b.Root(), Active: active == bundledVersion, Bundled: true}
		m, err := readManifest(b.Root())
		eb.Manifest = m
		if err != nil {
			eb.Problem = err.Error()
		}
		out = append(out, eb)
	}
	for _, en := range entries {
		if !en.IsDir() || strings.HasPrefix(en.Name(), ".") {
			continue
		}
		dir := filepath.Join(b.BuildsDir(), en.Name())
		eb := EngineBuild{Version: en.Name(), Dir: dir, Active: en.Name() == active}
		if info, err := en.Info(); err == nil {
			eb.Installed = info.ModTime().Unix()
		}
		m, err := readManifest(dir)
		eb.Manifest = m
		if err != nil {
			eb.Problem = err.Error()
		}
		out = append(out, eb)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Bundled != out[j].Bundled {
			return out[i].Bundled
		}
		return CompareVersions(out[i].Version, out[j].Version) > 0
	})
	return out
}

func (b *EngineBuilds) Job() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]any{"active": b.job.Active, "message": b.job.Message, "error": b.job.Error, "progress": b.job.Progress}
}

func (b *EngineBuilds) setJob(active bool, prog float64, msg, errMsg string) {
	b.mu.Lock()
	b.job.Active, b.job.Progress, b.job.Message, b.job.Error = active, prog, msg, errMsg
	b.mu.Unlock()
	b.e.app.Events.Emit("engine.job", b.Job())
}

// InstallZip installs a packaged engine build (nexus-engine-<ver>-win64.zip). The build is
// verified before it is moved into place; an existing build with the same version is kept.
// When activate is true it becomes the active build (used from the next NEXUS start if an
// engine is already loaded in this session).
func (b *EngineBuilds) InstallZip(zipPath string, activate bool) (EngineBuild, error) {
	if b.e.State().Running {
		return EngineBuild{}, errors.New("Close the running game before installing an engine build.")
	}
	b.setJob(true, 0.1, "Extracting engine build…", "")
	if err := os.MkdirAll(b.BuildsDir(), 0o755); err != nil {
		b.setJob(false, 0, "", err.Error())
		return EngineBuild{}, err
	}
	tmp := filepath.Join(b.BuildsDir(), fmt.Sprintf(".incoming-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmp)
	if err := extractZipTo(zipPath, tmp); err != nil {
		b.setJob(false, 0, "", err.Error())
		return EngineBuild{}, err
	}
	b.setJob(true, 0.6, "Verifying engine build…", "")
	m, err := verifyBuild(tmp)
	if err != nil {
		b.setJob(false, 0, "", err.Error())
		return EngineBuild{}, err
	}
	if m.Version == bundledVersion {
		err := errors.New(`"bundled" is reserved for the engine inside the NEXUS folder`)
		b.setJob(false, 0, "", err.Error())
		return EngineBuild{}, err
	}
	dst := filepath.Join(b.BuildsDir(), m.Version)
	if dirExists(dst) {
		if _, verr := verifyBuild(dst); verr == nil {
			// identical version already installed and intact: keep it
			_ = os.RemoveAll(tmp)
		} else {
			bad := dst + ".damaged-" + time.Now().Format("20060102-150405")
			if err := os.Rename(dst, bad); err != nil {
				b.setJob(false, 0, "", err.Error())
				return EngineBuild{}, err
			}
			if err := os.Rename(tmp, dst); err != nil {
				b.setJob(false, 0, "", err.Error())
				return EngineBuild{}, err
			}
		}
	} else if err := os.Rename(tmp, dst); err != nil {
		b.setJob(false, 0, "", err.Error())
		return EngineBuild{}, err
	}
	if activate {
		if err := b.Activate(m.Version); err != nil {
			b.setJob(false, 0, "", err.Error())
			return EngineBuild{}, err
		}
	}
	b.e.app.Log.Printf("engine build %s installed (commit %s, base %s)", m.Version, m.Commit, m.UpstreamBase)
	b.setJob(false, 1, "Engine "+m.Version+" installed.", "")
	return EngineBuild{Version: m.Version, Dir: dst, Active: activate, Manifest: m}, nil
}

// InstallURL downloads a build zip and installs it. The optional sha256 is checked first.
func (b *EngineBuilds) InstallURL(url, wantSHA string, activate bool) (EngineBuild, error) {
	if !strings.HasPrefix(url, "https://") {
		return EngineBuild{}, errors.New("only https:// download links are accepted")
	}
	tmp, err := os.CreateTemp("", "nexus-engine-*.zip")
	if err != nil {
		return EngineBuild{}, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	b.setJob(true, 0, "Downloading engine build…", "")
	sum, err := httpDownload(url, tmp.Name(), nil)
	if err != nil {
		b.setJob(false, 0, "", err.Error())
		return EngineBuild{}, err
	}
	if wantSHA != "" && !strings.EqualFold(strings.Fields(wantSHA)[0], sum) {
		err := errors.New("the download does not match its published checksum; nothing was installed")
		b.setJob(false, 0, "", err.Error())
		return EngineBuild{}, err
	}
	return b.InstallZip(tmp.Name(), activate)
}

// Activate makes an installed, verified build the active one.
func (b *EngineBuilds) Activate(version string) error {
	if version == bundledVersion {
		if !b.BundledPresent() {
			return errors.New("This NEXUS folder has no bundled engine. Extract the complete NEXUS ZIP again.")
		}
		if err := os.Remove(b.currentFile()); err != nil && !os.IsNotExist(err) {
			return err
		}
		b.e.app.Events.Emit("engine.changed", nil)
		return nil
	}
	if !safeVersion.MatchString(version) {
		return errors.New("invalid version")
	}
	dir := filepath.Join(b.BuildsDir(), version)
	if _, err := verifyBuild(dir); err != nil {
		return fmt.Errorf("engine %s cannot be used: %w", version, err)
	}
	if err := os.MkdirAll(b.BuildsDir(), 0o755); err != nil {
		return err
	}
	tmp := b.currentFile() + ".tmp"
	if err := os.WriteFile(tmp, []byte(version+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, b.currentFile()); err != nil {
		return err
	}
	b.e.app.Events.Emit("engine.changed", nil)
	return nil
}

// Rollback goes back from an added build: to the newest older added build, otherwise to the
// engine bundled with NEXUS.
func (b *EngineBuilds) Rollback() (string, error) {
	cur := b.activeVersion()
	if cur == bundledVersion || cur == "" {
		return "", errors.New("NEXUS is already using the engine bundled with it.")
	}
	for _, eb := range b.List() {
		if !eb.Bundled && eb.Version != cur && eb.Problem == "" && CompareVersions(eb.Version, cur) < 0 {
			return eb.Version, b.Activate(eb.Version)
		}
	}
	if b.BundledPresent() {
		return bundledVersion, b.Activate(bundledVersion)
	}
	return "", errors.New("There is no earlier engine build to go back to.")
}

// Remove deletes an inactive build (moved to the archive, not erased).
func (b *EngineBuilds) Remove(version string) error {
	if !safeVersion.MatchString(version) {
		return errors.New("invalid version")
	}
	if version == bundledVersion {
		return errors.New("The engine bundled with NEXUS is part of the NEXUS folder and cannot be removed here.")
	}
	if version == b.activeVersion() {
		return errors.New("The active engine build cannot be removed. Switch to another build first.")
	}
	dir := filepath.Join(b.BuildsDir(), version)
	if !dirExists(dir) {
		return errors.New("no such build")
	}
	dst := filepath.Join(b.e.app.Paths.Trash, "engine-builds", version+"-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(dir, dst); err != nil { // different volume: copy then delete
		if _, cerr := copyTree(dir, dst, false); cerr != nil {
			return cerr
		}
		return os.RemoveAll(dir)
	}
	return nil
}
