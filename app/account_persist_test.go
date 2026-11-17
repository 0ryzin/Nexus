package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The GitHub sign-out the user kept seeing: GitHub's sign-ins for OAuth apps last eight hours
// since August 2026 and NEXUS kept only the access token, so the first start eight hours
// after signing in was refused (401) and NEXUS signed out. These tests follow the
// regression steps of the refinement brief, with a GitHub stand-in that issues expiring
// tokens and refresh tokens the way github.com does.

// restartApp closes a and starts NEXUS again on the same data (install folder optional:
// the user's sign-outs happened to coincide with running a new download from a new folder).
func restartApp(t *testing.T, a *App, newInstall bool) *App {
	t.Helper()
	data := a.Paths.Data
	root := a.Paths.Root
	stopTestSession(a)
	a.Close()
	if newInstall {
		root = filepath.Join(t.TempDir(), "NEXUS-new-download")
	}
	b, err := testNewAppWithPaths(t, NewPathsSplit(root, data, a.Paths.IsPortable))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func requireConnected(t *testing.T, a *App, step string) {
	t.Helper()
	st := a.Account.Status()
	if !st.SignedIn || st.Connection != "connected" || st.Email != "@tom" {
		t.Fatalf("%s: not connected: signedIn=%v connection=%s email=%s ended=%q", step, st.SignedIn, st.Connection, st.Email, st.Ended)
	}
}

// storedCred reads session.bin the way NEXUS does (decrypted on Windows).
func storedCred(t *testing.T, a *App) ghCred {
	t.Helper()
	b, err := loadSecretFile(a.Account.tokenPath())
	if err != nil {
		t.Fatal(err)
	}
	return decodeCred(b)
}

// ageStoredSignIn makes the saved sign-in look issued more than eight hours ago, as after a
// night away, and tells the stand-in that those access tokens no longer work.
func ageStoredSignIn(t *testing.T, a *App, f *fakeGitHub) {
	t.Helper()
	c := storedCred(t, a)
	c.Expires = time.Now().Add(-time.Hour).Unix()
	if err := storeSecretFile(a.Account.tokenPath(), c.encode()); err != nil {
		t.Fatal(err)
	}
	f.expireAccess()
}

func TestGitHubSignInPersists(t *testing.T) {
	f := fakeGitHubServer(t)
	f.expiring = true
	pc, _ := newTestApp(t, "")
	pc.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})

	// 1. connect
	signInGitHub(t, pc, f, "Toms_Pc")
	if len(f.scopes) != 1 || !strings.Contains(f.scopes[0], "offline_access") || !strings.Contains(f.scopes[0], "gist") {
		t.Fatalf("scopes asked for: %v", f.scopes)
	}
	mustSync(t, pc)
	requireConnected(t, pc, "1 connect")
	c := storedCred(t, pc)
	if c.Refresh == "" || c.Expires == 0 || c.RefreshExp == 0 {
		t.Fatalf("1: the refresh token was not kept: %+v", c.Refresh != "")
	}
	raw, _ := os.ReadFile(pc.Account.tokenPath())
	if runtime.GOOS == "windows" && (bytes.Contains(raw, []byte(c.Access)) || bytes.Contains(raw, []byte(c.Refresh))) {
		t.Fatal("1: the sign-in is stored in plain text")
	}
	if b, _ := os.ReadFile(filepath.Join(pc.Account.dir, "account.json")); bytes.Contains(b, []byte(c.Access)) || bytes.Contains(b, []byte(c.Refresh)) {
		t.Fatal("1: a token reached account.json")
	}
	deviceID := pc.Account.Status().DeviceID

	// 2. restart NEXUS: still connected; and again after eight hours (the user's sign-out)
	pc = restartApp(t, pc, false)
	requireConnected(t, pc, "2 restart")
	mustSync(t, pc)
	ageStoredSignIn(t, pc, f)
	pc = restartApp(t, pc, true) // a new download in a new folder, the same data
	requireConnected(t, pc, "2 restart after 8 h")
	mustSync(t, pc)
	requireConnected(t, pc, "2 sync after 8 h")
	if f.renewals != 1 {
		t.Fatalf("2: renewals %d, want 1", f.renewals)
	}
	if c2 := storedCred(t, pc); c2.Refresh == c.Refresh || c2.Access == c.Access {
		t.Fatal("2: the renewed sign-in was not saved")
	}

	// GitHub refuses the access token earlier than its stated expiry: renewed, not signed out
	f.expireAccess()
	mustSync(t, pc)
	requireConnected(t, pc, "2 early 401")
	if f.renewals != 2 {
		t.Fatalf("2: renewals %d, want 2", f.renewals)
	}

	// 3. NEXUS user signs out and back in (switching users on the "Who's playing" screen),
	// settings change, a user is added: the GitHub connection is untouched
	tom := pc.Profiles.List()[0]
	pc.Settings.Update(func(g *GlobalSettings) { g.LastProfileID = "" })
	ann, _ := pc.Profiles.Create("Ann", Avatar{Kind: "preset", Preset: "star", Color: "#00ff00"})
	pc.Settings.Update(func(g *GlobalSettings) { g.LastProfileID = ann.ID })
	pc.Settings.Update(func(g *GlobalSettings) { g.LastProfileID = tom.ID })
	requireConnected(t, pc, "3 user switch")

	// 4. synchronise
	if _, err := pc.Account.Sync(); err != nil {
		t.Fatalf("4: %v", err)
	}
	requireConnected(t, pc, "4 sync")

	// 5. the network fails (also while a renewal is due), then recovers
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
	if _, err := pc.Account.Sync(); !errors.Is(err, errOffline) {
		t.Fatalf("5: offline sync: %v", err)
	}
	if st := pc.Account.Status(); !st.SignedIn || st.Connection != "problem" {
		t.Fatalf("5: a network failure changed the connection: %+v", st.Connection)
	}
	ageStoredSignIn(t, pc, f)
	pc = restartApp(t, pc, false) // restarted while offline and due for renewal
	if _, err := pc.Account.Sync(); !errors.Is(err, errOffline) {
		t.Fatalf("5: offline sync after restart: %v", err)
	}
	if !pc.Account.Status().SignedIn {
		t.Fatal("5: signed out while offline")
	}
	if _, err := pc.Account.TestConnection(); err == nil || !strings.Contains(err.Error(), "cannot be reached") {
		t.Fatalf("5: test connection offline: %v", err)
	}
	f.mu.Lock()
	f.down = false
	f.mu.Unlock()
	mustSync(t, pc) // synchronisation resumes
	requireConnected(t, pc, "5 recovered")
	if v, err := pc.Account.TestConnection(); err != nil || v["login"] != "@tom" {
		t.Fatalf("5: test connection: %v %v", v, err)
	}

	// 6. restart again
	pc = restartApp(t, pc, false)
	requireConnected(t, pc, "6 restart")
	mustSync(t, pc)
	if pc.Account.Status().DeviceID != deviceID {
		t.Fatal("6: this PC got a new device identity")
	}

	// 7. disconnect explicitly: the stored sign-in is gone, local data stays
	if err := pc.Account.SignOut(); err != nil {
		t.Fatalf("7: %v", err)
	}
	if st := pc.Account.Status(); st.SignedIn || st.Connection != "notconnected" {
		t.Fatalf("7: still connected: %+v", st.Connection)
	}
	if _, err := os.Stat(pc.Account.tokenPath()); !os.IsNotExist(err) {
		t.Fatal("7: the stored sign-in was not removed")
	}
	if len(pc.Profiles.List()) != 2 {
		t.Fatal("7: local users lost")
	}
	pc = restartApp(t, pc, false)
	if pc.Account.Status().SignedIn {
		t.Fatal("7: connected again after restart")
	}
}

