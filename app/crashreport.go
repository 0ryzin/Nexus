package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// Crash reports (docs/CRASHES.md).
//
// The engine runs inside the NEXUS process, so a native crash in it ends NEXUS too. Three things
// make sure such an end is never silent:
//
//  1. The engine installs its own crash handler when its log starts (engine patch 0027): a
//     "Host crash" Critical line with the faulting module+offset, thread and stack in
//     eden_log.txt, and eden_crash.dmp next to it.
//  2. NEXUS re-enables Windows Error Reporting, which the Go runtime turns off for the process
//     (SEM_NOGPFAULTERRORBOX), without its dialog (GOTRACEBACK=wer), and writes Go's own fatal
//     errors to Logs\crash-go.log instead of a console nobody sees.
//  3. While a game session runs, NEXUS keeps a small session record (Logs\session.json): the
//     game, phase and stage, shader preparation and shader counts, GPU and the session's main
//     renderer settings. A clean end removes it. If NEXUS starts and finds it, the last session
//     ended without NEXUS seeing it end: NEXUS writes a crash report to Logs\crashes from the
//     record and the logs, keeps the engine's dump with it, and tells the user once.
//
// Nothing sensitive goes into these files: no keys, account names, e-mail addresses or tokens,
// and the user's home folder is written as %USERPROFILE%.

// sessionRecord is what NEXUS was doing in the running game session.
type sessionRecord struct {
	NEXUS       string            `json:"nexus"`
	Engine      string            `json:"engine,omitempty"`
	PID         int               `json:"pid"`
	Started     time.Time         `json:"started"`
	Updated     time.Time         `json:"updated"`
	Game        string            `json:"game"`
	GameKey     string            `json:"gameKey,omitempty"`
	TitleID     string            `json:"titleId,omitempty"`
	Phase       string            `json:"phase"`           // NEXUS: starting | playing | stopping
	Stage       string            `json:"stage,omitempty"` // NEXUS: prepare | loading | shaders | precache | starting
	EngineState string            `json:"engineState,omitempty"`
	LoadStage   string            `json:"loadStage,omitempty"`
	LoadDone    uint64            `json:"loadDone,omitempty"`
	LoadTotal   uint64            `json:"loadTotal,omitempty"`
	Frames      uint64            `json:"frames,omitempty"`
	Suspended   bool              `json:"suspended,omitempty"`
	Prep        *PrepStatus       `json:"prep,omitempty"`
	Shaders     *ShaderStatus     `json:"shaders,omitempty"`
	GPU         string            `json:"gpu,omitempty"`
	Settings    map[string]string `json:"settings,omitempty"`
}

// crashTracker keeps the session record on disk while a session runs.
type crashTracker struct {
	path    string
	mu      sync.Mutex
	rec     *sessionRecord
	written time.Time
}

// sessionRecordEvery limits how often the record is rewritten while nothing important changes:
// a few hundred bytes every few seconds, nothing measurable.
const sessionRecordEvery = 3 * time.Second

func newCrashTracker(logs string) *crashTracker {
	return &crashTracker{path: filepath.Join(logs, "session.json")}
}

// begin starts a record for a session that is starting.
func (c *crashTracker) begin(r sessionRecord) {
	if c == nil {
		return
	}
	r.PID = os.Getpid()
	r.NEXUS = AppVersion
	if r.Started.IsZero() {
		r.Started = time.Now()
	}
	c.mu.Lock()
	c.rec = &r
	c.flushLocked(true)
	c.mu.Unlock()
}

// update changes the record; important changes (phase, stage) are written at once, the rest at
// most every sessionRecordEvery.
func (c *crashTracker) update(fn func(r *sessionRecord) (important bool)) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rec == nil {
		return
	}
	important := fn(c.rec)
	c.flushLocked(important)
}

func (c *crashTracker) flushLocked(now bool) {
	if !now && time.Since(c.written) < sessionRecordEvery {
		return
	}
	c.rec.Updated = time.Now()
	data, err := json.MarshalIndent(c.rec, "", "  ")
	if err != nil {
		return
	}
	if writeFileAtomic(c.path, data) == nil {
		c.written = time.Now()
	}
}

