package main

// Read-only parsers for Switch container formats, used only to identify games
// (title ID, name, icon) for the library. NEXUS never modifies game files.
//
//   NSP/NSZ  = PFS0 container of NCA/NCZ files (+ optional .tik tickets)
//   XCI/XCZ  = game-card image: header + HFS0 partitions ("secure" holds NCAs)
//   NRO      = homebrew executable with an optional ASET block (icon + NACP)
//
// NCA headers are AES-128-XTS encrypted with header_key from the user's own
// prod.keys. Without keys, NEXUS falls back to tickets and file names.

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type KeySet map[string][]byte

func LoadKeys(paths ...string) KeySet {
	ks := KeySet{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			b, err := hex.DecodeString(strings.TrimSpace(v))
			if err == nil {
				ks[strings.ToLower(strings.TrimSpace(k))] = b
			}
		}
		f.Close()
	}
	return ks
}

// GameInfo is everything the scanner could learn about one file.
type GameInfo struct {
	TitleID   string // 16 upper-case hex chars, base application ID
	FullID    string // exact program ID of the main content (update/DLC keep their suffix)
	Kind      string // base | update | dlc | homebrew | unknown
	Name      string
	Publisher string
	Version   string
	Icon      []byte // JPEG (Switch control data) or another image format (NUP metadata)
	Source    string // how the ID was obtained
}

func classifyID(id uint64) (base uint64, kind string) {
	switch {
	case id&0x1FFF == 0:
		return id, "base"
	case id&0x1FFF == 0x800:
		return id &^ 0x1FFF, "update"
	case id&0x1000 != 0:
		return (id &^ 0x1FFF), "dlc"
	default:
		return id &^ 0x1FFF, "base" // extra program in a multi-program title
	}
}

var reTitleID = regexp.MustCompile(`(?i)\b(01[0-9a-f]{14})\b`)
var reVersion = regexp.MustCompile(`(?i)\[v(\d+)\]`)

// ---------- generic container parsing ----------

type pfsEntry struct {
	Name   string
	Offset int64 // absolute offset within the file
	Size   int64
}

func readAt(r io.ReaderAt, off int64, n int) ([]byte, error) {
	buf := make([]byte, n)
	m, err := r.ReadAt(buf, off)
	if m == n {
		return buf, nil
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}
	return nil, err
}

// parsePartition reads a PFS0 or HFS0 header located at base.
func parsePartition(r io.ReaderAt, base int64) ([]pfsEntry, error) {
	hdr, err := readAt(r, base, 0x10)
	if err != nil {
		return nil, err
	}
	magic := string(hdr[0:4])
	var entrySize int64
	switch magic {
	case "PFS0":
		entrySize = 0x18
	case "HFS0":
		entrySize = 0x40
	default:
		return nil, fmt.Errorf("not a PFS0/HFS0 partition (%q)", magic)
	}
	n := int64(binary.LittleEndian.Uint32(hdr[4:]))
	strSize := int64(binary.LittleEndian.Uint32(hdr[8:]))
	if n <= 0 || n > 4096 || strSize > 1<<20 {
		return nil, errors.New("implausible partition header")
	}
	table, err := readAt(r, base+0x10, int(n*entrySize+strSize))
	if err != nil {
		return nil, err
	}
	strs := table[n*entrySize:]
	dataStart := base + 0x10 + n*entrySize + strSize
	out := make([]pfsEntry, 0, n)
	for i := int64(0); i < n; i++ {
		e := table[i*entrySize:]
		off := int64(binary.LittleEndian.Uint64(e[0:]))
		size := int64(binary.LittleEndian.Uint64(e[8:]))
		no := int64(binary.LittleEndian.Uint32(e[16:]))
		if no >= int64(len(strs)) {
			continue
		}
		name := strs[no:]
		if z := bytes.IndexByte(name, 0); z >= 0 {
			name = name[:z]
		}
		out = append(out, pfsEntry{Name: string(name), Offset: dataStart + off, Size: size})
	}
	return out, nil
}

