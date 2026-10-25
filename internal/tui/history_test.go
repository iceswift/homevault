package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"homevault/internal/statusdata"
)

func historyData() statusdata.Snapshot {
	data := localData()
	data.Projects[0].HistoryKnown = true
	for i := 0; i < 32; i++ {
		data.Projects[0].History = append(data.Projects[0].History, statusdata.JobRecord{ID: fmt.Sprintf("%024x", i+1), Status: "failed", Phase: "check", Started: clock.Add(-time.Hour), Finished: clock.Add(-time.Hour + 7*time.Second), BackupPassed: true})
	}
	return data
}

func TestHistoryNavigationDoesNotStartJobsAndStaysBounded(t *testing.T) {
	for _, size := range [][2]int{{120, 32}, {80, 24}, {60, 26}, {40, 18}, {30, 12}} {
		jobs := &fakeJobs{}
		m, err := NewManualJobs(Options{Width: size[0], Height: size[1], NoColor: true}, historyData(), nil, jobs)
		if err != nil {
			t.Fatal(err)
		}
		m = key(t, m, "h")
		for i := 0; i < 100; i++ {
			m = key(t, m, "pgdown")
		}
		if m.selected != 0 || m.historyProject != "alpha" || jobs.starts != 0 {
			t.Fatal("history moved project or started job")
		}
		m = key(t, m, "b")
		m = key(t, m, "enter")
		if jobs.starts != 0 || m.review != nil {
			t.Fatal("history allowed action")
		}
		lines := strings.Split(m.View(), "\n")
		if len(lines) > size[1] {
			t.Fatal("history height overflow")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("history width overflow")
			}
		}
		if size[0] >= 40 && !strings.Contains(m.View(), "32.") {
			t.Fatal("last history attempt unreachable")
		}
		m = key(t, m, "esc")
		if m.historyProject != "" || m.offset != 0 || m.selected != 0 {
			t.Fatal("return lost selection")
		}
	}
}

func TestHistoryRefreshFailureHidesCachedSuccessAndPreservesIdentity(t *testing.T) {
	data := historyData()
	m, _ := NewManualJobs(Options{Width: 120, Height: 32, NoColor: true}, data, func() (statusdata.Snapshot, error) {
		return statusdata.Snapshot{}, errors.New("PRIVATE must stay hidden")
	}, &fakeJobs{})
	m = key(t, m, "h")
	text := strings.Join(m.historyLines(120), "\n")
	if !strings.Contains(text, "duration 7s") || !strings.Contains(text, "Recorded failed") || !strings.Contains(text, "Last recorded backup/check success:") || !strings.Contains(text, "Metadata check: not recorded passed") {
		t.Fatal("failure erased prior success or mislabeled step")
	}
	next, cmd := m.Update(keyMessage("r"))
	m = next.(Model)
	if cmd == nil || strings.Contains(strings.Join(m.historyLines(120), "\n"), "1. Recorded") {
		t.Fatal("refresh showed cached history as current")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	view := m.View()
	if !m.readFailed || strings.Contains(view, "PRIVATE") || strings.Contains(strings.Join(m.historyLines(120), "\n"), "1. Recorded") {
		t.Fatal("failed refresh exposed stale success or private error")
	}
	data.Projects[0], data.Projects[1] = data.Projects[1], data.Projects[0]
	if err := m.applyData(data); err != nil || m.historyProject != "alpha" {
		t.Fatal("reordering changed history identity")
	}
	m.loading, m.readFailed = false, false
	data.Projects = data.Projects[:1]
	m.applyData(data)
	if !strings.Contains(m.View(), "no longer exists") {
		t.Fatal("removed project showed another project's history")
	}
}

func TestHistoryUnavailableAndInvalidSuccessAreConservative(t *testing.T) {
	m, _ := NewManualJobs(Options{}, localData(), nil, &fakeJobs{})
	m = key(t, m, "h")
	if !strings.Contains(strings.Join(m.historyLines(120), "\n"), "history unavailable") {
		t.Fatal("missing history treated as empty")
	}
	r := statusdata.JobRecord{Status: "success", Phase: "completed", BackupPassed: true, CheckPassed: true}
	if result, _ := historyResult(r); result != "Unverified record" {
		t.Fatal("incomplete success accepted")
	}
	ro, _ := NewReadOnly(Options{}, localData(), nil)
	ro = key(t, ro, "h")
	if ro.historyProject != "" {
		t.Fatal("read-only coordinator mode gained native history")
	}
}
