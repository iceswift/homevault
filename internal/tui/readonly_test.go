package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"homevault/internal/statusdata"
)

func localData() statusdata.Snapshot {
	return statusdata.Snapshot{ReadAt: clock, Notifications: "Disabled in plan", Projects: []statusdata.Project{
		{ID: "alpha", Name: "Alpha", Kind: "Selected files", State: "passed", Captured: clock.Add(-2 * time.Hour), Uploaded: clock.Add(-time.Hour), Attempt: "Recorded backup + check passed", Activation: "Enabled in plan; timer unknown", Schedule: "Unknown - not inspected", Recovery: "Unknown - not inspected", RecordScope: "Coordinator record"},
		{ID: "notes", Name: "Notes", Kind: "Completed export", State: "inactive", Uploaded: clock.Add(-3 * time.Hour), Attempt: "Legacy job passed", Activation: "Disabled in plan", RecordScope: "Legacy engine job only", Schedule: "Unknown - not inspected", Recovery: "Unknown - not inspected"},
	}}
}
func TestReadOnlyNavigationAndNoAutomaticLoading(t *testing.T) {
	calls := 0
	data := localData()
	m, err := NewReadOnly(Options{Width: 120, Height: 32, NoColor: true}, data, func() (statusdata.Snapshot, error) { calls++; return data, nil })
	if err != nil {
		t.Fatal(err)
	}
	if m.Init() != nil || calls != 0 {
		t.Fatal("automatic IO started")
	}
	view := m.View()
	if strings.Contains(view, "DEMO") || strings.Contains(view, "SAMPLE") || !strings.Contains(view, "READ ONLY") || !strings.Contains(view, "last read") {
		t.Fatal("mode not labelled")
	}
	for _, op := range []string{"b", "c", "d", "a", "s", "p"} {
		m = key(t, m, op)
		if !strings.Contains(m.View(), "No operation was started") || calls != 0 {
			t.Fatal("read-only action escaped its boundary")
		}
	}
	m = key(t, m, "/")
	m = key(t, m, "notes")
	m = key(t, m, "enter")
	m = key(t, m, "enter")
	if !m.details || !strings.Contains(m.View(), "Legacy engine job only") || !strings.Contains(m.View(), "Unknown") {
		t.Fatal("record provenance or unknown source age missing")
	}
}
func TestRefreshPreservesSelectionAndDowngradesCachedData(t *testing.T) {
	data := localData()
	fail := false
	calls := 0
	load := func() (statusdata.Snapshot, error) {
		calls++
		if fail {
			return statusdata.Snapshot{}, fmt.Errorf("PRIVATE_TOKEN must never reach display")
		}
		next := data
		next.ReadAt = clock.Add(time.Minute)
		next.Projects = []statusdata.Project{data.Projects[1], data.Projects[0]}
		return next, nil
	}
	m, _ := NewReadOnly(Options{Width: 120, Height: 32, NoColor: true, Project: "alpha"}, data, load)
	next, cmd := m.Update(keyMessage("r"))
	m = next.(Model)
	if cmd == nil || !m.loading || m.filtered()[m.selected].state != "unknown" {
		t.Fatal("refresh did not make cached status explicit")
	}
	if _, duplicate := m.Update(keyMessage("r")); duplicate != nil {
		t.Fatal("duplicate read started")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if calls != 1 || m.loading || m.readFailed || m.filtered()[m.selected].id != "alpha" || !m.now.Equal(clock.Add(time.Minute)) {
		t.Fatal("refresh lost identity or read clock")
	}
	fail = true
	next, cmd = m.Update(keyMessage("r"))
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if !m.readFailed || !strings.Contains(m.View(), "REFRESH FAILED") || strings.Contains(m.View(), "PRIVATE_TOKEN") {
		t.Fatal("failed refresh not safely displayed")
	}
	for _, p := range m.filtered() {
		if p.state != "unknown" {
			t.Fatal("cached green status survived failed read")
		}
	}
	if !m.now.Equal(clock.Add(time.Minute)) {
		t.Fatal("failed read advanced clock")
	}
	fail = false
	next, cmd = m.Update(keyMessage("r"))
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.readFailed || m.filtered()[m.selected].state != "passed" {
		t.Fatal("successful refresh did not restore validated records")
	}
}
func TestLiveFramesAndDisplaySanitization(t *testing.T) {
	data := localData()
	data.Projects[0].Name = "Alpha\x1b[31m\nunsafe"
	for _, size := range [][2]int{{120, 32}, {80, 24}, {60, 26}, {40, 18}, {18, 6}} {
		m, err := NewReadOnly(Options{Width: size[0], Height: size[1], NoColor: true, ASCII: true}, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		view := m.View()
		if strings.Contains(view, "\x1b") || strings.Contains(view, "DEMO ONLY") {
			t.Fatal("unsafe output or wrong mode")
		}
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("height overflow")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("width overflow")
			}
		}
	}
	m, err := NewReadOnly(Options{Width: 120, Height: 40, NoColor: true, Details: true, Location: time.FixedZone("ICT", 7*3600)}, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.View(), "12:00 ICT") {
		t.Fatal("display timezone ignored")
	}
}

func keyMessage(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
