package drive

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"homevault/internal/lock"
)

type callbackResult struct {
	code   string
	denied bool
}

// Authorize uses Google's system-browser desktop flow. It never opens a browser
// itself or changes an existing token; reauthorization uses a new private setup.
func Authorize(ctx context.Context, root string, port int, showURL func(string) error) error {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("OAuth redirects refused") }}
	return authorize(ctx, root, port, showURL, client)
}

func authorize(ctx context.Context, root string, port int, showURL func(string) error, client *http.Client) (resultErr error) {
	if port != 0 && (port < 1024 || port > 65535) {
		return errors.New("OAuth port must be 0 or between 1024 and 65535")
	}
	c, err := readConnection(root)
	if err != nil {
		return err
	}
	guard, err := lock.AcquireCredential(filepath.Join(root, "rclone.conf"))
	if err != nil {
		return errors.New("OAuth config is busy or its credential lock needs manual inspection")
	}
	defer func() { resultErr = errors.Join(resultErr, guard.Release()) }()
	c, err = readConnection(root)
	if err != nil {
		return err
	}
	if c.token != nil {
		return errors.New("existing authorization retained; prepare a new private directory to reconnect")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return errors.New("cannot bind loopback OAuth port; existing listeners were not changed")
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/"
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return errors.New("cannot generate OAuth state")
	}
	state := base64.RawURLEncoding.EncodeToString(nonce[:])
	verifier := oauth2.GenerateVerifier()
	cfg := oauth2.Config{ClientID: c.id, ClientSecret: c.secret, Endpoint: oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInParams}, RedirectURL: redirect, Scopes: []string{Scope}}
	link := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "consent select_account"), oauth2.SetAuthURLParam("include_granted_scopes", "false"))
	received := make(chan callbackResult, 1)
	var once sync.Once
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		reject := func() { http.Error(w, "Invalid OAuth callback.", http.StatusBadRequest) }
		host, _, remoteErr := net.SplitHostPort(r.RemoteAddr)
		if r.Method != "GET" || r.URL.Path != "/" || r.Host != listener.Addr().String() || remoteErr != nil || !net.ParseIP(host).IsLoopback() || len(r.URL.RawQuery) > 8192 {
			reject()
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			reject()
			return
		}
		if len(q["iss"]) > 1 || q.Get("iss") != "" && q.Get("iss") != "https://accounts.google.com" {
			reject()
			return
		}
		if len(q["error"]) > 1 || len(q["code"]) > 1 || q.Get("error") != "" && q.Get("code") != "" {
			reject()
			return
		}
		code := q.Get("code")
		denied := q.Get("error") != ""
		if !denied && (code == "" || len(code) > 4096 || strings.ContainsAny(code, "\x00\r\n")) {
			reject()
			return
		}
		accepted := false
		once.Do(func() { accepted = true; received <- callbackResult{code: code, denied: denied} })
		if !accepted {
			reject()
			return
		}
		if denied {
			io.WriteString(w, "Authorization was not completed. Return to the HomeVault terminal.\n")
			return
		}
		io.WriteString(w, "Authorization response received. Return to the HomeVault terminal to confirm completion.\n")
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}
	defer server.Close()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	if showURL == nil || showURL(link) != nil {
		return errors.New("could not display local authorization instructions")
	}
	var response callbackResult
	select {
	case response = <-received:
	case <-ctx.Done():
		return errors.New("OAuth cancelled or timed out; existing credentials were not replaced")
	case <-serveErr:
		return errors.New("local OAuth listener stopped before authorization")
	}
	if response.denied {
		return errors.New("Google authorization was declined; no token saved")
	}
	exchangeCtx := context.WithValue(ctx, oauth2.HTTPClient, client)
	token, err := cfg.Exchange(exchangeCtx, response.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return errors.New("Google token exchange failed; details withheld to protect authorization data")
	}
	if token == nil || token.AccessToken == "" || token.RefreshToken == "" || !strings.EqualFold(token.TokenType, "Bearer") || token.Expiry.IsZero() || !token.Expiry.After(time.Now()) {
		return errors.New("offline authorization returned no usable refresh token; no token saved")
	}
	if value := token.Extra("scope"); value != nil {
		granted, ok := value.(string)
		fields := strings.Fields(granted)
		if !ok || len(fields) != 1 || fields[0] != Scope {
			return errors.New("unexpected OAuth scope response; no token saved")
		}
	}
	current, err := readConnection(root)
	if err != nil {
		return err
	}
	if current.configHash != c.configHash {
		return errors.New("OAuth configuration changed during consent; no token saved")
	}
	c.token = &wireToken{AccessToken: token.AccessToken, TokenType: token.TokenType, RefreshToken: token.RefreshToken, Expiry: token.Expiry.UTC()}
	data, err := configText(c)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(root, ".oauth-pending-")
	if err != nil {
		return errors.New("cannot stage private OAuth config")
	}
	name := f.Name()
	defer os.Remove(name)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("cannot persist OAuth token; no credential values printed")
	}
	if err = ctx.Err(); err != nil {
		return errors.New("OAuth cancelled before token persistence")
	}
	if os.Rename(name, filepath.Join(root, "rclone.conf")) != nil {
		return errors.New("cannot atomically save private OAuth config")
	}
	return nil
}
