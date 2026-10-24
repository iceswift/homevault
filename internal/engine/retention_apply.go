package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

// RetentionReview is a review document, not a signature or authorization token.
type RetentionReview struct {
	Version      int           `json:"version"`
	RepositoryID string        `json:"repository_id"`
	Created      time.Time     `json:"created"`
	SnapshotSet  string        `json:"snapshot_set"`
	Plan         RetentionPlan `json:"plan"`
}

type RetentionResult struct {
	Project   string    `json:"project"`
	Started   time.Time `json:"started"`
	Finished  time.Time `json:"finished,omitempty"`
	Preserved string    `json:"preserved"`
	Requested []string  `json:"requested"`
	Status    string    `json:"status"`
	Verified  bool      `json:"verified"`
}

func snapshotSet(snapshots []Snapshot) string {
	copySet := append([]Snapshot(nil), snapshots...)
	for i := range copySet {
		s := &copySet[i]
		s.ID = strings.ToLower(s.ID)
		s.ShortID = ""
		s.Time = s.Time.UTC()
		s.Tags = append([]string(nil), s.Tags...)
		s.Paths = append([]string(nil), s.Paths...)
		sort.Strings(s.Tags)
		sort.Strings(s.Paths)
	}
	sort.Slice(copySet, func(i, j int) bool { return copySet[i].ID < copySet[j].ID })
	data, _ := json.Marshal(copySet)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (e *Engine) ReviewRetention(ctx context.Context, project string, policy RetentionPolicy) (review RetentionReview, err error) {
	if _, err = e.cfg.Project(project); err != nil {
		return review, err
	}
	if err = validateRetention(policy); err != nil {
		return review, err
	}
	err = e.withLock(ctx, func() error {
		snapshots, err := e.snapshots(ctx, project)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		plan, err := planRetention(project, snapshots, policy, now)
		if err != nil {
			return err
		}
		review = RetentionReview{Version: 1, RepositoryID: e.repositoryID(), Created: now, SnapshotSet: snapshotSet(snapshots), Plan: plan}
		return nil
	})
	return review, err
}

func LoadRetentionReview(file string) (RetentionReview, error) {
	var review RetentionReview
	info, statErr := os.Lstat(file)
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 8*1024*1024 || os.PathSeparator != '\\' && info.Mode().Perm()&0077 != 0 {
		return review, errors.New("retention review must be a private regular file of at most 8 MiB")
	}
	f, err := os.Open(file)
	if err != nil {
		return review, errors.New("cannot open retention review")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 8*1024*1024+1))
	d.DisallowUnknownFields()
	if d.Decode(&review) != nil || d.Decode(new(any)) != io.EOF {
		return RetentionReview{}, errors.New("invalid retention review JSON")
	}
	return review, nil
}

// ApplyRetention forgets only reviewed explicit IDs. It never invokes prune.
// Other tools must not mutate this repository concurrently; our lock is local.
func (e *Engine) ApplyRetention(ctx context.Context, project string, review RetentionReview, preserve string) (result RetentionResult, err error) {
	if _, err = e.cfg.Project(project); err != nil {
		return result, err
	}
	now := time.Now().UTC()
	if review.Version != 1 || review.RepositoryID != e.repositoryID() || review.Plan.Project != project || !review.Plan.DryRun || review.Plan.Timezone != "UTC" || review.Created.IsZero() || review.Created.After(now) || now.Sub(review.Created) > 24*time.Hour {
		return result, errors.New("retention review identity invalid or older than 24 hours; create and review a new plan")
	}
	preserve = strings.ToLower(preserve)
	if !snapshotID.MatchString(preserve) {
		return result, errors.New("preserve requires a complete snapshot ID verified by the owner")
	}
	if len(review.Plan.Candidates) > 64 {
		return result, errors.New("retention apply is limited to 64 candidate snapshots; use a more conservative reviewed policy")
	}
	err = e.withLock(ctx, func() (applyErr error) {
		snapshots, err := e.snapshots(ctx, project)
		if err != nil {
			return err
		}
		if snapshotSet(snapshots) != review.SnapshotSet {
			return errors.New("snapshot metadata changed since review; no snapshots forgotten")
		}
		expected, err := planRetention(project, snapshots, review.Plan.Policy, review.Created)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(expected, review.Plan) {
			return errors.New("retention plan differs from recomputed policy; no snapshots forgotten")
		}
		kept := map[string]bool{}
		for _, s := range expected.Keep {
			kept[s.ID] = true
		}
		if !kept[preserve] {
			return errors.New("preserved snapshot must be in the reviewed keep set")
		}
		result = RetentionResult{Project: project, Started: now, Preserved: preserve, Requested: []string{}, Status: "running"}
		for _, s := range expected.Candidates {
			result.Requested = append(result.Requested, s.ID)
		}
		audit := filepath.Join(e.cfg.StateDir, "retention", project+".json")
		if err := writeState(audit, result); err != nil {
			return err
		}
		defer func() {
			result.Finished = time.Now().UTC()
			result.Status = "failed"
			if applyErr == nil {
				result.Status = "success"
			}
			applyErr = errors.Join(applyErr, writeState(audit, result))
		}()
		if len(result.Requested) == 0 {
			result.Verified = true
			return nil
		}
		if _, err = e.command(ctx, "check before retention", false, "check"); err != nil {
			return err
		}
		// Re-read after potentially long metadata checking, before destructive work.
		snapshots, err = e.snapshots(ctx, project)
		if err != nil {
			return err
		}
		if snapshotSet(snapshots) != review.SnapshotSet {
			return errors.New("snapshots changed during preflight; no snapshots forgotten")
		}
		if now := time.Now().UTC(); now.Before(review.Created) || now.Sub(review.Created) > 24*time.Hour {
			return errors.New("retention review expired during preflight; create a new review")
		}
		args := append([]string{"forget", "--"}, result.Requested...)
		if _, err = e.command(ctx, "forget reviewed snapshots", false, args...); err != nil {
			return err
		}
		remaining, err := e.snapshots(ctx, project)
		if err != nil {
			return err
		}
		if len(remaining) != len(kept) {
			return errors.New("retention postcondition failed; inspect repository before retrying")
		}
		seen := map[string]bool{}
		for _, s := range remaining {
			id := strings.ToLower(s.ID)
			if !kept[id] || seen[id] {
				return errors.New("retention left an unexpected snapshot; inspect repository")
			}
			seen[id] = true
		}
		if _, err = e.command(ctx, "check after retention", false, "check"); err != nil {
			return err
		}
		result.Verified = true
		return nil
	})
	return result, err
}
