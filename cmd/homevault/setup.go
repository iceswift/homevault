package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"homevault/internal/config"
)

// setupDemo creates only a brand-new directory. It never adopts an existing tree.
func setupDemo(root string) error {
	if err := config.ValidateDestination(root); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return errors.New("demo directory must not exist and its parent must exist")
	}
	for _, p := range []string{"source", "secrets", "state"} {
		if err := os.Mkdir(filepath.Join(root, p), 0700); err != nil {
			return err
		}
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	key := filepath.Join(root, "secrets", "restic-password")
	if err := os.WriteFile(key, []byte(hex.EncodeToString(secret[:])), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "source", "proof.txt"), []byte("HomeVault synthetic restore proof\nข้อมูลทดสอบเท่านั้น\n"), 0600); err != nil {
		return err
	}
	cfg := config.Config{Version: 1, StateDir: filepath.Join(root, "state"), Repository: filepath.Join(root, "repository"), PasswordFile: key, Projects: []config.Project{{Name: "synthetic", Paths: []string{filepath.Join(root, "source")}}}}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(root, "homevault.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	fmt.Println("Synthetic sandbox created. Configuration:", path)
	fmt.Println("No repository initialized, production data read, or cloud upload performed.")
	return nil
}
