// Package jobops supervises explicitly reviewed native file jobs. It has no
// repository initialization, capture, restore, prune, schedule or unlock action.
package jobops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode"

	"homevault/internal/config"
	"homevault/internal/engine"
	"homevault/internal/statusdata"
)

type jobEngine interface {
	JobStatus(string) (engine.JobStatus, error)
	RunReviewedFileJob(context.Context, string, string, func() error) error
}
type pendingReview struct {
	view     statusdata.JobReview
	previous engine.JobStatus
}
type Backend struct {
	cfg          config.Config
	path, digest string
	engine       jobEngine
	mu           sync.Mutex
	pending      *pendingReview
	active       bool
	epoch        uint64
	cancel       context.CancelFunc
	workers      sync.WaitGroup
	timeout      time.Duration
}

func digest(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return "", errors.New("configuration unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("configuration unavailable")
	}
	defer f.Close()
	current, err := f.Stat()
	if err != nil || !os.SameFile(info, current) {
		return "", errors.New("configuration changed")
	}
	data, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return "", errors.New("configuration unavailable")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func New(path string) (*Backend, error) { return NewWithTimeout(path, 2*time.Hour) }
func NewWithTimeout(path string, timeout time.Duration) (*Backend, error) {
	if timeout <= 0 {
		return nil, errors.New("job timeout must be positive")
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("manual jobs require an absolute explicit configuration")
	}
	fingerprint, err := digest(path)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, errors.New("manual-job configuration invalid or unreadable")
	}
	e, err := engine.New(cfg)
	var available jobEngine
	if err == nil {
		available = e
	}
	b := &Backend{cfg: cfg, path: path, digest: fingerprint, engine: available, timeout: timeout}
	if err = b.unchanged(); err != nil {
		return nil, err
	}
	return b, nil
}
func (b *Backend) unchanged() error {
	value, err := digest(b.path)
	if err != nil || value != b.digest {
		return errors.New("configuration changed; restart and review again")
	}
	return nil
}
func (b *Backend) blocked(p config.Project) string {
	if b.cfg.RecoveryOnly {
		return "Recovery-only configuration refuses backup jobs."
	}
	if p.CaptureRequired {
		return "Capture-required project: use its reviewed capture workflow; this UI cannot pause apps."
	}
	if len(p.Databases) > 0 {
		return "Database project: use a reviewed database/capture workflow; this UI supports files only."
	}
	return ""
}
func (b *Backend) Load() (statusdata.Snapshot, error) {
	if err := b.unchanged(); err != nil {
		return statusdata.Snapshot{}, err
	}
	data := statusdata.Snapshot{ReadAt: time.Now().UTC(), Notifications: "Existing config policy; only after confirmed jobs"}
	for _, p := range b.cfg.Projects {
		row := statusdata.Project{ID: p.Name, Name: p.Name, ProjectName: p.Name, Kind: "Native file job", State: "unverified", BackupCheck: "no-record", Recovery: "Unknown - not inspected", Schedule: "Unknown - not inspected", Activation: "Manual only; not a workflow/timer gate", RecordScope: "Native engine job record", Attempt: "No local job record", Note: "Source age is unknown. This manual command does not read coordinator activation flags. This view does not inspect or enable schedules."}
		row.Readiness = b.readiness(p)
		var state engine.JobStatus
		err := errors.New("engine unavailable")
		if b.engine != nil {
			state, err = b.engine.JobStatus(p.Name)
		}
		if err != nil {
			row.State = "unknown"
			row.BackupCheck = "unknown"
			row.Note = "Local job record unreadable; no backup absence is inferred."
			if b.engine == nil {
				row.State = "blocked"
				row.Note = "Job tools or configuration unavailable; local history was not loaded. Open readiness (d) for diagnostics."
			}
		} else {
			row.Uploaded = state.LastSuccess
			row.HistoryKnown = true
			seen := map[string]bool{}
			add := func(a engine.JobAttempt) {
				if a.ID == "" || seen[a.ID] || len(row.History) >= 33 {
					return
				}
				seen[a.ID] = true
				row.History = append(row.History, statusdata.JobRecord{ID: a.ID, Status: a.Status, Phase: a.Phase, Started: a.Started, Finished: a.Finished, BackupPassed: a.BackupPassed, CheckPassed: a.CheckPassed, CapturePassed: a.CapturePassed})
			}
			add(state.Latest)
			for i := len(state.History) - 1; i >= 0; i-- {
				add(state.History[i])
			}
			switch state.Latest.Status {
			case "success":
				row.BackupCheck = "passed"
				row.Attempt = "Recorded backup + check passed"
			case "failed":
				row.BackupCheck = "failed"
				if state.Latest.Phase == "cancelled" {
					row.BackupCheck = "cancelled"
				}
				row.State = "failed"
				row.Attempt = "Recorded attempt failed"
			case "running":
				row.BackupCheck = "running"
				row.State = "running"
				row.Attempt = "Running record; process not inspected"
			}
		}
		if reason := b.blocked(p); reason != "" {
			row.State = "blocked"
			row.Note = reason
			row.Activation = "Blocked in this mode"
		}
		data.Projects = append(data.Projects, row)
	}
	return data, nil
}
func (b *Backend) Review(id string) (statusdata.JobReview, error) {
	b.mu.Lock()
	b.epoch++
	generation := b.epoch
	b.pending = nil
	busy := b.active
	b.mu.Unlock()
	if busy {
		return statusdata.JobReview{}, errors.New("a manual job is already active")
	}
	if err := b.unchanged(); err != nil {
		return statusdata.JobReview{}, err
	}
	p, err := b.cfg.Project(id)
	if err != nil {
		return statusdata.JobReview{}, errors.New("project unavailable")
	}
	view := statusdata.JobReview{ID: id, Sources: len(p.Paths), Destination: b.cfg.Repository, ConfigFingerprint: b.digest[:12], Notifications: b.cfg.DiscordWebhookFile != ""}
	if !safeText(b.cfg.Repository) || len(b.cfg.Repository) > 512 {
		view.Reason = "Destination cannot be displayed safely; use a separately reviewed CLI configuration."
		return view, nil
	}
	if reason := b.blocked(p); reason != "" {
		view.Reason = reason
		return view, nil
	}
	if b.engine == nil {
		view.Reason = "Tools or configuration unavailable; open readiness (d), fix issues and restart."
		return view, nil
	}
	previous, err := b.engine.JobStatus(id)
	if err != nil {
		view.Reason = "Job record unreadable; inspect it separately."
		return view, nil
	}
	if previous.Latest.Status == "running" {
		view.Reason = "A running record exists; investigate it before starting another job."
		return view, nil
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return view, errors.New("cannot prepare confirmation")
	}
	view.Allowed = true
	view.Token = hex.EncodeToString(nonce[:])
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.epoch != generation || b.active {
		return statusdata.JobReview{}, errors.New("review cancelled or superseded")
	}
	b.pending = &pendingReview{view: view, previous: previous}
	return view, nil
}
func (b *Backend) Discard() { b.mu.Lock(); b.epoch++; b.pending = nil; b.mu.Unlock() }
func (b *Backend) Start(token string) (<-chan statusdata.JobEvent, error) {
	b.mu.Lock()
	if b.active || b.pending == nil || !b.pending.view.Allowed || token == "" || token != b.pending.view.Token {
		b.mu.Unlock()
		return nil, errors.New("review confirmation unavailable")
	}
	review := *b.pending
	b.pending = nil
	b.epoch++
	if err := b.unchanged(); err != nil {
		b.mu.Unlock()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	b.cancel = cancel
	b.active = true
	b.workers.Add(1)
	b.mu.Unlock()
	events := make(chan statusdata.JobEvent, 8)
	// Start immediately, not from a deferred UI command: cancellation always owns
	// a real worker and the CLI waits for engine cleanup even if the terminal exits.
	go func() {
		defer b.workers.Done()
		defer close(events)
		defer cancel()
		emit := func(event statusdata.JobEvent) {
			if event.Done {
				events <- event
			} else if len(events) < cap(events)-1 {
				events <- event
			}
		}
		emit(statusdata.JobEvent{Phase: "starting", Message: "Waiting for engine lock and review validation."})
		done := make(chan error, 1)
		go func() {
			done <- b.engine.RunReviewedFileJob(ctx, review.view.ID, review.previous.Latest.ID, b.unchanged)
		}()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastPhase := ""
		var runErr error
	loop:
		for {
			select {
			case runErr = <-done:
				break loop
			case <-ticker.C:
				state, err := b.engine.JobStatus(review.view.ID)
				if err == nil && state.Latest.ID != review.previous.Latest.ID && state.Latest.Status == "running" {
					phase := state.Latest.Phase
					if (phase == "backup" || phase == "check") && phase != lastPhase {
						emit(statusdata.JobEvent{Phase: phase, Message: "Engine phase recorded locally; no percentage estimate."})
						lastPhase = phase
					}
				}
			}
		}
		state, statusErr := b.engine.JobStatus(review.view.ID)
		event := statusdata.JobEvent{Done: true, Phase: "finished", State: "failed", Message: "Job failed or completion could not be verified. Inspect local job status."}
		changed := statusErr == nil && state.Latest.ID != "" && state.Latest.ID != review.previous.Latest.ID
		verified := changed && state.Latest.Status == "success" && state.Latest.BackupPassed && state.Latest.CheckPassed && state.LastSuccess.Equal(state.Latest.Finished)
		switch {
		case verified && runErr == nil:
			event.State = "passed"
			event.Message = "Backup + metadata check passed. Application recovery is separate."
		case verified:
			event.State = "warning"
			event.Message = "Backup/check recorded as passed; a completion or notification warning needs review."
		case ctx.Err() != nil:
			event.State = "cancelled"
			event.Message = "Worker stopped after cancellation; inspect local status before retrying."
		case statusErr == nil && !changed:
			event.State = "blocked"
			event.Message = "No new job started. Configuration, job record or operation lock changed."
		}
		b.mu.Lock()
		b.active = false
		b.cancel = nil
		b.mu.Unlock()
		emit(event)
	}()
	return events, nil
}
func (b *Backend) Cancel() {
	b.mu.Lock()
	cancel := b.cancel
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
func (b *Backend) Wait() { b.workers.Wait() }

// Ensure destination strings cannot hide terminal controls before review.
func safeText(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
