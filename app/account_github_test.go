package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGitHub implements the parts of GitHub NEXUS uses: the device flow, /user and Gists.
type fakeGitHub struct {
	mu       sync.Mutex
	gists    map[string]map[string]string // id -> file -> content
	desc     map[string]string
	approved map[string]bool // device code -> approved
	n        int
	// account_session_test.go: which GitHub user a sign-in belongs to, each user's gists, and
	// failures to simulate
	nextUser int             // user id the next approved sign-in gets (0: 42)
	tokUser  map[string]int  // token -> user id
	owner    map[string]int  // gist id -> user id
	flaky401 int             // the next n gist requests answer 401 (GitHub having a moment)
	revoked  map[string]bool // tokens GitHub no longer accepts
	// account_persist_test.go: GitHub's eight-hour sign-ins (OAuth apps since August 2026)
	expiring bool           // issue access tokens that expire, with refresh tokens
	refresh  map[string]int // live refresh token -> user id
	issued   int            // tokens issued so far (device flow + renewals)
	renewals int            // successful renewals
	down     bool           // the network is down: every connection is dropped
	scopes   []string       // scope asked for at each device-flow start
	// savesync_test.go: raw file URLs, one per stored revision (as GitHub's raw_url)
	base string
	raw  map[string]string // path -> content
}

// rawURL stores content as a revision of a Gist file and returns its raw address.
func (f *fakeGitHub) rawURL(id, name, content string) string {
	if f.raw == nil {
		f.raw = map[string]string{}
	}
	p := fmt.Sprintf("/raw/%s/%d/%s", id, len(f.raw)+1, name)
	f.raw[p] = content
	return f.base + p
}

