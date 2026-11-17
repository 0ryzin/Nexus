package main

// Shared shader cache (docs/SHADERS.md, "Shared shader cache"). Every NEXUS user of the PC - the
// profiles of one NEXUS, and other Windows users with their own NEXUS data - uses one folder for
// the shader data that is the same on any GPU:
//
//	Windows: %ProgramData%\NEXUS\ShaderCache\<title id>\  (Users may read and write, inherited)
//	  lichii-cache.v2.bin  the engine's shader cache log: guest programs, pipeline keys, used lists
//	  warm-compute.bin     the precache file shader preparation makes (+ .count)
//
// The engine is told with the session's "shaderDir" (engine patch 0030). What belongs to one
// user's GPU and driver stays in that user's data (Engine\cache\shader\<title id>\): driver
// blobs, the Vulkan pipeline cache, the precache's "done" marker and progress - the GPU driver
// keeps its own cache per Windows user, so those cannot be shared.
//
// Safety: the engine appends to the log under the log's file lock (lichii-cache.v2.bin.lock),
// every record has its own checksums and a cut-off tail is skipped; NEXUS takes the same lock
// whenever it adds records (import, moving a user's older cache in). Preparing a game's shaders
// takes warm-compute.bin.lock, so two users never write the precache file at once.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SharedShaderDir is the shared folder ("" when sharing is off or there is none on this system).
func (e *Emulator) SharedShaderDir() string {
	if e.app.Settings.Get().Emulator.PrivateShaderCache {
		return ""
	}
	if d := os.Getenv("NEXUS_SHARED_SHADER_DIR"); d != "" {
		return d
	}
	return defaultSharedShaderDir()
}

// UserShaderDir is this user's own shader folder (the engine's default).
func (e *Emulator) UserShaderDir() string { return filepath.Join(e.DataDir(), "cache", "shader") }

// ShaderDir is where a game's portable shader data is: the shared folder, else the user's own.
func (e *Emulator) ShaderDir() string {
	if d := e.ensureSharedShaderDir(); d != "" {
		return d
	}
	return e.UserShaderDir()
}

var sharedDirMu sync.Mutex
var sharedDirReady = map[string]bool{}

// ensureSharedShaderDir creates the shared folder (with permissions every user of the PC can
// use) once per run; "" when it cannot be used, and then each user keeps their own cache.
func (e *Emulator) ensureSharedShaderDir() string {
	d := e.SharedShaderDir()
	if d == "" {
		return ""
	}
	sharedDirMu.Lock()
	defer sharedDirMu.Unlock()
	if ok, done := sharedDirReady[d]; done {
		if ok {
			return d
		}
		return ""
	}
	err := prepareSharedDir(d)
	sharedDirReady[d] = err == nil
	if err != nil {
		e.app.Log.Printf("shaders: the shared shader cache %s cannot be used (%v); this user's own is used", d, err)
		return ""
	}
	e.app.Log.Printf("shaders: shared shader cache %s", d)
	return d
}

// ---------- adding records under the engine's lock

// appendPortable adds src's live portable records that dst lacks to the log at dst (created when
// missing), holding the log's lock, by appending: records already in dst, or removed there by a
// tombstone, are left alone, and nothing already in dst is rewritten. A game running in another
// NEXUS keeps appending safely around it.
func appendPortable(dst string, src []byte) (added, present, removed int, err error) {
	_, srecs, _, err := parseV2Log(src)
	if err != nil {
		return 0, 0, 0, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, 0, 0, err
	}
	unlock, err := lockWait(dst+".lock", 5*time.Second)
	if err != nil {
		return 0, 0, 0, err
	}
	defer unlock()
	local, err := os.ReadFile(dst)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, 0, 0, err
	}
	var have map[rk]v2Record
	var gone map[rk]bool
	var prefix []byte
	if len(local) < v2FileHeaderSize {
		prefix = v2Header(time.Now().Unix()) // new (or cut off in its header, as the engine does)
		local = nil
	} else {
		ver, lrecs, _, perr := parseV2Log(local)
		if perr != nil {
			return 0, 0, 0, fmt.Errorf("the shader cache %s cannot be read (%v); nothing was changed", dst, perr)
		}
		if ver != v2FormatVersion {
			return 0, 0, 0, fmt.Errorf("the shader cache %s uses format %d; nothing was changed", dst, ver)
		}
		have, _, gone = liveRecords(local, lrecs)
		if pad := (8 - len(local)%8) % 8; pad > 0 {
			prefix = make([]byte, pad) // a record cut off by a crash: the next starts aligned
		}
	}
	live, order, _ := liveRecords(src, srecs)
	out := prefix
	for _, k := range order {
		r, ok := live[k]
		if !ok || !portableKind(r.kind) {
			continue
		}
		switch {
		case gone[k]:
			removed++
		case have != nil && func() bool { _, ok := have[k]; return ok }():
			present++
		default:
			out = append(out, src[r.off:r.end]...)
			added++
		}
	}
	if added == 0 {
		return 0, present, removed, nil
	}
	if local == nil {
		tmp := dst + ".nexus-new"
		if err := os.WriteFile(tmp, out, 0o666); err != nil {
			os.Remove(tmp)
			return 0, present, removed, err
		}
		if err := os.Rename(tmp, dst); err != nil {
			os.Remove(tmp)
			return 0, present, removed, err
		}
		return added, present, removed, nil
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return 0, present, removed, err
	}
	n, werr := f.Write(out)
	cerr := f.Close()
	if werr != nil || cerr != nil || n != len(out) {
		_ = os.Truncate(dst, int64(len(local))) // like the engine: cut off what was half written
		return 0, present, removed, errors.Join(werr, cerr, io.ErrShortWrite)
	}
	return added, present, removed, nil
}

