package accountd

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func testServer(t *testing.T) (*httptest.Server, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	c := &clock{t: time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)}
	s, err := New(Config{DataDir: dir, Iterations: 2000, Now: c.now, Logger: log.New(io.Discard, "", 0), RateLimit: 30})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return ts, c, dir
}

func call(t *testing.T, ts *httptest.Server, method, path, token string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

var pc = DeviceInfo{Name: "Tom's PC", Platform: "windows"}
var deck = DeviceInfo{Name: "Steam Deck", Platform: "linux"}

func register(t *testing.T, ts *httptest.Server) SessionResult {
	var r SessionResult
	if st := call(t, ts, "POST", "/v1/accounts", "", map[string]any{"email": "Tom@Example.org", "password": "correct horse battery", "displayName": "Tom", "device": pc}, &r); st != 201 {
		t.Fatalf("register: %d", st)
	}
	return r
}

func TestAccountsAndSessions(t *testing.T) {
	ts, clk, dir := testServer(t)
	var e map[string]string
	if call(t, ts, "POST", "/v1/accounts", "", map[string]any{"email": "tom@example.org", "password": "short", "displayName": "Tom", "device": pc}, &e); e["code"] != "weak_password" {
		t.Fatalf("weak password accepted: %v", e)
	}
	if call(t, ts, "POST", "/v1/accounts", "", map[string]any{"email": "not an email", "password": "long enough pw", "displayName": "Tom", "device": pc}, &e); e["code"] != "bad_email" {
		t.Fatal("bad email accepted")
	}
	r := register(t, ts)
	if r.Token == "" || r.RecoveryCode == "" || r.Account.Email != "tom@example.org" || len(r.Account.Devices) != 1 || !r.Account.Devices[0].Current {
		t.Fatalf("%+v", r)
	}
	if st := call(t, ts, "POST", "/v1/accounts", "", map[string]any{"email": "tom@example.org", "password": "another password", "displayName": "T", "device": pc}, &e); st != 409 {
		t.Fatal("duplicate e-mail")
	}
	// nothing secret is stored in clear
	db, _ := os.ReadFile(filepath.Join(dir, "accounts.json"))
	for _, secret := range []string{r.Token, "correct horse battery", r.RecoveryCode, NormalizeCode(r.RecoveryCode)} {
		if bytes.Contains(db, []byte(secret)) {
			t.Fatalf("secret %q stored in the database", secret)
		}
	}
	// sign in on a second device
	var s2 SessionResult
	if st := call(t, ts, "POST", "/v1/sessions", "", map[string]any{"email": "tom@example.org", "password": "correct horse battery", "device": deck}, &s2); st != 200 || len(s2.Account.Devices) != 2 {
		t.Fatalf("sign in: %d %+v", st, s2)
	}
	// wrong password: generic message, then a lock-out after 5
	for i := 0; i < 5; i++ {
		if st := call(t, ts, "POST", "/v1/sessions", "", map[string]any{"email": "tom@example.org", "password": "wrong password!", "device": deck}, &e); st != 401 {
			t.Fatalf("wrong password: %d", st)
		}
	}
	if st := call(t, ts, "POST", "/v1/sessions", "", map[string]any{"email": "tom@example.org", "password": "correct horse battery", "device": deck}, &e); st != 429 {
		t.Fatalf("no lock-out: %d", st)
	}
	clk.add(16 * time.Minute)
	if st := call(t, ts, "POST", "/v1/sessions", "", map[string]any{"email": "nobody@example.org", "password": "whatever pw", "device": deck}, &e); st != 401 || !strings.Contains(e["error"], "e-mail address or password") {
		t.Fatal("unknown e-mail must look like a wrong password")
	}
	// sign out ends that token only
	if st := call(t, ts, "DELETE", "/v1/sessions/current", s2.Token, nil, nil); st != 200 {
		t.Fatal(st)
	}
	if st := call(t, ts, "GET", "/v1/account", s2.Token, nil, &e); st != 401 {
		t.Fatal("signed-out token still works")
	}
	var acc AccountView
	if st := call(t, ts, "GET", "/v1/account", r.Token, nil, &acc); st != 200 || len(acc.Devices) != 1 {
		t.Fatalf("%d %+v", st, acc)
	}
	// tokens expire after 180 days without use
	clk.add(181 * 24 * time.Hour)
	if call(t, ts, "GET", "/v1/account", r.Token, nil, &e); e["code"] != "expired" {
		t.Fatalf("not expired: %v", e)
	}
	// the database survives a restart
	s, err := New(Config{DataDir: dir, Iterations: 2000, Logger: log.New(io.Discard, "", 0)})
	if err != nil || len(s.db.Accounts) != 1 {
		t.Fatal("restart lost the account", err)
	}
}

func TestRecoveryAndPasswordChange(t *testing.T) {
	ts, _, _ := testServer(t)
	r := register(t, ts)
	var other SessionResult
	call(t, ts, "POST", "/v1/sessions", "", map[string]any{"email": "tom@example.org", "password": "correct horse battery", "device": deck}, &other)
	var e map[string]string
	if st := call(t, ts, "POST", "/v1/recover", "", map[string]any{"email": "tom@example.org", "recoveryCode": "AAAAAA-BBBBBB-CCCCCC-DDDDDD", "newPassword": "a brand new password", "device": pc}, &e); st != 401 {
		t.Fatal("wrong recovery code accepted")
	}
	var rec SessionResult
	if st := call(t, ts, "POST", "/v1/recover", "", map[string]any{"email": "tom@example.org", "recoveryCode": strings.ToLower(r.RecoveryCode), "newPassword": "a brand new password", "device": pc}, &rec); st != 200 || rec.RecoveryCode == "" || rec.RecoveryCode == r.RecoveryCode {
		t.Fatalf("recover: %d", st)
	}
	for _, tok := range []string{r.Token, other.Token} {
		if st := call(t, ts, "GET", "/v1/account", tok, nil, nil); st != 401 {
			t.Fatal("old devices still signed in after recovery")
		}
	}
	if st := call(t, ts, "POST", "/v1/recover", "", map[string]any{"email": "tom@example.org", "recoveryCode": r.RecoveryCode, "newPassword": "a brand new password", "device": pc}, &e); st != 401 {
		t.Fatal("recovery code reusable")
	}
	// change password: needs the current one; other devices are signed out
	call(t, ts, "POST", "/v1/sessions", "", map[string]any{"email": "tom@example.org", "password": "a brand new password", "device": deck}, &other)
	if st := call(t, ts, "PATCH", "/v1/account", rec.Token, map[string]any{"password": "wrong", "newPassword": "third password here"}, &e); st != 403 {
		t.Fatal("password changed without the current one")
	}
	if st := call(t, ts, "PATCH", "/v1/account", rec.Token, map[string]any{"password": "a brand new password", "newPassword": "third password here", "displayName": "Thomas"}, nil); st != 200 {
		t.Fatal(st)
	}
	if call(t, ts, "GET", "/v1/account", other.Token, nil, nil) != 401 || call(t, ts, "GET", "/v1/account", rec.Token, nil, nil) != 200 {
		t.Fatal("password change: wrong devices signed out")
	}
}

func TestDeviceLinkingByCode(t *testing.T) {
	ts, clk, _ := testServer(t)
	r := register(t, ts)
	var code struct {
		Code    string
		Expires int64
	}
	if st := call(t, ts, "POST", "/v1/pair/codes", r.Token, nil, &code); st != 201 || len(code.Code) != 9 {
		t.Fatalf("code: %d %+v", st, code)
	}
	if st := call(t, ts, "POST", "/v1/pair/codes", "", nil, nil); st != 401 {
		t.Fatal("codes need a signed-in device")
	}
	var e map[string]any
	if st := call(t, ts, "POST", "/v1/pair/claim", "", map[string]any{"code": "WRONG-CODE", "device": deck}, &e); st != 400 {
		t.Fatal("wrong code accepted")
	}
	var cl struct {
		ClaimID, ClaimSecret, Account string
	}
	if st := call(t, ts, "POST", "/v1/pair/claim", "", map[string]any{"code": strings.ToLower(strings.ReplaceAll(code.Code, "-", " ")), "device": deck}, &cl); st != 201 || cl.ClaimID == "" || strings.Contains(cl.Account, "tom@") {
		t.Fatalf("claim: %d %+v", st, cl)
	}
	// single use
	if st := call(t, ts, "POST", "/v1/pair/claim", "", map[string]any{"code": code.Code, "device": deck}, &e); st != 400 {
		t.Fatal("code used twice")
	}
	var st struct {
		Status  string
		Session SessionResult
	}
	call(t, ts, "POST", "/v1/pair/claim/"+cl.ClaimID, "", map[string]any{"claimSecret": "guess"}, &e)
	if e["code"] != "no_request" {
		t.Fatal("claim status without its secret")
	}
	call(t, ts, "POST", "/v1/pair/claim/"+cl.ClaimID, "", map[string]any{"claimSecret": cl.ClaimSecret}, &st)
	if st.Status != "pending" || st.Session.Token != "" {
		t.Fatalf("token before approval: %+v", st)
	}
	var reqs struct{ Requests []map[string]any }
	call(t, ts, "GET", "/v1/pair/requests", r.Token, nil, &reqs)
	if len(reqs.Requests) != 1 || reqs.Requests[0]["name"] != "Steam Deck" || reqs.Requests[0]["platform"] != "linux" {
		t.Fatalf("%+v", reqs)
	}
	if code := call(t, ts, "POST", "/v1/pair/requests/"+reqs.Requests[0]["id"].(string), r.Token, map[string]any{"approve": true}, nil); code != 200 {
		t.Fatal(code)
	}
	call(t, ts, "POST", "/v1/pair/claim/"+cl.ClaimID, "", map[string]any{"claimSecret": cl.ClaimSecret}, &st)
	if st.Status != "approved" || st.Session.Token == "" || len(st.Session.Account.Devices) != 2 {
		t.Fatalf("approved: %+v", st)
	}
	deckTok := st.Session.Token
	// the token is handed over once
	if call(t, ts, "POST", "/v1/pair/claim/"+cl.ClaimID, "", map[string]any{"claimSecret": cl.ClaimSecret}, &e); e["code"] != "no_request" {
		t.Fatal("token handed over twice")
	}
	if call(t, ts, "GET", "/v1/account", deckTok, nil, nil) != 200 {
		t.Fatal("linked device cannot use its token")
	}
	// codes expire after 10 minutes
	call(t, ts, "POST", "/v1/pair/codes", r.Token, nil, &code)
	clk.add(11 * time.Minute)
	if st := call(t, ts, "POST", "/v1/pair/claim", "", map[string]any{"code": code.Code, "device": deck}, &e); st != 400 {
		t.Fatal("expired code accepted")
	}
	// declined
	call(t, ts, "POST", "/v1/pair/codes", r.Token, nil, &code)
	call(t, ts, "POST", "/v1/pair/claim", "", map[string]any{"code": code.Code, "device": deck}, &cl)
	call(t, ts, "GET", "/v1/pair/requests", r.Token, nil, &reqs)
	call(t, ts, "POST", "/v1/pair/requests/"+reqs.Requests[0]["id"].(string), r.Token, map[string]any{"approve": false}, nil)
	call(t, ts, "POST", "/v1/pair/claim/"+cl.ClaimID, "", map[string]any{"claimSecret": cl.ClaimSecret}, &st)
	if st.Status != "declined" {
		t.Fatalf("%+v", st)
	}
	// revoking a linked device
	var acc AccountView
	call(t, ts, "GET", "/v1/account", r.Token, nil, &acc)
	var deckID string
	for _, d := range acc.Devices {
		if d.Name == "Steam Deck" {
			deckID = d.ID
		}
	}
	if call(t, ts, "DELETE", "/v1/devices/"+deckID, r.Token, nil, &acc) != 200 || len(acc.Devices) != 1 {
		t.Fatal("remove device")
	}
	if call(t, ts, "GET", "/v1/account", deckTok, nil, nil) != 401 {
		t.Fatal("revoked device still signed in")
	}
}

func TestSyncItemsAndConflicts(t *testing.T) {
	ts, _, _ := testServer(t)
	r := register(t, ts)
	var d2 SessionResult
	call(t, ts, "POST", "/v1/sessions", "", map[string]any{"email": "tom@example.org", "password": "correct horse battery", "device": deck}, &d2)
	var it Item
	if st := call(t, ts, "PUT", "/v1/sync/profile/abc", r.Token, map[string]any{"baseVersion": 0, "data": []byte(`{"name":"Tom"}`)}, &it); st != 200 || it.Version != 1 || it.DeviceName != "Tom's PC" {
		t.Fatalf("%d %+v", st, it)
	}
	var page struct {
		Rev   int64
		Items []Item
	}
	call(t, ts, "GET", "/v1/sync?since=0", d2.Token, nil, &page)
	if page.Rev != 1 || len(page.Items) != 1 || string(page.Items[0].Data) != `{"name":"Tom"}` {
		t.Fatalf("%+v", page)
	}
	// both devices change version 1: the second upload is a conflict, not an overwrite
	call(t, ts, "PUT", "/v1/sync/profile/abc", d2.Token, map[string]any{"baseVersion": 1, "data": []byte(`{"name":"Tommy"}`)}, &it)
	var conflict struct {
		Code    string
		Current Item
	}
	if st := call(t, ts, "PUT", "/v1/sync/profile/abc", r.Token, map[string]any{"baseVersion": 1, "data": []byte(`{"name":"Thomas"}`)}, &conflict); st != 409 || conflict.Current.Version != 2 || string(conflict.Current.Data) != `{"name":"Tommy"}` {
		t.Fatalf("no conflict: %d %+v", st, conflict)
	}
	call(t, ts, "GET", "/v1/sync?since=1", r.Token, nil, &page)
	if len(page.Items) != 1 || page.Items[0].DeviceName != "Steam Deck" {
		t.Fatalf("%+v", page)
	}
	// data NEXUS never stores
	var e map[string]string
	for _, k := range []string{"saves/zelda", "keys/prod", "firmware/x", "profile/../../etc"} {
		if st := call(t, ts, "PUT", "/v1/sync/"+k, r.Token, map[string]any{"baseVersion": 0, "data": []byte("x")}, &e); st == 200 {
			t.Errorf("%s accepted", k)
		}
	}
	if st := call(t, ts, "PUT", "/v1/sync/amiibo/x", r.Token, map[string]any{"baseVersion": 0, "data": bytes.Repeat([]byte{1}, maxItemBytes+1)}, &e); st == 200 {
		t.Error("oversized item accepted")
	}
	if st := call(t, ts, "GET", "/v1/sync", "", nil, nil); st != 401 {
		t.Fatal("sync without sign-in")
	}
}

func TestRateLimit(t *testing.T) {
	ts, clk, _ := testServer(t)
	var e map[string]string
	got429 := false
	for i := 0; i < 40; i++ {
		if call(t, ts, "POST", "/v1/pair/claim", "", map[string]any{"code": "AAAA-BBBB", "device": deck}, &e) == 429 {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("code guessing is not limited")
	}
	clk.add(time.Minute)
	if call(t, ts, "POST", "/v1/pair/claim", "", map[string]any{"code": "AAAA-BBBB", "device": deck}, &e) != 400 {
		t.Fatal("limit not lifted after a minute")
	}
}
