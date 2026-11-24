package main

// Automatic NEXUS updates from the official release page (GitHub Releases of the NEXUS
// repository).
//
//	Check     GitHub's API for the latest release: version, notes, download size. A private
//	          repository needs a read-only access token, which the user enters in Settings and
//	          NEXUS stores encrypted for their Windows account (never in the program).
//	Download  The release ZIP and its published .sha256 into <data>\System\UpdateStaging (never a
//	          temporary folder; an interrupted download resumes). The ZIP is used only if its
//	          SHA-256 matches, it has the NEXUS layout (NEXUS\NEXUS.exe, NEXUS\Engine\
//	          nexus_engine.dll) and its version is the one announced. Nothing downloaded is run
//	          before this check.
//	Install   NEXUS starts the verified new NEXUS.exe from the staging folder in update mode and
//	          closes. That process waits for NEXUS to exit, moves the old program files aside
//	          (same drive: a rename) into <install>\.nexus-previous, moves the new ones in and
//	          starts the new NEXUS. The new NEXUS reports that it started and loaded its engine;
//	          if it does not within 90 seconds, the old files are moved back and the old NEXUS
//	          starts again.
//
// Only program files are replaced: NEXUS.exe, Engine, Resources, Licences, Source and the
// documents in the package. User data (profiles, saves, settings, captures, mods, game updates,
// controller settings) is in the data folder and is never touched; in a portable installation the
// folders System, UserData, Media and Games and portable.txt are never replaced (Games only gets
// files it does not have).

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const defaultReleaseRepo = "0ryzin/Nexus"

type UpdateInfo struct {
	Current    string `json:"current"`
	Latest     string `json:"latest,omitempty"`
	Available  bool   `json:"available"`
	Name       string `json:"name,omitempty"`
	Notes      string `json:"notes,omitempty"`
	Size       int64  `json:"size,omitempty"`
	Published  string `json:"published,omitempty"`
	Page       string `json:"page,omitempty"`
	NeedsToken bool   `json:"needsToken,omitempty"`
	// Manual: NEXUS cannot install this version itself (Linux: a new AppImage or tar.gz is
	// downloaded from the release page); the interface opens the page instead
	Manual    bool   `json:"manual,omitempty"`
	Error     string `json:"error,omitempty"`
	Checked   int64  `json:"checked"`
	assetURL  string
	sumURL    string
	assetName string
}

type UpdateJob struct {
	State string `json:"state"` // downloading | verifying | ready | installing | error
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error,omitempty"`
}

type Updater struct {
	app    *App
	mu     sync.Mutex
	last   UpdateInfo
	job    *UpdateJob
	client *http.Client
	api    string // https://api.github.com (tests replace it)
}

func NewUpdater(a *App) *Updater {
	return &Updater{app: a, client: &http.Client{Timeout: 30 * time.Minute}, api: "https://api.github.com"}
}

func (u *Updater) repo() string {
	if r := strings.TrimSpace(u.app.Settings.Get().Updates.Repo); r != "" {
		return r
	}
	return defaultReleaseRepo
}

func (u *Updater) stagingDir() string { return filepath.Join(u.app.Paths.System, "UpdateStaging") }

func (u *Updater) tokenPath() string { return filepath.Join(u.app.Paths.Settings, "github-token.bin") }

// SetToken stores (or with "" removes) the access token, encrypted for this Windows account.
func (u *Updater) SetToken(tok string) error {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		_ = os.Remove(u.tokenPath())
		return nil
	}
	enc, err := protectSecret([]byte(tok))
	if err != nil {
		return err
	}
	return writeFileAtomic(u.tokenPath(), enc)
}

func (u *Updater) token() string {
	b, err := os.ReadFile(u.tokenPath())
	if err != nil {
		return ""
	}
	t, err := unprotectSecret(b)
	if err != nil {
		return ""
	}
	return string(t)
}

func (u *Updater) HasToken() bool { return fileExists(u.tokenPath()) }

