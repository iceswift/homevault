package main

import (
	"fmt"
	"strings"
	"time"

	"homevault/internal/engine"
)

func (m model) pageSize() int {
	height := m.height
	if height == 0 {
		height = 24
	}
	return max(3, height-len(m.cfg.Projects)-11)
}

func (m *model) clampOffset() {
	m.offset = min(max(0, m.offset), max(0, len(strings.Split(m.status, "\n"))-m.pageSize()))
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
