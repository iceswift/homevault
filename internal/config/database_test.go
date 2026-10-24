package config

import (
	"path/filepath"
	"testing"
)

func TestDatabasePolicy(t *testing.T) {
	cfg := fixture(t)
	db := Database{Name: "main", Type: "sqlite", Source: filepath.Join(t.TempDir(), "lost.db")}
	cfg.Projects[0].Databases = []Database{db}
	if err := cfg.Validate(); err != nil {
		t.Fatal("lost database must not block recovery", err)
	}
	cfg.Projects[0].Paths = nil
	if err := cfg.Validate(); err != nil {
		t.Fatal("database-only project rejected", err)
	}
	cfg.Projects[0].Paths = []string{filepath.Dir(db.Source)}
	if err := cfg.Validate(); err == nil {
		t.Fatal("live database included as ordinary files")
	}
	cfg.Projects[0].Paths = nil
	cfg.Projects[0].Databases = append(cfg.Projects[0].Databases, db)
	if err := cfg.Validate(); err == nil {
		t.Fatal("duplicate DB accepted")
	}
	cfg.Projects[0].Databases = []Database{{Name: "main", Type: "postgres", Host: "localhost", Port: 5432, User: "app", Database: "postgres://secret", PasswordFile: filepath.Join(t.TempDir(), "pgpass")}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("connection URL accepted")
	}
}

func TestJobConfigurationProtectsWebhook(t *testing.T) {
	cfg := fixture(t)
	cfg.BackupMaxAge = "36h"
	cfg.DiscordWebhookFile = filepath.Join(t.TempDir(), "hook")
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.BackupMaxAge = "never"
	if cfg.Validate() == nil {
		t.Fatal("invalid max age accepted")
	}
	cfg.BackupMaxAge = "36h"
	cfg.DiscordWebhookFile = filepath.Join(cfg.Projects[0].Paths[0], "private-hook")
	if cfg.Validate() == nil {
		t.Fatal("webhook inside backup source accepted")
	}
	cfg.DiscordWebhookFile = "https://discord.com/api/webhooks/not-a-file"
	if cfg.Validate() == nil {
		t.Fatal("webhook URL accepted as configuration file")
	}
}
