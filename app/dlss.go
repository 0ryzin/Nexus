package main

// NVIDIA DLSS files for the NEXUS engine.
//
// The engine uses DLSS through NVIDIA Streamline, loaded at run time from <engine data>\streamline
// (see enhancements.go). NEXUS does not ship NVIDIA's files. When the user asks for them, NEXUS
// downloads them from NVIDIA's own Streamline SDK release on GitHub (NVIDIA-RTX/Streamline, the
// version the engine is built against), the source NVIDIA publishes for developers:
//
//   1. Check: what is already installed (nothing is downloaded twice; the NVIDIA app's own DLSS
//      model counts too).
//   2. Prepare: read the release ZIP's file table with HTTP range requests and show NVIDIA's
//      licence texts and the download size. Nothing is downloaded without the user's consent.
//   3. Install: download only the needed files (about 35 MB of the 276 MB ZIP), check each one's
//      CRC-32 from the ZIP's table and its NVIDIA code signature, then move it into place. A file is
//      written as "<name>.download" first, so an interrupted download leaves no half file under the
//      real name; the next attempt starts that file again.

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	streamlineVersion = "2.14.16"
	streamlineZipURL  = "https://github.com/NVIDIA-RTX/Streamline/releases/download/v2.14.16/streamline-sdk-v2.14.16.zip"
	streamlinePage    = "https://github.com/NVIDIA-RTX/Streamline/releases/tag/v2.14.16"
)

// dlssFiles: what the engine needs from the SDK's bin/x64 folder.
var dlssFiles = []struct{ entry, name, what string }{
	{"bin/x64/sl.interposer.dll", "sl.interposer.dll", "NVIDIA Streamline loader"},
	{"bin/x64/sl.common.dll", "sl.common.dll", "NVIDIA Streamline common plugin"},
	{"bin/x64/sl.dlss.dll", "sl.dlss.dll", "NVIDIA Streamline DLSS plugin"},
	{"bin/x64/nvngx_dlss.dll", "nvngx_dlss.dll", "NVIDIA DLSS"},
}

var dlssLicences = []struct{ entry, title string }{
	{"license.txt", "NVIDIA Streamline"},
	{"bin/x64/nvngx_dlss.license.txt", "NVIDIA DLSS"},
}

// verifyNVIDIASigned checks a downloaded DLL's code signature (Windows: Authenticode, signed by
// NVIDIA). Replaced in tests.
var verifyNVIDIASigned = checkNVIDIASignature

type DLSSStatus struct {
	Supported bool     `json:"supported"`
	GPU       string   `json:"gpu"`
	Why       string   `json:"why,omitempty"`
	Ready     bool     `json:"ready"`
	Folder    string   `json:"folder"`
	Installed []string `json:"installed"`
	Missing   []string `json:"missing"`
	AppModel  string   `json:"appModel,omitempty"` // the NVIDIA app's DLSS, used if nvngx_dlss.dll is missing
	Source    string   `json:"source"`
	Version   string   `json:"version"`
	Job       *DLSSJob `json:"job,omitempty"`
}

