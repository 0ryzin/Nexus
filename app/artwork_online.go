package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// OnlineArtwork downloads square game icons for games that have a Title ID but
// no artwork of their own. Primary source: Nlib API (https://api.nlib.cc/nx/,
// github.com/ghost-land/Nlib-API, data from blawar/titledb). Extra providers can
// be configured as URL templates containing {tid}. Results are validated
// (Title ID echo, name match, real square image) and cached per game.
type OnlineArtwork struct {
	app       *App
	lib       *Library
	mu        sync.Mutex
	queue     []string
	queued    map[string]bool
	forced    map[string]bool
	forcedRun map[string]bool
	busy      bool
	done      int
	client    *http.Client
	last      string
}

func NewOnlineArtwork(a *App, l *Library) *OnlineArtwork {
	return &OnlineArtwork{app: a, lib: l, queued: map[string]bool{}, forced: map[string]bool{}, forcedRun: map[string]bool{},
		client: &http.Client{Timeout: 20 * time.Second}}
}

func (o *OnlineArtwork) Status() map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()
	return map[string]any{"busy": o.busy, "pending": len(o.queue), "done": o.done, "last": o.last}
}

const onlineRetryAfter = 7 * 24 * time.Hour

// Queue schedules lookups. Automatic lookups only run for games without any
// local artwork; forced lookups (Refresh) always run.
func (o *OnlineArtwork) Queue(games []Game, force bool) {
	s := o.app.Settings.Get()
	if !s.Library.OnlineArtwork && !force {
		return
	}
	o.mu.Lock()
	for _, g := range games {
		if g.TitleID == "" || g.Kind == "update" || g.Kind == "dlc" || g.Kind == "homebrew" || g.Hidden {
			continue
		}
		if !force {
			if g.HasCustom || g.HasEmbed || g.HasOnline || g.ArtSource == "sidecar" || g.ArtSource == "artwork-folder" || g.ArtSource == "builtin" {
				continue
			}
			p := o.lib.GP.Get(g.Key)
			if p.Online.CheckedAt != 0 && time.Since(time.Unix(p.Online.CheckedAt, 0)) < onlineRetryAfter {
				continue
			}
		}
		if force {
			o.forced[g.Key] = true
		}
		if !o.queued[g.Key] {
			o.queued[g.Key] = true
			o.queue = append(o.queue, g.Key)
		}
	}
	start := !o.busy && len(o.queue) > 0
	if start {
		o.busy = true
	}
	o.mu.Unlock()
	if start {
		go o.run()
	}
}

func (o *OnlineArtwork) run() {
	changed := 0
	for {
		o.mu.Lock()
		if len(o.queue) == 0 {
			o.busy = false
			o.mu.Unlock()
			break
		}
		key := o.queue[0]
		o.queue = o.queue[1:]
		delete(o.queued, key)
		if o.forced[key] {
			o.forcedRun[key] = true
		}
		delete(o.forced, key)
		o.mu.Unlock()

		g, ok := o.lib.Get(key)
		if !ok {
			continue
		}
		o.mu.Lock()
		forced := o.forcedRun[key]
		delete(o.forcedRun, key)
		o.mu.Unlock()
		var res *onlineResult
		var err error
		if !forced {
			res, err = o.fromDatabase(g)
		}
		if forced || res == nil {
			res, err = o.lookup(g)
		}
		_, _ = o.lib.GP.Update(key, func(p *GameProfile) {
			p.Online.CheckedAt = time.Now().Unix()
			if err != nil {
				p.Online.Error = err.Error()
				return
			}
			if res.Name != "" {
				p.Online.Name = res.Name
			}
			if res.Publisher != "" {
				p.Online.Publisher = res.Publisher
			}
			if res.Released != "" {
				p.Online.Released = res.Released
			}
			p.Online.Error = ""
			if res.File != "" {
				p.Online.File = res.File
				p.Online.Source = res.Source
			}
		})
		msg := fmt.Sprintf("%s: %s", g.Title, "artwork downloaded")
		if err != nil {
			msg = fmt.Sprintf("%s: %v", g.Title, err)
			o.app.Log.Printf("online artwork %s (%s): %v", g.TitleID, g.Title, err)
		} else {
			o.app.Log.Printf("online artwork %s (%s): saved from %s", g.TitleID, g.Title, res.Source)
			changed++
		}
		o.lib.redecorate(key)
		o.mu.Lock()
		o.done++
		o.last = msg
		o.mu.Unlock()
		o.app.Events.Emit("artwork.online", map[string]any{"key": key, "ok": err == nil, "message": msg})
		time.Sleep(250 * time.Millisecond) // be polite to the API
	}
	if changed > 0 {
		o.app.Events.Emit("library.changed", nil)
	}
}

