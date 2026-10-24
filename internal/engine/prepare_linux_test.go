//go:build linux

package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"homevault/internal/config"
)

func TestSQLiteOnlinePreparation(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 required")
	}
	e := fixture(t)
	source := filepath.Join(t.TempDir(), "source with ' quotes.db")
	// Keep a WAL writer open while preparation runs: copying only the DB file
	// would miss the committed row. The subprocess exits when its stdin closes.
	writer := exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute('pragma journal_mode=wal');c.execute('create table proof(value text)');c.execute("insert into proof values ('verified')");c.commit();print('ready',flush=True);sys.stdin.read()`, source)
	stdin, _ := writer.StdinPipe()
	stdout, _ := writer.StdoutPipe()
	if err := writer.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); writer.Wait() }()
	var ready [6]byte
	if _, err := stdout.Read(ready[:]); err != nil {
		t.Fatal(err)
	}
	e.cfg.Projects[0].Databases = []config.Database{{Name: "main", Type: "sqlite", Source: source}}
	if err := os.Mkdir(e.cfg.StateDir, 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := e.prepareDatabases(context.Background(), e.cfg.Projects[0])
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stage)
	cmd := exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);assert c.execute('pragma journal_mode').fetchone()==('delete',);assert c.execute('select value from proof').fetchall()==[('verified',)]`, filepath.Join(stage, "main.sqlite"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restored SQLite validation: %v %s", err, output)
	}
	calls := 0
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) { calls++; return nil, nil }
	e.cfg.Projects[0].Databases[0].Source = source + "missing"
	if err := e.Backup(context.Background(), "photos"); err == nil {
		t.Fatal("missing DB accepted")
	}
	if calls != 0 {
		t.Fatal("restic executed after export failure")
	}
	entries, _ := filepath.Glob(filepath.Join(e.cfg.StateDir, "prepared-*"))
	if len(entries) != 1 || entries[0] != stage {
		t.Fatal("failed preparation leaked plaintext staging")
	}
}

func TestPostgresPreparationRoundTrip(t *testing.T) {
	if os.Getenv("HOMEVAULT_TEST_PG") != "1" {
		t.Skip("isolated PostgreSQL fixture required")
	}
	e := fixture(t)
	root := t.TempDir()
	password := filepath.Join(root, "pgpass")
	if err := os.WriteFile(password, []byte("127.0.0.1:5432:*:postgres:fixture-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(binary string, args ...string) {
		t.Helper()
		c := exec.Command(binary, args...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("fixture command: %v %s", err, out)
		}
	}
	run("createdb", "-h", "127.0.0.1", "-U", "postgres", "fixture")
	run("psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "fixture", "-c", "CREATE TABLE proof(value text); INSERT INTO proof VALUES ('verified');")
	db := config.Database{Name: "main", Type: "postgres", Host: "127.0.0.1", Port: 5432, Database: "fixture", User: "postgres", PasswordFile: password}
	e.cfg.Projects[0].Databases = []config.Database{db}
	if err := os.Mkdir(e.cfg.StateDir, 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := e.prepareDatabases(context.Background(), e.cfg.Projects[0])
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stage)
	run("createdb", "-h", "127.0.0.1", "-U", "postgres", "recovery")
	run("pg_restore", "--exit-on-error", "-h", "127.0.0.1", "-U", "postgres", "-d", "recovery", filepath.Join(stage, "main.dump"))
	out, err := exec.Command("psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "recovery", "-Atc", "SELECT value FROM proof").Output()
	if err != nil || strings.TrimSpace(string(out)) != "verified" {
		t.Fatal("PostgreSQL restored row mismatch")
	}
	e.cfg.Projects[0].Databases[0].Database = "missing"
	calls := 0
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) { calls++; return nil, nil }
	if err := e.Backup(context.Background(), "photos"); err == nil || calls != 0 {
		t.Fatal("failed PostgreSQL export did not block restic")
	}
}
