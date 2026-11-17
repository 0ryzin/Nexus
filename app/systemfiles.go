package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SystemFiles manages the console system files NEXUS needs (prod.keys,
// title.keys and firmware). The user's own files are imported, validated and
// kept in a NEXUS directory of their own (<data>\SystemFiles), separate from
// emulator program folders and from save data, then installed into every
// emulation engine in the layout that engine expects.
//
// Nothing here is ever bundled with NEXUS.
type SystemFiles struct {
	app *App
	// noBackground disables the follow-up library scan / icon refresh (tests)
	noBackground bool
	mu           sync.Mutex
	job          *SysJob
}

type SysJob struct {
	Kind     string   `json:"kind"` // keys | firmware | sync | remove
	Active   bool     `json:"active"`
	Progress float64  `json:"progress"`
	Message  string   `json:"message"`
	Error    string   `json:"error,omitempty"`
	Done     bool     `json:"done"`
	Warnings []string `json:"warnings,omitempty"`
}

// FirmwareInfo is read from the firmware's own SystemVersion archive
// (0100000000000809, RomFS file "file", nn::settings::system::FirmwareVersion).
type FirmwareInfo struct {
	Version     string `json:"version"`  // display version, e.g. "19.0.1"
	Numeric     string `json:"numeric"`  // major.minor.micro from the binary fields
	Title       string `json:"title"`    // display title, e.g. "NintendoSDK Firmware for NX 19.0.1-1.0"
	Platform    string `json:"platform"` // "NX"
	Revision    string `json:"revision"` // revision major.minor
	NCAs        int    `json:"ncas"`     // number of contents
	Bytes       int64  `json:"bytes"`
	InstalledAt int64  `json:"installedAt,omitempty"`
	Source      string `json:"source,omitempty"`
	KeyGenMax   int    `json:"keyGenMax"`  // highest NCA key generation used by the firmware
	Unreadable  int    `json:"unreadable"` // NCAs whose header did not decrypt
}

// KeysInfo is the validation result of a prod.keys file.
type KeysInfo struct {
	Present      bool     `json:"present"`
	Valid        bool     `json:"valid"`
	Count        int      `json:"count"`
	BadLines     int      `json:"badLines"`
	HeaderKey    bool     `json:"headerKey"`
	KeyAreaKeys  int      `json:"keyAreaKeys"`  // key_area_key_application_XX present
	MasterKeyMax int      `json:"masterKeyMax"` // highest master_key_XX (-1 none)
	SupportsUpTo string   `json:"supportsUpTo"` // firmware generation the keys cover
	TitleKeys    int      `json:"titleKeys"`    // entries in title.keys
	Verified     string   `json:"verified"`     // how header_key was checked against real content
	Errors       []string `json:"errors,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	InstalledAt  int64    `json:"installedAt,omitempty"`
}

// masterKeyFirmware maps a master key revision to the first firmware that
// needed it (from the public key-generation history used by hactool/LibHac).
var masterKeyFirmware = []string{"1.0.0", "3.0.0", "3.0.1", "4.0.0", "5.0.0", "6.0.0", "6.2.0", "7.0.0",
	"8.1.0", "9.0.0", "9.1.0", "12.1.0", "13.0.0", "14.0.0", "15.0.0", "16.0.0", "17.0.0", "18.0.0", "19.0.0", "20.0.0"}

const systemVersionID = 0x0100000000000809

func NewSystemFiles(a *App) *SystemFiles { return &SystemFiles{app: a} }

func (s *SystemFiles) Root() string        { return filepath.Join(s.app.Paths.System, "SystemFiles") }
func (s *SystemFiles) KeysDir() string     { return filepath.Join(s.Root(), "Keys") }
func (s *SystemFiles) ProdKeys() string    { return filepath.Join(s.KeysDir(), "prod.keys") }
func (s *SystemFiles) TitleKeys() string   { return filepath.Join(s.KeysDir(), "title.keys") }
func (s *SystemFiles) FirmwareDir() string { return filepath.Join(s.Root(), "Firmware") }
func (s *SystemFiles) Registered() string  { return filepath.Join(s.FirmwareDir(), "registered") }
func (s *SystemFiles) metaPath() string    { return filepath.Join(s.Root(), "systemfiles.json") }

type sysMeta struct {
	KeysInstalled     int64         `json:"keysInstalled,omitempty"`
	FirmwareInstalled int64         `json:"firmwareInstalled,omitempty"`
	Firmware          *FirmwareInfo `json:"firmware,omitempty"`
}

func (s *SystemFiles) meta() sysMeta {
	var m sysMeta
	_ = readJSON(s.metaPath(), &m)
	return m
}

func (s *SystemFiles) saveMeta(f func(*sysMeta)) {
	m := s.meta()
	f(&m)
	_ = os.MkdirAll(s.Root(), 0o755)
	_ = writeJSONAtomic(s.metaPath(), m)
}

func (s *SystemFiles) Job() *SysJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return nil
	}
	c := *s.job
	return &c
}

func (s *SystemFiles) setJob(j SysJob) {
	s.mu.Lock()
	c := j
	s.job = &c
	s.mu.Unlock()
	s.app.Events.Emit("system.job", j)
}

func (s *SystemFiles) busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.job != nil && s.job.Active
}

// ------------------------------------------------------------------ keys

var keyLine = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=\s*([0-9A-Fa-f]+)$`)

// ValidateKeys checks a prod.keys file without installing it.
func (s *SystemFiles) ValidateKeys(path string) KeysInfo {
	ki := KeysInfo{MasterKeyMax: -1}
	data, err := os.ReadFile(path)
	if err != nil {
		ki.Errors = append(ki.Errors, "The file could not be read: "+err.Error())
		return ki
	}
	ki.Present = true
	if len(data) > 1<<20 {
		ki.Errors = append(ki.Errors, "This file is too large to be a prod.keys file.")
		return ki
	}
	ks := KeySet{}
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		m := keyLine.FindStringSubmatch(line)
		if m == nil || len(m[2])%2 != 0 {
			ki.BadLines++
			continue
		}
		ks[strings.ToLower(m[1])] = hexBytes(m[2])
	}
	ki.Count = len(ks)
	if ki.Count == 0 {
		ki.Errors = append(ki.Errors, "No keys were found. A prod.keys file has lines like  header_key = 0123…  (it is usually created by Lockpick_RCM on your own console).")
		return ki
	}
	if hk := ks["header_key"]; len(hk) == 32 {
		ki.HeaderKey = true
	} else if hk != nil {
		ki.Errors = append(ki.Errors, fmt.Sprintf("header_key has the wrong length (%d bytes, expected 32).", len(hk)))
	} else {
		ki.Errors = append(ki.Errors, "header_key is missing. Without it no game or firmware can be read.")
	}
	for k, v := range ks {
		switch {
		case strings.HasPrefix(k, "key_area_key_application_"):
			if len(v) == 16 {
				ki.KeyAreaKeys++
			} else {
				ki.Errors = append(ki.Errors, k+" has the wrong length.")
			}
		case strings.HasPrefix(k, "master_key_") && !strings.Contains(k, "source"):
			if n, err := strconv.ParseInt(strings.TrimPrefix(k, "master_key_"), 16, 32); err == nil && len(v) == 16 && int(n) > ki.MasterKeyMax {
				ki.MasterKeyMax = int(n)
			}
		}
	}
	if ki.KeyAreaKeys == 0 {
		ki.Errors = append(ki.Errors, "No key_area_key_application_XX entries. Games cannot be decrypted with these keys.")
	}
	if ki.MasterKeyMax >= 0 {
		if ki.MasterKeyMax < len(masterKeyFirmware) {
			ki.SupportsUpTo = masterKeyFirmware[ki.MasterKeyMax]
		} else {
			ki.SupportsUpTo = "newer than " + masterKeyFirmware[len(masterKeyFirmware)-1]
		}
	}
	if ki.BadLines > 0 {
		ki.Warnings = append(ki.Warnings, fmt.Sprintf("%d line(s) were not valid key entries and were ignored.", ki.BadLines))
	}
	ki.Valid = len(ki.Errors) == 0
	// prove header_key against real content when firmware is available
	if ki.Valid {
		ki.Verified = s.verifyHeaderKey(ks)
		if strings.HasPrefix(ki.Verified, "failed") {
			ki.Valid = false
			ki.Errors = append(ki.Errors, "These keys cannot decrypt your installed firmware. They are probably from a different console or corrupted.")
		}
	}
	return ki
}

