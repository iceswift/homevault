//go:build !linux

package recovery

import (
	"os/exec"
	"time"
)

func configureCancellation(cmd *exec.Cmd) { cmd.WaitDelay = 5 * time.Second }
