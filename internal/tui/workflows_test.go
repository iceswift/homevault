package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"homevault/internal/statusdata"
)

func TestWorkflowEvidenceAndContextStayReadOnly(t *testing.T) {
	data := localData()
	data.Projects[0].BackupCheck = "passed"
	data.Projects[0].SourceFreshness = "stale"
	data.Projects[0].Activation = "Disabled in plan"
	data.Projects[0].State = "inactive"
	for _, size := range [][2]int{{156, 42}, {120, 32}, {80, 24}, {60, 26}, {40, 18}} {
		m, err := NewReadOnly(Options{Workflows: true, Context: "production", Width: size[0], Height: size[1], NoColor: true}, data, func() (statusdata.Snapshot, error) { return statusdata.Snapshot{}, errors.New("PRIVATE") })
		if err != nil {
			t.Fatal(err)
		}
		if m.jobs != nil || !strings.Contains(m.View(), "WORKFLOWS - READ ONLY") {
			t.Fatal("mode lost read-only boundary")
		}
		text := strings.Join(m.workflowDetail(m.projects[0], 120, false), "\n")
		for _, label := range []string{"Recorded passed", "Stale source", "Not inspected", "Disabled in plan", "systemd not inspected"} {
			if !strings.Contains(text, label) {
				t.Fatalf("missing independent evidence: %s", label)
			}
		}
		for _, op := range []string{"b", "c", "d", "a", "s", "p"} {
			m = key(t, m, op)
		}
		if m.review != nil || m.readinessProject != "" || m.jobActive {
			t.Fatal("action enabled in workflow view")
		}
		view := m.View()
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("height overflow")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("width overflow")
			}
		}
		next, cmd := m.Update(keyMessage("r"))
		m = next.(Model)
		if cmd == nil {
			t.Fatal("refresh missing")
		}
		next, _ = m.Update(cmd())
		m = next.(Model)
		text = strings.Join(m.workflowDetail(m.projects[0], 120, false), "\n")
		if strings.Contains(text, "Recorded passed") || strings.Contains(text, "Stale source") || strings.Contains(m.View(), "PRIVATE") {
			t.Fatal("failed refresh retained evidence or exposed error")
		}
	}
}
