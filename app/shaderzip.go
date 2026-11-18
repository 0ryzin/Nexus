package main

// Import Shaders from a ZIP (docs/SHADERS.md, "Importing shader caches from a ZIP").
//
// The ZIP only carries original shader-cache files; it is never used as a cache. NEXUS reads
// each file inside it (no extraction to disk: entries are read in memory, paths are never used
// to write anything), recognises the cache format from the file's own header, and converts only
// what the NEXUS engine can genuinely use:
//
//	LXSHCACH v2 log (lichii-cache.v2.bin: NEXUS, Lichii, Fritchii)  -> its portable records are
//	    added to the game's own log (the same merge as a shader pack: records this device has are
//	    kept, nothing replaced; the old log backed up)
//	NXSHPACK (a NEXUS shader pack)                                   -> imported as a pack
//	yuzucach, a version with the engine's record layout (54, 53, 52: NEXUS/Lichii fork disk
//	    caches, vulkan.fork-v<N>.bin)                               -> installed for the engine's
//	    own migration into the v2 log, only while the game has no v2 log yet (the engine starts a
//	    log from it at the next boot and never reads it again after that)
//
// and reports everything else with the reason, changing nothing for it:
//
//	yuzucach of another version (yuzu, Eden, Citron, Sudachi, older forks): another record layout
//	yuzuvkch (a Vulkan driver cache): compiled for one GPU and driver; not portable
//	Ryujinx / Ryubing caches (*.toc / *.data): Ryujinx's own format, not convertible
//	*.lock, *.tmp, and unknown files: not shader data
//
// The game is the one the import was started from; files under a folder named with another
// title ID are skipped.

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxShaderZipEntry = 1 << 30
	legacyCacheMagic  = "yuzucach"
	driverCacheMagic  = "yuzuvkch"
)

// the engine's guest cache versions with its record layout (pipeline_cache_files.h)
var engineLegacyVersions = map[uint32]bool{54: true, 53: true, 52: true}

var reTitleInPath = regexp.MustCompile(`(?i)(^|/)(0[0-9a-f]{15})(/|$)`)

// ShaderZipFile is the outcome for one file in the ZIP.
type ShaderZipFile struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Action string `json:"action"` // added | installed | skipped
	Reason string `json:"reason,omitempty"`
	Added  int    `json:"added,omitempty"`
}

type ShaderZipResult struct {
	Title    string          `json:"title"`
	Files    []ShaderZipFile `json:"files"`
	Added    int             `json:"added"`
	Usable   int             `json:"usable"` // files that were imported or installed
	LogBytes int64           `json:"logBytes"`
}