func (u *Updater) request(url, accept string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if t := u.token(); t != "" && strings.HasPrefix(url, u.api) {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	return u.client.Do(req)
}

// Check asks the release page for the latest version.
func (u *Updater) Check() UpdateInfo {
	info := UpdateInfo{Current: AppVersion, Checked: time.Now().Unix()}
	defer func() {
		u.mu.Lock()
		u.last = info
		u.mu.Unlock()
		_, _ = u.app.Settings.Update(func(s *GlobalSettings) { s.Updates.LastCheck = info.Checked })
	}()
	resp, err := u.request(fmt.Sprintf("%s/repos/%s/releases/latest", u.api, u.repo()), "application/vnd.github+json")
	if err != nil {
		info.Error = "The release page could not be reached. Check your internet connection."
		return info
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 404 || resp.StatusCode == 401 || resp.StatusCode == 403:
		if u.HasToken() {
			info.Error = "The access token was not accepted for " + u.repo() + ". Check it in Settings › Updates."
		} else {
			info.Error = "The NEXUS release page (" + u.repo() + ") is private, so NEXUS needs a read-only access token to check for updates. Add one in Settings › Updates."
		}
		info.NeedsToken = true
		return info
	case resp.StatusCode != 200:
		info.Error = "The release page answered " + resp.Status + "."
		return info
	}
	var rel struct {
		Tag       string `json:"tag_name"`
		Name      string `json:"name"`
		Body      string `json:"body"`
		Draft     bool   `json:"draft"`
		Pre       bool   `json:"prerelease"`
		Published string `json:"published_at"`
		HTMLURL   string `json:"html_url"`
		Assets    []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil || rel.Tag == "" {
		info.Error = "The release page sent an unexpected answer."
		return info
	}
	ver := strings.TrimPrefix(rel.Tag, "v")
	info.Latest, info.Name, info.Notes, info.Published, info.Page = ver, rel.Name, rel.Body, rel.Published, rel.HTMLURL
	// release pages start with the download table and install steps; the update dialog shows
	// only the notes after the marker (release_body.py)
	if i := strings.Index(info.Notes, "<!-- nexus:notes -->"); i >= 0 {
		info.Notes = strings.TrimSpace(info.Notes[i+len("<!-- nexus:notes -->"):])
	}
	if updatesManual { // Linux: announced, downloaded from the release page
		info.Available = CompareVersions(ver, AppVersion) > 0 && !rel.Draft && !rel.Pre
		info.Manual = true
		return info
	}
	want := "NEXUS-" + ver + "-Windows-x64.zip"
	for _, a := range rel.Assets {
		switch a.Name {
		case want:
			info.assetURL, info.Size, info.assetName = a.URL, a.Size, a.Name
		case want + ".sha256":
			info.sumURL = a.URL
		}
	}
	newer := CompareVersions(ver, AppVersion) > 0 && !rel.Draft && !rel.Pre
	switch {
	case !newer:
	case info.assetURL == "" || info.sumURL == "":
		info.Error = "Version " + ver + " is out, but its Windows download or checksum is missing from the release."
	default:
		info.Available = true
	}
	return info
}

func (u *Updater) Last() UpdateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.last
}

func (u *Updater) Job() *UpdateJob {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.job == nil {
		return nil
	}
	j := *u.job
	return &j
}

func (u *Updater) setJob(fn func(j *UpdateJob)) {
	u.mu.Lock()
	if u.job == nil {
		u.job = &UpdateJob{}
	}
	fn(u.job)
	j := *u.job
	u.mu.Unlock()
	u.app.Events.Emit("update.job", j)
}

// CheckInBackground: once at start-up when automatic checks are on (at most every 6 hours).
func (u *Updater) CheckInBackground() {
	s := u.app.Settings.Get().Updates
	if !s.AutoCheck || time.Since(time.Unix(s.LastCheck, 0)) < 6*time.Hour {
		return
	}
	go func() {
		time.Sleep(5 * time.Second) // after the start-up animation
		info := u.Check()
		if info.Available && info.Latest != u.app.Settings.Get().Updates.SkipVersion {
			u.app.Events.Emit("update.available", info)
		}
	}()
}

// Download fetches and verifies the release (in the background).
func (u *Updater) Download() error {
	info := u.Last()
	if !info.Available {
		return errors.New("No update is available. Check for updates first.")
	}
	u.mu.Lock()
	if u.job != nil && (u.job.State == "downloading" || u.job.State == "verifying" || u.job.State == "installing") {
		u.mu.Unlock()
		return errors.New("The update is already being downloaded.")
	}
	u.job = &UpdateJob{State: "downloading", Total: info.Size}
	u.mu.Unlock()
	go func() {
		if err := u.download(info); err != nil {
			u.app.Log.Printf("update: %v", err)
			u.setJob(func(j *UpdateJob) { j.State, j.Error = "error", err.Error() })
			return
		}
		u.setJob(func(j *UpdateJob) { j.State = "ready" })
	}()
	return nil
}

func (u *Updater) download(info UpdateInfo) error {
	dir := filepath.Join(u.stagingDir(), info.Latest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// the published checksum
	resp, err := u.request(info.sumURL, "application/octet-stream")
	if err != nil {
		return errors.New("the checksum could not be downloaded")
	}
	sumText, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("the checksum could not be downloaded (%s)", resp.Status)
	}
	fields := strings.Fields(string(sumText))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return errors.New("the published checksum is not a SHA-256 value")
	}
	want := strings.ToLower(fields[0])

	zipPath := filepath.Join(dir, info.assetName)
	if ok, _ := fileSHA256Is(zipPath, want); !ok {
		part := zipPath + ".part"
		if err := u.fetchResumable(info.assetURL, part, info.Size); err != nil {
			return err
		}
		u.setJob(func(j *UpdateJob) { j.State = "verifying" })
		ok, err := fileSHA256Is(part, want)
		if err != nil {
			return err
		}
		if !ok {
			_ = os.Remove(part)
			return errors.New("the download does not match the published checksum, so it was discarded")
		}
		if err := os.Rename(part, zipPath); err != nil {
			return err
		}
	}
	u.setJob(func(j *UpdateJob) { j.State = "verifying" })
	ext := filepath.Join(dir, "extracted")
	_ = os.RemoveAll(ext)
	if err := extractRelease(zipPath, ext, info.Latest); err != nil {
		_ = os.RemoveAll(ext)
		return err
	}
	return nil
}

