package integration

import (
	"context"
	"homevault/internal/config"
	"homevault/internal/engine"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRealBackupJobLifecycle(t *testing.T) {
	restic := os.Getenv("HOMEVAULT_TEST_RESTIC")
	if restic == "" {
		t.Skip("real Restic required")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0700)
	os.WriteFile(filepath.Join(source, "proof"), []byte("synthetic job proof"), 0600)
	key := filepath.Join(root, "password")
	os.WriteFile(key, []byte("fixture-only"), 0600)
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repo"), PasswordFile: key, ResticBinary: restic, Projects: []config.Project{{Name: "fixture", Paths: []string{source}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.RunBackupJob(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	before, err := e.JobStatus("fixture")
	if err != nil || before.LastSuccess.IsZero() || !before.Latest.CheckPassed {
		t.Fatal("successful job not persisted", err)
	}
	if err = os.Remove(filepath.Join(source, "proof")); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err = e.RunBackupJob(ctx, "fixture"); err == nil {
		t.Fatal("missing source not reported")
	}
	after, err := e.JobStatus("fixture")
	if err != nil || after.Latest.Status != "failed" || !before.LastSuccess.Equal(after.LastSuccess) {
		t.Fatal("failure lost previous success", err)
	}
	monitor, err := e.Monitor(ctx, "fixture", 36*time.Hour)
	if err != nil || monitor.Health != "failed" {
		t.Fatal(monitor, err)
	}
	t.Log("PASS real encrypted job backup/check, durable status, failure preservation and monitoring")
}
