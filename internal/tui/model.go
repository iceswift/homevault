// Package tui displays synthetic previews, read-only local records or explicitly reviewed manual jobs.
// Navigation and rendering are separate from the optional Jobs capability.
package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"homevault/internal/statusdata"
)

var clock = time.Date(2026, 10, 25, 6, 0, 0, 0, time.UTC)

type project struct {
	id, name, kind, state, attempt, recovery, schedule, note string
	activation, scope, projectName                           string
	captured, uploaded                                       time.Time
	readiness                                                *statusdata.Readiness
	backupCheck                                              string
	historyKnown                                             bool
	history                                                  []statusdata.JobRecord
}

func fixtures() []project {
	return []project{
		{id: "shop", name: "Shop demo", kind: "SQLite + media", state: "passed", captured: clock.Add(-2 * time.Hour), uploaded: clock.Add(-116 * time.Minute), attempt: "Backup + check passed", recovery: "Sample drill: 24 Oct, 08:00 UTC", schedule: "Not configured", note: "Review a restore separately from backup success."},
		{id: "notes", name: "Notes demo", kind: "Completed database export", state: "stale", captured: clock.Add(-50 * time.Hour), uploaded: clock.Add(-115 * time.Minute), attempt: "Backup + check passed", recovery: "Not tested", schedule: "Not configured", note: "A recent upload contains old source data. Source age exceeds the sample 36h limit."},
		{id: "files", name: "Files demo", kind: "Selected directories", state: "failed", captured: clock.Add(-5 * time.Hour), uploaded: clock.Add(-28 * time.Hour), attempt: "Repository check failed", recovery: "Not tested", schedule: "Not configured", note: "The failed attempt does not replace the earlier verified upload."},
		{id: "photos", name: "Photos demo", kind: "Selected directories", state: "inactive", attempt: "Never run", recovery: "Unknown", schedule: "Not activated", note: "No verified backup is recorded. Inactive is not a successful backup."},
	}
}

type Options struct {
	Width, Height           int
	ASCII, NoColor, Details bool
	Page, Project           string
	Context                 string
	Location                *time.Location
}

type Model struct {
	width, height, page, selected, offset    int
	ascii, noColor, details, searching, help bool
	query, notice                            string
	projects                                 []project
	live, loading, readFailed                bool
	now                                      time.Time
	location                                 *time.Location
	loader                                   Loader
	notifications                            string
	jobs                                     Jobs
	review                                   *statusdata.JobReview
	reviewLoading                            bool
	reviewGeneration                         uint64
	typed                                    string
	jobActive, jobPanel, quitAfterJob        bool
	jobID, jobPhase, jobState, jobMessage    string
	events                                   jobEvents
	historyProject                           string
	contextLabel                             string
	readinessProject                         string
}

func newLayout(options Options) (Model, error) {
	m := Model{width: options.Width, height: options.Height, ascii: options.ASCII, noColor: options.NoColor, details: options.Details, now: clock, location: options.Location}
	switch options.Context {
	case "", "unknown":
		m.contextLabel = "CONTEXT UNKNOWN"
	case "lab":
		m.contextLabel = "LAB (operator label)"
	case "production":
		m.contextLabel = "PRODUCTION (operator label)"
	default:
		return m, fmt.Errorf("context must be lab, production or unknown")
	}
	if m.width == 0 {
		m.width = 100
	}
	if m.height == 0 {
		m.height = 30
	}
	if m.width < 1 || m.height < 1 || m.width > 500 || m.height > 200 {
		return m, fmt.Errorf("terminal dimensions must be 1..500 columns and 1..200 rows")
	}
	switch options.Page {
	case "", "overview":
	case "projects":
		m.page = 1
	default:
		return m, fmt.Errorf("page must be overview or projects")
	}
	if m.location == nil {
		m.location = time.UTC
	}
	return m, nil
}