// fetchResumable downloads url into part, continuing a partial file when the server allows it.
func (u *Updater) fetchResumable(url, part string, size int64) error {
	var have int64
	if fi, err := os.Stat(part); err == nil && (size == 0 || fi.Size() < size) {
		have = fi.Size()
	} else {
		_ = os.Remove(part)
	}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	if t := u.token(); t != "" && strings.HasPrefix(url, u.api) {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return errors.New("the download could not start. Check your internet connection")
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case 206:
		flags |= os.O_APPEND
	case 200:
		have = 0
		flags |= os.O_TRUNC
	default:
		return fmt.Errorf("the download failed (%s)", resp.Status)
	}
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}
	u.setJob(func(j *UpdateJob) { j.Done = have })
	buf := make([]byte, 256<<10)
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return err
			}
			have += int64(n)
			if time.Since(last) > 250*time.Millisecond {
				last = time.Now()
				done := have
				u.setJob(func(j *UpdateJob) { j.Done = done })
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return errors.New("the download was interrupted; it continues where it stopped when you try again")
		}
	}
	done := have
	u.setJob(func(j *UpdateJob) { j.Done = done })
	return f.Close()
}

func fileSHA256Is(path, want string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, nil
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == want, nil
}

// extractRelease unpacks a verified release ZIP and checks its contents.
func extractRelease(zipPath, dst, version string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return errors.New("the update package cannot be opened")
	}
	defer zr.Close()
	have := map[string]bool{}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if !strings.HasPrefix(name, "NEXUS/") || strings.Contains(name, "..") || strings.Contains(name, ":") {
			return fmt.Errorf("the update package contains an unexpected path (%s)", f.Name)
		}
		have[name] = true
		target := filepath.Join(dst, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extractZipFile(f, target); err != nil {
			return fmt.Errorf("the update package could not be unpacked: %w", err)
		}
	}
	for _, need := range []string{"NEXUS/NEXUS.exe", "NEXUS/Engine/nexus_engine.dll", "NEXUS/Engine/engine.json", "NEXUS/RELEASE.json"} {
		if !have[need] {
			return fmt.Errorf("the update package is incomplete (%s is missing)", need)
		}
	}
	var rel struct {
		Version string `json:"version"`
	}
	_ = readJSON(filepath.Join(dst, "NEXUS", "RELEASE.json"), &rel)
	if rel.Version != "" && rel.Version != version {
		return fmt.Errorf("the update package is version %s, not %s", rel.Version, version)
	}
	exe, _ := os.ReadFile(filepath.Join(dst, "NEXUS", "NEXUS.exe"))
	if len(exe) < 2 || string(exe[:2]) != "MZ" {
		return errors.New("the update package's NEXUS.exe is not a Windows program")
	}
	return nil
}

