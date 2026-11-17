package main

// GitHub sign-in that lasts.
//
// Since August 2026 GitHub gives new OAuth apps, NEXUS's included, short-lived sign-ins: an
// access token that works for eight hours and a refresh token that lasts six months from its
// last use and renews it. NEXUS 0.6–0.7 kept only the access token, so eight hours after
// signing in GitHub answered 401 and NEXUS signed the device out ("NEXUS has signed me out of
// GitHub again"). Now the whole sign-in is kept, encrypted like before (Windows data
// protection), and renewed shortly before it expires or when GitHub refuses it.
//
// The sign-in ends on this device only when the user disconnects, or when GitHub refuses the
// refresh token itself (revoked at github.com/settings/applications, or unused for six
// months). A network failure, a server error or a single 401 never ends it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ghCred is what session.bin holds (encrypted). A file from NEXUS 0.7 or earlier holds only
// the access token; it is read as a ghCred without a refresh token.
type ghCred struct {
	V          int    `json:"v"`
	Access     string `json:"access"`
	Refresh    string `json:"refresh,omitempty"`
	Expires    int64  `json:"expires,omitempty"`        // unix seconds; 0 = does not expire
	RefreshExp int64  `json:"refreshExpires,omitempty"` // unix seconds; 0 = unknown
}

func decodeCred(b []byte) ghCred {
	if len(b) > 0 && b[0] == '{' {
		var c ghCred
		if json.Unmarshal(b, &c) == nil && c.V == 2 && c.Access != "" {
			return c
		}
	}
	return ghCred{V: 2, Access: strings.TrimSpace(string(b))}
}

func (c ghCred) encode() []byte {
	if c.Refresh == "" && c.Expires == 0 {
		return []byte(c.Access) // the old format: a server-account token, or GitHub's long-lived kind
	}
	c.V = 2
	b, _ := json.Marshal(c)
	return b
}

func (ac *Account) setCredLocked(c ghCred) {
	ac.token, ac.refresh, ac.tokExp, ac.refreshExp = c.Access, c.Refresh, c.Expires, c.RefreshExp
}

func (ac *Account) credLocked() ghCred {
	return ghCred{V: 2, Access: ac.token, Refresh: ac.refresh, Expires: ac.tokExp, RefreshExp: ac.refreshExp}
}

// ghTokenAnswer is GitHub's answer from /login/oauth/access_token (device flow or renewal).
// expires_in arrives as a number or, in older answers, as a string.
type ghTokenAnswer struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    json.RawMessage `json:"expires_in"`
	RefreshIn    json.RawMessage `json:"refresh_token_expires_in"`
	Error        string          `json:"error"`
	ErrorDesc    string          `json:"error_description"`
	Interval     int             `json:"interval"`
}

