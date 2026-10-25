package engine

import (
	"context"
	"errors"
	"os"
	"testing"

	"homevault/internal/config"
)

func TestReviewedJobRefusalDoesNotRunOrNotify(t *testing.T) {
	e := fixture(t)
	calls, notices := 0, 0
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) { calls++; return nil, nil }
	e.sendNotification = func(context.Context, string, string) error { notices++; return nil }
	if err := e.RunReviewedFileJob(context.Background(), "photos", "changed", nil); err == nil {
		t.Fatal("stale job review accepted")
	}
	if calls != 0 || notices != 0 {
		t.Fatal("refused job ran or notified")
	}
	if _, err := os.Stat(e.jobPath("photos")); !os.IsNotExist(err) {
		t.Fatal("refused job created attempt")
	}
	checks := 0
	if err := e.RunReviewedFileJob(context.Background(), "photos", "", func() error {
		checks++
		if checks == 2 {
			return errors.New("changed config")
		}
		return nil
	}); err == nil {
		t.Fatal("under-lock config change ignored")
	}
	if checks != 2 || calls != 0 || notices != 0 {
		t.Fatal("preconditions not enforced before writes/alerts")
	}
	e.cfg.Projects[0].Databases = []config.Database{{Type: "sqlite"}}
	if err := e.RunReviewedFileJob(context.Background(), "photos", "", nil); err == nil {
		t.Fatal("database accepted")
	}
	e.cfg.Projects[0].Databases = nil
	e.cfg.Projects[0].CaptureRequired = true
	if err := e.RunReviewedFileJob(context.Background(), "photos", "", nil); err == nil {
		t.Fatal("capture project accepted")
	}
}
