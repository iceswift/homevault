package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"homevault/internal/config"
)

func runSetup(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: setup NEW_DIRECTORY --project NAME --source ABS_DIRECTORY --repository REPOSITORY --password-file EXISTING_KEY [--sqlite ABS_DB] [--capture-required]")
	}
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	name := fs.String("project", "", "safe project name")
	source := fs.String("source", "", "existing directory of files/media; exclude live databases")
	repo := fs.String("repository", "", "absolute local repository or configured rclone remote")
	key := fs.String("password-file", "", "existing private repository key; never regenerated")
	db := fs.String("sqlite", "", "optional separate SQLite database file")
	capture := fs.Bool("capture-required", false, "require reviewed capture-backup workflow")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected setup argument")
	}
	p := config.Project{Name: *name, Paths: []string{*source}, CaptureRequired: *capture}
	if *db != "" {
		p.Databases = []config.Database{{Name: "main", Type: "sqlite", Source: *db}}
	}
	cfg := config.Config{Version: config.CurrentVersion, StateDir: filepath.Join(args[0], "state"), Repository: *repo, PasswordFile: *key, Projects: []config.Project{p}}
	if err := setupProject(args[0], cfg); err != nil {
		return err
	}
	fmt.Println("Private configuration and onboarding checklist created. No repository initialized, upload, app stop or timer activation.")
	fmt.Println("Configuration:", filepath.Join(args[0], "homevault.json"))
	fmt.Println("Next: read ONBOARDING.md, preserve the existing key off-server, then run doctor and a first restore drill.")
	return nil
}

func setupProject(root string, cfg config.Config) error {
	if err := config.ValidateDestination(root); err != nil {
		return err
	}
	if len(cfg.Projects) != 1 || cfg.StateDir != filepath.Join(root, "state") {
		return errors.New("setup requires one project and state inside the new directory")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	project := cfg.Projects[0]
	protected := append([]string{cfg.PasswordFile}, project.Paths...)
	if filepath.IsAbs(cfg.Repository) {
		protected = append(protected, cfg.Repository)
	}
	for _, db := range project.Databases {
		protected = append(protected, db.Source)
	}
	for _, p := range protected {
		a, e1 := config.Contains(root, p)
		b, e2 := config.Contains(p, root)
		if e1 != nil || e2 != nil || a || b {
			return errors.New("setup directory must not overlap source, database, key or repository")
		}
	}
	for _, p := range project.Paths {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			return errors.New("setup source must be an existing directory")
		}
	}
	for _, db := range project.Databases {
		info, err := os.Stat(db.Source)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("setup database must be an existing regular file")
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return errors.New("cannot encode configuration")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return errors.New("setup directory must be new and its parent must exist")
	}
	// Do not remove partial output on failure: inspect it before retrying in a new directory.
	if err := os.Mkdir(cfg.StateDir, 0700); err != nil {
		return errors.New("cannot create private state directory")
	}
	if err := writeNewPrivate(filepath.Join(root, "homevault.json"), data); err != nil {
		return err
	}
	return writeNewPrivate(filepath.Join(root, "ONBOARDING.md"), []byte(onboardingChecklist))
}

func writeNewPrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create private onboarding file")
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("cannot persist onboarding file; partial setup retained")
	}
	return nil
}

const onboardingChecklist = `# First backup and verified recovery

This setup only generated private files. It reused your existing key path and did
not copy or rotate the key, initialize a repository, authorize Google, read source
contents, stop an app, upload data or install a service. File mode bits do not
replace Windows ACL protection. Do not commit these files.

1. Review homevault.json. Source directories must contain files/media only;
   live SQLite files need the separate sqlite setting, PostgreSQL needs explicit
   database configuration. Other live DB formats are not automatically detected.
2. Preserve the existing repository password in an independent password manager
   or offline recovery location. Verify you can recover it without this server.
   An existing repository must use its original key. Never reinitialize it to fix
   a missing key. Configure rclone privately if using Drive; setup does not OAuth.
3. Install reviewed Restic/rclone executables. Run doctor with --config before
   the command. Doctor checks executable/config availability, not cloud access.
4. For an existing repository, list snapshots. Use init only for an intentionally
   new repository after checking its location. First practice with demo-setup.
5. Review source consistency before job-backup. Capture-required SQLite projects
   need a separate pinned capture plan and approved pause window; plain backup is
   refused. setup does not create or approve that plan.
6. Run backup/check, list snapshots and restore an explicit full ID into a new
   empty directory. Compare contents/hashes and validate DB exports. Restore never
   goes over production. Rehearse the app with a separate reviewed recovery plan.
7. Review status/history and retention-plan. Metadata check success is not a
   recovery guarantee. Retention preview deletes nothing.
8. Only after verified recovery, configure private optional notifications and
   review systemd templates/permissions. Test services manually before enabling
   timers. Nothing in this setup activates a schedule.

See the repository docs: DATABASE-BACKUPS.md, FRESH-CAPTURE.md,
RECOVERY-REHEARSAL.md, SCHEDULING-ALERTS.md and RETENTION.md.
`