// xciSecureEntries returns the NCA entries of a game-card image.
func xciSecureEntries(r io.ReaderAt) ([]pfsEntry, error) {
	for _, base := range []int64{0, 0x1000} { // some dumps prepend a 0x1000 key area
		h, err := readAt(r, base+0x100, 0x40)
		if err != nil || string(h[0:4]) != "HEAD" {
			continue
		}
		rootOff := int64(binary.LittleEndian.Uint64(h[0x30:]))
		root, err := parsePartition(r, base+rootOff)
		if err != nil {
			return nil, err
		}
		for _, p := range root {
			if p.Name == "secure" {
				return parsePartition(r, p.Offset)
			}
		}
		return nil, errors.New("XCI has no secure partition")
	}
	return nil, errors.New("not an XCI (no HEAD magic)")
}

// ---------- AES helpers ----------

func xtsGFMul(t []byte) {
	carry := byte(0)
	for i := 0; i < 16; i++ {
		next := t[i] >> 7
		t[i] = t[i]<<1 | carry
		carry = next
	}
	if carry != 0 {
		t[0] ^= 0x87
	}
}

// nintendoXTS decrypts (or encrypts) data in 0x200-byte sectors using the
// big-endian sector-number tweak that Nintendo's NCA headers use.
func nintendoXTS(key []byte, data []byte, firstSector uint64, decrypt bool) error {
	if len(key) != 32 || len(data)%0x200 != 0 {
		return errors.New("bad XTS key or length")
	}
	k1, _ := aes.NewCipher(key[:16])
	k2, _ := aes.NewCipher(key[16:])
	for s := 0; s*0x200 < len(data); s++ {
		tweak := make([]byte, 16)
		binary.BigEndian.PutUint64(tweak[8:], firstSector+uint64(s))
		k2.Encrypt(tweak, tweak)
		sec := data[s*0x200 : (s+1)*0x200]
		for b := 0; b < 0x200; b += 16 {
			blk := sec[b : b+16]
			for i := range blk {
				blk[i] ^= tweak[i]
			}
			if decrypt {
				k1.Decrypt(blk, blk)
			} else {
				k1.Encrypt(blk, blk)
			}
			for i := range blk {
				blk[i] ^= tweak[i]
			}
			xtsGFMul(tweak)
		}
	}
	return nil
}

func aesECBDecrypt(key, data []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	for i := 0; i+16 <= len(data); i += 16 {
		c.Decrypt(out[i:i+16], data[i:i+16])
	}
	return out, nil
}

// ---------- NCA ----------

type ncaHeader struct {
	raw         []byte // decrypted 0xC00 header
	ContentType byte
	ProgramID   uint64
	KeyGen      int
	RightsID    []byte
}

func (h *ncaHeader) hasRightsID() bool {
	for _, b := range h.RightsID {
		if b != 0 {
			return true
		}
	}
	return false
}

func readNCAHeader(r io.ReaderAt, off int64, ks KeySet) (*ncaHeader, error) {
	key := ks["header_key"]
	if len(key) != 32 {
		return nil, errors.New("header_key missing from prod.keys")
	}
	raw, err := readAt(r, off, 0xC00)
	if err != nil {
		return nil, err
	}
	if err := nintendoXTS(key, raw, 0, true); err != nil {
		return nil, err
	}
	magic := string(raw[0x200:0x204])
	if magic != "NCA3" && magic != "NCA2" {
		return nil, fmt.Errorf("NCA header did not decrypt (got %q) - wrong keys?", magic)
	}
	h := &ncaHeader{raw: raw}
	h.ContentType = raw[0x205]
	h.ProgramID = binary.LittleEndian.Uint64(raw[0x210:])
	kg := int(raw[0x206])
	if int(raw[0x220]) > kg {
		kg = int(raw[0x220])
	}
	if kg > 0 {
		kg--
	}
	h.KeyGen = kg
	h.RightsID = raw[0x230:0x240]
	return h, nil
}