func New(options Options) (Model, error) {
	m, err := newLayout(options)
	if err != nil {
		return m, err
	}
	m.projects = fixtures()
	if options.Project != "" {
		found := false
		for i, p := range m.projects {
			if p.id == options.Project {
				m.selected = i
				found = true
			}
		}
		if !found {
			return m, fmt.Errorf("sample project must be shop, notes, files or photos")
		}
	}
	return m, nil
}

func (m Model) Init() tea.Cmd { return nil }
func (m Model) filtered() []project {
	var result []project
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p.name+" "+p.kind), strings.ToLower(m.query)) {
			if m.live && (m.readFailed || m.loading) {
				p.state = "unknown"
				p.attempt = "Unknown - cached record"
				p.note = "Cached values are not current; refresh to validate local records. " + p.note
			}
			result = append(result, p)
		}
	}
	return result
}
func (m *Model) clamp() {
	n := len(m.filtered())
	m.selected = min(max(0, m.selected), max(0, n-1))
	m.offset = max(0, m.offset)
	if m.readinessProject != "" || m.historyProject != "" || m.help || m.details && n > 0 {
		_, _, height := m.layout()
		var lines []string
		if m.readinessProject != "" {
			lines = m.readinessLines(max(1, m.width))
		} else if m.historyProject != "" {
			lines = m.historyLines(max(1, m.width))
		} else if m.help {
			lines = m.helpLines(max(1, m.width))
		} else {
			lines = m.detail(m.filtered()[m.selected], max(1, m.width), false)
		}
		if len(lines) <= height {
			m.offset = 0
		} else {
			m.offset = min(m.offset, max(0, len(lines)-max(0, height-1)))
		}
	} else {
		m.offset = 0
	}
}
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case jobReviewResult:
		if !m.reviewLoading || msg.generation != m.reviewGeneration {
			return m, nil
		}
		m.reviewLoading = false
		if msg.err != nil {
			m.jobs.Discard()
			m.notice = "Review unavailable; refresh/restart and inspect configuration separately."
		} else {
			r := msg.review
			r.Destination = cleanDisplay(r.Destination)
			r.Reason = cleanDisplay(r.Reason)
			m.review = &r
			m.typed = ""
		}
		return m, nil
	case jobEventMsg:
		if !m.jobActive {
			return m, nil
		}
		if msg.event.Phase == "backup" || msg.event.Phase == "check" || msg.event.Phase == "starting" {
			m.jobPhase = msg.event.Phase
		}
		m.jobMessage = cleanDisplay(msg.event.Message)
		if !msg.event.Done {
			return m, waitJob(m.events)
		}
		m.jobActive = false
		m.jobState = msg.event.State
		m.jobPhase = "finished"
		if m.quitAfterJob {
			return m, tea.Quit
		}
		m.loading = true
		return m, m.refreshCmd()

	case refreshResult:
		if !m.live || !m.loading {
			return m, nil
		}
		m.loading = false
		if msg.err != nil || m.applyData(msg.data) != nil {
			m.readFailed = true
			m.notice = "Refresh failed; cached records are not current."
		} else {
			m.readFailed = false
			m.notice = "Local records refreshed. No cloud or service checks were made."
			if m.jobs != nil {
				m.notice = "Local job records refreshed."
			}
		}
		m.clamp()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = max(1, msg.Width)
		m.height = max(1, msg.Height)
		m.offset = 0
	case tea.KeyMsg:
		if handled, cmd := m.handleJobs(msg); handled {
			return m, cmd
		}
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.searching {
			switch key {
			case "esc":
				m.query = ""
				m.searching = false
			case "enter":
				m.searching = false
			case "backspace", "ctrl+h":
				r := []rune(m.query)
				if len(r) > 0 {
					m.query = string(r[:len(r)-1])
				}
			default:
				if msg.Type == tea.KeyRunes {
					runes := []rune(m.query)
					for _, r := range msg.Runes {
						if len(runes) < 80 && unicode.IsPrint(r) {
							runes = append(runes, r)
						}
					}
					m.query = string(runes)
				}
			}
			m.selected = 0
			m.offset = 0
			m.clamp()
			return m, nil
		}
		if key == "q" {
			return m, tea.Quit
		}
		if m.help {
			if key == "esc" || key == "?" {
				m.help = false
				m.offset = 0
			} else if key == "pgdown" || key == "down" {
				m.offset += max(1, m.height/3)
			} else if key == "pgup" || key == "up" {
				m.offset = max(0, m.offset-max(1, m.height/3))
			}
			m.clamp()
			return m, nil
		}
		m.notice = ""
		switch key {
		case "tab", "shift+tab", "1", "2":
			if key == "1" {
				m.page = 0
			} else if key == "2" {
				m.page = 1
			} else {
				m.page = 1 - m.page
			}
			m.details = false
			m.offset = 0
		case "up", "k":
			if m.details {
				m.offset = max(0, m.offset-1)
			} else {
				m.selected--
			}
		case "down", "j":
			if m.details {
				m.offset++
			} else {
				m.selected++
			}
		case "pgdown":
			if m.details {
				m.offset += max(1, m.height/2)
			} else {
				m.selected += max(1, m.height/4)
			}
		case "pgup":
			if m.details {
				m.offset = max(0, m.offset-max(1, m.height/2))
			} else {
				m.selected -= max(1, m.height/4)
			}
		case "enter":
			if len(m.filtered()) > 0 {
				m.details = true
				m.offset = 0
			}
		case "esc":
			if m.details {
				m.details = false
			} else {
				m.query = ""
			}
			m.offset = 0
		case "/":
			m.searching = true
			m.details = false
			m.offset = 0
		case "?":
			m.help = true
			m.offset = 0
		case "r":
			if m.live && m.loader != nil {
				if m.loading {
					return m, nil
				}
				m.loading = true
				m.readFailed = false
				load := m.loader
				return m, func() tea.Msg { data, err := load(); return refreshResult{data: data, err: err} }
			}
			m.notice = "Prototype only. No job started; no data changed."
		case "b", "c", "d", "a", "s", "p":
			if m.live {
				m.notice = "Read-only view. No operation was started."
			} else {
				m.notice = "Prototype only. No job started; no data changed."
			}
		}
		m.clamp()
	}
	return m, nil
}

