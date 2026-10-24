//go:build linux

package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/engine"
)

func TestPostgresApplicationRehearsal(t *testing.T) {
	restic := os.Getenv("HOMEVAULT_TEST_RESTIC")
	pgImage := os.Getenv("HOMEVAULT_TEST_PG_IMAGE")
	appImage := os.Getenv("HOMEVAULT_TEST_PG_APP_IMAGE")
	if restic == "" || pgImage == "" || appImage == "" {
		t.Skip("explicit local Restic, PostgreSQL and Node/pg fixture images required")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	run := func(ctx context.Context, binary string, args ...string) error {
		c := exec.CommandContext(ctx, binary, args...)
		if c.Run() != nil {
			return fmt.Errorf("fixture command failed (output withheld)")
		}
		return nil
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	marker := hex.EncodeToString(nonce[:])
	name := "homevault-pg-fixture-" + marker
	user := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	args := []string{"run", "--detach", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--memory", "256m", "--cpus", "0.5", "--pids-limit", "64", "--log-driver", "none", "--user", user, "--tmpfs", "/tmp:rw,noexec,nosuid,size=32m,mode=1777", "--tmpfs", "/var/run/postgresql:rw,noexec,nosuid,size=1m,mode=1777", "--tmpfs", "/var/lib/postgresql/data:rw,noexec,nosuid,size=128m,mode=1777", "--mount", "type=bind,source=" + root + ",target=" + root, "-e", "POSTGRES_HOST_AUTH_METHOD=trust", "-e", "POSTGRES_DB=fixture", "-e", "PGDATA=/var/lib/postgresql/data/pgdata", pgImage, "postgres", "-c", "listen_addresses=127.0.0.1", "-c", "unix_socket_directories=/var/run/postgresql", "-c", "shared_buffers=16MB"}
	if err := run(ctx, "docker", args...); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		if err := run(cleanup, "docker", "rm", "-f", "--volumes", name); err != nil {
			t.Error("source fixture cleanup failed")
		}
	}()
	deadline := time.Now().Add(45 * time.Second)
	for {
		if run(ctx, "docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "fixture") == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("source PostgreSQL not ready")
		}
		time.Sleep(250 * time.Millisecond)
	}
	hash := sha256.Sum256([]byte("fixture-only"))
	sql := "CREATE TABLE proof(marker text);INSERT INTO proof VALUES ('" + marker + "');CREATE TABLE accounts(username text,password_hash text);INSERT INTO accounts VALUES ('fixture','" + hex.EncodeToString(hash[:]) + "');"
	if err := run(ctx, "docker", "exec", name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "fixture", "-v", "ON_ERROR_STOP=1", "-c", sql); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "app")
	media := filepath.Join(root, "media")
	os.Mkdir(app, 0700)
	os.Mkdir(media, 0700)
	if err := os.WriteFile(filepath.Join(app, "app.cjs"), postgresFixtureApp, 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(media, "proof.txt"), []byte("restored-pg-media"), 0600)
	password := filepath.Join(root, "restic-password")
	pgpass := filepath.Join(root, "pgpass")
	os.WriteFile(password, []byte("fixture-only"), 0600)
	os.WriteFile(pgpass, []byte("127.0.0.1:5432:*:postgres:fixture-only\n"), 0600)
	// Test-only transport wrappers keep the host free of PostgreSQL packages.
	// Only this test root is mounted, not any production storage/socket.
	wrapper := func(nameOfTool string) string {
		p := filepath.Join(root, nameOfTool)
		script := "#!/bin/sh\nexec docker --host=unix:///var/run/docker.sock exec --user " + user + " -e PGPASSFILE=" + pgpass + " " + name + " " + nameOfTool + " \"$@\"\n"
		if err := os.WriteFile(p, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repo"), PasswordFile: password, ResticBinary: restic, PgDumpBinary: wrapper("pg_dump"), PgRestoreBinary: wrapper("pg_restore"), Projects: []config.Project{{Name: "postgres-demo", Paths: []string{app, media}, Databases: []config.Database{{Name: "main", Type: "postgres", Host: "127.0.0.1", Port: 5432, Database: "fixture", User: "postgres", PasswordFile: pgpass}}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "postgres-demo"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := e.Snapshots(ctx, "postgres-demo")
	if err != nil || len(snapshots) != 1 {
		t.Fatal("missing PostgreSQL snapshot", err)
	}
	// Save a custom archive for a deliberately broken SQL import fixture.
	archive := filepath.Join(root, "archive.dump")
	if err = run(ctx, "docker", "exec", name, "pg_dump", "-h", "127.0.0.1", "-U", "postgres", "-d", "fixture", "-Fc", "--file", archive); err != nil {
		t.Fatal(err)
	}
	if err = run(ctx, "docker", "stop", "--time", "5", name); err != nil {
		t.Fatal(err)
	}
	plan := engine.RecoveryPlan{Image: appImage, Command: []string{"node", "/data/app/app.cjs"}, Postgres: &engine.RecoveryPostgres{Image: pgImage, Artifact: "main"}, Mounts: []engine.RecoveryMount{{Source: strings.TrimPrefix(app, "/"), Target: "/data/app"}, {Source: strings.TrimPrefix(media, "/"), Target: "/data/media"}}}
	for _, c := range []struct{ kind, script string }{
		{"health", `const r=await fetch('http://127.0.0.1:8000/health');if(!r.ok||(await r.json()).status!=='ok')throw Error('health')`},
		{"database", `const {createRequire}=await import('node:module');const {Client}=createRequire('/app/package.json')('pg');const c=new Client({host:'127.0.0.1',user:'postgres',database:'recovery'});await c.connect();const r=await c.query('SELECT marker FROM proof');if(r.rows[0]?.marker!==` + strconv.Quote(marker) + `)throw Error('canary');await c.end()`},
		{"media", `const r=await fetch('http://127.0.0.1:8000/media');if(!r.ok||await r.text()!=='restored-pg-media')throw Error('media')`},
		{"login", `const base='http://127.0.0.1:8000';if((await fetch(base+'/dashboard')).status!==401)throw Error('auth required');const r=await fetch(base+'/login',{method:'POST',body:'fixture-only'});if(!r.ok||!(await r.json()).authenticated)throw Error('login');const cookie=r.headers.getSetCookie().map(s=>s.split(';')[0]).join('; ');const d=await fetch(base+'/dashboard',{headers:{Cookie:cookie}});if(!d.ok||(await d.json()).marker!==` + strconv.Quote(marker) + `)throw Error('session')`},
	} {
		plan.Checks = append(plan.Checks, engine.RecoveryCheck{Kind: c.kind, Command: []string{"node", "--input-type=module", "-e", c.script}})
	}
	reportPath, err := e.Rehearse(ctx, "postgres-demo", snapshots[0].ID, filepath.Join(root, "restore"), plan)
	if err != nil {
		t.Fatalf("PostgreSQL rehearsal: %v report=%s", err, reportPath)
	}
	b, _ := os.ReadFile(reportPath)
	var report engine.RecoveryReport
	if json.Unmarshal(b, &report) != nil || report.Status != "passed" || !report.DatabaseImported || !report.CleanupPassed || len(report.Checks) != 4 {
		t.Fatalf("invalid PostgreSQL report: %s", b)
	}
	checkClean := func(r engine.RecoveryReport) {
		t.Helper()
		out, err := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+r.ComposeProject).Output()
		if err != nil || strings.TrimSpace(string(out)) != "" {
			t.Fatal("recovery stack leaked")
		}
	}
	checkClean(report)
	composeData, _ := os.ReadFile(filepath.Join(filepath.Dir(reportPath), "compose.json"))
	var rendered struct {
		Services map[string]struct {
			Network  string `json:"network_mode"`
			Ports    []any  `json:"ports"`
			ReadOnly bool   `json:"read_only"`
		} `json:"services"`
	}
	if json.Unmarshal(composeData, &rendered) != nil || rendered.Services["database"].Network != "none" || rendered.Services["app"].Network != "service:database" || len(rendered.Services["app"].Ports) != 0 || len(rendered.Services["database"].Ports) != 0 || !rendered.Services["database"].ReadOnly {
		t.Fatal("multi-container network isolation not enforced")
	}
	t.Log("PASS PostgreSQL prepared backup, encrypted restore, automatic import, app health/DB/media/session and both-container cleanup")
	// Same-length SQL corruption leaves a readable custom archive, but actual
	// import must fail. This never alters a production database or repository.
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(raw), "CREATE TABLE public.proof", "BROKEN TABLE public.proof", 1)
	if broken == string(raw) {
		t.Fatal("could not construct invalid SQL fixture")
	}
	badDir := filepath.Join(root, "bad", "prepared-invalid")
	os.MkdirAll(badDir, 0700)
	os.WriteFile(filepath.Join(badDir, "main.dump"), []byte(broken), 0600)
	os.WriteFile(filepath.Join(badDir, "manifest.json"), []byte(`{"project":"postgres-demo"}`), 0600)
	cmd := exec.CommandContext(ctx, restic, "--repo", cfg.Repository, "backup", "--tag", "homevault:postgres-demo", "--", filepath.Dir(badDir), app, media)
	cmd.Env = append(os.Environ(), "RESTIC_PASSWORD_FILE="+password)
	if cmd.Run() != nil {
		t.Fatal("cannot back up invalid SQL fixture")
	}
	badSnapshots, err := e.Snapshots(ctx, "postgres-demo")
	if err != nil {
		t.Fatal(err)
	}
	badID := ""
	for _, s := range badSnapshots {
		if s.ID != snapshots[0].ID {
			badID = s.ID
		}
	}
	if badID == "" {
		t.Fatal("invalid SQL snapshot missing")
	}
	failedPath, failedErr := e.Rehearse(ctx, "postgres-demo", badID, filepath.Join(root, "failed-restore"), plan)
	if failedErr == nil {
		t.Fatal("invalid SQL import unexpectedly passed")
	}
	b, _ = os.ReadFile(failedPath)
	var failed engine.RecoveryReport
	if json.Unmarshal(b, &failed) != nil || failed.Status != "failed" || failed.DatabaseImported || !failed.CleanupPassed || len(failed.Checks) != 0 {
		t.Fatalf("invalid failure report: %s", b)
	}
	checkClean(failed)
	t.Log("PASS invalid PostgreSQL SQL import fails before app checks; both containers cleaned up")
}
