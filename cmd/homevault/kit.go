package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"

	"homevault/internal/config"
	"homevault/internal/recovery"
)

func runKitExport(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) < 4 {
		return errors.New("usage: kit-export PROJECT FULL_SNAPSHOT_ID NEW_KIT.age --recipient AGE_PUBLIC_KEY [--recovery-plan FILE] --confirm")
	}
	fs := flag.NewFlagSet("kit-export", flag.ContinueOnError)
	recipient := fs.String("recipient", "", "off-server age X25519 public recipient")
	plan := fs.String("recovery-plan", "", "optional private reviewed application recovery plan")
	tool := fs.String("age-binary", "age", "reviewed age 1.3.2 executable")
	confirm := fs.Bool("confirm", false, "encrypt selected original recovery credentials")
	if err := fs.Parse(args[4:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || !*confirm {
		return errors.New("kit-export requires explicit --confirm and no extra arguments")
	}
	info, err := recovery.Export(ctx, cfg, args[1], args[2], args[3], *recipient, *plan, *tool)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(info)
}

func runKitOpen(ctx context.Context, args []string) error {
	if len(args) < 3 {
		return errors.New("usage: kit-open KIT.age NEW_DIRECTORY --identity-file PRIVATE_KEY [--repository RELOCATED_REPO] --confirm")
	}
	fs := flag.NewFlagSet("kit-open", flag.ContinueOnError)
	identity := fs.String("identity-file", "", "private off-server age X25519 identity file")
	repo := fs.String("repository", "", "explicit repository relocation, otherwise keep encrypted kit metadata")
	restic := fs.String("restic-binary", "", "Restic executable on the new machine")
	rclone := fs.String("rclone-binary", "", "rclone executable on the new machine")
	tool := fs.String("age-binary", "age", "reviewed age 1.3.2 executable")
	confirm := fs.Bool("confirm", false, "write recovered plaintext credentials into a new private directory")
	if err := fs.Parse(args[3:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || !*confirm {
		return errors.New("kit-open requires explicit --confirm and no extra arguments")
	}
	info, err := recovery.Open(ctx, args[1], args[2], *identity, *repo, *restic, *rclone, *tool)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(info)
}
