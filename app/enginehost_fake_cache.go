package main

// The stand-in engine's shader cache (development and tests): the same log file and format as
// the real engine (lichii-cache.v2.bin in the session's shader folder), so the shared shader
// cache can be tested end to end with two NEXUS data folders. The record payloads are stand-ins.

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const fakeGamePipelines = 40

var reFakeTitle = regexp.MustCompile(`\[([0-9A-Fa-f]{16})\]`)

type fakeCache struct {
	path    string
	title   string
	have    map[uint64]bool
	built   map[uint64]bool
	pending [][]byte
}

func fakeShaderCache(req EngineRequest) *fakeCache {
	c := &fakeCache{have: map[uint64]bool{}, built: map[uint64]bool{}}
	c.title = strings.ToLower(req.TitleID)
	if m := reFakeTitle.FindStringSubmatch(filepath.Base(req.Game)); c.title == "" && m != nil {
		c.title = strings.ToLower(m[1])
	}
	if c.title == "" {
		return c
	}
	dir := req.ShaderDir
	if dir == "" {
		dir = filepath.Join(req.DataDir, "cache", "shader")
	}
	c.path = filepath.Join(dir, c.title, v2LogName)
	if b, err := os.ReadFile(c.path); err == nil {
		if _, recs, _, err := parseV2Log(b); err == nil {
			live, _, _ := liveRecords(b, recs)
			for k := range live {
				if k.kind == v2KindGraphics {
					c.have[k.key] = true
				}
			}
		}
	}
	return c
}

func (c *fakeCache) key(i int) uint64 {
	return uint64(crc32.ChecksumIEEE([]byte(fmt.Sprintf("%s pipeline %d", c.title, i))))<<8 | uint64(i)
}

// known: the game's pipelines in the cache when the session started.
func (c *fakeCache) known() []uint64 {
	var out []uint64
	for i := 1; i <= fakeGamePipelines; i++ {
		if c.have[c.key(i)] {
			out = append(out, c.key(i))
		}
	}
	return out
}

// build: the game has asked for its first `asked` pipelines; those not in the cache are built
// (and queued for the cache). Returns how many were built this session.
func (c *fakeCache) build(asked int) int {
	for i := 1; i <= asked && c.title != ""; i++ {
		k := c.key(i)
		if c.have[k] || c.built[k] {
			continue
		}
		c.built[k] = true
		c.pending = append(c.pending, v2EncodeRecord(v2KindGraphics, k, []byte(fmt.Sprintf("pipeline %d of %s", i, c.title))))
	}
	return len(c.built)
}

// flush appends the built pipelines to the cache, under its lock, as the engine does.
func (c *fakeCache) flush() {
	if c.path == "" || len(c.pending) == 0 {
		return
	}
	src := v2Header(1)
	for _, r := range c.pending {
		src = append(src, r...)
	}
	if _, _, _, err := appendPortable(c.path, src); err == nil {
		c.pending = nil
	}
}

// v2EncodeRecord frames a record like the engine (with a stand-in payload hash: NEXUS never
// checks it, the engine does).
func v2EncodeRecord(kind uint16, key uint64, payload []byte) []byte {
	h := make([]byte, v2RecordHeader)
	binary.LittleEndian.PutUint32(h, v2RecordMagic)
	binary.LittleEndian.PutUint16(h[4:], kind)
	binary.LittleEndian.PutUint32(h[8:], uint32(len(payload)))
	binary.LittleEndian.PutUint64(h[12:], key)
	binary.LittleEndian.PutUint32(h[20:], crc32.ChecksumIEEE(h[:20]))
	out := append(h, payload...)
	out = append(out, make([]byte, v2RecordHash)...)
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	return out
}
