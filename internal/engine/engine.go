// Package engine runs the installed restic executable. Restic provides encryption,
// deduplication and repository handling; HomeVault provides policy and coordination.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"homevault/internal/config"
	"homevault/internal/lock"
	"homevault/internal/notify"
)

type Snapshot struct {
	ID       string    `json:"id"`
	ShortID  string    `json:"short_id"`
	Time     time.Time `json:"time"`
	Hostname string    `json:"hostname"`
	Tags     []string  `json:"tags"`
	Paths    []string  `json:"paths"`
}

type runner func(context.Context, []string, []string, bool) ([]byte, error)

type Engine struct {
	cfg              config.Config
	binary           string
	rclone           string
	run              runner
	sendNotification func(context.Context, string, string) error
	armPauseGuard    func(string, string) error
}

var snapshotID = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func New(cfg config.Config) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	name := cfg.ResticBinary
	if name == "" {
		name = "restic"
	}
	binary, err := exec.LookPath(name)
	if err != nil {
		return nil, errors.New("restic executable not found; install restic or configure restic_binary")
	}
	var rclone string
	if strings.HasPrefix(cfg.Repository, "rclone:") {
		name = cfg.RcloneBinary
		if name == "" {
			name = "rclone"
		}
		rclone, err = exec.LookPath(name)
		if err != nil {
			return nil, errors.New("rclone executable not found; install rclone or configure rclone_binary")
		}
	}
	// Callers cannot mutate an engine's policy after construction through slices.
	projects := make([]config.Project, len(cfg.Projects))
	for i, project := range cfg.Projects {
		projects[i] = project
		projects[i].Paths = append([]string(nil), project.Paths...)
		projects[i].Databases = append([]config.Database(nil), project.Databases...)
	}
	cfg.Projects = projects
	engine := &Engine{cfg: cfg, binary: binary, rclone: rclone}
	engine.run = engine.execute
	engine.armPauseGuard = startPauseGuardian
	notifier := notify.New()
	engine.sendNotification = func(ctx context.Context, project, event string) error {
		return notifier.Send(ctx, engine.cfg.DiscordWebhookFile, project, event)
	}
	return engine, nil
}

// Init explicitly creates a repository. No other operation initializes it.
func (e *Engine) Init(ctx context.Context) error {
	return e.withLock(ctx, func() error {
		_, err := e.command(ctx, "initialize repository", false, "init")
		return err
	})
}

func (e *Engine) Backup(ctx context.Context, projectName string) error {
	project, err := e.cfg.Project(projectName)
	if err != nil {
		return err
	}
	if project.CaptureRequired {
		return errors.New("project requires capture-backup; plain backup refused")
	}
	return e.withLock(ctx, func() error { return e.backupLocked(ctx, project) })
}

func (e *Engine) backupLocked(ctx context.Context, project config.Project) (result error) {
	for _, source := range project.Paths {
		info, err := os.Stat(source)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("project %s source must be an existing directory before backup", project.Name)
		}
	}
	stage, err := e.prepareDatabases(ctx, project)
	if err != nil {
		return err
	}
	if stage != "" {
		defer func() { result = errors.Join(result, os.RemoveAll(stage)) }()
	}
	args := []string{"backup", "--tag", projectTag(project.Name), "--"}
	args = append(args, project.Paths...)
	if stage != "" {
		args = append(args, stage)
	}
	_, err = e.command(ctx, "backup", false, args...)
	return err
}

func (e *Engine) Snapshots(ctx context.Context, projectName string) (snapshots []Snapshot, err error) {
	if _, err := e.cfg.Project(projectName); err != nil {
		return nil, err
	}
	err = e.withLock(ctx, func() error {
		var queryErr error
		snapshots, queryErr = e.snapshots(ctx, projectName)
		return queryErr
	})
	return snapshots, err
}

func (e *Engine) Check(ctx context.Context) error {
	return e.withLock(ctx, func() error {
		_, err := e.command(ctx, "check repository metadata", false, "check")
		return err
	})
}

// Restore requires a complete snapshot ID belonging to the selected project. The
// destination must be new or empty and must not overlap any configured protected
// path. A failed restore may leave partial files, which are never silently removed.
// Restic's overwrite=never also protects files created concurrently after validation;
// such concurrent files may be skipped, so callers should use a private destination.
func (e *Engine) Restore(ctx context.Context, projectName, id, target string) error {
	if _, err := e.cfg.Project(projectName); err != nil {
		return err
	}
	if !snapshotID.MatchString(id) {
		return errors.New("restore requires an explicit complete 64-character hexadecimal snapshot ID")
	}
	return e.withLock(ctx, func() error { return e.restoreLocked(ctx, projectName, id, target) })
}

func (e *Engine) restoreLocked(ctx context.Context, projectName, id, target string) error {
	if err := e.validateRestoreTarget(target); err != nil {
		return err
	}
	snapshots, err := e.snapshots(ctx, projectName)
	if err != nil {
		return err
	}
	found := false
	for _, snapshot := range snapshots {
		if strings.EqualFold(snapshot.ID, id) {
			found = true
			id = snapshot.ID
			break
		}
	}
	if !found {
		return errors.New("snapshot does not belong to the selected project or no longer exists")
	}
	// Recheck immediately before creating/writing the destination.
	if err := e.validateRestoreTarget(target); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return errors.New("cannot create restore target directory")
	}
	if err := emptyDirectory(target); err != nil {
		return err
	}
	_, err = e.command(ctx, "restore", false, "restore", id, "--target", target, "--overwrite", "never")
	return err
}

