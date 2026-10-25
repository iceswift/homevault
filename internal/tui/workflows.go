package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) sourceLabel(p project) string {
	if m.loading || m.readFailed {
		return "Unknown (cached)"
	}
	switch p.sourceFreshness {
	case "fresh":
		return "Fresh at read"
	case "stale":
		return "Stale source"
	default:
		return "Unknown"
	}
}

func (m Model) workflowSummary() string {
	passed, stale, fresh := 0, 0, 0
	if !m.loading && !m.readFailed {
		for _, p := range m.projects {
			if p.backupCheck == "passed" {
				passed++
			}
			if p.sourceFreshness == "stale" {
				stale++
			}
			if p.sourceFreshness == "fresh" {
				fresh++
			}
		}
	}
	return fmt.Sprintf("%d workflows | Backup/check: %d recorded passed | Source: %d fresh / %d stale / %d unknown | Recovery: Not inspected", len(m.projects), passed, fresh, stale, len(m.projects)-fresh-stale)
}

func (m Model) workflowDetail(p project, width int, compact bool) []string {
	stamp := func(t time.Time) string {
		if m.loading || m.readFailed {
			return "Unknown - cached record"
		}
		if t.IsZero() {
			return "Unknown"
		}
		return t.In(m.location).Format("02 Jan 2006 15:04 MST") + " (" + m.age(t) + " ago)"
	}
	gate := p.activation
	if m.loading || m.readFailed {
		gate = "Unknown - cached plan"
	}
	fields := [][2]string{
		{"Backup/check (latest recorded attempt)", m.backupLabel(p)},
		{"Source freshness (at local read)", m.sourceLabel(p)},
		{"Application recovery", "Not inspected"},
		{"Workflow gate", gate},
		{"Schedule", "Unknown - systemd not inspected"},
		{"Record scope", p.scope},
		{"Source captured", stamp(p.captured)},
		{"Last recorded backup/check success", stamp(p.uploaded)},
		{"Latest attempt", p.attempt},
	}
	if !compact {
		fields = append(fields, [2]string{"Workflow ID", p.id}, [2]string{"Project ID", p.projectName}, [2]string{"Source type", p.kind}, [2]string{"Coordinator assessment", m.state(p)})
	}
	lines := []string{m.accent(p.name), ""}
	for _, f := range fields {
		lines = append(lines, m.muted(f[0]), f[1], "")
	}
	lines = append(lines, p.note, "", "Read-only: no backup, capture, service, timer or credential action.")
	if compact {
		lines = append(lines, "Enter: full details")
	}
	return m.wrap(strings.Join(lines, "\n"), width)
}

func (m Model) workflowList(width, height int) []string {
	rows := m.filtered()
	if len(rows) == 0 {
		return m.wrap("No matching workflows. Esc clears the filter.", width)
	}
	lines := []string{}
	rowHeight := 1
	if width >= 84 {
		lines = []string{m.muted(pad("WORKFLOW", 18) + pad("BACKUP/CHECK", 20) + pad("SOURCE", 15) + pad("PLAN GATE", 18) + "RECOVERY"), m.line(width)}
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
		gate := p.activation
		if m.loading || m.readFailed {
			gate = "Unknown (cached)"
		}
		if rowHeight == 1 {
			lines = append(lines, pad(cursor+p.name, 18)+pad(m.backupLabel(p), 20)+pad(m.sourceLabel(p), 15)+pad(gate, 18)+"Not inspected")
		} else {
			lines = append(lines, fit(cursor+p.name, width), fit("Backup/check: "+m.backupLabel(p), width), fit("Source: "+m.sourceLabel(p)+" | Recovery: Not inspected", width), fit("Gate: "+gate, width), "")
		}
	}
	if start > 0 || end < len(rows) {
		lines = append(lines, fmt.Sprintf("Workflows %d-%d of %d; arrows to scroll", start+1, end, len(rows)))
	}
	return lines
}