// sectionReader decrypts an AES-CTR NCA section on the fly.
type sectionReader struct {
	r      io.ReaderAt
	start  int64 // absolute file offset of the section
	ncaOff int64 // absolute file offset of the NCA (the CTR counts from the NCA start)
	key    []byte
	ctrHi  []byte // upper 8 bytes of the counter
	plain  bool
}

func (s *sectionReader) ReadAt(p []byte, off int64) (int, error) {
	abs := s.start + off
	if s.plain {
		return s.r.ReadAt(p, abs)
	}
	alignedStart := abs - ((abs - s.ncaOff) & 0xF)
	pad := int(abs - alignedStart)
	buf := make([]byte, (pad+len(p)+15)&^15)
	n, err := s.r.ReadAt(buf, alignedStart)
	if n < pad+len(p) && err != nil {
		if n <= pad {
			return 0, err
		}
	}
	block, err2 := aes.NewCipher(s.key)
	if err2 != nil {
		return 0, err2
	}
	iv := make([]byte, 16)
	copy(iv, s.ctrHi)
	binary.BigEndian.PutUint64(iv[8:], uint64((alignedStart-s.ncaOff)>>4))
	cipher.NewCTR(block, iv).XORKeyStream(buf, buf)
	c := copy(p, buf[pad:])
	if c < len(p) {
		return c, io.ErrUnexpectedEOF
	}
	return c, nil
}

func (h *ncaHeader) sectionKey(ks KeySet, tickets map[string][]byte) ([]byte, error) {
	if h.hasRightsID() {
		enc, ok := tickets[strings.ToLower(hex.EncodeToString(h.RightsID))]
		if !ok {
			return nil, errors.New("title key not available (no common ticket)")
		}
		kek := ks[fmt.Sprintf("titlekek_%02x", h.KeyGen)]
		if len(kek) != 16 {
			return nil, fmt.Errorf("titlekek_%02x missing", h.KeyGen)
		}
		return aesECBDecrypt(kek, enc)
	}
	kind := [...]string{"application", "ocean", "system"}[min(int(h.raw[0x207]), 2)]
	kakName := fmt.Sprintf("key_area_key_%s_%02x", kind, h.KeyGen)
	kak := ks[kakName]
	if len(kak) != 16 {
		return nil, fmt.Errorf("%s missing from prod.keys", kakName)
	}
	keys, err := aesECBDecrypt(kak, h.raw[0x300:0x340])
	if err != nil {
		return nil, err
	}
	return keys[0x20:0x30], nil // key index 2 = AES-CTR
}

// openRomFS returns a reader over section 0's RomFS data level.
func (h *ncaHeader) openRomFS(r io.ReaderAt, ncaOff int64, ks KeySet, tickets map[string][]byte) (io.ReaderAt, error) {
	fs := h.raw[0x400:0x600] // fs header of section 0
	start := int64(binary.LittleEndian.Uint32(h.raw[0x240:])) * 0x200
	if start == 0 {
		return nil, errors.New("section 0 empty")
	}
	if fs[0x2] != 0 || string(fs[0x8:0xC]) != "IVFC" {
		return nil, errors.New("section 0 is not an IVFC RomFS")
	}
	levelOff := int64(binary.LittleEndian.Uint64(fs[0x90:]))
	sr := &sectionReader{r: r, start: ncaOff + start, ncaOff: ncaOff}
	switch fs[0x4] {
	case 1:
		sr.plain = true
	case 3:
		key, err := h.sectionKey(ks, tickets)
		if err != nil {
			return nil, err
		}
		sr.key = key
		sr.ctrHi = make([]byte, 8)
		for j := 0; j < 8; j++ {
			sr.ctrHi[j] = fs[0x140+7-j]
		}
	default:
		return nil, fmt.Errorf("unsupported section crypto %d", fs[0x4])
	}
	return io.NewSectionReader(sr, levelOff, 1<<40), nil
}

// romfsFiles returns the contents of root-level files in a RomFS image.
func romfsFiles(r io.ReaderAt, want func(name string) bool, limit int64) (map[string][]byte, error) {
	return romfsDirFiles(r, "", want, limit)
}

