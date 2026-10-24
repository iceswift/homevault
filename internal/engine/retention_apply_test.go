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

func retentionFixture(t *testing.T) (*Engine, *[]Snapshot, *[]string) {
	t.Helper()
	e := fixture(t)
	set := []Snapshot{
		{ID: strings.Repeat("a", 64), Time: time.Now().Add(-time.Hour).UTC(), Hostname: "test", Tags: []string{"homevault:photos"}},
		{ID: strings.Repeat("b", 64), Time: time.Now().Add(-2 * time.Hour).UTC(), Hostname: "test", Tags: []string{"homevault:photos"}},
	}
	calls := []string{}
	e.run = func(_ context.Context, args, _ []string, _ bool) ([]byte, error) {
		calls = append(calls, args[4])
		if _, err := lock.Acquire(e.cfg.StateDir); !errors.Is(err, lock.ErrLocked) {
			t.Fatal("retention operation not locked")
		}
		switch args[4] {
		case "snapshots":
			return json.Marshal(set)
		case "check":
			return nil, nil
		case "forget":
			if strings.Join(args[5:], " ") != "-- "+strings.Repeat("b", 64) {
				t.Fatal("unexpected destructive arguments", args[5:])
			}
			set = set[:1]
			return nil, nil
		default:
			t.Fatal("unexpected command", args[4])
		}
		return nil, nil
	}
	return e, &set, &calls
}

func TestRetentionApplyExactIDsAndAudit(t *testing.T) {
	e, _, calls := retentionFixture(t)
	review, err := e.ReviewRetention(context.Background(), "photos", RetentionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the same JSON round trip used by the CLI.
	file := filepath.Join(t.TempDir(), "review.json")
	data, _ := json.Marshal(review)
	os.WriteFile(file, data, 0600)
	review, err = LoadRetentionReview(file)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.ApplyRetention(context.Background(), "photos", review, strings.Repeat("a", 64))
	if err != nil || !r.Verified || r.Status != "success" {
		t.Fatal("apply failed", r, err)
	}
	if strings.Join(*calls, ",") != "snapshots,snapshots,check,snapshots,forget,snapshots,check" {
		t.Fatal("wrong command order", *calls)
	}
	var audit RetentionResult
	if err := readState(filepath.Join(e.cfg.StateDir, "retention", "photos.json"), &audit); err != nil || !audit.Verified || audit.Status != "success" {
		t.Fatal("audit not persisted", audit, err)
	}
	state, err := e.JobStatus("photos")
	if err != nil || !state.LastSuccess.IsZero() || state.Latest.Status != "never-run" {
		t.Fatal("retention advanced backup freshness", state, err)
	}
}

func TestRetentionForgetFailureDoesNotClaimSuccess(t *testing.T) {
	e, _, _ := retentionFixture(t)
	review, err := e.ReviewRetention(context.Background(), "photos", RetentionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	original := e.run
	e.run = func(ctx context.Context, args, env []string, capture bool) ([]byte, error) {
		if args[4] == "forget" {
			return nil, errors.New("child output not retained")
		}
		return original(ctx, args, env, capture)
	}
	r, err := e.ApplyRetention(context.Background(), "photos", review, strings.Repeat("a", 64))
	if err == nil || r.Verified || r.Status != "failed" {
		t.Fatal("failed forget reported success", r, err)
	}
}

func TestRetentionApplyRefusesChangedOrUnsafeReview(t *testing.T) {
	for _, which := range []string{"expired", "future", "repo", "plan", "metadata", "preserve", "candidate-limit"} {
		t.Run(which, func(t *testing.T) {
			e, set, calls := retentionFixture(t)
			review, err := e.ReviewRetention(context.Background(), "photos", RetentionPolicy{})
			if err != nil {
				t.Fatal(err)
			}
			preserve := strings.Repeat("a", 64)
			switch which {
			case "expired":
				review.Created = time.Now().Add(-25 * time.Hour)
			case "future":
				review.Created = time.Now().Add(time.Hour)
			case "repo":
				review.RepositoryID = "other"
			case "plan":
				review.Plan.Keep = nil
			case "metadata":
				(*set)[1].Tags = append((*set)[1].Tags, "homevault:keep")
			case "preserve":
				preserve = strings.Repeat("b", 64)
			case "candidate-limit":
				review.Plan.Candidates = make([]RetentionEntry, 65)
			}
			if _, err = e.ApplyRetention(context.Background(), "photos", review, preserve); err == nil {
				t.Fatal("unsafe review accepted")
			}
			for _, c := range *calls {
				if c == "forget" {
					t.Fatal("deleted despite rejection")
				}
			}
		})
	}
}

func TestRetentionApplyCheckFailureAndLateMutation(t *testing.T) {
	for _, late := range []bool{false, true} {
		e, set, calls := retentionFixture(t)
		review, err := e.ReviewRetention(context.Background(), "photos", RetentionPolicy{})
		if err != nil {
			t.Fatal(err)
		}
		original := e.run
		e.run = func(ctx context.Context, args, env []string, capture bool) ([]byte, error) {
			if args[4] == "check" {
				if !late {
					return nil, errors.New("withheld check failure")
				}
				(*set)[1].Hostname = "new-host"
			}
			return original(ctx, args, env, capture)
		}
		if _, err = e.ApplyRetention(context.Background(), "photos", review, strings.Repeat("a", 64)); err == nil {
			t.Fatal("failed check or mutation accepted")
		}
		for _, c := range *calls {
			if c == "forget" {
				t.Fatal("forget ran after failed preflight")
			}
		}
		var audit RetentionResult
		if err := readState(filepath.Join(e.cfg.StateDir, "retention", "photos.json"), &audit); err != nil || audit.Status != "failed" || audit.Verified {
			t.Fatal("failure audit missing", audit, err)
		}
	}
}

func TestRetentionProtectsHostsAndSharedProjects(t *testing.T) {
	base := time.Now().UTC()
	s := []Snapshot{
		{ID: strings.Repeat("a", 64), Time: base.Add(-time.Hour), Hostname: "one"},
		{ID: strings.Repeat("b", 64), Time: base.Add(-2 * time.Hour), Hostname: "two"},
		{ID: strings.Repeat("c", 64), Time: base.Add(-3 * time.Hour), Hostname: "one", Tags: []string{"homevault:photos", "homevault:another"}},
	}
	p, err := planRetention("photos", s, RetentionPolicy{}, base)
	if err != nil || len(p.Keep) != 3 || len(p.Candidates) != 0 {
		t.Fatal("host/shared snapshot lost", p, err)
	}
}
