package drive

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"homevault/internal/lock"
)

type ConnectionStatus struct {
	Remote           string    `json:"remote"`
	Project          string    `json:"project"`
	RequestedScope   string    `json:"requested_scope"`
	State            string    `json:"state"`
	AccessExpires    time.Time `json:"access_expires,omitempty"`
	RefreshAvailable bool      `json:"refresh_available"`
	LiveChecked      bool      `json:"live_checked"`
	Accessible       bool      `json:"accessible"`
}

func localStatus(c connection) ConnectionStatus {
	s := ConnectionStatus{Remote: c.meta.Remote, Project: c.meta.Project, RequestedScope: "drive.file", State: "authorization_required"}
	if c.token != nil {
		s.AccessExpires = c.token.Expiry
		s.RefreshAvailable = c.token.RefreshToken != ""
		s.State = "authorized_cached"
		if !c.token.Expiry.After(time.Now().Add(30 * time.Second)) {
			s.State = "refresh_needed"
		}
	}
	return s
}

// Status never enumerates file names in its output. Online mode runs a shallow
// read-only remote listing; rclone may refresh the private token file.
func Status(ctx context.Context, root string, online bool, binary string) (status ConnectionStatus, resultErr error) {
	c, err := readConnection(root)
	if err != nil {
		return status, err
	}
	guard, err := lock.AcquireCredential(filepath.Join(root, "rclone.conf"))
	if err != nil {
		return status, errors.New("OAuth config is busy or credential lock needs inspection")
	}
	defer func() { resultErr = errors.Join(resultErr, guard.Release()) }()
	c, err = readConnection(root)
	if err != nil {
		return status, err
	}
	status = localStatus(c)
	if !online {
		return status, nil
	}
	if c.token == nil {
		return status, errors.New("authorize this prepared connection before an online status check")
	}
	if binary == "" {
		binary = "rclone"
	}
	program, err := exec.LookPath(binary)
	if err != nil {
		return status, errors.New("rclone executable not found for online status")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, "--config", filepath.Join(root, "rclone.conf"), "lsf", c.meta.Remote+":", "--dirs-only", "--max-depth", "1", "--retries", "1", "--low-level-retries", "1", "--contimeout", "5s", "--timeout", "10s")
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "RCLONE_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	configureCancellation(cmd)
	status.LiveChecked = true
	if cmd.Run() != nil {
		status.State = "unavailable"
		return status, errors.New("Drive reachability check failed; remote output withheld; inspect authorization/network privately")
	}
	c, err = readConnection(root)
	if err != nil {
		return status, err
	}
	status = localStatus(c)
	status.LiveChecked = true
	status.Accessible = true
	status.State = "reachable"
	return status, nil
}
