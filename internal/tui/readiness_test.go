package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"homevault/internal/statusdata"
)

func TestReadinessIsReadOnlyScrollableAndRedactsFailedRefresh(t *testing.T) {
	data := localData()
	r := &statusdata.Readiness{ConfigPath: "/generated/config.json", Destination: "/generated/repository", Fingerprint: "123456789abc"}
	for i := 0; i < 24; i++ {
		r.Checks = append(r.Checks, statusdata.ReadinessCheck{Name: "Metadata", State: "NOT CHECKED", Detail: "No runtime verification.", Action: "Review locally."})
	}
	data.Projects[0].Readiness = r
	for _, size := range [][2]int{{120, 32}, {60, 26}, {40, 18}} {
		jobs := &fakeJobs{}
		m, err := NewManualJobs(Options{Width: size[0], Height: size[1], NoColor: true}, data, func() (statusdata.Snapshot, error) { return statusdata.Snapshot{}, errors.New("PRIVATE-FAILURE") }, jobs)
		if err != nil {
			t.Fatal(err)
		}
		m = key(t, m, "d")
		if !strings.Contains(m.View(), "LOCAL READINESS") || m.readinessProject != "alpha" {
			t.Fatal("readiness did not open")
		}
		for i := 0; i < 80; i++ {
			m = key(t, m, "pgdown")
		}
		m = key(t, m, "b")
		m = key(t, m, "enter")
		m = key(t, m, "h")
		if jobs.starts != 0 || m.review != nil || m.historyProject != "" {
			t.Fatal("readiness allowed action or changed panels")
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
		if cmd == nil || strings.Contains(strings.Join(m.readinessLines(120), "\n"), "[NOT CHECKED]") {
			t.Fatal("refresh showed stale checks")
		}
		next, _ = m.Update(cmd())
		m = next.(Model)
		if strings.Contains(m.View(), "PRIVATE") || !strings.Contains(strings.Join(m.readinessLines(120), "\n"), "failed read") {
			t.Fatal("failed refresh leaked or overstated readiness")
		}
		m = key(t, m, "esc")
		if m.readinessProject != "" || m.selected != 0 {
			t.Fatal("return lost selection")
		}
	}
	ro, _ := NewReadOnly(Options{}, data, nil)
	ro = key(t, ro, "d")
	if ro.readinessProject != "" {
		t.Fatal("read-only coordinator gained engine diagnostics")
	}
}
