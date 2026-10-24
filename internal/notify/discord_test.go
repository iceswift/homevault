package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDiscordRedactionAndMinimalPayload(t *testing.T) {
	file := filepath.Join(t.TempDir(), "webhook")
	// Public fake fixture, never a live webhook credential.
	os.WriteFile(file, []byte("https://discord.com/api/webhooks/123/FAKE_TEST_TOKEN"), 0600)
	d := New()
	calls := 0
	d.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Host != "discord.com" {
			t.Fatal("wrong destination")
		}
		b, _ := io.ReadAll(r.Body)
		var p struct {
			Content  string `json:"content"`
			Mentions struct {
				Parse []string `json:"parse"`
			} `json:"allowed_mentions"`
		}
		if json.Unmarshal(b, &p) != nil || strings.Contains(string(b), "FAKE_TEST_TOKEN") || len(p.Mentions.Parse) != 0 || !strings.Contains(p.Content, "backup/check failed") {
			t.Fatal("unsafe payload")
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})
	if err := d.Send(context.Background(), file, "demo", "failed"); err != nil || calls != 1 {
		t.Fatal(err)
	}
	d.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("SECRET endpoint token") })
	if err := d.Send(context.Background(), file, "demo", "failed"); err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "FAKE_TEST_TOKEN") {
		t.Fatal("error leaked secret", err)
	}
	for _, u := range []string{"http://discord.com/api/webhooks/123/FAKE", "https://example.com/api/webhooks/123/FAKE", "https://discord.com.evil.invalid/api/webhooks/123/FAKE", "https://discord.com:444/api/webhooks/123/FAKE", "https://discord.com/api/webhooks/123/FAKE?x=y"} {
		os.WriteFile(file, []byte(u), 0600)
		if d.Send(context.Background(), file, "demo", "failed") == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
}