// end removes the record: the session ended and NEXUS saw it end.
func (c *crashTracker) end() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.rec = nil
	_ = os.Remove(c.path)
	c.mu.Unlock()
}

// sessionSettings: the session's main renderer settings, for the report (no paths).
func sessionSettings(cfg string) map[string]string {
	ini, _ := LoadIni(cfg)
	if ini == nil {
		return nil
	}
	out := map[string]string{}
	for _, k := range []string{"backend", "gpu_accuracy", "resolution_setup", "use_asynchronous_shaders",
		"use_reactive_flushing", "vram_usage_mode", "use_disk_shader_cache", "precache_shaders", "use_vsync"} {
		if v, ok := ini.GetSetting("Renderer", k); ok {
			out[k] = v
		}
	}
	for _, k := range []string{"use_multi_core", "memory_layout_mode"} {
		if v, ok := ini.GetSetting("Core", k); ok {
			out[k] = v
		}
	}
	return out
}

// ---------- process-level crash capture

// installCrashCapture makes a crash of the NEXUS process leave a trace (see the top of this file).
func (a *App) installCrashCapture() {
	if runtime.GOOS == "windows" {
		debug.SetTraceback("wer") // Windows Error Reporting back on, without its dialog
	}
	path := filepath.Join(a.Paths.Logs, "crash-go.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close() // SetCrashOutput keeps its own duplicate
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		a.Log.Printf("crash: Go crash output not set: %v", err)
	}
}

// ---------- the report after an unclean end

// CrashNotice is shown once after NEXUS restarts following an unclean end of a game session.
type CrashNotice struct {
	Title     string `json:"title"`
	When      int64  `json:"when"`
	Message   string `json:"message"`
	Detail    string `json:"detail"`
	Technical string `json:"technical,omitempty"`
	Log       string `json:"log"`
}

var (
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	// lines that could carry account or key material are left out of reports entirely
	reSensitive = regexp.MustCompile(`(?i)token|password|secret|bearer|authorization|account|prod\.keys|title\.keys|keys?\s*=|amiibo key`)
)

// sanitizeLine removes personal information from a log line for a crash report.
func sanitizeLine(l, home string) (string, bool) {
	if reSensitive.MatchString(l) {
		return "", false
	}
	if home != "" {
		l = replaceFold(l, home, "%USERPROFILE%")
		l = replaceFold(l, filepath.ToSlash(home), "%USERPROFILE%")
	}
	return reEmail.ReplaceAllString(l, "<e-mail>"), true
}

func replaceFold(s, old, repl string) string {
	if old == "" {
		return s
	}
	lo, lold := strings.ToLower(s), strings.ToLower(old)
	var b strings.Builder
	for {
		i := strings.Index(lo, lold)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString(repl)
		s, lo = s[i+len(old):], lo[i+len(old):]
	}
}

// reportLines: the last n lines of a log file (after `since` for NEXUS's own log), sanitised.
func reportLines(path string, n int, keep func(string) bool) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(data) > 512<<10 {
		data = data[len(data)-512<<10:]
	}
	home, _ := os.UserHomeDir()
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.TrimRight(lines[i], " \t")
		if strings.TrimSpace(l) == "" || (keep != nil && !keep(l)) {
			continue
		}
		l, ok := sanitizeLine(l, home)
		if !ok {
			continue
		}
		if len(l) > 400 {
			l = l[:400] + "…"
		}
		out = append(out, l)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// checkLastSession runs at start, before the engine is loaded (the engine log is still the
