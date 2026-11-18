package main

// Diagnostics between the user's own devices (Settings › Diagnostics › Send this device's logs to
// my other devices). A Steam Deck in Gaming Mode has no easy way to copy a file off it, and its
// engine log is what shows why a game renders black or is silent. The user presses one button on
// the Deck; at its next synchronisation (start-up, or every 15 minutes) every other NEXUS device
// on the same account puts the logs into its own Logs folder ("From other devices").
//
//   - What is sent: the last part of NEXUS's own log and of the engine's log (and the newest crash
//     report), each line sanitised as in crash reports (crashreport.go: lines that mention
//     accounts, tokens, keys or passwords are left out; the home folder and e-mail addresses are
//     replaced), plus about.txt (NEXUS and engine versions, platform, Steam Deck / Gaming Mode,
//     graphics and processor names). Never key, firmware, save or game files.
//   - Where: like game saves (savesync.go), a ZIP in a secret Gist of the user's own GitHub
//     account, one per sending device, and an entry in nexus-diagnostics.json in the NEXUS sync
//     Gist. Nothing goes anywhere else.
//   - The receiving device checks the ZIP's SHA-256 and extracts it safely (unzipSafe).

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	diagIndexFile   = "nexus-diagnostics.json"
	diagMaxLogBytes = 6 << 20 // the last part of each log that is sent
	diagMaxZip      = 20 << 20
	diagKeepPerDev  = 5
)

