package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"homevault/internal/drive"
	"homevault/internal/engine"
)

func (m model) localDriveStatus() resultMsg {
	if m.cfg.RcloneConfigFile == "" {
		return resultMsg{text: "No explicit rclone_config_file configured. Use drive-setup/drive-auth and review the configuration path before deployment."}
	}
	status, err := drive.Status(m.ctx, filepath.Dir(m.cfg.RcloneConfigFile), false, "")
	return resultMsg{text: fmt.Sprintf("Drive remote: %s\nProject: %s\nRequested scope: %s\nLocal state: %s\nAccess expiry (UTC): %s\nRefresh token available: %t\nNo live check, account identity check or backup/restore proof.\nUse drive-status DIRECTORY --online for an explicit reachability check.", status.Remote, status.Project, status.RequestedScope, status.State, displayTime(status.AccessExpires), status.RefreshAvailable), err: err}
}

func (m model) pageSize() int {
	height := m.height
	if height == 0 {
		height = 24
	}
	return max(1, height-max(1, len(m.cfg.Projects))-12)
}

func tuiWelcome() string {
	return "GET STARTED\n" +
		"1. Select a project with Up/Down.\n" +
		"2. Press h for verified job history or s for cloud snapshots.\n" +
		"3. Press d for local Drive credentials; this does not test connectivity.\n" +
		"4. Press g for recovery instructions.\n\n" +
		"Backup requires confirmation. Retention preview never deletes data.\n" +
		"Schedule and workflow activation are managed separately; this screen does not verify timers."
}

func (m model) detailLines() []string {
	text := m.status
	if text == "" {
		text = tuiWelcome()
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	return strings.Split(ansi.Hardwrap(text, max(1, width-1), true), "\n")
}

func (m *model) clampOffset() {
	m.offset = min(max(0, m.offset), max(0, len(m.detailLines())-m.pageSize()))
}

func displayTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatJobHistory(s engine.JobStatus) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s\nLast successful backup+check (UTC): %s\n", s.Project, displayTime(s.LastSuccess))
	if len(s.History) == 0 {
		b.WriteString("No completed job history. Plain backup does not record job success.\n")
	}
	if s.Latest.Status != "" {
		fmt.Fprintf(&b, "Latest: %s · phase %s · started %s\n", s.Latest.Status, s.Latest.Phase, displayTime(s.Latest.Started))
	}
	b.WriteString("Backup/check success does not prove application recovery.\nRecent attempts (newest first):\n")
	for i := len(s.History) - 1; i >= 0; i-- {
		a := s.History[i]
		fmt.Fprintf(&b, "%s · %s · %s\n  capture=%t backup=%t check=%t\n", displayTime(a.Started), a.Status, a.Phase, a.CapturePassed, a.BackupPassed, a.CheckPassed)
	}
	return b.String()
}

func formatRetention(p engine.RetentionPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RETENTION PREVIEW · UTC · no deletion\nPolicy: daily %d / weekly %d / monthly %d\nKeep %d · candidates %d\n", p.Policy.Daily, p.Policy.Weekly, p.Policy.Monthly, len(p.Keep), len(p.Candidates))
	b.WriteString("Candidates are outside policy; recovery safety is not verified.\n")
	for _, s := range p.Keep {
		fmt.Fprintf(&b, "KEEP %s\n  %s · %s\n", s.ID, displayTime(s.Time), strings.Join(s.Reasons, ", "))
	}
	for _, s := range p.Candidates {
		fmt.Fprintf(&b, "CANDIDATE %s\n  %s\n", s.ID, displayTime(s.Time))
	}
	return b.String()
}

func recoveryHelp() string {
	return "RECOVERY GUIDE · commands run separately in your terminal\n" +
		"1. Preserve the repository key and private transport configuration off-server.\n" +
		"2. List snapshots and choose a complete 64-character ID.\n" +
		"3. homevault --config CONFIG restore PROJECT FULL_ID NEW_EMPTY_TARGET --confirm\n" +
		"4. Validate exported DB and media; use an isolated rehearsal plan.\n" +
		"5. Verify health/database/media/login and cleanup before trusting recovery.\n" +
		"Never restore over production. See docs/RECOVERY-REHEARSAL.md.\n" +
		"Capture-required projects use the reviewed capture-backup CLI; b cannot stop them."
}
