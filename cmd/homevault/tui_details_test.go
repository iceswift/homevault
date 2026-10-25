package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/config"
	"homevault/internal/engine"
)

func TestTUIBackupConfirmationDoesNotStartOrChangeProject(t *testing.T) {
	m := model{cfg: config.Config{Projects: []config.Project{{Name: "one"}, {Name: "two"}}}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = updated.(model)
	if cmd != nil || m.busy || !m.confirmBackup || !strings.Contains(m.status, "one?") {
		t.Fatal("backup started without confirmation")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if cmd != nil || m.confirmBackup || m.selected != 0 || !strings.Contains(m.status, "cancelled") {
		t.Fatal("confirmation allowed a different project to be selected")
	}
	m.cfg.Projects[0].CaptureRequired = true
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd != nil || updated.(model).confirmBackup {
		t.Fatal("capture-only project offered interactive backup")
	}
}

func TestTUIWelcomeAndWrappedPaging(t *testing.T) {
	m := model{width: 32, height: 24}
	if !strings.Contains(m.View(), "No projects configured") {
		t.Fatal("empty configuration has no guidance")
	}
	for _, line := range m.detailLines() {
		if len([]rune(line)) > 31 {
			t.Fatal("detail line exceeds terminal width", line)
		}
	}
	m.status = strings.Repeat("long detail ", 100)
	m.offset = 10000
	m.clampOffset()
	if m.offset != len(m.detailLines())-m.pageSize() {
		t.Fatal("paging does not account for wrapping")
	}
}

func TestTUIJobHistoryDoesNotExposeRepositoryIdentity(t *testing.T) {
	s := engine.JobStatus{Project: "example", RepositoryID: "private-repository-fingerprint", Latest: engine.JobAttempt{Status: "never-run"}}
	text := formatJobHistory(s)
	if strings.Contains(text, s.RepositoryID) || !strings.Contains(text, "never") || !strings.Contains(text, "does not prove") {
		t.Fatal("incorrect history summary", text)
	}
	s.History = []engine.JobAttempt{{Status: "failed", Started: time.Unix(1, 0)}, {Status: "success", Started: time.Unix(2, 0)}}
	text = formatJobHistory(s)
	if strings.Index(text, "1970-01-01T00:00:02Z") > strings.Index(text, "1970-01-01T00:00:01Z") {
		t.Fatal("history order wrong")
	}
}

func TestTUIPagingBoundsAndRecoveryGuide(t *testing.T) {
	m := model{height: 18, status: strings.Repeat("line\n", 100)}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(model)
	if m.offset == 0 || !strings.Contains(m.View(), "of 101") {
		t.Fatal("page down failed")
	}
	for i := 0; i < 100; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = updated.(model)
	}
	if m.offset > 101-m.pageSize() {
		t.Fatal("scroll exceeded content")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(model)
	if m.offset != 0 || !strings.Contains(m.status, "Never restore over production") {
		t.Fatal("guide absent")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if updated.(model).height != 24 {
		t.Fatal("resize not handled")
	}
}
