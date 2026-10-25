package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/statusdata"
)

type fakeJobs struct {
	starts, cancels, discards int
	events                    chan statusdata.JobEvent
	blocked                   bool
}

func (f *fakeJobs) Review(id string) (statusdata.JobReview, error) {
	return statusdata.JobReview{ID: id, Token: "one-use", Allowed: !f.blocked, Reason: "Capture-required project", Sources: 1, Destination: "/generated/repository"}, nil
}
func (f *fakeJobs) Discard() { f.discards++ }
func (f *fakeJobs) Start(string) (<-chan statusdata.JobEvent, error) {
	f.starts++
	return f.events, nil
}
func (f *fakeJobs) Cancel() { f.cancels++ }
func (f *fakeJobs) Wait()   {}
func reviewUI(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(keyMessage("b"))
	if cmd == nil {
		t.Fatal("review not queued")
	}
	m = next.(Model)
	next, _ = m.Update(cmd())
	return next.(Model)
}
func TestManualJobsNeedTypedReviewAndWaitBeforeQuit(t *testing.T) {
	jobs := &fakeJobs{events: make(chan statusdata.JobEvent, 2)}
	m, _ := NewManualJobs(Options{NoColor: true, Width: 80, Height: 28}, localData(), func() (statusdata.Snapshot, error) { return localData(), nil }, jobs)
	m = reviewUI(t, m)
	m = key(t, m, "enter")
	if jobs.starts != 0 {
		t.Fatal("Enter alone started job")
	}
	m = key(t, m, "down")
	if m.selected != 0 {
		t.Fatal("review selection drifted")
	}
	m = key(t, m, "esc")
	if jobs.starts != 0 || jobs.discards == 0 {
		t.Fatal("cancelled review mutated job")
	}
	m = reviewUI(t, m)
	m = key(t, m, "alpha")
	next, wait := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if jobs.starts != 1 || !m.jobActive || wait == nil {
		t.Fatal("confirmed job did not start")
	}
	next, quit := m.Update(keyMessage("q"))
	m = next.(Model)
	if quit != nil || jobs.cancels != 1 || !m.quitAfterJob {
		t.Fatal("quit did not wait for cleanup")
	}
	jobs.events <- statusdata.JobEvent{Done: true, State: "cancelled", Message: "Worker cleanup finished"}
	_, quit = m.Update(wait())
	if quit == nil {
		t.Fatal("did not quit after worker completion")
	}
}
func TestBlockedReviewAndReadonlyModeHaveNoStart(t *testing.T) {
	jobs := &fakeJobs{blocked: true}
	m, _ := NewManualJobs(Options{NoColor: true, Width: 60, Height: 24}, localData(), nil, jobs)
	m = reviewUI(t, m)
	m = key(t, m, "alpha")
	m = key(t, m, "enter")
	if jobs.starts != 0 {
		t.Fatal("blocked review started")
	}
	ro, _ := NewReadOnly(Options{NoColor: true}, localData(), nil)
	ro = key(t, ro, "b")
	if ro.review != nil || ro.jobs != nil {
		t.Fatal("read-only boundary changed")
	}
}
