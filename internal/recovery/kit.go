// Package recovery builds portable age-encrypted credential kits. It does not
// include backup data or execute an application/repository operation on open.
package recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"homevault/internal/config"
	"homevault/internal/engine"
	"homevault/internal/lock"
)

const maxKit = 4 * 1024 * 1024

type payload struct {
	Version    int             `json:"version"`
	Created    time.Time       `json:"created"`
	Repository string          `json:"repository"`
	Project    string          `json:"project"`
	Snapshot   string          `json:"snapshot"`
	Databases  []string        `json:"databases,omitempty"`
	Password   []byte          `json:"restic_password"`
	Rclone     []byte          `json:"rclone_config,omitempty"`
	Plan       json.RawMessage `json:"recovery_plan,omitempty"`
}

type Info struct {
	Version      int       `json:"version"`
	Created      time.Time `json:"created"`
	Project      string    `json:"project"`
	Snapshot     string    `json:"snapshot"`
	KitSHA256    string    `json:"kit_sha256"`
	HasRclone    bool      `json:"has_rclone"`
	HasPlan      bool      `json:"has_plan"`
	RecoveryOnly bool      `json:"recovery_only"`
}

var fullID = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
var recipientID = regexp.MustCompile(`^age1[a-z0-9]{58}$`)
var identityLine = regexp.MustCompile(`^AGE-SECRET-KEY-1[A-Z0-9]{40,100}$`)

type limitBuffer struct{ b bytes.Buffer }

func (b *limitBuffer) Write(p []byte) (int, error) {
	if len(p) > maxKit-b.b.Len() {
		return 0, errors.New("recovery payload exceeds size limit")
	}
	return b.b.Write(p)
}

func privateRead(file string, limit int64) ([]byte, error) {
	info, err := os.Lstat(file)
	if !filepath.IsAbs(file) || err != nil || !info.Mode().IsRegular() || info.Size() > limit || os.PathSeparator != '\\' && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("recovery secret/plan must be an absolute private regular file within size limit")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.New("cannot read private recovery input")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("cannot read bounded recovery input")
	}
	return data, nil
}

func ageProgram(ctx context.Context, binary string) (string, error) {
	if binary == "" {
		binary = "age"
	}
	program, err := exec.LookPath(binary)
	if err != nil {
		return "", errors.New("age executable not found; install reviewed age 1.3.2")
	}
	cmd := exec.CommandContext(ctx, program, "--version")
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil || strings.TrimPrefix(strings.TrimSpace(string(out)), "v") != "1.3.2" {
		return "", errors.New("recovery kits require tested age 1.3.2")
	}
	return program, nil
}

func command(ctx context.Context, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Stderr = io.Discard
	configureCancellation(cmd)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "AGE_") && strings.ToUpper(key) != "AGEDEBUG" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	return cmd
}

func writePrivate(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("recovery output must be new")
	}
	_, a := f.Write(data)
	b := f.Sync()
	c := f.Close()
	if a != nil || b != nil || c != nil {
		return errors.New("cannot persist private recovery output; partial files retained")
	}
	return nil
}

