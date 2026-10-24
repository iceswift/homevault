package integration

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

	"homevault/internal/config"
	"homevault/internal/engine"
	"homevault/internal/recovery"
)

func TestPortableKitAfterOriginalMachineLoss(t *testing.T) {
	age, restic := os.Getenv("HOMEVAULT_TEST_AGE"), os.Getenv("HOMEVAULT_TEST_RESTIC")
	if age == "" || restic == "" {
		t.Skip("explicit age 1.3.2 and Restic required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := t.TempDir()
	original := filepath.Join(root, "original")
	offserver := filepath.Join(root, "offserver")
	for _, p := range []string{original, offserver, filepath.Join(original, "files")} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(original, "original-repository-key")
	if err := os.WriteFile(key, []byte("generated-original-restic-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(original, "files")
	proof := []byte("portable recovery proof\nภาษาไทย\n")
	os.WriteFile(filepath.Join(source, "proof.txt"), proof, 0600)
	cfg := config.Config{Version: 1, StateDir: filepath.Join(original, "state"), Repository: filepath.Join(root, "external-repository"), PasswordFile: key, ResticBinary: restic, Projects: []config.Project{{Name: "fixture", Paths: []string{source}}}}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Backup(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := e.Snapshots(ctx, "fixture")
	if err != nil || len(snapshots) != 1 {
		t.Fatal("snapshot missing", err)
	}
	identity := filepath.Join(offserver, "identity.txt")
	keygen := filepath.Join(filepath.Dir(age), "age-keygen")
	if exec.CommandContext(ctx, keygen, "-o", identity).Run() != nil {
		t.Fatal("cannot generate fixture off-server identity")
	}
	public, err := exec.CommandContext(ctx, keygen, "-y", identity).Output()
	if err != nil {
		t.Fatal("cannot read fixture public recipient")
	}
	kit := filepath.Join(offserver, "fixture.age")
	info, err := recovery.Export(ctx, cfg, "fixture", snapshots[0].ID, kit, strings.TrimSpace(string(public)), "", age)
	if err != nil || !info.RecoveryOnly {
		t.Fatal("kit export failed", err)
	}
	ciphertext, _ := os.ReadFile(kit)
	if bytes.Contains(ciphertext, []byte("generated-original-restic-secret")) || bytes.Contains(ciphertext, []byte(cfg.Repository)) {
		t.Fatal("plaintext in encrypted kit")
	}
	if _, err = recovery.Export(ctx, cfg, "fixture", snapshots[0].ID, kit, strings.TrimSpace(string(public)), "", age); err == nil {
		t.Fatal("existing kit overwritten")
	}
	// Only generated original-machine files are removed. External repo and custody survive.
	if err = os.RemoveAll(original); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.New(cfg); err == nil {
		t.Fatal("original machine credentials unexpectedly survived")
	}
	relocated := filepath.Join(root, "relocated-repository")
	if err = os.Rename(cfg.Repository, relocated); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "new-machine")
	opened, err := recovery.Open(ctx, kit, target, identity, relocated, restic, "", age)
	if err != nil || opened.KitSHA256 != info.KitSHA256 || !opened.RecoveryOnly {
		t.Fatal("portable kit open failed", err)
	}
	recovered, err := config.Load(filepath.Join(target, "homevault.json"))
	if err != nil || !recovered.RecoveryOnly {
		t.Fatal("unsafe recovered config", err)
	}
	fresh, err := engine.New(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Init(ctx) == nil || fresh.Backup(ctx, "fixture") == nil || fresh.RunBackupJob(ctx, "fixture") == nil {
		t.Fatal("recovery configuration permitted writes")
	}
	restore := filepath.Join(root, "restored")
	if err = fresh.Restore(ctx, "fixture", snapshots[0].ID, restore); err != nil {
		t.Fatal(err)
	}
	var restored []byte
	if err = filepath.WalkDir(restore, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "proof.txt" {
			restored, err = os.ReadFile(p)
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, proof) {
		t.Fatal("fresh-machine byte proof failed")
	}
	if _, err = recovery.Open(ctx, kit, target, identity, relocated, restic, "", age); err == nil {
		t.Fatal("existing target overwritten")
	}
	// Wrong identity and damaged ciphertext must produce no plaintext target.
	wrong := filepath.Join(offserver, "wrong-identity.txt")
	if exec.CommandContext(ctx, keygen, "-o", wrong).Run() != nil {
		t.Fatal("cannot create wrong identity fixture")
	}
	for _, bad := range []struct{ file, key string }{{kit, wrong}, {filepath.Join(offserver, "damaged.age"), identity}} {
		if bad.file != kit {
			changed := append([]byte(nil), ciphertext...)
			changed[len(changed)-1] ^= 1
			os.WriteFile(bad.file, changed, 0600)
		}
		out := filepath.Join(root, "refused")
		if _, err = recovery.Open(ctx, bad.file, out, bad.key, relocated, restic, "", age); err == nil {
			t.Fatal("unauthenticated kit accepted")
		}
		if _, err = os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("plaintext output created before authentication")
		}
	}
	encoded, _ := json.Marshal(opened)
	if bytes.Contains(encoded, []byte("generated-original-restic-secret")) {
		t.Fatal("credential value leaked in report")
	}
	t.Log("PASS original source/key/state lost; encrypted off-server kit opened with independently kept identity; relocated repository restored exactly; mutation/wrong-key/tamper/overwrite guards passed")
}

func TestPortableDriveCredentialsOffline(t *testing.T) {
	age := os.Getenv("HOMEVAULT_TEST_AGE")
	if age == "" {
		t.Skip("explicit age 1.3.2 required; no Drive requests")
	}
	root := t.TempDir()
	source := filepath.Join(root, "files")
	os.Mkdir(source, 0700)
	key := filepath.Join(root, "restic-key")
	os.WriteFile(key, []byte("fixture-original-cloud-key"), 0600)
	rclone := filepath.Join(root, "rclone.conf")
	text := "[gdrive]\ntype = drive\nclient_id = 123-fixture.apps.googleusercontent.com\nclient_secret = fixture-client-secret\nscope = drive.file\ntoken = {\"access_token\":\"fixture-access\",\"refresh_token\":\"fixture-refresh\",\"token_type\":\"Bearer\",\"expiry\":\"2026-10-24T00:00:00Z\",\"id_token\":\"unrelated-not-exported\"}\n"
	os.WriteFile(rclone, []byte(text), 0600)
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: "rclone:gdrive:HomeVault/fixture", PasswordFile: key, RcloneConfigFile: rclone, Projects: []config.Project{{Name: "fixture", Paths: []string{source}}}}
	identity := filepath.Join(root, "identity.txt")
	keygen := filepath.Join(filepath.Dir(age), "age-keygen")
	if exec.Command(keygen, "-o", identity).Run() != nil {
		t.Fatal("fixture keygen failed")
	}
	public, err := exec.Command(keygen, "-y", identity).Output()
	if err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(root, "cloud.age")
	info, err := recovery.Export(context.Background(), cfg, "fixture", strings.Repeat("a", 64), kit, strings.TrimSpace(string(public)), "", age)
	if err != nil || !info.HasRclone {
		t.Fatal("offline cloud kit export failed", err)
	}
	target := filepath.Join(root, "new-machine")
	if _, err = recovery.Open(context.Background(), kit, target, identity, "", "", "", age); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "private", "rclone.conf"))
	if err != nil || !bytes.Contains(data, []byte("fixture-refresh")) || bytes.Contains(data, []byte("unrelated-not-exported")) {
		t.Fatal("incorrect dedicated transport recovery")
	}
	if _, err = recovery.Open(context.Background(), kit, filepath.Join(root, "local-mirror"), identity, filepath.Join(root, "mirror-repository"), "", "", age); err != nil {
		t.Fatal("cloud kit cannot recover from local repository mirror", err)
	}
}
