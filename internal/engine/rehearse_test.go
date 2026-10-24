package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryPlanRejectsComposeFields(t *testing.T) {
	p := validPlan()
	data, _ := json.Marshal(p)
	file := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecoveryPlan(file); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`,"privileged":true}`, `,"ports":["8000:8000"]}`, `,"env_file":"/etc/secrets"}`} {
		unsafe := append(append([]byte{}, data[:len(data)-1]...), []byte(extra)...)
		if err := os.WriteFile(file, unsafe, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRecoveryPlan(file); err == nil {
			t.Fatal("arbitrary Compose field accepted")
		}
	}
}

func validPlan() RecoveryPlan {
	p := RecoveryPlan{Image: "python:3.12-alpine", Command: []string{"python", "/data/app.py"}, Mounts: []RecoveryMount{{Prepared: true, Target: "/data/db"}}}
	for _, kind := range []string{"health", "database", "media", "login"} {
		p.Checks = append(p.Checks, RecoveryCheck{Kind: kind, Command: []string{"true"}})
	}
	return p
}
func TestRecoveryPlanRejectsUnsafeMountsAndMissingChecks(t *testing.T) {
	if err := validPlan().validate(); err != nil {
		t.Fatal(err)
	}
	for _, m := range []RecoveryMount{{Source: "../prod", Target: "/data"}, {Source: "/srv/prod", Target: "/data"}, {Prepared: true, Target: "/var/run/docker.sock"}, {Prepared: true, Target: "/data/../etc"}, {Prepared: true, Source: "both", Target: "/data"}, {Target: "/data"}} {
		p := validPlan()
		p.Mounts = []RecoveryMount{m}
		if p.validate() == nil {
			t.Fatalf("unsafe mount accepted: %+v", m)
		}
	}
	p := validPlan()
	p.Checks = p.Checks[:3]
	if p.validate() == nil {
		t.Fatal("missing login check accepted")
	}
	p = validPlan()
	p.Mounts = append(p.Mounts, RecoveryMount{Source: "files", Target: "/data/db/child"})
	if p.validate() == nil {
		t.Fatal("overlapping mounts accepted")
	}
}
func TestRecoveryMountsRequireUniquePreparedManifest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "state", "prepared-one")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"project":"demo"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := recoveryMounts(root, "demo", validPlan()); err != nil {
		t.Fatal(err)
	}
	if _, err := recoveryMounts(root, "other", validPlan()); err == nil {
		t.Fatal("wrong project manifest accepted")
	}
	second := filepath.Join(root, "prepared-two")
	os.Mkdir(second, 0700)
	os.WriteFile(filepath.Join(second, "manifest.json"), []byte(`{"project":"demo"}`), 0600)
	if _, err := recoveryMounts(root, "demo", validPlan()); err == nil {
		t.Fatal("ambiguous prepared directories accepted")
	}
}

func TestPostgresRecoveryPolicy(t *testing.T) {
	p := validPlan()
	p.Postgres = &RecoveryPostgres{Image: "postgres:16-bookworm", Artifact: "main"}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{"../main", "/data/main", "--flag", "", "name;command"} {
		p.Postgres.Artifact = artifact
		if p.validate() == nil {
			t.Fatal("unsafe PostgreSQL artifact accepted")
		}
	}
	root := t.TempDir()
	dir := filepath.Join(root, "prepared-one")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"project":"demo"}`), 0600)
	if _, err := recoveryPostgresService(root, "demo", RecoveryPostgres{Image: "postgres:16-bookworm", Artifact: "main"}); err == nil {
		t.Fatal("missing dump accepted")
	}
	os.WriteFile(filepath.Join(dir, "main.dump"), []byte("fixture"), 0600)
	service, err := recoveryPostgresService(root, "demo", RecoveryPostgres{Image: "postgres:16-bookworm", Artifact: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if service["network_mode"] != "none" || service["read_only"] != true || service["ports"] != nil {
		t.Fatal("PostgreSQL isolation policy weakened")
	}
	mount := service["volumes"].([]any)[0].(map[string]any)
	if mount["read_only"] != true || mount["target"] != "/data/restore.dump" {
		t.Fatal("archive mount must be read-only")
	}
}
