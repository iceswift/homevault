package tui

import (
	"fmt"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/statusdata"
)

type Jobs interface {
	Review(string) (statusdata.JobReview, error)
	Discard()
	Start(string) (<-chan statusdata.JobEvent, error)
	Cancel()
	Wait()
}
type jobReviewResult struct {
	generation uint64
	review     statusdata.JobReview
	err        error
}
type jobEventMsg struct{ event statusdata.JobEvent }
type jobEvents <-chan statusdata.JobEvent

func NewManualJobs(options Options, data statusdata.Snapshot, loader Loader, jobs Jobs) (Model, error) {
	m, err := NewReadOnly(options, data, loader)
	if err != nil {
		return m, err
	}
	m.jobs = jobs
	return m, nil
}
func waitJob(events jobEvents) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			event = statusdata.JobEvent{Done: true, State: "unknown", Message: "Worker ended; inspect local job status."}
		}
		return jobEventMsg{event}
	}
}
func (m Model) refreshCmd() tea.Cmd {
	if m.loader == nil {
		return nil
	}
	load := m.loader
	return func() tea.Msg { data, err := load(); return refreshResult{data: data, err: err} }
}
func (m *Model) discardReview() {
	m.reviewGeneration++
	m.reviewLoading = false
	m.review = nil
	m.typed = ""
	m.offset = 0
	if m.jobs != nil {
		m.jobs.Discard()
	}
}

