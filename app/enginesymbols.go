package main

// Engine symbols (docs/CRASHES.md, "Engine symbols"): every Windows build ships
// Engine\nexus_engine.symbols.gz next to nexus_engine.dll, made by CI from the link map of that
// very DLL (tools/release/make_symbols.py). It lists the engine's functions by address. NEXUS
// uses it only to turn the "nexus_engine.dll+0x…" frames of a crash into function names, and
// only when its first line names the SHA-256 of the DLL that crashed: a table of another build
// would name the wrong functions, so it is never used for one.

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const engineSymbolsFile = "nexus_engine.symbols.gz"

type engineSymbol struct {
	rva  uint64
	obj  string
	name string
}

type engineSymbols struct {
	sha  string
	syms []engineSymbol
}

// loadEngineSymbols reads the table in dir. wantSHA is the SHA-256 of the DLL whose addresses are
// to be decoded (from its engine.json); a table made for another DLL is refused.
func loadEngineSymbols(dir, wantSHA string) (*engineSymbols, error) {
	f, err := os.Open(filepath.Join(dir, engineSymbolsFile))
	if err != nil {
		return nil, fmt.Errorf("no engine symbols with this engine (%w)", err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("the engine symbols are damaged: %w", err)
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	if !sc.Scan() {
		return nil, errors.New("the engine symbols are empty")
	}
	head := strings.Fields(sc.Text())
	if len(head) != 5 || head[0] != "NEXUSSYM" || head[1] != "1" {
		return nil, errors.New("the engine symbols have an unknown format")
	}
	if wantSHA == "" || !strings.EqualFold(head[2], wantSHA) {
		return nil, fmt.Errorf("the engine symbols are for another engine build (%.12s, not %.12s)", head[2], wantSHA)
	}
	count, _ := strconv.Atoi(head[4])
	s := &engineSymbols{sha: strings.ToLower(head[2]), syms: make([]engineSymbol, 0, min(count, 1<<21))}
	for sc.Scan() {
		rvaS, rest, ok := strings.Cut(sc.Text(), " ")
		obj, name, ok2 := strings.Cut(rest, " ")
		rva, err := strconv.ParseUint(rvaS, 16, 64)
		if !ok || !ok2 || err != nil {
			return nil, errors.New("the engine symbols are damaged")
		}
		s.syms = append(s.syms, engineSymbol{rva, obj, name})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("the engine symbols are damaged: %w", err)
	}
	if len(s.syms) != count {
		return nil, fmt.Errorf("the engine symbols are incomplete (%d of %d)", len(s.syms), count)
	}
	if !sort.SliceIsSorted(s.syms, func(i, j int) bool { return s.syms[i].rva < s.syms[j].rva }) {
		return nil, errors.New("the engine symbols are not in order")
	}
	return s, nil
}

// resolve names the function containing rva: "Name+0x12 (object.obj)".
func (s *engineSymbols) resolve(rva uint64) (string, bool) {
	i := sort.Search(len(s.syms), func(i int) bool { return s.syms[i].rva > rva }) - 1
	if i < 0 {
		return "", false
	}
	// past the last symbol by more than 1 MB: outside the code the table covers
	if i == len(s.syms)-1 && rva-s.syms[i].rva > 1<<20 {
		return "", false
	}
	sym := s.syms[i]
	return fmt.Sprintf("%s+0x%x (%s)", sym.name, rva-sym.rva, sym.obj), true
}

var reEngineFrame = regexp.MustCompile(`nexus_engine\.dll\+0x([0-9a-fA-F]+)`)

// decodeEngineFrames lists every nexus_engine.dll frame of the lines, decoded, in order (each
// once).
func (s *engineSymbols) decodeEngineFrames(lines []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range lines {
		for _, m := range reEngineFrame.FindAllStringSubmatch(l, -1) {
			if seen[m[0]] {
				continue
			}
			seen[m[0]] = true
			rva, err := strconv.ParseUint(m[1], 16, 64)
			if err != nil {
				continue
			}
			if name, ok := s.resolve(rva); ok {
				out = append(out, m[0]+"  "+name)
			} else {
				out = append(out, m[0]+"  (not in the symbol table)")
			}
		}
	}
	return out
}

// engineDLLHash: the SHA-256 of the engine in dir, from its engine.json (verified when loaded).
func engineDLLHash(dir string) string {
	if m, err := readManifest(dir); err == nil {
		return strings.ToLower(m.Files[engineLibName])
	}
	return ""
}

func engineCommit(dir string) string {
	if m, err := readManifest(dir); err == nil {
		return m.Commit
	}
	return ""
}
