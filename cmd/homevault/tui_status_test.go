package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTUIStatusHasNoToolOrSecretFileDependency(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base.json")
	plan := filepath.Join(root, "plan.json")
	write := func(path string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(base, map[string]any{"version": 1, "repository": "rclone:demo:Samples", "rclone_config_file": filepath.Join(root, "absent-secret"), "password_file": filepath.Join(root, "absent-key"), "state_dir": filepath.Join(root, "missing-engine-state")})
	write(plan, map[string]any{"version": 1, "root": filepath.Join(root, "missing-workflow-state"), "base_config": base, "homevault_binary": filepath.Join(root, "missing-executable"), "notifications_enabled": true, "workflows": []any{map[string]any{"name": "demo", "project": "demo", "kind": "files", "enabled": false, "max_age_hours": 36, "files": []any{}, "trees": []any{}}}})
	t.Setenv("PATH", "")
	if err := run(context.Background(), []string{"--config", filepath.Join(root, "not-loaded.json"), "tui-status", "--plan", plan, "--render", "--ascii", "--timezone", "UTC"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		t.Fatal("read-only command wrote state or credential locks")
	}
	if err := run(context.Background(), []string{"tui-status", "--render"}); err == nil {
		t.Fatal("implicit plan discovery accepted")
	}
}
