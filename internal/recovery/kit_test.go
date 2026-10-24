package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthenticatedMalformedKitCreatesNoCredentialTree(t *testing.T) {
	age := os.Getenv("HOMEVAULT_TEST_AGE")
	if age == "" {
		t.Skip("explicit age 1.3.2 required")
	}
	root := t.TempDir()
	identity := filepath.Join(root, "identity.txt")
	keygen := filepath.Join(filepath.Dir(age), "age-keygen")
	if exec.Command(keygen, "-o", identity).Run() != nil {
		t.Fatal("fixture keygen failed")
	}
	public, err := exec.Command(keygen, "-y", identity).Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, which := range []string{"project", "repository", "version", "empty-key"} {
		t.Run(which, func(t *testing.T) {
			p := payload{Version: 1, Created: time.Now().UTC(), Repository: filepath.Join(root, "repo"), Project: "fixture", Snapshot: strings.Repeat("a", 64), Password: []byte("PRIVATE_VALUE_NOT_FOR_ERRORS")}
			switch which {
			case "project":
				p.Project = "../../escape"
			case "repository":
				p.Repository = "https://invalid.example/PRIVATE_VALUE_NOT_FOR_ERRORS"
			case "version":
				p.Version = 2
			case "empty-key":
				p.Password = nil
			}
			data, _ := json.Marshal(p)
			kit := filepath.Join(t.TempDir(), "bad.age")
			file, err := os.OpenFile(kit, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(age, "--encrypt", "--recipient", strings.TrimSpace(string(public)))
			cmd.Stdin = bytes.NewReader(data)
			cmd.Stdout = file
			if err := cmd.Run(); err != nil {
				file.Close()
				t.Fatal("fixture encryption failed")
			}
			file.Close()
			target := filepath.Join(root, "refused")
			if _, err := Open(context.Background(), kit, target, identity, "", "", "", age); err == nil || strings.Contains(err.Error(), "PRIVATE_VALUE_NOT_FOR_ERRORS") {
				t.Fatal("malformed kit accepted or value leaked", err)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("credential output created before valid metadata")
			}
		})
	}
}
