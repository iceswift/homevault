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

	"homevault/internal/lock"
)

const pruneFixtureSummary = `loading all snapshots...
Would have made the following changes:

to repack:             0 blobs / 0 B
this removes:          0 blobs / 0 B
to delete:             5 blobs / 1.001 MiB
total prune:           5 blobs / 1.001 MiB
remaining:             9 blobs / 2.002 MiB
unused size after prune: 0 B (0.00% of remaining size)
`

type pruneFixtureState struct {
	snapshots []Snapshot
	packs     string
	index     string
	summary   string
	version   string
	prunes    int
	checks    int
	checkFail bool
	pruneFail bool
}

func pruneFixture(t *testing.T) (*Engine, *pruneFixtureState) {
	t.Helper()
	e := fixture(t)
	f := &pruneFixtureState{packs: strings.Repeat("d", 64) + "\n" + strings.Repeat("e", 64) + "\n", index: strings.Repeat("f", 64) + "\n", summary: pruneFixtureSummary, version: "restic 0.19.1 compiled with go1.24.5 on linux/amd64"}
	for i, tag := range []string{"homevault:photos", "homevault:other", ""} {
		s := Snapshot{ID: strings.Repeat(string(rune('a'+i)), 64), Time: time.Now().UTC().Add(-time.Duration(i+1) * time.Hour)}
		if tag != "" {
			s.Tags = []string{tag}
		}
		f.snapshots = append(f.snapshots, s)
	}
	e.run = func(_ context.Context, args, _ []string, capture bool) ([]byte, error) {
		if _, err := lock.Acquire(e.cfg.StateDir); !errors.Is(err, lock.ErrLocked) {
			t.Fatal("prune phase not locked")
		}
		switch args[4] {
		case "version":
			return []byte(f.version), nil
		case "snapshots":
			if strings.Join(args[4:], " ") != "snapshots --json" {
				t.Fatal("prune narrowed snapshot scope", args)
			}
			return json.Marshal(f.snapshots)
		case "list":
			if args[5] == "packs" {
				return []byte(f.packs), nil
			}
			if args[5] == "index" {
				return []byte(f.index), nil
			}
			t.Fatal("unexpected inventory kind")
		case "check":
			f.checks++
			if f.checkFail {
				return nil, errors.New("withheld failure")
			}
			return nil, nil
		case "prune":
			if strings.Join(args[4:], " ") == "prune --dry-run --max-repack-size=0 --max-unused=unlimited" {
				if !capture {
					t.Fatal("preview not captured")
				}
				return []byte(f.summary), nil
			}
			if strings.Join(args[4:], " ") != "prune --max-repack-size=0 --max-unused=unlimited" || capture {
				t.Fatal("unsafe prune invocation", args)
			}
			f.prunes++
			if f.pruneFail {
				return nil, errors.New("withheld prune failure")
			}
			f.packs = strings.Repeat("e", 64) + "\n"
			return nil, nil
		default:
			t.Fatal("unexpected command", args[4])
		}
		return nil, errors.New("unexpected fixture command")
	}
	return e, f
}

