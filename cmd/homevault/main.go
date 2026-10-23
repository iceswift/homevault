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
		fmt.Fprintln(fs.Output(), "Usage: homevault [--config FILE] [--timeout 2h] COMMAND\nCommands: version, demo-setup NEW_DIRECTORY, doctor, init, backup PROJECT, snapshots PROJECT, check, restore PROJECT FULL_SNAPSHOT_ID NEW_TARGET --confirm, tui")
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