func Export(ctx context.Context, cfg config.Config, project, snapshot, output, recipient, planFile, binary string) (info Info, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if cfg.RecoveryOnly {
		return info, errors.New("recovery-only configuration cannot export another credential kit")
	}
	if err := cfg.Validate(); err != nil {
		return info, err
	}
	p, err := cfg.Project(project)
	if err != nil {
		return info, err
	}
	if !fullID.MatchString(snapshot) || !recipientID.MatchString(recipient) {
		return info, errors.New("kit export requires a full snapshot ID and one age X25519 public recipient")
	}
	if err := config.ValidateDestination(output); err != nil {
		return info, err
	}
	// Never place even encrypted recovery material into a configured backup tree.
	protected := []string{cfg.StateDir, cfg.PasswordFile, cfg.RcloneConfigFile}
	if filepath.IsAbs(cfg.Repository) {
		protected = append(protected, cfg.Repository)
	}
	for _, pr := range cfg.Projects {
		protected = append(protected, pr.Paths...)
		for _, db := range pr.Databases {
			protected = append(protected, db.Source, db.PasswordFile)
		}
	}
	for _, path := range protected {
		if path == "" {
			continue
		}
		a, e1 := config.Contains(path, output)
		b, e2 := config.Contains(output, path)
		if e1 != nil || e2 != nil || a || b {
			return info, errors.New("kit output overlaps source, repository, state or credentials")
		}
	}
	program, err := ageProgram(ctx, binary)
	if err != nil {
		return info, err
	}
	guard, err := lock.Acquire(cfg.StateDir)
	if err != nil {
		return info, err
	}
	defer func() { resultErr = errors.Join(resultErr, guard.Release()) }()
	bundle := payload{Version: 1, Created: time.Now().UTC(), Repository: cfg.Repository, Project: project, Snapshot: strings.ToLower(snapshot)}
	bundle.Password, err = privateRead(cfg.PasswordFile, 64*1024)
	if err != nil {
		return info, err
	}
	defer clear(bundle.Password)
	for _, db := range p.Databases {
		bundle.Databases = append(bundle.Databases, db.Type+":"+db.Name)
	}
	if strings.HasPrefix(cfg.Repository, "rclone:") {
		if cfg.RcloneConfigFile == "" {
			return info, errors.New("cloud kit export requires an explicit dedicated rclone_config_file; ambient account settings are not exported")
		}
		credentials, err := lock.AcquireCredential(cfg.RcloneConfigFile)
		if err != nil {
			return info, errors.New("recovery credentials are busy")
		}
		defer func() { resultErr = errors.Join(resultErr, credentials.Release()) }()
		raw, err := privateRead(cfg.RcloneConfigFile, 1024*1024)
		if err != nil {
			return info, err
		}
		defer clear(raw)
		bundle.Rclone, err = normalizeDriveConfig(raw, cfg.Repository)
		if err != nil {
			return info, err
		}
		defer clear(bundle.Rclone)
	}
	if planFile != "" {
		bundle.Plan, err = privateRead(planFile, 1024*1024)
		if err != nil {
			return info, err
		}
		if _, err = engine.ParseRecoveryPlan(bundle.Plan); err != nil {
			return info, errors.New("invalid explicitly selected application recovery plan")
		}
	}
	data, err := json.Marshal(bundle)
	if err != nil || len(data) > maxKit {
		return info, errors.New("cannot encode bounded recovery kit")
	}
	defer clear(data)
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return info, errors.New("encrypted kit output must be new")
	}
	cmd := command(ctx, program, "--encrypt", "--recipient", recipient)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = f
	runErr := cmd.Run()
	syncErr := f.Sync()
	closeErr := f.Close()
	if runErr != nil || syncErr != nil || closeErr != nil {
		return info, errors.New("kit encryption failed; inspect partial encrypted output; no credential values printed")
	}
	if st, e := os.Stat(output); e != nil || st.Size() > maxKit+1024*1024 {
		return info, errors.New("encrypted kit output exceeded size limit")
	}
	ciphertext, err := os.ReadFile(output)
	if err != nil {
		return info, errors.New("cannot checksum encrypted kit")
	}
	sum := sha256.Sum256(ciphertext)
	return Info{Version: 1, Created: bundle.Created, Project: project, Snapshot: bundle.Snapshot, KitSHA256: hex.EncodeToString(sum[:]), HasRclone: len(bundle.Rclone) > 0, HasPlan: len(bundle.Plan) > 0, RecoveryOnly: true}, nil
}

