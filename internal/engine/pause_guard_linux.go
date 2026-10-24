//go:build linux

package engine

import (
	"os/exec"
	"syscall"
)

func detachPauseGuardian(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return nil
}

func pauseOwnerAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
