package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTUIDemoNeedsNoConfigOrExternalTools(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "invalid.json")
	original := []byte("invalid configuration; must never be loaded")
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	if err := run(context.Background(), []string{"--config", file, "tui-demo", "--render", "--no-color", "--ascii", "--width", "80", "--height", "24"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != string(original) {
		t.Fatal("prototype changed config")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("prototype created local state")
	}
	if err := run(context.Background(), []string{"--config", file, "tui-demo", "--project", "production"}); err == nil {
		t.Fatal("accepted real/unknown project")
	}
}
