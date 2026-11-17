package main

// Portable shader data between your own devices (docs/SHADERS.md).
//
// A game's shader cache v2 log (<engine data>/cache/shader/<title id>/lichii-cache.v2.bin) holds
// records that do not depend on the GPU: the game's shader programs, the pipelines it used (render
// state and shader environments) and which pipelines each session used. Another device's engine can
// build those pipelines before play ("Building shaders from earlier sessions"). The driver's
// compiled code (lichii-cache.v2.driver.bin, vulkan_pipelines.*.bin, pipeline binaries) is not
// portable and is never exported.
//
// A shader pack is "NXSHPACK", u32 length, that many bytes of JSON (ShaderPackInfo), then a valid
// v2 log: the exporting device's live Program, Graphics, Compute and Used records, copied byte for
// byte (their payload hashes travel with them; the engine checks them when it loads the log).
//
// Importing never replaces anything: it adds the records whose kind and key this device does not
// have (or removed on purpose: a local tombstone wins), after checking the pack's checksum, title
// and log format. The new log is written to a temporary file and renamed over the old one, which is
// kept as a backup. Nothing is done while a game runs or shaders are being prepared. The pack holds
// the game's shader code, so it is not uploaded to the NEXUS Account: you move it yourself.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	shaderPackMagic   = "NXSHPACK"
	shaderPackFormat  = 1
	v2LogName         = "lichii-cache.v2.bin"
	v2FileMagic       = "LXSHCACH"
	v2FormatVersion   = 2
	v2FileHeaderSize  = 32
	v2RecordMagic     = 0x5243584C // "LXCR"
	v2RecordHeader    = 24
	v2RecordHash      = 8
	v2MaxPayload      = 256 << 20
	v2KindProgram     = 1
	v2KindGraphics    = 2
	v2KindCompute     = 3
	v2KindTombstone   = 4
	v2KindUsed        = 5
	maxShaderPackSize = 2 << 30
)

// ShaderPackInfo describes a shader pack.
type ShaderPackInfo struct {
	Format    int    `json:"format"`
	TitleID   string `json:"titleId"`
	Title     string `json:"title"`
	LogFormat uint32 `json:"logFormat"` // the v2 log's format version
	Programs  int    `json:"programs"`
	Graphics  int    `json:"graphics"`
	Compute   int    `json:"compute"`
	Sessions  int    `json:"sessions"` // Used records
	LogBytes  int64  `json:"logBytes"`
	SHA256    string `json:"sha256"` // of the log bytes
	Device    string `json:"device"`
	Nexus     string `json:"nexus"`
	Created   int64  `json:"created"`
}

// ShaderImportResult is what an import changed.
type ShaderImportResult struct {
	Title    string `json:"title"`
	Added    int    `json:"added"`   // records added
	Present  int    `json:"present"` // already on this device
	Removed  int    `json:"removed"` // removed on this device before (a local tombstone)
	Backup   string `json:"backup,omitempty"`
	LogBytes int64  `json:"logBytes"`
}

type v2Record struct {
	kind     uint16
	key      uint64
	off, end int // the whole framed record
}

type rk struct {
	kind uint16
	key  uint64
}

func v2RecordSize(payload uint32) int {
	return (v2RecordHeader + int(payload) + v2RecordHash + 7) / 8 * 8
}

// parseV2Log reads a log's framing (not the payload hashes: the engine checks those). validEnd is
// where the last whole record ends (a cut-off tail after it is dropped).
func parseV2Log(b []byte) (version uint32, recs []v2Record, validEnd int, err error) {
	if len(b) < v2FileHeaderSize || string(b[:8]) != v2FileMagic {
		return 0, nil, 0, errors.New("not a shader cache file")
	}
	if binary.LittleEndian.Uint32(b[24:]) != crc32.ChecksumIEEE(b[:24]) {
		return 0, nil, 0, errors.New("the shader cache file's header is damaged")
	}
	version = binary.LittleEndian.Uint32(b[8:])
	off := int(binary.LittleEndian.Uint32(b[12:]))
	if off < v2FileHeaderSize || off > len(b) {
		return 0, nil, 0, errors.New("the shader cache file's header is damaged")
	}
	validEnd = off
	for off+v2RecordHeader <= len(b) {
		h := b[off : off+v2RecordHeader]
		if binary.LittleEndian.Uint32(h) != v2RecordMagic || binary.LittleEndian.Uint32(h[20:]) != crc32.ChecksumIEEE(h[:20]) {
			off += 8 // like the engine: look for the next intact header
			continue
		}
		kind, size := binary.LittleEndian.Uint16(h[4:]), binary.LittleEndian.Uint32(h[8:])
		if kind == 0 || size > v2MaxPayload {
			off += 8
			continue
		}
		end := off + v2RecordSize(size)
		if end > len(b) {
			break // cut off
		}
		recs = append(recs, v2Record{kind: kind, key: binary.LittleEndian.Uint64(h[12:]), off: off, end: end})
		off, validEnd = end, end
	}
	return version, recs, validEnd, nil
}

