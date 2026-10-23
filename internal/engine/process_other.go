//go:build !linux

package engine

import (
	"os/exec"
	"time"
)

func configureCancellation(cmd *exec.Cmd) {
	cmd.WaitDelay = 5 * time.Second
}
