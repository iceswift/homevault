//go:build linux

package integration

import (
	"context"
	"encoding/json"
	"homevault/internal/config"
	"homevault/internal/engine"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFreshCaptureJobsAndOrphanRecovery(t *testing.T) {
	restic := os.Getenv("HOMEVAULT_TEST_RESTIC")
	image := os.Getenv("HOMEVAULT_TEST_CAPTURE_IMAGE")
	cli := os.Getenv("HOMEVAULT_TEST_CAPTURE_BINARY")
	if restic == "" || image == "" || cli == "" {
		t.Skip("explicit local Restic, Python fixture image and compiled HomeVault CLI required")
	}
	root := t.TempDir()
	data := filepath.Join(root, "live")
	os.Mkdir(data, 0700)
	media := filepath.Join(data, "media")
	os.Mkdir(media, 0700)
	os.WriteFile(filepath.Join(media, "proof.txt"), []byte("first"), 0600)
	db := filepath.Join(data, "main.sqlite")
	if exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute('pragma journal_mode=wal');c.execute('create table proof(v text)');c.execute("insert into proof values ('first')");c.commit()`, db).Run() != nil {
		t.Fatal("SQLite fixture failed")
	}
	app := `from http.server import HTTPServer,BaseHTTPRequestHandler
import sqlite3
c=sqlite3.connect('/data/main.sqlite')
c.execute('pragma journal_mode=wal')
c.execute('create table if not exists live_wal(v text)')
c.execute('delete from live_wal')
c.execute("insert into live_wal values ('kept')")
c.commit()
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*a):pass
 def do_GET(self):self.send_response(200);self.end_headers();self.wfile.write(b'ok')
HTTPServer(('127.0.0.1',8000),Handler).serve_forever()
`
	os.WriteFile(filepath.Join(data, "app.py"), []byte(app), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	docker := func(args ...string) []byte {
		t.Helper()
		c := exec.CommandContext(ctx, "docker", args...)
		b, err := c.Output()
		if err != nil {
			t.Fatal("fixture Docker operation failed; output withheld")
		}
		return b
	}
	healthCmd := `python -I -c "import urllib.request;assert urllib.request.urlopen('http://127.0.0.1:8000/').status==200"`
	id := strings.TrimSpace(string(docker("run", "--detach", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--memory", "128m", "--cpus", "0.5", "--pids-limit", "64", "--log-driver", "none", "--user", strconv.Itoa(os.Getuid())+":"+strconv.Itoa(os.Getgid()), "--mount", "type=bind,source="+data+",target=/data", "--health-cmd", healthCmd, "--health-interval", "1s", "--health-timeout", "2s", "--health-retries", "2", "--entrypoint", "python", image, "-I", "/data/app.py")))
	if len(id) != 64 {
		t.Fatal("invalid fixture container ID")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if exec.CommandContext(cleanup, "docker", "rm", "-f", "--volumes", id).Run() != nil {
			t.Error("fixture cleanup failed")
		}
	}()
	waitHealthy := func() {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for {
			b := docker("inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{end}}", id)
			if strings.TrimSpace(string(b)) == "healthy" {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("fixture app did not resume healthy")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	waitHealthy()
	if info, err := os.Stat(db + "-wal"); err != nil || info.Size() == 0 {
		t.Fatal("live WAL fixture missing")
	}
	imageID := strings.TrimSpace(string(docker("inspect", "--format", "{{.Image}}", id)))
	key := filepath.Join(root, "key")
	os.WriteFile(key, []byte("synthetic-only"), 0600)
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repo"), PasswordFile: key, ResticBinary: restic, Projects: []config.Project{{Name: "fresh", Paths: []string{media}, Databases: []config.Database{{Name: "main", Type: "sqlite", Source: db}}}}}
	cfg.Projects[0].CaptureRequired = true
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if e.Backup(ctx, "fresh") == nil || e.RunBackupJob(ctx, "fresh") == nil {
		t.Fatal("capture-required project accepted ordinary backup")
	}
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	plan := engine.CapturePlan{ContainerID: id, ImageID: imageID, DataRoot: data, WindowStart: "00:00", WindowMinutes: 1440, Timezone: "Asia/Bangkok", MaxPauseSeconds: 30, GuardianBinary: cli}
	if err = e.CheckCaptureSource(ctx, "fresh", plan); err != nil {
		t.Fatal("read-only capture preflight", err)
	}
	if err = e.RunCapturedJob(ctx, "fresh", plan); err != nil {
		t.Fatal("first fresh job", err)
	}
	waitHealthy()
	first, err := e.Snapshots(ctx, "fresh")
	if err != nil || len(first) != 1 {
		t.Fatal("first snapshot missing", err)
	}
	if exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);c.execute("update proof set v='second'");c.commit()`, db).Run() != nil {
		t.Fatal("second DB update failed")
	}
	os.WriteFile(filepath.Join(media, "proof.txt"), []byte("second"), 0600)
	if err = e.RunCapturedJob(ctx, "fresh", plan); err != nil {
		t.Fatal("second fresh job", err)
	}
	waitHealthy()
	snapshots, err := e.Snapshots(ctx, "fresh")
	if err != nil || len(snapshots) != 2 {
		t.Fatal("second snapshot missing", err)
	}
	for _, snapshot := range snapshots {
		want := "second"
		if snapshot.ID == first[0].ID {
			want = "first"
		}
		target := filepath.Join(root, "restore-"+want)
		if err = e.Restore(ctx, "fresh", snapshot.ID, target); err != nil {
			t.Fatal(err)
		}
		foundDB, foundMedia := false, false
		if err = filepath.WalkDir(target, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Name() == "main.sqlite" {
				foundDB = true
				c := exec.Command("python3", "-I", "-c", `import sqlite3,sys
c=sqlite3.connect(sys.argv[1]);assert c.execute('select v from proof').fetchone()==(sys.argv[2],);assert c.execute('select v from live_wal').fetchone()==('kept',)`, p, want)
				if c.Run() != nil {
					t.Fatal("restored DB did not match job version")
				}
			}
			if d.Name() == "proof.txt" {
				foundMedia = true
				b, _ := os.ReadFile(p)
				if string(b) != want {
					t.Fatal("restored media not fresh")
				}
			}
			return nil
		}); err != nil || !foundDB || !foundMedia {
			t.Fatal("restore proof missing", err)
		}
	}
	status, err := e.JobStatus("fresh")
	if err != nil || !status.Latest.CapturePassed || !status.Latest.CheckPassed {
		t.Fatal("capture job state invalid", err)
	}
	good := status.LastSuccess
	badCfg := cfg
	badCfg.PythonBinary = "/usr/bin/false"
	bad, err := engine.New(badCfg)
	if err != nil {
		t.Fatal(err)
	}
	if bad.RunCapturedJob(ctx, "fresh", plan) == nil {
		t.Fatal("capture failure incorrectly passed")
	}
	waitHealthy()
	failed, _ := bad.JobStatus("fresh")
	if failed.Latest.Status != "failed" || failed.Latest.CapturePassed || !failed.LastSuccess.Equal(good) {
		t.Fatal("capture failure advanced last success")
	}
	unchanged, err := e.Snapshots(ctx, "fresh")
	if err != nil || len(unchanged) != 2 {
		t.Fatal("failed capture created a snapshot", err)
	}
	// Simulate an orphaned pause with a missing backup key and stale operation
	// lock. Rescue CLI must not need Engine/config/repository access to restart.
	guardDir := filepath.Join(cfg.StateDir, "guards")
	os.MkdirAll(guardDir, 0700)
	guardFile := filepath.Join(guardDir, "orphan.json")
	guard := map[string]any{"version": 1, "container_id": id, "image_id": imageID, "nonce": strings.Repeat("c", 24), "deadline": time.Now().Add(-time.Second).UTC()}
	b, _ := json.Marshal(guard)
	os.WriteFile(guardFile, b, 0600)
	docker("stop", "--time", "5", id)
	os.Rename(key, key+"-away")
	os.Mkdir(filepath.Join(cfg.StateDir, "operation.lock"), 0700)
	rescue := exec.CommandContext(ctx, cli, "pause-guardian", guardFile)
	if rescue.Run() != nil {
		t.Fatal("independent orphan rescue failed")
	}
	waitHealthy()
	if _, err = os.Stat(guardFile); !os.IsNotExist(err) {
		t.Fatal("orphan guard not cleared")
	}
	if _, err = os.Stat(filepath.Join(cfg.StateDir, "operation.lock")); err != nil {
		t.Fatal("rescue removed backup operation lock")
	}
	t.Log("PASS two fresh DB/media versions, shared job lock/status, failed-capture resume, and orphan rescue independent of missing key/stale lock")
}
