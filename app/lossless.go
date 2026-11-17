package main

// Lossless Scaling setup (docs/LOSSLESS.md). NEXUS's frame generation can use the shaders of the
// user's own Lossless Scaling (Steam app 993090): the engine reads them from Lossless.dll in
// <engine data>\lossless (video_core/frame_gen/lossless_dll.cpp). NEXUS never downloads, bundles
// or shares that file; it only copies it from the user's own installation on this PC.
//
// Finding it: Steam's install folder comes from the registry (HKCU\Software\Valve\Steam
// SteamPath, HKLM ... InstallPath) and the usual folders; every Steam library is read from
// libraryfolders.vdf; in each library the app's manifest (appmanifest_993090.acf) names its
// install folder. The user can also choose a folder: the Lossless Scaling folder, a Steam
// library or Steam itself; NEXUS finds Lossless.dll in it.
//
// Checking it, before anything is copied, the way the engine reads it: a Windows PE file whose
// RCDATA resources hold every shader the engine needs (ids 255, 256, 280-302 at the native FP16
// offset +49) as SPIR-V modules. The DLL is never loaded or run, only read, so its CPU
// architecture does not matter; it is reported. An identical file already installed is not
// copied again; a different valid one replaces the installed one only when the user says so.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const losslessAppID = "993090"

// LosslessFile is one Lossless.dll and what NEXUS found in it.
type LosslessFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	SHA256   string `json:"sha256"`
	Machine  string `json:"machine,omitempty"` // x64 | x86 | ARM64 | …
	Shaders  int    `json:"shaders"`           // required shaders present
	Valid    bool   `json:"valid"`
	Problem  string `json:"problem,omitempty"`
}

// LosslessSetup is what the setup screen shows.
type LosslessSetup struct {
	State      string        `json:"state"` // ready | found | update | notfound | invalid | unsupported
	Message    string        `json:"message"`
	Installed  *LosslessFile `json:"installed,omitempty"`
	Found      *LosslessFile `json:"found,omitempty"`
	SteamRoots []string      `json:"steamRoots"`
	Libraries  []string      `json:"libraries"`
	Searched   []string      `json:"searched"` // where NEXUS looked for the app
}

// losslessRequired: resource ids the engine needs (lossless_dll.cpp, PerformanceShaderIds +
// NATIVE_FP16_OFFSET).
func losslessRequired() []uint32 {
	ids := []uint32{255, 256}
	for id := uint32(280); id <= 302; id++ {
		ids = append(ids, id)
	}
	for i := range ids {
		ids[i] += 49
	}
	return ids
}

// ---------- PE resource reading (only what the check needs)

type peSection struct{ va, vsize, raw, rawSize uint32 }

func peRVA(secs []peSection, rva uint32) (uint32, bool) {
	for _, s := range secs {
		size := s.vsize
		if s.rawSize > size {
			size = s.rawSize
		}
		if rva >= s.va && rva < s.va+size {
			return rva - s.va + s.raw, true
		}
	}
	return 0, false
}

