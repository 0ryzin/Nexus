package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"strings"
)

// NUP ("NX Package") is the Lichii engine's container for Switch 2 games: loose ExeFS/RomFS trees
// of a base program, its updates and add-on content in one file. Each package carries metadata
// (TLV records) with the title name, publisher, display version and icons, which is what the NEXUS
// library needs. Layout (from the engine's file_sys/nup_container.cpp, all little-endian):
//
//	superblock 0x80: magic "NUP\x01", header_size u16, header_version u16, toc_offset u64,
//	  toc_size u64, toc_sha256 [32], blob_offset u64, file_size u64, package_count u32, flags u32 …
//	TOC: header 0x30 ("TOC\x01", package_count u32, dir_count u64, file_count u64, pool_offset u64,
//	  pool_size u64, reserved u64), then package records of 0x40 bytes:
//	  title_id u64, root/exefs/romfs dir u32×3, version u32, kind u8 (0 base, 1 update, 2 add-on),
//	  flags u8, …, meta_offset u64 @0x20, meta_size u32 @0x28
//	meta: TLVs (type u16, reserved u16, size u32, payload)

const (
	nupSuperSize   = 0x80
	nupTocHdrSize  = 0x30
	nupPackageSize = 0x40
	nupMetaIcon    = 0x0003
	nupMetaVersion = 0x0004
	nupMetaTitle   = 0x0005
	nupMetaPub     = 0x0006
	nupMetaLTitles = 0x0009
	nupMetaLIcons  = 0x000A
)

// NupPackage is one program inside a NUP file.
type NupPackage struct {
	TitleID        uint64
	Version        uint32
	Kind           string // base | update | dlc
	Name           string
	Publisher      string
	DisplayVersion string
	Icon           []byte
	LocalIcons     map[string][]byte
	LocalTitles    map[string]string
}

var errNotNup = errors.New("not a NUP package")

