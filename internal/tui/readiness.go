package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleReadiness(msg tea.KeyMsg) (bool, tea.Cmd) {
	key := msg.String()
	if m.readinessProject == "" {
		if m.historyProject != "" {
			return false, nil
		}
		if key != "d" || m.searching || m.help {
			return false, nil
		}
		rows := m.filtered()
		if len(rows) > 0 {
			m.readinessProject = rows[m.selected].id
			m.offset = 0
			m.notice = ""
		}
		return true, nil
	}
	defer m.clamp()
	switch key {
	case "esc", "d":
		m.readinessProject = ""
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
		m.notice = "Readiness is read-only. Esc returns to project actions."
	}
	return true, nil
}

func (m Model) readinessLines(width int) []string {
	text := "LOCAL READINESS / METADATA ONLY\nProject: " + m.readinessProject + "\n\n"
	if m.loading || m.readFailed {
		return m.wrap(text+"Observations unavailable while refreshing or after a failed read. Press r to retry.\nCached checks are not presented as current readiness.", width)
	}
	for _, p := range m.projects {
		if p.id != m.readinessProject {
			continue
		}
		if p.readiness == nil {
			return m.wrap(text+"No local readiness observations available. No readiness is inferred.", width)
		}
		r := p.readiness
		text += "Config: " + r.ConfigPath + "\nFingerprint: sha256 " + r.Fingerprint + "\nDestination: " + r.Destination + "\n\n"
		text += "No tool execution, credential-content reads, cloud requests, write probes or repairs.\nOBSERVED means metadata found, not backup-ready. Context is an operator label only.\n"
		for _, c := range r.Checks {
			state := c.State
			switch state {
			case "OBSERVED", "ATTENTION", "BLOCKED", "NOT CHECKED":
			default:
				state = "NOT CHECKED"
			}
			text += "\n[" + state + "] " + c.Name + "\n" + c.Detail + "\nNext: " + c.Action + "\n"
		}
		return m.wrap(strings.TrimSuffix(text, "\n"), width)
	}
	return m.wrap(text+"Project no longer exists in loaded configuration. Esc returns.", width)
}
