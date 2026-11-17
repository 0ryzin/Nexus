package main

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fakeGitHubServer(t *testing.T) *fakeGitHub {
	t.Helper()
	t.Setenv("NEXUS_ACCOUNT_SERVER", "")
	f := &fakeGitHub{gists: map[string]map[string]string{}, desc: map[string]string{}, approved: map[string]bool{}, revoked: map[string]bool{}}
	ts := httptest.NewServer(f)
	t.Cleanup(ts.Close)
	oldW, oldA := githubWeb, githubAPI
	githubWeb, githubAPI = ts.URL, ts.URL
	t.Cleanup(func() { githubWeb, githubAPI = oldW, oldA })
	return f
}

// gistUsers: the user names in a GitHub user's NEXUS sync data.
func gistUsers(f *fakeGitHub, user int) string {
	var items map[string]remoteItem
	_ = json.Unmarshal([]byte(gistOf(f, user)["nexus-profile.json"]), &items)
	var names []string
	for _, it := range items {
		var p struct{ Name string }
		_ = json.Unmarshal(it.Data, &p)
		names = append(names, p.Name)
	}
	return strings.Join(names, ",")
}

func gistOf(f *fakeGitHub, user int) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, o := range f.owner {
		if o == user {
			return f.gists[id]
		}
	}
	return nil
}

// TestAccountSwitchHoldsUploads: signing out and in with a different GitHub account does not
// copy this device's data (which came from the first account) into the second until the user
// chooses; downloads continue; signing out keeps every local user.
func TestAccountSwitchHoldsUploads(t *testing.T) {
	f := fakeGitHubServer(t)
	pc, _ := newTestApp(t, "")
	pc.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	signInGitHub(t, pc, f, "PC")
	mustSync(t, pc)
	if !strings.Contains(gistUsers(f, 42), "Tom") {
		t.Fatal("first account did not receive the user")
	}
	if err := pc.Account.SignOut(); err != nil {
		t.Fatal(err)
	}
	if len(pc.Profiles.List()) != 1 {
		t.Fatal("signing out removed a local user")
	}

	f.nextUser = 77
	signInGitHub(t, pc, f, "PC")
	st := pc.Account.Status()
	if !st.HoldUploads || st.State != "held" || st.HoldFrom != "@tom" {
		t.Fatalf("switch not held: %+v", st)
	}
	mustSync(t, pc)
	if strings.Contains(gistUsers(f, 77), "Tom") {
		t.Fatal("the first account's user was uploaded to the second account")
	}
	if err := pc.Account.ReleaseHold(true); err != nil {
		t.Fatal(err)
	}
	mustSync(t, pc)
	if !strings.Contains(gistUsers(f, 77), "Tom") {
		t.Fatal("after choosing to send, the user did not reach the second account")
	}
	// the same account again does not hold
	pc.Account.SignOut()
	signInGitHub(t, pc, f, "PC")
	if pc.Account.Status().HoldUploads {
		t.Fatal("signing in to the same account again held uploads")
	}
}

// TestAccountKeepHereReleasesHold: choosing to keep this device's data sends only later changes.
func TestAccountKeepHereReleasesHold(t *testing.T) {
	f := fakeGitHubServer(t)
	pc, _ := newTestApp(t, "")
	pc.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	signInGitHub(t, pc, f, "PC")
	mustSync(t, pc)
	pc.Account.SignOut()
	f.nextUser = 77
	signInGitHub(t, pc, f, "PC")
	pc.Account.ReleaseHold(false)
	mustSync(t, pc)
	if strings.Contains(gistUsers(f, 77), "Tom") {
		t.Fatal("kept data was sent")
	}
	ann, _ := pc.Profiles.Create("Ann", Avatar{Kind: "glyph"})
	_ = ann
	mustSync(t, pc)
	if g := gistUsers(f, 77); !strings.Contains(g, "Ann") || strings.Contains(g, "Tom") {
		t.Fatalf("later change not sent or kept data sent: %s", g)
	}
}

// TestAccountTransient401: one 401 from GitHub while /user still accepts the sign-in keeps the
// device signed in (it is retried); a sign-in GitHub really revoked ends the session.
func TestAccountTransient401(t *testing.T) {
	f := fakeGitHubServer(t)
	pc, _ := newTestApp(t, "")
	signInGitHub(t, pc, f, "PC")
	mustSync(t, pc)
	f.mu.Lock()
	f.flaky401 = 1
	f.mu.Unlock()
	if _, err := pc.Account.Sync(); !errors.Is(err, errOffline) {
		t.Fatalf("a passing 401: %v", err)
	}
	if !pc.Account.Status().SignedIn {
		t.Fatal("a passing 401 signed the device out")
	}
	mustSync(t, pc)

	f.mu.Lock()
	f.revoked["gho_"+strings.TrimPrefix(pc.Account.token, "gho_")] = true
	f.mu.Unlock()
	pc.Account.Sync()
	if st := pc.Account.Status(); st.SignedIn || st.Ended == "" {
		t.Fatalf("revoked sign-in kept: %+v", st)
	}
}

// TestAccountTokenReadRetried: a stored sign-in that cannot be read for a moment keeps the
// device signed in and is read again later; five failures in a row ask to sign in again.
func TestAccountTokenReadRetried(t *testing.T) {
	f := fakeGitHubServer(t)
	pc, _ := newTestApp(t, "")
	signInGitHub(t, pc, f, "PC")
	mustSync(t, pc)
	ac := pc.Account
	path := ac.tokenPath()
	good, _ := os.ReadFile(path)
	unreadable := func() { os.Remove(path); os.Mkdir(path, 0o700) } // a read error, not "missing"
	ac.mu.Lock()
	ac.token = ""
	ac.mu.Unlock()
	unreadable()
	if _, err := ac.Sync(); !errors.Is(err, errOffline) || !ac.Status().SignedIn {
		t.Fatalf("one unreadable read: %v %+v", err, ac.Status())
	}
	os.Remove(path)
	os.WriteFile(path, good, 0o600)
	mustSync(t, pc)

	ac.mu.Lock()
	ac.token = ""
	ac.mu.Unlock()
	unreadable()
	for i := 0; i < 6; i++ {
		ac.Sync()
	}
	if st := ac.Status(); st.SignedIn {
		t.Fatal("a sign-in that never becomes readable stays signed in")
	}
}

// TestSyncWaitsOnlyForAGameInFront: a game kept running behind Home does not stop
// synchronisation (it used to, for as long as the game ran).
func TestSyncWaitsOnlyForAGameInFront(t *testing.T) {
	if !syncWaitsForGame(RunState{Running: true}, "synced") {
		t.Fatal("game in front: should wait")
	}
	if syncWaitsForGame(RunState{Running: true, Suspended: true, Background: true}, "synced") {
		t.Fatal("game behind Home stopped synchronisation")
	}
	if syncWaitsForGame(RunState{Running: true}, "pending") || syncWaitsForGame(RunState{}, "synced") {
		t.Fatal("pending changes or no game should not wait")
	}
}