// crashed session's). It returns the notice to show, or nil.
func (a *App) checkLastSession() *CrashNotice {
	path := filepath.Join(a.Paths.Logs, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var rec sessionRecord
	if json.Unmarshal(data, &rec) != nil || rec.PID == os.Getpid() {
		_ = os.Remove(path)
		return nil
	}
	n, err := a.writeCrashReport(rec, time.Now())
	_ = os.Remove(path)
	if err != nil {
		a.Log.Printf("crash: the last session of %s ended unexpectedly; no report: %v", rec.Game, err)
		return nil
	}
	a.Log.Printf("crash: the last session of %s ended unexpectedly (%s, stage %q); report %s",
		rec.Game, rec.Phase, rec.Stage, n.Log)
	return n
}

// writeCrashReport writes Logs\crashes\crash-<time>.txt (and moves the engine's dump next to it).
func (a *App) writeCrashReport(rec sessionRecord, now time.Time) (*CrashNotice, error) {
	dir := filepath.Join(a.Paths.Logs, "crashes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	stamp := now.Format("20060102-150405")
	engineLog := filepath.Join(a.Emu.DataDir(), "log", "eden_log.txt")
	var b strings.Builder
	fmt.Fprintf(&b, "NEXUS crash report\nWritten %s by NEXUS %s\n\n", now.Format(time.RFC1123), AppVersion)
	fmt.Fprintf(&b, "The game session below ended without NEXUS seeing it end: NEXUS itself closed.\n\n")
	fmt.Fprintf(&b, "Game:           %s (%s)\n", rec.Game, firstNonEmpty(rec.TitleID, "?"))
	fmt.Fprintf(&b, "NEXUS / engine: %s / %s\n", rec.NEXUS, firstNonEmpty(rec.Engine, "?"))
	fmt.Fprintf(&b, "Session start:  %s\n", rec.Started.Format(time.RFC1123))
	fmt.Fprintf(&b, "Last update:    %s (%s after the start)\n", rec.Updated.Format(time.RFC1123), rec.Updated.Sub(rec.Started).Round(time.Second))
	fmt.Fprintf(&b, "Phase / stage:  %s / %s\n", firstNonEmpty(rec.Phase, "?"), firstNonEmpty(rec.Stage, "-"))
	fmt.Fprintf(&b, "Engine state:   %s, load stage %q %d/%d, %d frames%s\n", firstNonEmpty(rec.EngineState, "?"),
		rec.LoadStage, rec.LoadDone, rec.LoadTotal, rec.Frames, map[bool]string{true: ", paused (NEXUS Home)"}[rec.Suspended])
	if p := rec.Prep; p != nil {
		fmt.Fprintf(&b, "Shader preparation: %s (stage %q, %d/%d, %d archives, %d programs, %d skipped, %.1f s)%s\n",
			p.State, p.Stage, p.Done, p.Total, p.Archives, p.Programs, p.Skipped, p.Seconds, map[bool]string{true: " error: " + p.Error}[p.Error != ""])
	}
	if s := rec.Shaders; s != nil {
		fmt.Fprintf(&b, "Shaders:        %d from the cache, %d prebuilt, %d compiled when used, %s queued, %s building, %s in the background queue, %s failed\n",
			s.FromCache, s.FromPrebuild, s.Compiled, optCount(s.Queued), optCount(s.Running), optCount(s.BackgroundQueued), optCount(s.Failed))
	}
	fmt.Fprintf(&b, "GPU:            %s\n", firstNonEmpty(rec.GPU, "?"))
	if len(rec.Settings) > 0 {
		keys := make([]string, 0, len(rec.Settings))
		for k := range rec.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("Settings:      ")
		for _, k := range keys {
			fmt.Fprintf(&b, " %s=%s", k, rec.Settings[k])
		}
		b.WriteString("\n")
	}

	// the engine's own crash report, when its handler ran
	hostCrash := reportLines(engineLog, 8, func(l string) bool { return strings.Contains(l, "Host crash") })
	b.WriteString("\n--- Engine crash handler\n")
	if len(hostCrash) == 0 {
		b.WriteString("(no \"Host crash\" line: the engine's handler did not run. The crash was in NEXUS itself, a fail-fast\n" +
			" the handler cannot catch, or the process was ended from outside, for example by Windows when memory ran out.)\n")
	}
	for _, l := range hostCrash {
		b.WriteString(l + "\n")
	}
	dump := filepath.Join(a.Emu.DataDir(), "log", "eden_crash.dmp")
	if fi, err := os.Stat(dump); err == nil && !fi.ModTime().Before(rec.Started) {
		kept := filepath.Join(dir, "crash-"+stamp+".dmp")
		if os.Rename(dump, kept) == nil {
			fmt.Fprintf(&b, "Minidump: %s (%d KB)\n", filepath.Base(kept), fi.Size()>>10)
		}
	}
	if d := windowsCrashDump(rec.Started); d != "" {
		fmt.Fprintf(&b, "Windows Error Reporting dump: %s\n", d)
	}
	if goCrash := reportLines(filepath.Join(a.Paths.Logs, "crash-go.log"), 60, nil); len(goCrash) > 0 {
		if fi, err := os.Stat(filepath.Join(a.Paths.Logs, "crash-go.log")); err == nil && !fi.ModTime().Before(rec.Started) {
			b.WriteString("\n--- NEXUS (Go) fatal error output\n" + strings.Join(goCrash, "\n") + "\n")
		}
	}
	errs := reportLines(engineLog, 25, func(l string) bool {
		return strings.Contains(l, "<Error>") || strings.Contains(l, "<Critical>")
	})
	b.WriteString("\n--- Engine log: the last error lines\n" + strings.Join(errs, "\n") + "\n")
	tail := reportLines(engineLog, 60, nil)
	b.WriteString("\n--- Engine log: the last lines\n" + strings.Join(tail, "\n") + "\n")
	since := rec.Started.Format("2006/01/02 15:04")
	nexusTail := reportLines(filepath.Join(a.Paths.Logs, "nexus.log"), 30, func(l string) bool { return l >= since })
	b.WriteString("\n--- NEXUS log: the last lines of the session\n" + strings.Join(nexusTail, "\n") + "\n")

	p := filepath.Join(dir, "crash-"+stamp+".txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	cleanOldLogs(dir, "crash-", 20)
	where := map[string]string{"prepare": "while preparing shaders", "starting": "while the game was starting",
		"playing": "while you were playing", "stopping": "while the game was closing"}[rec.Phase]
	if rec.Phase == "starting" && rec.Stage == "prepare" {
		where = "while preparing shaders"
	}
	tech := strings.TrimSpace(strings.Join(hostCrash, "\n"))
	if tech == "" {
		tech = strings.Join(errs[max(0, len(errs)-8):], "\n")
	}
	return &CrashNotice{
		Title:     rec.Game,
		When:      rec.Updated.Unix(),
		Message:   "NEXUS closed unexpectedly",
		Detail:    fmt.Sprintf("NEXUS closed %s (%s). Your saves were not changed by this report. A crash report with what NEXUS and the engine were doing has been saved; it contains no keys or account details.", firstNonEmpty(where, "during the game session"), rec.Updated.Format("15:04")),
		Technical: tech,
		Log:       p,
	}, nil
}

// windowsCrashDump: a Windows Error Reporting dump of NEXUS written since the session started.
func windowsCrashDump(since time.Time) string {
	la := os.Getenv("LOCALAPPDATA")
	if la == "" {
		return ""
	}
	dir := filepath.Join(la, "CrashDumps")
	entries, _ := os.ReadDir(dir)
	newest, at := "", since
	for _, e := range entries {
		n := strings.ToLower(e.Name())
		if !strings.HasPrefix(n, "nexus") || !strings.HasSuffix(n, ".dmp") {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().After(at) {
			newest, at = filepath.Join(dir, e.Name()), fi.ModTime()
		}
	}
	if home, _ := os.UserHomeDir(); home != "" && newest != "" {
		newest = replaceFold(newest, home, "%USERPROFILE%")
	}
	return newest
}

func optCount(p *uint64) string {
	if p == nil {
		return "?"
	}
	return fmt.Sprint(*p)
}

func (a *App) setCrashNotice(n *CrashNotice) {
	a.crashMu.Lock()
	a.crashNotice = n
	a.crashMu.Unlock()
}

// takeCrashNotice returns the notice once.
func (a *App) takeCrashNotice() *CrashNotice {
	a.crashMu.Lock()
	defer a.crashMu.Unlock()
	n := a.crashNotice
	a.crashNotice = nil
	return n
}