func hexBytes(h string) []byte {
	out := make([]byte, len(h)/2)
	for i := 0; i < len(out); i++ {
		v, _ := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		out[i] = byte(v)
	}
	return out
}

// verifyHeaderKey decrypts one firmware NCA header with the key.
func (s *SystemFiles) verifyHeaderKey(ks KeySet) string {
	for _, dir := range s.firmwareSources() {
		entries, _ := os.ReadDir(dir)
		tried := 0
		for _, e := range entries {
			if !strings.HasSuffix(strings.ToLower(e.Name()), ".nca") {
				continue
			}
			r, c, err := openNandNCA(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			_, err = readNCAHeader(r, 0, ks)
			c()
			tried++
			if err == nil {
				return "decrypts the installed firmware"
			}
			if tried >= 3 {
				return "failed: could not decrypt the installed firmware"
			}
		}
	}
	return "not checked yet (no firmware installed)"
}

// KeysStatus validates the installed keys.
func (s *SystemFiles) KeysStatus() KeysInfo {
	if !fileExists(s.ProdKeys()) {
		return KeysInfo{MasterKeyMax: -1}
	}
	ki := s.ValidateKeys(s.ProdKeys())
	ki.TitleKeys = countKeyLines(s.TitleKeys())
	ki.InstalledAt = s.meta().KeysInstalled
	return ki
}

func countKeyLines(p string) int {
	data, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n := 0
	for _, l := range strings.Split(string(data), "\n") {
		if keyLine.MatchString(strings.TrimSpace(l)) {
			n++
		}
	}
	return n
}

// InstallKeys validates and installs prod.keys (and an optional title.keys).
// An existing file is archived, never overwritten in place.
func (s *SystemFiles) InstallKeys(prod, title string) (KeysInfo, error) {
	ki := s.ValidateKeys(prod)
	if !ki.Valid {
		return ki, errors.New(strings.Join(ki.Errors, " "))
	}
	if title != "" && countKeyLines(title) == 0 {
		return ki, errors.New("The title.keys file contains no valid entries.")
	}
	if err := os.MkdirAll(s.KeysDir(), 0o755); err != nil {
		return ki, err
	}
	s.archive("keys", []string{s.ProdKeys(), s.TitleKeys()}, title == "")
	if err := copyFileAtomic(prod, s.ProdKeys()); err != nil {
		return ki, err
	}
	if title != "" {
		if err := copyFileAtomic(title, s.TitleKeys()); err != nil {
			return ki, err
		}
	}
	s.saveMeta(func(m *sysMeta) { m.KeysInstalled = nowUnix() })
	warns := s.deployKeys()
	ki = s.KeysStatus()
	ki.Warnings = append(ki.Warnings, warns...)
	s.app.Log.Printf("system files: keys installed (%d keys, master key %02x)", ki.Count, ki.MasterKeyMax)
	// only after a successful install, and before returning: a background copy used to start on
	// every call (rejected files too) and could copy keys while the next install wrote them
	s.app.Emu.SyncSystemVault()
	if !s.noBackground {
		go s.app.Avatars.Ensure(false)
		go s.app.Library.Scan()
	}
	return ki, nil
}

// archive moves files aside into <data>\Archived\system-files-<time>.
// With onlyFirst, only the first path is archived (title.keys is kept).
func (s *SystemFiles) archive(kind string, paths []string, onlyFirst bool) string {
	base := filepath.Join(s.app.Paths.Trash, "system-files-"+time.Now().Format("20060102-150405"))
	dst := filepath.Join(base, kind)
	for i := 2; dirExists(dst); i++ {
		dst = filepath.Join(fmt.Sprintf("%s-%d", base, i), kind)
	}
	moved := ""
	for i, p := range paths {
		if onlyFirst && i > 0 {
			break
		}
		if !fileExists(p) && !dirExists(p) {
			continue
		}
		_ = os.MkdirAll(dst, 0o755)
		if err := os.Rename(p, filepath.Join(dst, filepath.Base(p))); err == nil {
			moved = dst
		}
	}
	return moved
}

func copyFileAtomic(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// ------------------------------------------------------------------ firmware

// firmwareSources lists folders of firmware NCAs: NEXUS's own copy first, then
// what the engines already have (so existing installs are recognised).
func (s *SystemFiles) firmwareSources() []string {
	out := []string{s.Registered()}
	for _, t := range s.engineTargets() {
		out = append(out, t.FirmwareDir)
	}
	return out
}

// ReadFirmware identifies the firmware in a folder of NCAs.
func ReadFirmware(dir string, ks KeySet) (*FirmwareInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.New("no firmware folder")
	}
	fi := &FirmwareInfo{}
	var verNCA string
	for _, e := range entries {
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".nca") {
			continue
		}
		fi.NCAs++
		p := filepath.Join(dir, e.Name())
		fi.Bytes += pathSize(p)
		r, c, err := openNandNCA(p)
		if err != nil {
			fi.Unreadable++
			continue
		}
		h, err := readNCAHeader(r, 0, ks)
		c()
		if err != nil {
			fi.Unreadable++
			continue
		}
		if h.KeyGen > fi.KeyGenMax {
			fi.KeyGenMax = h.KeyGen
		}
		if h.ProgramID == systemVersionID && h.ContentType == 4 {
			verNCA = p
		}
	}
	if fi.NCAs == 0 {
		return nil, errors.New("no NCA files")
	}
	if len(ks["header_key"]) != 32 {
		return fi, errors.New("prod.keys is needed to read the firmware")
	}
	if fi.Unreadable == fi.NCAs {
		return fi, errors.New("none of its files can be decrypted with your prod.keys (keys from another console, or not a firmware dump)")
	}
	if verNCA == "" {
		return fi, errors.New("the SystemVersion archive (0100000000000809) is missing, so this is not a complete firmware")
	}
	r, c, err := openNandNCA(verNCA)
	if err != nil {
		return fi, err
	}
	defer c()
	h, err := readNCAHeader(r, 0, ks)
	if err != nil {
		return fi, err
	}
	rom, err := h.openRomFS(r, 0, ks, nil)
	if err != nil {
		return fi, fmt.Errorf("the SystemVersion archive could not be decrypted (keys too old for this firmware?): %w", err)
	}
	files, err := romfsFiles(rom, func(n string) bool { return n == "file" }, 0x1000)
	if err != nil || len(files["file"]) < 0x100 {
		return fi, errors.New("the SystemVersion archive is damaged")
	}
	b := files["file"]
	cstr := func(x []byte) string {
		if i := strings.IndexByte(string(x), 0); i >= 0 {
			x = x[:i]
		}
		return strings.TrimSpace(string(x))
	}
	fi.Numeric = fmt.Sprintf("%d.%d.%d", b[0], b[1], b[2])
	fi.Revision = fmt.Sprintf("%d.%d", b[4], b[5])
	fi.Platform = cstr(b[0x08:0x28])
	fi.Version = cstr(b[0x68:0x80])
	fi.Title = cstr(b[0x80:0x100])
	if fi.Version == "" {
		fi.Version = fi.Numeric
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(fi.Version) {
		return fi, errors.New("the firmware version information is not recognised")
	}
	return fi, nil
}

func pathSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	if !st.IsDir() {
		return st.Size()
	}
	n, _ := dirSize(p)
	return n
}

