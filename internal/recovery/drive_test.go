package recovery

import (
	"strings"
	"testing"
)

func TestDedicatedDriveKitRejectsOtherAccountsAndEndpoints(t *testing.T) {
	config := "[gdrive]\ntype = drive\nclient_id = 123-fixture.apps.googleusercontent.com\nclient_secret = fixture-client-secret\nscope = drive.file\ntoken = {\"access_token\":\"fixture-access\",\"refresh_token\":\"fixture-refresh\",\"token_type\":\"Bearer\",\"expiry\":\"2026-10-24T00:00:00Z\"}\n"
	if _, err := normalizeDriveConfig([]byte(config), "rclone:gdrive:HomeVault/fixture"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{config + "[other]\ntype = drive\n", config + "token_url = https://invalid.example/token\n", strings.Replace(config, "drive.file", "drive", 1), strings.Replace(config, "client_secret = fixture-client-secret", "client_secret = invalid\n injected = true", 1)} {
		if _, err := normalizeDriveConfig([]byte(bad), "rclone:gdrive:HomeVault/fixture"); err == nil {
			t.Fatal("unsafe or unrelated transport exported")
		}
	}
}
