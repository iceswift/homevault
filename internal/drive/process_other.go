//go:build !linux

package drive

import (
	"os/exec"
	"time"
)

func configureCancellation(cmd *exec.Cmd) { cmd.WaitDelay = 5 * time.Second }
