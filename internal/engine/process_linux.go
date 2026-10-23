//go:build linux

package engine

import (
	"os/exec"
	"syscall"
	"time"
)

func configureCancellation(cmd *exec.Cmd) {
	// rclone belongs to the same process group as restic. Kill the group on
	// cancellation so a remote writer cannot outlive the operation's state lock.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
}
