//go:build linux

package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

// This opts into a reviewed, already-installed DealDoor demo image. It never
// mounts production data or reads a production .env. All users/media are seeded
// from that image into a new private fixture directory.
func TestDealDoorDemoRecovery(t *testing.T) {
	image := os.Getenv("HOMEVAULT_TEST_DEALDOOR_IMAGE")
	restic := os.Getenv("HOMEVAULT_TEST_RESTIC")
	if image == "" || restic == "" {
		t.Skip("explicit local DealDoor demo image and Restic required")
	}
	root := t.TempDir()
	data := filepath.Join(root, "seed-data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	docker := func(ctx context.Context, args ...string) error {
		c := exec.CommandContext(ctx, "docker", args...)
		c.Env = append(os.Environ(), "DOCKER_HOST=unix:///var/run/docker.sock", "COMPOSE_DISABLE_ENV_FILE=1")
		if c.Run() != nil {
			return fmt.Errorf("fixture Docker command failed (output withheld)")
		}
		return nil
	}
	if err := docker(ctx, "image", "inspect", image); err != nil {
		t.Fatal("review/install image first", err)
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("cannot generate fixture identity", err)
	}
	name := "homevault-dealdoor-fixture-" + hex.EncodeToString(nonce[:])
	created := false
	defer func() {
		if !created {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := docker(cleanup, "rm", "-f", name); err != nil {
			t.Error("fixture container cleanup failed")
		}
	}()
	command := []string{"env", "NODE_ENV=production", "DEMO_MODE=true", "DEMO_ALLOW_REGISTRATION=false", "DEMO_ALLOW_UPLOADS=false", "DB_DRIVER=sqlite", "DATABASE_URL=/data/db/main.sqlite", "STORAGE_DRIVER=local", "MEDIA_LOCAL_ROOT=/data/media", "HOST=127.0.0.1", "PORT=4321", "node", "scripts/start-server.mjs"}
	args := []string{"run", "--detach", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--memory", "256m", "--cpus", "0.5", "--pids-limit", "64", "--log-driver", "none", "--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "--tmpfs", "/tmp:rw,noexec,nosuid,size=32m,mode=1777", "--mount", "type=bind,source=" + data + ",target=/data", "--entrypoint", command[0], image}
	args = append(args, command[1:]...)
	if err := docker(ctx, args...); err != nil {
		t.Fatal(err)
	}
	created = true
	health := `const r=await fetch('http://127.0.0.1:4321/api/health');const j=await r.json();if(!r.ok||j.status!=='healthy'||j.checks.database.status!=='healthy')throw Error('health')`
	deadline := time.Now().Add(time.Minute)
	for {
		attempt, stop := context.WithTimeout(ctx, 5*time.Second)
		err := docker(attempt, "exec", name, "node", "--input-type=module", "-e", health)
		stop()
		if err == nil {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			t.Fatal("DealDoor demo startup did not become healthy")
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Canary is not part of DealDoor's seeder, so re-seeding an empty DB cannot
	// falsely satisfy the recovery proof.
	marker := hex.EncodeToString(nonce[:])
	sql := `import Database from 'better-sqlite3';const d=new Database('/data/db/main.sqlite');d.exec('CREATE TABLE homevault_recovery_proof(marker TEXT NOT NULL)');d.prepare('INSERT INTO homevault_recovery_proof VALUES (?)').run(` + strconv.Quote(marker) + `);d.close()`
	if err := docker(ctx, "exec", name, "node", "--input-type=module", "-e", sql); err != nil {
		t.Fatal("cannot add synthetic canary", err)
	}
	// Stop only this test-owned container before backup: fixture DB/media cannot
	// change during the backup. No application quiesce feature is implied.
	if err := docker(ctx, "stop", "--time", "5", name); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(data, "media")
	proof, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aV1kAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	proofPath := filepath.Join(media, "uploads", "homevault-proof.png")
	if err := os.WriteFile(proofPath, proof, 0600); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(root, "password")
	if err := os.WriteFile(key, []byte("fixture-only-not-production"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repository"), PasswordFile: key, ResticBinary: restic, Projects: []config.Project{{Name: "dealdoor-demo", Paths: []string{media}, Databases: []config.Database{{Name: "main", Type: "sqlite", Source: filepath.Join(data, "db", "main.sqlite")}}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "dealdoor-demo"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := e.Snapshots(ctx, "dealdoor-demo")
	if err != nil || len(snapshots) != 1 {
		t.Fatal("snapshot not created", err)
	}
	// Original DB is absent: the application must read the prepared backup.
	if err = os.Rename(filepath.Join(data, "db"), filepath.Join(data, "db-lost")); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(proof)
	database := `import Database from 'better-sqlite3';const d=new Database('/data/db/main.sqlite',{readonly:true});if(d.prepare('SELECT marker FROM homevault_recovery_proof').get()?.marker!==` + strconv.Quote(marker) + `)throw Error('canary missing');if(d.prepare('SELECT count(*) AS n FROM users').get().n<3)throw Error('demo users missing');d.close()`
	mediaCheck := `import {createHash} from 'node:crypto';const r=await fetch('http://127.0.0.1:4321/uploads/homevault-proof.png');if(!r.ok||createHash('sha256').update(Buffer.from(await r.arrayBuffer())).digest('hex')!==` + strconv.Quote(hex.EncodeToString(digest[:])) + `)throw Error('media mismatch')`
	// Credentials below are the public seed fixture, never an owner's account.
	login := `const base='http://127.0.0.1:4321';const a=await fetch(base+'/dashboard',{redirect:'manual'});if(a.status<300||a.status>=400)throw Error('dashboard must require auth');const r=await fetch(base+'/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json','Origin':base},body:JSON.stringify({email:'free-demo@dealdoor.co',password:'password123'})});const j=await r.json();const cookie=r.headers.getSetCookie().map(x=>x.split(';')[0]).join('; ');if(!r.ok||!j.success||j.data.email!=='free-demo@dealdoor.co'||!cookie)throw Error('login failed');const page=await fetch(base+'/dashboard',{redirect:'manual',headers:{Cookie:cookie}});if(page.status!==200||!(await page.text()).includes('html'))throw Error('session not accepted')`
	plan := engine.RecoveryPlan{Image: image, Command: command, Mounts: []engine.RecoveryMount{{Prepared: true, Target: "/data/db", Writable: true}, {Source: strings.TrimPrefix(media, "/"), Target: "/data/media"}}}
	for _, c := range []struct{ kind, script string }{{"health", health}, {"database", database}, {"media", mediaCheck}, {"login", login}} {
		plan.Checks = append(plan.Checks, engine.RecoveryCheck{Kind: c.kind, Command: []string{"node", "--input-type=module", "-e", c.script}})
	}
	reportPath, err := e.Rehearse(ctx, "dealdoor-demo", snapshots[0].ID, filepath.Join(root, "restore"), plan)
	if err != nil {
		t.Fatalf("DealDoor rehearsal failed: %v; report=%s", err, reportPath)
	}
	b, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report engine.RecoveryReport
	if json.Unmarshal(b, &report) != nil || report.Status != "passed" || !report.CleanupPassed || len(report.Checks) != 4 {
		t.Fatal("invalid recovery report")
	}
	t.Log("PASS actual DealDoor image with newly seeded fixtures: restored DB canary, media HTTP bytes, login cookie and authenticated dashboard; recovery container cleaned up")
}
