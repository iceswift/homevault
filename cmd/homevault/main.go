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
		fmt.Fprintln(fs.Output(), "Usage: homevault [--config FILE] [--timeout 2h] COMMAND\nCommands: version, demo-setup NEW_DIRECTORY, doctor, init, backup PROJECT, job-backup PROJECT, capture-check PROJECT PLAN.json, capture-backup PROJECT PLAN.json --confirm, status PROJECT, monitor PROJECT --max-age 36h, notify-test PROJECT, snapshots PROJECT, check, restore PROJECT FULL_SNAPSHOT_ID NEW_TARGET --confirm, rehearse PROJECT FULL_SNAPSHOT_ID NEW_TARGET PLAN.json --confirm, resume-paused GUARD_FILE [--force], tui")
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
	if *timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	cfg, err := config.Load(*file)
	if err != nil {
		return err
	}
	e, err := engine.New(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	switch a[0] {
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
	cfg      config.Config
	e        *engine.Engine
	ctx      context.Context
	selected int
	busy     bool
	status   string
	quitting bool
	cancel   context.CancelFunc
	workers  *operationGroup
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
	case resultMsg:
		m.busy = false
		m.status = v.text
		if v.err != nil {
			m.status = "ERROR: " + v.err.Error()
		}
		if m.quitting {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyMsg:
		switch v.String() {
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
		case "b", "s", "c":
			if m.busy || len(m.cfg.Projects) == 0 {
				return m, nil
			}
			name := m.cfg.Projects[m.selected].Name
			m.busy = true
			m.status = "Working… (closing the UI cancels this interactive operation)"
			return m, m.workers.start(func() resultMsg {
				switch v.String() {
				case "b":
					err := m.e.Backup(m.ctx, name)
					return resultMsg{"Backup completed: " + name, err}
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
	b.WriteString("\n↑/↓ select · b backup · s snapshots · c check · q quit\n")
	if m.busy {
		b.WriteString("Operation in progress\n")
	}
	b.WriteString("\n" + m.status + "\n")
	b.WriteString("\nRestore uses the explicit CLI command. Scheduled backups run via systemd.\n")
	return b.String()
}
