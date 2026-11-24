package main

// GameBanana mods (0.9.3): search a game's mods on gamebanana.com, show a mod's files, and
// install one through the same installer as Import a mod (gamecontent.go AddMod): the archive is
// checked whole before anything is written (no paths outside the mod, no links, size limit, a
// mod for another game refused), the game's own files are never changed, nothing is overwritten,
// and nothing downloaded is ever run.
//
// API: GameBanana's public API v11, no account or key (Util/Game/NameMatch, Game/<id>/Subfeed,
// Mod/<id>/ProfilePage). Idea and reference: SimpleModDownloader by PoloNX (GPL-3.0,
// github.com/PoloNX/SimpleModDownloader), a Switch homebrew that downloads GameBanana mods; this
// is NEXUS's own code, none of it is copied.
//
// Downloads are untrusted: only https from gamebanana.com hosts (redirects included), a size
// limit, the MD5 GameBanana lists checked, ZIP archives only (7z / RAR: the user extracts them and
// uses Import), and the downloaded data is kept in memory, never written as it came.

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var gbAPI = "https://gamebanana.com/apiv11"

// gbTestHost: a stand-in GameBanana in tests (host:port); empty in NEXUS
var gbTestHost = ""

const gbMaxDownload = 2 << 30 // 2 GB

type GBMod struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Author   string `json:"author,omitempty"`
	Category string `json:"category,omitempty"`
	Thumb    string `json:"thumb,omitempty"`
	Likes    int64  `json:"likes,omitempty"`
	Views    int64  `json:"views,omitempty"`
	Updated  int64  `json:"updated,omitempty"`
	URL      string `json:"url,omitempty"`
}

type GBFile struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	MD5         string `json:"md5,omitempty"`
	Description string `json:"description,omitempty"`
	Added       int64  `json:"added,omitempty"`
	Zip         bool   `json:"zip"`                // NEXUS can install it (a .zip)
	Analysis    string `json:"analysis,omitempty"` // GameBanana's own file scan result, as given
	download    string
}

type GBModPage struct {
	GBMod
	Description string   `json:"description,omitempty"`
	Files       []GBFile `json:"files"`
	Game        string   `json:"game,omitempty"`
}

type GBSearch struct {
	GameID   int64   `json:"gameId"`
	GameName string  `json:"gameName"`
	Mods     []GBMod `json:"mods"`
	Complete bool    `json:"complete"`
}

type gameBanana struct {
	app    *App
	client *http.Client
	mu     sync.Mutex
	games  map[string]gbGame // title -> GameBanana game
}

type gbGame struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func newGameBanana(a *App) *gameBanana {
	gb := &gameBanana{app: a, games: map[string]gbGame{}}
	gb.client = &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		if !gbAllowedURL(r.URL) {
			return fmt.Errorf("the download went to %s, which is not GameBanana; stopped", r.URL.Host)
		}
		return nil
	}}
	return gb
}

// gbAllowedURL: https to gamebanana.com or one of its subdomains (files.gamebanana.com, …)
func gbAllowedURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if gbTestHost != "" && u.Host == gbTestHost {
		return true
	}
	return h == "gamebanana.com" || strings.HasSuffix(h, ".gamebanana.com")
}