// ImportShaderZip imports the shader caches a ZIP carries into game g.
func (e *Emulator) ImportShaderZip(zipPath string, g *Game) (ShaderZipResult, error) {
	res := ShaderZipResult{Title: g.Title, Files: []ShaderZipFile{}}
	if err := e.shaderFilesBusy(); err != nil {
		return res, err
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return res, fmt.Errorf("This file is not a ZIP archive NEXUS can read (%v).", err)
	}
	defer zr.Close()
	seen := map[string]bool{}
	// older-format caches last: one is installed only while the game has no current-format log,
	// so a log elsewhere in the same archive must be imported first, whatever the archive order
	files := append([]*zip.File(nil), zr.File...)
	sort.SliceStable(files, func(i, j int) bool { return !isLegacyZipEntry(files[i]) && isLegacyZipEntry(files[j]) })
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(f.Name, "\\", "/")
		out := ShaderZipFile{Name: name, Action: "skipped"}
		add := func() { res.Files = append(res.Files, out) }
		base := strings.ToLower(path.Base(name))
		switch {
		case strings.Contains(name, "../") || strings.HasPrefix(name, "/"):
			out.Reason = "unsafe path in the archive; ignored"
			add()
			continue
		case base == shaderExportManifest:
			out.Action, out.Format = "checked", "NEXUS export description"
			out.Reason = "names the game, format and checksums; checked before importing"
			add()
			continue
		case strings.HasSuffix(base, ".lock") || strings.HasSuffix(base, ".tmp") || strings.HasPrefix(base, "."):
			out.Reason = "not shader data (lock or temporary file)"
			add()
			continue
		case strings.HasSuffix(base, ".toc") || strings.HasSuffix(base, ".data"):
			out.Format = "Ryujinx / Ryubing shader cache"
			out.Reason = "Ryujinx's own cache format: NEXUS cannot convert it (its shader and state records are not those of the NEXUS engine)"
			add()
			continue
		}
		if m := reTitleInPath.FindStringSubmatch(name); m != nil && !strings.EqualFold(m[2], g.TitleID) {
			out.Reason = fmt.Sprintf("for another game (%s)", strings.ToUpper(m[2]))
			add()
			continue
		}
		if f.UncompressedSize64 > maxShaderZipEntry {
			out.Reason = "larger than NEXUS accepts"
			add()
			continue
		}
		data, err := readZipEntry(f, maxShaderZipEntry)
		if err != nil {
			out.Reason = "could not be read from the archive: " + err.Error()
			add()
			continue
		}
		if len(data) < 12 {
			out.Reason = "not a shader cache NEXUS knows"
			add()
			continue
		}
		magic := string(data[:8])
		switch magic {
		case v2FileMagic:
			out.Format = "NEXUS / Lichii shader cache (v2 log)"
			if _, _, _, err := parseV2Log(data); err != nil {
				out.Reason = "damaged or another format version: " + err.Error()
				break
			}
			added, _, _, err := appendPortable(e.ShaderLogPath(g.TitleID), data)
			if err != nil {
				return res, fmt.Errorf("%s could not be added; the game's shader data was not changed: %v", name, err)
			}
			out.Action, out.Added = "added", added
			res.Added += added
			res.Usable++
		case shaderPackMagic:
			out.Format = "NEXUS shader pack"
			r, err := e.ImportShaders(bytes.NewReader(data), func(id string) *Game {
				if strings.EqualFold(id, g.TitleID) {
					return g
				}
				return nil
			})
			if err != nil {
				out.Reason = err.Error()
				break
			}
			out.Action, out.Added = "added", r.Added
			res.Added += r.Added
			res.Usable++
		case legacyCacheMagic:
			ver := binary.LittleEndian.Uint32(data[8:12])
			out.Format = fmt.Sprintf("yuzu-family shader cache, version %d", ver)
			if !engineLegacyVersions[ver] {
				out.Reason = fmt.Sprintf("version %d has another record layout than the NEXUS engine's (52-54): it comes from yuzu, Eden or another fork and cannot be converted", ver)
				break
			}
			if seen[fmt.Sprint(ver)] {
				out.Reason = "another copy of the same cache version is already in this archive"
				break
			}
			if fileExists(e.ShaderLogPath(g.TitleID)) {
				out.Reason = "this game already has shader data in the current format; the engine converts this older format only for a game that has none yet. Its pipelines are not added"
				break
			}
			dir := filepath.Dir(e.ShaderLogPath(g.TitleID))
			dst := filepath.Join(dir, fmt.Sprintf("vulkan.fork-v%d.bin", ver))
			if fileExists(dst) {
				out.Reason = "this device already has a cache of this version for the game; it was kept"
				break
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return res, err
			}
			tmp := dst + ".nexus-import"
			if err := os.WriteFile(tmp, data, 0o644); err != nil {
				return res, err
			}
			if err := os.Rename(tmp, dst); err != nil {
				os.Remove(tmp)
				return res, err
			}
			seen[fmt.Sprint(ver)] = true
			out.Action = "installed"
			out.Reason = "the engine converts it into the game's shader data at its next start"
			res.Usable++
		case driverCacheMagic:
			out.Format = "Vulkan driver cache"
			out.Reason = "compiled for one graphics card and driver; not portable, and rebuilt on this device by itself"
		default:
			out.Reason = "not a shader cache NEXUS knows"
		}
		add()
	}
	if fi, err := os.Stat(e.ShaderLogPath(g.TitleID)); err == nil {
		res.LogBytes = fi.Size()
	}
	e.app.Log.Printf("shaders: ZIP import for %s: %d file(s) usable, %d record(s) added", g.TitleID, res.Usable, res.Added)
	if len(res.Files) == 0 {
		return res, errors.New("The ZIP archive is empty.")
	}
	return res, nil
}

func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("too large")
	}
	return b, nil
}

// isLegacyZipEntry: the entry starts like an older-format (yuzu-family) shader cache.
func isLegacyZipEntry(f *zip.File) bool {
	if f.FileInfo().IsDir() || f.UncompressedSize64 < 12 || f.UncompressedSize64 > maxShaderZipEntry {
		return false
	}
	rc, err := f.Open()
	if err != nil {
		return false
	}
	defer rc.Close()
	head := make([]byte, len(legacyCacheMagic))
	if _, err := io.ReadFull(rc, head); err != nil {
		return false
	}
	return string(head) == legacyCacheMagic
}