func (f *fakeGitHub) userOf(auth string) int {
	if u := f.tokUser[strings.TrimPrefix(auth, "Bearer ")]; u != 0 {
		return u
	}
	return 42
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		if hj, ok := w.(http.Hijacker); ok {
			c, _, _ := hj.Hijack()
			c.Close()
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	auth := r.Header.Get("Authorization")
	if c, ok := f.raw[r.URL.Path]; ok && strings.HasPrefix(r.URL.Path, "/raw/") { // public by its address
		fmt.Fprint(w, c)
		return
	}
	switch {
	case r.URL.Path == "/login/oauth/access_token" && r.URL.Query().Get("grant_type") == "refresh_token":
		rt := r.URL.Query().Get("refresh_token")
		u, ok := f.refresh[rt]
		if !ok {
			json.NewEncoder(w).Encode(map[string]string{"error": "bad_refresh_token", "error_description": "The refresh token passed is incorrect or expired."})
			return
		}
		delete(f.refresh, rt) // GitHub rotates: the old refresh token is spent
		f.renewals++
		json.NewEncoder(w).Encode(f.issue(u, fmt.Sprintf("r%d", f.renewals)))
	case r.URL.Path == "/login/device/code":
		f.scopes = append(f.scopes, r.URL.Query().Get("scope"))
		f.n++
		dc := fmt.Sprintf("dc%d", f.n)
		json.NewEncoder(w).Encode(map[string]any{"device_code": dc, "user_code": fmt.Sprintf("ABCD-%04d", f.n), "verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 0})
	case r.URL.Path == "/login/oauth/access_token":
		dc := r.URL.Query().Get("device_code")
		if !f.approved[dc] {
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		if f.expiring {
			delete(f.approved, dc)
			json.NewEncoder(w).Encode(f.issue(max(f.nextUser, 42), dc))
			return
		}
		if f.nextUser != 0 {
			if f.tokUser == nil {
				f.tokUser = map[string]int{}
			}
			f.tokUser["gho_"+dc] = f.nextUser
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_" + dc})
	case !strings.HasPrefix(auth, "Bearer gho_") || f.revoked[strings.TrimPrefix(auth, "Bearer ")]:
		w.WriteHeader(401)
		fmt.Fprint(w, `{"message":"Bad credentials"}`)
	case f.flaky401 > 0 && strings.HasPrefix(r.URL.Path, "/gists"):
		f.flaky401--
		w.WriteHeader(401)
		fmt.Fprint(w, `{"message":"Bad credentials"}`)
	case r.URL.Path == "/user":
		if u := f.userOf(auth); u != 42 {
			fmt.Fprintf(w, `{"id":%d,"login":"user%d","name":"User %d"}`, u, u, u)
		} else {
			fmt.Fprint(w, `{"id":42,"login":"tom","name":"Tom"}`)
		}
	case r.URL.Path == "/gists" && r.Method == "GET":
		var out []map[string]any
		for id, files := range f.gists {
			if o := f.owner[id]; o != 0 && o != f.userOf(auth) || o == 0 && f.userOf(auth) != 42 {
				continue
			}
			fs := map[string]any{}
			for n := range files {
				fs[n] = map[string]any{}
			}
			out = append(out, map[string]any{"id": id, "description": f.desc[id], "files": fs})
		}
		json.NewEncoder(w).Encode(out)
	case r.URL.Path == "/gists" && r.Method == "POST":
		var b struct {
			Description string
			Public      bool
			Files       map[string]struct{ Content string }
		}
		json.NewDecoder(r.Body).Decode(&b)
		if b.Public {
			w.WriteHeader(422)
			return
		}
		id := fmt.Sprintf("g%d", len(f.gists)+1)
		f.gists[id], f.desc[id] = map[string]string{}, b.Description
		if f.owner == nil {
			f.owner = map[string]int{}
		}
		f.owner[id] = f.userOf(auth)
		fs := map[string]any{}
		for n, c := range b.Files {
			f.gists[id][n] = c.Content
			fs[n] = map[string]any{"raw_url": f.rawURL(id, n, c.Content)}
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "files": fs})
	case strings.HasPrefix(r.URL.Path, "/gists/"):
		id := strings.TrimPrefix(r.URL.Path, "/gists/")
		g, ok := f.gists[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		if r.Method == "PATCH" {
			var b struct {
				Files map[string]struct{ Content string }
			}
			json.NewDecoder(r.Body).Decode(&b)
			for n, c := range b.Files {
				g[n] = c.Content
			}
		}
		fs := map[string]any{}
		for n, c := range g {
			fs[n] = map[string]any{"content": c, "raw_url": f.rawURL(id, n, c)}
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "files": fs})
	default:
		w.WriteHeader(404)
	}
}

// issue: an eight-hour access token and a refresh token, as GitHub answers (expires_in as a
// number).
func (f *fakeGitHub) issue(user int, tag string) map[string]any {
	f.issued++
	if f.tokUser == nil {
		f.tokUser = map[string]int{}
	}
	if f.refresh == nil {
		f.refresh = map[string]int{}
	}
	at, rt := fmt.Sprintf("gho_%s_%d", tag, f.issued), fmt.Sprintf("ghr_%s_%d", tag, f.issued)
	f.tokUser[at], f.refresh[rt] = user, user
	return map[string]any{"access_token": at, "expires_in": 28800, "refresh_token": rt, "refresh_token_expires_in": 15897600, "scope": "gist", "token_type": "bearer"}
}

// expireAccess: every access token issued so far stops working (eight hours have passed).
func (f *fakeGitHub) expireAccess() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for tok := range f.tokUser {
		f.revoked[tok] = true
	}
}

// revokeApp: the user revoked NEXUS at github.com/settings/applications.
func (f *fakeGitHub) revokeApp() {
	f.expireAccess()
	f.mu.Lock()
	f.refresh = map[string]int{}
	f.mu.Unlock()
}

func signInGitHub(t *testing.T, a *App, f *fakeGitHub, name string) {
	t.Helper()
	v, err := a.Account.StartGitHubSignIn(name)
	if err != nil || v["userCode"] == "" {
		t.Fatal(err)
	}
	if st, _ := a.Account.GitHubSignInStatus(); st != "pending" || a.Account.Status().SignedIn {
		t.Fatal("signed in before approval on GitHub")
	}
	f.mu.Lock()
	f.approved[fmt.Sprintf("dc%d", f.n)] = true
	f.mu.Unlock()
	a.Account.ghFlow.next = a.Account.ghFlow.next.Add(-1e12)
	if st, err := a.Account.GitHubSignInStatus(); st != "approved" || err != nil {
		t.Fatalf("%s %v", st, err)
	}
}

// TestAccountGitHub: sign in with GitHub on a PC and a Steam Deck, synchronise through a private
// Gist, remove a device.
func TestAccountGitHub(t *testing.T) {
	t.Setenv("NEXUS_ACCOUNT_SERVER", "")
	f := &fakeGitHub{gists: map[string]map[string]string{}, desc: map[string]string{}, approved: map[string]bool{}}
	ts := httptest.NewServer(f)
	defer ts.Close()
	oldW, oldA := githubWeb, githubAPI
	githubWeb, githubAPI = ts.URL, ts.URL
	defer func() { githubWeb, githubAPI = oldW, oldA }()

	pc, _ := newTestApp(t, "")
	deck, _ := newTestApp(t, "")
	tom, _ := pc.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	pc.Amiibo.CreateVirtual("0100000000040002", "Link", "", tom.ID)
	pc.Profiles.AddSession(tom.ID, "t-0100aaaa00010000", "Alpha Quest", SessionRec{Start: 1, End: 3601, Seconds: 3600})

	signInGitHub(t, pc, f, "Tom's PC")
	st := pc.Account.Status()
	if !st.SignedIn || st.Provider != "github" || st.Email != "@tom" || len(st.Devices) != 1 {
		t.Fatalf("%+v", st)
	}
	mustSync(t, pc)
	if len(f.gists) != 1 {
		t.Fatalf("%d gists", len(f.gists))
	}
	for _, g := range f.gists {
		if _, ok := g["nexus-profile.json"]; !ok {
			t.Fatalf("no users in the gist: %v", g)
		}
	}

	signInGitHub(t, deck, f, "Steam Deck")
	if len(f.gists) != 1 {
		t.Fatal("a second gist was created")
	}
	mustSync(t, deck)
	if names := deck.Profiles.List(); len(names) != 1 || names[0].Name != "Tom" {
		t.Fatalf("users on the deck: %+v", names)
	}
	if l := deck.Amiibo.List(); len(l) != 1 || !l[0].Available() {
		t.Fatalf("amiibo on the deck: %+v", l)
	}
	if g := deck.Profiles.Stats(deck.Profiles.List()[0].ID).Games["t-0100aaaa00010000"]; g == nil || g.Seconds != 3600 {
		t.Fatalf("play time on the deck: %+v", g)
	}
	// a change on the deck reaches the PC
	deck.Amiibo.Update(deck.Amiibo.List()[0].ID, func(x *Amiibo) { x.Name = "Hero" })
	mustSync(t, deck)
	mustSync(t, pc)
	if pc.Amiibo.List()[0].Name != "Hero" {
		t.Fatal("deck change not received")
	}
	// both change the same thing: conflict, not overwrite
	pc.Amiibo.Update(pc.Amiibo.List()[0].ID, func(x *Amiibo) { x.Name = "PC" })
	deck.Amiibo.Update(deck.Amiibo.List()[0].ID, func(x *Amiibo) { x.Name = "Deck" })
	mustSync(t, pc)
	if sum := mustSync(t, deck); sum.Conflicts != 1 || deck.Amiibo.List()[0].Name != "Deck" {
		t.Fatalf("%+v", sum)
	}
	// remove the deck from the PC: the deck signs itself out at its next sync
	pc.Account.Refresh()
	var deckID string
	for _, d := range pc.Account.Status().Devices {
		if !d.Current {
			deckID = d.ID
		}
	}
	if err := pc.Account.RemoveDevice(deckID); err != nil || len(pc.Account.Status().Devices) != 1 {
		t.Fatal("remove", err)
	}
	if _, err := deck.Account.Sync(); err == nil || deck.Account.Status().SignedIn {
		t.Fatal("removed device still signed in")
	}
	if len(deck.Profiles.List()) != 1 {
		t.Fatal("local data lost")
	}
}
