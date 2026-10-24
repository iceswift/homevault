package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"homevault/internal/config"
	"homevault/internal/lock"
)

func fixture(t *testing.T) *Engine {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "photos with spaces; echo secret")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	password := filepath.Join(root, "password")
	if err := os.WriteFile(password, []byte("secret-test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repo"), PasswordFile: password, ResticBinary: binary, Projects: []config.Project{{Name: "photos", Paths: []string{source}}}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestExplicitRcloneConfigIsolationAndLock(t *testing.T) {
	e := fixture(t)
	file := filepath.Join(t.TempDir(), "private-rclone.conf")
	if err := os.WriteFile(file, []byte("synthetic config only"), 0600); err != nil {
		t.Fatal(err)
	}
	e.cfg.RcloneConfigFile = file
	e.rclone = "synthetic-rclone"
	t.Setenv("RCLONE_CONFIG", "other-account")
	t.Setenv("RCLONE_CONFIG_GDRIVE_TOKEN", "other-account-token")
	t.Setenv("RCLONE_DRIVE_TOKEN", "other-token")
	e.run = func(_ context.Context, _ []string, env []string, _ bool) ([]byte, error) {
		count := 0
		for _, entry := range env {
			if strings.HasPrefix(entry, "RCLONE_") {
				if entry != "RCLONE_CONFIG="+file {
					t.Fatal("ambient rclone override inherited")
				}
				count++
			}
		}
		if count != 1 {
			t.Fatal("explicit credential config missing")
		}
		if _, err := lock.AcquireCredential(file); !errors.Is(err, lock.ErrLocked) {
			t.Fatal("credential refresh not locked")
		}
		return nil, nil
	}
	if _, err := e.command(context.Background(), "synthetic config check", false, "check"); err != nil {
		t.Fatal(err)
	}
	if err := e.validateRestoreTarget(filepath.Dir(file)); err == nil {
		t.Fatal("restore can overwrite credential config")
	}
}

func TestExplicitCredentialPathChangesRepositoryProofIdentity(t *testing.T) {
	e := fixture(t)
	e.rclone = "synthetic-rclone"
	legacy := e.repositoryID()
	e.cfg.RcloneConfigFile = filepath.Join(t.TempDir(), "rclone.conf")
	first := e.repositoryID()
	e.cfg.RcloneConfigFile = filepath.Join(t.TempDir(), "rclone.conf")
	if first == legacy || first == e.repositoryID() {
		t.Fatal("credential path change reused previous repository proof")
	}
	e.cfg.RcloneConfigFile = ""
	if legacy != e.repositoryID() {
		t.Fatal("legacy identity changed")
	}
}

func TestBackupArgumentsEnvironmentAndExplicitInit(t *testing.T) {
	e := fixture(t)
	t.Setenv("RESTIC_PASSWORD", "never-propagate")
	t.Setenv("RESTIC_PASSWORD_COMMAND", "never-execute")
	t.Setenv("RESTIC_REPOSITORY", "never-use")
	var calls [][]string
	e.run = func(ctx context.Context, args, env []string, capture bool) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		for _, entry := range env {
			if strings.Contains(entry, "never-") {
				t.Fatalf("inherited restic setting propagated: %s", entry)
			}
		}
		want := "RESTIC_PASSWORD_FILE=" + e.cfg.PasswordFile
		found := false
		for _, entry := range env {
			found = found || entry == want
		}
		if !found {
			t.Error("password file environment variable missing")
		}
		if capture {
			t.Error("backup/init should not capture stdout")
		}
		if _, err := lock.Acquire(e.cfg.StateDir); !errors.Is(err, lock.ErrLocked) {
			t.Fatalf("command ran without global lock: %v", err)
		}
		return nil, nil
	}
	if err := e.Backup(context.Background(), "photos"); err != nil {
		t.Fatal(err)
	}
	want := []string{"--repo", e.cfg.Repository, "--cache-dir", filepath.Join(e.cfg.StateDir, "cache"), "backup", "--tag", "homevault:photos", "--", e.cfg.Projects[0].Paths[0]}
	if !reflect.DeepEqual(calls, [][]string{want}) {
		t.Fatalf("backup arguments = %#v", calls)
	}
	if err := e.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := calls[1][len(calls[1])-1]; got != "init" {
		t.Fatalf("init command = %s", got)
	}
}