// romfsDirFiles returns files directly inside the top-level directory dir ("" = root).
func romfsDirFiles(r io.ReaderAt, dir string, want func(name string) bool, limit int64) (map[string][]byte, error) {
	hdr, err := readAt(r, 0, 0x50)
	if err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint64(hdr[0:]) != 0x50 {
		return nil, errors.New("bad RomFS header")
	}
	dirMetaOff := int64(binary.LittleEndian.Uint64(hdr[0x18:]))
	dirMetaSize := int64(binary.LittleEndian.Uint64(hdr[0x20:]))
	fileMetaOff := int64(binary.LittleEndian.Uint64(hdr[0x38:]))
	fileMetaSize := int64(binary.LittleEndian.Uint64(hdr[0x40:]))
	dataOff := int64(binary.LittleEndian.Uint64(hdr[0x48:]))
	if fileMetaSize <= 0 || fileMetaSize > 8<<20 || dirMetaSize > 8<<20 {
		return nil, errors.New("implausible RomFS file table")
	}
	parentWanted := uint32(0)
	if dir != "" {
		dm, err := readAt(r, dirMetaOff, int(dirMetaSize))
		if err != nil {
			return nil, err
		}
		found := false
		for p := 0; p+0x18 <= len(dm); {
			e := dm[p:]
			parent := binary.LittleEndian.Uint32(e[0:])
			nameLen := int(binary.LittleEndian.Uint32(e[20:]))
			if 0x18+nameLen > len(e) {
				break
			}
			if parent == 0 && p != 0 && string(e[0x18:0x18+nameLen]) == dir {
				parentWanted, found = uint32(p), true
				break
			}
			p += (0x18 + nameLen + 3) &^ 3
		}
		if !found {
			return nil, fmt.Errorf("RomFS has no %q directory", dir)
		}
	}
	meta, err := readAt(r, fileMetaOff, int(fileMetaSize))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for p := 0; p+0x20 <= len(meta); {
		e := meta[p:]
		parent := binary.LittleEndian.Uint32(e[0:])
		off := int64(binary.LittleEndian.Uint64(e[8:]))
		size := int64(binary.LittleEndian.Uint64(e[16:]))
		nameLen := int(binary.LittleEndian.Uint32(e[28:]))
		if 0x20+nameLen > len(e) {
			break
		}
		name := string(e[0x20 : 0x20+nameLen])
		if parent == parentWanted && want(name) && size > 0 && size <= limit {
			if data, err := readAt(r, dataOff+off, int(size)); err == nil {
				out[name] = data
			}
		}
		p += (0x20 + nameLen + 3) &^ 3
	}
	return out, nil
}

// DecompressYaz0 decodes Nintendo's Yaz0 LZ format (same algorithm as Lichii's
// new_user_dialog.cpp, used for the firmware avatar images).
func DecompressYaz0(in []byte) ([]byte, error) {
	if len(in) < 16 || string(in[0:4]) != "Yaz0" {
		return nil, errors.New("not Yaz0 data")
	}
	size := int(binary.BigEndian.Uint32(in[4:]))
	if size <= 0 || size > 64<<20 {
		return nil, errors.New("implausible Yaz0 size")
	}
	out := make([]byte, 0, size)
	src := in[16:]
	i := 0
	var header byte
	bits := 0
	for len(out) < size {
		if bits == 0 {
			if i >= len(src) {
				return nil, io.ErrUnexpectedEOF
			}
			header, bits = src[i], 8
			i++
		}
		if header&0x80 != 0 {
			if i >= len(src) {
				return nil, io.ErrUnexpectedEOF
			}
			out = append(out, src[i])
			i++
		} else {
			if i+1 >= len(src) {
				return nil, io.ErrUnexpectedEOF
			}
			b1, b2 := src[i], src[i+1]
			i += 2
			dist := int(b1&0xF)<<8 | int(b2)
			length := int(b1 >> 4)
			if length == 0 {
				if i >= len(src) {
					return nil, io.ErrUnexpectedEOF
				}
				length = int(src[i]) + 0x12
				i++
			} else {
				length += 2
			}
			pos := len(out) - dist - 1
			if pos < 0 {
				return nil, errors.New("corrupt Yaz0 back-reference")
			}
			for k := 0; k < length && len(out) < size; k++ {
				out = append(out, out[pos+k])
			}
		}
		header <<= 1
		bits--
	}
	return out, nil
}

