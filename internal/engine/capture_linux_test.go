//go:build linux

package engine

import (
	"context"
	"errors"
	"homevault/internal/config"
	"homevault/internal/lock"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeCaptureDocker struct {
	state                 captureIdentity
	stopHook              func()
	stopCount, startCount int
	startErr              error
}

func (f *fakeCaptureDocker) inspect(context.Context, string) (captureIdentity, error) {
	return f.state, nil
}
func (f *fakeCaptureDocker) stop(context.Context, string) error {
	f.stopCount++
	f.state.Running = false
	if f.stopHook != nil {
		f.stopHook()
	}
	return nil
}
func (f *fakeCaptureDocker) start(ctx context.Context, _ string) error {
	if ctx.Err() != nil {
		return errors.New("resume used cancelled context")
	}
	f.startCount++
	if f.startErr != nil {
		return f.startErr
	}
	f.state.Running = true
	f.state.Health = "healthy"
	return nil
}
func captureFixture(t *testing.T) (*Engine, CapturePlan, *fakeCaptureDocker) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 required")
	}
	e := fixture(t)
	root := filepath.Join(filepath.Dir(e.cfg.StateDir), "live")
	os.Mkdir(root, 0700)
	media := filepath.Join(root, "media")
	os.Mkdir(media, 0700)
	os.WriteFile(filepath.Join(media, "proof"), []byte("current-media"), 0600)
	db := filepath.Join(root, "main.sqlite")
	if exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute('create table proof(v text)');c.execute("insert into proof values ('current')");c.commit()`, db).Run() != nil {
		t.Fatal("DB fixture failed")
	}
	p := testCapturePlan(t)
	p.DataRoot = root
	p.WindowStart = "00:00"
	p.WindowMinutes = 1440
	var identity captureIdentity
	identity.ID = p.ContainerID
	identity.Image = p.ImageID
	identity.Running = true
	identity.Health = "healthy"
	identity.Started = "unchanged"
	identity.Mounts = append(identity.Mounts, struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	}{"bind", root, "/data"})
	e.cfg.Projects[0].Paths = []string{media}
	e.cfg.Projects[0].Databases = []config.Database{{Name: "main", Type: "sqlite", Source: db}}
	e.cfg.Projects[0].CaptureRequired = true
	e.armPauseGuard = func(string, string) error { return nil } // No real background process/Docker.
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) { return nil, nil }
	return e, p, &fakeCaptureDocker{state: identity}
}
func TestFreshCaptureJobLockAndFailureResume(t *testing.T) {
	e, p, d := captureFixture(t)
	prepare := func(ctx context.Context, project config.Project) (config.Project, func() error, error) {
		if _, err := lock.Acquire(e.cfg.StateDir); !errors.Is(err, lock.ErrLocked) {
			t.Fatal("capture not locked")
		}
		return e.captureProject(ctx, project, p, d)
	}
	if err := e.runBackupJob(context.Background(), "photos", prepare); err != nil {
		t.Fatal(err)
	}
	s, err := e.JobStatus("photos")
	if err != nil || !s.Latest.CapturePassed || s.Latest.Status != "success" || d.stopCount != 1 || d.startCount != 1 {
		t.Fatal(s, err)
	}
	captures, _ := filepath.Glob(filepath.Join(e.cfg.StateDir, "captured-*"))
	if len(captures) != 0 {
		t.Fatal("successful plaintext capture not cleaned")
	}
	old := s.LastSuccess
	d.stopHook = func() { os.Remove(e.cfg.Projects[0].Databases[0].Source) }
	if e.runBackupJob(context.Background(), "photos", prepare) == nil {
		t.Fatal("failed capture accepted")
	}
	s, _ = e.JobStatus("photos")
	if s.Latest.Status != "failed" || s.Latest.Phase != "capture" || s.Latest.CapturePassed || !s.LastSuccess.Equal(old) || !d.state.Running {
		t.Fatal("failure/resume status incorrect", s)
	}
	if _, err := os.Stat(filepath.Join(e.cfg.StateDir, "guards", "photos.json")); !os.IsNotExist(err) {
		t.Fatal("successful resume left active guard")
	}
}
func TestCaptureRejectsFrozenSourceAndResumesOnCancellation(t *testing.T) {
	e, p, d := captureFixture(t)
	original := e.cfg.Projects[0]
	frozen := original
	frozen.Paths = []string{filepath.Join(t.TempDir(), "old-copy")}
	if p.verifySource(frozen, d.state) == nil {
		t.Fatal("old frozen path accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.stopHook = cancel
	if e.runBackupJob(ctx, "photos", func(ctx context.Context, project config.Project) (config.Project, func() error, error) {
		return e.captureProject(ctx, project, p, d)
	}) == nil {
		t.Fatal("cancelled capture passed")
	}
	if !d.state.Running || d.startCount != 1 {
		t.Fatal("cancelled job did not independently resume app")
	}
}
func TestGuardRecoveryWithStaleLock(t *testing.T) {
	e, p, d := captureFixture(t)
	file := filepath.Join(e.cfg.StateDir, "guards", "photos.json")
	guard := pauseGuard{Version: 1, ContainerID: p.ContainerID, ImageID: p.ImageID, Nonce: strings.Repeat("c", 24), Deadline: time.Now().Add(-time.Second)}
	if err := writeState(file, guard); err != nil {
		t.Fatal(err)
	}
	held, err := lock.Acquire(e.cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	d.state.Running = false
	if err := recoverPaused(context.Background(), file, false, d); err != nil {
		t.Fatal(err)
	}
	if !d.state.Running || d.startCount != 1 {
		t.Fatal("guard did not resume")
	}
	if err := writeState(file, guard); err != nil {
		t.Fatal(err)
	}
	d.state.Image = "sha256:" + strings.Repeat("d", 64)
	if recoverPaused(context.Background(), file, true, d) == nil {
		t.Fatal("different image resumed")
	}
	if d.startCount != 1 {
		t.Fatal("unsafe restart attempted")
	}
	d.state.Image = p.ImageID
	guard.Deadline = time.Now().Add(10 * time.Second)
	guard.OwnerPID = os.Getpid()
	writeState(file, guard)
	if recoverPaused(context.Background(), file, true, d) == nil || d.startCount != 1 {
		t.Fatal("forced resume interrupted a live capture owner")
	}
}

func TestPauseGuardianDoesNotFollowNextJob(t *testing.T) {
	e, p, _ := captureFixture(t)
	file := filepath.Join(e.cfg.StateDir, "guards", "photos.json")
	guard := pauseGuard{Version: 1, ContainerID: p.ContainerID, ImageID: p.ImageID, Nonce: strings.Repeat("a", 24), Deadline: time.Now().Add(10 * time.Second), OwnerPID: os.Getpid()}
	if err := writeState(file, guard); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- PauseGuardian(ctx, file) }()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(file + ".ready"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("guardian not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	guard.Nonce = strings.Repeat("b", 24)
	if err := writeState(file, guard); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal("guardian followed another job", err)
	}
}