type onlineResult struct {
	Name, File, Source  string
	Publisher, Released string
}

func (o *OnlineArtwork) get(url string, max int64) ([]byte, string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion+" (game library frontend)")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, "", errNotFound
	}
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > max {
		return nil, "", errors.New("response too large")
	}
	return data, resp.Header.Get("Content-Type"), nil
}

var errNotFound = errors.New("not found")

// validIcon checks that data is a real, roughly square JPEG/PNG of useful size.
func validIcon(data []byte) (string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("not a valid image: %v", err)
	}
	if cfg.Width < 128 || cfg.Height < 128 {
		return "", fmt.Errorf("image too small (%dx%d)", cfg.Width, cfg.Height)
	}
	r := float64(cfg.Width) / float64(cfg.Height)
	if r < 0.85 || r > 1.18 {
		return "", fmt.Errorf("image is not a square icon (%dx%d)", cfg.Width, cfg.Height)
	}
	switch format {
	case "jpeg":
		return ".jpg", nil
	case "png":
		return ".png", nil
	}
	return "", errors.New("unsupported image format " + format)
}

var reNonAlnum = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func nameTokens(s string) []string {
	s = strings.ToLower(strings.NewReplacer("™", "", "®", "", "©", "", "&", " and ").Replace(s))
	return strings.Fields(reNonAlnum.ReplaceAllString(s, " "))
}

// namesMatch is deliberately lenient (editions, punctuation, subtitles differ
// between regions) but rejects clearly different games.
func namesMatch(a, b string) bool {
	ta, tb := nameTokens(a), nameTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return true
	}
	ja, jb := strings.Join(ta, " "), strings.Join(tb, " ")
	if strings.Contains(ja, jb) || strings.Contains(jb, ja) {
		return true
	}
	set := map[string]bool{}
	for _, t := range ta {
		set[t] = true
	}
	common := 0
	for _, t := range tb {
		if set[t] {
			common++
		}
	}
	return float64(common)/float64(min(len(ta), len(tb))) >= 0.5
}

func (o *OnlineArtwork) lookup(g Game) (*onlineResult, error) {
	s := o.app.Settings.Get()
	tid := strings.ToUpper(g.TitleID)
	// The Title ID read from the game's own header/ticket is exact, so the
	// metadata's echoed ID is the main check. Names are compared when we have the
	// game's real name, or when the ID itself only came from the file name.
	localName := g.EmbedName
	if localName == "" {
		localName = o.lib.gameListCachedName(g.TitleID)
	}
	if localName == "" && g.IDSource == "filename" {
		localName = g.FileTitle
	}
	var errs []string
	// 1) Nlib API
	if base := strings.TrimRight(s.Library.NlibURL, "/"); base != "" {
		res, err := o.nlib(base, tid, localName, g.Key)
		if err == nil {
			return res, nil
		}
		errs = append(errs, "Nlib: "+err.Error())
	}
	// 2) additional providers (URL templates)
	for _, tpl := range s.Library.ArtProviders {
		tpl = strings.TrimSpace(tpl)
		if tpl == "" || !strings.Contains(tpl, "{tid}") {
			continue
		}
		u := strings.NewReplacer("{tid}", tid, "{tidlower}", strings.ToLower(tid)).Replace(tpl)
		data, _, err := o.get(u, 8<<20)
		if err == nil {
			var ext string
			if ext, err = validIcon(data); err == nil {
				name := "online-icon" + ext
				if err = o.save(g.Key, name, data); err == nil {
					host := hostOf(u)
					o.lib.artCache().Put(ArtDBRecord{TitleID: tid, Source: host}, data, ext)
					return &onlineResult{File: name, Source: host}, nil
				}
			}
		}
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return nil, errors.New("no online provider configured")
	}
	return nil, errors.New(strings.Join(errs, "; "))
}