// parseNACPPublisher returns the publisher of the first language that has a name.
func parseNACPPublisher(nacp []byte) string {
	if len(nacp) < 0x4000 {
		return ""
	}
	for lang := 0; lang < 16; lang++ {
		e := nacp[lang*0x300 : lang*0x300+0x300]
		if z := bytes.IndexByte(e[:0x200], 0); z > 0 {
			p := e[0x200:]
			if z := bytes.IndexByte(p, 0); z >= 0 {
				p = p[:z]
			}
			return strings.TrimSpace(string(p))
		}
	}
	return ""
}

func parseNACPName(nacp []byte) (name, version string) {
	if len(nacp) < 0x4000 {
		return "", ""
	}
	for lang := 0; lang < 16; lang++ { // AmericanEnglish first, then any language with a name
		e := nacp[lang*0x300 : lang*0x300+0x200]
		if z := bytes.IndexByte(e, 0); z > 0 {
			name = strings.TrimSpace(string(e[:z]))
			break
		}
	}
	v := nacp[0x3060 : 0x3060+0x10]
	if z := bytes.IndexByte(v, 0); z >= 0 {
		v = v[:z]
	}
	return name, string(v)
}

// ---------- top level ----------

// IdentifyFile inspects one game file. Errors are non-fatal: whatever was
// learned (often just the name from the file name) is still returned.
func IdentifyFile(path string, ks KeySet) (GameInfo, error) {
	info := GameInfo{Kind: "unknown"}
	base := strings.TrimSuffix(pathBase(path), pathExt(path))
	if m := reTitleID.FindStringSubmatch(base); m != nil {
		id := parseHexID(m[1])
		b, kind := classifyID(id)
		info.TitleID, info.FullID, info.Kind, info.Source = fmtID(b), fmtID(id), kind, "filename"
	}
	if m := reVersion.FindStringSubmatch(base); m != nil {
		info.Version = "v" + m[1]
	}
	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()
	ext := strings.ToLower(strings.TrimPrefix(pathExt(path), "."))
	switch ext {
	case "nro":
		return identifyNRO(f, info)
	case "nup":
		fi, err := f.Stat()
		if err != nil {
			return info, err
		}
		return identifyNUP(f, fi.Size(), info)
	case "nsp", "nsz":
		entries, err := parsePartition(f, 0)
		if err != nil {
			return info, err
		}
		return identifyEntries(f, entries, ks, info)
	case "xci", "xcz":
		entries, err := xciSecureEntries(f)
		if err != nil {
			return info, err
		}
		return identifyEntries(f, entries, ks, info)
	}
	return info, nil
}

