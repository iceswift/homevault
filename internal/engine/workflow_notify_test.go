package engine

import (
	"context"
	"testing"
)

func TestWorkflowEventsRejectRecoveryAndArbitraryContent(t *testing.T) {
	e := fixture(t)
	sent := 0
	e.sendNotification = func(_ context.Context, project, event string) error {
		if project != "photos" || event != "failed" {
			t.Fatal("unexpected notification scope")
		}
		sent++
		return nil
	}
	for _, event := range []string{"test", "success", "failed\nsecret", "arbitrary content"} {
		if err := e.NotifyEvent(context.Background(), "photos", event); err == nil {
			t.Fatal("arbitrary workflow content accepted")
		}
	}
	if sent != 0 {
		t.Fatal("invalid event reached notifier")
	}
	if err := e.NotifyEvent(context.Background(), "photos", "failed"); err != nil || sent != 1 {
		t.Fatal("approved event not delivered", err)
	}
	status, err := e.JobStatus("photos")
	if err != nil || !status.LastSuccess.IsZero() || status.Latest.Status != "never-run" {
		t.Fatal("notification manufactured job freshness", err)
	}
	e.cfg.RecoveryOnly = true
	if err := e.NotifyEvent(context.Background(), "photos", "failed"); err == nil {
		t.Fatal("recovery configuration allowed notification")
	}
}
