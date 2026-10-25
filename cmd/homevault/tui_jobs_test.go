package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"homevault/internal/config"
)

func TestTUIJobsRequiresExplicitConfigAndRenderDoesNotRun(t *testing.T) {
	if err := run(context.Background(), []string{"tui-jobs", "--render"}); err == nil {
		t.Fatal("implicit config accepted")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0700)
	key := filepath.Join(root, "key")
	os.WriteFile(key, []byte("generated-key"), 0600)
	exe, _ := os.Executable()
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repository"), PasswordFile: key, ResticBinary: exe, Projects: []config.Project{{Name: "demo", Paths: []string{source}}}}
	file := filepath.Join(root, "config.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(file, data, 0600)
	if err := run(context.Background(), []string{"--config", file, "tui-jobs", "--render", "--ascii", "--timezone", "UTC"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cfg.StateDir, cfg.Repository} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("render initialized or created state")
		}
	}
}