// FirmwareStatus describes the NEXUS copy of the firmware.
func (s *SystemFiles) FirmwareStatus() (*FirmwareInfo, string) {
	if !dirHasFiles(s.Registered()) {
		return nil, "not-installed"
	}
	ks := LoadKeys(s.ProdKeys())
	fi, err := ReadFirmware(s.Registered(), ks)
	if err != nil {
		if fi == nil {
			return nil, "not-installed"
		}
		return fi, "unrecognised: " + err.Error()
	}
	m := s.meta()
	fi.InstalledAt = m.FirmwareInstalled
	if m.Firmware != nil {
		fi.Source = m.Firmware.Source
	}
	return fi, "ok"
}

type fwSource struct {
	name string
	open func() (io.ReadCloser, error)
	size int64
}

// collectFirmware lists NCAs in a folder (flat, split "x.nca/00" folders) or a
// zip archive, the inputs console firmware dumps come in.
func collectFirmware(src string) ([]fwSource, func(), error) {
	st, err := os.Stat(src)
	if err != nil {
		return nil, nil, fmt.Errorf("%s could not be opened", src)
	}
	var out []fwSource
	if st.IsDir() {
		err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".nca") {
				return nil
			}
			if d.IsDir() { // split NCA folder (00, 01, …)
				pp := p
				out = append(out, fwSource{name: name, size: pathSize(p), open: func() (io.ReadCloser, error) {
					r, c, err := openNandNCA(pp)
					if err != nil {
						return nil, err
					}
					return &readerAtCloser{r: r, c: c, n: pathSize(pp)}, nil
				}})
				return filepath.SkipDir
			}
			pp := p
			info, _ := d.Info()
			out = append(out, fwSource{name: name, size: info.Size(), open: func() (io.ReadCloser, error) { return os.Open(pp) }})
			return nil
		})
		return out, func() {}, err
	}
	if !strings.EqualFold(filepath.Ext(src), ".zip") {
		return nil, nil, errors.New("choose the folder that contains the firmware's .nca files, or a .zip of it")
	}
	zr, err := zip.OpenReader(src)
	if err != nil {
		return nil, nil, fmt.Errorf("the zip archive could not be opened: %w", err)
	}
	parts := map[string][]*zip.File{}
	for _, f := range zr.File {
		n := strings.ReplaceAll(f.Name, "\\", "/")
		segs := strings.Split(n, "/")
		last := segs[len(segs)-1]
		switch {
		case strings.HasSuffix(strings.ToLower(last), ".nca") && !f.FileInfo().IsDir():
			ff := f
			out = append(out, fwSource{name: last, size: int64(f.UncompressedSize64), open: func() (io.ReadCloser, error) { return ff.Open() }})
		case len(segs) >= 2 && strings.HasSuffix(strings.ToLower(segs[len(segs)-2]), ".nca") && regexp.MustCompile(`^[0-9A-Fa-f]{2}$`).MatchString(last):
			parts[segs[len(segs)-2]] = append(parts[segs[len(segs)-2]], f)
		}
	}
	for name, fs := range parts {
		sort.Slice(fs, func(i, j int) bool { return fs[i].Name < fs[j].Name })
		fs := fs
		var size int64
		for _, f := range fs {
			size += int64(f.UncompressedSize64)
		}
		out = append(out, fwSource{name: name, size: size, open: func() (io.ReadCloser, error) { return multiZipReader(fs) }})
	}
	return out, func() { zr.Close() }, nil
}

