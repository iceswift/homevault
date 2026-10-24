package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"homevault/internal/config"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type CapturePlan struct {
	ContainerID     string `json:"container_id"`
	ImageID         string `json:"image_id"`
	DataRoot        string `json:"data_root"`
	WindowStart     string `json:"window_start"`
	WindowMinutes   int    `json:"window_minutes"`
	Timezone        string `json:"timezone"`
	MaxPauseSeconds int    `json:"max_pause_seconds"`
	GuardianBinary  string `json:"guardian_binary,omitempty"`
}
type captureIdentity struct {
	ID      string `json:"id"`
	Image   string `json:"image"`
	Running bool   `json:"running"`
	Started string `json:"started"`
	Health  string `json:"health"`
	Mounts  []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"mounts"`
}
type captureBackend interface {
	inspect(context.Context, string) (captureIdentity, error)
	stop(context.Context, string) error
	start(context.Context, string) error
}
type dockerCapture struct{}

var containerIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var imageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var ErrOutsideMaintenanceWindow = errors.New("outside reviewed maintenance window; capture skipped")

func LoadCapturePlan(file string) (CapturePlan, error) {
	var plan CapturePlan
	if err := readState(file, &plan); err != nil {
		return plan, errors.New("invalid capture plan file")
	}
	return plan, plan.validate()
}
func (p CapturePlan) validate() error {
	if !containerIDPattern.MatchString(p.ContainerID) || !imageIDPattern.MatchString(p.ImageID) || config.ValidateDestination(p.DataRoot) != nil {
		return errors.New("capture requires a pinned container/image and absolute data root")
	}
	if _, err := time.Parse("15:04", p.WindowStart); err != nil {
		return errors.New("window_start must be HH:MM")
	}
	if p.WindowMinutes < 1 || p.WindowMinutes > 1440 || p.MaxPauseSeconds < 5 || p.MaxPauseSeconds > 60 {
		return errors.New("capture window must be 1-1440 minutes and pause deadline 5-60 seconds")
	}
	if p.Timezone == "" {
		return errors.New("capture timezone is required")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return errors.New("unknown capture timezone")
	}
	if p.GuardianBinary != "" && (!filepath.IsAbs(p.GuardianBinary) || strings.ContainsAny(p.GuardianBinary, "\x00\r\n")) {
		return errors.New("guardian_binary must be an absolute reviewed executable")
	}
	return nil
}
func (p CapturePlan) inWindow(now time.Time) bool {
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return false
	}
	start, err := time.Parse("15:04", p.WindowStart)
	if err != nil {
		return false
	}
	local := now.In(loc)
	minute := local.Hour()*60 + local.Minute()
	first := start.Hour()*60 + start.Minute()
	elapsed := (minute - first + 1440) % 1440
	return elapsed < p.WindowMinutes
}

func dockerOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		k := strings.ToUpper(key)
		if !strings.HasPrefix(k, "DOCKER_") && !strings.HasPrefix(k, "COMPOSE_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "DOCKER_HOST=unix:///var/run/docker.sock")
	cmd.Stderr = io.Discard
	var output boundedBuffer
	cmd.Stdout = &output
	configureCancellation(cmd)
	if cmd.Run() != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("capture Docker operation failed (output withheld)")
	}
	return output.Bytes(), nil
}
func (d dockerCapture) inspect(ctx context.Context, id string) (captureIdentity, error) {
	var result captureIdentity
	// Never request Config.Env: no production environment/secrets are loaded.
	format := `{"id":{{json .Id}},"image":{{json .Image}},"running":{{json .State.Running}},"started":{{json .State.StartedAt}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}},"mounts":{{json .Mounts}}}`
	b, err := dockerOutput(ctx, "inspect", "--format", format, id)
	if err != nil {
		return result, err
	}
	if json.Unmarshal(b, &result) != nil {
		return result, errors.New("invalid capture container metadata")
	}
	return result, nil
}
func (d dockerCapture) stop(ctx context.Context, id string) error {
	_, err := dockerOutput(ctx, "stop", "--time", "5", id)
	return err
}
func (d dockerCapture) start(ctx context.Context, id string) error {
	_, err := dockerOutput(ctx, "start", id)
	return err
}

func (p CapturePlan) verifySource(project config.Project, identity captureIdentity) error {
	if identity.ID != p.ContainerID || identity.Image != p.ImageID || !identity.Running || identity.Health != "healthy" {
		return errors.New("capture container identity or healthy running state changed")
	}
	if len(identity.Mounts) != 1 || identity.Mounts[0].Type != "bind" || identity.Mounts[0].Destination != "/data" {
		return errors.New("capture supports one reviewed /data bind mount only")
	}
	a, err := config.Contains(p.DataRoot, identity.Mounts[0].Source)
	b, other := config.Contains(identity.Mounts[0].Source, p.DataRoot)
	if err != nil || other != nil || !a || !b {
		return errors.New("capture data root does not match live container mount")
	}
	if len(project.Paths) != 1 || len(project.Databases) != 1 || project.Databases[0].Type != "sqlite" {
		return errors.New("capture adapter requires exactly one SQLite database and one media directory")
	}
	for _, source := range []string{project.Paths[0], project.Databases[0].Source} {
		inside, err := config.Contains(p.DataRoot, source)
		same, _ := config.Contains(source, p.DataRoot)
		if err != nil || !inside || same {
			return errors.New("capture configuration must reference live DB/media beneath the reviewed data root, not an old frozen copy")
		}
	}
	return nil
}

func (e *Engine) RunCapturedJob(ctx context.Context, project string, plan CapturePlan) error {
	if e.cfg.RecoveryOnly {
		return errors.New("recovery-only configuration refuses capture")
	}
	if runtime.GOOS != "linux" {
		return errors.New("fresh capture requires Linux")
	}
	if err := plan.validate(); err != nil {
		return err
	}
	if _, err := e.cfg.Project(project); err != nil {
		return err
	}
	if !plan.inWindow(time.Now()) {
		return ErrOutsideMaintenanceWindow
	}
	return e.runBackupJob(ctx, project, func(ctx context.Context, p config.Project) (config.Project, func() error, error) {
		return e.captureProject(ctx, p, plan, dockerCapture{})
	})
}

// CheckCaptureSource is a read-only deployment check, not a capture/backup test.
// It deliberately does not arm a guardian, acquire a write lock or stop anything.
func (e *Engine) CheckCaptureSource(ctx context.Context, project string, plan CapturePlan) error {
	if runtime.GOOS != "linux" {
		return errors.New("fresh capture requires Linux")
	}
	if err := plan.validate(); err != nil {
		return err
	}
	p, err := e.cfg.Project(project)
	if err != nil {
		return err
	}
	identity, err := (dockerCapture{}).inspect(ctx, plan.ContainerID)
	if err != nil {
		return err
	}
	if err = plan.verifySource(p, identity); err != nil {
		return err
	}
	info, err := os.Lstat(p.Databases[0].Source)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("live DB must be a nonempty regular file")
	}
	f, err := os.Open(p.Databases[0].Source)
	if err != nil {
		return errors.New("live DB not readable")
	}
	f.Close()
	if _, err = exec.LookPath(tool(e.cfg.PythonBinary, "python3")); err != nil {
		return errors.New("Python backup tool unavailable")
	}
	if _, err = os.Lstat(filepath.Join(e.cfg.StateDir, "guards", p.Name+".json")); !os.IsNotExist(err) {
		return errors.New("capture guard remains; review before stopping")
	}
	return checkMediaTree(ctx, p.Paths[0])
}

func (e *Engine) captureProject(ctx context.Context, p config.Project, plan CapturePlan, docker captureBackend) (captured config.Project, cleanup func() error, result error) {
	// Called only under the job's operation lock. Recheck window after waiting.
	if !plan.inWindow(time.Now()) {
		return captured, nil, ErrOutsideMaintenanceWindow
	}
	before, err := docker.inspect(ctx, plan.ContainerID)
	if err != nil {
		return captured, nil, err
	}
	if err = plan.verifySource(p, before); err != nil {
		return captured, nil, err
	}
	dbInfo, err := os.Lstat(p.Databases[0].Source)
	if err != nil || !dbInfo.Mode().IsRegular() || dbInfo.Size() == 0 {
		return captured, nil, errors.New("live database must be a readable nonempty regular file before stopping")
	}
	dbFile, err := os.Open(p.Databases[0].Source)
	if err != nil {
		return captured, nil, errors.New("live database is not readable; no app stopped")
	}
	dbFile.Close()
	if _, err = exec.LookPath(tool(e.cfg.PythonBinary, "python3")); err != nil {
		return captured, nil, errors.New("Python SQLite backup tool unavailable; no app stopped")
	}
	if err = privateDir(e.cfg.StateDir); err != nil {
		return captured, nil, err
	}
	guardFile := filepath.Join(e.cfg.StateDir, "guards", p.Name+".json")
	if _, err = os.Lstat(guardFile); !os.IsNotExist(err) {
		return captured, nil, errors.New("previous capture guard remains; inspect/resume before retry")
	}
	if err = checkMediaTree(ctx, p.Paths[0]); err != nil {
		return captured, nil, err
	}
	destination, err := os.MkdirTemp(e.cfg.StateDir, "captured-")
	if err != nil {
		return captured, nil, errors.New("cannot create private fresh capture")
	}
	os.Mkdir(filepath.Join(destination, "db"), 0700)
	os.Mkdir(filepath.Join(destination, "raw"), 0700)
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return captured, nil, err
	}
	guard := pauseGuard{Version: 1, ContainerID: plan.ContainerID, ImageID: plan.ImageID, Nonce: hex.EncodeToString(nonce[:]), Deadline: time.Now().Add(time.Duration(plan.MaxPauseSeconds) * time.Second).UTC(), OwnerPID: os.Getpid()}
	if err = writeState(guardFile, guard); err != nil {
		return captured, nil, err
	}
	if err = e.armPauseGuard(plan.GuardianBinary, guardFile); err != nil {
		_ = clearPauseGuard(guardFile, guard.Nonce)
		return captured, nil, err
	}
	pauseCtx, cancel := context.WithDeadline(ctx, guard.Deadline)
	defer cancel()
	stopCompleted := false
	// Always attempt restart with an independent timeout, even after cancellation.
	// A stop timeout retains the armed guard to cover delayed Docker stop requests.
	resume := func() error {
		resumeCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := docker.start(resumeCtx, plan.ContainerID); err != nil {
			return err
		}
		for {
			info, err := docker.inspect(resumeCtx, plan.ContainerID)
			if err == nil && info.ID == plan.ContainerID && info.Image == plan.ImageID && info.Running && info.Health == "healthy" {
				if stopCompleted {
					return clearPauseGuard(guardFile, guard.Nonce)
				}
				return nil
			}
			select {
			case <-resumeCtx.Done():
				return errors.New("capture resume health failed; guard retained")
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	pausedErr := func() error {
		defer func() { result = errors.Join(result, resume()) }()
		armed, err := loadPauseGuard(guardFile)
		if err != nil || armed.Nonce != guard.Nonce || time.Until(guard.Deadline) < time.Second {
			return errors.New("pause guard expired before stop; capture cancelled")
		}
		if err := docker.stop(pauseCtx, plan.ContainerID); err != nil {
			return err
		}
		stopCompleted = true
		info, err := docker.inspect(pauseCtx, plan.ContainerID)
		if err != nil || info.Running {
			return errors.New("capture container did not stop")
		}
		database := filepath.Join(destination, "db", "main.sqlite")
		// Offline DB+WAL pair is copied only while the sole writer is stopped.
		// SQLite reconstructs sidecars in private staging, never in production.
		raw := filepath.Join(destination, "raw", "main.sqlite")
		if err = copyCaptureFile(pauseCtx, p.Databases[0].Source, raw); err != nil {
			return err
		}
		if _, walErr := os.Lstat(p.Databases[0].Source + "-wal"); walErr == nil {
			if err = copyCaptureFile(pauseCtx, p.Databases[0].Source+"-wal", raw+"-wal"); err != nil {
				return err
			}
		} else if !os.IsNotExist(walErr) {
			return errors.New("cannot inspect stopped database WAL")
		}
		if err = preparationCommand(pauseCtx, tool(e.cfg.PythonBinary, "python3"), []string{"-I", "-c", sqliteBackup, raw, database}); err != nil {
			return err
		}
		if err = copyMediaTree(pauseCtx, p.Paths[0], filepath.Join(destination, "media")); err != nil {
			return err
		}
		info, err = docker.inspect(pauseCtx, plan.ContainerID)
		if err != nil || info.Running || info.Started != before.Started {
			return errors.New("source resumed/changed while copying; capture rejected")
		}
		return nil
	}()
	result = errors.Join(result, pausedErr)
	if result != nil {
		return captured, nil, result
	} // Retain failed plaintext for private diagnosis.
	// A fresh manifest accompanies media inside the encrypted snapshot. No secrets.
	manifest := struct {
		Project    string    `json:"project"`
		CapturedAt time.Time `json:"captured_at"`
		Image      string    `json:"image"`
	}{p.Name, time.Now().UTC(), plan.ImageID}
	if err = writeState(filepath.Join(destination, "info", "capture.json"), manifest); err != nil {
		return captured, nil, err
	}
	captured = p
	captured.Paths = []string{filepath.Join(destination, "media"), filepath.Join(destination, "info")}
	captured.Databases = append([]config.Database(nil), p.Databases...)
	captured.Databases[0].Source = filepath.Join(destination, "db", "main.sqlite")
	cleanup = func() error { return os.RemoveAll(destination) } // Only the owned MkdirTemp directory.
	return captured, cleanup, nil
}

func checkMediaTree(ctx context.Context, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return errors.New("cannot inspect media")
		}
		info, err := d.Info()
		if err != nil || !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("media symlinks/special files are not allowed")
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return errors.New("media is not readable; no app stopped")
			}
			f.Close()
		}
		return nil
	})
}
func copyMediaTree(ctx context.Context, source, target string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return errors.New("cannot read media")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("unsafe media entry")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		out := filepath.Join(target, rel)
		if info.IsDir() {
			return os.Mkdir(out, 0700)
		}
		return copyCaptureFile(ctx, path, out)
	})
}

func copyCaptureFile(ctx context.Context, source, target string) error {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("capture file must be regular, not a symlink")
	}
	in, err := os.Open(source)
	if err != nil {
		return errors.New("cannot open capture source")
	}
	defer in.Close()
	actual, err := in.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return errors.New("capture source changed while opening")
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("cannot create captured file")
	}
	defer f.Close()
	buffer := make([]byte, 128*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := in.Read(buffer)
		if n > 0 {
			if _, err = f.Write(buffer[:n]); err != nil {
				return errors.New("cannot copy capture file")
			}
		}
		if readErr == io.EOF {
			return f.Close()
		}
		if readErr != nil {
			return errors.New("cannot read capture file")
		}
	}
}