func (o *OnlineArtwork) nlib(base, tid, localName, key string) (*onlineResult, error) {
	data, _, err := o.get(fmt.Sprintf("%s/nx/%s?fields=name,icon,publisher,releaseDate", base, tid), 1<<20)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, errors.New("title not in database")
		}
		return nil, err
	}
	var meta struct {
		ID          string          `json:"id"`
		Name        string          `json:"name"`
		Icon        string          `json:"icon"`
		Publisher   string          `json:"publisher"`
		ReleaseDate json.RawMessage `json:"releaseDate"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("bad metadata response: %v", err)
	}
	if !strings.EqualFold(meta.ID, tid) {
		return nil, fmt.Errorf("metadata is for %s, not %s", meta.ID, tid)
	}
	if localName != "" && meta.Name != "" && !namesMatch(localName, meta.Name) {
		return nil, fmt.Errorf("name mismatch (%q vs %q)", meta.Name, localName)
	}
	res := &onlineResult{Name: meta.Name, Source: hostOf(base), Publisher: meta.Publisher}
	if r := strings.Trim(string(meta.ReleaseDate), `" `); r != "" && r != "null" {
		res.Released = r
	}
	if meta.Icon == "" {
		return res, errors.New("no icon available for this title")
	}
	img, _, err := o.get(fmt.Sprintf("%s/nx/%s/icon/512", base, tid), 8<<20)
	if err != nil {
		return nil, fmt.Errorf("icon download failed: %v", err)
	}
	ext, err := validIcon(img)
	if err != nil {
		return nil, err
	}
	res.File = "online-icon" + ext
	if err := o.save(key, res.File, img); err != nil {
		return nil, err
	}
	o.lib.artCache().Put(ArtDBRecord{TitleID: tid, Name: meta.Name, Publisher: res.Publisher, Released: res.Released, Source: res.Source}, img, ext)
	return res, nil
}

// fromDatabase serves a Title ID already in the local artwork database
// (no network). Returns nil, nil when it isn't there.
func (o *OnlineArtwork) fromDatabase(g Game) (*onlineResult, error) {
	r, data, ok := o.lib.artCache().Get(g.TitleID)
	if !ok {
		return nil, nil
	}
	ext := filepath.Ext(r.File)
	if err := o.save(g.Key, "online-icon"+ext, data); err != nil {
		return nil, err
	}
	return &onlineResult{Name: r.Name, File: "online-icon" + ext, Source: r.Source + " (local database)", Publisher: r.Publisher, Released: r.Released}, nil
}

func (o *OnlineArtwork) save(key, name string, data []byte) error {
	if err := os.MkdirAll(o.lib.GP.Dir(key), 0o755); err != nil {
		return err
	}
	for _, e := range []string{".jpg", ".png"} {
		_ = os.Remove(o.lib.GP.File(key, "online-icon"+e))
	}
	tmp := o.lib.GP.File(key, name+".part")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, o.lib.GP.File(key, name))
}

func hostOf(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	return strings.SplitN(u, "/", 2)[0]
}