// ReadNupPackages reads the package list and metadata of a NUP file (never its game data).
func ReadNupPackages(r io.ReaderAt, size int64) ([]NupPackage, error) {
	sb, err := readAt(r, 0, nupSuperSize)
	if err != nil || !bytes.Equal(sb[:4], []byte{'N', 'U', 'P', 1}) {
		return nil, errNotNup
	}
	le := binary.LittleEndian
	tocOff, tocSize := int64(le.Uint64(sb[8:])), int64(le.Uint64(sb[16:]))
	blobOff, fileSize := int64(le.Uint64(sb[56:])), int64(le.Uint64(sb[64:]))
	count := le.Uint32(sb[72:])
	if fileSize != size || tocOff < nupSuperSize || tocOff > blobOff || tocSize > blobOff-tocOff || tocSize > 256<<20 || count == 0 {
		return nil, errors.New("the NUP package header is damaged")
	}
	toc, err := readAt(r, tocOff, int(tocSize))
	if err != nil {
		return nil, err
	}
	if sum := sha256.Sum256(toc); !bytes.Equal(sum[:], sb[24:56]) {
		return nil, errors.New("the NUP package is damaged (its table of contents does not match its checksum)")
	}
	if len(toc) < nupTocHdrSize || le.Uint32(toc[0:]) != 0x01434F54 || le.Uint32(toc[4:]) != count {
		return nil, errors.New("the NUP package table is damaged")
	}
	if int64(nupTocHdrSize)+int64(count)*nupPackageSize > int64(len(toc)) {
		return nil, errors.New("the NUP package table is damaged")
	}
	var out []NupPackage
	for i := 0; i < int(count); i++ {
		rec := toc[nupTocHdrSize+i*nupPackageSize:]
		p := NupPackage{TitleID: le.Uint64(rec[0:]), Version: le.Uint32(rec[0x14:])}
		switch rec[0x18] {
		case 0:
			p.Kind = "base"
		case 1:
			p.Kind = "update"
		case 2:
			p.Kind = "dlc"
		default:
			return nil, errors.New("the NUP package lists an unknown content type")
		}
		metaOff, metaSize := int64(le.Uint64(rec[0x20:])), int64(le.Uint32(rec[0x28:]))
		if metaOff > 0 && metaSize > 0 && metaOff <= size && metaSize <= size-metaOff && metaSize <= 64<<20 {
			meta, err := readAt(r, metaOff, int(metaSize))
			if err != nil {
				return nil, err
			}
			if err := p.parseMeta(meta); err != nil {
				return nil, err
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func (p *NupPackage) parseMeta(b []byte) error {
	le := binary.LittleEndian
	for pos := 0; pos+8 <= len(b); {
		typ, n := le.Uint16(b[pos:]), int(le.Uint32(b[pos+4:]))
		pos += 8
		if n < 0 || pos+n > len(b) {
			return errors.New("the NUP package metadata is damaged")
		}
		v := b[pos : pos+n]
		pos += n
		switch typ {
		case nupMetaIcon:
			p.Icon = append([]byte(nil), v...)
		case nupMetaTitle:
			p.Name = strings.TrimRight(string(v), "\x00")
		case nupMetaPub:
			p.Publisher = strings.TrimRight(string(v), "\x00")
		case nupMetaVersion:
			p.DisplayVersion = strings.TrimRight(string(v), "\x00")
		case nupMetaLTitles, nupMetaLIcons:
			for rp := 0; rp < len(v); {
				ll := int(v[rp])
				rp++
				if rp+ll+4 > len(v) {
					return errors.New("the NUP package metadata is damaged")
				}
				lang := string(v[rp : rp+ll])
				rp += ll
				dl := int(le.Uint32(v[rp:]))
				rp += 4
				if dl < 0 || rp+dl > len(v) {
					return errors.New("the NUP package metadata is damaged")
				}
				if typ == nupMetaLTitles {
					if p.LocalTitles == nil {
						p.LocalTitles = map[string]string{}
					}
					p.LocalTitles[lang] = strings.TrimRight(string(v[rp:rp+dl]), "\x00")
				} else {
					if p.LocalIcons == nil {
						p.LocalIcons = map[string][]byte{}
					}
					p.LocalIcons[lang] = append([]byte(nil), v[rp:rp+dl]...)
				}
				rp += dl
			}
		}
	}
	return nil
}

// bestIcon: the package icon, else English, else Japanese, else any localised icon.
func (p NupPackage) bestIcon() []byte {
	if isImage(p.Icon) {
		return p.Icon
	}
	for _, l := range []string{"AmericanEnglish", "BritishEnglish", "en-US", "en-GB", "en", "Japanese", "ja"} {
		if ic := p.LocalIcons[l]; isImage(ic) {
			return ic
		}
	}
	for _, ic := range p.LocalIcons {
		if isImage(ic) {
			return ic
		}
	}
	return nil
}

func (p NupPackage) bestTitle() string {
	if p.Name != "" {
		return p.Name
	}
	for _, l := range []string{"AmericanEnglish", "BritishEnglish", "en-US", "en-GB", "en"} {
		if t := p.LocalTitles[l]; t != "" {
			return t
		}
	}
	for _, t := range p.LocalTitles {
		if t != "" {
			return t
		}
	}
	return ""
}

// isImage: JPEG, PNG, WebP, BMP or GIF data (game icons are JPEG on Switch; other containers may
// carry PNG).
func isImage(b []byte) bool {
	switch {
	case len(b) < 12:
		return false
	case b[0] == 0xFF && b[1] == 0xD8:
		return true
	case bytes.HasPrefix(b, []byte("\x89PNG")):
		return true
	case bytes.HasPrefix(b, []byte("RIFF")) && string(b[8:12]) == "WEBP":
		return true
	case bytes.HasPrefix(b, []byte("BM")), bytes.HasPrefix(b, []byte("GIF8")):
		return true
	}
	return false
}

// identifyNUP fills GameInfo from a NUP file's base package (or its first package).
func identifyNUP(f io.ReaderAt, size int64, info GameInfo) (GameInfo, error) {
	pkgs, err := ReadNupPackages(f, size)
	if err != nil {
		if info.TitleID != "" {
			return info, nil // keep the ID from the file name
		}
		return info, err
	}
	pick := pkgs[0]
	for _, p := range pkgs {
		if p.Kind == "base" {
			pick = p
			break
		}
	}
	b, kind := classifyID(pick.TitleID)
	if pick.Kind == "base" {
		kind = "base"
	}
	info.TitleID, info.FullID, info.Kind, info.Source = fmtID(b), fmtID(pick.TitleID), kind, "nup"
	if n := pick.bestTitle(); n != "" {
		info.Name = n
	}
	if pick.Publisher != "" {
		info.Publisher = pick.Publisher
	}
	// the newest included update's version is what runs
	ver := pick.DisplayVersion
	for _, p := range pkgs {
		if p.Kind == "update" && p.DisplayVersion != "" {
			ver = p.DisplayVersion
		}
	}
	if ver != "" {
		info.Version = ver
	}
	if ic := pick.bestIcon(); ic != nil {
		info.Icon = ic
	} else {
		for _, p := range pkgs { // e.g. an update carrying the icon
			if ic := p.bestIcon(); ic != nil {
				info.Icon = ic
				break
			}
		}
	}
	return info, nil
}
