package drive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func prepared(t *testing.T) (string, string) {
	t.Helper()
	parent := t.TempDir()
	file := filepath.Join(parent, "desktop.json")
	data := []byte(`{"installed":{"client_id":"123-fixture.apps.googleusercontent.com","client_secret":"fixture-client-secret","project_id":"homevault-fixture","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "connection")
	if err := Setup(root, file, "gdrive", "homevault-fixture"); err != nil {
		t.Fatal(err)
	}
	return root, file
}

func callback(t *testing.T, link, state, code, denied, issuer, host string, expected int) {
	t.Helper()
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	q := url.Values{"state": {state}}
	if code != "" {
		q.Set("code", code)
	}
	if denied != "" {
		q.Set("error", denied)
	}
	if issuer != "" {
		q.Set("iss", issuer)
	}
	req, err := http.NewRequest("GET", parsed.Query().Get("redirect_uri")+"?"+q.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != expected || strings.Contains(string(body), code) && code != "" {
		t.Fatalf("callback status/privacy: %d", response.StatusCode)
	}
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("callback response not private")
	}
}

func TestOAuthPKCEStateAndPrivatePersistence(t *testing.T) {
	root, _ := prepared(t)
	var authorization *url.URL
	exchanges := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		exchanges++
		if r.URL.String() != tokenURL || r.Method != "POST" {
			t.Fatal("unexpected token destination", r.URL)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		verifier := r.Form.Get("code_verifier")
		hash := sha256.Sum256([]byte(verifier))
		if len(verifier) < 43 || base64.RawURLEncoding.EncodeToString(hash[:]) != authorization.Query().Get("code_challenge") || authorization.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("PKCE not bound to request")
		}
		if r.Form.Get("code") != "fixture-code-sensitive" || r.Form.Get("client_secret") != "fixture-client-secret" || r.Form.Get("redirect_uri") != authorization.Query().Get("redirect_uri") {
			t.Fatal("token form differs from prepared client/redirect")
		}
		body := `{"access_token":"fixture-access-sensitive","refresh_token":"fixture-refresh-sensitive","token_type":"Bearer","expires_in":3600,"scope":"` + Scope + `"}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	err := authorize(context.Background(), root, 0, func(link string) error {
		var err error
		authorization, err = url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		q := authorization.Query()
		if authorization.Scheme != "https" || authorization.Host != "accounts.google.com" || q.Get("scope") != Scope || q.Get("access_type") != "offline" || !strings.Contains(q.Get("prompt"), "select_account") || strings.Contains(link, "fixture-client-secret") {
			t.Fatal("unsafe auth URL")
		}
		callback(t, link, "wrong-state", "not-accepted", "", "", "", 400)
		callback(t, link, q.Get("state"), "not-accepted", "", "https://evil.invalid", "", 400)
		callback(t, link, q.Get("state"), "not-accepted", "", "", "evil.invalid", 400)
		callback(t, link, q.Get("state"), "fixture-code-sensitive", "", "https://accounts.google.com", "", 200)
		callback(t, link, q.Get("state"), "replayed-code", "", "", "", 400)
		return nil
	}, client)
	if err != nil || exchanges != 1 {
		t.Fatal("OAuth failed", err, exchanges)
	}
	c, err := readConnection(root)
	if err != nil || c.token == nil || c.token.RefreshToken != "fixture-refresh-sensitive" {
		t.Fatal("token not privately saved", err)
	}
	s, err := Status(context.Background(), root, false, "")
	if err != nil || s.State != "authorized_cached" || !s.RefreshAvailable || s.LiveChecked {
		t.Fatal("bad local status", s, err)
	}
	data, _ := json.Marshal(s)
	for _, secret := range []string{"fixture-access-sensitive", "fixture-refresh-sensitive", "fixture-client-secret"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("secret in status")
		}
	}
	before, _ := os.ReadFile(filepath.Join(root, "rclone.conf"))
	if err := authorize(context.Background(), root, 0, func(string) error { t.Fatal("reauthorization started over existing token"); return nil }, client); err == nil {
		t.Fatal("existing token overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(root, "rclone.conf"))
	if !bytes.Equal(before, after) {
		t.Fatal("existing token changed")
	}
}

func TestOAuthFailuresKeepOriginalConfigAndRedact(t *testing.T) {
	for _, which := range []string{"denied", "exchange", "refresh-missing", "scope", "changed", "cancel"} {
		t.Run(which, func(t *testing.T) {
			root, _ := prepared(t)
			path := filepath.Join(root, "rclone.conf")
			before, _ := os.ReadFile(path)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if which == "exchange" {
					return nil, errors.New("PRIVATE_TOKEN_MUST_NOT_ECHO")
				}
				refresh := "fixture-refresh"
				if which == "refresh-missing" {
					refresh = ""
				}
				scope := Scope
				if which == "scope" {
					scope = "https://www.googleapis.com/auth/drive"
				}
				body := `{"access_token":"fixture-access","refresh_token":"` + refresh + `","token_type":"Bearer","expires_in":3600,"scope":"` + scope + `"}`
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			err := authorize(ctx, root, 0, func(link string) error {
				parsed, _ := url.Parse(link)
				if which == "cancel" {
					cancel()
					return nil
				}
				if which == "changed" {
					if err := os.WriteFile(path, append(before, []byte("# external edit\n")...), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if which == "denied" {
					callback(t, link, parsed.Query().Get("state"), "", "access_denied", "", "", 200)
				} else {
					callback(t, link, parsed.Query().Get("state"), "fixture-code", "", "", "", 200)
				}
				return nil
			}, client)
			if err == nil || strings.Contains(err.Error(), "PRIVATE_TOKEN_MUST_NOT_ECHO") {
				t.Fatal("unsafe failure result", err)
			}
			after, _ := os.ReadFile(path)
			if which != "changed" && !bytes.Equal(before, after) {
				t.Fatal("failed authorization altered config")
			}
			if strings.Contains(string(after), "token =") {
				t.Fatal("failed flow saved a token")
			}
		})
	}
}

func TestSetupRefusesOverwriteAndUnsafeInputs(t *testing.T) {
	root, file := prepared(t)
	before, _ := os.ReadFile(filepath.Join(root, "rclone.conf"))
	if err := Setup(root, file, "gdrive", ""); err == nil {
		t.Fatal("existing config overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(root, "rclone.conf"))
	if !bytes.Equal(before, after) {
		t.Fatal("existing config changed")
	}
	if err := Setup(filepath.Join(filepath.Dir(root), "bad"), file, "bad\nremote", ""); err == nil {
		t.Fatal("remote injection accepted")
	}
	if err := Setup(filepath.Join(filepath.Dir(root), "bad"), file, "gdrive", "other-project"); err == nil {
		t.Fatal("wrong project accepted")
	}
	if err := os.WriteFile(file, []byte(`{"web":{"client_id":"not-desktop"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Setup(filepath.Join(filepath.Dir(root), "bad"), file, "gdrive", ""); err == nil {
		t.Fatal("web client accepted")
	}
}