func (m Model) ink(text, code string) string {
	if m.noColor {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
func (m Model) muted(text string) string  { return m.ink(text, "38;5;245") }
func (m Model) accent(text string) string { return m.ink(text, "38;5;81") }
func (m Model) line(width int) string {
	if m.ascii {
		return strings.Repeat("-", max(0, width))
	}
	return strings.Repeat("─", max(0, width))
}
func fit(text string, width int) string { return ansi.Truncate(text, max(0, width), "") }
func pad(text string, width int) string {
	return fit(text, width) + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}
func (m Model) wrap(text string, width int) []string {
	return strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
}
func (m Model) age(value time.Time) string {
	if value.IsZero() {
		if m.live {
			return "unknown"
		}
		return "never"
	}
	if value.After(m.now) {
		return "future"
	}
	minutes := int(m.now.Sub(value).Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh", minutes/60)
}
func (m Model) state(p project) string {
	if m.jobs != nil {
		return m.backupLabel(p)
	}
	labels := map[string]string{"passed": "Passed", "stale": "Stale source", "failed": "Check failed", "inactive": "Not activated"}
	colors := map[string]string{"passed": "38;5;114", "stale": "38;5;221", "failed": "38;5;210", "inactive": "38;5;245"}
	labels["blocked"] = "Blocked"
	colors["blocked"] = "38;5;221"
	labels["unknown"] = "Unknown"
	labels["unverified"] = "Unverified"
	labels["running"] = "Running record"
	colors["unknown"] = "38;5;221"
	colors["unverified"] = "38;5;221"
	colors["running"] = "38;5;81"
	if m.live {
		labels["failed"] = "Failed"
		labels["inactive"] = "Disabled in plan"
	}
	return m.ink(labels[p.state], colors[p.state])
}
func (m Model) detail(p project, width int, compact bool) []string {
	if m.jobs != nil {
		return m.jobDetail(p, width, compact)
	}
	lines := []string{m.accent(p.name), m.state(p), ""}
	stamp := func(t time.Time) string {
		if t.IsZero() {
			if m.live {
				return "Unknown"
			}
			return "Never"
		}
		return t.In(m.location).Format("02 Jan, 15:04 MST") + " (" + m.age(t) + " ago)"
	}
	fields := [][2]string{{"Source captured", stamp(p.captured)}, {"Verified upload", stamp(p.uploaded)}, {"Latest attempt", p.attempt}, {"Recovery drill", p.recovery}, {"Schedule", p.schedule}}
	if m.live {
		fields[1][0] = "Recorded verified upload"
		fields = append([][2]string{{"Record scope", p.scope}, {"Workflow gate", p.activation}}, fields...)
	}
	if !compact {
		fields = append([][2]string{{"Source type", p.kind}}, fields...)
		if m.live {
			fields = append([][2]string{{"Project ID", p.projectName}}, fields...)
		}
	}
	for _, field := range fields {
		if compact {
			lines = append(lines, m.muted(field[0]), field[1])
		} else {
			lines = append(lines, m.muted(field[0]), field[1], "")
		}
	}
	lines = append(lines, "", p.note)
	if compact {
		lines = append(lines, "", "Enter: full details")
	}
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, m.wrap(line, width)...)
	}
	return wrapped
}
func (m Model) list(width, height int) []string {
	if m.jobs != nil {
		return m.jobList(width, height)
	}
	projects := m.filtered()
	if len(projects) == 0 {
		if m.live {
			return m.wrap("No matching local projects. Esc clears the filter; / edits it.", width)
		}
		return m.wrap("No matching sample projects. Esc clears the filter; / edits it.", width)
	}
	var lines []string
	rowHeight := 1
	if m.width < 80 {
		rowHeight = 3
		lines = []string{m.muted("PROJECT / SOURCE AGE / UPLOAD AGE"), ""}
	} else {
		lines = []string{m.muted(pad("PROJECT", 23) + pad("SOURCE AGE", 12) + pad("UPLOAD AGE", 12) + "RESULT"), m.muted(m.line(width))}
	}
	count := max(1, (height-len(lines)-1)/rowHeight)
	start := min(max(0, m.selected-count+1), max(0, len(projects)-count))
	end := min(len(projects), start+count)
	for i := start; i < end; i++ {
		p := projects[i]
		cursor := "  "
		if i == m.selected {
			cursor = "> "
			if !m.ascii {
				cursor = "› "
			}
		}
		name := cursor + p.name
		if i == m.selected {
			name = m.accent(name)
		}
		if rowHeight == 1 {
			lines = append(lines, pad(name, 23)+pad(m.age(p.captured), 12)+pad(m.age(p.uploaded), 12)+m.state(p))
		} else {
			lines = append(lines, name+" / "+m.state(p), m.muted("  source "+m.age(p.captured)+" | upload "+m.age(p.uploaded)), "")
		}
	}
	if start > 0 || end < len(projects) {
		lines = append(lines, m.muted(fmt.Sprintf("Projects %d-%d of %d; arrows to scroll", start+1, end, len(projects))))
	}
	return lines
}
func window(lines []string, offset, height int) []string {
	if height <= 0 {
		return nil
	}
	if len(lines) <= height {
		return lines
	}
	content := max(0, height-1)
	start := min(max(0, offset), max(0, len(lines)-content))
	result := append([]string{}, lines[start:min(len(lines), start+content)]...)
	return append(result, fmt.Sprintf("Lines %d-%d/%d | PgUp/PgDn", start+1, min(len(lines), start+content), len(lines)))
}
func (m Model) helpLines(width int) []string {
	if m.jobs != nil {
		return m.jobsHelp(width)
	}
	if m.live {
		return m.liveHelp(width)
	}
	return m.wrap("DESIGN PREVIEW HELP\n\nThis command has no engine or live configuration.\nAll projects, ages, results and recovery evidence are synthetic.\n\nTab / 1 / 2: choose Overview or Projects\nUp/Down or j/k: select a sample\nEnter: open full project details\n/: filter projects by name or type\nEsc: return, or clear the filter\nPgUp/PgDn: scroll details or a long list\n?: show this help\nq / Ctrl+C: quit (q can be typed while searching)\n\nBackup, restore, checks and settings are unavailable here.\nUse tui for the existing live interface, not this prototype.\nNo background jobs, timers, file writes or network calls are made.", width)
}

func (m Model) layout() (head, footer []string, available int) {
	width, height := max(1, m.width), max(1, m.height)
	head = []string{m.accent("HOMEVAULT") + m.muted("  /  DESIGN PREVIEW"), m.muted("Synthetic data only - no real services"), m.line(width)}
	if m.live {
		head = []string{m.accent("HOMEVAULT") + m.muted("  /  LOCAL RECORDS - READ ONLY"), m.muted("Ages at last read; no live checks"), m.line(width)}
	}
	if m.jobs != nil {
		head = []string{m.accent("HOMEVAULT") + m.muted(" / MANUAL FILE JOBS"), m.muted(m.contextLabel + " | b reviews; typed confirmation required"), m.line(width)}
	}
	tabs := []string{"[1] Overview", "[2] Projects"}
	tabs[m.page] = m.accent(tabs[m.page])
	head = append(head, strings.Join(tabs, "    "), "")
	if m.searching {
		head = append(head, m.wrap("Filter: "+m.query+"_  (Enter apply / Esc clear)", width)...)
	} else if m.query != "" {
		head = append(head, m.wrap("Filter: "+m.query, width)...)
	}
	if m.page == 0 && !m.details && !m.help && !m.reviewLoading && m.review == nil && !m.jobPanel && m.historyProject == "" && m.readinessProject == "" {
		if m.live {
			head = append(head, m.wrap(m.summary(), width)...)
		} else if width < 80 {
			head = append(head, m.wrap("4 samples: 1 pass / 1 stale / 1 fail / 1 off", width)...)
			head = append(head, "2 samples need attention")
		} else {
			head = append(head, m.wrap("SAMPLE HEALTH  1 passed / 1 stale / 1 failed / 1 inactive", width)...)
			head = append(head, m.wrap("Attention: Notes demo has old source data; Files demo failed its check.", width)...)
		}
	}
	if m.live && (m.readFailed || m.loading) {
		label := "REFRESH FAILED - cached records"
		if m.loading {
			label = "Refreshing local files; prior values are cached"
		}
		head = append(head, m.wrap(m.ink(label, "38;5;221"), width)...)
	}
	if m.notice != "" && m.review == nil {
		head = append(head, m.wrap(m.notice, width)...)
	}
	head = append(head, "")
	hint := "Enter details | Tab page | / search | ? help | q quit"
	if m.details {
		hint = "Esc back | Up/Down or PgUp/PgDn scroll | q quit"
	}
	if m.searching {
		hint = "Type to filter | Enter apply | Esc clear | Ctrl+C quit"
	}
	if m.help {
		hint = "Esc back | PgUp/PgDn scroll | q quit"
	}
	if m.live && !m.searching && !m.help {
		hint += " | r refresh"
	}
	if m.jobs != nil && !m.searching && !m.help && !m.details {
		hint = "Enter details | d readiness | h history | b review | / search | r refresh | ? help | q quit"
	}
	if m.historyProject != "" {
		hint = "Esc/h back | Up/Down/PgUp/PgDn scroll | r refresh | q quit"
	}
	if m.readinessProject != "" {
		hint = "Esc/d back | Up/Down/PgUp/PgDn scroll | r refresh | q quit"
	}
	if m.reviewLoading || m.review != nil {
		hint = "Review only | Enter confirms exact ID | Esc cancels"
	}
	if m.jobPanel {
		if m.jobActive {
			hint = "x cancel | q cancel, wait, quit"
		} else {
			hint = "Enter/Esc back | q quit"
		}
	}
	footer = []string{m.muted(m.line(width))}
	if m.review != nil && m.review.Allowed {
		footer = append(footer, m.wrap("Confirm "+m.review.ID+": "+m.typed+"_", width)...)
		if m.notice != "" {
			hint = "ID must match exactly | Enter confirm | Esc cancel"
		}
	}
	footer = append(footer, m.wrap(hint, width)...)
	if m.live {
		label := "LOCAL RECORDS | last read " + m.now.In(m.location).Format("02 Jan 2006 15:04 MST")
		if width < 80 {
			label = "READ ONLY | read " + m.now.In(m.location).Format("02 Jan 15:04 MST")
		}
		if m.jobs != nil {
			label = "MANUAL JOBS | last read " + m.now.In(m.location).Format("02 Jan 15:04 MST")
		}
		footer = append(footer, m.muted(label))
	} else {
		footer = append(footer, m.muted("DEMO ONLY | fixed clock: 25 Oct 2026, 06:00 UTC"))
	}
	available = max(1, height-len(head)-len(footer))
	return head, footer, available
}

func (m Model) View() string {
	width, height := max(1, m.width), max(1, m.height)
	if width < 40 || height < 18 {
		title := "HOMEVAULT DEMO"
		if m.live {
			title = "HOMEVAULT READ ONLY"
		}
		if m.jobs != nil {
			title = "MANUAL FILE JOBS / " + m.contextLabel
		}
		lines := []string{title, "Resize to at least 40x18", "q / Ctrl+C: quit"}
		if m.jobActive {
			lines = []string{title, "Job active; x cancels", "q cancels, waits, quits"}
		} else if m.reviewLoading || m.review != nil {
			lines = []string{title, "Resize before confirming", "Esc cancels; Ctrl+C quits"}
		}
		if len(lines) > height {
			lines = lines[:height]
		}
		for i, line := range lines {
			lines[i] = fit(line, width)
		}
		return strings.Join(lines, "\n")
	}
	head, footer, available := m.layout()
	projects := m.filtered()
	var body []string
	switch {
	case m.reviewLoading || m.review != nil || m.jobPanel:
		body = window(m.jobsPanel(width), m.offset, available)
	case m.readinessProject != "":
		body = window(m.readinessLines(width), m.offset, available)
	case m.historyProject != "":
		body = window(m.historyLines(width), m.offset, available)
	case m.help:
		body = window(m.helpLines(width), m.offset, available)
	case m.details && len(projects) > 0:
		body = window(m.detail(projects[m.selected], width, false), m.offset, available)
	case width >= 110 && len(projects) > 0:
		left := (width - 3) * 3 / 5
		right := width - left - 3
		a, b := m.list(left, available), m.detail(projects[m.selected], right, true)
		separator := " | "
		if !m.ascii {
			separator = " │ "
		}
		for i := 0; i < available; i++ {
			x, y := "", ""
			if i < len(a) {
				x = a[i]
			}
			if i < len(b) {
				y = b[i]
			}
			body = append(body, pad(x, left)+m.muted(separator)+fit(y, right))
		}
	default:
		body = m.list(width, available)
	}
	lines := append(head, body...)
	for len(lines) < height-len(footer) {
		lines = append(lines, "")
	}
	if len(lines) > height-len(footer) {
		lines = lines[:max(0, height-len(footer))]
	}
	lines = append(lines, footer...)
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = fit(line, width)
	}
	return strings.Join(lines, "\n")
}
