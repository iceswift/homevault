package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/engine"
)

// Destructive work is confined to newly generated local repositories.
func TestRealReviewedUnusedPackPrune(t *testing.T) {
	binary := os.Getenv("HOMEVAULT_TEST_RESTIC")
	if binary == "" {
		t.Skip("explicit pinned Restic required")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	otherSource := filepath.Join(root, "other")
	for _, p := range []string{source, otherSource} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(root, "key")
	if err := os.WriteFile(key, []byte("generated-prune-fixture-only"), 0600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: repo, PasswordFile: key, ResticBinary: binary, Projects: []config.Project{{Name: "primary", Paths: []string{source}}, {Name: "other", Paths: []string{otherSource}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	old := make([]byte, 2*1024*1024)
	current := make([]byte, len(old))
	if _, err = rand.Read(old); err != nil {
		t.Fatal(err)
	}
	if _, err = rand.Read(current); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "proof.bin")
	if err = os.WriteFile(file, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "primary"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, current, 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "primary"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(otherSource, "proof.bin"), []byte("other-project-survives"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	retention, err := e.ReviewRetention(ctx, "primary", engine.RetentionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(retention.Plan.Candidates) != 1 {
		t.Fatal("expected one obsolete synthetic snapshot")
	}
	preserve := retention.Plan.Keep[0].ID
	if _, err = e.ApplyRetention(ctx, "primary", retention, preserve); err != nil {
		t.Fatal(err)
	}
	packBytes := func() int64 {
		var total int64
		if err := filepath.WalkDir(filepath.Join(repo, "data"), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				info, err := d.Info()
				if err != nil {
					return err
				}
				total += info.Size()
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return total
	}
	before := packBytes()
	review, err := e.ReviewPrune(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if review.Scope != "repository" || len(review.SnapshotIDs) != 2 || review.Estimate.TotalRemove.Blobs == 0 || review.Estimate.Repack.Blobs != 0 {
		t.Fatal("bad real prune estimate", review.Estimate)
	}
	if packBytes() != before {
		t.Fatal("dry-run changed data packs")
	}
	// Load from a private review file, as the manual CLI workflow does.
	plan := filepath.Join(root, "prune.json")
	data, _ := json.Marshal(review)
	if err = os.WriteFile(plan, data, 0600); err != nil {
		t.Fatal(err)
	}
	review, err = engine.LoadPruneReview(plan)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ApplyPrune(ctx, review, preserve)
	if err != nil || !result.Verified || result.Status != "success" {
		t.Fatal("real prune failed", result, err)
	}
	if after := packBytes(); after >= before {
		t.Fatalf("unused packs not reclaimed: %d -> %d", before, after)
	}
	for _, project := range []string{"primary", "other"} {
		snapshots, err := e.Snapshots(ctx, project)
		if err != nil || len(snapshots) != 1 {
			t.Fatal("retained snapshot lost", project, err)
		}
		target := filepath.Join(root, "restore-"+project)
		if err = e.Restore(ctx, project, snapshots[0].ID, target); err != nil {
			t.Fatal(err)
		}
		var actual []byte
		if err = filepath.WalkDir(target, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Name() == "proof.bin" {
				actual, err = os.ReadFile(p)
				return err
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		expected := current
		if project == "other" {
			expected = []byte("other-project-survives")
		}
		if !bytes.Equal(actual, expected) {
			t.Fatal("post-prune restore mismatch", project)
		}
	}
	if _, err = e.ApplyPrune(ctx, review, preserve); err == nil {
		t.Fatal("old pack/index inventory reused")
	}
	if _, err = e.ReviewPrune(ctx); err != nil {
		t.Fatal("post-cleanup no-op preview failed", err)
	}
	t.Logf("PASS generated local unused packs reclaimed; both retained projects restored exactly; data bytes before=%d after=%d", before, packBytes())
}
