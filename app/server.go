package main

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/gif"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

var errNotSupported = errors.New("Not supported on this platform.")

type Server struct {
	app   *App
	token string
	mux   *http.ServeMux
	ln    net.Listener
}

func NewServer(a *App) *Server {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	s := &Server{app: a, token: hex.EncodeToString(b), mux: http.NewServeMux()}
	s.routes()
	return s
}

// Start listens on a random loopback port and returns the URL to open.
func (s *Server) Start(port int) (string, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", err
	}
	s.ln = ln
	srv := &http.Server{Handler: s.guard(s.mux), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), s.token), nil
}

// guard only lets the NEXUS window talk to the API: the first request carries
// a random token which is then stored in an HttpOnly cookie.
func (s *Server) guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("NEXUS_DEV_NOAUTH") != "1" {
			ok := false
			if c, err := r.Cookie("nexus_t"); err == nil && c.Value == s.token {
				ok = true
			}
			if t := r.URL.Query().Get("t"); t == s.token {
				ok = true
				http.SetCookie(w, &http.Cookie{Name: "nexus_t", Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			}
			if !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if r.Method != http.MethodGet && r.Header.Get("X-Nexus") != "1" {
				http.Error(w, "missing header", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
		// a change made in NEXUS: synchronise it soon (debounced; nothing happens when signed out)
		if r.Method != http.MethodGet && s.app.Account != nil && !strings.HasPrefix(r.URL.Path, "/api/account") && !strings.HasPrefix(r.URL.Path, "/api/emulator") {
			s.app.Account.Touch()
		}
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	var le *LaunchError
	if errors.As(err, &le) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": le.Message, "code": le.Code, "detail": le.Detail})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

func readBody(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// readImage accepts a raw image upload and returns its bytes + extension.
func readImage(r *http.Request) ([]byte, string, error) {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 40<<20+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > 40<<20 {
		return nil, "", errors.New("Image is larger than 40 MB.")
	}
	switch http.DetectContentType(data) {
	case "image/png":
		return data, ".png", nil
	case "image/jpeg":
		return data, ".jpg", nil
	case "image/webp":
		return data, ".webp", nil
	case "image/gif":
		return data, ".gif", nil
	}
	return nil, "", errors.New("Unsupported image. Use PNG, JPG or WEBP.")
}

// readAnimated accepts the animated formats WebView2 (Chromium) plays natively.
func readAnimated(r *http.Request) ([]byte, string, string, error) {
	defer r.Body.Close()
	const limit = 200 << 20
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(data) > limit {
		return nil, "", "", errors.New("The file is larger than 200 MB. Use a shorter or smaller video for a wallpaper.")
	}
	switch ct := http.DetectContentType(data); {
	case ct == "image/gif":
		if g, err := gif.DecodeAll(bytes.NewReader(data)); err != nil || len(g.Image) < 2 {
			return nil, "", "", errors.New("This GIF has only one frame. Choose it as a static wallpaper instead.")
		}
		return data, ".gif", "image/gif", nil
	case ct == "image/webp":
		if len(data) < 64 || !bytes.Contains(data[:min(len(data), 4096)], []byte("ANIM")) {
			return nil, "", "", errors.New("This WebP image is not animated. Choose it as a static wallpaper instead.")
		}
		return data, ".webp", "image/webp", nil
	case ct == "video/webm":
		return data, ".webm", "video/webm", nil
	case ct == "video/mp4" || (len(data) > 12 && string(data[4:8]) == "ftyp"):
		return data, ".mp4", "video/mp4", nil
	}
	return nil, "", "", errors.New("Unsupported animated wallpaper. Use GIF, animated WebP, MP4 (H.264) or WebM.")
}

func (s *Server) routes() {
	m := s.mux
	a := s.app

	// ----- static -----
	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	m.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, _ := webFS.ReadFile("web/index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		static.ServeHTTP(w, r)
	})
	// Sounds and themes can be overridden by dropping files into Resources/.
	m.HandleFunc("GET /res/sounds/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.PathValue("name"))
		for _, ext := range []string{".wav", ".ogg", ".mp3"} {
			p := filepath.Join(a.Paths.Resources, "Sounds", strings.TrimSuffix(name, filepath.Ext(name))+ext)
			if fileExists(p) {
				http.ServeFile(w, r, p)
				return
			}
		}
		data, err := webFS.ReadFile("web/assets/sounds/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(data)
	})
	m.HandleFunc("GET /res/themes", func(w http.ResponseWriter, r *http.Request) {
		// user CSS themes: Resources/Themes/*.css
		entries, _ := os.ReadDir(filepath.Join(a.Paths.Resources, "Themes"))
		var out []string
		for _, e := range entries {
			if strings.EqualFold(filepath.Ext(e.Name()), ".css") {
				out = append(out, e.Name())
			}
		}
		writeJSON(w, out)
	})
	m.HandleFunc("GET /res/themes/{name}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(a.Paths.Resources, "Themes", filepath.Base(r.PathValue("name"))))
	})

	// ----- state & events -----
	m.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		st := s.state()
		// ?boot=1: the interface's first request also carries the game list of the user who is
		// picked automatically, saving a round trip before Home can show (startup measurements)
		if r.URL.Query().Get("boot") == "1" {
			g := a.Settings.Get()
			if _, ok := a.Profiles.Get(g.LastProfileID); ok && !g.AskProfile {
				stats := a.Profiles.Stats(g.LastProfileID)
				st["bootGames"] = map[string]any{"profile": g.LastProfileID, "games": a.Library.Games(), "stats": stats.Games, "recent": stats.Recent(), "status": a.Library.Status()}
			}
		}
		writeJSON(w, st)
	})
	m.HandleFunc("GET /api/events", s.events)
	m.HandleFunc("GET /api/system", func(w http.ResponseWriter, r *http.Request) {
		hasBat, pct, charging := powerInfo()
		writeJSON(w, map[string]any{
			"battery":    map[string]any{"present": hasBat, "percent": pct, "charging": charging},
			"online":     networkUp(),
			"xinput":     xinputConnected(),
			"time":       time.Now().Unix(),
			"fullscreen": a.UI.IsFullscreen(),
			"emulator":   a.Emu.State(),
		})
	})
	m.HandleFunc("GET /api/startup", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, startupMarkList()) })
	m.HandleFunc("POST /api/startup/mark", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Name, Detail string }
		_ = readBody(r, &b)
		if uiStartupMarks[b.Name] {
			if len(b.Detail) > 80 {
				b.Detail = b.Detail[:80]
			}
			a.markStartup(b.Name, b.Detail)
		}
		writeJSON(w, true)
	})
	m.HandleFunc("GET /api/storage", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.storage())
	})
	m.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if err := readBody(r, &raw); err != nil {
			writeErr(w, 400, err)
			return
		}
		st, err := a.Settings.Patch(raw)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, st)
	})
	m.HandleFunc("POST /api/settings/reset", func(w http.ResponseWriter, r *http.Request) {
		st, err := a.Settings.Update(func(g *GlobalSettings) {
			keep := g.LastProfileID
			dirs := g.Library.Dirs
			*g = DefaultSettings()
			g.LastProfileID = keep
			g.Library.Dirs = dirs
		})
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, st)
	})

	// ----- app window -----
	m.HandleFunc("POST /api/app/fullscreen", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ On bool }
		_ = readBody(r, &b)
		a.UI.SetFullscreen(b.On)
		_, _ = a.Settings.Update(func(g *GlobalSettings) { g.Display.Fullscreen = b.On })
		writeJSON(w, map[string]any{"fullscreen": a.UI.IsFullscreen()})
	})
	m.HandleFunc("POST /api/app/minimize", func(w http.ResponseWriter, r *http.Request) { a.UI.Minimize(); writeJSON(w, true) })
	m.HandleFunc("POST /api/app/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, true)
		go func() { time.Sleep(150 * time.Millisecond); a.UI.Quit() }()
	})
	m.HandleFunc("POST /api/app/log", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Level, Message string }
		_ = readBody(r, &b)
		a.Log.Printf("ui[%s]: %s", b.Level, b.Message)
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/app/open-link", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Which string }
		_ = readBody(r, &b)
		link := map[string]string{"github-device": "https://github.com/login/device", "github-apps": "https://github.com/settings/applications", "github-settings": "https://github.com/settings/profile"}[b.Which]
		if link == "" {
			writeErr(w, 400, errors.New("unknown link"))
			return
		}
		if err := openInExplorer(link); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/app/open-folder", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Which string }
		_ = readBody(r, &b)
		dir := map[string]string{
			"root": a.Paths.Root, "data": a.Paths.Data, "games": a.Paths.Games, "logs": a.Paths.Logs, "artwork": a.Paths.Artwork,
			"saves": a.Paths.Saves, "media": a.Paths.Media, "screenshots": filepath.Join(a.Paths.Media, "Screenshots"), "recordings": filepath.Join(a.Paths.Media, "Recordings"), "savebackups": filepath.Join(a.Paths.UserData, "Backups", "Saves"), "engine": a.Emu.Builds.Root(), "keys": a.Emu.KeysDir(),
			"userdir": a.Emu.DataDir(), "enginedata": a.Emu.DataDir(), "archived": a.Paths.Trash, "resources": a.Paths.Resources, "licences": a.Paths.Licences, "systemfiles": a.Sys.Root(),
			"avatars": a.avatarRoot(), "profiles": a.Paths.Profiles, "lossless": a.Emu.losslessDir(), "streamline": a.Emu.streamlineDir(), "gameprofiles": filepath.Join(a.Paths.System, "GameProfiles"),
		}[b.Which]
		if dir == "" {
			writeErr(w, 400, errors.New("unknown folder"))
			return
		}
		if b.Which == "saves" {
			a.Saves.TidyAll()
		}
		_ = os.MkdirAll(dir, 0o755)
		if err := openInExplorer(dir); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, map[string]string{"path": dir})
	})
	m.HandleFunc("POST /api/app/pick-folder", func(w http.ResponseWriter, r *http.Request) {
		p, err := a.UI.PickFolder("Choose a games folder")
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]string{"path": p})
	})

	// ----- library -----
	m.HandleFunc("GET /api/games", func(w http.ResponseWriter, r *http.Request) {
		pid := r.URL.Query().Get("profile")
		stats := a.Profiles.Stats(pid)
		writeJSON(w, map[string]any{"games": a.Library.Games(), "stats": stats.Games, "recent": stats.Recent(), "status": a.Library.Status()})
	})
	m.HandleFunc("POST /api/library/scan", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			if err := a.Library.Scan(); err != nil {
				a.Log.Printf("scan: %v", err)
			}
		}()
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/library/dirs", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Action, Path string }
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		path := strings.TrimSpace(strings.Trim(b.Path, `"`))
		if b.Action == "add" {
			if !filepath.IsAbs(path) && !(len(path) > 1 && path[1] == ':') {
				writeErr(w, 400, errors.New("Please enter a full folder path, e.g. D:\\Switch\\Games"))
				return
			}
			if !dirExists(path) {
				writeErr(w, 400, fmt.Errorf("Folder not found: %s", path))
				return
			}
		}
		st, err := a.Settings.Update(func(g *GlobalSettings) {
			var out []string
			for _, d := range g.Library.Dirs {
				if !strings.EqualFold(filepath.Clean(a.Paths.Resolve(d)), filepath.Clean(a.Paths.Resolve(path))) {
					out = append(out, d)
				}
			}
			if b.Action == "add" {
				out = append(out, a.Paths.Portable(path))
			}
			g.Library.Dirs = out
		})
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		go a.Library.Scan()
		writeJSON(w, st)
	})
	m.HandleFunc("GET /art/{key}", func(w http.ResponseWriter, r *http.Request) {
		p := a.Library.ArtPath(r.PathValue("key"), r.URL.Query().Get("src"))
		if p == "" {
			http.NotFound(w, r)
			return
		}
		if tw, err := strconv.Atoi(r.URL.Query().Get("w")); err == nil && tw >= 64 && tw <= 1024 {
			p = a.Thumbnail(p, tw) // tiles: cached, downscaled copy
		}
		w.Header().Set("Cache-Control", "max-age=86400")
		http.ServeFile(w, r, p)
	})
	m.HandleFunc("POST /api/games/{key}/art", func(w http.ResponseWriter, r *http.Request) {
		data, ext, err := readImage(r)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := a.Library.SetCustomArt(r.PathValue("key"), data, ext); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	m.HandleFunc("DELETE /api/games/{key}/art", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Library.ClearCustomArt(r.PathValue("key")); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	// a game's Cards background or title picture (kind background | logo; DELETE restores none)
	m.HandleFunc("POST /api/games/{key}/art/{kind}", func(w http.ResponseWriter, r *http.Request) {
		data, ext, err := readImage(r)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := a.Library.SetExtraArt(r.PathValue("key"), r.PathValue("kind"), data, ext); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	m.HandleFunc("DELETE /api/games/{key}/art/{kind}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Library.SetExtraArt(r.PathValue("key"), r.PathValue("kind"), nil, ""); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	m.HandleFunc("POST /api/games/{key}/art-options", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Preference, Fit string }
		_ = readBody(r, &b)
		if b.Preference != "" && b.Preference != "auto" && b.Preference != "custom" && b.Preference != "embedded" && b.Preference != "online" {
			writeErr(w, 400, errors.New("invalid artwork preference"))
			return
		}
		if err := a.Library.SetArtOptions(r.PathValue("key"), b.Preference, b.Fit); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	m.HandleFunc("DELETE /api/games/{key}/online-art", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Library.RemoveOnlineArt(r.PathValue("key")); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	m.HandleFunc("POST /api/games/{key}/refresh", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		go func() {
			if err := a.Library.RefreshGame(key); err != nil {
				a.Log.Printf("refresh %s: %v", key, err)
				a.Events.Emit("artwork.online", map[string]any{"key": key, "ok": false, "message": err.Error()})
			}
		}()
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/games/{key}/hidden", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Hidden bool }
		_ = readBody(r, &b)
		if err := a.Library.SetHidden(r.PathValue("key"), b.Hidden); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	m.HandleFunc("POST /api/games/{key}/title", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Title string }
		_ = readBody(r, &b)
		if err := a.Library.SetTitle(r.PathValue("key"), b.Title); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, _ := a.Library.Get(r.PathValue("key"))
		writeJSON(w, g)
	})
	m.HandleFunc("POST /api/games/{key}/reveal", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("Game not found."))
			return
		}
		if err := revealInExplorer(g.Path); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, true)
	})

	// ----- emulator -----
	m.HandleFunc("POST /api/launch", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Key, ProfileID string
			Force          bool
		}
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		g, ok := a.Library.Get(b.Key)
		if !ok {
			writeErr(w, 404, &LaunchError{Code: "not-found", Message: "That game is no longer in the library.", Detail: "Try refreshing the library."})
			return
		}
		if fatal, warns := a.Emu.Preflight(&g); fatal != nil {
			writeErr(w, 409, fatal)
			return
		} else if len(warns) > 0 && !b.Force {
			w.WriteHeader(409)
			writeJSON(w, map[string]any{"warnings": warns})
			return
		}
		if err := a.Emu.Launch(&g, b.ProfileID); err != nil {
			writeErr(w, 409, err)
			return
		}
		writeJSON(w, a.Emu.State())
	})
	m.HandleFunc("POST /api/emulator/stop", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Force bool }
		_ = readBody(r, &b)
		if err := a.Emu.Stop(b.Force); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/emulator/focus", func(w http.ResponseWriter, r *http.Request) { a.Emu.FocusRunning(); writeJSON(w, true) })
	m.HandleFunc("POST /api/emulator/suspend", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ On bool }
		_ = readBody(r, &b)
		if err := a.Emu.Suspend(b.On); err != nil {
			writeErr(w, 409, err)
			return
		}
		writeJSON(w, true)
	})
	// the loading screen has faded out: show the game window (engine 0.4)
	// shader preparation (shaderprep.go)
	m.HandleFunc("POST /api/games/{key}/prepare-shaders", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Force bool }
		_ = readBody(r, &b)
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("No such game."))
			return
		}
		if err := a.Emu.PrepareShadersNow(&g, b.Force); err != nil {
			writeErr(w, 409, err)
			return
		}
		writeJSON(w, true)
	})
	// portable shader data between your own devices (shadercache.go)
	m.HandleFunc("GET /api/games/{key}/shaders/export", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("No such game."))
			return
		}
		var buf bytes.Buffer
		if _, err := a.Emu.ExportShaders(&g, &buf); err != nil {
			writeErr(w, 409, err)
			return
		}
		name := shaderPackFileName(g.Title, g.TitleID)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Write(buf.Bytes())
	})
	m.HandleFunc("POST /api/games/{key}/shaders/import", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("No such game."))
			return
		}
		res, err := a.Emu.ImportShaders(r.Body, func(id string) *Game {
			if strings.EqualFold(id, g.TitleID) {
				return &g
			}
			return nil
		})
		if err != nil {
			writeErr(w, 409, err)
			return
		}
		writeJSON(w, res)
	})
	// the shader cache NEXUS uses (shared by the PC's NEXUS users unless turned off; sharedcache.go)
	m.HandleFunc("GET /api/shaders/cache", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Emu.SharedCacheStatus())
	})
	m.HandleFunc("GET /api/games/{key}/shaders/cache", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, http.StatusNotFound, errors.New("game not found"))
			return
		}
		t, have := a.Emu.GameShaderCache(g.TitleID)
		writeJSON(w, map[string]any{"have": have, "cache": t})
	})
	m.HandleFunc("POST /api/shaders/prepare/cancel", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Emu.cancelPrep())
	})
	m.HandleFunc("POST /api/emulator/skip-precache", func(w http.ResponseWriter, r *http.Request) {
		a.Emu.SkipPrecache()
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/emulator/reveal", func(w http.ResponseWriter, r *http.Request) {
		if st := a.Emu.State(); st.Running {
			if h, err := a.Emu.Host(); err == nil {
				h.Reveal()
			}
		}
		writeJSON(w, true)
	})
	// ----- NEXUS updates (updater.go)
	m.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"info": a.Updater.Last(), "job": a.Updater.Job(), "settings": a.Settings.Get().Updates,
			"hasToken": a.Updater.HasToken(), "failed": a.updateFailed, "current": AppVersion, "canStoreToken": runtime.GOOS == "windows"})
	})
	m.HandleFunc("POST /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Updater.Check())
	})
	m.HandleFunc("POST /api/update/download", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Updater.Download(); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Updater.Job())
	})
	m.HandleFunc("POST /api/update/install", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Updater.Install(); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/update/later", func(w http.ResponseWriter, r *http.Request) {
		v := a.Updater.Last().Latest
		_, _ = a.Settings.Update(func(s *GlobalSettings) { s.Updates.SkipVersion = v })
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/update/settings", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			AutoCheck *bool
			Repo      *string
			Token     *string
		}
		_ = readBody(r, &b)
		if b.Token != nil {
			if err := a.Updater.SetToken(*b.Token); err != nil {
				writeErr(w, 400, err)
				return
			}
		}
		if b.Repo != nil && *b.Repo != "" && !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(*b.Repo) {
			writeErr(w, 400, errors.New("The release repository is written as owner/name."))
			return
		}
		_, _ = a.Settings.Update(func(s *GlobalSettings) {
			if b.AutoCheck != nil {
				s.Updates.AutoCheck = *b.AutoCheck
			}
			if b.Repo != nil {
				s.Updates.Repo = strings.TrimSpace(*b.Repo)
			}
		})
		writeJSON(w, true)
	})
	// ----- NVIDIA DLSS files (dlss.go)
	m.HandleFunc("GET /api/dlss", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Emu.DLSSStatus())
	})
	m.HandleFunc("POST /api/dlss/prepare", func(w http.ResponseWriter, r *http.Request) {
		offer, err := a.Emu.PrepareDLSS()
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, offer)
	})
	m.HandleFunc("POST /api/dlss/install", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Accepted bool }
		_ = readBody(r, &b)
		if err := a.Emu.InstallDLSS(b.Accepted); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Emu.DLSSStatus())
	})
	// ----- Media (media.go)
	m.HandleFunc("GET /api/media", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": a.Media.List(), "screenshots": a.Media.ScreenshotsDir(), "recordings": a.Media.RecordingsDir(),
			"recordingAvailable": false})
	})
	m.HandleFunc("GET /media/file", func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Media.Path(r.URL.Query().Get("id"))
		if err != nil {
			writeErr(w, 404, err)
			return
		}
		if wd, _ := strconv.Atoi(r.URL.Query().Get("w")); wd > 0 && mediaImageExts[strings.ToLower(filepath.Ext(p))] {
			p = a.Thumbnail(p, wd)
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, p) // ranges: recordings can be played and seeked
	})
	m.HandleFunc("POST /api/media/delete", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID      string
			Confirm bool
		}
		_ = readBody(r, &b)
		if !b.Confirm {
			writeErr(w, 400, errors.New("Deleting a capture needs confirmation."))
			return
		}
		if err := a.Media.Delete(b.ID); err != nil {
			writeErr(w, 400, err)
			return
		}
		a.Events.Emit("media.changed", nil)
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/media/reveal", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ID string }
		_ = readBody(r, &b)
		p, err := a.Media.Path(b.ID)
		if err != nil {
			writeErr(w, 404, err)
			return
		}
		if err := revealInExplorer(p); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/media/open", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ID string }
		_ = readBody(r, &b)
		p, err := a.Media.Path(b.ID)
		if err != nil {
			writeErr(w, 404, err)
			return
		}
		if err := openWithDefaultApp(p); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, true)
	})
	// ----- controllers (controllers.go)
	m.HandleFunc("GET /api/controllers", func(w http.ResponseWriter, r *http.Request) {
		pid := r.URL.Query().Get("profile")
		list, errText := a.Controllers.List(pid)
		writeJSON(w, map[string]any{"pads": list, "error": errText})
	})
	m.HandleFunc("POST /api/controllers/players", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ProfileID string
			Keys      []string
		}
		_ = readBody(r, &b)
		if err := a.Controllers.SetPlayers(b.ProfileID, b.Keys); err != nil {
			writeErr(w, 400, err)
			return
		}
		list, _ := a.Controllers.List(b.ProfileID)
		writeJSON(w, map[string]any{"pads": list})
	})
	m.HandleFunc("POST /api/controllers/forget", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Key string }
		_ = readBody(r, &b)
		if err := a.Controllers.Forget(b.Key); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/controllers/update", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Key         string
			Nickname    *string
			Layout      *GameLayout
			ClearLayout bool
		}
		_ = readBody(r, &b)
		kp, err := a.Controllers.Update(b.Key, b.Nickname, b.Layout, b.ClearLayout)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, kp)
	})
	m.HandleFunc("POST /api/emulator/volume", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Percent int }
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		if b.Percent < 0 || b.Percent > 200 {
			writeErr(w, 400, errors.New("volume must be between 0 and 200%"))
			return
		}
		if err := a.Emu.SetVolume(b.Percent); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, map[string]any{"volume": a.Emu.Volume(), "state": a.Emu.State()})
	})

	// ----- NEXUS engine -----
	m.HandleFunc("GET /api/engine", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.engineInfo())
	})
	m.HandleFunc("POST /api/engine/install", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Path, URL, SHA256 string
			Activate          *bool
		}
		_ = readBody(r, &b)
		activate := b.Activate == nil || *b.Activate
		var err error
		switch {
		case b.URL != "":
			_, err = a.Emu.Builds.InstallURL(b.URL, b.SHA256, activate)
		default:
			path := strings.Trim(strings.TrimSpace(b.Path), `"`)
			if path == "" {
				p, perr := a.UI.PickFile("Select a NEXUS engine build", "NEXUS engine build (*.zip)|*.zip|")
				if perr != nil {
					writeJSON(w, map[string]any{"cancelled": true})
					return
				}
				path = p
			}
			_, err = a.Emu.Builds.InstallZip(path, activate)
		}
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, s.engineInfo())
	})
	m.HandleFunc("POST /api/engine/activate", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Version string }
		_ = readBody(r, &b)
		if a.Emu.State().Running {
			writeErr(w, 409, errors.New("Close the running game first."))
			return
		}
		if err := a.Emu.Builds.Activate(b.Version); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, s.engineInfo())
	})
	m.HandleFunc("POST /api/engine/rollback", func(w http.ResponseWriter, r *http.Request) {
		if a.Emu.State().Running {
			writeErr(w, 409, errors.New("Close the running game first."))
			return
		}
		if _, err := a.Emu.Builds.Rollback(); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, s.engineInfo())
	})
	m.HandleFunc("POST /api/engine/remove", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Version string }
		_ = readBody(r, &b)
		if err := a.Emu.Builds.Remove(b.Version); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, s.engineInfo())
	})
	m.HandleFunc("POST /api/engine/open-folder", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Which string }
		_ = readBody(r, &b)
		dir := a.Emu.DataDir()
		if b.Which == "builds" {
			dir = a.Emu.Builds.BuildsDir()
		}
		_ = os.MkdirAll(dir, 0o755)
		if err := openInExplorer(dir); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, true)
	})
	// ----- hardware and "Optimise for my device"
	m.HandleFunc("GET /api/hardware", func(w http.ResponseWriter, r *http.Request) {
		h := a.DetectHardware(r.URL.Query().Get("refresh") == "1")
		writeJSON(w, map[string]any{"hardware": h, "class": classifyDevice(h)})
	})
	optReply := func(w http.ResponseWriter, plan OptPlan, game string) {
		writeJSON(w, map[string]any{"plan": plan, "backup": a.Emu.OptBackupInfo(game)})
	}
	m.HandleFunc("GET /api/optimise", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		prof := OptProfile(q.Get("profile"))
		if prof == "" {
			prof = OptBalanced
		}
		plan, err := a.Emu.Plan(prof, q.Get("game"))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		optReply(w, plan, q.Get("game"))
	})
	m.HandleFunc("POST /api/optimise", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Profile OptProfile
			Game    string
			Only    []string
		}
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		plan, err := a.Emu.ApplyPlan(b.Profile, b.Game, b.Only)
		if err != nil {
			writeErr(w, 409, err)
			return
		}
		if b.Game != "" {
			a.Events.Emit("library.changed", nil)
		}
		optReply(w, plan, b.Game)
	})
	m.HandleFunc("POST /api/optimise/restore", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Game string }
		_ = readBody(r, &b)
		if err := a.Emu.RestoreOptimisation(b.Game); err != nil {
			writeErr(w, 409, err)
			return
		}
		writeJSON(w, true)
	})
	// the last game session ended unexpectedly: what to tell the user, once (crashreport.go)
	m.HandleFunc("GET /api/crash/last", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.takeCrashNotice())
	})
	m.HandleFunc("POST /api/app/reveal-log", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Path string }
		_ = readBody(r, &b)
		rel, err := filepath.Rel(a.Paths.Logs, b.Path)
		if err != nil || strings.HasPrefix(rel, "..") || !fileExists(b.Path) {
			writeErr(w, 400, errors.New("That log file is not available."))
			return
		}
		if err := revealInExplorer(b.Path); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, true)
	})
	// ----- experimental: NEXUS Device Linking (404 unless the feature is on)
	link := func(w http.ResponseWriter) *DeviceLink {
		if a.Link == nil {
			writeErr(w, 404, errors.New("not available"))
		}
		return a.Link
	}
	m.HandleFunc("GET /api/link", func(w http.ResponseWriter, r *http.Request) {
		l := link(w)
		if l == nil {
			return
		}
		users := map[string]string{}
		for _, p := range a.Profiles.List() {
			users[p.ID] = UserID(p.ID)
		}
		writeJSON(w, map[string]any{"device": map[string]string{"id": l.id.DeviceID, "name": l.id.DeviceName}, "port": l.Port,
			"devices": l.Devices(), "requests": l.PendingRequests(), "conflicts": l.Conflicts(), "userIds": users})
	})
	m.HandleFunc("POST /api/link/code", func(w http.ResponseWriter, r *http.Request) {
		l := link(w)
		if l == nil {
			return
		}
		var b struct{ Profile string }
		_ = readBody(r, &b)
		code, exp, err := l.StartPairing(b.Profile)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"code": code, "expires": exp.Unix(), "userId": UserID(b.Profile)})
	})
	m.HandleFunc("POST /api/link/decide", func(w http.ResponseWriter, r *http.Request) {
		l := link(w)
		if l == nil {
			return
		}
		var b struct {
			ID      string
			Approve bool
		}
		_ = readBody(r, &b)
		if err := l.Decide(b.ID, b.Approve); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/link/pair", func(w http.ResponseWriter, r *http.Request) {
		l := link(w)
		if l == nil {
			return
		}
		var b struct{ UserID, Code, Addr string }
		_ = readBody(r, &b)
		addr := b.Addr
		if addr == "" {
			var err error
			if addr, err = Discover(profileIDFromUserID(b.UserID), linkDefaultPort, 3*time.Second); err != nil {
				writeErr(w, 400, err)
				return
			}
		}
		d, err := l.PairWith(addr, b.UserID, b.Code)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		res, serr := l.SyncWith(d.DeviceID, addr)
		writeJSON(w, map[string]any{"device": d, "sync": res, "syncError": errString(serr)})
	})
	m.HandleFunc("POST /api/link/sync", func(w http.ResponseWriter, r *http.Request) {
		l := link(w)
		if l == nil {
			return
		}
		var b struct{ Device string }
		_ = readBody(r, &b)
		res, err := l.SyncWith(b.Device, "")
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, res)
	})
	m.HandleFunc("POST /api/link/remove", func(w http.ResponseWriter, r *http.Request) {
		l := link(w)
		if l == nil {
			return
		}
		var b struct{ Device string }
		_ = readBody(r, &b)
		if err := l.RemoveDevice(b.Device); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/link/resolve", func(w http.ResponseWriter, r *http.Request) {
		l := link(w)
		if l == nil {
			return
		}
		var b struct{ ID, Keep string }
		_ = readBody(r, &b)
		if err := l.ResolveConflict(b.ID, b.Keep); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/dev/fake-home", func(w http.ResponseWriter, r *http.Request) {
		// development only: the stand-in engine's Home key (no effect with the real engine)
		if !fakeEngineEnabled() || fakeLast == nil {
			writeErr(w, 404, errors.New("not available"))
			return
		}
		fakeLast.PressHome()
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/dev/fake-screenshot", func(w http.ResponseWriter, r *http.Request) {
		if !fakeEngineEnabled() || fakeLast == nil {
			writeErr(w, 404, errors.New("not available"))
			return
		}
		if err := fakeLast.PressScreenshot(); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/dev/fake-record", func(w http.ResponseWriter, r *http.Request) {
		// development only: the stand-in engine's record key (F12)
		if !fakeEngineEnabled() || fakeLast == nil {
			writeErr(w, 404, errors.New("not available"))
			return
		}
		fakeLast.PressRecord()
		writeJSON(w, true)
	})
	// ----- platform (platform_mobile.go): device readiness on mobile; nothing on Windows
	m.HandleFunc("GET /api/platform", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.PlatformInfo()) })
	m.HandleFunc("POST /api/dev/fake-device", func(w http.ResponseWriter, r *http.Request) {
		// development only: what the Android app reports, to test the interface on a PC
		if !fakeEngineEnabled() {
			writeErr(w, 404, errors.New("not available"))
			return
		}
		var b struct {
			Device    *DeviceProfile
			Lifecycle int
			Thermal   *int
			Trim      int
		}
		_ = readBody(r, &b)
		if b.Device != nil {
			a.SetDevice(*b.Device)
		}
		if b.Lifecycle != 0 {
			a.OnLifecycle(LifecycleEvent(b.Lifecycle))
		}
		if b.Thermal != nil {
			a.OnThermal(*b.Thermal)
		}
		if b.Trim != 0 {
			a.OnTrimMemory(b.Trim)
		}
		writeJSON(w, a.PlatformInfo())
	})
	// ----- screen recording (recorder.go) and screenshots on request
	m.HandleFunc("GET /api/recording", func(w http.ResponseWriter, r *http.Request) {
		caps := a.Recorder.Caps()
		eff := a.Recorder.settingsFor(caps)
		writeJSON(w, map[string]any{"status": a.Recorder.Status(), "caps": caps, "settings": a.Settings.Get().Recording,
			"effective": eff, "bitrate": RecordingBitrate(eff), "folder": firstNonEmpty(a.Settings.Get().Recording.Folder, a.Media.RecordingsDir())})
	})
	m.HandleFunc("POST /api/recording/toggle", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Recorder.Toggle(); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Recorder.Status())
	})
	m.HandleFunc("POST /api/recording/folder", func(w http.ResponseWriter, r *http.Request) {
		p, err := a.UI.PickFolder("Choose where recordings are saved")
		if err != nil || p == "" {
			writeJSON(w, map[string]any{"cancelled": true})
			return
		}
		writeJSON(w, map[string]string{"path": p})
	})
	m.HandleFunc("POST /api/emulator/screenshot", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Emu.Screenshot(); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("GET /api/perf", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"sample": perf.Sample(a.Emu), "metrics": PerfMetrics,
			"text":     formatPerf(perf.Sample(a.Emu), a.Settings.Get().PerfMonitor.Metrics, "    "),
			"segments": perfSegments(perf.Sample(a.Emu), a.Settings.Get().PerfMonitor.Metrics)})
	})
	// ----- NEXUS engine data: setup and import of other emulators' data (enginedata.go)
	m.HandleFunc("GET /api/engine-data", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Emu.EngineDataStatus())
	})
	// ----- frame generation, DLSS, FSR 4 (enhancements.go)
	m.HandleFunc("GET /api/enhancements", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Emu.Enhancements())
	})
	// Lossless Scaling setup (lossless.go): status, then copying from the user's own installation
	m.HandleFunc("GET /api/enhancements/lossless", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Emu.LosslessStatus(r.URL.Query().Get("folder")))
	})
	m.HandleFunc("POST /api/enhancements/lossless", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Path       string
			Pick       bool // choose Lossless.dll itself
			PickFolder bool // choose the Lossless Scaling, Steam library or Steam folder
			Locate     bool // with PickFolder: only look, do not copy yet
			Replace    bool
		}
		_ = readBody(r, &b)
		if b.Pick || b.PickFolder {
			var p string
			var err error
			if b.PickFolder {
				p, err = a.UI.PickFolder("Choose the Lossless Scaling folder, or your Steam folder")
			} else {
				p, err = a.UI.PickFile("Choose Lossless.dll from the Lossless Scaling folder", "Lossless.dll|Lossless.dll|")
			}
			if err != nil || p == "" {
				writeJSON(w, map[string]any{"cancelled": true})
				return
			}
			b.Path = p
		}
		if b.Locate {
			writeJSON(w, a.Emu.LosslessStatus(b.Path))
			return
		}
		st, err := a.Emu.SetupLossless(b.Path, b.Replace)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error(), "status": st})
			return
		}
		writeJSON(w, map[string]any{"status": st})
	})
	// ----- controller layouts for games (controls.go)
	m.HandleFunc("GET /api/controls/layouts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"switchButtons": SwitchButtons, "positions": PhysicalPositions, "bases": BaseLayouts})
	})
	m.HandleFunc("POST /api/profiles/{id}/game-layout", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := a.Profiles.Get(id); !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		var l GameLayout
		if err := readBody(r, &l); err != nil {
			writeErr(w, 400, err)
			return
		}
		l, err := normalizeLayout(l)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		// clear first: a JSON merge would otherwise keep buttons that were removed
		_, _ = a.Profiles.Update(id, []byte(`{"controls":{"gameLayout":{"buttons":null}}}`))
		patch, _ := json.Marshal(map[string]any{"controls": map[string]any{"gameLayout": l}})
		p, err := a.Profiles.Update(id, patch)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, p)
	})
	m.HandleFunc("GET /api/games/{key}/layout", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("Game not found."))
			return
		}
		var pr *Profile
		if p, ok := a.Profiles.Get(r.URL.Query().Get("profile")); ok {
			pr = &p
		}
		l, src, preset := a.Emu.EffectiveLayout(&g, pr)
		writeJSON(w, map[string]any{"layout": l, "source": src, "preset": preset, "own": a.Library.GP.Get(g.Key).Controls, "resolved": l.Resolve()})
	})
	m.HandleFunc("POST /api/games/{key}/layout", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("Game not found."))
			return
		}
		var b struct{ Layout *GameLayout }
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		if b.Layout != nil {
			l, err := normalizeLayout(*b.Layout)
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			b.Layout = &l
		}
		a.Library.GP.Update(g.Key, func(p *GameProfile) { p.Controls = b.Layout })
		writeJSON(w, true)
	})
	m.HandleFunc("GET /api/compat", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Compat.All())
	})
	m.HandleFunc("POST /api/games/{key}/compat", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("Game not found."))
			return
		}
		var b struct{ Rating, Note string }
		_ = readBody(r, &b)
		if err := a.Compat.Rate(g.Key, g.Title, b.Rating, strings.TrimSpace(b.Note)); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Compat.Get(g.Key))
	})

	// ----- profiles -----
	// ----- NEXUS emulation settings (global + per game) -----
	m.HandleFunc("GET /api/engine-settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Emu.GlobalSettingsView())
	})
	m.HandleFunc("POST /api/engine-settings", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Settings map[string]*string `json:"settings"`
			ResetAll bool               `json:"resetAll"`
		}
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		if b.ResetAll {
			b.Settings = map[string]*string{}
			for k := range a.Emu.GlobalNexusSettings() {
				b.Settings[k] = nil
			}
		}
		if err := a.Emu.SetGlobal(b.Settings); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Emu.GlobalSettingsView())
	})
	m.HandleFunc("GET /api/games/{key}/settings", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("Game not found."))
			return
		}
		writeJSON(w, a.Emu.GameSettingsView(g))
	})
	m.HandleFunc("POST /api/games/{key}/settings", func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.Library.Get(r.PathValue("key"))
		if !ok {
			writeErr(w, 404, errors.New("Game not found."))
			return
		}
		var b struct {
			Settings     map[string]*string `json:"settings"`
			InputProfile *string            `json:"inputProfile"`
			ResetAll     bool               `json:"resetAll"`
		}
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		if b.ResetAll {
			b.Settings = map[string]*string{}
			for k := range gameNexusSettings(a.Library.GP.Get(g.Key)) {
				b.Settings[k] = nil
			}
			empty := ""
			b.InputProfile = &empty
			_, _ = a.Library.GP.Update(g.Key, func(p *GameProfile) { p.Emu, p.Ryu = nil, nil })
		}
		if len(b.Settings) > 0 || b.InputProfile != nil {
			if err := a.Emu.SetGameSettings(g, b.Settings, b.InputProfile); err != nil {
				writeErr(w, 400, err)
				return
			}
		}
		g, _ = a.Library.Get(g.Key)
		writeJSON(w, a.Emu.GameSettingsView(g))
	})

	m.HandleFunc("GET /api/artwork-db", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"cache": a.Library.artCache().Stats(), "builtin": a.Library.BuiltinInfo()})
	})
	m.HandleFunc("POST /api/artwork-db/clear", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Library.artCache().Clear(); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, map[string]any{"cache": a.Library.artCache().Stats()})
	})

	// ----- system files (keys + firmware) -----
	m.HandleFunc("GET /api/system-files", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Sys.Status())
	})
	pick := func(w http.ResponseWriter, path, title, filter string, folder bool) (string, bool) {
		if path != "" {
			return path, true
		}
		var p string
		var err error
		if folder {
			p, err = a.UI.PickFolder(title)
		} else {
			p, err = a.UI.PickFile(title, filter)
		}
		if err != nil {
			writeJSON(w, map[string]any{"cancelled": true})
			return "", false
		}
		return p, true
	}
	m.HandleFunc("POST /api/system-files/keys", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Prod, Title, Engine string }
		_ = readBody(r, &b)
		for _, t := range a.Sys.engineTargets() {
			if b.Engine != "" && t.Engine == b.Engine {
				b.Prod = filepath.Join(t.KeysDir, "prod.keys")
				if tk := filepath.Join(t.KeysDir, "title.keys"); fileExists(tk) {
					b.Title = tk
				}
			}
		}
		prod, ok := pick(w, b.Prod, "Select your prod.keys", "Key files (*.keys)|*.keys|All files (*.*)|*.*|", false)
		if !ok {
			return
		}
		ki, err := a.Sys.InstallKeys(prod, b.Title)
		if err != nil {
			w.WriteHeader(400)
			writeJSON(w, map[string]any{"error": err.Error(), "keys": ki})
			return
		}
		writeJSON(w, map[string]any{"keys": ki, "status": a.Sys.Status()})
	})
	m.HandleFunc("POST /api/system-files/title-keys", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Path string }
		_ = readBody(r, &b)
		p, ok := pick(w, b.Path, "Select your title.keys", "Key files (*.keys)|*.keys|All files (*.*)|*.*|", false)
		if !ok {
			return
		}
		if !fileExists(a.Sys.ProdKeys()) {
			writeErr(w, 400, errors.New("Install prod.keys first."))
			return
		}
		if _, err := a.Sys.InstallKeys(a.Sys.ProdKeys(), p); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Sys.Status())
	})
	m.HandleFunc("POST /api/system-files/firmware", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Path   string
			Zip    bool
			Engine string
		}
		_ = readBody(r, &b)
		for _, t := range a.Sys.engineTargets() {
			if b.Engine != "" && t.Engine == b.Engine {
				b.Path = t.FirmwareDir
			}
		}
		p, ok := pick(w, b.Path, "Select your firmware", "Firmware archive (*.zip)|*.zip|", !b.Zip)
		if !ok {
			return
		}
		if err := a.Sys.InstallFirmware(p); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Sys.Job())
	})
	m.HandleFunc("POST /api/system-files/sync", func(w http.ResponseWriter, r *http.Request) {
		if st := a.Emu.State(); st.Running {
			writeErr(w, 409, errors.New("Close the running game first."))
			return
		}
		writeJSON(w, map[string]any{"warnings": a.Sys.Sync(), "status": a.Sys.Status()})
	})
	m.HandleFunc("POST /api/system-files/remove", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Which   string
			Engines bool
		}
		_ = readBody(r, &b)
		dst, err := a.Sys.Remove(b.Which, b.Engines)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"archivedTo": dst, "status": a.Sys.Status()})
	})

	m.HandleFunc("GET /api/profiles/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.ProfileSummary(r.PathValue("id")))
	})
	m.HandleFunc("POST /api/profiles/{id}/playtime", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Key            string
			Hours, Minutes int64
		}
		_ = readBody(r, &b)
		if b.Key == "" || b.Minutes < 0 || b.Minutes > 59 || b.Hours < 0 {
			writeErr(w, 400, errors.New("Enter hours and minutes (0-59)."))
			return
		}
		g, err := a.Profiles.SetPlaytime(r.PathValue("id"), b.Key, b.Hours*3600+b.Minutes*60)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		a.Events.Emit("stats.changed", map[string]any{"profileId": r.PathValue("id"), "key": b.Key})
		writeJSON(w, g)
	})

	m.HandleFunc("GET /api/profiles", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.Profiles.List()) })
	m.HandleFunc("POST /api/profiles", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name   string
			Avatar Avatar
		}
		if err := readBody(r, &b); err != nil {
			writeErr(w, 400, err)
			return
		}
		p, err := a.Profiles.Create(b.Name, b.Avatar)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		a.Events.Emit("profiles.changed", nil)
		writeJSON(w, p)
	})
	m.HandleFunc("PATCH /api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if err := readBody(r, &raw); err != nil {
			writeErr(w, 400, err)
			return
		}
		p, err := a.Profiles.Update(r.PathValue("id"), raw)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		a.Saves.SyncProfileName(p.ID) // keep "<id> - <name>" in step with the name
		writeJSON(w, p)
	})
	m.HandleFunc("DELETE /api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Confirm string }
		_ = readBody(r, &b)
		p, ok := a.Profiles.Get(r.PathValue("id"))
		if !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		if b.Confirm != p.Name {
			writeErr(w, 400, errors.New("Confirmation did not match the profile name."))
			return
		}
		if st := a.Emu.State(); st.Running && st.ProfileID == p.ID {
			writeErr(w, 409, errors.New("This profile is playing a game right now."))
			return
		}
		archive, err := a.Profiles.Delete(p.ID)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		a.Controllers.RemoveProfile(p.ID)
		a.Log.Printf("profile %s (%s) removed; data archived to %s", p.Name, p.ID, archive)
		a.Events.Emit("profiles.changed", nil)
		writeJSON(w, map[string]string{"archive": archive})
	})
	m.HandleFunc("POST /api/profiles/{id}/select", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := a.Profiles.Get(id); !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		_, _ = a.Settings.Update(func(g *GlobalSettings) { g.LastProfileID = id })
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/profiles/{id}/avatar", func(w http.ResponseWriter, r *http.Request) {
		data, ext, err := readImage(r)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		p, err := a.Profiles.SetAvatarImage(r.PathValue("id"), data, ext)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		// remember how a composed icon was built so the picker can re-open it
		q := r.URL.Query()
		meta, _ := json.Marshal(map[string]any{"avatar": map[string]any{"char": q.Get("char"), "bg": q.Get("bg"), "frame": q.Get("frame"), "color": firstNonEmpty(q.Get("color"), p.Avatar.Color)}})
		if p, err = a.Profiles.Update(p.ID, meta); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, p)
	})
	m.HandleFunc("GET /api/avatars", func(w http.ResponseWriter, r *http.Request) {
		st := a.Avatars.Status()
		var palette []string
		if b, err := os.ReadFile(filepath.Join(a.Paths.Resources, "ProfileIcons", "palette.txt")); err == nil {
			for _, f := range strings.Fields(string(b)) {
				if len(f) == 7 && f[0] == '#' {
					palette = append(palette, f)
				}
			}
		}
		writeJSON(w, map[string]any{"list": a.Avatars.List(), "categories": a.Avatars.Categories(), "status": st, "palette": palette,
			"firmware": a.Emu.FirmwarePresent(), "keys": a.Emu.KeysPresent(), "message": st.Message, "folder": a.avatarRoot()})
	})
	m.HandleFunc("POST /api/avatars/provision", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Avatars.Ensure(true))
	})
	m.HandleFunc("POST /api/avatars/import", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Path string }
		_ = readBody(r, &b)
		path := strings.Trim(strings.TrimSpace(b.Path), `"`)
		if path == "" {
			p, err := a.UI.PickFolder("Choose a folder of profile icons")
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			path = p
		}
		n, cats, err := a.Avatars.ImportFolder(path)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"count": n, "categories": cats})
	})
	m.HandleFunc("POST /api/avatars/import-zip", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		n, cats, err := a.Avatars.ImportZip(r.Body, r.URL.Query().Get("name"))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"count": n, "categories": cats})
	})
	m.HandleFunc("DELETE /api/avatars/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Avatars.RemoveCategory(r.PathValue("id")); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("GET /avatarres/{path...}", func(w http.ResponseWriter, r *http.Request) {
		rel := filepath.Clean(filepath.FromSlash(r.PathValue("path")))
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=86400")
		http.ServeFile(w, r, filepath.Join(a.Paths.Resources, "ProfileIcons", rel))
	})
	m.HandleFunc("GET /avatarlib/{path...}", func(w http.ResponseWriter, r *http.Request) {
		rel := filepath.Clean(filepath.FromSlash(r.PathValue("path")))
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=86400")
		http.ServeFile(w, r, filepath.Join(a.avatarRoot(), rel))
	})
	m.HandleFunc("GET /avatar/{id}", func(w http.ResponseWriter, r *http.Request) {
		p := a.Profiles.AvatarPath(r.PathValue("id"))
		if p == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, p)
	})
	// Wallpapers: a static image and, optionally, an animated one (GIF, animated WebP, MP4, WebM)
	// with a still frame. Files live in System/Wallpapers; nothing outside it is ever removed.
	setWallpaper := func(w http.ResponseWriter, id string, fields map[string]any, remove ...string) {
		patch, _ := json.Marshal(map[string]any{"theme": map[string]any{"wallpaper": fields}})
		p, err := a.Profiles.Update(id, patch)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		for _, old := range remove {
			if old != "" && strings.HasPrefix(old, id+"-") && !strings.ContainsAny(old, `/\`) {
				_ = os.Remove(filepath.Join(a.Paths.Wallpapers, old))
			}
		}
		writeJSON(w, p)
	}
	m.HandleFunc("POST /api/profiles/{id}/wallpaper", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		pr, ok := a.Profiles.Get(id)
		if !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		cur := pr.Theme.Wallpaper
		switch r.URL.Query().Get("kind") {
		case "animated":
			data, ext, mime, err := readAnimated(r)
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			name := fmt.Sprintf("%s-anim-%d%s", id, time.Now().UnixMilli(), ext)
			if err := os.WriteFile(filepath.Join(a.Paths.Wallpapers, name), data, 0o644); err != nil {
				writeErr(w, 500, err)
				return
			}
			setWallpaper(w, id, map[string]any{"animated": name, "animatedType": mime, "poster": "", "use": "animated"}, cur.Animated, cur.Poster)
		case "poster":
			data, ext, err := readImage(r)
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			if cur.Animated == "" {
				writeErr(w, 400, errors.New("No animated wallpaper to attach the still frame to."))
				return
			}
			name := fmt.Sprintf("%s-poster-%d%s", id, time.Now().UnixMilli(), ext)
			if err := os.WriteFile(filepath.Join(a.Paths.Wallpapers, name), data, 0o644); err != nil {
				writeErr(w, 500, err)
				return
			}
			setWallpaper(w, id, map[string]any{"poster": name}, cur.Poster)
		default:
			data, ext, err := readImage(r)
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			name := fmt.Sprintf("%s-%d%s", id, time.Now().UnixMilli(), ext)
			if err := os.WriteFile(filepath.Join(a.Paths.Wallpapers, name), data, 0o644); err != nil {
				writeErr(w, 500, err)
				return
			}
			setWallpaper(w, id, map[string]any{"file": name, "use": "static"}, cur.File)
		}
	})
	m.HandleFunc("POST /api/profiles/{id}/wallpaper/use", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		pr, ok := a.Profiles.Get(id)
		if !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		var b struct{ Use string }
		_ = readBody(r, &b)
		if b.Use != "static" && b.Use != "animated" {
			writeErr(w, 400, errors.New("use must be static or animated"))
			return
		}
		if b.Use == "animated" && pr.Theme.Wallpaper.Animated == "" {
			writeErr(w, 400, errors.New("No animated wallpaper has been imported."))
			return
		}
		setWallpaper(w, id, map[string]any{"use": b.Use})
	})
	m.HandleFunc("DELETE /api/profiles/{id}/wallpaper", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		pr, ok := a.Profiles.Get(id)
		if !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		cur := pr.Theme.Wallpaper
		switch r.URL.Query().Get("kind") {
		case "animated":
			setWallpaper(w, id, map[string]any{"animated": "", "animatedType": "", "poster": "", "use": "static"}, cur.Animated, cur.Poster)
		case "static":
			setWallpaper(w, id, map[string]any{"file": ""}, cur.File)
		default: // back to the built-in background
			dt := DefaultTheme().Wallpaper
			setWallpaper(w, id, map[string]any{"file": "", "animated": "", "animatedType": "", "poster": "", "use": "static",
				"brightness": dt.Brightness, "blur": dt.Blur, "fit": dt.Fit}, cur.File, cur.Animated, cur.Poster)
		}
	})
	m.HandleFunc("GET /wallpaper/{file}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(a.Paths.Wallpapers, filepath.Base(r.PathValue("file"))))
	})
	// ----- per-game updates and mods (gamecontent.go)
	m.HandleFunc("GET /api/games/{key}/content", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		ups, err := a.Content.Updates(key)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		mods, _ := a.Content.Mods(key)
		g, _ := a.Library.Get(key)
		writeJSON(w, map[string]any{"updates": ups, "mods": mods, "busy": a.Content.Busy(key), "titleId": g.TitleID,
			"modsDir": a.Content.modsDir(g.TitleID), "updatesDir": a.Content.updatesDir(g.TitleID)})
	})
	m.HandleFunc("POST /api/games/{key}/updates/add", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Path string }
		_ = readBody(r, &b)
		p, ok := pick(w, b.Path, "Choose a game update package", "Game update (*.nsp;*.nsz;*.xci;*.xcz;*.nup)|*.nsp;*.nsz;*.xci;*.xcz;*.nup|", false)
		if !ok {
			return
		}
		u, err := a.Content.AddUpdate(r.PathValue("key"), p)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, u)
	})
	m.HandleFunc("POST /api/games/{key}/updates/toggle", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			File string
			On   bool
		}
		_ = readBody(r, &b)
		if err := a.Content.SetUpdateEnabled(r.PathValue("key"), b.File, b.On); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/games/{key}/updates/remove", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			File    string
			Confirm bool
		}
		_ = readBody(r, &b)
		if !b.Confirm {
			writeErr(w, 400, errors.New("Removing an update needs confirmation."))
			return
		}
		if err := a.Content.RemoveUpdate(r.PathValue("key"), b.File); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/games/{key}/mods/add", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		var added []string
		var err error
		if ct := r.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/zip") || strings.HasPrefix(ct, "application/octet-stream") {
			data, rerr := io.ReadAll(io.LimitReader(r.Body, 2<<30))
			if rerr != nil {
				writeErr(w, 400, rerr)
				return
			}
			added, err = a.Content.AddMod(key, "", data, firstNonEmpty(r.URL.Query().Get("name"), "mod.zip"))
		} else {
			var b struct{ Path, Pick string }
			_ = readBody(r, &b)
			var p string
			var ok bool
			if b.Pick == "zip" {
				p, ok = pick(w, b.Path, "Choose a mod (ZIP archive)", "ZIP archive (*.zip)|*.zip|", false)
			} else {
				p, ok = pick(w, b.Path, "Choose the mod's folder", "", true)
			}
			if !ok {
				return
			}
			added, err = a.Content.AddMod(key, p, nil, "")
		}
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"added": added})
	})
	// Manage Mods: installed and built-in mods with information, compatibility and conflicts (mods.go)
	m.HandleFunc("GET /api/games/{key}/mods", func(w http.ResponseWriter, r *http.Request) {
		v, err := a.Content.ModsView(r.PathValue("key"))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, v)
	})
	m.HandleFunc("POST /api/games/{key}/mods/toggle", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name string
			On   bool
		}
		_ = readBody(r, &b)
		if err := a.Content.SetModEnabled(r.PathValue("key"), b.Name, b.On); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/games/{key}/mods/remove", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name    string
			Confirm bool
		}
		_ = readBody(r, &b)
		if !b.Confirm {
			writeErr(w, 400, errors.New("Removing a mod needs confirmation."))
			return
		}
		if err := a.Content.RemoveMod(r.PathValue("key"), b.Name); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	// ----- Save Data (saves.go)
	m.HandleFunc("GET /api/profiles/{id}/saves", func(w http.ResponseWriter, r *http.Request) {
		sp, err := a.Saves.Player(r.PathValue("id"))
		if err != nil {
			writeErr(w, 404, err)
			return
		}
		writeJSON(w, map[string]any{"player": sp, "backups": a.Saves.Backups(sp.ProfileID, ""), "migrationError": a.saveMigrationErr,
			"shared": a.Saves.SharedEngineSaves() != "", "busy": a.Saves.busy(sp.ProfileID)})
	})
	m.HandleFunc("POST /api/profiles/{id}/saves/open", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Folder string }
		_ = readBody(r, &b)
		id := r.PathValue("id")
		if _, ok := a.Profiles.Get(id); !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		dir := a.Profiles.SaveDir(id)
		if b.Folder != "" {
			p, err := a.Saves.folderFor(id, b.Folder)
			if err != nil {
				writeErr(w, 404, err)
				return
			}
			dir = p
		}
		_ = os.MkdirAll(dir, 0o755)
		if err := openInExplorer(dir); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, map[string]string{"path": dir})
	})
	m.HandleFunc("GET /api/profiles/{id}/saves/export", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		pr, ok := a.Profiles.Get(id)
		if !ok {
			writeErr(w, 404, errors.New("Profile not found."))
			return
		}
		folder := r.URL.Query().Get("folder")
		what := "all saves"
		if folder != "" {
			if _, err := a.Saves.folderFor(id, folder); err != nil {
				writeErr(w, 404, err)
				return
			}
			what = folder
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="NEXUS saves - %s - %s - %s.zip"`, safeFolderName(pr.Name), safeFolderName(what), time.Now().Format("2006-01-02")))
		if err := a.Saves.Export(id, folder, w); err != nil {
			a.Log.Printf("save export: %v", err)
		}
	})
	m.HandleFunc("POST /api/profiles/{id}/saves/backup", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Folder string }
		_ = readBody(r, &b)
		bk, err := a.Saves.Backup(r.PathValue("id"), b.Folder, "manual")
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, bk)
	})
	m.HandleFunc("GET /api/profiles/{id}/saves/backups", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Saves.Backups(r.PathValue("id"), r.URL.Query().Get("titleId")))
	})
	m.HandleFunc("POST /api/profiles/{id}/saves/restore", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			File    string
			Confirm bool
		}
		_ = readBody(r, &b)
		if !b.Confirm {
			writeErr(w, 400, errors.New("Restoring replaces the current save: confirm first."))
			return
		}
		if err := a.Saves.Restore(r.PathValue("id"), b.File); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/profiles/{id}/saves/delete", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Folder  string
			Confirm bool
		}
		_ = readBody(r, &b)
		if !b.Confirm {
			writeErr(w, 400, errors.New("Deleting a save needs confirmation."))
			return
		}
		bk, err := a.Saves.Delete(r.PathValue("id"), b.Folder)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, bk)
	})
	m.HandleFunc("POST /api/profiles/{id}/saves/use-copy", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Folder string }
		_ = readBody(r, &b)
		if err := a.Saves.UseOtherCopy(r.PathValue("id"), b.Folder); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	// import, step 1: inspect a folder (picked natively), a ZIP (upload or picked), or the engine's
	// shared saves
	m.HandleFunc("POST /api/saves/inspect", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "application/zip") || strings.HasPrefix(ct, "application/octet-stream") {
			data, err := io.ReadAll(io.LimitReader(r.Body, maxSaveZipBytes+1))
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			if int64(len(data)) > maxSaveZipBytes {
				writeErr(w, 400, errZipTooLarge)
				return
			}
			insp, err := a.Saves.Inspect("", data, firstNonEmpty(r.URL.Query().Get("name"), "saves.zip"))
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			writeJSON(w, insp)
			return
		}
		var b struct {
			Path string
			Pick string // "folder" | "zip" | "shared"
		}
		_ = readBody(r, &b)
		path := b.Path
		switch {
		case b.Pick == "shared":
			path = a.Saves.SharedEngineSaves()
			if path == "" {
				writeErr(w, 404, errors.New("There are no saves in the NEXUS engine's shared storage."))
				return
			}
		case path == "" && b.Pick == "zip":
			p, ok := pick(w, "", "Import Save from ZIP: choose the ZIP archive", "ZIP archive (*.zip)|*.zip|", false)
			if !ok {
				return
			}
			path = p
		case path == "":
			p, ok := pick(w, "", "Choose the save folder (a game's folder, or a folder with several)", "", true)
			if !ok {
				return
			}
			path = p
		}
		insp, err := a.Saves.Inspect(path, nil, "")
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, insp)
	})
	// import, step 2: the saves the user confirmed, into a user
	m.HandleFunc("POST /api/saves/import", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Token     string
			ProfileID string
			Picks     []SaveImportPick
		}
		_ = readBody(r, &b)
		conflicts, err := a.Saves.Import(b.Token, b.ProfileID, b.Picks)
		if errors.Is(err, errSaveConflict) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "conflicts": conflicts})
			return
		}
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	// ----- amiibo (amiibo.go) -----
	amiiboState := func() map[string]any {
		st := a.Emu.State()
		return map[string]any{"list": a.Amiibo.List(), "keys": a.Amiibo.KeysInstalled(), "scanning": st.Running && st.Amiibo == "scanning", "running": st.Running}
	}
	m.HandleFunc("GET /api/amiibo", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, amiiboState()) })
	m.HandleFunc("POST /api/amiibo/import", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Path, Owner string }
		_ = readBody(r, &b)
		if b.Path == "" {
			p, err := a.UI.PickFile("Choose an amiibo dump", "amiibo dump (*.bin;*.nfc)|*.bin;*.nfc|All files|*.*|")
			if err != nil || p == "" {
				writeJSON(w, map[string]any{"cancelled": true})
				return
			}
			b.Path = p
		}
		am, err := a.Amiibo.Import(b.Path, b.Owner)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, am)
	})
	m.HandleFunc("POST /api/amiibo/virtual", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			AmiiboID string
			AmiiboDetails
		}
		_ = readBody(r, &b)
		am, err := a.Amiibo.CreateVirtualWith(b.AmiiboID, b.AmiiboDetails)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, am)
	})
	m.HandleFunc("POST /api/amiibo/update", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID    string
			Name  *string
			Owner *string
		}
		_ = readBody(r, &b)
		am, err := a.Amiibo.Update(b.ID, func(x *Amiibo) {
			if b.Name != nil {
				x.Name = *b.Name
			}
			if b.Owner != nil {
				x.Owner = *b.Owner
			}
		})
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, am)
	})
	m.HandleFunc("POST /api/amiibo/edit", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID string
			AmiiboDetails
		}
		_ = readBody(r, &b)
		am, err := a.Amiibo.Edit(b.ID, b.AmiiboDetails)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, am)
	})
	m.HandleFunc("POST /api/amiibo/duplicate", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ID string }
		_ = readBody(r, &b)
		am, err := a.Amiibo.Duplicate(b.ID)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, am)
	})
	m.HandleFunc("POST /api/amiibo/art/{id}", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
		var am Amiibo
		if err == nil {
			if len(data) == 0 {
				am, err = a.Amiibo.ClearArt(r.PathValue("id"))
			} else {
				am, err = a.Amiibo.SetArt(r.PathValue("id"), data)
			}
		}
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, am)
	})
	m.HandleFunc("GET /api/amiibo/art/{id}", func(w http.ResponseWriter, r *http.Request) {
		p := a.Amiibo.ArtPath(r.PathValue("id"))
		if p == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, p)
	})
	m.HandleFunc("POST /api/amiibo/remove", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ID string }
		_ = readBody(r, &b)
		if err := a.Amiibo.Remove(b.ID); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/amiibo/place", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ID string }
		_ = readBody(r, &b)
		if err := a.Amiibo.Place(b.ID); err != nil {
			writeErr(w, 409, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("POST /api/amiibo/keys", func(w http.ResponseWriter, r *http.Request) {
		p, err := a.UI.PickFile("Choose your amiibo keys file (key_retail.bin)", "key_retail.bin|key_retail.bin|All files|*.*|")
		if err != nil || p == "" {
			writeJSON(w, map[string]any{"cancelled": true})
			return
		}
		if err := a.Amiibo.InstallKeys(p); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, true)
	})
	m.HandleFunc("GET /api/amiibo/lookup", func(w http.ResponseWriter, r *http.Request) {
		list, err := LookupAmiibo(r.URL.Query().Get("q"), r.URL.Query().Get("by"))
		if err != nil {
			writeErr(w, 502, err)
			return
		}
		writeJSON(w, map[string]any{"results": list})
	})
	// ----- NEXUS Account (account.go)
	acct := func(w http.ResponseWriter, err error) {
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, a.Account.Status())
	}
	m.HandleFunc("GET /api/account", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.Account.Status()) })
	m.HandleFunc("POST /api/account/server", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Server string }
		_ = readBody(r, &b)
		acct(w, a.Account.SetServer(b.Server))
	})
	m.HandleFunc("POST /api/account/register", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Email, Password, DisplayName, DeviceName string }
		_ = readBody(r, &b)
		code, err := a.Account.Register(b.Email, b.Password, b.DisplayName, b.DeviceName)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"recoveryCode": code, "status": a.Account.Status()})
	})
	m.HandleFunc("POST /api/account/signin", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Email, Password, DeviceName string }
		_ = readBody(r, &b)
		acct(w, a.Account.SignIn(b.Email, b.Password, b.DeviceName))
	})
	m.HandleFunc("POST /api/account/recover", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Email, Code, NewPassword, DeviceName string }
		_ = readBody(r, &b)
		code, err := a.Account.Recover(b.Email, b.Code, b.NewPassword, b.DeviceName)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"recoveryCode": code, "status": a.Account.Status()})
	})
	m.HandleFunc("POST /api/account/signout", func(w http.ResponseWriter, r *http.Request) { acct(w, a.Account.SignOut()) })
	m.HandleFunc("POST /api/account/refresh", func(w http.ResponseWriter, r *http.Request) { acct(w, a.Account.Refresh()) })
	m.HandleFunc("POST /api/account/hold", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Send bool }
		_ = readBody(r, &b)
		acct(w, a.Account.ReleaseHold(b.Send))
	})
	m.HandleFunc("POST /api/account/update", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = readBody(r, &b)
		patch := map[string]any{}
		for _, k := range []string{"displayName", "password", "newPassword", "deviceName"} {
			if v, ok := b[k]; ok {
				patch[k] = v
			}
		}
		acct(w, a.Account.UpdateAccount(patch))
	})
	m.HandleFunc("POST /api/account/devices/remove", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ID string }
		_ = readBody(r, &b)
		acct(w, a.Account.RemoveDevice(b.ID))
	})
	m.HandleFunc("POST /api/account/link/code", func(w http.ResponseWriter, r *http.Request) {
		code, exp, err := a.Account.LinkCode()
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"code": code, "expires": exp})
	})
	m.HandleFunc("GET /api/account/link/requests", func(w http.ResponseWriter, r *http.Request) {
		reqs, err := a.Account.LinkRequests()
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"requests": reqs})
	})
	m.HandleFunc("POST /api/account/link/decide", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID      string
			Approve bool
		}
		_ = readBody(r, &b)
		acct(w, a.Account.DecideLink(b.ID, b.Approve))
	})
	m.HandleFunc("POST /api/account/link/claim", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Code, DeviceName string }
		_ = readBody(r, &b)
		_, err := a.Account.ClaimCode(b.Code, b.DeviceName)
		acct(w, err)
	})
	m.HandleFunc("POST /api/account/link/status", func(w http.ResponseWriter, r *http.Request) {
		st, err := a.Account.LinkStatus()
		if err != nil && st != "pending" {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"status": st, "account": a.Account.Status()})
	})
	m.HandleFunc("POST /api/account/link/cancel", func(w http.ResponseWriter, r *http.Request) { a.Account.CancelLink(); acct(w, nil) })
	m.HandleFunc("POST /api/account/sync", func(w http.ResponseWriter, r *http.Request) {
		_, err := a.Account.Sync()
		acct(w, err)
	})
	m.HandleFunc("POST /api/account/sync/settings", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			On         *bool
			Categories map[string]bool
		}
		_ = readBody(r, &b)
		a.Account.SetSync(b.On, b.Categories)
		acct(w, nil)
	})
	m.HandleFunc("POST /api/account/conflict", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ ID, Keep string }
		_ = readBody(r, &b)
		acct(w, a.Account.ResolveConflict(b.ID, b.Keep))
	})
	m.HandleFunc("GET /api/account/conflict", func(w http.ResponseWriter, r *http.Request) {
		d, err := a.Account.ConflictDetails(r.URL.Query().Get("id"))
		if err != nil {
			writeErr(w, 404, err)
			return
		}
		writeJSON(w, d)
	})
	m.HandleFunc("POST /api/account/github/start", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ DeviceName string }
		_ = readBody(r, &b)
		v, err := a.Account.StartGitHubSignIn(b.DeviceName)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, v)
	})
	m.HandleFunc("POST /api/account/github/status", func(w http.ResponseWriter, r *http.Request) {
		st, err := a.Account.GitHubSignInStatus()
		if err != nil && st != "pending" {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"status": st, "account": a.Account.Status()})
	})
	m.HandleFunc("POST /api/account/github/cancel", func(w http.ResponseWriter, r *http.Request) { a.Account.CancelGitHubSignIn(); acct(w, nil) })
	m.HandleFunc("POST /api/account/test", func(w http.ResponseWriter, r *http.Request) {
		v, err := a.Account.TestConnection()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "status": a.Account.Status()})
			return
		}
		v["ok"] = true
		v["status"] = a.Account.Status()
		writeJSON(w, v)
	})
	m.HandleFunc("POST /api/account/onboard-seen", func(w http.ResponseWriter, r *http.Request) { a.Account.SetOnboardSeen(); acct(w, nil) })
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	ch := s.app.Events.Subscribe()
	defer s.app.Events.Unsubscribe(ch)
	_, _ = fmt.Fprint(w, ": hello\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			fl.Flush()
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) state() map[string]any {
	a := s.app
	formats := []map[string]string{}
	for _, f := range []string{"nsp", "xci", "nro", "nsz", "xcz", "nup", "nca", "nso"} {
		formats = append(formats, map[string]string{"ext": f, "label": SupportedFormats[f]})
	}
	return map[string]any{
		"app":          map[string]any{"name": AppName, "model": ConsoleModel, "version": AppVersion, "os": runtime.GOOS, "native": a.UI.Native(), "root": a.Paths.Root, "data": a.Paths.Data, "portable": a.Paths.IsPortable, "migrated": a.migrated},
		"settings":     a.Settings.Get(),
		"profiles":     a.Profiles.List(),
		"emulator":     a.Emu.Status(),
		"engine":       s.engineInfo(),
		"formats":      formats,
		"library":      a.Library.Status(),
		"scanDirs":     a.Library.ScanDirs(),
		"gamesDir":     a.Paths.Games,
		"avatarColors": avatarColors,
		"avatarGlyphs": avatarGlyphs,
		"padModels":    PadModels(),
		"maxProfiles":  MaxProfiles,
		"features":     map[string]bool{"deviceLink": a.Link != nil},
	}
}

