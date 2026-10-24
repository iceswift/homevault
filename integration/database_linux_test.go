//go:build linux

package integration

import (
	"context"
	"homevault/internal/config"
	"homevault/internal/engine"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDatabaseEncryptedRoundTrip(t *testing.T) {
	restic := os.Getenv("HOMEVAULT_TEST_RESTIC")
	if restic == "" {
		t.Skip("real Restic required")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 required")
	}
	root := t.TempDir()
	source := filepath.Join(root, "live.db")
	key := filepath.Join(root, "password")
	if err := os.WriteFile(key, []byte("synthetic-only"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute('create table proof(v text)');c.execute("insert into proof values ('verified')");c.commit()`, source).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repo"), PasswordFile: key, ResticBinary: restic, Projects: []config.Project{{Name: "database", Databases: []config.Database{{Name: "main", Type: "sqlite", Source: source}}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "database"); err != nil {
		t.Fatal(err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(cfg.StateDir, "prepared-*"))
	if len(leftovers) != 0 {
		t.Fatal("plaintext stage retained")
	}
	snapshots, err := e.Snapshots(ctx, "database")
	if err != nil || len(snapshots) != 1 {
		t.Fatal("snapshot missing", err)
	}
	if err = e.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(source); err != nil {
		t.Fatal(err)
	}
	e, err = engine.New(cfg)
	if err != nil {
		t.Fatal("lost source blocked recovery", err)
	}
	target := filepath.Join(root, "restore")
	if err = e.Restore(ctx, "database", snapshots[0].ID, target); err != nil {
		t.Fatal(err)
	}
	found := false
	err = filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "main.sqlite" {
			found = true
			return exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);assert c.execute('select v from proof').fetchall()==[('verified',)]`, path).Run()
		}
		return nil
	})
	if err != nil || !found {
		t.Fatal("restored database query failed", err)
	}
}
