//go:build !linux

package engine

import (
	"errors"
	"os/exec"
)

func detachPauseGuardian(*exec.Cmd) error { return errors.New("pause guardian requires Linux") }

func pauseOwnerAlive(int) bool { return false }