func (s *Server) storage() map[string]any {
	a := s.app
	sz := func(p string) map[string]any {
		b, n := dirSize(p)
		return map[string]any{"path": p, "bytes": b, "files": n}
	}
	free, total := diskSpace(a.Paths.Root)
	return map[string]any{
		"disk":       map[string]any{"free": free, "total": total},
		"games":      sz(a.Paths.Games),
		"saves":      sz(a.Paths.Saves),
		"artwork":    sz(a.Paths.Artwork),
		"engine":     sz(a.Emu.Builds.Root()),
		"engineData": sz(a.Emu.DataDir()),
		"logs":       sz(a.Paths.Logs),
		"archived":   sz(a.Paths.Trash),
		"wallpaper":  sz(a.Paths.Wallpapers),
	}
}

func networkUp() bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, ad := range addrs {
			if ipn, ok := ad.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
				return true
			}
		}
	}
	return false
}

func openInExplorer(dir string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", dir).Start()
	}
	return exec.Command("xdg-open", dir).Start()
}

func revealInExplorer(file string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", "/select,", file).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(file)).Start()
}

// openWithDefaultApp opens a file in the program Windows uses for it (Photos, a video player).
func openWithDefaultApp(file string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", file).Start() // opens it in its default program
	}
	return exec.Command("xdg-open", file).Start()
}

// engineInfo describes the NEXUS engine for the Settings page: installed builds, the active
// build's own description (systems, renderers, components), state and data folder.
func (s *Server) engineInfo() map[string]any {
	a := s.app
	out := a.Emu.Status()
	builds := a.Emu.Builds.List()
	if builds == nil {
		builds = []EngineBuild{}
	}
	out["builds"] = builds
	out["active"] = a.Emu.Builds.activeVersion()
	out["activeLabel"] = a.Emu.Builds.ActiveVersionLabel()
	out["bundled"] = a.Emu.Builds.BundledPresent()
	out["engineDir"] = a.Emu.Builds.ActiveDir()
	out["job"] = a.Emu.Builds.Job()
	out["buildsDir"] = a.Emu.Builds.BuildsDir()
	out["apiVersion"] = engineAPIVersion
	if ok, _ := out["installed"].(bool); ok {
		if h, err := a.Emu.Host(); err == nil {
			if info, err := h.Info(); err == nil {
				out["info"] = info
			} else {
				out["infoError"] = err.Error()
			}
		}
	}
	return out
}