// ---------- install

// never replaced in the NEXUS folder: user data of portable installations and the user's games
var updateKeep = map[string]bool{"system": true, "userdata": true, "media": true, "portable.txt": true, "streamline": true, ".nexus-previous": true}

// Install starts the verified new NEXUS in update mode; NEXUS then closes.
func (u *Updater) Install() error {
	info := u.Last()
	if st := u.app.Emu.State(); st.Running {
		return errors.New("Close the game first.")
	}
	ext := filepath.Join(u.stagingDir(), info.Latest, "extracted", "NEXUS")
	exe := filepath.Join(ext, "NEXUS.exe")
	if info.Latest == "" || !fileExists(exe) {
		return errors.New("Download the update first.")
	}
	// the NEXUS folder must be writable (not e.g. Program Files)
	probe := filepath.Join(u.app.Paths.Root, ".nexus-write-test")
	if err := os.WriteFile(probe, []byte("x"), 0o644); err != nil {
		return fmt.Errorf("NEXUS cannot change its own folder (%s). Move NEXUS to a folder you can write to, or update it by hand from the release page.", u.app.Paths.Root)
	}
	_ = os.Remove(probe)
	cmd := exec.Command(exe, "--apply-update", ext, u.app.Paths.Root, fmt.Sprint(os.Getpid()), AppVersion)
	cmd.Dir = ext
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("the update could not be started: %w", err)
	}
	u.setJob(func(j *UpdateJob) { j.State = "installing" })
	u.app.Log.Printf("update: installing %s; NEXUS closes now", info.Latest)
	go func() {
		time.Sleep(600 * time.Millisecond)
		u.app.UI.Quit()
	}()
	return nil
}

