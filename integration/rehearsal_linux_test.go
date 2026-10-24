//go:build linux

package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/engine"
)

func TestIsolatedApplicationRehearsal(t *testing.T) {
	restic := os.Getenv("HOMEVAULT_TEST_RESTIC")
	image := os.Getenv("HOMEVAULT_TEST_RECOVERY_IMAGE")
	if restic == "" || image == "" {
		t.Skip("real Restic and explicitly installed recovery fixture image required")
	}
	root := t.TempDir()
	app := filepath.Join(root, "app")
	os.Mkdir(app, 0700)
	// Source bundled into the test binary to avoid relying on the remote working directory.
	if err := os.WriteFile(filepath.Join(app, "app.py"), fixtureApp, 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(app, "proof.txt"), []byte("restored-media"), 0600)
	source := filepath.Join(root, "live.db")
	key := filepath.Join(root, "password")
	os.WriteFile(key, []byte("fixture-only"), 0600)
	if out, err := exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute('create table proof(value text)');c.execute("insert into proof values ('verified')");c.commit()`, source).CombinedOutput(); err != nil {
		t.Fatalf("fixture setup %v %s", err, out)
	}
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repo"), PasswordFile: key, ResticBinary: restic, Projects: []config.Project{{Name: "fixture", Paths: []string{app}, Databases: []config.Database{{Name: "main", Type: "sqlite", Source: source}}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := e.Snapshots(ctx, "fixture")
	if err != nil || len(snapshots) != 1 {
		t.Fatal("snapshot missing", err)
	}
	os.Remove(source) // Recovery cannot depend on the live database.
	plan := engine.RecoveryPlan{Image: image, Command: []string{"python", "-I", "/data/app/app.py"}, Mounts: []engine.RecoveryMount{{Source: strings.TrimPrefix(app, "/"), Target: "/data/app"}, {Prepared: true, Target: "/data/db"}}}
	for _, check := range []struct{ kind, script string }{
		{"health", `import urllib.request,json;assert json.load(urllib.request.urlopen('http://127.0.0.1:8000/health'))['status']=='ok'`},
		{"database", `import sqlite3;c=sqlite3.connect('file:/data/db/main.sqlite?mode=ro',uri=True);assert c.execute('select value from proof').fetchone()==('verified',)`},
		{"media", `import urllib.request;assert urllib.request.urlopen('http://127.0.0.1:8000/media').read()==b'restored-media'`},
		{"login", `import urllib.request,json;assert json.load(urllib.request.urlopen('http://127.0.0.1:8000/login',data=b'fixture-only'))['authenticated']`},
	} {
		plan.Checks = append(plan.Checks, engine.RecoveryCheck{Kind: check.kind, Command: []string{"python", "-I", "-c", check.script}})
	}
	reportPath, err := e.Rehearse(ctx, "fixture", snapshots[0].ID, filepath.Join(root, "restore"), plan)
	if err != nil {
		t.Fatalf("rehearsal: %v report=%s", err, reportPath)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report engine.RecoveryReport
	if json.Unmarshal(data, &report) != nil || report.Status != "passed" || !report.CleanupPassed || len(report.Checks) != 4 {
		t.Fatalf("invalid report: %s", data)
	}
	composeData, err := os.ReadFile(filepath.Join(filepath.Dir(reportPath), "compose.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rendered struct {
		Services map[string]struct {
			Network  string   `json:"network_mode"`
			ReadOnly bool     `json:"read_only"`
			Ports    []any    `json:"ports"`
			CapDrop  []string `json:"cap_drop"`
		} `json:"services"`
	}
	if json.Unmarshal(composeData, &rendered) != nil || rendered.Services["app"].Network != "none" || !rendered.Services["app"].ReadOnly || len(rendered.Services["app"].Ports) != 0 || len(rendered.Services["app"].CapDrop) != 1 || rendered.Services["app"].CapDrop[0] != "ALL" {
		t.Fatal("unsafe generated Compose policy")
	}
	// Docker's project label must no longer identify any running/stopped container.
	output, err := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+report.ComposeProject).Output()
	if err != nil || strings.TrimSpace(string(output)) != "" {
		t.Fatal("recovery container leaked")
	}
	t.Log("PASS encrypted restore, application health/database/media/login and isolated Compose cleanup")
	// A failed/cancelled check must not produce a passing report or leave the
	// Compose app running. Cleanup has its own context, independent of timeout.
	plan.Checks[0].Command = []string{"python", "-I", "-c", "raise SystemExit(7)"}
	failedCtx, stop := context.WithTimeout(ctx, 8*time.Second)
	defer stop()
	failedReport, failedErr := e.Rehearse(failedCtx, "fixture", snapshots[0].ID, filepath.Join(root, "failed-restore"), plan)
	if failedErr == nil || failedReport == "" {
		t.Fatal("failed check was reported as successful")
	}
	data, err = os.ReadFile(failedReport)
	if err != nil {
		t.Fatal(err)
	}
	var failure engine.RecoveryReport
	if json.Unmarshal(data, &failure) != nil || failure.Status != "failed" || !failure.CleanupPassed || failure.Checks["health"] {
		t.Fatalf("invalid failed report: %s", data)
	}
	output, err = exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+failure.ComposeProject).Output()
	if err != nil || strings.TrimSpace(string(output)) != "" {
		t.Fatal("failed recovery left a container")
	}
	t.Log("PASS failed check/timeout report and independent cleanup")
	if _, err := e.Snapshots(ctx, "fixture"); err != nil {
		t.Fatal("failed rehearsal leaked the operation lock", err)
	}
}
