//go:build linux

package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/engine"
	"homevault/internal/recovery"
)

func TestFreshContainerApplicationDisasterRecovery(t *testing.T) {
	age, restic, cli, image := os.Getenv("HOMEVAULT_TEST_AGE"), os.Getenv("HOMEVAULT_TEST_RESTIC"), os.Getenv("HOMEVAULT_TEST_KIT_BINARY"), os.Getenv("HOMEVAULT_TEST_DISASTER_IMAGE")
	if age == "" || restic == "" || cli == "" || image == "" {
		t.Skip("explicit age/Restic/CLI and preinstalled Python fixture image required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	root := t.TempDir()
	original := filepath.Join(root, "original")
	custody := filepath.Join(root, "custody")
	app := filepath.Join(original, "app")
	tools := filepath.Join(root, "tools")
	newMachine := filepath.Join(root, "new-machine")
	for _, p := range []string{original, custody, app, tools, newMachine} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, source := range map[string]string{"age": age, "restic": restic, "homevault": cli} {
		in, err := os.Open(source)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.OpenFile(filepath.Join(tools, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			in.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(out, in)
		in.Close()
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatal("cannot copy fixture tool")
		}
	}
	if err := os.WriteFile(filepath.Join(app, "app.py"), fixtureApp, 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(app, "proof.txt"), []byte("restored-media"), 0600)
	database := filepath.Join(original, "live.db")
	key := filepath.Join(original, "restic-key")
	os.WriteFile(key, []byte("generated-disaster-restic-key"), 0600)
	if exec.CommandContext(ctx, "python3", "-I", "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute('create table proof(value text)');c.execute("insert into proof values ('verified')");c.commit()`, database).Run() != nil {
		t.Fatal("cannot create generated fixture database")
	}
	repo := filepath.Join(root, "external-repository")
	cfg := config.Config{Version: 1, StateDir: filepath.Join(original, "state"), Repository: repo, PasswordFile: key, ResticBinary: restic, Projects: []config.Project{{Name: "fixture", Paths: []string{app}, Databases: []config.Database{{Name: "main", Type: "sqlite", Source: database}}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := e.Snapshots(ctx, "fixture")
	if err != nil || len(snapshots) != 1 {
		t.Fatal("fixture snapshot missing", err)
	}
	identity := filepath.Join(custody, "identity.txt")
	keygen := filepath.Join(filepath.Dir(age), "age-keygen")
	if exec.CommandContext(ctx, keygen, "-o", identity).Run() != nil {
		t.Fatal("cannot create fixture off-server identity")
	}
	public, err := exec.CommandContext(ctx, keygen, "-y", identity).Output()
	if err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(custody, "fixture.age")
	plan := engine.RecoveryPlan{Image: image, Command: []string{"python", "-I", "/data/app/app.py"}, Mounts: []engine.RecoveryMount{{Source: strings.TrimPrefix(app, "/"), Target: "/data/app"}, {Prepared: true, Target: "/data/db"}}}
	for _, check := range []struct{ kind, script string }{
		{"health", `import urllib.request,json;assert json.load(urllib.request.urlopen('http://127.0.0.1:8000/health'))['status']=='ok'`},
		{"database", `import sqlite3;c=sqlite3.connect('file:/data/db/main.sqlite?mode=ro',uri=True);assert c.execute('select value from proof').fetchone()==('verified',)`},
		{"media", `import urllib.request;assert urllib.request.urlopen('http://127.0.0.1:8000/media').read()==b'restored-media'`},
		{"login", `import urllib.request,json;assert json.load(urllib.request.urlopen('http://127.0.0.1:8000/login',data=b'fixture-only'))['authenticated']`},
	} {
		plan.Checks = append(plan.Checks, engine.RecoveryCheck{Kind: check.kind, Command: []string{"python", "-I", "-c", check.script}})
	}
	planFile := filepath.Join(original, "recovery-plan.json")
	planData, _ := json.Marshal(plan)
	if err = os.WriteFile(planFile, planData, 0600); err != nil {
		t.Fatal(err)
	}
	info, err := recovery.Export(ctx, cfg, "fixture", snapshots[0].ID, kit, strings.TrimSpace(string(public)), planFile, age)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "drill.py")
	os.WriteFile(script, disasterSupervisor, 0600)
	if err = os.RemoveAll(original); err != nil {
		t.Fatal(err)
	} // generated machine only
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "homevault-disaster-" + hex.EncodeToString(nonce[:])
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
	})
	hash := sha256.Sum256(fixtureApp)
	args := []string{"run", "--rm", "--pull=never", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--memory", "256m", "--cpus", "0.5", "--pids-limit", "64", "--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=32m", "-e", "GOMAXPROCS=2", "-e", "ORIGINAL_PATH=" + original, "-e", "APP_SHA256=" + hex.EncodeToString(hash[:])}
	for _, m := range []struct {
		source, target string
		ro             bool
	}{{tools, "/tools", true}, {repo, "/repository", false}, {newMachine, "/data", false}, {kit, "/kit/fixture.age", true}, {identity, "/custody/identity.txt", true}, {script, "/drill.py", true}} {
		mount := "type=bind,source=" + m.source + ",target=" + m.target
		if m.ro {
			mount += ",readonly"
		}
		args = append(args, "--mount", mount)
	}
	args = append(args, image, "python", "-I", "/drill.py")
	cmd := exec.CommandContext(ctx, "docker", args...)
	data, err := cmd.Output()
	if err != nil {
		t.Fatal("fresh-container disaster drill failed; child output withheld", err)
	}
	var report struct {
		OriginalAbsent bool   `json:"original_absent"`
		RecoveryOnly   bool   `json:"recovery_only"`
		Health         bool   `json:"health"`
		Database       bool   `json:"database"`
		Media          bool   `json:"media"`
		Login          bool   `json:"login"`
		Kit            string `json:"kit_sha256"`
	}
	if json.Unmarshal(data, &report) != nil || !report.OriginalAbsent || !report.RecoveryOnly || !report.Health || !report.Database || !report.Media || !report.Login || report.Kit != info.KitSHA256 {
		t.Fatal("fresh-container recovery report invalid")
	}
	remaining, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "name=^/"+name+"$").Output()
	if err != nil || strings.TrimSpace(string(remaining)) != "" {
		t.Fatal("fixture container not cleaned up")
	}
	t.Log("PASS fresh network-none container with no original source/state/key mount: kit opened, mutation refused, DB/media/app source restored, health/database/media/login passed, container removed")
}
