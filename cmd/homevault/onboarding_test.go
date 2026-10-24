package main

import (
	"os"
	"path/filepath"
	"testing"

	"homevault/internal/config"
)

func TestSetupReusesKeyAndRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	source, key, repo := filepath.Join(root, "media"), filepath.Join(root, "existing-key"), filepath.Join(root, "repo")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("existing-test-key"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "setup")
	args := []string{target, "--project", "photos", "--source", source, "--repository", repo, "--password-file", key}
	if err := runSetup(args); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(target, "homevault.json"))
	if err != nil || cfg.PasswordFile != key || cfg.Repository != repo {
		t.Fatal("wrong generated config", err)
	}
	if _, err := os.Stat(repo); !os.IsNotExist(err) {
		t.Fatal("repository initialized")
	}
	if err := runSetup(args); err == nil {
		t.Fatal("existing setup overwritten")
	}
	data, _ := os.ReadFile(key)
	if string(data) != "existing-test-key" {
		t.Fatal("key changed")
	}
	if _, err := os.Stat(filepath.Join(target, "ONBOARDING.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSetupFailsBeforeCreatingOutput(t *testing.T) {
	root := t.TempDir()
	source, key := filepath.Join(root, "media"), filepath.Join(root, "key")
	os.Mkdir(source, 0700)
	os.WriteFile(key, []byte("test-key"), 0600)
	for _, extra := range [][]string{{"--capture-required"}, {"--sqlite", filepath.Join(root, "missing.db")}, {"--unexpected"}} {
		target := filepath.Join(root, "new")
		args := []string{target, "--project", "photos", "--source", source, "--repository", filepath.Join(root, "repo"), "--password-file", key}
		if err := runSetup(append(args, extra...)); err == nil {
			t.Fatal("invalid setup accepted", extra)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("created output before validation")
		}
	}
}
