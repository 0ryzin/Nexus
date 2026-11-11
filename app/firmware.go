package main

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Console avatar icons are NOT shipped with NEXUS (they are Nintendo artwork).
// Exactly like Lichii's "Set avatar" dialog (src/yuzu/configuration/system/new_user_dialog.cpp),
// NEXUS reads them from the firmware the user installed from their own console:
// system data archive 010000000000080A, RomFS folder "chara", Yaz0-compressed
// 256x256 RGBA images. They are converted to PNG in System/Avatars/firmware.

const avatarArchiveID = 0x010000000000080A

type AvatarSet struct {
	mu    sync.Mutex
	app   *App
	busy  bool
	state FirmwareAvatarState
}

// firmwareDir is the first folder that holds firmware: NEXUS's own system
// files, then the engines' installs.
func (a *App) firmwareDir() string {
	srcs := a.Sys.firmwareSources()
	for _, d := range srcs {
		if dirHasFiles(d) {
			return d
		}
	}
	return srcs[0]
}

func (a *App) avatarDir() string { return filepath.Join(a.Paths.System, "Avatars", "firmware") }

// openNandNCA opens a registered NAND content, which may be a plain file or a
// folder of "00", "01"... parts (Lichii's split NCA layout).
func openNandNCA(path string) (io.ReaderAt, func(), error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !fi.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		return f, func() { f.Close() }, nil
	}
	var parts []*os.File
	var sizes []int64
	for i := 0; i < 0x100; i++ {
		var f *os.File
		for _, n := range []string{fmt.Sprintf("%02X", i), fmt.Sprintf("%02x", i)} {
			if g, err := os.Open(filepath.Join(path, n)); err == nil {
				f = g
				break
			}
		}
		if f == nil {
			break
		}
		st, _ := f.Stat()
		parts = append(parts, f)
		sizes = append(sizes, st.Size())
	}
	if len(parts) == 0 {
		return nil, nil, errors.New("empty split NCA")
	}
	closer := func() {
		for _, p := range parts {
			p.Close()
		}
	}
	return &concatReader{parts: parts, sizes: sizes}, closer, nil
}

type concatReader struct {
	parts []*os.File
	sizes []int64
}

func (c *concatReader) ReadAt(p []byte, off int64) (int, error) {
	n := 0
	for i, f := range c.parts {
		if off >= c.sizes[i] {
			off -= c.sizes[i]
			continue
		}
		m, err := f.ReadAt(p[n:], off)
		n += m
		if n == len(p) {
			return n, nil
		}
		if err != nil && err != io.EOF {
			return n, err
		}
		off = 0
	}
	return n, io.EOF
}

// FirmwareAvatarState describes the automatic provisioning of console icons.
type FirmwareAvatarState struct {
	State   string `json:"state"` // ready | working | no-firmware | no-keys | not-found | error
	Count   int    `json:"count"`
	Message string `json:"message,omitempty"`
	Rev     string `json:"rev,omitempty"`
	At      int64  `json:"at,omitempty"`
}

func (s *AvatarSet) sourcePath() string { return filepath.Join(s.app.avatarDir(), ".source.json") }

// sourceRevision fingerprints the installed firmware + keys so icons are
// re-extracted automatically when the user installs or updates firmware.
func (s *AvatarSet) sourceRevision() (string, bool, bool) {
	a := s.app
	reg := a.firmwareDir()
	entries, err := os.ReadDir(reg)
	fw := err == nil && len(entries) > 0
	keys := len(LoadKeys(a.Library.keyPaths()...)["header_key"]) == 32
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return fmt.Sprintf("%x|%d|%s", len(names), len(strings.Join(names, ",")), keysRevision(a.Library.keyPaths())) + "|" + strings.Join(names, ","), fw, keys
}

func (s *AvatarSet) Status() FirmwareAvatarState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.State == "" {
		var st FirmwareAvatarState
		if readJSON(s.sourcePath(), &st) == nil {
			s.state = st
		}
	}
	return s.state
}

func (s *AvatarSet) setState(st FirmwareAvatarState) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
	s.app.Events.Emit("avatars.state", st)
}