func rawSeconds(r json.RawMessage) int64 {
	s := strings.Trim(string(r), `" `)
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func (a ghTokenAnswer) cred(now time.Time) ghCred {
	c := ghCred{V: 2, Access: a.AccessToken, Refresh: a.RefreshToken}
	if n := rawSeconds(a.ExpiresIn); n > 0 {
		c.Expires = now.Unix() + n
	}
	if n := rawSeconds(a.RefreshIn); n > 0 {
		c.RefreshExp = now.Unix() + n
	}
	return c
}

// ghRenewSoon: renew this long before the access token expires, so a request never starts
// with a token about to run out.
const ghRenewSoon = 10 * time.Minute

var errGitHubExpired = errors.New("GitHub no longer accepts NEXUS's sign-in on this device (it was revoked at github.com, or not used for six months). Reconnect to GitHub to continue synchronising.")

// ghRenew exchanges the refresh token for a new sign-in. force renews even when the access
// token has not expired yet (GitHub refused it). Returns errOffline when GitHub cannot be
// reached or answers with a temporary error (the sign-in is kept and renewal is tried
// again later) and errGitHubExpired only when GitHub refuses the refresh token itself.
func (ac *Account) ghRenew(force bool) error {
	ac.refreshMu.Lock()
	defer ac.refreshMu.Unlock()
	ac.mu.Lock()
	cur := ac.credLocked()
	ac.mu.Unlock()
	if cur.Refresh == "" {
		return nil // a sign-in that does not expire, or nothing to renew with
	}
	if !force && (cur.Expires == 0 || time.Until(time.Unix(cur.Expires, 0)) > ghRenewSoon) {
		return nil // still fresh (or another request renewed it while this one waited)
	}
	ac.mu.Lock()
	ac.renewing = true
	ac.mu.Unlock()
	ac.changed()
	defer func() {
		ac.mu.Lock()
		ac.renewing = false
		ac.mu.Unlock()
		ac.changed()
	}()
	form := url.Values{"client_id": {githubClientID}, "grant_type": {"refresh_token"}, "refresh_token": {cur.Refresh}}
	var r ghTokenAnswer
	code, err := ac.gh("POST", githubWeb+"/login/oauth/access_token?"+form.Encode(), nil, &r)
	if err != nil {
		if code == 0 || code >= 500 || errors.Is(err, errOffline) {
			return errOffline
		}
		return fmt.Errorf("%w (GitHub answered %d while renewing the sign-in)", errOffline, code)
	}
	switch r.Error {
	case "":
	case "bad_refresh_token", "invalid_grant":
		// GitHub itself says the refresh token is no good: revoked, expired, or already used.
		// "Already used" can only happen if a renewal's answer was lost before it was saved;
		// either way only a new sign-in helps.
		ac.app.Log.Printf("account: GitHub refused the stored refresh token (%s)", r.Error)
		return errGitHubExpired
	default:
		ac.app.Log.Printf("account: GitHub could not renew the sign-in just now (%s)", r.Error)
		return errOffline
	}
	if r.AccessToken == "" {
		return errOffline
	}
	next := r.cred(time.Now())
	if next.Refresh == "" { // GitHub rotates refresh tokens; if it ever does not, keep the old one
		next.Refresh, next.RefreshExp = cur.Refresh, cur.RefreshExp
	}
	ac.mu.Lock()
	ac.setCredLocked(next)
	ac.mu.Unlock()
	// GitHub has already retired the old refresh token: the new one must be kept now. If it
	// cannot be written, it stays in memory and writing is tried again at the next renewal or
	// synchronisation (ghPersistPending).
	if err := storeSecretFile(ac.tokenPath(), next.encode()); err != nil {
		ac.mu.Lock()
		ac.st.LastError = "NEXUS could not save the renewed GitHub sign-in on this device; it tries again."
		ac.credUnsaved = true
		ac.mu.Unlock()
		ac.app.Log.Printf("account: renewed GitHub sign-in not saved: %v", err)
	} else {
		ac.mu.Lock()
		ac.credUnsaved = false
		ac.mu.Unlock()
		ac.app.Log.Printf("account: GitHub sign-in renewed (valid until %s)", time.Unix(next.Expires, 0).Format("15:04"))
	}
	return nil
}

// ghPersistPending writes a renewed sign-in that could not be saved before.
func (ac *Account) ghPersistPending() {
	ac.mu.Lock()
	pending, c := ac.credUnsaved, ac.credLocked()
	ac.mu.Unlock()
	if !pending || c.Access == "" {
		return
	}
	if storeSecretFile(ac.tokenPath(), c.encode()) == nil {
		ac.mu.Lock()
		ac.credUnsaved = false
		ac.mu.Unlock()
	}
}

// TestConnection asks GitHub whether NEXUS's sign-in works now (renewing it if needed) and
// answers who it belongs to. A failure to reach GitHub is reported, never treated as signed out.
func (ac *Account) TestConnection() (map[string]any, error) {
	if !ac.isGitHub() || !ac.Status().SignedIn {
		return nil, errors.New("NEXUS is not connected to GitHub on this device.")
	}
	var u struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	start := time.Now()
	if _, err := ac.gh("GET", githubAPI+"/user", nil, &u); err != nil {
		if errors.Is(err, errOffline) {
			return nil, errors.New("GitHub cannot be reached right now. NEXUS stays connected and synchronises when it can.")
		}
		return nil, err
	}
	ac.mu.Lock()
	exp, rexp := ac.tokExp, ac.refreshExp
	ac.mu.Unlock()
	return map[string]any{"login": "@" + u.Login, "name": u.Name, "ms": time.Since(start).Milliseconds(), "renews": exp != 0, "expires": exp, "refreshExpires": rexp}, nil
}
