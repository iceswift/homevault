package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/config"
)

func TestRetentionApplyRequiresExplicitConfirmation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "lab")
	if err := setupDemo(root); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "homevault.json")
	cfg, err := config.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ResticBinary, err = os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	err = run(context.Background(), []string{"--config", file, "retention-apply", "synthetic", "missing-review.json", "--preserve", strings.Repeat("a", 64)})
	if err == nil || !strings.Contains(err.Error(), "usage: retention-apply") {
		t.Fatal("missing confirmation not rejected", err)
	}
	if _, err := os.Stat(cfg.Repository); !os.IsNotExist(err) {
		t.Fatal("repository touched without confirmation")
	}
}

func TestPruneApplyRequiresScopeAndConfirmation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "lab")
	if err := setupDemo(root); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "homevault.json")
	cfg, err := config.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ResticBinary, err = os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"prune-apply", "missing.json", "--repository-wide", "--preserve", strings.Repeat("a", 64)},
		{"prune-apply", "missing.json", "--preserve", strings.Repeat("a", 64), "--confirm"},
	} {
		err := run(context.Background(), append([]string{"--config", file}, args...))
		if err == nil || !strings.Contains(err.Error(), "usage: prune-apply") {
			t.Fatal("missing prune acknowledgment not refused", err)
		}
	}
	if _, err := os.Stat(cfg.Repository); !os.IsNotExist(err) {
		t.Fatal("repository touched without prune acknowledgments")
	}
}

func TestDriveOnboardingBeforeBackupConfiguration(t *testing.T) {
	parent := t.TempDir()
	client := filepath.Join(parent, "desktop.json")
	root := filepath.Join(parent, "drive")
	data := []byte(`{"installed":{"client_id":"123-fixture.apps.googleusercontent.com","client_secret":"fixture-client-secret","project_id":"homevault-fixture"}}`)
	if err := os.WriteFile(client, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"drive-setup", root, "--client-json", client, "--project-id", "homevault-fixture"}); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), []string{"drive-auth", root})
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatal("authorization started without explicit confirmation", err)
	}
	m := model{ctx: context.Background(), cfg: config.Config{RcloneConfigFile: filepath.Join(root, "rclone.conf")}}
	status := m.localDriveStatus()
	if status.err != nil || !strings.Contains(status.text, "authorization_required") || strings.Contains(status.text, "fixture-client-secret") {
		t.Fatal("bad safe TUI connection status", status.err)
	}
}

func TestKitCommandsRequireConfirmationBeforeTools(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "recovered")
	err := run(context.Background(), []string{"kit-open", filepath.Join(root, "missing.age"), target, "--identity-file", filepath.Join(root, "identity.txt")})
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatal("kit-open accessed old config/tools without confirmation", err)
	}
	lab := filepath.Join(root, "lab")
	if err := setupDemo(lab); err != nil {
		t.Fatal(err)
	}
	err = run(context.Background(), []string{"--config", filepath.Join(lab, "homevault.json"), "kit-export", "synthetic", strings.Repeat("a", 64), filepath.Join(root, "new.age"), "--recipient", "not-used-without-confirm"})
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatal("kit-export started without confirmation", err)
	}
}

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

func TestWorkflowNotificationRequiresConfirmation(t *testing.T) {
	lab := filepath.Join(t.TempDir(), "lab")
	if err := setupDemo(lab); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), []string{"--config", filepath.Join(lab, "homevault.json"), "notify-event", "synthetic", "failed"})
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatal("workflow notification accepted without confirmation", err)
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