func TestPruneReviewApplyAndAllSnapshotScope(t *testing.T) {
	e, f := pruneFixture(t)
	review, err := e.ReviewPrune(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.prunes != 0 || review.Scope != "repository" || len(review.SnapshotIDs) != 3 || review.Estimate.TotalRemove.Size != "1.001 MiB" {
		t.Fatal("bad dry-run scope or estimate", review)
	}
	file := filepath.Join(t.TempDir(), "review.json")
	data, _ := json.Marshal(review)
	os.WriteFile(file, data, 0600)
	review, err = LoadPruneReview(file)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ApplyPrune(context.Background(), review, f.snapshots[0].ID)
	if err != nil || !result.Verified || result.Status != "success" || f.prunes != 1 || len(f.snapshots) != 3 {
		t.Fatal("apply failed", result, err)
	}
	var audit PruneResult
	if err := readState(filepath.Join(e.cfg.StateDir, "prune", "latest.json"), &audit); err != nil || !audit.Verified || audit.SnapshotCount != 3 {
		t.Fatal("missing audit", audit, err)
	}
	job, err := e.JobStatus("photos")
	if err != nil || !job.LastSuccess.IsZero() {
		t.Fatal("prune advanced backup freshness")
	}
	if _, err := e.ApplyPrune(context.Background(), review, f.snapshots[0].ID); err == nil {
		t.Fatal("stale storage review reused")
	}
}

func TestPruneRefusesUnsafeOrChangedReview(t *testing.T) {
	for _, which := range []string{"expired", "future", "repo", "scope", "mode", "version", "snapshots", "storage", "estimate", "preserve", "check", "late-storage", "late-snapshot"} {
		t.Run(which, func(t *testing.T) {
			e, f := pruneFixture(t)
			review, err := e.ReviewPrune(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			preserve := f.snapshots[0].ID
			switch which {
			case "expired":
				review.Created = time.Now().Add(-25 * time.Hour)
			case "future":
				review.Created = time.Now().Add(time.Hour)
			case "repo":
				review.RepositoryID = "wrong"
			case "scope":
				review.Scope = "project"
			case "mode":
				review.Mode = "repack-everything"
			case "version":
				f.version = "restic 0.20.0"
			case "snapshots":
				f.snapshots = f.snapshots[:2]
			case "storage":
				f.packs += strings.Repeat("1", 64) + "\n"
			case "estimate":
				review.Estimate.TotalRemove.Size = "2 MiB"
			case "preserve":
				preserve = strings.Repeat("1", 64)
			case "check":
				f.checkFail = true
			case "late-storage", "late-snapshot":
				original := e.run
				e.run = func(ctx context.Context, args, env []string, capture bool) ([]byte, error) {
					if args[4] == "check" {
						if which == "late-storage" {
							f.index = strings.Repeat("2", 64)
						} else {
							f.snapshots[0].Hostname = "changed"
						}
					}
					return original(ctx, args, env, capture)
				}
			}
			if _, err := e.ApplyPrune(context.Background(), review, preserve); err == nil {
				t.Fatal("unsafe prune accepted")
			}
			if f.prunes != 0 {
				t.Fatal("destructive prune executed despite refusal")
			}
		})
	}
}

func TestPruneParserRejectsUnknownOrRepackingOutput(t *testing.T) {
	for _, text := range []string{"", strings.Replace(pruneFixtureSummary, "to delete:", "renamed:", 1), pruneFixtureSummary + "to delete: 5 blobs / 1 MiB\n", strings.Replace(pruneFixtureSummary, "0 blobs / 0 B", "1 blobs / 1 KiB", 1)} {
		if _, err := parsePruneEstimate([]byte(text)); err == nil {
			t.Fatal("unsupported output accepted")
		}
	}
	e, f := pruneFixture(t)
	f.snapshots = nil
	if _, err := e.ReviewPrune(context.Background()); err == nil || f.prunes != 0 {
		t.Fatal("empty repository authorized")
	}
	f.snapshots = []Snapshot{{ID: "bad", Time: time.Now()}}
	if _, err := e.ReviewPrune(context.Background()); err == nil {
		t.Fatal("invalid snapshot authorized")
	}
}

func TestPruneFailureDoesNotClaimVerification(t *testing.T) {
	e, f := pruneFixture(t)
	review, err := e.ReviewPrune(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.pruneFail = true
	result, err := e.ApplyPrune(context.Background(), review, f.snapshots[0].ID)
	if err == nil || result.Verified || result.Status != "failed" {
		t.Fatal("prune failure reported as success", result, err)
	}
	var audit PruneResult
	if err := readState(filepath.Join(e.cfg.StateDir, "prune", "latest.json"), &audit); err != nil || audit.Status != "failed" || audit.Verified {
		t.Fatal("failed audit not persisted", audit, err)
	}
}

func TestPruneCancellationAndPostCheckFailure(t *testing.T) {
	for _, which := range []string{"cancel", "post-check", "post-snapshot"} {
		t.Run(which, func(t *testing.T) {
			e, f := pruneFixture(t)
			review, err := e.ReviewPrune(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			original := e.run
			e.run = func(ctx context.Context, args, env []string, capture bool) ([]byte, error) {
				if which == "post-check" && args[4] == "check" && f.prunes > 0 {
					return nil, errors.New("post-check failed")
				}
				data, err := original(ctx, args, env, capture)
				if args[4] == "prune" && capture && which == "cancel" {
					cancel()
				}
				if args[4] == "prune" && !capture && which == "post-snapshot" {
					f.snapshots[0].Hostname = "changed"
				}
				return data, err
			}
			result, err := e.ApplyPrune(ctx, review, f.snapshots[0].ID)
			if err == nil || result.Verified || result.Status != "failed" {
				t.Fatal("failed verification reported success", result, err)
			}
			if which == "cancel" && f.prunes != 0 {
				t.Fatal("destructive prune started after cancellation")
			}
		})
	}
}

func TestLoadPruneReviewRejectsUnknownFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(file, []byte(`{"version":1,"transport_secret":"DO_NOT_REPEAT_THIS"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPruneReview(file); err == nil || strings.Contains(err.Error(), "DO_NOT_REPEAT_THIS") {
		t.Fatal("invalid review accepted or value echoed", err)
	}
}
