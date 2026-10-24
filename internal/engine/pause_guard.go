package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type pauseGuard struct {
	Version     int       `json:"version"`
	ContainerID string    `json:"container_id"`
	ImageID     string    `json:"image_id"`
	Nonce       string    `json:"nonce"`
	Deadline    time.Time `json:"deadline"`
	OwnerPID    int       `json:"owner_pid,omitempty"`
}

func loadPauseGuard(file string) (pauseGuard, error) {
	var guard pauseGuard
	if !filepath.IsAbs(file) {
		return guard, errors.New("guard path must be absolute")
	}
	parent, err := os.Lstat(filepath.Dir(file))
	if os.IsNotExist(err) {
		return guard, err
	}
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&0077 != 0 {
		return guard, errors.New("guard directory must be private")
	}
	info, err := os.Lstat(file)
	if err != nil {
		return guard, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return guard, errors.New("guard must be a private regular file")
	}
	if err = readState(file, &guard); err != nil {
		return guard, err
	}
	if guard.Version != 1 || !containerIDPattern.MatchString(guard.ContainerID) || !imageIDPattern.MatchString(guard.ImageID) || !recoveryAttemptID.MatchString(guard.Nonce) || guard.Deadline.IsZero() || guard.Deadline.After(time.Now().Add(65*time.Second)) {
		return guard, errors.New("invalid pause guard identity/deadline")
	}
	return guard, nil
}
func clearPauseGuard(file, nonce string) error {
	guard, err := loadPauseGuard(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if guard.Nonce != nonce {
		return errors.New("refusing to clear a different pause guard")
	}
	var ready struct {
		Nonce string `json:"nonce"`
	}
	if readState(file+".ready", &ready) == nil && ready.Nonce == nonce {
		_ = os.Remove(file + ".ready")
	}
	return os.Remove(file)
}
func startPauseGuardian(binary, file string) error {
	if binary == "" {
		var err error
		binary, err = os.Executable()
		if err != nil {
			return err
		}
	}
	cmd := exec.Command(binary, "pause-guardian", file)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := detachPauseGuardian(cmd); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return errors.New("cannot launch independent pause guardian")
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var ready struct {
			Nonce string `json:"nonce"`
		}
		guard, err := loadPauseGuard(file)
		if err == nil && readState(file+".ready", &ready) == nil && ready.Nonce == guard.Nonce {
			return nil
		}
		select {
		case <-exited:
			return errors.New("pause guardian exited before readiness")
		case <-time.After(25 * time.Millisecond):
		}
	}
	return errors.New("pause guardian did not acknowledge readiness; no app was stopped")
}

// RecoverPaused is independent of Restic/config/operation locks. It can resume
// the exact pinned container even if backup credentials or a stale lock broke.
// --force is for an operator or reviewed systemd ExecStopPost after job exit.
func RecoverPaused(ctx context.Context, file string, force bool) error {
	if runtime.GOOS != "linux" {
		return errors.New("pause recovery requires Linux")
	}
	return recoverPaused(ctx, file, force, dockerCapture{})
}
func recoverPaused(ctx context.Context, file string, force bool, docker captureBackend) error {
	guard, err := loadPauseGuard(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !force && time.Now().Before(guard.Deadline) {
		return nil
	}
	if force && time.Now().Before(guard.Deadline) && (guard.OwnerPID <= 0 || pauseOwnerAlive(guard.OwnerPID)) {
		return errors.New("active pause owner has not exited; refuse early forced resume")
	}
	info, err := docker.inspect(ctx, guard.ContainerID)
	if err != nil {
		return err
	}
	if info.ID != guard.ContainerID || info.Image != guard.ImageID {
		return errors.New("paused container identity changed; manual review required")
	}
	if err = docker.start(ctx, guard.ContainerID); err != nil {
		return err
	}
	for {
		info, err = docker.inspect(ctx, guard.ContainerID)
		if err == nil && info.ID == guard.ContainerID && info.Image == guard.ImageID && info.Running && info.Health == "healthy" {
			return clearPauseGuard(file, guard.Nonce)
		}
		select {
		case <-ctx.Done():
			return errors.New("pause recovery health check failed; guard retained")
		case <-time.After(250 * time.Millisecond):
		}
	}
}
func PauseGuardian(ctx context.Context, file string) error {
	if runtime.GOOS != "linux" {
		return errors.New("pause guardian requires Linux")
	}
	initial, err := loadPauseGuard(file)
	if err != nil {
		return err
	}
	if err = writeState(file+".ready", struct {
		Nonce string `json:"nonce"`
	}{initial.Nonce}); err != nil {
		return err
	}
	defer func() {
		var ready struct {
			Nonce string `json:"nonce"`
		}
		if readState(file+".ready", &ready) == nil && ready.Nonce == initial.Nonce {
			_ = os.Remove(file + ".ready")
		}
	}()
	for {
		guard, err := loadPauseGuard(file)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if guard.Nonce != initial.Nonce {
			return nil
		} // Never follow a later job's guard.
		if !time.Now().Before(guard.Deadline) {
			attempt, stop := context.WithTimeout(ctx, 30*time.Second)
			err = recoverPaused(attempt, file, false, dockerCapture{})
			stop()
			if err == nil {
				return nil
			}
			// A bounded retry covers a late stop API completion or brief daemon outage.
			attempt, stop = context.WithTimeout(ctx, 30*time.Second)
			defer stop()
			return recoverPaused(attempt, file, false, dockerCapture{})
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
