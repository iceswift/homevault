package drive

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

func checkRcloneRemote(root, binary string) error {
	cmd := exec.Command(binary, "--config", filepath.Join(root, "rclone.conf"), "listremotes")
	cmd.Env = []string{}
	data, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(data)) != "gdrive:" {
		return errors.New("prepared synthetic config was not recognized by real rclone; output withheld")
	}
	return nil
}