func (gb *gameBanana) getJSON(u string, v any) error {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	req.Header.Set("Accept", "application/json")
	resp, err := gb.client.Do(req)
	if err != nil {
		return fmt.Errorf("GameBanana could not be reached (%v).", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GameBanana answered %s.", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(v)
}

var reNonWord = regexp.MustCompile(`[^a-z0-9]+`)

func gbNorm(s string) string {
	return strings.Trim(reNonWord.ReplaceAllString(strings.ToLower(s), " "), " ")
}

// GameFor finds the GameBanana game for a library title (remembered for the session).
func (gb *gameBanana) GameFor(title string) (gbGame, error) {
	gb.mu.Lock()
	if g, ok := gb.games[title]; ok {
		gb.mu.Unlock()
		return g, nil
	}
	gb.mu.Unlock()
	var r struct {
		Records []struct {
			ID   int64  `json:"_idRow"`
			Name string `json:"_sName"`
		} `json:"_aRecords"`
	}
	q := strings.TrimSpace(regexp.MustCompile(`\s*[\[(].*$`).ReplaceAllString(title, ""))
	if err := gb.getJSON(gbAPI+"/Util/Game/NameMatch?_sName="+url.QueryEscape(q), &r); err != nil {
		return gbGame{}, err
	}
	if len(r.Records) == 0 {
		return gbGame{}, fmt.Errorf("GameBanana has no game called %q.", q)
	}
	want := gbNorm(q)
	best := r.Records[0]
	for _, x := range r.Records {
		if gbNorm(x.Name) == want {
			best = x
			break
		}
	}
	g := gbGame{best.ID, best.Name}
	gb.mu.Lock()
	gb.games[title] = g
	gb.mu.Unlock()
	return g, nil
}

type gbRecord struct {
	ID        int64  `json:"_idRow"`
	Name      string `json:"_sName"`
	Model     string `json:"_sModelName"`
	Profile   string `json:"_sProfileUrl"`
	Updated   int64  `json:"_tsDateUpdated"`
	Modified  int64  `json:"_tsDateModified"`
	Likes     int64  `json:"_nLikeCount"`
	Views     int64  `json:"_nViewCount"`
	Submitter struct {
		Name string `json:"_sName"`
	} `json:"_aSubmitter"`
	RootCategory struct {
		Name string `json:"_sName"`
	} `json:"_aRootCategory"`
	Preview struct {
		Images []struct {
			Base string `json:"_sBaseUrl"`
			F220 string `json:"_sFile220"`
			F    string `json:"_sFile"`
		} `json:"_aImages"`
	} `json:"_aPreviewMedia"`
}

func (r gbRecord) mod() GBMod {
	m := GBMod{ID: r.ID, Name: r.Name, Author: r.Submitter.Name, Category: r.RootCategory.Name, Likes: r.Likes, Views: r.Views, URL: r.Profile}
	m.Updated = r.Updated
	if m.Updated == 0 {
		m.Updated = r.Modified
	}
	if len(r.Preview.Images) > 0 {
		im := r.Preview.Images[0]
		f := im.F220
		if f == "" {
			f = im.F
		}
		if u, err := url.Parse(im.Base + "/" + f); err == nil && gbAllowedURL(u) {
			m.Thumb = u.String()
		}
	}
	return m
}

// Search lists a game's mods on GameBanana (query optional), 15 a page.
func (gb *gameBanana) Search(title, query string, page int) (GBSearch, error) {
	g, err := gb.GameFor(title)
	if err != nil {
		return GBSearch{}, err
	}
	if page < 1 {
		page = 1
	}
	u := fmt.Sprintf("%s/Game/%d/Subfeed?_nPage=%d&_sSort=default&_csvModelInclusions=Mod", gbAPI, g.ID, page)
	if q := strings.TrimSpace(query); len(q) >= 2 {
		u += "&_sName=" + url.QueryEscape(q)
	}
	var r struct {
		Meta struct {
			Complete bool `json:"_bIsComplete"`
		} `json:"_aMetadata"`
		Records []gbRecord `json:"_aRecords"`
	}
	if err := gb.getJSON(u, &r); err != nil {
		return GBSearch{}, err
	}
	out := GBSearch{GameID: g.ID, GameName: g.Name, Mods: []GBMod{}, Complete: r.Meta.Complete}
	for _, x := range r.Records {
		if x.Model == "" || x.Model == "Mod" {
			out.Mods = append(out.Mods, x.mod())
		}
	}
	return out, nil
}

// Mod returns a mod's page with its files.
func (gb *gameBanana) Mod(id int64) (GBModPage, error) {
	var r struct {
		gbRecord
		Text string `json:"_sText"`
		Game struct {
			Name string `json:"_sName"`
		} `json:"_aGame"`
		Files []struct {
			ID       int64  `json:"_idRow"`
			File     string `json:"_sFile"`
			Size     int64  `json:"_nFilesize"`
			MD5      string `json:"_sMd5Checksum"`
			URL      string `json:"_sDownloadUrl"`
			Desc     string `json:"_sDescription"`
			Added    int64  `json:"_tsDateAdded"`
			Analysis string `json:"_sAnalysisResult"`
		} `json:"_aFiles"`
	}
	if err := gb.getJSON(fmt.Sprintf("%s/Mod/%d/ProfilePage", gbAPI, id), &r); err != nil {
		return GBModPage{}, err
	}
	p := GBModPage{GBMod: r.gbRecord.mod(), Game: r.Game.Name, Files: []GBFile{}, Description: gbPlainText(r.Text, 4000)}
	if p.ID == 0 {
		p.ID = id
	}
	for _, f := range r.Files {
		p.Files = append(p.Files, GBFile{ID: f.ID, Name: f.File, Size: f.Size, MD5: strings.ToLower(f.MD5), Description: f.Desc, Added: f.Added,
			Zip: strings.EqualFold(filepath.Ext(f.File), ".zip"), Analysis: f.Analysis, download: f.URL})
	}
	return p, nil
}

var reTags = regexp.MustCompile(`<[^>]*>`)

// gbPlainText: a mod's description without HTML (shown as text, never as markup)
func gbPlainText(s string, max int) string {
	s = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n").Replace(s)
	s = reTags.ReplaceAllString(s, "")
	s = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&nbsp;", " ").Replace(s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// Install downloads one file of a mod and installs it for the game (key).
func (gb *gameBanana) Install(key string, modID, fileID int64) ([]string, error) {
	page, err := gb.Mod(modID)
	if err != nil {
		return nil, err
	}
	var f *GBFile
	for i := range page.Files {
		if page.Files[i].ID == fileID {
			f = &page.Files[i]
		}
	}
	if f == nil {
		return nil, errors.New("That file is no longer on the mod's GameBanana page.")
	}
	if !f.Zip {
		return nil, fmt.Errorf("%s is not a ZIP archive. NEXUS installs ZIP archives only: download it from GameBanana, extract it, and use Import a mod folder.", f.Name)
	}
	if f.Size > gbMaxDownload {
		return nil, errors.New("This file is larger than NEXUS downloads (2 GB).")
	}
	u, err := url.Parse(f.download)
	if err != nil || !gbAllowedURL(u) {
		return nil, errors.New("The file's download address is not on GameBanana; it was not downloaded.")
	}
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	dl := *gb.client
	dl.Timeout = 30 * time.Minute
	resp, err := dl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("The download failed (%v).", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("The download failed: GameBanana answered %s.", resp.Status)
	}
	limit := int64(gbMaxDownload)
	if f.Size > 0 {
		limit = f.Size + 1<<20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("The download failed (%v).", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("The download is larger than GameBanana says the file is; it was not installed.")
	}
	if f.MD5 != "" {
		sum := md5.Sum(data)
		if hex.EncodeToString(sum[:]) != f.MD5 {
			return nil, errors.New("The downloaded file does not match GameBanana's checksum; it was not installed.")
		}
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) && !bytes.HasPrefix(data, []byte("PK\x05\x06")) {
		return nil, errors.New("The downloaded file is not a ZIP archive; it was not installed.")
	}
	name := modName(page.Name)
	if name == "" {
		name = strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
	}
	added, err := gb.app.Content.AddMod(key, "", data, name+".zip")
	if err != nil {
		return added, err
	}
	g, _ := gb.app.Content.game(key)
	for _, n := range added {
		src := map[string]any{"source": "gamebanana", "modId": modID, "fileId": fileID, "file": f.Name, "name": page.Name, "author": page.Author, "url": page.URL, "installed": time.Now().Unix()}
		b, _ := json.MarshalIndent(src, "", "  ")
		_ = os.WriteFile(filepath.Join(gb.app.Content.modsDir(g.TitleID), n, ".nexus-source.json"), b, 0o644)
	}
	gb.app.Log.Printf("mods: GameBanana mod %d file %d installed for %s as %s", modID, fileID, g.TitleID, strings.Join(added, ", "))
	return added, nil
}

func atoi64(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