func TestCommandFailureRedactsSecretsAndReleasesLock(t *testing.T) {
	e := fixture(t)
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) {
		return []byte("SECRET-STDOUT"), errors.New("SECRET-STDERR/password/location")
	}
	if err := e.Backup(context.Background(), "photos"); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("missing/redaction-unsafe error: %v", err)
	}
	acquired, err := lock.Acquire(e.cfg.StateDir)
	if err != nil {
		t.Fatal("failure leaked lock:", err)
	}
	if err := acquired.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreVerifiesProjectAndEmptyDestination(t *testing.T) {
	e := fixture(t)
	id := strings.Repeat("a", 64)
	wrong := strings.Repeat("b", 64)
	var calls [][]string
	e.run = func(_ context.Context, args, _ []string, capture bool) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if capture {
			return json.Marshal([]Snapshot{{ID: id, Tags: []string{"homevault:photos"}}, {ID: wrong, Tags: []string{"homevault:other"}}})
		}
		return nil, nil
	}
	target := filepath.Join(t.TempDir(), "new", "restore")
	if err := e.Restore(context.Background(), "photos", wrong, target); err == nil {
		t.Fatal("wrong-project snapshot restored")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("rejected restore created target")
	}
	if len(calls) != 1 {
		t.Fatal("wrong-project restore executed")
	}
	if err := e.Restore(context.Background(), "photos", "latest", target); err == nil {
		t.Fatal("implicit snapshot selector accepted")
	}
	if err := e.Restore(context.Background(), "photos", id, target); err != nil {
		t.Fatal(err)
	}
	last := calls[len(calls)-1]
	want := []string{"--repo", e.cfg.Repository, "--cache-dir", filepath.Join(e.cfg.StateDir, "cache"), "restore", id, "--target", target, "--overwrite", "never"}
	if !reflect.DeepEqual(last, want) {
		t.Fatalf("restore arguments = %#v", last)
	}
	if err := os.WriteFile(filepath.Join(target, "keep.txt"), []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	count := len(calls)
	if err := e.Restore(context.Background(), "photos", id, target); err == nil {
		t.Fatal("nonempty target accepted")
	}
	if len(calls) != count {
		t.Fatal("command executed against nonempty target")
	}
	data, _ := os.ReadFile(filepath.Join(target, "keep.txt"))
	if string(data) != "keep me" {
		t.Fatal("existing target changed")
	}
}

func TestRestoreRejectsProtectedOverlaps(t *testing.T) {
	e := fixture(t)
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) {
		t.Fatal("restic must not run for unsafe destination")
		return nil, nil
	}
	targets := []string{e.cfg.StateDir, e.cfg.Repository, e.cfg.PasswordFile, filepath.Dir(e.cfg.StateDir), e.cfg.Projects[0].Paths[0], filepath.Join(e.cfg.Projects[0].Paths[0], "restored")}
	for _, target := range targets {
		if err := e.Restore(context.Background(), "photos", strings.Repeat("a", 64), target); err == nil {
			t.Fatalf("protected destination accepted: %s", target)
		}
	}
}

func TestLostSourceAllowsRecoveryButBlocksBackup(t *testing.T) {
	e := fixture(t)
	if err := os.Remove(e.cfg.Projects[0].Paths[0]); err != nil {
		t.Fatal(err)
	}
	// Reconstructing the engine after a restart must work without the lost disk.
	recovered, err := New(e.cfg)
	if err != nil {
		t.Fatalf("missing source blocked engine creation: %v", err)
	}
	id := strings.Repeat("a", 64)
	calls := 0
	recovered.run = func(_ context.Context, args, _ []string, capture bool) ([]byte, error) {
		calls++
		if capture {
			return json.Marshal([]Snapshot{{ID: id, Tags: []string{"homevault:photos"}}})
		}
		return nil, nil
	}
	if _, err := recovered.Snapshots(context.Background(), "photos"); err != nil {
		t.Fatalf("lost source blocked snapshot inspection: %v", err)
	}
	if err := recovered.Check(context.Background()); err != nil {
		t.Fatalf("lost source blocked repository check: %v", err)
	}
	target := filepath.Join(t.TempDir(), "recovered")
	if err := recovered.Restore(context.Background(), "photos", id, target); err != nil {
		t.Fatalf("lost source blocked restore: %v", err)
	}
	before := calls
	if err := recovered.Backup(context.Background(), "photos"); err == nil {
		t.Fatal("backup accepted missing source")
	}
	if calls != before {
		t.Fatal("restic ran before backup rejected missing source")
	}
}