func identifyEntries(f io.ReaderAt, entries []pfsEntry, ks KeySet, info GameInfo) (GameInfo, error) {
	tickets := map[string][]byte{}
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name), ".tik") && e.Size >= 0x2C0 {
			tik, err := readAt(f, e.Offset, 0x2C0)
			if err != nil {
				continue
			}
			rid := strings.ToLower(hex.EncodeToString(tik[0x2A0:0x2B0]))
			if tik[0x281] == 0 { // common ticket: title key block holds the AES-wrapped key
				tickets[rid] = append([]byte(nil), tik[0x180:0x190]...)
			}
			if info.Source == "" || info.Source == "filename" {
				id := parseHexID(rid[:16])
				b, kind := classifyID(id)
				info.TitleID, info.FullID, info.Kind, info.Source = fmtID(b), fmtID(id), kind, "ticket"
			}
		}
	}
	if len(ks["header_key"]) != 32 {
		if info.TitleID == "" {
			return info, errors.New("no prod.keys header_key: title could not be identified")
		}
		return info, nil
	}
	var control *ncaHeader
	var controlOff int64
	var lastErr error
	for _, e := range entries {
		ln := strings.ToLower(e.Name)
		if !(strings.HasSuffix(ln, ".nca") || strings.HasSuffix(ln, ".ncz")) || e.Size < 0xC00 {
			continue
		}
		h, err := readNCAHeader(f, e.Offset, ks)
		if err != nil {
			lastErr = err
			continue
		}
		switch h.ContentType {
		case 0: // Program
			b, kind := classifyID(h.ProgramID)
			if info.Source != "nca" || kind == "base" {
				info.TitleID, info.FullID, info.Kind, info.Source = fmtID(b), fmtID(h.ProgramID), kind, "nca"
			}
		case 2: // Control
			if strings.HasSuffix(ln, ".nca") && control == nil {
				control, controlOff = h, e.Offset
			}
		case 5: // PublicData (DLC)
			if info.Source != "nca" {
				b, _ := classifyID(h.ProgramID)
				info.TitleID, info.FullID, info.Kind, info.Source = fmtID(b), fmtID(h.ProgramID), "dlc", "nca"
			}
		}
	}
	if control != nil {
		if info.TitleID == "" {
			b, kind := classifyID(control.ProgramID)
			info.TitleID, info.FullID, info.Kind, info.Source = fmtID(b), fmtID(control.ProgramID), kind, "nca"
		}
		rom, err := control.openRomFS(f, controlOff, ks, tickets)
		if err == nil {
			files, err := romfsFiles(rom, func(n string) bool {
				ln := strings.ToLower(n)
				return ln == "control.nacp" || strings.HasPrefix(ln, "icon_")
			}, 8<<20)
			if err == nil {
				nacp, ok := files["control.nacp"]
				if !ok {
					nacp, ok = files["Control.nacp"]
				}
				if ok {
					if n, v := parseNACPName(nacp); n != "" {
						info.Name = n
						info.Publisher = parseNACPPublisher(nacp)
						if v != "" {
							info.Version = v
						}
					}
				}
				for _, lang := range []string{"AmericanEnglish", "BritishEnglish", "Japanese", "French", "German"} {
					if ic, ok := files["icon_"+lang+".dat"]; ok && isImage(ic) {
						info.Icon = ic
						break
					}
				}
				if info.Icon == nil {
					for n, d := range files {
						if strings.HasPrefix(strings.ToLower(n), "icon_") && isImage(d) {
							info.Icon = d
							break
						}
					}
				}
			} else {
				lastErr = err
			}
		} else {
			lastErr = err
		}
	}
	if info.TitleID == "" && lastErr != nil {
		return info, lastErr
	}
	return info, nil
}

func identifyNRO(f io.ReaderAt, info GameInfo) (GameInfo, error) {
	info.Kind = "homebrew"
	h, err := readAt(f, 0x10, 0x10)
	if err != nil || string(h[0:4]) != "NRO0" {
		return info, errors.New("not an NRO")
	}
	size := int64(binary.LittleEndian.Uint32(h[8:]))
	a, err := readAt(f, size, 0x38)
	if err != nil || string(a[0:4]) != "ASET" {
		return info, nil // homebrew without assets
	}
	iconOff, iconSize := int64(binary.LittleEndian.Uint64(a[8:])), int64(binary.LittleEndian.Uint64(a[16:]))
	nacpOff, nacpSize := int64(binary.LittleEndian.Uint64(a[24:])), int64(binary.LittleEndian.Uint64(a[32:]))
	if iconSize > 0 && iconSize < 4<<20 {
		if ic, err := readAt(f, size+iconOff, int(iconSize)); err == nil {
			info.Icon = ic
		}
	}
	if nacpSize >= 0x4000 && nacpSize < 1<<20 {
		if nacp, err := readAt(f, size+nacpOff, int(nacpSize)); err == nil {
			info.Name, info.Version = parseNACPName(nacp)
		}
	}
	info.Source = "nro"
	return info, nil
}