// Ensure provisions the console icon library automatically: it runs at
// startup, after Lichii closes (the user may have installed firmware there)
// and periodically until icons are available. force re-extracts.
func (s *AvatarSet) Ensure(force bool) FirmwareAvatarState {
	rev, fw, keys := s.sourceRevision()
	cur := s.Status()
	if !force && cur.State == "ready" && cur.Rev == rev && len(s.List()) > 0 {
		return cur
	}
	if !fw {
		st := FirmwareAvatarState{State: "no-firmware", Message: "No console firmware is installed yet. The profile icons come from your firmware: install it in Settings › System files.", Rev: rev, At: nowUnix()}
		if len(s.List()) > 0 {
			st.State, st.Count = "ready", len(s.List()) // keep previously extracted icons
		}
		s.setState(st)
		return st
	}
	if !keys {
		st := FirmwareAvatarState{State: "no-keys", Message: "prod.keys is missing. Put the keys dumped from your own console in the keys folder, then the icons load automatically.", Rev: rev, At: nowUnix()}
		s.setState(st)
		return st
	}
	s.setState(FirmwareAvatarState{State: "working", Message: "Loading profile icons from your firmware…", Rev: rev})
	n, err := s.Extract()
	st := FirmwareAvatarState{State: "ready", Count: n, Rev: rev, At: nowUnix()}
	if err != nil {
		st.State, st.Message, st.Count = "error", err.Error(), 0
		if strings.Contains(err.Error(), "was not found") {
			st.State = "not-found"
		}
	}
	_ = writeJSONAtomic(s.sourcePath(), st)
	s.setState(st)
	return st
}

// Extract finds the avatar archive in the installed firmware and writes PNGs.
func (s *AvatarSet) Extract() (int, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return 0, errors.New("Already extracting.")
	}
	s.busy = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }()

	a := s.app
	reg := a.firmwareDir()
	entries, err := os.ReadDir(reg)
	if err != nil || len(entries) == 0 {
		return 0, errors.New("No firmware is installed. Install your console's firmware in Settings › System files, then try again.")
	}
	ks := LoadKeys(a.Library.keyPaths()...)
	if len(ks["header_key"]) != 32 {
		return 0, errors.New("prod.keys is missing or has no header_key. Put the keys from your own console in the keys folder first.")
	}
	var lastErr error
	for _, e := range entries {
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".nca") {
			continue
		}
		r, closeFn, err := openNandNCA(filepath.Join(reg, e.Name()))
		if err != nil {
			continue
		}
		h, err := readNCAHeader(r, 0, ks)
		if err != nil || h.ProgramID != avatarArchiveID || h.ContentType != 4 /*Data*/ {
			closeFn()
			continue
		}
		n, err := s.extractFrom(r, h, ks)
		closeFn()
		if err != nil {
			lastErr = err
			continue
		}
		a.Log.Printf("avatars: extracted %d firmware avatar images", n)
		return n, nil
	}
	if lastErr != nil {
		return 0, fmt.Errorf("The avatar archive was found but could not be read: %v", lastErr)
	}
	return 0, errors.New("The avatar archive (010000000000080A) was not found in the installed firmware. Reinstall the firmware in Settings › System files.")
}

// extractFrom decodes every Yaz0 image in the archive. "chara" (the folder
// Lichii's own avatar dialog uses) becomes characters/; any other folder of
// 256x256 images (e.g. backgrounds) is kept under its own name.
func (s *AvatarSet) extractFrom(r io.ReaderAt, h *ncaHeader, ks KeySet) (int, error) {
	rom, err := h.openRomFS(r, 0, ks, nil)
	if err != nil {
		return 0, err
	}
	files, err := romfsWalk(rom)
	if err != nil {
		return 0, err
	}
	root := s.app.avatarDir()
	tmp := root + ".new"
	_ = os.RemoveAll(tmp)
	count := 0
	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f.Path), ".szs") || f.Size <= 16 || f.Size > 8<<20 {
			continue
		}
		data, err := readAt(rom, f.Off, int(f.Size))
		if err != nil {
			continue
		}
		raw, err := DecompressYaz0(data)
		if err != nil || len(raw) != 256*256*4 {
			continue
		}
		dir := "other"
		if i := strings.Index(f.Path, "/"); i > 0 {
			dir = strings.ToLower(f.Path[:i])
		}
		if dir == "chara" {
			dir = "characters"
		}
		img := &image.NRGBA{Pix: raw, Stride: 256 * 4, Rect: image.Rect(0, 0, 256, 256)}
		out := filepath.Join(tmp, dir, strings.TrimSuffix(pathBase(f.Path), filepath.Ext(f.Path))+".png")
		_ = os.MkdirAll(filepath.Dir(out), 0o755)
		fh, err := os.Create(out)
		if err != nil {
			return count, err
		}
		err = png.Encode(fh, img)
		fh.Close()
		if err == nil {
			count++
		}
	}
	if count == 0 {
		_ = os.RemoveAll(tmp)
		return 0, errors.New("no 256x256 icon images in the archive")
	}
	// swap in atomically so the picker never sees a half-written library
	_ = os.RemoveAll(root)
	if err := os.Rename(tmp, root); err != nil {
		return 0, err
	}
	return count, nil
}