type readerAtCloser struct {
	r   io.ReaderAt
	c   func()
	off int64
	n   int64
}

func (r *readerAtCloser) Read(p []byte) (int, error) {
	if r.off >= r.n {
		return 0, io.EOF
	}
	if int64(len(p)) > r.n-r.off {
		p = p[:r.n-r.off]
	}
	k, err := r.r.ReadAt(p, r.off)
	r.off += int64(k)
	if err == io.EOF && k > 0 {
		err = nil
	}
	return k, err
}
func (r *readerAtCloser) Close() error { r.c(); return nil }

func multiZipReader(fs []*zip.File) (io.ReadCloser, error) {
	var rs []io.Reader
	var cs []io.Closer
	for _, f := range fs {
		rc, err := f.Open()
		if err != nil {
			for _, c := range cs {
				c.Close()
			}
			return nil, err
		}
		rs = append(rs, rc)
		cs = append(cs, rc)
	}
	return struct {
		io.Reader
		io.Closer
	}{io.MultiReader(rs...), closerFunc(func() error {
		for _, c := range cs {
			c.Close()
		}
		return nil
	})}, nil
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

var ncaFileName = regexp.MustCompile(`(?i)^[0-9a-f]{32}(\.cnmt)?\.nca$`)

// InstallFirmware validates a firmware folder/zip and installs it into NEXUS's
// system files, then into every engine. The previous firmware is archived only
// after the new one has been fully validated.
func (s *SystemFiles) InstallFirmware(src string) (err error) {
	defer func() {
		if err == nil {
			go s.app.Emu.SaveFirmwareToVault()
		}
	}()
	if s.busy() {
		return errors.New("Another system file operation is still running.")
	}
	if st := s.app.Emu.State(); st.Running {
		return errors.New("Close the running game before changing firmware.")
	}
	ks := LoadKeys(s.ProdKeys())
	if len(ks["header_key"]) != 32 {
		return errors.New("Install your prod.keys first. NEXUS needs them to check that the firmware is genuine and complete.")
	}
	files, closeFn, err := collectFirmware(src)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		closeFn()
		return errors.New("No .nca files were found there. Choose the folder (or .zip) that contains your firmware dump's .nca files.")
	}
	s.setJob(SysJob{Kind: "firmware", Active: true, Message: "Checking firmware…"})
	go func() {
		defer closeFn()
		info, warns, err := s.installFirmware(files, ks, src)
		if err != nil {
			s.app.Log.Printf("system files: firmware install failed: %v", err)
			s.setJob(SysJob{Kind: "firmware", Done: true, Error: err.Error()})
			return
		}
		s.setJob(SysJob{Kind: "firmware", Done: true, Progress: 1, Warnings: warns,
			Message: fmt.Sprintf("Firmware %s installed and verified (%d files).", info.Version, info.NCAs)})
		if !s.noBackground {
			go s.app.Avatars.Ensure(true)
		}
	}()
	return nil
}

