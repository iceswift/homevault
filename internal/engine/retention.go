package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RetentionPolicy counts occupied UTC calendar buckets, not elapsed durations.
type RetentionPolicy struct {
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
}

type RetentionEntry struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Reasons []string  `json:"reasons,omitempty"`
}

type RetentionPlan struct {
	Project    string           `json:"project"`
	DryRun     bool             `json:"dry_run"`
	Timezone   string           `json:"timezone"`
	Policy     RetentionPolicy  `json:"policy"`
	Keep       []RetentionEntry `json:"keep"`
	Candidates []RetentionEntry `json:"candidates"`
}

// PlanRetention only lists snapshots. It never invokes forget, prune or delete.
func (e *Engine) PlanRetention(ctx context.Context, project string, policy RetentionPolicy) (RetentionPlan, error) {
	if err := validateRetention(policy); err != nil {
		return RetentionPlan{}, err
	}
	snapshots, err := e.Snapshots(ctx, project)
	if err != nil {
		return RetentionPlan{}, err
	}
	return planRetention(project, snapshots, policy, time.Now())
}

func validateRetention(p RetentionPolicy) error {
	if p.Daily < 0 || p.Weekly < 0 || p.Monthly < 0 || p.Daily > 10000 || p.Weekly > 10000 || p.Monthly > 10000 {
		return errors.New("retention counts must be between 0 and 10000")
	}
	return nil
}

func planRetention(project string, snapshots []Snapshot, policy RetentionPolicy, now time.Time) (RetentionPlan, error) {
	if err := validateRetention(policy); err != nil {
		return RetentionPlan{}, err
	}
	plan := RetentionPlan{Project: project, DryRun: true, Timezone: "UTC", Policy: policy, Keep: []RetentionEntry{}, Candidates: []RetentionEntry{}}
	ordered := append([]Snapshot(nil), snapshots...)
	seenIDs := make(map[string]bool)
	for i, s := range ordered {
		id := strings.ToLower(s.ID)
		if !snapshotID.MatchString(s.ID) || seenIDs[id] || s.Time.IsZero() {
			return RetentionPlan{}, errors.New("invalid or duplicate snapshot metadata; no retention plan produced")
		}
		seenIDs[id] = true
		ordered[i].ID = id
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Time.Equal(ordered[j].Time) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Time.After(ordered[j].Time)
	})
	days, weeks, months := map[string]bool{}, map[string]bool{}, map[string]bool{}
	latestPast := false
	for i, s := range ordered {
		t := s.Time.UTC()
		entry := RetentionEntry{ID: s.ID, Time: t}
		if i == 0 {
			entry.Reasons = append(entry.Reasons, "latest")
		}
		if s.Time.After(now) {
			entry.Reasons = append(entry.Reasons, "future-time")
		}
		for _, tag := range s.Tags {
			if tag == "homevault:keep" {
				entry.Reasons = append(entry.Reasons, "pinned")
				break
			}
		}
		// Future timestamps are protected but do not consume normal calendar slots.
		if !s.Time.After(now) {
			if !latestPast {
				latestPast = true
				if i != 0 {
					entry.Reasons = append(entry.Reasons, "latest-nonfuture")
				}
			}
			year, week := t.ISOWeek()
			buckets := []struct {
				name, key string
				limit     int
				seen      map[string]bool
			}{
				{"daily", t.Format("2006-01-02"), policy.Daily, days},
				{"weekly", fmt.Sprintf("%04d-%02d", year, week), policy.Weekly, weeks},
				{"monthly", t.Format("2006-01"), policy.Monthly, months},
			}
			for _, b := range buckets {
				if !b.seen[b.key] && len(b.seen) < b.limit {
					b.seen[b.key] = true
					entry.Reasons = append(entry.Reasons, b.name)
				}
			}
		}
		if len(entry.Reasons) > 0 {
			plan.Keep = append(plan.Keep, entry)
		} else {
			plan.Candidates = append(plan.Candidates, entry)
		}
	}
	return plan, nil
}
