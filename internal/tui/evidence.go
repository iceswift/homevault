package tui

import (
	"fmt"
	"strings"
)

// Manual-job evidence is independent of action availability and source/recovery
// evidence. A blocked capture project may still have a recorded successful job.
func (m Model) backupLabel(p project) string {
	if m.loading || m.readFailed {
		return "Unknown (cached)"
	}
	switch p.backupCheck {
	case "passed":
		return m.ink("Recorded passed", "38;5;114")
	case "failed":
		return m.ink("Recorded failed", "38;5;210")
	case "cancelled":
		return m.ink("Recorded cancelled", "38;5;221")
	case "running":
		return "Running record"
	case "no-record":
		return "No local record"
	default:
		return "Unknown"
	}
}

func (m Model) jobSummary() string {
	passed := 0
	if !m.loading && !m.readFailed {
		for _, p := range m.projects {
			if p.backupCheck == "passed" {
				passed++
			}
		}
	}
	return fmt.Sprintf("BACKUP/CHECK %d/%d recorded passed | Source freshness: Unknown | Recovery: Not inspected", passed, len(m.projects))
}

func (m Model) jobDetail(p project, width int, compact bool) []string {
	action := "Review required; no workflow/timer gate loaded"
	if p.state == "blocked" {
		action = "Blocked in this mode"
	}
	stamp := "Unknown"
	if !p.uploaded.IsZero() {
		stamp = p.uploaded.In(m.location).Format("02 Jan 2006 15:04 MST") + " (" + m.age(p.uploaded) + " ago)"
	}
	if m.loading || m.readFailed {
		stamp = "Unknown - cached records are not current"
	}
	fields := [][2]string{
		{"Backup/check", m.backupLabel(p)},
		{"Source freshness", "Unknown - not evaluated"},
		{"Application recovery", "Not inspected"},
		{"Last recorded backup/check success", stamp},
		{"Manual action", action},
		{"Record scope", p.scope},
	}
	if !compact {
		fields = append(fields, [2]string{"Project ID", p.id}, [2]string{"Source type", p.kind}, [2]string{"Schedule", p.schedule})
	}
	lines := []string{m.accent(p.name), ""}
	for _, f := range fields {
		lines = append(lines, m.muted(f[0]), f[1], "")
	}
	lines = append(lines, p.note)
	if compact {
		lines = append(lines, "", "Enter: full details | h: history")
	}
	return m.wrap(strings.Join(lines, "\n"), width)
}

func (m Model) jobList(width, height int) []string {
	rows := m.filtered()
	if len(rows) == 0 {
		return m.wrap("No matching local projects. Esc clears the filter.", width)
	}
	lines := []string{}
	rowHeight := 1
	if width >= 64 {
		lines = []string{m.muted(pad("PROJECT", 18) + pad("BACKUP/CHECK", 20) + pad("SOURCE", 12) + "RECOVERY"), m.line(width)}
	} else {
		rowHeight = 5
	}
	count := max(1, (height-len(lines)-1)/rowHeight)
	start := min(max(0, m.selected-count+1), max(0, len(rows)-count))
	end := min(len(rows), start+count)
	for i := start; i < end; i++ {
		p := rows[i]
		cursor := "  "
		if i == m.selected {
			cursor = "> "
		}
		name := cursor + p.name
		if rowHeight == 1 {
			lines = append(lines, pad(name, 18)+pad(m.backupLabel(p), 20)+pad("Unknown", 12)+"Not inspected")
		} else {
			lines = append(lines, fit(name, width), fit("Backup/check: "+m.backupLabel(p), width), "Source freshness: Unknown", "Recovery: Not inspected", "")
		}
	}
	if start > 0 || end < len(rows) {
		lines = append(lines, fmt.Sprintf("Projects %d-%d of %d; arrows to scroll", start+1, end, len(rows)))
	}
	return lines
}