func (s *SystemFiles) installFirmware(files []fwSource, ks KeySet, src string) (*FirmwareInfo, []string, error) {
	stage := filepath.Join(s.Root(), "Firmware.new")
	_ = os.RemoveAll(stage)
	reg := filepath.Join(stage, "registered")
	if err := os.MkdirAll(reg, 0o755); err != nil {
		return nil, nil, err
	}
	var total, done int64
	for _, f := range files {
		total += f.size
	}
	seen := map[string]bool{}
	for i, f := range files {
		name := strings.ToLower(f.name)
		if !ncaFileName.MatchString(name) {
			_ = os.RemoveAll(stage)
			return nil, nil, fmt.Errorf("%s is not a firmware content file (expected names like 0a1b…32 hex….nca)", f.name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		rc, err := f.open()
		if err != nil {
			_ = os.RemoveAll(stage)
			return nil, nil, fmt.Errorf("could not read %s: %w", f.name, err)
		}
		w, err := os.Create(filepath.Join(reg, name))
		if err != nil {
			rc.Close()
			_ = os.RemoveAll(stage)
			return nil, nil, err
		}
		n, err := io.Copy(w, rc)
		rc.Close()
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.RemoveAll(stage)
			return nil, nil, fmt.Errorf("could not copy %s: %w", f.name, err)
		}
		done += n
		if i%8 == 0 && total > 0 {
			s.setJob(SysJob{Kind: "firmware", Active: true, Progress: float64(done) / float64(total) * 0.6,
				Message: fmt.Sprintf("Copying firmware… %d / %d files", i+1, len(files))})
		}
	}
	s.setJob(SysJob{Kind: "firmware", Active: true, Progress: 0.62, Message: "Validating firmware…"})
	info, err := ReadFirmware(reg, ks)
	if err != nil {
		_ = os.RemoveAll(stage)
		return nil, nil, fmt.Errorf("This firmware was not installed: %v.", err)
	}
	var warns []string
	if info.Unreadable > 0 {
		_ = os.RemoveAll(stage)
		return nil, nil, fmt.Errorf("This firmware was not installed: %d of %d files could not be decrypted with your keys. The dump may be damaged, or the keys may be from an older firmware.", info.Unreadable, info.NCAs)
	}
	if info.KeyGenMax >= 0 && info.KeyGenMax < 64 && len(ks[fmt.Sprintf("key_area_key_application_%02x", info.KeyGenMax)]) != 16 {
		warns = append(warns, fmt.Sprintf("Your prod.keys do not include the keys for firmware %s (key generation %d). Dump your keys again from the same console after updating it, or games made for this firmware will not start.", info.Version, info.KeyGenMax))
	}
	if info.NCAs < 40 {
		warns = append(warns, fmt.Sprintf("Only %d firmware files were found; a full firmware usually has well over 100. It was identified as %s, but it may be incomplete.", info.NCAs, info.Version))
	}
	// swap into place (old copy archived, not deleted)
	cur := s.FirmwareDir()
	if dirExists(cur) {
		s.archive("firmware", []string{cur}, false)
	}
	if err := os.Rename(stage, cur); err != nil {
		return nil, nil, fmt.Errorf("could not move the new firmware into place: %w", err)
	}
	info.Source = src
	info.InstalledAt = nowUnix()
	s.saveMeta(func(m *sysMeta) { m.FirmwareInstalled = info.InstalledAt; m.Firmware = info })
	s.setJob(SysJob{Kind: "firmware", Active: true, Progress: 0.7, Message: "Installing into the emulation engines…"})
	warns = append(warns, s.deployFirmware(func(p float64) {
		s.setJob(SysJob{Kind: "firmware", Active: true, Progress: 0.7 + p*0.29, Message: "Installing into the emulation engines…"})
	})...)
	s.app.Log.Printf("system files: firmware %s installed (%d NCAs, keygen %d)", info.Version, info.NCAs, info.KeyGenMax)
	return info, warns, nil
}

// ------------------------------------------------------------------ engines

// EngineTarget is where one engine expects the system files.
type EngineTarget struct {
	Engine      string `json:"engine"`
	Name        string `json:"name"`
	KeysDir     string `json:"keysDir"`
	FirmwareDir string `json:"firmwareDir"`
	SplitLayout bool   `json:"splitLayout"`
}

// engineTargets: the NEXUS engine reads keys from <engine data>/keys and firmware NCAs stored
// flat in <engine data>/nand/system/Contents/registered (the layout of its upstream core).
func (s *SystemFiles) engineTargets() []EngineTarget {
	e := s.app.Emu
	return []EngineTarget{{Engine: engineID, Name: "NEXUS Engine", KeysDir: e.KeysDir(), FirmwareDir: e.FirmwareDir()}}
}

// engineInstalled: the engine data folder belongs to NEXUS, so system files are always
// installed there, even before an engine build is installed.
func (s *SystemFiles) engineInstalled(string) bool { return true }

// deployKeys copies the NEXUS keys into every installed engine. A different
// existing key file is backed up first.
func (s *SystemFiles) deployKeys() []string {
	var warns []string
	for _, t := range s.engineTargets() {
		if !s.engineInstalled(t.Engine) {
			continue
		}
		if err := os.MkdirAll(t.KeysDir, 0o755); err != nil {
			warns = append(warns, t.Name+": "+err.Error())
			continue
		}
		for _, f := range []string{"prod.keys", "title.keys"} {
			src := filepath.Join(s.KeysDir(), f)
			if !fileExists(src) {
				continue
			}
			dst := filepath.Join(t.KeysDir, f)
			if sameFile(src, dst) {
				continue
			}
			if fileExists(dst) {
				_ = copyFileOverwrite(dst, dst+".nexus-backup-"+time.Now().Format("20060102-150405"))
			}
			if err := copyFileAtomic(src, dst); err != nil {
				warns = append(warns, fmt.Sprintf("%s: could not install %s (%v)", t.Name, f, err))
			}
		}
	}
	return warns
}

func sameFile(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && string(x) == string(y)
}

// deployFirmware installs the NEXUS firmware into every installed engine,
// building the engine's copy beside it and swapping it in; the engine's
// previous firmware is archived, not deleted.
func (s *SystemFiles) deployFirmware(progress func(float64)) []string {
	var warns []string
	src := s.Registered()
	entries, _ := os.ReadDir(src)
	targets := s.engineTargets()
	for ti, t := range targets {
		if !s.engineInstalled(t.Engine) {
			continue
		}
		stage := t.FirmwareDir + ".nexus-new"
		_ = os.RemoveAll(stage)
		if err := os.MkdirAll(stage, 0o755); err != nil {
			warns = append(warns, t.Name+": "+err.Error())
			continue
		}
		ok := true
		for i, e := range entries {
			from := filepath.Join(src, e.Name())
			to := filepath.Join(stage, e.Name())
			if t.SplitLayout {
				_ = os.MkdirAll(to, 0o755)
				to = filepath.Join(to, "00")
			}
			if err := linkOrCopy(from, to); err != nil {
				warns = append(warns, fmt.Sprintf("%s: could not copy firmware (%v); its previous firmware was kept.", t.Name, err))
				ok = false
				break
			}
			if progress != nil && i%16 == 0 {
				progress((float64(ti) + float64(i)/float64(len(entries))) / float64(len(targets)))
			}
		}
		if !ok {
			_ = os.RemoveAll(stage)
			continue
		}
		if dirExists(t.FirmwareDir) {
			s.archive("firmware-"+t.Engine, []string{t.FirmwareDir}, false)
		}
		_ = os.MkdirAll(filepath.Dir(t.FirmwareDir), 0o755)
		if err := os.Rename(stage, t.FirmwareDir); err != nil {
			warns = append(warns, fmt.Sprintf("%s: could not activate the new firmware (%v).", t.Name, err))
		}
	}
	return warns
}

// linkOrCopy hard-links when possible (same drive: no extra space), else copies.
func linkOrCopy(from, to string) error {
	if err := os.Link(from, to); err == nil {
		return nil
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Sync re-installs NEXUS's keys and firmware into every engine (e.g. after
// installing a new engine).
func (s *SystemFiles) Sync() []string {
	warns := s.deployKeys()
	if dirHasFiles(s.Registered()) {
		warns = append(warns, s.deployFirmware(nil)...)
	}
	return warns
}

// EnsureEngine installs NEXUS's own copy of the keys and firmware (Settings › System files)
// into the engine data folder when the engine does not have them yet, e.g. after updating from
// NEXUS 1.x, whose copies were kept for the old external emulators.
func (s *SystemFiles) EnsureEngine() []string {
	var warns []string
	if fileExists(s.ProdKeys()) && !s.app.Emu.KeysPresent() {
		warns = append(warns, s.deployKeys()...)
	}
	if dirHasFiles(s.Registered()) && !s.app.Emu.FirmwarePresent() {
		warns = append(warns, s.deployFirmware(nil)...)
	}
	return warns
}

// EngineSystemState is what one engine currently has.
type EngineSystemState struct {
	EngineTarget
	Installed bool   `json:"installed"`
	Keys      string `json:"keys"`     // match | different | missing
	Firmware  string `json:"firmware"` // detected version, "" none
	FWState   string `json:"fwState"`  // match | different | missing | unrecognised
}

func (s *SystemFiles) EngineStates(nexusFW *FirmwareInfo) []EngineSystemState {
	ks := LoadKeys(s.ProdKeys())
	var out []EngineSystemState
	for _, t := range s.engineTargets() {
		st := EngineSystemState{EngineTarget: t, Installed: s.engineInstalled(t.Engine)}
		dst := filepath.Join(t.KeysDir, "prod.keys")
		switch {
		case !fileExists(dst):
			st.Keys = "missing"
		case fileExists(s.ProdKeys()) && sameFile(s.ProdKeys(), dst):
			st.Keys = "match"
		default:
			st.Keys = "different"
		}
		useKs := ks
		if len(useKs["header_key"]) != 32 {
			useKs = LoadKeys(dst)
		}
		if !dirHasFiles(t.FirmwareDir) {
			st.FWState = "missing"
		} else if fi, err := ReadFirmware(t.FirmwareDir, useKs); err != nil {
			st.FWState = "unrecognised"
		} else {
			st.Firmware = fi.Version
			st.FWState = "different"
			if nexusFW != nil && nexusFW.Version == fi.Version {
				st.FWState = "match"
			}
		}
		out = append(out, st)
	}
	return out
}

// Remove archives NEXUS's keys or firmware (and optionally each engine's copy).
func (s *SystemFiles) Remove(which string, engines bool) (string, error) {
	if s.busy() {
		return "", errors.New("Another system file operation is still running.")
	}
	if st := s.app.Emu.State(); st.Running {
		return "", errors.New("Close the running game first.")
	}
	var dst string
	switch which {
	case "keys":
		dst = s.archive("keys", []string{s.ProdKeys(), s.TitleKeys()}, false)
		s.saveMeta(func(m *sysMeta) { m.KeysInstalled = 0 })
		if engines {
			for _, t := range s.engineTargets() {
				s.archive("keys-"+t.Engine, []string{filepath.Join(t.KeysDir, "prod.keys"), filepath.Join(t.KeysDir, "title.keys")}, false)
			}
		}
	case "firmware":
		dst = s.archive("firmware", []string{s.FirmwareDir()}, false)
		s.saveMeta(func(m *sysMeta) { m.FirmwareInstalled = 0; m.Firmware = nil })
		if engines {
			for _, t := range s.engineTargets() {
				s.archive("firmware-"+t.Engine, []string{t.FirmwareDir}, false)
			}
		}
	default:
		return "", errors.New("unknown system file")
	}
	s.app.Log.Printf("system files: %s removed (archived to %s, engines=%v)", which, dst, engines)
	return dst, nil
}

// Status is everything the System files page shows.
func (s *SystemFiles) Status() map[string]any {
	fw, fwState := s.FirmwareStatus()
	keys := s.KeysStatus()
	var detected string
	if fw != nil && fwState == "ok" {
		detected = fw.Version
	}
	engines := s.EngineStates(fw)
	if detected == "" {
		for _, e := range engines {
			if e.Firmware != "" && e.Installed {
				detected = e.Firmware + " (in " + e.Name + ")"
				break
			}
		}
	}
	ready := keys.Valid && fw != nil && fwState == "ok"
	if fw != nil && keys.MasterKeyMax >= 0 && fw.KeyGenMax > keys.MasterKeyMax {
		keys.Warnings = append(keys.Warnings, fmt.Sprintf("These keys are older than the installed firmware %s. Dump your keys again from your console.", fw.Version))
	}
	return map[string]any{
		"root":          s.Root(),
		"keys":          keys,
		"firmware":      fw,
		"firmwareState": fwState,
		"detected":      detected,
		"ready":         ready,
		"engines":       engines,
		"job":           s.Job(),
		"updateCheck":   "Nintendo does not publish a firmware feed NEXUS can verify, so NEXUS does not claim whether a newer firmware exists. Check your console's System Settings › System › System Update.",
	}
}
