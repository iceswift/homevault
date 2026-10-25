package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestManualEvidenceSeparatesSuccessFreshnessRecoveryAndActions(t *testing.T) {
	data := localData()
	data.Projects[0].BackupCheck = "passed"
	data.Projects[0].State = "blocked"
	for _, size := range [][2]int{{156, 42}, {120, 32}, {80, 24}, {60, 26}, {40, 18}} {
		m, err := NewManualJobs(Options{Width: size[0], Height: size[1], NoColor: true, Context: "lab"}, data, nil, &fakeJobs{})
		if err != nil {
			t.Fatal(err)
		}
		view := m.View()
		if !strings.Contains(view, "LAB (operator label)") || strings.Contains(view, "Unverified") || !strings.Contains(view, "Recorded passed") {
			t.Fatal("success still ambiguous or lab label missing")
		}
		detail := strings.Join(m.jobDetail(m.projects[0], 100, false), "\n")
		for _, label := range []string{"Backup/check", "Recorded passed", "Source freshness", "Unknown - not evaluated", "Application recovery", "Not inspected", "Blocked in this mode", "Last recorded backup/check success"} {
			if !strings.Contains(detail, label) {
				t.Fatalf("missing evidence dimension: %s", label)
			}
		}
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("height overflow")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("width overflow")
			}
		}
		m.readFailed = true
		if strings.Contains(m.backupLabel(m.projects[0]), "passed") || strings.Contains(strings.Join(m.jobDetail(m.projects[0], 100, false), "\n"), "Recorded passed") || strings.Contains(m.jobSummary(), "1/2 recorded passed") {
			t.Fatal("cached success treated as current evidence")
		}
	}
}

func TestManualContextIsExplicitAndNeverGuessed(t *testing.T) {
	data := localData()
	data.Projects[0].Name = "synthetic-lab-production"
	m, err := NewManualJobs(Options{}, data, nil, &fakeJobs{})
	if err != nil || !strings.Contains(m.View(), "CONTEXT UNKNOWN") {
		t.Fatal("context guessed from project name")
	}
	if _, err := NewManualJobs(Options{Context: "invalid"}, data, nil, &fakeJobs{}); err == nil {
		t.Fatal("invalid context accepted")
	}
	prod, _ := NewManualJobs(Options{Context: "production"}, data, nil, &fakeJobs{})
	if !strings.Contains(prod.View(), "PRODUCTION (operator label)") {
		t.Fatal("explicit production label missing")
	}
}
