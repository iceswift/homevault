package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RecoveryPlan is a deliberately restricted alternative to running arbitrary
// production Compose files. Image/commands must be reviewed and trusted.
type RecoveryPlan struct {
	Image    string            `json:"image"`
	Command  []string          `json:"command"`
	Mounts   []RecoveryMount   `json:"mounts"`
	Checks   []RecoveryCheck   `json:"checks"`
	Postgres *RecoveryPostgres `json:"postgres,omitempty"`
}

// RecoveryPostgres imports one trusted custom archive into an ephemeral local
// cluster. No production credentials, roles, network or storage are reused.
type RecoveryPostgres struct {
	Image    string `json:"image"`
	Artifact string `json:"artifact"`
}
type RecoveryMount struct {
	// Source is relative to the restored tree. Prepared selects the unique
	// manifest-bearing prepared-* directory instead; never a production path.
	Source   string `json:"source,omitempty"`
	Prepared bool   `json:"prepared,omitempty"`
	Target   string `json:"target"`
	Writable bool   `json:"writable,omitempty"`
}
type RecoveryCheck struct {
	Kind    string   `json:"kind"`
	Command []string `json:"command"`
}
type RecoveryReport struct {
	Project          string          `json:"project"`
	Snapshot         string          `json:"snapshot"`
	ComposeProject   string          `json:"compose_project"`
	Started          time.Time       `json:"started"`
	Finished         time.Time       `json:"finished"`
	Status           string          `json:"status"`
	RestorePassed    bool            `json:"restore_passed"`
	Checks           map[string]bool `json:"checks"`
	CleanupPassed    bool            `json:"cleanup_passed"`
	DatabaseImported bool            `json:"database_imported,omitempty"`
	// Never embed command output, environment, image metadata or credentials.
}

