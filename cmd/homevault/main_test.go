package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/config"
)

func TestDemoSetupRefusesExistingDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "lab")
	if err := setupDemo(root); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(root, "homevault.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0].Name != "synthetic" {
		t.Fatal("unexpected generated project")
	}
	before, err := os.ReadFile(cfg.PasswordFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := setupDemo(root); err == nil {
		t.Fatal("overwrote existing sandbox")
	}
	after, _ := os.ReadFile(cfg.PasswordFile)
	if string(before) != string(after) {
		t.Fatal("changed existing key")
	}
}

func TestTUIQuitWaitsForCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := &operationGroup{}
	cleaned := make(chan struct{})
	cmd := g.start(func() resultMsg {
		<-ctx.Done()
		time.Sleep(10 * time.Millisecond)
		close(cleaned)
		return resultMsg{err: ctx.Err()}
	})
	m := model{ctx: ctx, cancel: cancel, workers: g, busy: true}
	updated, quit := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quit != nil {
		t.Fatal("quit before active operation cleanup")
	}
	m = updated.(model)
	if !m.quitting {
		t.Fatal("quit intent lost")
	}
	msg := cmd()
	g.Wait()
	select {
	case <-cleaned:
	default:
		t.Fatal("cleanup not complete")
	}
	_, quit = m.Update(msg)
	if quit == nil {
		t.Fatal("did not quit after cleanup")
	}
}
func TestTUINavigationAndBusyGuard(t *testing.T) {
	m := model{ctx: context.Background(), cfg: config.Config{Projects: []config.Project{{Name: "one"}, {Name: "two"}}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.selected != 1 {
		t.Fatal("selection did not move")
	}
	m.busy = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd != nil {
		t.Fatal("started overlapping operation")
	}
	updated, _ = m.Update(resultMsg{text: "done"})
	m = updated.(model)
	if m.busy || m.status != "done" {
		t.Fatal("result not displayed")
	}
}
