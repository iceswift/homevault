package tui

import (
	"fmt"
	"strings"
	"unicode"

	"homevault/internal/statusdata"
)

type Loader func() (statusdata.Snapshot, error)
type refreshResult struct {
	data statusdata.Snapshot
	err  error
}

func cleanDisplay(s string) string {
	runes := []rune{}
	for _, r := range s {
		if len(runes) >= 512 {
			break
		}
		if unicode.IsPrint(r) {
			runes = append(runes, r)
		} else {
			runes = append(runes, ' ')
		}
	}
	return string(runes)
}
func NewReadOnly(options Options, data statusdata.Snapshot, loader Loader) (Model, error) {
	m, err := newLayout(options)
	if err != nil {
		return m, err
	}
	m.live = true
	m.loader = loader
	if err = m.applyData(data); err != nil {
		return m, err
	}
	if options.Project != "" {
		found := false
		for i, p := range m.projects {
			if p.id == options.Project {
				m.selected = i
				found = true
			}
		}
		if !found {
			return m, fmt.Errorf("project ID is not in the loaded plan")
		}
	}
	return m, nil
}
func (m *Model) applyData(data statusdata.Snapshot) error {
	if data.ReadAt.IsZero() || len(data.Projects) > 128 {
		return fmt.Errorf("invalid local-record snapshot")
	}
	selected := ""
	visible := m.filtered()
	if m.selected >= 0 && m.selected < len(visible) {
		selected = visible[m.selected].id
	}
	projects := []project{}
	ids := map[string]bool{}
	for _, row := range data.Projects {
		if row.ID == "" || len(row.ID) > 64 || ids[row.ID] || cleanDisplay(row.ID) != row.ID {
			return fmt.Errorf("invalid local-record identity")
		}
		ids[row.ID] = true
		state := row.State
		switch state {
		case "passed", "stale", "failed", "inactive", "running", "unknown", "unverified":
		default:
			state = "unknown"
		}
		projects = append(projects, project{id: row.ID, name: cleanDisplay(row.Name), kind: cleanDisplay(row.Kind), state: state, attempt: cleanDisplay(row.Attempt), recovery: cleanDisplay(row.Recovery), schedule: cleanDisplay(row.Schedule), note: cleanDisplay(row.Note), activation: cleanDisplay(row.Activation), scope: cleanDisplay(row.RecordScope), projectName: cleanDisplay(row.ProjectName), captured: row.Captured, uploaded: row.Uploaded})
	}
	m.projects = projects
	m.now = data.ReadAt
	m.notifications = cleanDisplay(data.Notifications)
	m.selected = 0
	for i, p := range m.filtered() {
		if p.id == selected {
			m.selected = i
		}
	}
	return nil
}
func (m Model) summary() string {
	counts := map[string]int{}
	for _, p := range m.projects {
		state := p.state
		if m.readFailed || m.loading {
			state = "unknown"
		}
		counts[state]++
	}
	values := []string{}
	for _, state := range []string{"passed", "stale", "failed", "running", "inactive", "unverified", "unknown"} {
		if counts[state] > 0 {
			values = append(values, fmt.Sprintf("%d %s", counts[state], state))
		}
	}
	return fmt.Sprintf("LOCAL RECORDS  %d workflows | %s", len(m.projects), strings.Join(values, " / "))
}
func (m Model) liveHelp(width int) []string {
	return m.wrap("READ-ONLY LOCAL STATUS\n\nThis view reads explicitly selected plan/config metadata and local status records.\nNo engine object, backup tools, cloud API, Docker or systemd command is used.\nPassword, OAuth and webhook file contents are not opened.\n\nTab / 1 / 2: Overview or Projects\nUp/Down or j/k: select; Enter: full details\n/: filter; Esc: back/clear; PgUp/PgDn: scroll\nr: manually reload local metadata (close help first)\nq / Ctrl+C: quit\n\nNo automatic polling, state writes, locks, notifications or mutations.\nAn enabled plan gate does not prove an active systemd timer.\nMissing local records do not prove cloud backups are absent.\nLegacy jobs have unknown source age; no workflow freshness is inferred.\nRecovery evidence and cloud reachability are not inspected.\n\nNotifications: "+m.notifications+"\n\nOn refresh failure, previous values stay visible as cached Unknown records.\nThe footer is the last successful local read time, not a live heartbeat.", width)
}
