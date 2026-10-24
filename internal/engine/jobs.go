package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"homevault/internal/config"
	"homevault/internal/lock"
)

type JobAttempt struct {
	ID            string    `json:"id"`
	Started       time.Time `json:"started"`
	Finished      time.Time `json:"finished,omitempty"`
	Status        string    `json:"status"`
	Phase         string    `json:"phase"`
	BackupPassed  bool      `json:"backup_passed"`
	CheckPassed   bool      `json:"check_passed"`
	CapturePassed bool      `json:"capture_passed,omitempty"`
}
type JobStatus struct {
	Version      int          `json:"version"`
	Project      string       `json:"project"`
	RepositoryID string       `json:"repository_id"`
	LastSuccess  time.Time    `json:"last_success,omitempty"`
	Latest       JobAttempt   `json:"latest"`
	History      []JobAttempt `json:"history"`
}
type MonitorResult struct {
	Project      string    `json:"project"`
	Health       string    `json:"health"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	Notification string    `json:"notification"`
}
type sentAlert struct {
	Key    string    `json:"key"`
	Kind   string    `json:"kind"`
	SentAt time.Time `json:"sent_at"`
}

var recoveryAttemptID = regexp.MustCompile(`^[0-9a-f]{24}$`)

func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("cannot create job state directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 && os.PathSeparator != '\\' {
		return errors.New("job state directory must be private and not a symlink")
	}
	return nil
}
func writeState(filename string, value any) error {
	if err := privateDir(filepath.Dir(filename)); err != nil {
		return err
	}
	if info, err := os.Lstat(filename); err == nil && !info.Mode().IsRegular() {
		return errors.New("job state must not be a symlink or special file")
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".state-*")
	if err != nil {
		return errors.New("cannot create job state")
	}
	name := f.Name()
	defer os.Remove(name)
	if err = json.NewEncoder(f).Encode(value); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot persist job state")
	}
	if os.Rename(name, filename) != nil {
		return errors.New("cannot atomically replace job state")
	}
	return nil
}
func readState(filename string, value any) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return errors.New("unsafe or oversized job state")
	}
	f, err := os.Open(filename)
	if err != nil {
		return errors.New("cannot open job state")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1024*1024+1))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(new(any)) != io.EOF {
		return errors.New("invalid job state")
	}
	return nil
}
func (e *Engine) jobPath(project string) string {
	return filepath.Join(e.cfg.StateDir, "jobs", project+".json")
}
func (e *Engine) repositoryID() string {
	identity := e.cfg.Repository
	if e.rclone != "" && e.cfg.RcloneConfigFile != "" {
		identity += "\x00rclone-config\x00" + e.cfg.RcloneConfigFile
	}
	h := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(h[:])
}
func (e *Engine) JobStatus(project string) (JobStatus, error) {
	if _, err := e.cfg.Project(project); err != nil {
		return JobStatus{}, err
	}
	state := JobStatus{Version: 1, Project: project, RepositoryID: e.repositoryID(), Latest: JobAttempt{Status: "never-run"}, History: []JobAttempt{}}
	var loaded JobStatus
	err := readState(e.jobPath(project), &loaded)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state = loaded
	if state.Version != 1 || state.Project != project || state.RepositoryID != e.repositoryID() || len(state.History) > 32 {
		return state, errors.New("job state identity or version mismatch")
	}
	validAttempt := func(a JobAttempt) bool {
		if !recoveryAttemptID.MatchString(a.ID) || a.Started.IsZero() {
			return false
		}
		if a.Status == "running" {
			return a.Phase == "capture" || a.Phase == "backup" || a.Phase == "check"
		}
		if a.Finished.IsZero() || a.Finished.Before(a.Started) {
			return false
		}
		if a.Status == "success" {
			return a.Phase == "completed" && a.BackupPassed && a.CheckPassed
		}
		return a.Status == "failed" && (a.Phase == "capture" || a.Phase == "backup" || a.Phase == "check" || a.Phase == "cancelled")
	}
	if !validAttempt(state.Latest) || state.Latest.Status == "success" && !state.LastSuccess.Equal(state.Latest.Finished) {
		return state, errors.New("invalid job attempt state")
	}
	for _, attempt := range state.History {
		if !validAttempt(attempt) {
			return state, errors.New("invalid job history")
		}
	}
	return state, nil
}

// RunBackupJob holds the engine lock across backup AND metadata check. A green
// job means exactly those two phases, never a restore/application recovery drill.
func (e *Engine) RunBackupJob(ctx context.Context, project string) error {
	return e.runBackupJob(ctx, project, nil)
}

type jobPreparation func(context.Context, config.Project) (config.Project, func() error, error)

func (e *Engine) runBackupJob(ctx context.Context, project string, prepare jobPreparation) error {
	if e.cfg.RecoveryOnly {
		return errors.New("recovery-only configuration refuses backup/capture jobs")
	}
	p, err := e.cfg.Project(project)
	if err != nil {
		return err
	}
	if p.CaptureRequired && prepare == nil {
		return errors.New("project requires capture-backup; ordinary job-backup refused")
	}
	jobErr := e.withLock(ctx, func() (result error) {
		state, err := e.JobStatus(project)
		if err != nil {
			return err
		}
		var nonce [12]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return errors.New("cannot generate job identity")
		}
		attempt := JobAttempt{ID: hex.EncodeToString(nonce[:]), Started: time.Now().UTC(), Status: "running", Phase: "backup"}
		if prepare != nil {
			attempt.Phase = "capture"
		}
		state.Latest = attempt
		if err = writeState(e.jobPath(project), state); err != nil {
			return err
		}
		defer func() {
			attempt.Finished = time.Now().UTC()
			attempt.Status = "failed"
			if result == nil {
				attempt.Status = "success"
				attempt.Phase = "completed"
				state.LastSuccess = attempt.Finished
			}
			if ctx.Err() != nil && result != nil {
				attempt.Status = "failed"
				attempt.Phase = "cancelled"
			}
			state.Latest = attempt
			state.History = append(state.History, attempt)
			if len(state.History) > 32 {
				state.History = state.History[len(state.History)-32:]
			}
			result = errors.Join(result, writeState(e.jobPath(project), state))
		}()
		if prepare != nil {
			var cleanup func() error
			p, cleanup, err = prepare(ctx, p)
			if err != nil {
				return err
			}
			if cleanup != nil {
				defer func() {
					if result == nil {
						result = errors.Join(result, cleanup())
					}
				}()
			}
			attempt.CapturePassed = true
			attempt.Phase = "backup"
			state.Latest = attempt
			if err = writeState(e.jobPath(project), state); err != nil {
				return err
			}
		}
		if err = e.backupLocked(ctx, p); err != nil {
			return err
		}
		attempt.BackupPassed = true
		attempt.Phase = "check"
		state.Latest = attempt
		if err = writeState(e.jobPath(project), state); err != nil {
			return err
		}
		if _, err = e.command(ctx, "job repository metadata check", false, "check"); err != nil {
			return err
		}
		attempt.CheckPassed = true
		return nil
	})
	// Notify after releasing the operation lock. Cancellation still allows a
	// bounded failure notification; delivery failure never advances LastSuccess.
	notificationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	age := 36 * time.Hour
	if e.cfg.BackupMaxAge != "" {
		age, _ = time.ParseDuration(e.cfg.BackupMaxAge)
	}
	_, notificationErr := e.Monitor(notificationCtx, project, age)
	return errors.Join(jobErr, notificationErr)
}

func evaluateJob(s JobStatus, now time.Time, maxAge time.Duration) (string, string, string) {
	anchor := s.LastSuccess
	if anchor.IsZero() {
		anchor = s.Latest.Started
	}
	if anchor.IsZero() || now.Sub(anchor) > maxAge {
		return "stale", "stale", "stale:" + s.LastSuccess.Format(time.RFC3339Nano)
	}
	if s.Latest.Status == "failed" {
		return "failed", "failed", "failed:" + s.Latest.ID
	}
	if s.Latest.Status == "running" {
		return "running", "", ""
	}
	return "healthy", "success", "success:" + s.Latest.ID
}
func (e *Engine) Monitor(ctx context.Context, project string, maxAge time.Duration) (MonitorResult, error) {
	result := MonitorResult{Project: project, Notification: "disabled"}
	if maxAge <= 0 {
		return result, errors.New("maximum backup age must be positive")
	}
	state, err := e.JobStatus(project)
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	if state.LastSuccess.After(now.Add(5*time.Minute)) || state.Latest.Started.After(now.Add(5*time.Minute)) {
		return result, errors.New("job timestamps are in the future; check clock and state")
	}
	result.LastSuccess = state.LastSuccess
	health, event, key := evaluateJob(state, now, maxAge)
	p, _ := e.cfg.Project(project)
	if p.CaptureRequired && health == "healthy" && !state.Latest.CapturePassed {
		health = "failed"
		event = "failed"
		key = "capture-required:" + state.Latest.ID
	}
	result.Health = health
	if e.cfg.DiscordWebhookFile != "" {
		result.Notification = "not-needed"
	}
	if e.cfg.DiscordWebhookFile == "" || event == "" {
		return result, nil
	}
	alertDir := filepath.Join(e.cfg.StateDir, "alerts", project)
	if err = privateDir(alertDir); err != nil {
		return result, err
	}
	alertLock, err := lock.Acquire(alertDir)
	if err != nil {
		return result, err
	}
	defer alertLock.Release()
	var previous sentAlert
	if err = readState(filepath.Join(alertDir, "sent.json"), &previous); err != nil && !os.IsNotExist(err) {
		return result, err
	}
	if health == "healthy" {
		if previous.Kind == "failed" || previous.Kind == "stale" {
			event = "recovered"
			key = "recovered:" + state.Latest.ID
		} else if !e.cfg.NotifySuccess {
			result.Notification = "not-needed"
			return result, nil
		}
	}
	if previous.Key == key {
		result.Notification = "deduplicated"
		return result, nil
	}
	if err = e.sendNotification(ctx, project, event); err != nil {
		result.Notification = "failed"
		return result, err
	}
	if err = writeState(filepath.Join(alertDir, "sent.json"), sentAlert{Key: key, Kind: event, SentAt: now}); err != nil {
		return result, err
	}
	result.Notification = "sent"
	return result, nil
}

func (e *Engine) NotifyTest(ctx context.Context, project string) error {
	if _, err := e.cfg.Project(project); err != nil {
		return err
	}
	return e.sendNotification(ctx, project, "test")
}

// NotifyEvent lets a reviewed external capture workflow reuse the redacted
// notifier. It never manufactures a backup result or changes job freshness.
func (e *Engine) NotifyEvent(ctx context.Context, project, event string) error {
	if e.cfg.RecoveryOnly {
		return errors.New("recovery-only configuration refuses workflow notifications")
	}
	if _, err := e.cfg.Project(project); err != nil {
		return err
	}
	if event != "failed" && event != "stale" && event != "recovered" {
		return errors.New("workflow event must be failed, stale or recovered")
	}
	return e.sendNotification(ctx, project, event)
}