// TestGitHubRevokedEndsSignIn: only GitHub refusing the refresh token itself (NEXUS revoked at
// github.com) ends the sign-in, and the interface then says so and keeps who it was.
func TestGitHubRevokedEndsSignIn(t *testing.T) {
	f := fakeGitHubServer(t)
	f.expiring = true
	pc, _ := newTestApp(t, "")
	signInGitHub(t, pc, f, "PC")
	mustSync(t, pc)
	f.revokeApp()
	if _, err := pc.Account.Sync(); err == nil {
		t.Fatal("sync with a revoked sign-in succeeded")
	}
	st := pc.Account.Status()
	if st.SignedIn || st.Connection != "expired" || st.Email != "@tom" || st.Ended == "" {
		t.Fatalf("revoked: %v %s %s", st.SignedIn, st.Connection, st.Ended)
	}
	// reconnect: the same device entry, not another "PC" in the device list
	id := st.DeviceID
	signInGitHub(t, pc, f, "PC")
	mustSync(t, pc)
	requireConnected(t, pc, "reconnected")
	pc.Account.Refresh()
	if st := pc.Account.Status(); st.DeviceID != id || len(st.Devices) != 1 {
		t.Fatalf("reconnect: same device %v, %d devices", st.DeviceID == id, len(st.Devices))
	}
}

// TestGitHubRenewalServerTrouble: GitHub failing while renewing (5xx, or an unknown error)
// is temporary; the sign-in is kept.
func TestGitHubRenewalServerTrouble(t *testing.T) {
	f := fakeGitHubServer(t)
	f.expiring = true
	pc, _ := newTestApp(t, "")
	signInGitHub(t, pc, f, "PC")
	mustSync(t, pc)
	ageStoredSignIn(t, pc, f)
	pc = restartApp(t, pc, false)

	// a renewal answered 503 by a proxy in front of GitHub
	trouble := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer trouble.Close()
	old := githubWeb
	githubWeb = trouble.URL
	if _, err := pc.Account.Sync(); !errors.Is(err, errOffline) || !pc.Account.Status().SignedIn {
		t.Fatalf("503 during renewal: %v %v", err, pc.Account.Status().SignedIn)
	}
	githubWeb = old
	mustSync(t, pc)
	requireConnected(t, pc, "after trouble")
}

// TestOldSignInStillWorks: a session.bin from NEXUS 0.7 (the access token alone) is read, and
// is not signed out while GitHub accepts it.
func TestOldSignInStillWorks(t *testing.T) {
	f := fakeGitHubServer(t)
	pc, _ := newTestApp(t, "")
	signInGitHub(t, pc, f, "PC") // the stand-in issues long-lived tokens unless expiring is set
	mustSync(t, pc)
	if c := storedCred(t, pc); c.Refresh != "" {
		t.Fatal("unexpected refresh token")
	}
	raw, _ := loadSecretFile(pc.Account.tokenPath())
	if len(raw) == 0 || raw[0] == '{' {
		t.Fatal("a long-lived sign-in should keep the old file format")
	}
	pc = restartApp(t, pc, false)
	mustSync(t, pc)
	requireConnected(t, pc, "old format")
}