func parseHexID(s string) uint64 {
	var v uint64
	_, _ = fmt.Sscanf(s, "%x", &v)
	return v
}

func fmtID(v uint64) string { return fmt.Sprintf("%016X", v) }

func pathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func pathExt(p string) string {
	b := pathBase(p)
	if i := strings.LastIndex(b, "."); i >= 0 {
		return b[i:]
	}
	return ""
}

// CleanTitle turns "Some Game [0100ABCD...][v0] (USA).nsp" into "Some Game".
func CleanTitle(file string) string {
	s := strings.TrimSuffix(pathBase(file), pathExt(file))
	s = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|\{[^}]*\}`).ReplaceAllString(s, " ")
	s = strings.NewReplacer("_", " ", ".", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		s = strings.TrimSuffix(pathBase(file), pathExt(file))
	}
	return s
}

type romfsEntry struct {
	Path string // "dir/sub/name.ext"
	Off  int64  // absolute offset inside the RomFS image
	Size int64
}

// romfsWalk lists every file in a RomFS image by following the directory tree
// (dir entry: parent, sibling, child dir, child file, hash, name len, name;
// file entry: parent, sibling, offset, size, hash, name len, name).
func romfsWalk(r io.ReaderAt) ([]romfsEntry, error) {
	hdr, err := readAt(r, 0, 0x50)
	if err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint64(hdr[0:]) != 0x50 {
		return nil, errors.New("bad RomFS header")
	}
	dmOff, dmSize := int64(binary.LittleEndian.Uint64(hdr[0x18:])), int64(binary.LittleEndian.Uint64(hdr[0x20:]))
	fmOff, fmSize := int64(binary.LittleEndian.Uint64(hdr[0x38:])), int64(binary.LittleEndian.Uint64(hdr[0x40:]))
	dataOff := int64(binary.LittleEndian.Uint64(hdr[0x48:]))
	if dmSize <= 0 || fmSize < 0 || dmSize > 16<<20 || fmSize > 16<<20 {
		return nil, errors.New("implausible RomFS tables")
	}
	dm, err := readAt(r, dmOff, int(dmSize))
	if err != nil {
		return nil, err
	}
	fm := []byte{}
	if fmSize > 0 {
		if fm, err = readAt(r, fmOff, int(fmSize)); err != nil {
			return nil, err
		}
	}
	const none = 0xFFFFFFFF
	var out []romfsEntry
	seen := map[uint32]bool{}
	var walkDir func(off uint32, prefix string, depth int)
	walkDir = func(off uint32, prefix string, depth int) {
		for off != none && int(off)+0x18 <= len(dm) && !seen[off] && depth < 16 {
			seen[off] = true
			e := dm[off:]
			sibling := binary.LittleEndian.Uint32(e[4:])
			childDir := binary.LittleEndian.Uint32(e[8:])
			childFile := binary.LittleEndian.Uint32(e[12:])
			nl := int(binary.LittleEndian.Uint32(e[20:]))
			name := ""
			if nl > 0 && 0x18+nl <= len(e) {
				name = string(e[0x18 : 0x18+nl])
			}
			p := prefix
			if off != 0 {
				p = prefix + name + "/"
			}
			for f := childFile; f != none && int(f)+0x20 <= len(fm); {
				fe := fm[f:]
				fl := int(binary.LittleEndian.Uint32(fe[28:]))
				if 0x20+fl > len(fe) {
					break
				}
				out = append(out, romfsEntry{Path: p + string(fe[0x20:0x20+fl]),
					Off: dataOff + int64(binary.LittleEndian.Uint64(fe[8:])), Size: int64(binary.LittleEndian.Uint64(fe[16:]))})
				next := binary.LittleEndian.Uint32(fe[4:])
				if next == f {
					break
				}
				f = next
			}
			walkDir(childDir, p, depth+1)
			if off == 0 {
				return // root has no siblings
			}
			off = sibling
		}
	}
	walkDir(0, "", 0)
	return out, nil
}
