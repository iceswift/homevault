//go:build linux

package drive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOnlineStatusIsolatesConfigAndWithholdsChildOutput(t *testing.T) {
	for _, fail := range []bool{false, true} {
		root, _ := prepared(t)
		c, err := readConnection(root)
		if err != nil {
			t.Fatal(err)
		}
		c.token = &wireToken{AccessToken: "fixture-access-private", RefreshToken: "fixture-refresh-private", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
		data, _ := configText(c)
		os.WriteFile(filepath.Join(root, "rclone.conf"), data, 0600)
		t.Setenv("RCLONE_CONFIG", "must-not-inherit")
		t.Setenv("RCLONE_DRIVE_TOKEN", "must-not-inherit")
		t.Setenv("RCLONE_LOG_FILE", "must-not-inherit")
		script := `#!/bin/sh
test "$1" = "--config" || exit 21
test "$3" = "lsf" || exit 22
test "$4" = "gdrive:" || exit 23
test -z "$RCLONE_CONFIG$RCLONE_DRIVE_TOKEN$RCLONE_LOG_FILE" || exit 24
printf 'PRIVATE_REMOTE_OUTPUT_MUST_NOT_ECHO' >&2
printf 'PRIVATE_REMOTE_DIRECTORY_NAME'
`
		if fail {
			script += "exit 1\n"
		} else {
			script += "exit 0\n"
		}
		binary := filepath.Join(t.TempDir(), "fake rclone")
		os.WriteFile(binary, []byte(script), 0700)
		status, err := Status(context.Background(), root, true, binary)
		if (err != nil) != fail || !status.LiveChecked || status.Accessible == fail {
			t.Fatal("incorrect online status", status, err)
		}
		out, _ := json.Marshal(status)
		if strings.Contains(string(out), "PRIVATE_") || err != nil && strings.Contains(err.Error(), "PRIVATE_") {
			t.Fatal("child output leaked")
		}
	}
}

func TestPreparedConfigWithRealRcloneOffline(t *testing.T) {
	binary := os.Getenv("HOMEVAULT_TEST_RCLONE")
	if binary == "" {
		t.Skip("explicit rclone binary required; offline config-only check")
	}
	root, _ := prepared(t)
	if err := checkRcloneRemote(root, binary); err != nil {
		t.Fatal(err)
	}
}
