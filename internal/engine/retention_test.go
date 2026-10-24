package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRetentionCalendarUnionAndProtection(t *testing.T) {
	stamp := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	snap := func(n int, date string, tags ...string) Snapshot {
		return Snapshot{ID: fmt.Sprintf("%064x", n), Time: stamp(date), Tags: tags}
	}
	input := []Snapshot{
		snap(1, "2026-01-05T10:00:00Z"),
		snap(2, "2026-01-05T09:00:00Z"),
		snap(3, "2026-01-04T09:00:00Z"),
		snap(4, "2025-12-31T09:00:00Z"),
		snap(5, "2025-11-01T09:00:00Z", "homevault:keep"),
		snap(6, "2027-01-01T09:00:00Z"),
	}
	p, err := planRetention("test", input, RetentionPolicy{Daily: 1, Weekly: 2, Monthly: 2}, stamp("2026-01-06T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.DryRun || len(p.Keep) != 5 || len(p.Candidates) != 1 || p.Candidates[0].ID != input[1].ID {
		t.Fatalf("unexpected plan: %+v", p)
	}
	if !strings.Contains(strings.Join(p.Keep[0].Reasons, ","), "future-time") {
		t.Fatal("future snapshot not protected")
	}
	if input[0].ID != fmt.Sprintf("%064x", 1) {
		t.Fatal("input reordered")
	}
	// Dec 31 belongs to the same ISO week as Jan 4, but a different month.
	for _, k := range p.Keep {
		if k.ID == input[3].ID && strings.Join(k.Reasons, ",") != "monthly" {
			t.Fatal("ISO week boundary wrong", k)
		}
	}
}

func TestRetentionZeroPolicyAndInvalidMetadata(t *testing.T) {
	now := time.Now().UTC()
	s := Snapshot{ID: strings.Repeat("a", 64), Time: now.Add(-time.Hour)}
	p, err := planRetention("test", []Snapshot{s}, RetentionPolicy{}, now)
	if err != nil || len(p.Keep) != 1 || len(p.Candidates) != 0 {
		t.Fatal("latest must survive zero policy", p, err)
	}
	for _, input := range [][]Snapshot{{s, s}, {{ID: "short", Time: now}}, {{ID: s.ID}}} {
		if _, err := planRetention("test", input, RetentionPolicy{}, now); err == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
	if _, err := planRetention("test", nil, RetentionPolicy{Daily: -1}, now); err == nil {
		t.Fatal("negative policy accepted")
	}
	p, err = planRetention("test", nil, RetentionPolicy{}, now)
	if err != nil || p.Keep == nil || p.Candidates == nil {
		t.Fatal("empty plan ambiguous")
	}
}

func TestRetentionOnlyListsProjectSnapshots(t *testing.T) {
	e := fixture(t)
	calls := 0
	e.run = func(_ context.Context, args, _ []string, _ bool) ([]byte, error) {
		calls++
		if strings.Join(args[4:], " ") != "snapshots --json --tag homevault:photos" {
			t.Fatalf("unexpected command: %v", args[4:])
		}
		return json.Marshal([]Snapshot{{ID: strings.Repeat("a", 64), Time: time.Now().Add(-time.Hour), Tags: []string{"homevault:photos"}}})
	}
	p, err := e.PlanRetention(context.Background(), "photos", RetentionPolicy{Daily: 7})
	if err != nil || calls != 1 || len(p.Keep) != 1 {
		t.Fatal("read-only plan failed", p, err)
	}
}

func TestRetentionFutureDoesNotDisplaceNewestRealBackup(t *testing.T) {
	now := time.Now().UTC()
	s := []Snapshot{
		{ID: strings.Repeat("a", 64), Time: now.Add(time.Hour)},
		{ID: strings.Repeat("b", 64), Time: now.Add(-time.Hour)},
		{ID: strings.Repeat("c", 64), Time: now.Add(-2 * time.Hour)},
	}
	p, err := planRetention("test", s, RetentionPolicy{}, now)
	if err != nil || len(p.Keep) != 2 || len(p.Candidates) != 1 || p.Keep[1].Reasons[0] != "latest-nonfuture" {
		t.Fatal("clock anomaly displaced newest real backup", p, err)
	}
	duplicate := s[0]
	duplicate.ID = strings.ToUpper(duplicate.ID)
	if _, err := planRetention("test", []Snapshot{s[0], duplicate}, RetentionPolicy{}, now); err == nil {
		t.Fatal("case-variant duplicate accepted")
	}
}