func Open(ctx context.Context, input, target, identity, repository, restic, rclone, binary string) (info Info, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err = config.ValidateDestination(target); err != nil {
		return info, err
	}
	if _, e := os.Lstat(target); !os.IsNotExist(e) {
		return info, errors.New("recovery target must not exist")
	}
	key, err := privateRead(identity, 64*1024)
	if err != nil {
		return info, err
	}
	defer clear(key)
	count := 0
	for _, raw := range strings.Split(string(key), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !identityLine.MatchString(line) {
			return info, errors.New("recovery identity must contain one plain age X25519 private key, not plugins/passphrases")
		}
		count++
	}
	if count != 1 {
		return info, errors.New("recovery identity must contain exactly one private key")
	}
	stat, err := os.Lstat(input)
	if !filepath.IsAbs(input) || err != nil || !stat.Mode().IsRegular() || stat.Size() > maxKit+1024*1024 {
		return info, errors.New("encrypted kit must be an absolute bounded regular file")
	}
	f, e := os.Open(input)
	if e != nil {
		return info, errors.New("cannot open encrypted kit")
	}
	ciphertext, e := io.ReadAll(io.LimitReader(f, maxKit+1024*1024+1))
	f.Close()
	if e != nil || len(ciphertext) > maxKit+1024*1024 {
		return info, errors.New("encrypted kit exceeds input limit")
	}
	program, err := ageProgram(ctx, binary)
	if err != nil {
		return info, err
	}
	var plaintext limitBuffer
	defer func() { clear(plaintext.b.Bytes()) }()
	cmd := command(ctx, program, "--decrypt", "--identity", identity)
	cmd.Stdin = bytes.NewReader(ciphertext)
	cmd.Stdout = &plaintext
	if cmd.Run() != nil {
		return info, errors.New("kit decryption/authentication failed; no plaintext written")
	}
	var bundle payload
	decoder := json.NewDecoder(bytes.NewReader(plaintext.b.Bytes()))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bundle) != nil || decoder.Decode(new(any)) != io.EOF || bundle.Version != 1 || bundle.Created.IsZero() || !fullID.MatchString(bundle.Snapshot) || len(bundle.Password) == 0 || len(bundle.Password) > 64*1024 {
		return info, errors.New("invalid decrypted recovery schema")
	}
	defer clear(bundle.Password)
	defer clear(bundle.Rclone)
	if len(bundle.Plan) > 0 {
		if _, e = engine.ParseRecoveryPlan(bundle.Plan); e != nil {
			return info, errors.New("recovered application plan is invalid")
		}
	}
	if repository == "" {
		repository = bundle.Repository
	}
	private := filepath.Join(target, "private")
	cfg := config.Config{Version: config.CurrentVersion, RecoveryOnly: true, StateDir: filepath.Join(target, "state"), Repository: repository, PasswordFile: filepath.Join(private, "restic-password"), ResticBinary: restic, RcloneBinary: rclone, Projects: []config.Project{{Name: bundle.Project, Paths: []string{filepath.Join(target, "unavailable-original-source", bundle.Project)}}}}
	// Validate repository/project paths before creating any plaintext output.
	if len(bundle.Rclone) > 0 {
		normalized, normalizeErr := normalizeDriveConfig(bundle.Rclone, bundle.Repository)
		if normalizeErr != nil {
			return info, normalizeErr
		}
		defer clear(normalized)
		if strings.HasPrefix(repository, "rclone:") {
			bundle.Rclone, err = normalizeDriveConfig(normalized, repository)
		} else {
			bundle.Rclone = nil
		}
		if err != nil {
			return info, err
		}
		if len(bundle.Rclone) > 0 {
			cfg.RcloneConfigFile = filepath.Join(private, "rclone.conf")
		}
	}
	if strings.HasPrefix(repository, "rclone:") && len(bundle.Rclone) == 0 {
		return info, errors.New("cloud recovery requires a separately reviewed explicit transport configuration")
	}
	if err := config.ValidateRecoveryMetadata(cfg); err != nil {
		return info, err
	}
	if err = os.Mkdir(target, 0700); err != nil {
		return info, errors.New("cannot create new private recovery target")
	}
	for _, dir := range []string{private, cfg.StateDir} {
		if err = os.Mkdir(dir, 0700); err != nil {
			return info, errors.New("cannot create private recovery directories; partial output retained")
		}
	}
	if err = writePrivate(cfg.PasswordFile, bundle.Password); err != nil {
		return info, err
	}
	if len(bundle.Rclone) > 0 {
		if err = writePrivate(cfg.RcloneConfigFile, bundle.Rclone); err != nil {
			return info, err
		}
	}
	if err = cfg.Validate(); err != nil {
		return info, errors.New("recovered configuration failed final validation; private partial output retained")
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err = writePrivate(filepath.Join(target, "homevault.json"), data); err != nil {
		return info, err
	}
	if len(bundle.Plan) > 0 {
		if err = writePrivate(filepath.Join(target, "recovery-plan.json"), bundle.Plan); err != nil {
			return info, err
		}
	}
	sum := sha256.Sum256(ciphertext)
	info = Info{Version: 1, Created: bundle.Created, Project: bundle.Project, Snapshot: strings.ToLower(bundle.Snapshot), KitSHA256: hex.EncodeToString(sum[:]), HasRclone: len(bundle.Rclone) > 0, HasPlan: len(bundle.Plan) > 0, RecoveryOnly: true}
	data, _ = json.MarshalIndent(info, "", "  ")
	if err = writePrivate(filepath.Join(target, "recovery-info.json"), data); err != nil {
		return info, err
	}
	if err = writePrivate(filepath.Join(target, "RESTORE-FIRST.md"), []byte(recoveryGuide)); err != nil {
		return info, err
	}
	return info, nil
}

const recoveryGuide = `# Restore first

This folder contains plaintext recovery credentials. Keep it private and outside Git.
No repository was initialized, backed up, pruned, or restored merely by opening it.
The generated configuration is recovery_only=true. Do not remove that guard to fix
an access error. Existing repositories always use the ORIGINAL restored password.

1. Review homevault.json and recovery-info.json. Confirm repository/account, paths,
   tool versions and the selected snapshot. Relocate a local repository explicitly.
2. Install reviewed Restic/rclone/database tools and exact compatible app images.
3. Run doctor, snapshots PROJECT, check, then restore PROJECT FULL_ID NEW_EMPTY_TARGET
   --confirm with --config PATH before each command. Compare files/DB/media hashes.
4. If a recovery-plan.json was explicitly bundled, review every image/command first.
   Rehearse in isolated Docker, verify health/database/media/login and cleanup.
5. Production cutover, secrets not included here, proxy/DNS and schedules are manual.

The kit does not automatically include backup data, container images, .env files,
database login credentials or machine configuration. An explicitly included plan
may itself contain sensitive commands; review it privately. Copying this folder alone is not proof
of recovery. A stored snapshot can later be removed; inspect current repository state.
Keep the age private identity OFF the original server, separately from this kit.
`
