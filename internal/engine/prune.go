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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const pruneMode = "unused-packs-only"
const testedPruneVersion = "0.19.1"

type PruneQuantity struct {
	Blobs uint64 `json:"blobs"`
	Size  string `json:"approximate_size"`
}

type PruneEstimate struct {
	Repack       PruneQuantity `json:"repack"`
	RepackRemove PruneQuantity `json:"repack_remove"`
	Delete       PruneQuantity `json:"delete"`
	TotalRemove  PruneQuantity `json:"total_remove"`
	Remaining    PruneQuantity `json:"remaining"`
}

type PruneReview struct {
	Version       int           `json:"version"`
	RepositoryID  string        `json:"repository_id"`
	Scope         string        `json:"scope"`
	Created       time.Time     `json:"created"`
	SnapshotSet   string        `json:"snapshot_set"`
	SnapshotIDs   []string      `json:"snapshot_ids"`
	StorageSet    string        `json:"storage_set"`
	ResticVersion string        `json:"restic_version"`
	Mode          string        `json:"mode"`
	Estimate      PruneEstimate `json:"estimate"`
}

type PruneResult struct {
	Started       time.Time     `json:"started"`
	Finished      time.Time     `json:"finished,omitempty"`
	RepositoryID  string        `json:"repository_id"`
	Preserved     string        `json:"preserved"`
	SnapshotCount int           `json:"snapshot_count"`
	Estimate      PruneEstimate `json:"estimate"`
	Status        string        `json:"status"`
	Verified      bool          `json:"verified"`
}

func (e *Engine) pruneVersion(ctx context.Context) error {
	data, err := e.command(ctx, "read prune tool version", true, "version")
	if err != nil {
		return err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 || fields[0] != "restic" || fields[1] != testedPruneVersion {
		return errors.New("prune requires tested Restic 0.19.1; unsupported output/version refused")
	}
	return nil
}

// This scope deliberately includes untagged snapshots and every other project.
func (e *Engine) repositorySnapshots(ctx context.Context) ([]Snapshot, error) {
	data, err := e.command(ctx, "list repository snapshots", true, "snapshots", "--json")
	if err != nil {
		return nil, err
	}
	var snapshots []Snapshot
	if json.Unmarshal(data, &snapshots) != nil {
		return nil, errors.New("invalid repository snapshot metadata")
	}
	if len(snapshots) == 0 {
		return nil, errors.New("prune refuses a repository with no retained snapshots")
	}
	seen := map[string]bool{}
	for _, s := range snapshots {
		id := strings.ToLower(s.ID)
		if !snapshotID.MatchString(id) || seen[id] || s.Time.IsZero() {
			return nil, errors.New("invalid or duplicate repository snapshot metadata")
		}
		seen[id] = true
	}
	return snapshots, nil
}

func snapshotIDs(snapshots []Snapshot) []string {
	ids := make([]string, 0, len(snapshots))
	for _, s := range snapshots {
		ids = append(ids, strings.ToLower(s.ID))
	}
	sort.Strings(ids)
	return ids
}

// Fingerprint remote pack and index IDs, not cache/lock files or credential data.
func (e *Engine) pruneStorageSet(ctx context.Context) (string, error) {
	inventory := map[string][]string{}
	for _, kind := range []string{"packs", "index"} {
		data, err := e.command(ctx, "read prune storage inventory", true, "list", kind)
		if err != nil {
			return "", err
		}
		ids := []string{}
		seen := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			id := strings.TrimSpace(line)
			if id == "" {
				continue
			}
			if !snapshotID.MatchString(id) || seen[strings.ToLower(id)] {
				return "", errors.New("invalid prune storage inventory")
			}
			id = strings.ToLower(id)
			seen[id] = true
			ids = append(ids, id)
		}
		sort.Strings(ids)
		inventory[kind] = ids
	}
	data, _ := json.Marshal(inventory)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

var pruneSummaryLine = regexp.MustCompile(`(?m)^(to repack|this removes|to delete|total prune|remaining):[ \t]+([0-9]+) blobs / ([0-9]+(?:\.[0-9]+)? (?:B|KiB|MiB|GiB|TiB|PiB|EiB))[ \t]*\r?$`)

func parsePruneEstimate(data []byte) (PruneEstimate, error) {
	var estimate PruneEstimate
	if strings.Count(string(data), "Would have made the following changes:") != 1 {
		return estimate, errors.New("unsupported prune dry-run summary")
	}
	quantities := map[string]PruneQuantity{}
	for _, match := range pruneSummaryLine.FindAllStringSubmatch(string(data), -1) {
		if _, exists := quantities[match[1]]; exists {
			return estimate, errors.New("duplicate prune summary field")
		}
		n, err := strconv.ParseUint(match[2], 10, 64)
		if err != nil {
			return estimate, errors.New("invalid prune summary count")
		}
		quantities[match[1]] = PruneQuantity{Blobs: n, Size: match[3]}
	}
	if len(quantities) != 5 {
		return estimate, errors.New("incomplete prune summary; no prune authorized")
	}
	estimate = PruneEstimate{Repack: quantities["to repack"], RepackRemove: quantities["this removes"], Delete: quantities["to delete"], TotalRemove: quantities["total prune"], Remaining: quantities["remaining"]}
	if estimate.Repack.Blobs != 0 || estimate.Repack.Size != "0 B" || estimate.RepackRemove.Blobs != 0 || estimate.RepackRemove.Size != "0 B" {
		return PruneEstimate{}, errors.New("prune would repack data; unused-packs-only mode refused")
	}
	return estimate, nil
}

func (e *Engine) pruneDryRun(ctx context.Context) (PruneEstimate, error) {
	data, err := e.command(ctx, "preview unused-pack prune", true, "prune", "--dry-run", "--max-repack-size=0", "--max-unused=unlimited")
	if err != nil {
		return PruneEstimate{}, err
	}
	return parsePruneEstimate(data)
}

func (e *Engine) ReviewPrune(ctx context.Context) (review PruneReview, err error) {
	err = e.withLock(ctx, func() error {
		if err := e.pruneVersion(ctx); err != nil {
			return err
		}
		snapshots, err := e.repositorySnapshots(ctx)
		if err != nil {
			return err
		}
		storage, err := e.pruneStorageSet(ctx)
		if err != nil {
			return err
		}
		if _, err = e.command(ctx, "check before prune preview", false, "check"); err != nil {
			return err
		}
		estimate, err := e.pruneDryRun(ctx)
		if err != nil {
			return err
		}
		after, err := e.repositorySnapshots(ctx)
		if err != nil {
			return err
		}
		afterStorage, err := e.pruneStorageSet(ctx)
		if err != nil {
			return err
		}
		if snapshotSet(after) != snapshotSet(snapshots) || storage != afterStorage {
			return errors.New("repository changed during prune preview; create a new review")
		}
		review = PruneReview{Version: 1, RepositoryID: e.repositoryID(), Scope: "repository", Created: time.Now().UTC(), SnapshotSet: snapshotSet(snapshots), SnapshotIDs: snapshotIDs(snapshots), StorageSet: storage, ResticVersion: testedPruneVersion, Mode: pruneMode, Estimate: estimate}
		return nil
	})
	return review, err
}

func LoadPruneReview(file string) (PruneReview, error) {
	var review PruneReview
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8*1024*1024 || os.PathSeparator != '\\' && info.Mode().Perm()&0077 != 0 {
		return review, errors.New("prune review must be a private regular file of at most 8 MiB")
	}
	f, err := os.Open(file)
	if err != nil {
		return review, errors.New("cannot open prune review")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 8*1024*1024+1))
	d.DisallowUnknownFields()
	if d.Decode(&review) != nil || d.Decode(new(any)) != io.EOF {
		return PruneReview{}, errors.New("invalid prune review JSON")
	}
	return review, nil
}

