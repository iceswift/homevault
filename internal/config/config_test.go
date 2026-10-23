package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixture(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	password := filepath.Join(root, "password")
	if err := os.WriteFile(password, []byte("test-only-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repository"), PasswordFile: password, Projects: []Project{{Name: "photos", Paths: []string{source}}}}
}

func TestValidConfigurationAndLoad(t *testing.T) {
	cfg := fixture(t)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(cfg)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Projects[0].Name != "photos" {
		t.Fatalf("Load() = %+v, %v", loaded, err)
	}
	for _, bad := range []string{strings.TrimSuffix(string(data), "}") + `,"password":"do-not-repeat"}`, string(data) + `{}`, string(data) + " trailing"} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || strings.Contains(err.Error(), "do-not-repeat") {
			t.Fatalf("unsafe or missing parse error: %v", err)
		}
	}
}

func TestRejectUnsafeConfiguration(t *testing.T) {
	tests := map[string]func(*Config){
		"version":             func(c *Config) { c.Version = 2 },
		"blank state":         func(c *Config) { c.StateDir = "" },
		"relative state":      func(c *Config) { c.StateDir = "state" },
		"blank repository":    func(c *Config) { c.Repository = "" },
		"url credentials":     func(c *Config) { c.Repository = "rest:https://user:secret@example.invalid/repo" },
		"unsupported url":     func(c *Config) { c.Repository = "https://example.invalid/repo" },
		"rclone credentials":  func(c *Config) { c.Repository = "rclone:user:secret@host:path" },
		"blank rclone path":   func(c *Config) { c.Repository = "rclone:remote:" },
		"password missing":    func(c *Config) { c.PasswordFile += ".missing" },
		"unsafe project name": func(c *Config) { c.Projects[0].Name = "--tag;bad" },
		"duplicate name":      func(c *Config) { c.Projects = append(c.Projects, c.Projects[0]) },
		"no projects":         func(c *Config) { c.Projects = nil },
		"no sources":          func(c *Config) { c.Projects[0].Paths = nil },
		"blank source":        func(c *Config) { c.Projects[0].Paths[0] = "" },
		"repository in source": func(c *Config) {
			c.Repository = filepath.Join(c.Projects[0].Paths[0], "repo")
		},
		"state in source": func(c *Config) { c.StateDir = filepath.Join(c.Projects[0].Paths[0], "state") },
		"source in state": func(c *Config) { c.StateDir = filepath.Dir(c.Projects[0].Paths[0]) },
		"password in source": func(c *Config) {
			c.Projects[0].Paths[0] = filepath.Dir(c.PasswordFile)
		},
		"root source": func(c *Config) {
			c.Projects[0].Paths[0] = filepath.VolumeName(c.StateDir) + string(filepath.Separator)
		},
		"root state": func(c *Config) {
			c.StateDir = filepath.VolumeName(c.StateDir) + string(filepath.Separator)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := fixture(t)
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestPasswordPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission policy")
	}
	cfg := fixture(t)
	if err := os.Chmod(cfg.PasswordFile, 0644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("world-readable password accepted")
	}
}

func TestMissingSourcesAllowRecoveryConfiguration(t *testing.T) {
	cfg := fixture(t)
	if err := os.Remove(cfg.Projects[0].Paths[0]); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("lost source blocked recovery configuration: %v", err)
	}
	data, _ := json.Marshal(cfg)
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err != nil {
		t.Fatalf("lost source blocked configuration loading: %v", err)
	}
}

func TestSymlinkAndNonexistentAncestorContainment(t *testing.T) {
	cfg := fixture(t)
	alias := filepath.Join(filepath.Dir(cfg.StateDir), "source-link")
	if err := os.Symlink(cfg.Projects[0].Paths[0], alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cfg.Repository = filepath.Join(alias, "not-created", "repository")
	if err := cfg.Validate(); err == nil {
		t.Fatal("repository alias within source accepted")
	}
}

func TestRcloneRepository(t *testing.T) {
	cfg := fixture(t)
	cfg.Repository = "rclone:archive:homevault/repository"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
