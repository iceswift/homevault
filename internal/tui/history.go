package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/statusdata"
)

func (m *Model) handleHistory(msg tea.KeyMsg) (bool, tea.Cmd) {
	key := msg.String()
	if m.historyProject == "" {
		if key != "h" || m.searching || m.help {
			return false, nil
		}
		rows := m.filtered()
		if len(rows) > 0 {
			m.historyProject = rows[m.selected].id
			m.offset = 0
			m.notice = ""
		}
		return true, nil
	}
	defer m.clamp()
	switch key {
	case "esc", "h":
		m.historyProject = ""
		m.offset = 0
		m.notice = ""
	case "q", "ctrl+c":
		return true, tea.Quit
	case "up", "k":
		m.offset--
	case "down", "j":
		m.offset++
	case "pgup":
		m.offset -= max(1, m.height/2)
	case "pgdown":
		m.offset += max(1, m.height/2)
	case "r":
		if !m.loading && m.loader != nil {
			m.loading = true
			m.notice = ""
			return true, m.refreshCmd()
		}
	case "b", "enter":
		m.notice = "History is read-only. Esc returns to project actions."
	}
	return true, nil
}

func (m Model) historyLines(width int) []string {
	text := "LOCAL ATTEMPT HISTORY\nProject: " + m.historyProject + "\n\n"
	var selected *project
	for i := range m.projects {
		if m.projects[i].id == m.historyProject {
			selected = &m.projects[i]
			break
		}
	}
	if selected == nil {
		return m.wrap(text+"Project no longer exists in the loaded configuration. Esc returns.", width)
	}
	p := selected
	text += "Native engine records only; up to 32 completed attempts plus a running record.\nNo cloud snapshot lookup. Backup/check does not prove application recovery.\n"
	if m.loading || m.readFailed {
		return m.wrap(text+"\nHistory unavailable while local records are refreshing or cached after a failed read.\nPress r to retry; no current success is inferred.", width)
	}
	if !p.historyKnown {
		return m.wrap(text+"\nLocal history unavailable. This does not mean cloud backups are absent.", width)
	}
	stamp := func(t time.Time) string {
		if t.IsZero() {
			return "Unknown"
		}
		return t.In(m.location).Format("02 Jan 2006 15:04:05 MST")
	}
	text += "Last recorded backup/check success: " + stamp(p.uploaded) + "\n"
	if len(p.history) == 0 {
		return m.wrap(text+"\nNo local attempts recorded. Cloud backups and recovery were not inspected.", width)
	}
	for i, record := range p.history {
		result, phase := historyResult(record)
		duration := "Unknown"
		if !record.Started.IsZero() && !record.Finished.IsZero() && !record.Finished.Before(record.Started) {
			duration = record.Finished.Sub(record.Started).Round(time.Second).String()
		}
		text += fmt.Sprintf("\n%d. %s | %s\nAttempt: %s\nStarted: %s\nFinished: %s | duration %s\nBackup: %s | Metadata check: %s\n", i+1, result, phase, record.ID, stamp(record.Started), stamp(record.Finished), duration, recordedStep(record.BackupPassed), recordedStep(record.CheckPassed))
		if record.CapturePassed {
			text += "Capture: recorded passed; app recovery remains unverified\n"
		}
	}
	return m.wrap(strings.TrimSuffix(text, "\n"), width)
}

func recordedStep(passed bool) string {
	if passed {
		return "recorded passed"
	}
	return "not recorded passed"
}

func historyResult(r statusdata.JobRecord) (string, string) {
	phase := map[string]string{"capture": "capture", "backup": "backup", "check": "metadata check", "cancelled": "cancelled", "completed": "completed"}[r.Phase]
	if phase == "" {
		phase = "unknown phase"
	}
	switch r.Status {
	case "success":
		if r.BackupPassed && r.CheckPassed && r.Phase == "completed" && !r.Started.IsZero() && !r.Finished.Before(r.Started) {
			return "Recorded passed", phase
		}
	case "failed":
		if r.Phase == "cancelled" {
			return "Recorded cancelled", phase
		}
		return "Recorded failed", phase
	case "running":
		return "Running record (process not inspected)", phase
	}
	return "Unverified record", phase
}