func updateLog(root, format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(root, ".nexus-previous", "update.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().Format(time.RFC3339)}, args...)...)
}

// applyUpdate runs in the new NEXUS.exe (from the staging folder): args are the new files'
// folder, the NEXUS folder, the old NEXUS's process ID and the old version.
func applyUpdate(args []string) int {
	if len(args) < 4 {
		return 2
	}
	src, root, oldVer := args[0], args[1], args[3]
	var pid int
	fmt.Sscan(args[2], &pid)
	waitForExit(pid, 60*time.Second)
	prev := filepath.Join(root, ".nexus-previous")
	backup := filepath.Join(prev, oldVer)
	_ = os.RemoveAll(backup)
	if err := os.MkdirAll(backup, 0o755); err != nil {
		return 3
	}
	updateLog(root, "updating %s from %s (old version %s)", root, src, oldVer)
	entries, err := os.ReadDir(src)
	if err != nil {
		updateLog(root, "cannot read %s: %v", src, err)
		return 3
	}
	var moved, placed []string
	rollback := func(why string) int {
		updateLog(root, "rolling back: %s", why)
		for _, n := range placed {
			_ = os.RemoveAll(filepath.Join(root, n))
		}
		for _, n := range moved {
			_ = os.Rename(filepath.Join(backup, n), filepath.Join(root, n))
		}
		_ = exec.Command(filepath.Join(root, "NEXUS.exe"), "--update-failed", why).Start()
		return 1
	}
	for _, e := range entries {
		n := e.Name()
		if updateKeep[strings.ToLower(n)] {
			continue
		}
		if strings.EqualFold(n, "Games") || strings.EqualFold(n, "Customisations") { // the user's own folders: only add what they lack
			_ = filepath.Walk(filepath.Join(src, n), func(p string, info os.FileInfo, err error) error {
				if err == nil && info.Mode().IsRegular() {
					rel, _ := filepath.Rel(src, p)
					if t := filepath.Join(root, rel); !fileExists(t) {
						_ = copyFileTmpRename(p, t)
					}
				}
				return nil
			})
			continue
		}
		cur := filepath.Join(root, n)
		if fileExists(cur) || dirExists(cur) {
			var err error
			for i := 0; i < 20; i++ { // files may stay locked for a moment after NEXUS exits
				if err = os.Rename(cur, filepath.Join(backup, n)); err == nil {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}
			if err != nil {
				return rollback(fmt.Sprintf("%s could not be moved aside: %v", n, err))
			}
			moved = append(moved, n)
		}
		if err := copyTreeOrFile(filepath.Join(src, n), cur); err != nil {
			placed = append(placed, n)
			return rollback(fmt.Sprintf("%s could not be installed: %v", n, err))
		}
		placed = append(placed, n)
	}
	// start the new NEXUS and wait for it to report a good start (engine loaded)
	okFile := filepath.Join(prev, "started-ok")
	_ = os.Remove(okFile)
	cmd := exec.Command(filepath.Join(root, "NEXUS.exe"), "--update-applied", okFile)
	cmd.Dir = root
	if err := cmd.Start(); err != nil {
		return rollback("the new NEXUS could not be started: " + err.Error())
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.After(90 * time.Second)
	for {
		select {
		case err := <-exited:
			if fileExists(okFile) {
				updateLog(root, "update to the new version complete (it has since closed)")
				return 0
			}
			return rollback(fmt.Sprintf("the new NEXUS closed before it finished starting (%v)", err))
		case <-deadline:
			if fileExists(okFile) {
				updateLog(root, "update complete")
				return 0
			}
			_ = cmd.Process.Kill()
			time.Sleep(time.Second)
			return rollback("the new NEXUS did not finish starting within 90 seconds")
		case <-time.After(500 * time.Millisecond):
			if fileExists(okFile) {
				updateLog(root, "update complete")
				return 0
			}
		}
	}
}

func copyTreeOrFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return copyFileTmpRename(src, dst)
	}
	return backupTree(src, dst)
}

// confirmUpdateStart: the new NEXUS, started by applyUpdate, reports a good start once its engine
// has loaded; it also keeps only the newest previous version as a fallback.
func (a *App) confirmUpdateStart(okFile string) {
	go func() {
		ok := false
		for i := 0; i < 60 && !ok; i++ {
			if h, err := a.Emu.Host(); err == nil {
				if _, err := h.Info(); err == nil {
					ok = true
					break
				}
			}
			time.Sleep(time.Second)
		}
		if !ok {
			a.Log.Printf("update: the engine did not load; the update will be rolled back")
			return
		}
		_ = os.WriteFile(okFile, []byte(AppVersion), 0o644)
		a.Log.Printf("update: NEXUS %s started after an update", AppVersion)
		a.Events.Emit("update.done", map[string]string{"version": AppVersion})
		_ = os.RemoveAll(filepath.Join(a.Paths.System, "UpdateStaging"))
	}()
}

// preferBundledEngine: after an update the engine that came with it is used, unless the user's
// own chosen engine build is newer (their build stays installed either way).
func (a *App) preferBundledEngine() {
	b := a.Emu.Builds
	sel := b.selection()
	if sel == "" || !b.BundledPresent() {
		return
	}
	m, err := readManifest(b.Root())
	if err != nil {
		return
	}
	if CompareVersions(m.Version, sel) >= 0 {
		if err := b.Activate(bundledVersion); err == nil {
			a.Log.Printf("update: using the engine %s that came with this NEXUS (was %s)", m.Version, sel)
		}
	}
}

// updatesManual: NEXUS announces new versions but does not install them (everything except
// Windows; tests of the Windows install path turn it off).
var updatesManual = runtime.GOOS != "windows"