// lockWait takes a file lock, trying for up to wait.
func lockWait(path string, wait time.Duration) (func(), error) {
	give := time.Now().Add(wait)
	for {
		unlock, busy, err := tryLockFile(path)
		if err == nil {
			return unlock, nil
		}
		if !busy || time.Now().After(give) {
			if busy {
				return nil, errors.New("the shader cache is busy (another NEXUS is writing it); try again in a moment")
			}
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// ---------- moving a user's earlier (private) cache into the shared one

// shareUserCaches adds what this user's own cache has to the shared one, once per title and
// version of that cache. The user's files are kept (they are used again if sharing is turned off).
func (e *Emulator) shareUserCaches() {
	shared := e.ensureSharedShaderDir()
	if shared == "" {
		return
	}
	dirs, _ := os.ReadDir(e.UserShaderDir())
	for _, d := range dirs {
		if !d.IsDir() || len(d.Name()) != 16 {
			continue
		}
		title := strings.ToLower(d.Name())
		local := filepath.Join(e.UserShaderDir(), title, v2LogName)
		fi, err := os.Stat(local)
		if err != nil || fi.Size() <= v2FileHeaderSize {
			continue
		}
		mark := filepath.Join(e.UserShaderDir(), title, "shared.nexus")
		stamp := fmt.Sprintf("%d %d", fi.Size(), fi.ModTime().UnixNano())
		if b, _ := os.ReadFile(mark); strings.TrimSpace(string(b)) == stamp {
			continue
		}
		src, err := os.ReadFile(local)
		if err != nil {
			continue
		}
		added, present, _, err := appendPortable(filepath.Join(shared, title, v2LogName), src)
		if err != nil {
			e.app.Log.Printf("shaders: %s not added to the shared cache yet: %v", strings.ToUpper(title), err)
			continue
		}
		// the precache file, when the shared folder has none: copied whole, under its lock
		for _, name := range []string{"warm-compute.bin", "warm-compute.count", "precache-graphics.bin"} {
			from, to := filepath.Join(e.UserShaderDir(), title, name), filepath.Join(shared, title, name)
			if fileExists(from) && !fileExists(to) {
				if unlock, err := lockWait(filepath.Join(shared, title, "warm-compute.bin.lock"), time.Second); err == nil {
					if !fileExists(to) {
						_ = copyFileAtomic(from, to)
					}
					unlock()
				}
			}
		}
		_ = os.WriteFile(mark, []byte(stamp+"\n"), 0o644)
		e.app.Log.Printf("shaders: %s: this user's shader cache added to the shared one (%d records added, %d already there)",
			strings.ToUpper(title), added, present)
	}
}

// ---------- status for the interface

// SharedCacheTitle is one game in the cache.
type SharedCacheTitle struct {
	TitleID  string `json:"titleId"`
	Title    string `json:"title,omitempty"`
	Programs int    `json:"programs"`
	Graphics int    `json:"graphics"`
	Compute  int    `json:"compute"`
	Sessions int    `json:"sessions"` // play sessions recorded (any user)
	Bytes    int64  `json:"bytes"`
	Updated  int64  `json:"updated"`
	Precache bool   `json:"precache"` // a precache file from shader preparation
	DoneHere bool   `json:"doneHere"` // this user's driver cache was filled from it (marker)
	Damaged  bool   `json:"damaged"`  // the log cannot be read
	Format   uint32 `json:"format"`   // log format
	Usable   bool   `json:"usable"`   // this engine reads this format
	Location string `json:"location"` // shared | private
}

// SharedCacheStatus is the Shaders section of Settings.
type SharedCacheStatus struct {
	Shared  bool               `json:"shared"`
	Dir     string             `json:"dir"`
	Bytes   int64              `json:"bytes"`
	Records int                `json:"records"`
	Titles  []SharedCacheTitle `json:"titles"`
}

type cacheIndexEntry struct {
	size, mod int64
	t         SharedCacheTitle
}

var cacheIndex struct {
	sync.Mutex
	m map[string]cacheIndexEntry
}

// titleStats reads a game's log once per size and modification time (an index in memory), so
// opening the screen again does not read hundreds of megabytes again.
func titleStats(dir, title string) (SharedCacheTitle, bool) {
	p := filepath.Join(dir, title, v2LogName)
	fi, err := os.Stat(p)
	if err != nil {
		return SharedCacheTitle{}, false
	}
	cacheIndex.Lock()
	if cacheIndex.m == nil {
		cacheIndex.m = map[string]cacheIndexEntry{}
	}
	if e, ok := cacheIndex.m[p]; ok && e.size == fi.Size() && e.mod == fi.ModTime().UnixNano() {
		cacheIndex.Unlock()
		return e.t, true
	}
	cacheIndex.Unlock()
	t := SharedCacheTitle{TitleID: strings.ToUpper(title), Bytes: fi.Size(), Updated: fi.ModTime().Unix()}
	if b, err := os.ReadFile(p); err == nil {
		ver, recs, _, perr := parseV2Log(b)
		t.Format, t.Usable = ver, perr == nil && ver == v2FormatVersion
		if perr != nil {
			t.Damaged = true
		} else {
			live, _, _ := liveRecords(b, recs)
			for k := range live {
				switch k.kind {
				case v2KindProgram:
					t.Programs++
				case v2KindGraphics:
					t.Graphics++
				case v2KindCompute:
					t.Compute++
				case v2KindUsed:
					t.Sessions++
				}
			}
		}
	}
	cacheIndex.Lock()
	cacheIndex.m[p] = cacheIndexEntry{fi.Size(), fi.ModTime().UnixNano(), t}
	cacheIndex.Unlock()
	return t, true
}

// SharedCacheStatus lists the games in the cache NEXUS uses now.
func (e *Emulator) SharedCacheStatus() SharedCacheStatus {
	dir := e.ShaderDir()
	st := SharedCacheStatus{Shared: dir != e.UserShaderDir(), Dir: dir, Titles: []SharedCacheTitle{}}
	ents, _ := os.ReadDir(dir)
	for _, d := range ents {
		if !d.IsDir() || len(d.Name()) != 16 {
			continue
		}
		t, ok := titleStats(dir, strings.ToLower(d.Name()))
		if !ok {
			continue
		}
		t.Precache = fileExists(filepath.Join(dir, d.Name(), "warm-compute.bin"))
		t.DoneHere = fileExists(filepath.Join(e.UserShaderDir(), strings.ToLower(d.Name()), "warm-compute.done"))
		t.Location = map[bool]string{true: "shared", false: "private"}[st.Shared]
		if g := e.app.Library.ByTitleID(t.TitleID); g != nil {
			t.Title = g.Title
		}
		st.Bytes += t.Bytes
		st.Records += t.Programs + t.Graphics + t.Compute
		st.Titles = append(st.Titles, t)
	}
	sort.Slice(st.Titles, func(i, j int) bool { return st.Titles[i].Updated > st.Titles[j].Updated })
	return st
}

// GameShaderCache is one game's line in its menu and information.
func (e *Emulator) GameShaderCache(titleID string) (SharedCacheTitle, bool) {
	dir := e.ShaderDir()
	t, ok := titleStats(dir, strings.ToLower(titleID))
	if ok {
		t.Precache = fileExists(filepath.Join(dir, strings.ToLower(titleID), "warm-compute.bin"))
		t.DoneHere = fileExists(filepath.Join(e.UserShaderDir(), strings.ToLower(titleID), "warm-compute.done"))
		t.Location = map[bool]string{true: "shared", false: "private"}[dir != e.UserShaderDir()]
	}
	return t, ok
}

// sharedForEngine: the session's "shaderDir" ("" = the user's own folder).
func (e *Emulator) sharedForEngine() string { return e.ensureSharedShaderDir() }