func TestSnapshotFilteringAndInvalidJSON(t *testing.T) {
	e := fixture(t)
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) {
		return []byte(`[{"id":"` + strings.Repeat("a", 64) + `","tags":["homevault:other"]},{"id":"` + strings.Repeat("b", 64) + `","tags":["homevault:photos"]}]`), nil
	}
	snapshots, err := e.Snapshots(context.Background(), "photos")
	if err != nil || len(snapshots) != 1 || snapshots[0].ShortID != "bbbbbbbb" {
		t.Fatalf("snapshots = %+v, %v", snapshots, err)
	}
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) { return []byte("SECRET invalid"), nil }
	if _, err := e.Snapshots(context.Background(), "photos"); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("invalid JSON error = %v", err)
	}
}

func TestCanceledOperationDoesNotExecute(t *testing.T) {
	e := fixture(t)
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) {
		t.Fatal("canceled command executed")
		return nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.Check(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestExecuteDoesNotInterpretShellAndBoundsOutput(t *testing.T) {
	e := fixture(t)
	value := "space ; $(echo do-not-execute) & value"
	args := []string{"-test.run=TestCommandHelper", "--", "echo", value}
	data, err := e.execute(context.Background(), args, append(os.Environ(), "HOMEVAULT_TEST_HELPER=1"), true)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got, []string{"echo", value}) {
		t.Fatalf("literal args = %#v, err=%v, output=%s", got, err, data)
	}
	var buffer boundedBuffer
	if _, err := buffer.Write(make([]byte, 8*1024*1024+1)); !errors.Is(err, errOutputLimit) || buffer.Len() != 0 {
		t.Fatalf("output bound = len %d, error %v", buffer.Len(), err)
	}
	data, err = e.execute(context.Background(), []string{"-test.run=TestCommandHelper", "--", "oversized"}, append(os.Environ(), "HOMEVAULT_TEST_HELPER=1"), true)
	if err == nil || len(data) > 8*1024*1024 {
		t.Fatalf("real command output bound = len %d, error %v", len(data), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = e.execute(ctx, []string{"-test.run=TestCommandHelper", "--", "sleep"}, append(os.Environ(), "HOMEVAULT_TEST_HELPER=1"), false)
	if err == nil || ctx.Err() == nil {
		t.Fatalf("child not canceled: %v", err)
	}
}

func TestNonzeroChildExitIsFailureAndStderrIsSuppressed(t *testing.T) {
	e := fixture(t)
	e.run = func(ctx context.Context, args, env []string, capture bool) ([]byte, error) {
		return e.execute(ctx, []string{"-test.run=TestCommandHelper", "--", "fail"}, append(env, "HOMEVAULT_TEST_HELPER=1"), capture)
	}
	if err := e.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "exit code 7") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("nonzero child error = %v", err)
	}
}

func TestCommandHelper(t *testing.T) {
	if os.Getenv("HOMEVAULT_TEST_HELPER") != "1" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) > 0 && args[0] == "sleep" {
		time.Sleep(time.Minute)
	}
	if len(args) > 0 && args[0] == "fail" {
		_, _ = os.Stderr.WriteString("SECRET backend credentials\n")
		os.Exit(7)
	}
	if len(args) > 0 && args[0] == "oversized" {
		chunk := make([]byte, 64*1024)
		for i := 0; i < 256; i++ {
			if _, err := os.Stdout.Write(chunk); err != nil {
				os.Exit(0)
			}
		}
		os.Exit(0)
	}
	_ = json.NewEncoder(os.Stdout).Encode(args)
	os.Exit(0)
}
