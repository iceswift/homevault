package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/engine"
)

func TestRealResticRoundTrip(t *testing.T) {
	binary := os.Getenv("HOMEVAULT_TEST_RESTIC")
	if binary == "" {
		t.Skip("set HOMEVAULT_TEST_RESTIC to run real encrypted backup/restore")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	state := filepath.Join(root, "state")
	repo := filepath.Join(root, "repository")
	key := filepath.Join(root, "password")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("HomeVault synthetic proof\nภาษาไทย\n")
	if err := os.WriteFile(filepath.Join(source, "proof.txt"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("synthetic-test-password-not-a-real-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Version: 1, StateDir: state, Repository: repo, PasswordFile: key, ResticBinary: binary, Projects: []config.Project{{Name: "synthetic", Paths: []string{source}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "synthetic"); err != nil {
		t.Fatal(err)
	}
	snaps, err := e.Snapshots(ctx, "synthetic")
	if err != nil || len(snaps) != 1 {
		t.Fatalf("snapshots=%d err=%v", len(snaps), err)
	}
	if err = e.Check(ctx); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "restore")
	if err = e.Restore(ctx, "synthetic", snaps[0].ID, target); err != nil {
		t.Fatal(err)
	}
	found := false
	err = filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "proof.txt" {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if string(b) != string(payload) {
				t.Errorf("restored content mismatch")
			}
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("restored proof missing")
	}
	if err = e.Restore(ctx, "synthetic", snaps[0].ID, target); err == nil {
		t.Fatal("restore overwrote nonempty destination")
	}
	// Change a file and create a second snapshot: the first snapshot must remain available.
	if err = os.WriteFile(filepath.Join(source, "proof.txt"), []byte("second version"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "synthetic"); err != nil {
		t.Fatal(err)
	}
	snaps, err = e.Snapshots(ctx, "synthetic")
	if err != nil || len(snaps) != 2 {
		t.Fatalf("version history lost: %d %v", len(snaps), err)
	}
	// A disaster recovery must work even when the original source no longer exists.
	if err = os.Rename(source, filepath.Join(root, "source-lost")); err != nil {
		t.Fatal(err)
	}
	e, err = engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err = e.Snapshots(ctx, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Restore(ctx, "synthetic", snaps[0].ID, filepath.Join(root, "disaster-restore")); err != nil {
		t.Fatal(err)
	}
}