// DiagEntry is one device's last sent diagnostics in nexus-diagnostics.json.
type DiagEntry struct {
	DeviceID string `json:"deviceId"`
	Device   string `json:"device"`
	Platform string `json:"platform"`
	NEXUS    string `json:"nexus"`
	Created  int64  `json:"created"`
	Gist     string `json:"gist"`
	Raw      string `json:"raw"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Note     string `json:"note,omitempty"`
}

// DiagReceived is a bundle this device received.
type DiagReceived struct {
	Device  string `json:"device"`
	Created int64  `json:"created"`
	Folder  string `json:"folder"`
}

var diagMu sync.Mutex

func (a *App) diagFetchedPath() string {
	return filepath.Join(a.Paths.System, "diagnostics-received.json")
}

// diagAccount: the GitHub account is signed in and synchronising.
func (a *App) diagAccount() (saveRemote, string, string, error) {
	ac := a.Account
	if ac == nil {
		return saveRemote{}, "", "", errors.New("no account")
	}
	ac.mu.Lock()
	ok := ac.st.SignedIn && ac.st.Provider == "github" && ac.st.SyncOn
	devID, devName := ac.st.DeviceID, ac.st.DeviceName
	ac.mu.Unlock()
	if !ok {
		return saveRemote{}, "", "", errors.New("Sign in to your NEXUS account (with synchronisation on) on this device and on the device that should receive the logs.")
	}
	return saveRemote{ac: ac}, devID, devName, nil
}

func (r saveRemote) diagIndex() (map[string]DiagEntry, error) {
	r.ac.mu.Lock()
	id := r.ac.st.GistID
	r.ac.mu.Unlock()
	if id == "" {
		return nil, errors.New("the NEXUS sync data on GitHub is not set up yet")
	}
	var g struct {
		Files map[string]gistFile `json:"files"`
	}
	if _, err := r.ac.gh("GET", githubAPI+"/gists/"+url.PathEscape(id), nil, &g); err != nil {
		return nil, err
	}
	out := map[string]DiagEntry{}
	f, ok := g.Files[diagIndexFile]
	if !ok {
		return out, nil
	}
	content := f.Content
	if f.Truncated && f.RawURL != "" {
		b, err := r.download(f.RawURL, 4<<20)
		if err != nil {
			return nil, err
		}
		content = string(b)
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("the diagnostics list on GitHub is damaged: %v", err)
	}
	return out, nil
}

// sanitisedTail: the last max bytes of a log file, each line sanitised (sanitizeLine).
func sanitisedTail(path string, max int) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	cut := false
	if len(data) > max {
		data, cut = data[len(data)-max:], true
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	home, _ := os.UserHomeDir()
	var b bytes.Buffer
	if cut {
		b.WriteString("[NEXUS: only the last part of this log was sent]\n")
	}
	dropped := 0
	for _, l := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		s, ok := sanitizeLine(l, home)
		if !ok {
			dropped++
			continue
		}
		b.WriteString(s)
		b.WriteByte('\n')
	}
	if dropped > 0 {
		fmt.Fprintf(&b, "[NEXUS: %d line(s) that could carry account or key information were left out]\n", dropped)
	}
	return b.Bytes(), true
}

// collectDiagnostics builds the ZIP that is sent.
func (a *App) collectDiagnostics(note string) ([]byte, []string, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	var names []string
	add := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		names = append(names, name)
		return err
	}
	var about strings.Builder
	pi := a.PlatformInfo()
	fmt.Fprintf(&about, "NEXUS diagnostics\nSent: %s\nNEXUS: %s\n", time.Now().Format(time.RFC1123), AppVersion)
	fmt.Fprintf(&about, "System: %s/%s, Steam Deck: %v, Gaming Mode: %v, Steam running: %v\n", runtime.GOOS, runtime.GOARCH, pi["steamDeck"], pi["gamingMode"], pi["steam"])
	h := a.DetectHardware(false)
	fmt.Fprintf(&about, "Processor: %s, %d threads\nMemory: %d MB\nOS: %+v\n", h.CPU.Model, h.CPU.Threads, h.Memory.TotalMB, h.OS)
	for _, g := range h.GPUs {
		fmt.Fprintf(&about, "Graphics: %s (%04x:%04x) driver %s, Vulkan %s\n", g.Model, g.VendorID, g.DeviceID, g.Driver, g.VulkanAPI)
	}
	if note != "" {
		fmt.Fprintf(&about, "\nNote from the user: %s\n", note)
	}
	if err := add("about.txt", []byte(about.String())); err != nil {
		return nil, nil, err
	}
	if d, ok := sanitisedTail(filepath.Join(a.Paths.Logs, "nexus.log"), diagMaxLogBytes); ok {
		if err := add("nexus.log", d); err != nil {
			return nil, nil, err
		}
	}
	for _, f := range []string{"eden_log.txt", "eden_log.txt.old.txt", "eden_log.txt.old"} {
		if d, ok := sanitisedTail(filepath.Join(a.Emu.DataDir(), "log", f), diagMaxLogBytes); ok {
			if err := add("engine-"+f, d); err != nil {
				return nil, nil, err
			}
		}
	}
	if crashes, _ := filepath.Glob(filepath.Join(a.Paths.Logs, "crashes", "crash-*.txt")); len(crashes) > 0 {
		sort.Strings(crashes)
		if d, ok := sanitisedTail(crashes[len(crashes)-1], 1<<20); ok {
			_ = add("crash-report.txt", d)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	if buf.Len() > diagMaxZip {
		return nil, nil, fmt.Errorf("the logs are too large to send (%d MB)", buf.Len()>>20)
	}
	return buf.Bytes(), names, nil
}

// SendDiagnostics sends this device's logs to the user's other devices.
func (a *App) SendDiagnostics(note string) (DiagEntry, error) {
	diagMu.Lock()
	defer diagMu.Unlock()
	r, devID, devName, err := a.diagAccount()
	if err != nil {
		return DiagEntry{}, err
	}
	if len(note) > 500 {
		note = note[:500]
	}
	data, names, err := a.collectDiagnostics(note)
	if err != nil {
		return DiagEntry{}, err
	}
	idx, err := r.diagIndex()
	if err != nil {
		return DiagEntry{}, err
	}
	gist, raw, err := r.putBlob(idx[devID].Gist, "NEXUS diagnostics: "+devName, data)
	if err != nil {
		return DiagEntry{}, err
	}
	sum := sha256.Sum256(data)
	e := DiagEntry{DeviceID: devID, Device: devName, Platform: runtime.GOOS, NEXUS: AppVersion, Created: nowUnix(),
		Gist: gist, Raw: raw, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Note: note}
	// read again just before writing: another device may have sent meanwhile
	if cur, err := r.diagIndex(); err == nil {
		idx = cur
	}
	idx[devID] = e
	b, _ := json.MarshalIndent(idx, "", " ")
	r.ac.mu.Lock()
	id := r.ac.st.GistID
	r.ac.mu.Unlock()
	if _, err := r.ac.gh("PATCH", githubAPI+"/gists/"+url.PathEscape(id), map[string]any{"files": map[string]any{diagIndexFile: map[string]string{"content": string(b)}}}, nil); err != nil {
		return DiagEntry{}, err
	}
	a.Log.Printf("diagnostics: sent %d file(s), %d KB, to the other devices of this account (%s)", len(names), len(data)>>10, strings.Join(names, ", "))
	return e, nil
}

var reDiagName = regexp.MustCompile(`[^A-Za-z0-9 ._-]+`)

// FetchDiagnostics receives the logs other devices sent since last time. Returns what arrived.
func (a *App) FetchDiagnostics() ([]DiagReceived, error) {
	diagMu.Lock()
	defer diagMu.Unlock()
	r, devID, _, err := a.diagAccount()
	if err != nil {
		return nil, nil
	}
	idx, err := r.diagIndex()
	if err != nil {
		return nil, err
	}
	seen := map[string]int64{}
	_ = readJSON(a.diagFetchedPath(), &seen)
	var got []DiagReceived
	for id, e := range idx {
		if id == devID || e.Created <= seen[id] || e.Raw == "" || e.Size > diagMaxZip {
			continue
		}
		raw, err := r.download(e.Raw, diagMaxZip*2)
		if err != nil {
			a.Log.Printf("diagnostics: from %s: %v", e.Device, err)
			continue
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			a.Log.Printf("diagnostics: from %s: the received logs are damaged; ignored", e.Device)
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != e.SHA256 {
			a.Log.Printf("diagnostics: from %s: the received logs do not match their checksum; ignored", e.Device)
			continue
		}
		name := strings.TrimSpace(reDiagName.ReplaceAllString(firstNonEmpty(e.Device, "Other device"), "_"))
		base := filepath.Join(a.Paths.Logs, "From other devices")
		dir := filepath.Join(base, fmt.Sprintf("%s %s", name, time.Unix(e.Created, 0).Format("2006-01-02 1504")))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return got, err
		}
		if err := unzipSafe(data, dir, 64<<20); err != nil {
			a.Log.Printf("diagnostics: from %s: %v", e.Device, err)
			continue
		}
		seen[id] = e.Created
		got = append(got, DiagReceived{Device: e.Device, Created: e.Created, Folder: dir})
		a.Log.Printf("diagnostics: received logs from %s (sent %s) into %s", e.Device, time.Unix(e.Created, 0).Format(time.RFC1123), dir)
		// keep the newest few per device
		old, _ := filepath.Glob(filepath.Join(base, name+" ????-??-?? ????"))
		sort.Strings(old)
		for len(old) > diagKeepPerDev {
			_ = os.RemoveAll(old[0])
			old = old[1:]
		}
	}
	if len(got) > 0 {
		_ = writeJSONAtomic(a.diagFetchedPath(), seen)
	}
	return got, nil
}