// liveRecords: the latest record of each (kind, key), without those a later tombstone removed.
// tombstoned lists what tombstones removed.
func liveRecords(b []byte, recs []v2Record) (live map[rk]v2Record, order []rk, tombstoned map[rk]bool) {
	live, tombstoned = map[rk]v2Record{}, map[rk]bool{}
	for _, r := range recs {
		if r.kind == v2KindTombstone {
			if r.end-r.off >= v2RecordHeader+2 {
				target := rk{binary.LittleEndian.Uint16(b[r.off+v2RecordHeader:]), r.key}
				delete(live, target)
				tombstoned[target] = true
			}
			continue
		}
		k := rk{r.kind, r.key}
		if _, seen := live[k]; !seen {
			order = append(order, k)
		}
		live[k] = r
		delete(tombstoned, k)
	}
	return
}

func portableKind(k uint16) bool {
	return k == v2KindProgram || k == v2KindGraphics || k == v2KindCompute || k == v2KindUsed
}

func v2Header(created int64) []byte {
	h := make([]byte, v2FileHeaderSize)
	copy(h, v2FileMagic)
	binary.LittleEndian.PutUint32(h[8:], v2FormatVersion)
	binary.LittleEndian.PutUint32(h[12:], v2FileHeaderSize)
	binary.LittleEndian.PutUint64(h[16:], uint64(created))
	binary.LittleEndian.PutUint32(h[24:], crc32.ChecksumIEEE(h[:24]))
	return h
}

// ShaderLogPath is a game's shader cache v2 log.
func (e *Emulator) ShaderLogPath(titleID string) string {
	return filepath.Join(e.ShaderDir(), strings.ToLower(titleID), v2LogName)
}

func (e *Emulator) shaderFilesBusy() error {
	if e.State().Running {
		return errors.New("Close the running game first: its shader cache is in use.")
	}
	if e.PreparingKey() != "" {
		return errors.New("Shaders are being prepared; try again when that has finished.")
	}
	return nil
}

// ExportShaders writes a game's portable shader data as a shader pack to w. Returns its info.
func (e *Emulator) ExportShaders(g *Game, w io.Writer) (ShaderPackInfo, error) {
	info := ShaderPackInfo{Format: shaderPackFormat, TitleID: strings.ToUpper(g.TitleID), Title: g.Title,
		Device: defaultDeviceName(), Nexus: AppVersion, Created: time.Now().Unix()}
	if g.TitleID == "" {
		return info, errors.New("This game has no title ID.")
	}
	if err := e.shaderFilesBusy(); err != nil {
		return info, err
	}
	b, err := os.ReadFile(e.ShaderLogPath(g.TitleID))
	if errors.Is(err, os.ErrNotExist) {
		return info, errors.New("This game has no shader data on this device yet: play it (or prepare its shaders) first.")
	} else if err != nil {
		return info, err
	}
	ver, recs, _, err := parseV2Log(b)
	if err != nil {
		return info, err
	}
	if ver != v2FormatVersion {
		return info, fmt.Errorf("This game's shader cache uses format %d, which this NEXUS cannot export.", ver)
	}
	live, order, _ := liveRecords(b, recs)
	log := bytes.NewBuffer(v2Header(time.Now().Unix()))
	for _, k := range order {
		r, ok := live[k]
		if !ok || !portableKind(r.kind) {
			continue
		}
		log.Write(b[r.off:r.end])
		switch r.kind {
		case v2KindProgram:
			info.Programs++
		case v2KindGraphics:
			info.Graphics++
		case v2KindCompute:
			info.Compute++
		case v2KindUsed:
			info.Sessions++
		}
	}
	if info.Programs+info.Graphics+info.Compute == 0 {
		return info, errors.New("This game's shader cache has nothing to export yet.")
	}
	sum := sha256.Sum256(log.Bytes())
	info.LogFormat, info.LogBytes, info.SHA256 = v2FormatVersion, int64(log.Len()), hex.EncodeToString(sum[:])
	meta, _ := json.Marshal(info)
	var hdr [12]byte
	copy(hdr[:], shaderPackMagic)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(len(meta)))
	for _, part := range [][]byte{hdr[:], meta, log.Bytes()} {
		if _, err := w.Write(part); err != nil {
			return info, err
		}
	}
	e.app.Log.Printf("shaders: exported %s: %d programs, %d graphics, %d compute, %d sessions, %d bytes",
		info.TitleID, info.Programs, info.Graphics, info.Compute, info.Sessions, info.LogBytes)
	return info, nil
}

