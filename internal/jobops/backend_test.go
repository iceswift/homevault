package jobops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/engine"
	"homevault/internal/statusdata"
)

type fakeEngine struct {
	mu      sync.Mutex
	state   engine.JobStatus
	gate    chan struct{}
	calls   int
	warning bool
}

func (f *fakeEngine) JobStatus(string) (engine.JobStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}
func (f *fakeEngine) RunReviewedFileJob(ctx context.Context, _ string, expected string, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	f.mu.Lock()
	if f.state.Latest.ID != expected {
		f.mu.Unlock()
		return errors.New("changed")
	}
	f.calls++
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		f.mu.Lock()
		f.state.Latest = engine.JobAttempt{ID: strings.Repeat("b", 24), Status: "failed", Phase: "cancelled"}
		f.mu.Unlock()
		return ctx.Err()
	case <-f.gate:
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	f.state.Latest = engine.JobAttempt{ID: strings.Repeat("c", 24), Status: "success", Phase: "completed", Finished: now, BackupPassed: true, CheckPassed: true}
	f.state.LastSuccess = now
	if f.warning {
		return errors.New("PRIVATE notification detail")
	}
	return nil
}
func backendFixture(t *testing.T) (*Backend, *fakeEngine) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0700)
	key := filepath.Join(root, "key")
	os.WriteFile(key, []byte("generated-test-key"), 0600)
	exe, _ := os.Executable()
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repository"), PasswordFile: key, ResticBinary: exe, Projects: []config.Project{{Name: "demo", Paths: []string{source}}}}
	path := filepath.Join(root, "config.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(path, data, 0600)
	b, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeEngine{state: engine.JobStatus{Project: "demo", Latest: engine.JobAttempt{Status: "never-run"}}, gate: make(chan struct{})}
	b.engine = f
	return b, f
}
func final(events <-chan statusdata.JobEvent) statusdata.JobEvent {
	var last statusdata.JobEvent
	for event := range events {
		last = event
	}
	return last
}
func TestReviewDoesNotWriteAndTokensAreSingleUse(t *testing.T) {
	b, f := backendFixture(t)
	r, err := b.Review("demo")
	if err != nil || !r.Allowed {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.cfg.StateDir); !os.IsNotExist(err) {
		t.Fatal("review created state")
	}
	if f.calls != 0 {
		t.Fatal("review started job")
	}
	b.Discard()
	if _, err := b.Start(r.Token); err == nil {
		t.Fatal("cancelled review accepted")
	}
	r, _ = b.Review("demo")
	events, err := b.Start(r.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Start(r.Token); err == nil {
		t.Fatal("double start accepted")
	}
	close(f.gate)
	event := final(events)
	b.Wait()
	if event.State != "passed" {
		t.Fatal(event.State)
	}
	if _, err := b.Start(r.Token); err == nil {
		t.Fatal("token reused")
	}
}
func TestChangedConfigAndBlockedProjectsCannotStart(t *testing.T) {
	b, f := backendFixture(t)
	r, _ := b.Review("demo")
	file, _ := os.OpenFile(b.path, os.O_APPEND|os.O_WRONLY, 0)
	file.WriteString("\n")
	file.Close()
	if _, err := b.Start(r.Token); err == nil || f.calls != 0 {
		t.Fatal("changed config accepted")
	}
	b, _ = backendFixture(t)
	b.cfg.Projects[0].CaptureRequired = true
	r, err := b.Review("demo")
	if err != nil || r.Allowed || !strings.Contains(r.Reason, "Capture-required") {
		t.Fatal("capture not blocked")
	}
}
func TestCancelWaitAndCompletionWarning(t *testing.T) {
	b, f := backendFixture(t)
	r, _ := b.Review("demo")
	events, err := b.Start(r.Token)
	if err != nil {
		t.Fatal(err)
	}
	b.Cancel()
	event := final(events)
	b.Wait()
	if event.State != "cancelled" {
		t.Fatal(event.State)
	}
	b, f = backendFixture(t)
	f.warning = true
	r, _ = b.Review("demo")
	events, _ = b.Start(r.Token)
	close(f.gate)
	event = final(events)
	b.Wait()
	if event.State != "warning" || strings.Contains(event.Message, "PRIVATE") {
		t.Fatal("warning erased verification or exposed detail")
	}
}

func TestLoadHistoryIsNewestFirstDeduplicatedAndReadOnly(t *testing.T) {
	b, f := backendFixture(t)
	now := time.Now().UTC()
	old := engine.JobAttempt{ID: strings.Repeat("a", 24), Status: "success", Phase: "completed", Started: now.Add(-time.Hour), Finished: now.Add(-59 * time.Minute), BackupPassed: true, CheckPassed: true}
	failed := engine.JobAttempt{ID: strings.Repeat("b", 24), Status: "failed", Phase: "check", Started: now.Add(-time.Minute), Finished: now, BackupPassed: true}
	f.state.History = []engine.JobAttempt{old, failed}
	f.state.Latest = failed
	f.state.LastSuccess = old.Finished
	data, err := b.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := data.Projects[0]
	if !p.HistoryKnown || len(p.History) != 2 || p.History[0].ID != failed.ID || p.History[1].ID != old.ID || !p.Uploaded.Equal(old.Finished) {
		t.Fatal("history ordering, deduplication or prior success lost")
	}
	if p.History[0].CheckPassed || !p.History[0].BackupPassed || p.State != "failed" {
		t.Fatal("failed check mislabeled")
	}
	f.state.Latest = engine.JobAttempt{ID: strings.Repeat("c", 24), Status: "running", Phase: "backup", Started: now}
	data, err = b.Load()
	if err != nil || len(data.Projects[0].History) != 3 || data.Projects[0].History[0].Status != "running" {
		t.Fatal("running attempt lost")
	}
	if _, err := os.Stat(b.cfg.StateDir); !os.IsNotExist(err) || f.calls != 0 {
		t.Fatal("history read created state or ran job")
	}
}