// handleJobs returns before ordinary navigation whenever a review/job owns input.
func (m *Model) handleJobs(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.jobs == nil {
		return false, nil
	}
	defer m.clampJobOffset()
	key := msg.String()
	if m.jobActive {
		switch key {
		case "q", "ctrl+c":
			m.quitAfterJob = true
			m.jobs.Cancel()
			m.jobMessage = "Cancelling; waiting for child cleanup and lock release."
		case "x":
			m.jobs.Cancel()
			m.jobMessage = "Cancelling; waiting for child cleanup and lock release."
		case "pgdown", "down":
			m.offset++
		case "pgup", "up":
			m.offset = max(0, m.offset-1)
		}
		return true, nil
	}
	if m.reviewLoading || m.review != nil {
		if key == "esc" {
			m.discardReview()
			return true, nil
		}
		if key == "ctrl+c" || key == "q" && m.review != nil && !m.review.Allowed {
			m.discardReview()
			return true, tea.Quit
		}
		if m.reviewLoading {
			return true, nil
		}
		if !m.review.Allowed {
			return true, nil
		}
		switch key {
		case "enter":
			if m.width < 40 || m.height < 18 {
				m.notice = "Enlarge terminal before confirming."
				return true, nil
			}
			if m.typed != m.review.ID {
				m.notice = "Type the exact project ID, then press Enter."
				return true, nil
			}
			id, token := m.review.ID, m.review.Token
			events, err := m.jobs.Start(token)
			m.review = nil
			m.typed = ""
			m.notice = ""
			m.offset = 0
			if err != nil {
				m.notice = "No job started. Refresh/restart and review again."
				return true, nil
			}
			m.jobID = id
			m.jobActive = true
			m.jobPanel = true
			m.jobPhase = "starting"
			m.jobState = "running"
			m.jobMessage = "Waiting for engine lock and validation."
			m.events = events
			return true, waitJob(events)
		case "backspace", "ctrl+h":
			m.notice = ""
			r := []rune(m.typed)
			if len(r) > 0 {
				m.typed = string(r[:len(r)-1])
			}
		case "pgdown", "down":
			m.offset++
		case "pgup", "up":
			m.offset = max(0, m.offset-1)
		default:
			if msg.Type == tea.KeyRunes {
				m.notice = ""
				r := []rune(m.typed)
				for _, v := range msg.Runes {
					if len(r) < 64 && unicode.IsPrint(v) {
						r = append(r, v)
					}
				}
				m.typed = string(r)
			}
		}
		return true, nil
	}
	if m.jobPanel {
		switch key {
		case "esc", "enter":
			m.jobPanel = false
			m.offset = 0
		case "q", "ctrl+c":
			return true, tea.Quit
		}
		return true, nil
	}
	if key == "b" && !m.searching && !m.help {
		if m.loading {
			m.notice = "Wait for the current metadata refresh."
			return true, nil
		}
		rows := m.filtered()
		if len(rows) == 0 {
			return true, nil
		}
		id := rows[m.selected].id
		m.reviewGeneration++
		generation := m.reviewGeneration
		m.reviewLoading = true
		m.notice = ""
		m.offset = 0
		jobs := m.jobs
		return true, func() tea.Msg { review, err := jobs.Review(id); return jobReviewResult{generation, review, err} }
	}
	return false, nil
}
func (m Model) jobsPanel(width int) []string {
	if m.reviewLoading {
		return m.wrap("PREPARING REVIEW\n\nNo job has started. Esc cancels.", width)
	}
	if m.review != nil {
		r := m.review
		text := "REVIEW BACKUP + METADATA CHECK\n\nProject: " + r.ID + "\nConfiguration: sha256 " + r.ConfigFingerprint + "\nSource paths: " + fmtCount(r.Sources) + " (inspect config for the complete list)\nDestination: " + r.Destination + "\n\n"
		if !r.Allowed {
			text += "BLOCKED\n" + r.Reason + "\n\nEsc: return | q: quit"
			return m.wrap(text, width)
		}
		text += "Manual file job only. No app pause or coordinated database capture.\nSource paths must not contain live database storage.\nNo initialization, restore, deletion, workflow activation or timer changes.\n"
		if r.Notifications {
			text += "Configured notifications may be sent after this confirmed job.\n"
		} else {
			text += "No webhook is configured for this job.\n"
		}
		text += "\nConfiguration/job identity is checked again under the engine lock.\n\nType project ID to start: " + m.typed + "_\nEnter: confirm | Esc: cancel\n"
		if m.notice != "" {
			text += "\n" + m.notice
		}
		return m.wrap(text, width)
	}
	return m.wrap("MANUAL FILE JOB\n\nProject: "+m.jobID+"\nPhase: "+m.jobPhase+"\nResult: "+m.jobState+"\n\n"+m.jobMessage+"\n\nProgress comes from engine records; no percentage is invented.\nBackup/check success does not prove application recovery.\n\n"+func() string {
		if m.jobActive {
			return "x: cancel and stay | q/Ctrl+C: cancel, wait, quit"
		}
		return "Enter/Esc: return | q: quit"
	}(), width)
}
func fmtCount(n int) string { return fmt.Sprint(n) }
func (m Model) jobsHelp(width int) []string {
	return m.wrap("MANUAL FILE JOBS\n\nb: review selected project; type its exact ID then Enter to start\nEsc: cancel review (no job)\nx: request cancellation while running\nq/Ctrl+C during a job: cancel and wait for cleanup before quitting\n\nOnly file-only projects are supported. Capture-required, database and recovery-only configurations are blocked.\nThis command invokes manual native jobs, not coordinator workflows: their enabled flags/windows are not loaded.\nExisting engine operation/credential locks, config validation and process cancellation are used.\nNo repository initialization, app pause, restore, deletion, unlock or timer action exists here.\nConfigured notifications may follow a confirmed job.\n\nSources must exclude live databases. A successful job is backup/check evidence only.\nr manually refreshes local job records; source freshness/recovery remain unknown.\nHard process kills/power loss can require independent lock/recovery investigation; no automatic stale unlock is performed.", width)
}

func (m *Model) clampJobOffset() {
	if !m.reviewLoading && m.review == nil && !m.jobPanel {
		return
	}
	_, _, height := m.layout()
	lines := m.jobsPanel(max(1, m.width))
	if len(lines) <= height {
		m.offset = 0
	} else {
		m.offset = min(max(0, m.offset), max(0, len(lines)-max(0, height-1)))
	}
}