type DLSSOffer struct {
	Files    []string `json:"files"`
	Bytes    int64    `json:"bytes"` // download size
	Licences []struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"licences"`
	Source string `json:"source"`
}

type DLSSJob struct {
	Running bool   `json:"running"`
	Done    int64  `json:"done"`
	Total   int64  `json:"total"`
	File    string `json:"file"`
	Error   string `json:"error,omitempty"`
	OK      bool   `json:"ok"`
}

type dlssManager struct {
	mu      sync.Mutex
	job     *DLSSJob
	url     string
	client  *http.Client
	entries map[string]zipEntry
	size    int64
}

var dlssMgr = &dlssManager{url: streamlineZipURL, client: &http.Client{Timeout: 10 * time.Minute}}

// appDLSSModel: the DLSS model the NVIDIA app keeps up to date (the engine uses it when
// nvngx_dlss.dll is not in the folder).
func appDLSSModel() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(pd, "NVIDIA", "NGX", "models", "dlss", "versions", "*", "files", "*.bin"))
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

// dlssMissing: the DLSS files the engine still needs (the NVIDIA app's model counts for
// nvngx_dlss.dll).
func (e *Emulator) dlssMissing() []string {
	var out []string
	app := appDLSSModel()
	for _, f := range dlssFiles {
		if !fileExists(filepath.Join(e.streamlineDir(), f.name)) && !(f.name == "nvngx_dlss.dll" && app != "") {
			out = append(out, f.name)
		}
	}
	return out
}

func (e *Emulator) DLSSStatus() DLSSStatus {
	en := e.Enhancements()
	st := DLSSStatus{Supported: en.DLSS.Supported, GPU: en.GPU, Folder: e.streamlineDir(), Source: streamlinePage, Version: streamlineVersion,
		Installed: []string{}, Missing: []string{}}
	if !st.Supported {
		st.Why = en.DLSS.Why
	}
	st.AppModel = appDLSSModel()
	for _, f := range dlssFiles {
		if fileExists(filepath.Join(st.Folder, f.name)) {
			st.Installed = append(st.Installed, f.name)
		}
	}
	st.Missing = append(st.Missing, e.dlssMissing()...)
	st.Ready = st.Supported && len(st.Missing) == 0
	dlssMgr.mu.Lock()
	if dlssMgr.job != nil {
		j := *dlssMgr.job
		st.Job = &j
	}
	dlssMgr.mu.Unlock()
	return st
}

// ---------- reading single files from a ZIP on a web server

type zipEntry struct {
	method      uint16
	crc         uint32
	compSize    uint64
	size        uint64
	localOffset uint64
}

func (m *dlssManager) rangeGet(from, to int64) ([]byte, int64, error) {
	req, _ := http.NewRequest("GET", m.url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("NVIDIA's download could not be reached: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, 0, fmt.Errorf("NVIDIA's download answered %s", resp.Status)
	}
	total := int64(0)
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			total, _ = strconv.ParseInt(cr[i+1:], 10, 64)
		}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, to-from+1))
	if err != nil {
		return nil, 0, fmt.Errorf("the download was interrupted: %w", err)
	}
	if int64(len(b)) != to-from+1 {
		return nil, 0, errors.New("the download was cut short")
	}
	return b, total, nil
}

// open reads the ZIP's central directory (end record, then the file table).
func (m *dlssManager) open() error {
	if m.entries != nil {
		return nil
	}
	_, total, err := m.rangeGet(0, 0)
	if err != nil {
		return err
	}
	if total < 22 {
		return errors.New("NVIDIA's download has no size")
	}
	tailLen := min(total, 65536+22)
	tail, _, err := m.rangeGet(total-tailLen, total-1)
	if err != nil {
		return err
	}
	i := bytes.LastIndex(tail, []byte("PK\x05\x06"))
	if i < 0 || i+22 > len(tail) {
		return errors.New("NVIDIA's download is not a ZIP archive")
	}
	le := binary.LittleEndian
	cdSize, cdOff := int64(le.Uint32(tail[i+12:])), int64(le.Uint32(tail[i+16:]))
	if cdOff == 0xFFFFFFFF || cdSize == 0xFFFFFFFF { // ZIP64
		j := bytes.LastIndex(tail[:i], []byte("PK\x06\x06"))
		if j < 0 {
			return errors.New("NVIDIA's ZIP archive could not be read")
		}
		cdSize, cdOff = int64(le.Uint64(tail[j+40:])), int64(le.Uint64(tail[j+48:]))
	}
	if cdOff < 0 || cdSize <= 0 || cdOff+cdSize > total || cdSize > 64<<20 {
		return errors.New("NVIDIA's ZIP archive has a damaged file table")
	}
	cd, _, err := m.rangeGet(cdOff, cdOff+cdSize-1)
	if err != nil {
		return err
	}
	entries := map[string]zipEntry{}
	for p := 0; p+46 <= len(cd) && string(cd[p:p+4]) == "PK\x01\x02"; {
		nameLen, extraLen, commentLen := int(le.Uint16(cd[p+28:])), int(le.Uint16(cd[p+30:])), int(le.Uint16(cd[p+32:]))
		if p+46+nameLen+extraLen > len(cd) {
			break
		}
		e := zipEntry{method: le.Uint16(cd[p+10:]), crc: le.Uint32(cd[p+16:]), compSize: uint64(le.Uint32(cd[p+20:])),
			size: uint64(le.Uint32(cd[p+24:])), localOffset: uint64(le.Uint32(cd[p+42:]))}
		name := string(cd[p+46 : p+46+nameLen])
		// ZIP64 sizes/offset in the extra field
		extra := cd[p+46+nameLen : p+46+nameLen+extraLen]
		for q := 0; q+4 <= len(extra); {
			id, n := le.Uint16(extra[q:]), int(le.Uint16(extra[q+2:]))
			if id == 1 && q+4+n <= len(extra) {
				v := extra[q+4 : q+4+n]
				k := 0
				read := func(dst *uint64) {
					if *dst == 0xFFFFFFFF && k+8 <= len(v) {
						*dst = le.Uint64(v[k:])
						k += 8
					}
				}
				read(&e.size)
				read(&e.compSize)
				read(&e.localOffset)
			}
			q += 4 + n
		}
		entries[strings.TrimPrefix(name, "./")] = e
		p += 46 + nameLen + extraLen + commentLen
	}
	if len(entries) == 0 {
		return errors.New("NVIDIA's ZIP archive has an empty file table")
	}
	m.entries, m.size = entries, total
	return nil
}

func (m *dlssManager) find(name string) (zipEntry, bool) {
	if e, ok := m.entries[name]; ok {
		return e, true
	}
	for k, e := range m.entries { // the SDK ZIP may have a top folder
		if strings.HasSuffix(k, "/"+name) {
			return e, true
		}
	}
	return zipEntry{}, false
}

// read downloads one entry and checks its size and CRC-32.
func (m *dlssManager) read(e zipEntry, progress func(int64)) ([]byte, error) {
	if e.compSize > 256<<20 {
		return nil, errors.New("unexpected file size")
	}
	hdr, _, err := m.rangeGet(int64(e.localOffset), int64(e.localOffset)+29)
	if err != nil {
		return nil, err
	}
	if string(hdr[:4]) != "PK\x03\x04" {
		return nil, errors.New("NVIDIA's ZIP archive has a damaged entry")
	}
	start := int64(e.localOffset) + 30 + int64(binary.LittleEndian.Uint16(hdr[26:])) + int64(binary.LittleEndian.Uint16(hdr[28:]))
	var comp []byte
	if e.compSize > 0 {
		// in 4 MB parts, so progress moves and a slow connection does not time out
		for off := int64(0); off < int64(e.compSize); off += 4 << 20 {
			end := min(int64(e.compSize), off+4<<20)
			part, _, err := m.rangeGet(start+off, start+end-1)
			if err != nil {
				return nil, err
			}
			comp = append(comp, part...)
			if progress != nil {
				progress(int64(len(part)))
			}
		}
	}
	var data []byte
	switch e.method {
	case 0:
		data = comp
	case 8:
		data, err = io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(comp)), int64(e.size)+1))
		if err != nil {
			return nil, errors.New("a downloaded file could not be unpacked")
		}
	default:
		return nil, fmt.Errorf("unsupported compression %d", e.method)
	}
	if uint64(len(data)) != e.size || crc32.ChecksumIEEE(data) != e.crc {
		return nil, errors.New("a downloaded file failed its integrity check (CRC-32)")
	}
	return data, nil
}

// PrepareDLSS reads the release's file table and licence texts: what would be downloaded.
func (e *Emulator) PrepareDLSS() (DLSSOffer, error) {
	st := e.DLSSStatus()
	if !st.Supported {
		return DLSSOffer{}, errors.New(st.Why)
	}
	if len(st.Missing) == 0 {
		return DLSSOffer{}, errors.New("NVIDIA DLSS is already installed.")
	}
	m := dlssMgr
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job != nil && m.job.Running {
		return DLSSOffer{}, errors.New("The download is already running.")
	}
	if err := m.open(); err != nil {
		return DLSSOffer{}, err
	}
	offer := DLSSOffer{Source: streamlinePage, Files: st.Missing}
	for _, f := range dlssFiles {
		if contains(st.Missing, f.name) {
			ent, ok := m.find(f.entry)
			if !ok {
				return DLSSOffer{}, fmt.Errorf("%s is not in NVIDIA's release", f.name)
			}
			offer.Bytes += int64(ent.compSize)
		}
	}
	for _, l := range dlssLicences {
		ent, ok := m.find(l.entry)
		if !ok {
			return DLSSOffer{}, fmt.Errorf("NVIDIA's licence text (%s) is missing from the release", l.entry)
		}
		text, err := m.read(ent, nil)
		if err != nil {
			return DLSSOffer{}, err
		}
		offer.Licences = append(offer.Licences, struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		}{l.title, string(text)})
	}
	return offer, nil
}

// InstallDLSS downloads the missing files in the background (after the user accepted NVIDIA's
// licences). Progress: "dlss.job" events.
func (e *Emulator) InstallDLSS(accepted bool) error {
	if !accepted {
		return errors.New("NVIDIA's licences must be accepted first.")
	}
	st := e.DLSSStatus()
	if !st.Supported {
		return errors.New(st.Why)
	}
	m := dlssMgr
	m.mu.Lock()
	if m.job != nil && m.job.Running {
		m.mu.Unlock()
		return errors.New("The download is already running.")
	}
	if m.entries == nil {
		m.mu.Unlock()
		return errors.New("Check what is needed first.")
	}
	job := &DLSSJob{Running: true}
	for _, f := range dlssFiles {
		if contains(st.Missing, f.name) {
			if ent, ok := m.find(f.entry); ok {
				job.Total += int64(ent.compSize)
			}
		}
	}
	m.job = job
	m.mu.Unlock()
	go e.installDLSS(st.Missing, job)
	return nil
}

func (e *Emulator) installDLSS(missing []string, job *DLSSJob) {
	m := dlssMgr
	update := func(fn func(j *DLSSJob)) {
		m.mu.Lock()
		fn(job)
		j := *job
		m.mu.Unlock()
		e.app.Events.Emit("dlss.job", j)
	}
	dir := e.streamlineDir()
	fail := func(err error) {
		e.app.Log.Printf("dlss: %v", err)
		update(func(j *DLSSJob) { j.Running, j.Error = false, err.Error() })
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
		return
	}
	for _, f := range dlssFiles {
		if !contains(missing, f.name) {
			continue
		}
		target := filepath.Join(dir, f.name)
		if fileExists(target) {
			continue // installed meanwhile
		}
		update(func(j *DLSSJob) { j.File = f.name })
		ent, ok := m.find(f.entry)
		if !ok {
			fail(fmt.Errorf("%s is not in NVIDIA's release", f.name))
			return
		}
		data, err := m.read(ent, func(n int64) { update(func(j *DLSSJob) { j.Done += n }) })
		if err != nil {
			fail(fmt.Errorf("%s: %v", f.name, err))
			return
		}
		partial := target + ".download"
		_ = os.Remove(partial)
		if err := os.WriteFile(partial, data, 0o644); err != nil {
			fail(err)
			return
		}
		if err := verifyNVIDIASigned(partial); err != nil {
			_ = os.Remove(partial)
			fail(fmt.Errorf("%s was not installed: %v", f.name, err))
			return
		}
		if err := os.Rename(partial, target); err != nil {
			_ = os.Remove(partial)
			fail(err)
			return
		}
		e.app.Log.Printf("dlss: installed %s (%s %s, signed by NVIDIA)", f.name, f.what, streamlineVersion)
	}
	update(func(j *DLSSJob) { j.Running, j.OK, j.File = false, true, "" })
}

// cleanDLSSPartials removes files left by an interrupted download (called at start-up).
func (e *Emulator) cleanDLSSPartials() {
	matches, _ := filepath.Glob(filepath.Join(e.streamlineDir(), "*.download"))
	for _, p := range matches {
		_ = os.Remove(p)
	}
}