func LoadRecoveryPlan(filename string) (RecoveryPlan, error) {
	f, err := os.Open(filename)
	if err != nil {
		return RecoveryPlan{}, errors.New("cannot open recovery plan")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1024*1024+1))
	d.DisallowUnknownFields()
	var p RecoveryPlan
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF {
		return p, errors.New("invalid recovery plan JSON")
	}
	return p, p.validate()
}
func (p RecoveryPlan) validate() error {
	if p.Image == "" || strings.HasPrefix(p.Image, "-") || strings.ContainsAny(p.Image, "$ \x00\r\n") {
		return errors.New("recovery requires an explicit local image reference")
	}
	if p.Postgres != nil {
		if p.Postgres.Image == "" || strings.HasPrefix(p.Postgres.Image, "-") || strings.ContainsAny(p.Postgres.Image, "$ \x00\r\n") || !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`).MatchString(p.Postgres.Artifact) {
			return errors.New("postgres recovery requires a local image and safe artifact name")
		}
	}
	validCommand := func(a []string) bool {
		if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
			return false
		}
		for _, s := range a {
			if strings.ContainsRune(s, 0) {
				return false
			}
		}
		return true
	}
	if !validCommand(p.Command) || len(p.Mounts) == 0 {
		return errors.New("recovery requires explicit command and restored-data mounts")
	}
	for i, m := range p.Mounts {
		if (m.Source == "") == !m.Prepared {
			return errors.New("mount requires exactly one relative source or prepared selector")
		}
		if m.Source != "" && (path.IsAbs(m.Source) || filepath.IsAbs(m.Source) || filepath.Clean(m.Source) == "." || filepath.Clean(m.Source) == ".." || strings.HasPrefix(filepath.Clean(m.Source), ".."+string(filepath.Separator)) || strings.ContainsAny(m.Source, "\x00\r\n\\")) {
			return errors.New("mount sources must stay inside restored tree")
		}
		// Only /data descendants: no executable, socket, system or Docker mounts.
		if m.Target != "/data" && !strings.HasPrefix(m.Target, "/data/") || path.Clean(m.Target) != m.Target || strings.ContainsAny(m.Target, "\x00\r\n\\") {
			return errors.New("container mount targets must be /data or descendants")
		}
		for _, other := range p.Mounts[:i] {
			if other.Target == m.Target || strings.HasPrefix(m.Target, other.Target+"/") || strings.HasPrefix(other.Target, m.Target+"/") {
				return errors.New("container mount targets must not overlap")
			}
		}
	}
	seen := map[string]bool{}
	for _, c := range p.Checks {
		if !validCommand(c.Command) || seen[c.Kind] {
			return errors.New("checks require unique kinds and explicit commands")
		}
		seen[c.Kind] = true
	}
	if len(seen) != 4 || !seen["health"] || !seen["database"] || !seen["media"] || !seen["login"] {
		return errors.New("recovery requires health, database, media and login checks")
	}
	return nil
}

func recoveryMounts(root, project string, plan RecoveryPlan) ([]any, error) {
	var prepared []string
	// Reject all symlinks and special files before exposing anything to Docker.
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("recovery refuses symlinks and special files")
		}
		if d.Name() == "manifest.json" && strings.HasPrefix(filepath.Base(filepath.Dir(p)), "prepared-") {
			if info.Size() > 65536 {
				return errors.New("preparation manifest exceeds size limit")
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			var m struct {
				Project string `json:"project"`
			}
			if json.Unmarshal(b, &m) == nil && m.Project == project {
				prepared = append(prepared, filepath.Dir(p))
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.New("restored tree contains unsafe or unreadable entries")
	}
	var mounts []any
	for _, m := range plan.Mounts {
		source := filepath.Join(root, m.Source)
		if m.Prepared {
			if len(prepared) != 1 {
				return nil, errors.New("expected one prepared database directory in restored snapshot")
			}
			source = prepared[0]
		}
		// Resolve both paths, including ancestor symlinks; a restored root must be private.
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return nil, errors.New("restored mount source missing")
		}
		base, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, errors.New("cannot resolve restored tree")
		}
		rel, err := filepath.Rel(base, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, errors.New("restored mount escapes target")
		}
		mounts = append(mounts, map[string]any{"type": "bind", "source": resolved, "target": m.Target, "read_only": !m.Writable, "bind": map[string]any{"create_host_path": false}})
	}
	return mounts, nil
}

func dockerCommand(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	// Pin local daemon and strip Docker/Compose overrides, including implicit .env.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if !strings.HasPrefix(upper, "DOCKER_") && !strings.HasPrefix(upper, "COMPOSE_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "DOCKER_HOST=unix:///var/run/docker.sock", "COMPOSE_DISABLE_ENV_FILE=1")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	configureCancellation(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("recovery Docker command failed (output withheld)")
	}
	return nil
}

var recoveryProject = regexp.MustCompile(`^homevault-drill-[0-9a-f]{24}$`)

// Rehearse preserves restored files and a report, but always attempts to remove
// only its randomly named Compose project, even on timeout or failed checks.
func (e *Engine) Rehearse(ctx context.Context, project, id, target string, plan RecoveryPlan) (reportPath string, err error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("recovery rehearsal currently requires Linux")
	}
	if err = plan.validate(); err != nil {
		return "", err
	}
	if _, err = e.cfg.Project(project); err != nil {
		return "", err
	}
	if !snapshotID.MatchString(id) {
		return "", errors.New("rehearsal requires a complete snapshot ID")
	}
	err = e.withLock(ctx, func() (result error) {
		if err := dockerCommand(ctx, "image", "inspect", plan.Image); err != nil {
			return errors.New("recovery image must already exist locally; no automatic pull/build")
		}
		if plan.Postgres != nil {
			if err := dockerCommand(ctx, "image", "inspect", plan.Postgres.Image); err != nil {
				return errors.New("PostgreSQL recovery image must already exist locally")
			}
			p, _ := e.cfg.Project(project)
			found := false
			for _, db := range p.Databases {
				found = found || db.Type == "postgres" && db.Name == plan.Postgres.Artifact
			}
			if !found {
				return errors.New("PostgreSQL artifact must match a configured PostgreSQL export")
			}
		}
		stateInfo, statErr := os.Stat(e.cfg.StateDir)
		if statErr != nil || stateInfo.Mode().Perm()&0077 != 0 {
			return errors.New("recovery requires private state_dir (0700)")
		}
		if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
			return errors.New("rehearsal requires a brand-new restore target")
		}
		work, err := os.MkdirTemp(e.cfg.StateDir, "recovery-")
		if err != nil {
			return errors.New("cannot create recovery report directory")
		}
		var nonce [12]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return errors.New("cannot generate recovery project identity")
		}
		name := "homevault-drill-" + hex.EncodeToString(nonce[:])
		if !recoveryProject.MatchString(name) {
			return errors.New("invalid recovery project identity")
		}
		reportPath = filepath.Join(work, "report.json")
		report := RecoveryReport{Project: project, Snapshot: strings.ToLower(id), ComposeProject: name, Started: time.Now().UTC(), Status: "failed", Checks: map[string]bool{}}
		composeFile := filepath.Join(work, "compose.json")
		started := false
		defer func() {
			if started {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cleanupErr := dockerCommand(cleanupCtx, "compose", "--project-name", name, "--file", composeFile, "down", "--volumes", "--timeout", "5")
				report.CleanupPassed = cleanupErr == nil
				result = errors.Join(result, cleanupErr)
			}
			report.Finished = time.Now().UTC()
			if result == nil && report.RestorePassed && report.CleanupPassed {
				report.Status = "passed"
			}
			b, _ := json.MarshalIndent(report, "", "  ")
			if err := os.WriteFile(reportPath, b, 0600); err != nil {
				result = errors.Join(result, errors.New("cannot save recovery report"))
			}
		}()
		if err := e.restoreLocked(ctx, project, id, target); err != nil {
			return err
		}
		report.RestorePassed = true
		mounts, err := recoveryMounts(target, project, plan)
		if err != nil {
			return err
		}
		app := map[string]any{
			"image": plan.Image, "pull_policy": "never", "entrypoint": plan.Command, "command": []string{}, "user": strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()),
			"network_mode": "none", "read_only": true, "cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"}, "restart": "no",
			"mem_limit": "256m", "cpus": 0.5, "pids_limit": 64, "volumes": mounts, "tmpfs": []string{"/tmp:rw,noexec,nosuid,size=32m,mode=1777"},
			"logging":     map[string]any{"driver": "none"},
			"healthcheck": map[string]any{"disable": true},
		}
		services := map[string]any{"app": app}
		if plan.Postgres != nil {
			db, err := recoveryPostgresService(target, project, *plan.Postgres)
			if err != nil {
				return err
			}
			services["database"] = db
			// Two containers, one network namespace with no external interface.
			app["network_mode"] = "service:database"
			app["depends_on"] = []string{"database"}
		}
		compose := map[string]any{"services": services}
		b, _ := json.MarshalIndent(compose, "", "  ")
		b = []byte(strings.ReplaceAll(string(b), "$", "$$"))
		if err = os.WriteFile(composeFile, b, 0600); err != nil {
			return errors.New("cannot write isolated recovery Compose file")
		}
		args := []string{"compose", "--project-name", name, "--file", composeFile}
		started = true // Partial up failures still require down.
		if plan.Postgres != nil {
			if err = dockerCommand(ctx, append(args, "up", "--detach", "--no-build", "--pull", "never", "database")...); err != nil {
				return err
			}
			ready := append(append([]string{}, args...), "exec", "-T", "database", "pg_isready", "--host", "127.0.0.1", "--username", "postgres", "--dbname", "recovery")
			if err = retryRecoveryCommand(ctx, 40*time.Second, ready); err != nil {
				return errors.New("recovery PostgreSQL did not become ready")
			}
			load := append(append([]string{}, args...), "exec", "-T", "database", "pg_restore", "--exit-on-error", "--single-transaction", "--no-owner", "--no-privileges", "--no-password", "--host", "127.0.0.1", "--username", "postgres", "--dbname", "recovery", "/data/restore.dump")
			if err = dockerCommand(ctx, load...); err != nil {
				return errors.New("recovery PostgreSQL import failed (output withheld)")
			}
			report.DatabaseImported = true
		}
		if err = dockerCommand(ctx, append(args, "up", "--detach", "--no-build", "--pull", "never")...); err != nil {
			return err
		}
		for _, check := range plan.Checks {
			// Bounded readiness retries; all checks must return zero. No output is logged.
			deadline := time.Now().Add(20 * time.Second)
			for {
				checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err = dockerCommand(checkCtx, append(append(append([]string{}, args...), "exec", "-T", "app"), check.Command...)...)
				cancel()
				if err == nil {
					break
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if time.Now().After(deadline) {
					return errors.New("recovery check failed: " + check.Kind)
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(250 * time.Millisecond):
				}
			}
			report.Checks[check.Kind] = true
		}
		return nil
	})
	return reportPath, err
}
