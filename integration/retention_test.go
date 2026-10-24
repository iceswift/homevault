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

// Only generated local repositories are used. No cloud/production deletion.
func TestRealRetentionReviewedForget(t *testing.T) {
	binary := os.Getenv("HOMEVAULT_TEST_RESTIC")
	if binary == "" {
		t.Skip("explicit Restic binary required")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0700)
	key := filepath.Join(root, "key")
	os.WriteFile(key, []byte("synthetic-retention-test-only"), 0600)
	repo := filepath.Join(root, "repo")
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: repo, PasswordFile: key, ResticBinary: binary, Projects: []config.Project{{Name: "primary", Paths: []string{source}}, {Name: "other", Paths: []string{source}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "proof.txt")
	os.WriteFile(file, []byte("version-one"), 0600)
	if err = e.Backup(ctx, "primary"); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	// Different content prevents Restic skipping an unchanged snapshot.
	os.WriteFile(file, []byte("version-two"), 0600)
	if err = e.Backup(ctx, "primary"); err != nil {
		t.Fatal(err)
	}
	review, err := e.ReviewRetention(ctx, "primary", engine.RetentionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Plan.Keep) != 1 || len(review.Plan.Candidates) != 1 {
		t.Fatal("unexpected synthetic policy", review.Plan)
	}
	dataFiles := func() map[string]bool {
		files := map[string]bool{}
		if err := filepath.WalkDir(filepath.Join(repo, "data"), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				files[p] = true
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := dataFiles()
	result, err := e.ApplyRetention(ctx, "primary", review, review.Plan.Keep[0].ID)
	if err != nil || !result.Verified {
		t.Fatal("synthetic apply failed", result, err)
	}
	other, err := e.Snapshots(ctx, "other")
	if err != nil || len(other) != 1 {
		t.Fatal("unrelated project altered", other, err)
	}
	after := dataFiles()
	if len(before) != len(after) {
		t.Fatal("data packs changed despite no prune")
	}
	for p := range before {
		if !after[p] {
			t.Fatal("data pack removed")
		}
	}
	target := filepath.Join(root, "restored")
	if err = e.Restore(ctx, "primary", review.Plan.Keep[0].ID, target); err != nil {
		t.Fatal(err)
	}
	var restored []byte
	filepath.WalkDir(target, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "proof.txt" {
			restored, _ = os.ReadFile(p)
		}
		return nil
	})
	if string(restored) != "version-two" {
		t.Fatal("retained snapshot not recoverable")
	}
	if _, err = e.ApplyRetention(ctx, "primary", review, review.Plan.Keep[0].ID); err == nil {
		t.Fatal("stale review reused")
	}
}
