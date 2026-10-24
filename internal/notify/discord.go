// Package notify sends minimal, credential-redacted backup events to Discord.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var webhookPath = regexp.MustCompile(`^/api/webhooks/[0-9]+/[a-zA-Z0-9._-]+$`)

type Discord struct{ client *http.Client }

func New() *Discord {
	return &Discord{client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (d *Discord) Send(ctx context.Context, file, project, event string) error {
	if !filepath.IsAbs(file) {
		return errors.New("Discord webhook must be read from an absolute private file")
	}
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 4096 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("Discord webhook file must be a private regular file (0600)")
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return errors.New("cannot read Discord webhook file")
	}
	u, err := url.Parse(strings.TrimSpace(string(b)))
	if err != nil || u.Scheme != "https" || u.Host != "discord.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !webhookPath.MatchString(u.Path) {
		return errors.New("invalid Discord webhook endpoint; only official HTTPS webhooks are supported")
	}
	allowed := map[string]string{"failed": "backup/check failed", "stale": "backup is overdue or has never completed", "recovered": "backup health recovered", "success": "backup and metadata check completed", "test": "notification test (no backup was performed)"}
	description, ok := allowed[event]
	if !ok {
		return errors.New("unknown notification event")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`).MatchString(project) {
		return errors.New("invalid notification project")
	}
	payload, _ := json.Marshal(map[string]any{"content": "HomeVault · " + project + " · " + description, "allowed_mentions": map[string]any{"parse": []string{}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return errors.New("cannot prepare Discord request")
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(req)
	if err != nil {
		return errors.New("Discord delivery failed; endpoint and child error withheld")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("Discord delivery rejected; response withheld")
	}
	return nil
}