// readShaderPack reads and verifies a pack (everything is checked before anything is changed).
func readShaderPack(r io.Reader) (ShaderPackInfo, []byte, error) {
	var info ShaderPackInfo
	b, err := io.ReadAll(io.LimitReader(r, maxShaderPackSize+1))
	if err != nil {
		return info, nil, err
	}
	if len(b) > maxShaderPackSize {
		return info, nil, errors.New("The file is too large to be a shader pack.")
	}
	if len(b) < 12 || string(b[:8]) != shaderPackMagic {
		return info, nil, errors.New("This is not a NEXUS shader pack.")
	}
	n := int(binary.LittleEndian.Uint32(b[8:]))
	if n <= 0 || 12+n > len(b) || json.Unmarshal(b[12:12+n], &info) != nil {
		return info, nil, errors.New("The shader pack is damaged (its description cannot be read).")
	}
	if info.Format != shaderPackFormat {
		return info, nil, fmt.Errorf("The shader pack has format %d; this NEXUS reads format %d. Update NEXUS on this device.", info.Format, shaderPackFormat)
	}
	log := b[12+n:]
	if int64(len(log)) != info.LogBytes {
		return info, nil, fmt.Errorf("The shader pack is incomplete (%d of %d bytes): copy it again.", len(log), info.LogBytes)
	}
	if sum := sha256.Sum256(log); hex.EncodeToString(sum[:]) != info.SHA256 {
		return info, nil, errors.New("The shader pack is damaged (its checksum does not match): copy it again.")
	}
	if info.LogFormat != v2FormatVersion {
		return info, nil, fmt.Errorf("The shader pack's cache format is %d; this NEXUS engine uses %d, so it cannot use it.", info.LogFormat, v2FormatVersion)
	}
	return info, log, nil
}

// ImportShaders adds a pack's shader data to the game with the pack's title ID.
func (e *Emulator) ImportShaders(r io.Reader, games func(titleID string) *Game) (ShaderImportResult, error) {
	var res ShaderImportResult
	info, log, err := readShaderPack(r)
	if err != nil {
		return res, err
	}
	g := games(info.TitleID)
	if g == nil {
		return res, fmt.Errorf("%s (%s) is not in your library on this device.", firstNonEmpty(info.Title, "The pack's game"), info.TitleID)
	}
	res.Title = g.Title
	if err := e.shaderFilesBusy(); err != nil {
		return res, err
	}
	if _, _, _, err := parseV2Log(log); err != nil {
		return res, err
	}
	// added to the game's cache under the engine's lock, by appending (sharedcache.go): records
	// already there or removed there stay as they are, and a game another NEXUS user is playing
	// can go on writing
	path := e.ShaderLogPath(g.TitleID)
	added, present, removed, err := appendPortable(path, log)
	if err != nil {
		return res, fmt.Errorf("The shader data for %s was not imported; nothing was changed: %v", g.Title, err)
	}
	res.Added, res.Present, res.Removed = added, present, removed
	if fi, err := os.Stat(path); err == nil {
		res.LogBytes = fi.Size()
	}
	if res.Added == 0 {
		e.app.Log.Printf("shaders: import for %s added nothing (%d present, %d removed here)", g.TitleID, res.Present, res.Removed)
		return res, nil
	}
	e.app.Log.Printf("shaders: imported into %s: %d records added, %d present, %d removed here, from %s (%s)",
		g.TitleID, res.Added, res.Present, res.Removed, info.Device, info.Nexus)
	return res, nil
}

// shaderPackFileName: "<title> shaders [<title id>].nxshaders", without characters Windows refuses.
func shaderPackFileName(title, titleID string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return -1
		}
		return r
	}, title)
	return strings.TrimSpace(firstNonEmpty(clean, "Game")) + " shaders [" + titleID + "].nxshaders"
}
