package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/config"
	"homevault/internal/engine"
)

var version = "0.1.0-dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "homevault:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("homevault", flag.ContinueOnError)
	file := fs.String("config", "homevault.json", "configuration file")
	timeout := fs.Duration("timeout", 2*time.Hour, "operation timeout")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Reviewed workflow event: notify-event PROJECT failed|stale|recovered --confirm (notification only; no job freshness change)")
		fmt.Fprintln(fs.Output(), "Portable recovery: kit-export PROJECT FULL_ID NEW_KIT.age --recipient AGE_PUBLIC_KEY --confirm; kit-open KIT.age NEW_DIRECTORY --identity-file PRIVATE_KEY --confirm")
		fmt.Fprintln(fs.Output(), "Drive onboarding: drive-setup NEW_DIRECTORY --client-json FILE; drive-auth DIRECTORY --confirm; drive-status DIRECTORY [--online]")
		fmt.Fprintln(fs.Output(), "Unused-pack cleanup: prune-plan; prune-save NEW_PLAN.json; prune-apply PLAN.json --repository-wide --preserve VERIFIED_FULL_ID --confirm")
		fmt.Fprintln(fs.Output(), "Reviewed retention: retention-save PROJECT NEW_PLAN.json [policy flags]; retention-apply PROJECT PLAN.json --preserve VERIFIED_KEPT_FULL_ID --confirm (removes snapshot records; no prune)")
		fmt.Fprintln(fs.Output(), "Project setup: setup NEW_DIRECTORY --project NAME --source ABS_DIRECTORY --repository REPOSITORY --password-file EXISTING_KEY [--sqlite ABS_DB] [--capture-required]")
		fmt.Fprintln(fs.Output(), "Retention preview: retention-plan PROJECT [--keep-daily 7] [--keep-weekly 4] [--keep-monthly 12] (no deletion)")
		fmt.Fprintln(fs.Output(), "Usage: homevault [--config FILE] [--timeout 2h] COMMAND\nCommands: version, demo-setup NEW_DIRECTORY, doctor, init, backup PROJECT, job-backup PROJECT, capture-check PROJECT PLAN.json, capture-backup PROJECT PLAN.json --confirm, status PROJECT, monitor PROJECT --max-age 36h, notify-test PROJECT, snapshots PROJECT, check, restore PROJECT FULL_SNAPSHOT_ID NEW_TARGET --confirm, rehearse PROJECT FULL_SNAPSHOT_ID NEW_TARGET PLAN.json --confirm, resume-paused GUARD_FILE [--force], tui, tui-demo (synthetic design preview; no config or backup tools required), tui-status --plan FILE (read-only local records), tui-jobs (explicit --config; reviewed manual file jobs)")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	a := fs.Args()
	if len(a) == 0 {
		fs.Usage()
		return nil
	}
	if a[0] == "version" {
		fmt.Println("homevault", version)
		return nil
	}
	if a[0] == "tui-jobs" {
		explicit := ""
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "config" {
				explicit = *file
			}
		})
		return runTUIJobs(ctx, explicit, *timeout, a[1:])
	}
	if a[0] == "tui-status" {
		return runTUIStatus(ctx, a[1:])
	}
	if a[0] == "tui-demo" {
		return runTUIDemo(ctx, a[1:])
	}
	if a[0] == "pause-guardian" || a[0] == "resume-paused" {
		if len(a) != 2 && !(a[0] == "resume-paused" && len(a) == 3 && a[2] == "--force") {
			return errors.New("usage: pause-guardian GUARD_FILE | resume-paused GUARD_FILE [--force]")
		}
		if a[0] == "pause-guardian" {
			return engine.PauseGuardian(ctx, a[1])
		}
		resumeCtx, stop := context.WithTimeout(ctx, 45*time.Second)
		defer stop()
		return engine.RecoverPaused(resumeCtx, a[1], len(a) == 3)
	}
	if a[0] == "demo-setup" {
		if len(a) != 2 {
			return errors.New("usage: demo-setup NEW_DIRECTORY")
		}
		return setupDemo(a[1])
	}
	if a[0] == "setup" {
		return runSetup(a[1:])
	}
	if a[0] == "kit-open" {
		if *timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		return runKitOpen(ctx, a)
	}
	if a[0] == "drive-setup" || a[0] == "drive-auth" || a[0] == "drive-status" {
		if *timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		return runDrive(ctx, a)
	}
	if a[0] == "notify-event" && (len(a) != 4 || a[3] != "--confirm") {
		return errors.New("usage: notify-event PROJECT failed|stale|recovered --confirm")
	}
	if *timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	cfg, err := config.Load(*file)
	if err != nil {
		return err
	}
	if a[0] == "kit-export" {
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		return runKitExport(ctx, cfg, a)
	}
	e, err := engine.New(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	switch a[0] {
	case "prune-plan", "prune-save":
		if a[0] == "prune-plan" && len(a) != 1 || a[0] == "prune-save" && len(a) != 2 {
			return errors.New("usage: prune-plan | prune-save NEW_ABSOLUTE_PLAN.json")
		}
		if a[0] == "prune-save" {
			if err := config.ValidateDestination(a[1]); err != nil {
				return err
			}
		}
		review, err := e.ReviewPrune(ctx)
		if err != nil {
			return err
		}
		if a[0] == "prune-plan" {
			return json.NewEncoder(os.Stdout).Encode(review)
		}
		data, err := json.MarshalIndent(review, "", "  ")
		if err != nil {
			return err
		}
		if err := writeNewPrivate(a[1], data); err != nil {
			return err
		}
		fmt.Println("Private repository-wide unused-pack prune review saved; dry-run only. Review approximate sizes and independently verify a retained snapshot before apply.")
		return nil
	case "prune-apply":
		if len(a) != 6 || a[2] != "--repository-wide" || a[3] != "--preserve" || a[5] != "--confirm" {
			return errors.New("usage: prune-apply PLAN.json --repository-wide --preserve VERIFIED_FULL_ID --confirm")
		}
		review, err := engine.LoadPruneReview(a[1])
		if err != nil {
			return err
		}
		result, err := e.ApplyPrune(ctx, review, a[4])
		if result.Status != "" {
			if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
				return encodeErr
			}
		}
		return err
	case "doctor":
		fmt.Println("PASS configuration and required executables; repository access, OAuth and restore are not tested")
	case "init":
		if len(a) != 1 {
			return errors.New("usage: init")
		}
		return e.Init(ctx)
	case "backup":
		if len(a) != 2 {
			return errors.New("usage: backup PROJECT")
		}
		return e.Backup(ctx, a[1])
	case "job-backup":
		if len(a) != 2 {
			return errors.New("usage: job-backup PROJECT")
		}
		return e.RunBackupJob(ctx, a[1])
	case "notify-event":
		if len(a) != 4 || a[3] != "--confirm" {
			return errors.New("usage: notify-event PROJECT failed|stale|recovered --confirm")
		}
		return e.NotifyEvent(ctx, a[1], a[2])
	case "capture-backup":
		if len(a) != 4 || a[3] != "--confirm" {
			return errors.New("usage: capture-backup PROJECT PLAN.json --confirm")
		}
		plan, err := engine.LoadCapturePlan(a[2])
		if err != nil {
			return err
		}
		err = e.RunCapturedJob(ctx, a[1], plan)
		if errors.Is(err, engine.ErrOutsideMaintenanceWindow) {
			fmt.Println("SKIP outside reviewed maintenance window; no app stopped")
			return nil
		}
		return err
	case "capture-check":
		if len(a) != 3 {
			return errors.New("usage: capture-check PROJECT PLAN.json")
		}
		plan, err := engine.LoadCapturePlan(a[2])
		if err != nil {
			return err
		}
		if err = e.CheckCaptureSource(ctx, a[1], plan); err != nil {
			return err
		}
		fmt.Println("PASS capture source identity/layout/read access; no app stopped and no backup performed")
		return nil
	case "status":
		if len(a) != 2 {
			return errors.New("usage: status PROJECT")
		}
		state, err := e.JobStatus(a[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(state)
	case "monitor":
		if len(a) != 4 || a[2] != "--max-age" {
			return errors.New("usage: monitor PROJECT --max-age 36h")
		}
		age, err := time.ParseDuration(a[3])
		if err != nil || age <= 0 {
			return errors.New("max-age must be a positive duration")
		}
		result, err := e.Monitor(ctx, a[1], age)
		if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
			return encodeErr
		}
		if err != nil {
			return err
		}
		if result.Health == "failed" || result.Health == "stale" {
			return errors.New("backup health is not current; inspect status")
		}
		return nil
	case "notify-test":
		if len(a) != 2 {
			return errors.New("usage: notify-test PROJECT")
		}
		return e.NotifyTest(ctx, a[1])
	case "snapshots":
		if len(a) != 2 {
			return errors.New("usage: snapshots PROJECT")
		}
		snapshots, err := e.Snapshots(ctx, a[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(snapshots)
	case "retention-plan", "retention-save":
		if len(a) < 2 {
			return errors.New("usage: retention-plan PROJECT [--keep-daily 7] [--keep-weekly 4] [--keep-monthly 12]")
		}
		flagStart := 2
		if a[0] == "retention-save" {
			if len(a) < 3 {
				return errors.New("usage: retention-save PROJECT NEW_ABSOLUTE_PLAN.json [--keep-daily 7] [--keep-weekly 4] [--keep-monthly 12]")
			}
			if err := config.ValidateDestination(a[2]); err != nil {
				return err
			}
			flagStart = 3
		}
		flags := flag.NewFlagSet("retention-plan", flag.ContinueOnError)
		daily := flags.Int("keep-daily", 7, "occupied UTC days to keep")
		weekly := flags.Int("keep-weekly", 4, "occupied ISO weeks to keep")
		monthly := flags.Int("keep-monthly", 12, "occupied UTC months to keep")
		if err := flags.Parse(a[flagStart:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected retention-plan argument")
		}
		policy := engine.RetentionPolicy{Daily: *daily, Weekly: *weekly, Monthly: *monthly}
		if a[0] == "retention-save" {
			review, err := e.ReviewRetention(ctx, a[1], policy)
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(review, "", "  ")
			if err != nil {
				return err
			}
			if err := writeNewPrivate(a[2], data); err != nil {
				return err
			}
			fmt.Println("Private retention review saved; no snapshots forgotten. Review keep/candidates and preserve a verified kept snapshot before apply.")
			return nil
		}
		plan, err := e.PlanRetention(ctx, a[1], policy)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(plan)
	case "retention-apply":
		if len(a) != 6 || a[3] != "--preserve" || a[5] != "--confirm" {
			return errors.New("usage: retention-apply PROJECT PLAN.json --preserve VERIFIED_KEPT_FULL_ID --confirm")
		}
		review, err := engine.LoadRetentionReview(a[2])
		if err != nil {
			return err
		}
		result, err := e.ApplyRetention(ctx, a[1], review, a[4])
		if result.Project != "" {
			if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
				return encodeErr
			}
		}
		return err
	case "check":
		return e.Check(ctx)
	case "rehearse":
		if len(a) != 6 || a[5] != "--confirm" {
			return errors.New("usage: rehearse PROJECT FULL_SNAPSHOT_ID NEW_TARGET PLAN.json --confirm")
		}
		plan, err := engine.LoadRecoveryPlan(a[4])
		if err != nil {
			return err
		}
		report, err := e.Rehearse(ctx, a[1], a[2], a[3], plan)
		if report != "" {
			fmt.Println("Recovery report:", report)
		}
		return err
	case "restore":
		if len(a) != 5 || a[4] != "--confirm" {
			return errors.New("usage: restore PROJECT SNAPSHOT NEW_TARGET --confirm (target must be new or empty)")
		}
		return e.Restore(ctx, a[1], a[2], a[3])
	case "tui":
		opCtx, stopOperation := context.WithCancel(ctx)
		workers := &operationGroup{}
		_, err := tea.NewProgram(model{cfg: cfg, e: e, ctx: opCtx, cancel: stopOperation, workers: workers}, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
		// Signal/timeout exits can bypass result delivery. Always cancel AND wait
		// before main exits so children are reaped and the state lock is released.
		stopOperation()
		workers.Wait()
		return err
	default:
		return fmt.Errorf("unknown command %q", a[0])
	}
	return nil
}

type resultMsg struct {
	text string
	err  error
}
type model struct {
	cfg           config.Config
	e             *engine.Engine
	ctx           context.Context
	selected      int
	busy          bool
	status        string
	quitting      bool
	cancel        context.CancelFunc
	workers       *operationGroup
	height        int
	width         int
	confirmBackup bool
	offset        int
}

type operationGroup struct{ sync.WaitGroup }

// Start immediately, rather than inside tea.Cmd: Bubble Tea may discard a queued
// command when its context closes. A buffered result never blocks worker cleanup.
func (g *operationGroup) start(fn func() resultMsg) tea.Cmd {
	result := make(chan resultMsg, 1)
	g.Add(1)
	go func() { defer g.Done(); result <- fn() }()
	return func() tea.Msg { return <-result }
}

func (m model) Init() tea.Cmd { return nil }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = v.Height
		m.width = v.Width
		m.offset = 0
		return m, nil
	case resultMsg:
		m.busy = false
		m.offset = 0
		m.status = v.text
		if v.err != nil {
			m.status = "ERROR: " + v.err.Error()
		}
		if m.quitting {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyMsg:
		if m.confirmBackup {
			m.confirmBackup = false
			if v.String() != "enter" {
				m.status = "Backup cancelled. No job started."
				return m, nil
			}
			// Reuse the guarded operation path only after explicit confirmation.
			v = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}
		} else if v.String() == "b" && !m.busy && len(m.cfg.Projects) > 0 {
			p := m.cfg.Projects[m.selected]
			if m.cfg.RecoveryOnly || p.CaptureRequired {
				m.status = "Interactive backup unavailable for this project.\nUse the reviewed capture workflow or recovery CLI. Press g for guidance."
				return m, nil
			}
			m.confirmBackup = true
			m.offset = 0
			m.status = "Start backup + repository check for " + p.Name + "?\nThis uploads data to the configured repository.\nEnter: start · any other key: cancel"
			return m, nil
		}
		switch v.String() {
		case "?":
			m.status = tuiWelcome()
			m.offset = 0
			return m, nil
		case "pgdown":
			m.offset += m.pageSize()
			m.clampOffset()
			return m, nil
		case "pgup":
			m.offset -= m.pageSize()
			m.clampOffset()
			return m, nil
		case "g":
			if !m.busy {
				m.status = recoveryHelp()
				m.offset = 0
			}
			return m, nil
		case "q", "ctrl+c":
			if m.busy {
				m.quitting = true
				m.status = "Cancelling; waiting for child cleanup and lock release…"
				if m.cancel != nil {
					m.cancel()
				}
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected+1 < len(m.cfg.Projects) {
				m.selected++
			}
		case "b", "s", "c", "h", "p", "d":
			if m.busy || len(m.cfg.Projects) == 0 {
				return m, nil
			}
			name := m.cfg.Projects[m.selected].Name
			m.busy = true
			m.status = "Working… (closing the UI cancels this interactive operation)"
			return m, m.workers.start(func() resultMsg {
				switch v.String() {
				case "b":
					err := m.e.RunBackupJob(m.ctx, name)
					return resultMsg{"Backup and metadata check completed: " + name + "\nApplication recovery is a separate rehearsal.", err}
				case "h":
					state, err := m.e.JobStatus(name)
					return resultMsg{formatJobHistory(state), err}
				case "d":
					return m.localDriveStatus()
				case "p":
					plan, err := m.e.PlanRetention(m.ctx, name, engine.RetentionPolicy{Daily: 7, Weekly: 4, Monthly: 12})
					return resultMsg{formatRetention(plan), err}
				case "c":
					err := m.e.Check(m.ctx)
					return resultMsg{"Repository check completed", err}
				default:
					s, err := m.e.Snapshots(m.ctx, name)
					var b strings.Builder
					for _, x := range s {
						fmt.Fprintf(&b, "%s\n  %s\n", x.ID, x.Time.Format(time.RFC3339))
					}
					if len(s) == 0 {
						b.WriteString("No snapshots")
					}
					return resultMsg{b.String(), err}
				}
			})
		}
	}
	return m, nil
}
func (m model) View() string {
	var b strings.Builder
	b.WriteString("HOMEVAULT · encrypted backup orchestrator\n\n")
	for i, p := range m.cfg.Projects {
		cursor := "  "
		if i == m.selected {
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, p.Name)
	}
	if len(m.cfg.Projects) == 0 {
		b.WriteString("No projects configured. Review your configuration before continuing.\n")
	}
	b.WriteString("\n↑/↓ select · b backup+check · h job history · s snapshots · c check\n")
	b.WriteString("p retention preview · d Drive status · g recovery guide · ? help\nPgUp/PgDn scroll · q quit\n")
	if m.busy {
		b.WriteString("Operation in progress\n")
	}
	lines := m.detailLines()
	start := min(max(m.offset, 0), max(len(lines)-m.pageSize(), 0))
	end := min(start+m.pageSize(), len(lines))
	b.WriteString("\n" + strings.Join(lines[start:end], "\n") + "\n")
	if len(lines) > m.pageSize() {
		fmt.Fprintf(&b, "Lines %d–%d of %d · PgUp/PgDn\n", start+1, end, len(lines))
	}
	b.WriteString("\nRestore uses the explicit CLI command. Scheduled backups run via systemd.\n")
	return b.String()
}
