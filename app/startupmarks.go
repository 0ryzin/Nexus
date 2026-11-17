package main

import (
	"sort"
	"sync"
	"time"
)

// Startup milestones (refinement brief, 17 Nov): when the window appears, the interface is
// usable, the library is available, the account is restored and the first background
// synchronisation has finished. Each is logged once ("startup: ... after 1.234s") and kept for
// GET /api/startup, so a slow start can be read from the user's log.
var startupMarks = struct {
	sync.Mutex
	m map[string]time.Duration
}{m: map[string]time.Duration{}}

// markStartup records a milestone the first time it happens; detail goes to the log only.
func (a *App) markStartup(name, detail string) {
	d := time.Since(startupBegan)
	startupMarks.Lock()
	if _, seen := startupMarks.m[name]; seen || len(startupMarks.m) > 32 {
		startupMarks.Unlock()
		return
	}
	startupMarks.m[name] = d
	startupMarks.Unlock()
	if detail != "" {
		detail = " (" + detail + ")"
	}
	if a != nil && a.Log != nil {
		a.Log.Printf("startup: %s after %v%s", name, d.Round(time.Millisecond), detail)
	}
}

type startupMark struct {
	Name string `json:"name"`
	Ms   int64  `json:"ms"`
}

func startupMarkList() []startupMark {
	startupMarks.Lock()
	defer startupMarks.Unlock()
	out := make([]startupMark, 0, len(startupMarks.m))
	for n, d := range startupMarks.m {
		out = append(out, startupMark{n, d.Milliseconds()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ms < out[j].Ms })
	return out
}

// startupMarkNames the interface may report (anything else is ignored).
var uiStartupMarks = map[string]bool{"interface ready": true, "home interactive": true, "profile picker shown": true, "library available": true}