func (e *Engine) snapshots(ctx context.Context, projectName string) ([]Snapshot, error) {
	data, err := e.command(ctx, "list snapshots", true, "snapshots", "--json", "--tag", projectTag(projectName))
	if err != nil {
		return nil, err
	}
	var snapshots []Snapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil, errors.New("restic returned invalid snapshot JSON")
	}
	filtered := make([]Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !snapshotID.MatchString(snapshot.ID) {
			return nil, errors.New("restic returned an invalid snapshot ID")
		}
		for _, tag := range snapshot.Tags {
			if tag == projectTag(projectName) {
				if snapshot.ShortID == "" {
					snapshot.ShortID = snapshot.ID[:8]
				}
				filtered = append(filtered, snapshot)
				break
			}
		}
	}
	return filtered, nil
}

func (e *Engine) validateRestoreTarget(target string) error {
	if err := config.ValidateDestination(target); err != nil {
		return fmt.Errorf("restore target: %w", err)
	}
	protected := []string{e.cfg.StateDir, e.cfg.PasswordFile, e.cfg.DiscordWebhookFile, e.cfg.RcloneConfigFile}
	if filepath.IsAbs(e.cfg.Repository) {
		protected = append(protected, e.cfg.Repository)
	}
	for _, project := range e.cfg.Projects {
		protected = append(protected, project.Paths...)
		for _, db := range project.Databases {
			protected = append(protected, db.Source, db.PasswordFile)
		}
	}
	for _, path := range protected {
		if path == "" {
			continue
		}
		contains, err := config.Contains(target, path)
		if err != nil {
			return errors.New("cannot resolve restore target")
		}
		within, err := config.Contains(path, target)
		if err != nil {
			return errors.New("cannot resolve restore target")
		}
		if contains || within {
			return errors.New("restore target must not overlap sources, repository, state_dir or password_file")
		}
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("restore target must be a new or empty directory, not a file or symlink")
	}
	return emptyDirectory(target)
}

func emptyDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.New("cannot inspect restore target")
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	if len(names) > 0 {
		return errors.New("restore target must be empty; existing files will not be overwritten")
	}
	if err != io.EOF {
		return errors.New("cannot verify restore target is empty")
	}
	return nil
}

func (e *Engine) withLock(ctx context.Context, action func() error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := e.cfg.Validate(); err != nil {
		return err
	}
	operationLock, err := lock.Acquire(e.cfg.StateDir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, operationLock.Release()) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return action()
}

func projectTag(name string) string { return "homevault:" + name }

func (e *Engine) command(ctx context.Context, action string, capture bool, commandArgs ...string) (output []byte, resultErr error) {
	if e.rclone != "" && e.cfg.RcloneConfigFile != "" {
		credentialLock, err := lock.AcquireCredential(e.cfg.RcloneConfigFile)
		if err != nil {
			return nil, errors.New("rclone credential file is busy or its lock needs manual inspection")
		}
		defer func() { resultErr = errors.Join(resultErr, credentialLock.Release()) }()
	}
	args := []string{"--repo", e.cfg.Repository, "--cache-dir", filepath.Join(e.cfg.StateDir, "cache")}
	if e.rclone != "" {
		args = append(args, "-o", "rclone.program="+e.rclone)
	}
	args = append(args, commandArgs...)
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if e.cfg.RcloneConfigFile != "" && strings.HasPrefix(upper, "RCLONE_") {
			continue
		}
		if !strings.HasPrefix(upper, "RESTIC_") {
			env = append(env, entry)
		}
	}
	env = append(env, "RESTIC_PASSWORD_FILE="+e.cfg.PasswordFile)
	if e.cfg.RcloneConfigFile != "" {
		env = append(env, "RCLONE_CONFIG="+e.cfg.RcloneConfigFile)
	}
	data, err := e.run(ctx, args, env, capture)
	if err == nil {
		return data, nil
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s: %w", action, ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, fmt.Errorf("%s failed: restic exit code %d (child output withheld to protect credentials)", action, exitErr.ExitCode())
	}
	if errors.Is(err, errOutputLimit) {
		return nil, fmt.Errorf("%s failed: snapshot output exceeds 8 MiB limit", action)
	}
	return nil, fmt.Errorf("%s failed: could not execute restic (child output withheld to protect credentials)", action)
}

var errOutputLimit = errors.New("command output exceeds limit")

// Do not embed bytes.Buffer: its promoted ReadFrom method would let io.Copy
// bypass Write and defeat the limit when os/exec copies from the child's pipe.
type boundedBuffer struct{ buffer bytes.Buffer }

func (b *boundedBuffer) Len() int      { return b.buffer.Len() }
func (b *boundedBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > 8*1024*1024-b.Len() {
		return 0, errOutputLimit
	}
	return b.buffer.Write(p)
}

func (e *Engine) execute(ctx context.Context, args, env []string, capture bool) ([]byte, error) {
	cmd := exec.CommandContext(ctx, e.binary, args...)
	cmd.Env = env
	cmd.Stderr = io.Discard
	cmd.Stdout = io.Discard
	var output boundedBuffer
	if capture {
		cmd.Stdout = &output
	}
	configureCancellation(cmd)
	err := cmd.Run()
	return output.Bytes(), err
}