func validPruneReviewTime(t time.Time) bool {
	now := time.Now().UTC()
	return !t.IsZero() && !t.After(now) && now.Sub(t) <= 24*time.Hour
}

// ApplyPrune never forgets snapshots. The caller acknowledges repository-wide scope.
// Do not concurrently access this repository with another client/state directory.
func (e *Engine) ApplyPrune(ctx context.Context, review PruneReview, preserve string) (result PruneResult, err error) {
	if review.Version != 1 || review.RepositoryID != e.repositoryID() || review.Scope != "repository" || review.Mode != pruneMode || review.ResticVersion != testedPruneVersion || !validPruneReviewTime(review.Created) {
		return result, errors.New("prune review identity/mode/version invalid or expired; create a new review")
	}
	preserve = strings.ToLower(preserve)
	if !snapshotID.MatchString(preserve) {
		return result, errors.New("preserve requires a full independently verified snapshot ID")
	}
	err = e.withLock(ctx, func() (applyErr error) {
		if err := e.pruneVersion(ctx); err != nil {
			return err
		}
		snapshots, err := e.repositorySnapshots(ctx)
		if err != nil {
			return err
		}
		ids := snapshotIDs(snapshots)
		if snapshotSet(snapshots) != review.SnapshotSet || !reflect.DeepEqual(ids, review.SnapshotIDs) {
			return errors.New("repository snapshots changed since prune review")
		}
		found := false
		for _, id := range ids {
			if id == preserve {
				found = true
			}
		}
		if !found {
			return errors.New("preserved snapshot is not in the repository review")
		}
		storage, err := e.pruneStorageSet(ctx)
		if err != nil {
			return err
		}
		if storage != review.StorageSet {
			return errors.New("pack/index inventory changed since prune review")
		}
		result = PruneResult{Started: time.Now().UTC(), RepositoryID: e.repositoryID(), Preserved: preserve, SnapshotCount: len(ids), Estimate: review.Estimate, Status: "running"}
		audit := filepath.Join(e.cfg.StateDir, "prune", "latest.json")
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
		if _, err = e.command(ctx, "check before prune", false, "check"); err != nil {
			return err
		}
		estimate, err := e.pruneDryRun(ctx)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(estimate, review.Estimate) {
			return errors.New("prune estimate changed; create and review a new plan")
		}
		current, err := e.repositorySnapshots(ctx)
		if err != nil {
			return err
		}
		currentStorage, err := e.pruneStorageSet(ctx)
		if err != nil {
			return err
		}
		if snapshotSet(current) != review.SnapshotSet || currentStorage != review.StorageSet || !validPruneReviewTime(review.Created) {
			return errors.New("repository or review age changed during prune preflight")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err = e.command(ctx, "prune reviewed unused packs", false, "prune", "--max-repack-size=0", "--max-unused=unlimited"); err != nil {
			return err
		}
		if _, err = e.command(ctx, "check after prune", false, "check"); err != nil {
			return err
		}
		after, err := e.repositorySnapshots(ctx)
		if err != nil {
			return err
		}
		if snapshotSet(after) != review.SnapshotSet {
			return errors.New("snapshot metadata changed after prune; inspect repository")
		}
		result.Verified = true
		return nil
	})
	return result, err
}