// checkLosslessImage validates a Lossless.dll image the way the engine reads it.
func checkLosslessImage(img []byte) (machine string, shaders int, problem string) {
	le := binary.LittleEndian
	u16 := func(o uint32) (uint16, bool) {
		if uint64(o)+2 > uint64(len(img)) {
			return 0, false
		}
		return le.Uint16(img[o:]), true
	}
	u32 := func(o uint32) (uint32, bool) {
		if uint64(o)+4 > uint64(len(img)) {
			return 0, false
		}
		return le.Uint32(img[o:]), true
	}
	notPE := "This is not a Windows DLL."
	if mz, ok := u16(0); !ok || mz != 0x5A4D {
		return "", 0, notPE
	}
	peOff, ok := u32(0x3C)
	if !ok {
		return "", 0, notPE
	}
	if sig, ok := u32(peOff); !ok || sig != 0x00004550 {
		return "", 0, notPE
	}
	coff := peOff + 4
	mach, _ := u16(coff)
	machine = map[uint16]string{0x14c: "x86", 0x8664: "x64", 0xAA64: "ARM64", 0x1c4: "ARM"}[mach]
	if machine == "" {
		machine = fmt.Sprintf("0x%04x", mach)
	}
	nsec, _ := u16(coff + 2)
	optSize, _ := u16(coff + 16)
	opt := coff + 20
	magic, _ := u16(opt)
	var dd uint32
	switch magic {
	case 0x10B:
		dd = opt + 96
	case 0x20B:
		dd = opt + 112
	default:
		return machine, 0, notPE
	}
	resRVA, ok := u32(dd + 2*8)
	if !ok || resRVA == 0 {
		return machine, 0, "This DLL has no resources: it is not Lossless Scaling's Lossless.dll."
	}
	var secs []peSection
	so := opt + uint32(optSize)
	for i := uint32(0); i < uint32(nsec); i++ {
		b := so + i*40
		va, ok1 := u32(b + 12)
		vs, ok2 := u32(b + 8)
		rs, ok3 := u32(b + 16)
		ra, ok4 := u32(b + 20)
		if !(ok1 && ok2 && ok3 && ok4) {
			return machine, 0, notPE
		}
		secs = append(secs, peSection{va: va, vsize: vs, raw: ra, rawSize: rs})
	}
	base, ok := peRVA(secs, resRVA)
	if !ok {
		return machine, 0, notPE
	}
	// directory entries: (id, offset) pairs after a 16-byte header
	entries := func(dir uint32) [][2]uint32 {
		named, _ := u16(dir + 12)
		idc, _ := u16(dir + 14)
		var out [][2]uint32
		for i := uint32(0); i < uint32(named)+uint32(idc) && i < 4096; i++ {
			id, ok1 := u32(dir + 16 + i*8)
			off, ok2 := u32(dir + 16 + i*8 + 4)
			if !ok1 || !ok2 {
				break
			}
			out = append(out, [2]uint32{id, off})
		}
		return out
	}
	found := map[uint32][]byte{}
	for _, t := range entries(base) {
		if t[0] != 10 || t[1]&0x80000000 == 0 { // RT_RCDATA
			continue
		}
		for _, n := range entries(base + t[1]&0x7fffffff) {
			if n[0]&0x80000000 != 0 || n[1]&0x80000000 == 0 {
				continue // named, or not a directory
			}
			langs := entries(base + n[1]&0x7fffffff)
			if len(langs) == 0 || langs[0][1]&0x80000000 != 0 {
				continue
			}
			leaf := base + langs[0][1]
			rva, ok1 := u32(leaf)
			size, ok2 := u32(leaf + 4)
			if !ok1 || !ok2 {
				continue
			}
			if fo, ok := peRVA(secs, rva); ok && uint64(fo)+uint64(size) <= uint64(len(img)) {
				found[n[0]] = img[fo : fo+size]
			}
		}
	}
	missing := 0
	for _, id := range losslessRequired() {
		d := found[id]
		if len(d) >= 4 && le.Uint32(d) == 0x07230203 { // SPIR-V magic
			shaders++
		} else {
			missing++
		}
	}
	if missing > 0 {
		if shaders == 0 {
			return machine, 0, "This DLL does not contain Lossless Scaling's frame generation shaders. Use Lossless.dll from the Lossless Scaling app folder."
		}
		return machine, shaders, fmt.Sprintf("%d of the %d frame generation shaders are missing: this Lossless Scaling version is not one the NEXUS engine can use.", missing, missing+shaders)
	}
	return machine, shaders, ""
}

// inspectLossless reads and checks one Lossless.dll.
func inspectLossless(path string) *LosslessFile {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return nil
	}
	f := &LosslessFile{Path: path, Size: fi.Size(), Modified: fi.ModTime().Unix()}
	if fi.Size() > 256<<20 {
		f.Problem = "The file is far larger than Lossless.dll is."
		return f
	}
	img, err := os.ReadFile(path)
	if err != nil {
		f.Problem = "NEXUS could not read it: " + err.Error()
		return f
	}
	sum := sha256.Sum256(img)
	f.SHA256 = hex.EncodeToString(sum[:])
	f.Machine, f.Shaders, f.Problem = checkLosslessImage(img)
	f.Valid = f.Problem == ""
	return f
}

