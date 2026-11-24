package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Tom's PC (0.8.1–0.9.2): the account page stayed blank and the Home account indicator was
// missing because Status waited for a synchronisation that was sending pictures.
func TestStatusFastWhileArtUploads(t *testing.T) {
	t.Setenv("NEXUS_ACCOUNT_SERVER", "")
	f := &fakeGitHub{gists: map[string]map[string]string{}, desc: map[string]string{}, approved: map[string]bool{}}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "gists") || r.Method == "PATCH" {
			time.Sleep(700 * time.Millisecond) // a picture upload on a home connection
		}
		f.ServeHTTP(w, r)
	}))
	defer slow.Close()
	f.base = slow.URL
	oldWeb, oldAPI := githubWeb, githubAPI
	githubWeb, githubAPI = slow.URL, slow.URL
	defer func() { githubWeb, githubAPI = oldWeb, oldAPI }()
	pc, _ := newTestApp(t, "")
	for _, tid := range []string{"0400486024DEC000", "0100AAAA00010000", "0100BBBB00010000"} {
		pc.installSyncedArt("t-"+strings.ToLower(tid), "icon", append([]byte("\xff\xd8\xff"), bytes.Repeat([]byte(tid), 50000)...), ".jpg")
	}
	signInGitHub(t, pc, f, "PC")
	done := make(chan struct{})
	go func() { pc.Account.Sync(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	worst := time.Duration(0)
	for i := 0; i < 5; i++ {
		start := time.Now()
		st := pc.Account.Status()
		if d := time.Since(start); d > worst {
			worst = d
		}
		if !st.SignedIn {
			t.Fatal("status lost the sign-in during the upload")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if worst > 400*time.Millisecond {
		t.Fatalf("Status took %v while pictures were being sent (was about 2 s before 0.9.3)", worst)
	}
	t.Logf("slowest Status during the upload: %v", worst.Round(time.Millisecond))
	<-done
	if st := pc.Account.Status(); st.Pending != 0 {
		t.Fatalf("after the synchronisation %d item(s) still pending", st.Pending)
	}
}

// 24 Nov, Tom's PC on 0.9.2: the background synchronisation started and never finished (no
// answer from GitHub). Status must still answer at once, so the account page and the Home
// account indicator appear.
func TestStatusFastWhileGitHubHangs(t *testing.T) {
	t.Setenv("NEXUS_ACCOUNT_SERVER", "")
	f := &fakeGitHub{gists: map[string]map[string]string{}, desc: map[string]string{}, approved: map[string]bool{}}
	hang := make(chan struct{})
	var hanging atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hanging.Load() {
			select {
			case <-hang:
			case <-r.Context().Done():
			}
			return
		}
		f.ServeHTTP(w, r)
	}))
	defer srv.Close()
	defer close(hang)
	f.base = srv.URL
	oldWeb, oldAPI := githubWeb, githubAPI
	githubWeb, githubAPI = srv.URL, srv.URL
	defer func() { githubWeb, githubAPI = oldWeb, oldAPI }()
	pc, _ := newTestApp(t, "")
	signInGitHub(t, pc, f, "PC")
	hanging.Store(true)
	go pc.Account.Sync()
	time.Sleep(300 * time.Millisecond)
	for i := 0; i < 3; i++ {
		start := time.Now()
		st := pc.Account.Status()
		if d := time.Since(start); d > 400*time.Millisecond {
			t.Fatalf("Status took %v while GitHub did not answer", d)
		}
		if !st.SignedIn {
			t.Fatal("status lost the sign-in")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
