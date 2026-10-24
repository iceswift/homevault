package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/lock"
)

func TestBackupJobSuccessFailureAndHistory(t *testing.T) {
	e := fixture(t)
	var calls []string
	e.run = func(_ context.Context, args, _ []string, _ bool) ([]byte, error) {
		if _, err := lock.Acquire(e.cfg.StateDir); !errors.Is(err, lock.ErrLocked) {
			t.Fatal("job phases not locked")
		}
		calls = append(calls, args[4])
		return nil, nil
	}
	if err := e.RunBackupJob(context.Background(), "photos"); err != nil {
		t.Fatal(err)
	}
	s, err := e.JobStatus("photos")
	if err != nil || s.Latest.Status != "success" || !s.Latest.BackupPassed || !s.Latest.CheckPassed || s.LastSuccess.IsZero() || len(s.History) != 1 {
		t.Fatalf("bad status: %+v %v", s, err)
	}
	if strings.Join(calls, ",") != "backup,check" {
		t.Fatal("job did not check", calls)
	}
	previousSuccess := s.LastSuccess
	e.run = func(_ context.Context, args, _ []string, _ bool) ([]byte, error) {
		if args[4] == "check" {
			return nil, errors.New("SECRET credentials")
		}
		return nil, nil
	}
	if err := e.RunBackupJob(context.Background(), "photos"); err == nil {
		t.Fatal("check failure accepted")
	}
	s, err = e.JobStatus("photos")
	if err != nil || s.Latest.Status != "failed" || !s.Latest.BackupPassed || s.Latest.CheckPassed || !s.LastSuccess.Equal(previousSuccess) {
		t.Fatal("failure advanced last success", s, err)
	}
	b, _ := os.ReadFile(e.jobPath("photos"))
	if strings.Contains(string(b), "SECRET") {
		t.Fatal("secret persisted")
	}
	for i := 0; i < 35; i++ {
		if e.RunBackupJob(context.Background(), "photos") == nil {
			t.Fatal("expected failure")
		}
	}
	s, err = e.JobStatus("photos")
	if err != nil || len(s.History) != 32 {
		t.Fatal("history not bounded", err)
	}
	monitor, err := e.Monitor(context.Background(), "photos", 36*time.Hour)
	if err != nil || monitor.Health != "failed" || monitor.Notification != "disabled" {
		t.Fatal(monitor, err)
	}
	e.cfg.Repository = filepath.Join(t.TempDir(), "other-repo")
	if _, err = e.JobStatus("photos"); err == nil {
		t.Fatal("repository identity mismatch accepted")
	}
}
func TestStaleDetectionAndInterruptedJob(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		s    JobStatus
		want string
	}{
		{JobStatus{}, "stale"},
		{JobStatus{LastSuccess: now.Add(-37 * time.Hour), Latest: JobAttempt{Status: "running", Started: now}}, "stale"},
		{JobStatus{LastSuccess: now.Add(-time.Hour), Latest: JobAttempt{Status: "running", Started: now}}, "running"},
		{JobStatus{Latest: JobAttempt{Status: "failed", Started: now}}, "failed"},
		{JobStatus{LastSuccess: now, Latest: JobAttempt{Status: "success", Started: now}}, "healthy"},
	} {
		got, _, _ := evaluateJob(test.s, now, 36*time.Hour)
		if got != test.want {
			t.Fatalf("got %s want %s", got, test.want)
		}
	}
	e := fixture(t)
	os.MkdirAll(filepath.Dir(e.jobPath("photos")), 0700)
	state := JobStatus{Version: 1, Project: "photos", RepositoryID: e.repositoryID(), Latest: JobAttempt{ID: strings.Repeat("a", 24), Status: "running", Phase: "backup", Started: now.Add(-40 * time.Hour)}}
	if err := writeState(e.jobPath("photos"), state); err != nil {
		t.Fatal(err)
	}
	// Simulate stale global operation lock after a crash: monitoring must still
	// read atomic state and report overdue, not silently skip forever.
	held, err := lock.Acquire(e.cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	result, err := e.Monitor(context.Background(), "photos", 36*time.Hour)
	if err != nil || result.Health != "stale" {
		t.Fatal(result, err)
	}
	b, _ := json.Marshal(state)
	os.WriteFile(e.jobPath("photos"), append(b, []byte("garbage")...), 0600)
	if _, err = e.JobStatus("photos"); err == nil {
		t.Fatal("corrupt state accepted")
	}
	state.RepositoryID = ""
	b, _ = json.Marshal(state)
	os.WriteFile(e.jobPath("photos"), b, 0600)
	if _, err = e.JobStatus("photos"); err == nil {
		t.Fatal("missing repository identity accepted")
	}
}