// ---------- finding Lossless Scaling

var (
	reVdfPath   = regexp.MustCompile(`"path"\s+"([^"]+)"`)
	reInstalDir = regexp.MustCompile(`"installdir"\s+"([^"]+)"`)
)

func vdfUnescape(s string) string { return strings.ReplaceAll(s, `\\`, `\`) }

// steamLibraries: every Steam library of the given Steam folders (the folders themselves first).
func steamLibraries(roots []string) []string {
	seen := map[string]bool{}
	var libs []string
	add := func(p string) {
		p = filepath.Clean(p)
		k := strings.ToLower(p)
		if p == "." || seen[k] || !dirExists(p) {
			return
		}
		seen[k] = true
		libs = append(libs, p)
	}
	for _, r := range roots {
		add(r)
		for _, vdf := range []string{filepath.Join(r, "steamapps", "libraryfolders.vdf"), filepath.Join(r, "config", "libraryfolders.vdf")} {
			if data, err := os.ReadFile(vdf); err == nil {
				for _, m := range reVdfPath.FindAllStringSubmatch(string(data), -1) {
					add(vdfUnescape(m[1]))
				}
			}
		}
	}
	return libs
}

// losslessInLibrary: Lossless.dll in one Steam library (by the app manifest, then the usual name).
func losslessInLibrary(lib string) string {
	dirs := []string{}
	if data, err := os.ReadFile(filepath.Join(lib, "steamapps", "appmanifest_"+losslessAppID+".acf")); err == nil {
		if m := reInstalDir.FindStringSubmatch(string(data)); m != nil {
			dirs = append(dirs, vdfUnescape(m[1]))
		}
	}
	dirs = append(dirs, "Lossless Scaling")
	for _, d := range dirs {
		if p := filepath.Join(lib, "steamapps", "common", d, "Lossless.dll"); fileExists(p) {
			return p
		}
	}
	return ""
}

// losslessInFolder: the user chose a folder. It can be the Lossless Scaling folder, a Steam
// library, Steam itself or a folder above them: look a few levels down.
func losslessInFolder(dir string) string {
	if p := filepath.Join(dir, "Lossless.dll"); fileExists(p) {
		return p
	}
	for _, lib := range steamLibraries([]string{dir}) {
		if p := losslessInLibrary(lib); p != "" {
			return p
		}
	}
	found := ""
	root := filepath.Clean(dir)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			if rel != "." && strings.Count(filepath.ToSlash(rel), "/") >= 4 {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(d.Name(), "Lossless.dll") {
			found = p
		}
		return nil
	})
	return found
}

// steamRootsFn finds this PC's Steam folders (lossless_windows.go / lossless_other.go); tests
// replace it.
var steamRootsFn = steamRoots

// FindLossless looks for Lossless Scaling in every Steam library of this PC.
func FindLossless() (path string, roots, libs []string) {
	roots = steamRootsFn()
	libs = steamLibraries(roots)
	for _, l := range libs {
		if p := losslessInLibrary(l); p != "" {
			return p, roots, libs
		}
	}
	return "", roots, libs
}

// ---------- setup

func (e *Emulator) losslessInstalledPath() string {
	return filepath.Join(e.losslessDir(), "Lossless.dll")
}

// LosslessStatus: the setup screen's state. folder, when set, is where the user pointed NEXUS.
func (e *Emulator) LosslessStatus(folder string) LosslessSetup {
	st := LosslessSetup{SteamRoots: []string{}, Libraries: []string{}}
	if !e.Enhancements().FrameGen.Supported {
		st.State, st.Message = "unsupported", "Frame generation needs a Vulkan graphics card that the NEXUS engine can use."
		return st
	}
	st.Installed = inspectLossless(e.losslessInstalledPath())
	var src string
	if folder != "" {
		src = losslessInFolder(folder)
		st.Searched = []string{folder}
	} else {
		src, st.SteamRoots, st.Libraries = FindLossless()
		st.Searched = append([]string{}, st.Libraries...)
	}
	if src != "" {
		st.Found = inspectLossless(src)
	}
	inst, found := st.Installed, st.Found
	switch {
	case inst != nil && inst.Valid && (found == nil || found.SHA256 == inst.SHA256):
		st.State, st.Message = "ready", "Lossless Scaling integration ready."
		if found == nil && folder == "" {
			st.Message += " (Lossless Scaling was not found in your Steam libraries now; the copy in NEXUS keeps working.)"
		}
	case inst != nil && inst.Valid && found != nil && found.Valid:
		st.State = "update"
		newer := "a different"
		if found.Modified > inst.Modified {
			newer = "a newer"
		}
		st.Message = fmt.Sprintf("Lossless Scaling integration ready. Your Lossless Scaling has %s Lossless.dll (%s) than the one in NEXUS (%s). Updating is optional: the one in NEXUS keeps working.",
			newer, time.Unix(found.Modified, 0).Format("2 Jan 2006"), time.Unix(inst.Modified, 0).Format("2 Jan 2006"))
	case found != nil && found.Valid:
		st.State, st.Message = "found", "Lossless Scaling detected."
	case found != nil:
		st.State, st.Message = "invalid", "Lossless Scaling was found, but its Lossless.dll cannot be used: "+found.Problem
	case folder != "":
		st.State, st.Message = "notfound", "Lossless.dll was not found in that folder. Choose the Lossless Scaling folder (in Steam: right-click Lossless Scaling › Manage › Browse local files), or your Steam folder."
	default:
		st.State, st.Message = "notfound", "Lossless Scaling files not found."
	}
	if inst != nil && !inst.Valid && st.State == "notfound" {
		st.Message += " The copy in NEXUS cannot be used either: " + inst.Problem
	}
	return st
}

// SetupLossless copies the checked Lossless.dll from the user's own installation into NEXUS.
// replace must be true to change a working copy that differs. Nothing is copied when the same
// file is already installed.
func (e *Emulator) SetupLossless(src string, replace bool) (LosslessSetup, error) {
	if src == "" {
		src, _, _ = FindLossless()
		if src == "" {
			return e.LosslessStatus(""), errors.New("Lossless Scaling files not found. Use “Locate Lossless Scaling installation” to choose its folder.")
		}
	}
	if fi, err := os.Stat(src); err == nil && fi.IsDir() {
		if p := losslessInFolder(src); p != "" {
			src = p
		} else {
			return e.LosslessStatus(src), errors.New("Lossless.dll was not found in that folder.")
		}
	}
	f := inspectLossless(src)
	if f == nil {
		return e.LosslessStatus(""), errors.New("That file does not exist.")
	}
	if !f.Valid {
		return e.LosslessStatus(""), errors.New(f.Problem)
	}
	inst := inspectLossless(e.losslessInstalledPath())
	if inst != nil && inst.SHA256 == f.SHA256 {
		return e.LosslessStatus(filepath.Dir(src)), nil // already installed: nothing to copy
	}
	if inst != nil && inst.Valid && !replace {
		return e.LosslessStatus(filepath.Dir(src)), errors.New("A working Lossless.dll is already installed. Choose Update to replace it with this one.")
	}
	if err := os.MkdirAll(e.losslessDir(), 0o755); err != nil {
		return LosslessSetup{}, err
	}
	tmp := e.losslessInstalledPath() + ".new"
	if err := copyFileOverwrite(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return LosslessSetup{}, err
	}
	if c := inspectLossless(tmp); c == nil || c.SHA256 != f.SHA256 {
		_ = os.Remove(tmp)
		return LosslessSetup{}, errors.New("The copy did not match the original; nothing was changed.")
	}
	// a cached translation of an older Lossless.dll must not be reused
	_ = os.Remove(filepath.Join(e.losslessDir(), "lsfg_spirv.cache"))
	if err := os.Rename(tmp, e.losslessInstalledPath()); err != nil {
		_ = os.Remove(tmp)
		return LosslessSetup{}, err
	}
	e.app.Log.Printf("frame generation: Lossless.dll (%d bytes, %s, %d shaders) copied from the user's Lossless Scaling", f.Size, f.Machine, f.Shaders)
	return e.LosslessStatus(filepath.Dir(src)), nil
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