func TestNotificationDedupRecoveryAndDeliveryFailure(t *testing.T) {
	e := fixture(t)
	e.cfg.DiscordWebhookFile = filepath.Join(t.TempDir(), "fake-webhook")
	now := time.Now().UTC()
	state := JobStatus{Version: 1, Project: "photos", RepositoryID: e.repositoryID(), LastSuccess: now.Add(-time.Hour), Latest: JobAttempt{ID: strings.Repeat("a", 24), Started: now.Add(-time.Minute), Finished: now, Status: "failed", Phase: "backup"}}
	if err := writeState(e.jobPath("photos"), state); err != nil {
		t.Fatal(err)
	}
	var sent []string
	fail := false
	e.sendNotification = func(_ context.Context, p, event string) error {
		if p != "photos" {
			t.Fatal("wrong project")
		}
		if fail {
			return errors.New("delivery failed")
		}
		sent = append(sent, event)
		return nil
	}
	for i := 0; i < 2; i++ {
		if _, err := e.Monitor(context.Background(), "photos", 36*time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(sent, ",") != "failed" {
		t.Fatal("duplicate failure alert", sent)
	}
	state.Latest = JobAttempt{ID: strings.Repeat("b", 24), Started: now.Add(-time.Second), Finished: now, Status: "success", Phase: "completed", BackupPassed: true, CheckPassed: true}
	state.LastSuccess = now
	writeState(e.jobPath("photos"), state)
	for i := 0; i < 2; i++ {
		if _, err := e.Monitor(context.Background(), "photos", 36*time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(sent, ",") != "failed,recovered" {
		t.Fatal("wrong recovery events", sent)
	}
	state.LastSuccess = now.Add(-40 * time.Hour)
	state.Latest = JobAttempt{ID: strings.Repeat("c", 24), Started: now, Status: "running", Phase: "backup"}
	writeState(e.jobPath("photos"), state)
	fail = true
	if _, err := e.Monitor(context.Background(), "photos", 36*time.Hour); err == nil {
		t.Fatal("delivery failure swallowed")
	}
	fail = false
	for i := 0; i < 2; i++ {
		if _, err := e.Monitor(context.Background(), "photos", 36*time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(sent, ",") != "failed,recovered,stale" {
		t.Fatal("delivery retry/dedup failed", sent)
	}
}

func TestCaptureRequiredDoesNotAcceptLegacyOrdinarySuccess(t *testing.T) {
	e := fixture(t)
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) { return nil, nil }
	if err := e.RunBackupJob(context.Background(), "photos"); err != nil {
		t.Fatal(err)
	}
	e.cfg.Projects[0].CaptureRequired = true
	e.cfg.Projects[0].Databases = []config.Database{{Name: "main", Type: "sqlite", Source: filepath.Join(t.TempDir(), "db.sqlite")}}
	if e.Backup(context.Background(), "photos") == nil || e.RunBackupJob(context.Background(), "photos") == nil {
		t.Fatal("ordinary backup bypassed capture requirement")
	}
	result, err := e.Monitor(context.Background(), "photos", 36*time.Hour)
	if err != nil || result.Health != "failed" {
		t.Fatal("legacy ordinary success treated as healthy capture", result, err)
	}
}
