// Package config loads and validates HomeVault's versioned JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const CurrentVersion = 1

type Config struct {
	Version            int       `json:"version"`
	RecoveryOnly       bool      `json:"recovery_only,omitempty"`
	StateDir           string    `json:"state_dir"`
	Repository         string    `json:"repository"`
	PasswordFile       string    `json:"password_file"`
	ResticBinary       string    `json:"restic_binary,omitempty"`
	RcloneBinary       string    `json:"rclone_binary,omitempty"`
	RcloneConfigFile   string    `json:"rclone_config_file,omitempty"`
	PythonBinary       string    `json:"python_binary,omitempty"`
	PgDumpBinary       string    `json:"pg_dump_binary,omitempty"`
	PgRestoreBinary    string    `json:"pg_restore_binary,omitempty"`
	DiscordWebhookFile string    `json:"discord_webhook_file,omitempty"`
	NotifySuccess      bool      `json:"notify_success,omitempty"`
	BackupMaxAge       string    `json:"backup_max_age,omitempty"`
	Projects           []Project `json:"projects"`
}

type Project struct {
	Name            string     `json:"name"`
	Paths           []string   `json:"paths"`
	Databases       []Database `json:"databases,omitempty"`
	CaptureRequired bool       `json:"capture_required,omitempty"`
}

// PostgreSQL credentials use a libpq password file, never a connection URL.
type Database struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Source       string `json:"source,omitempty"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	Database     string `json:"database,omitempty"`
	User         string `json:"user,omitempty"`
	PasswordFile string `json:"password_file,omitempty"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// Load accepts exactly one JSON object and rejects unknown fields. Paths must be
// absolute: their interpretation never changes with the caller's working directory.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open configuration: %w", err)
	}
	defer f.Close()
	var cfg Config
	decoder := json.NewDecoder(io.LimitReader(f, 1024*1024+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		// JSON decoder errors can repeat secret-valued input; keep errors generic.
		return Config{}, errors.New("invalid configuration JSON or unknown field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Config{}, errors.New("configuration must contain exactly one JSON object")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) Project(name string) (Project, error) {
	for _, project := range cfg.Projects {
		if project.Name == name {
			return project, nil
		}
	}
	return Project{}, errors.New("project is not configured")
}

// Validate supports local absolute repositories and rclone:<remote>:<path> only.
// Credentials belong in password_file and the user's existing rclone configuration.
func (cfg Config) Validate() error {
	if cfg.DiscordWebhookFile != "" && ValidateDestination(cfg.DiscordWebhookFile) != nil {
		return errors.New("discord_webhook_file must be an absolute private file path, never the webhook URL")
	}
	if cfg.BackupMaxAge != "" {
		age, err := time.ParseDuration(cfg.BackupMaxAge)
		if err != nil || age <= 0 {
			return errors.New("backup_max_age must be a positive Go duration, for example 36h")
		}
	}
	if cfg.Version != CurrentVersion {
		return fmt.Errorf("unsupported configuration version (expected %d)", CurrentVersion)
	}
	if err := ValidateDestination(cfg.StateDir); err != nil {
		return fmt.Errorf("state_dir: %w", err)
	}
	if err := validateRepository(cfg.Repository); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.PasswordFile) == "" || !filepath.IsAbs(cfg.PasswordFile) {
		return errors.New("password_file must be an absolute file path")
	}
	info, err := os.Stat(cfg.PasswordFile)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("password_file must be an existing regular file")
	}
	if info.Size() == 0 {
		return errors.New("password_file must not be empty")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("password_file must not be accessible by group or others (use chmod 600)")
	}
	for _, binary := range []string{cfg.ResticBinary, cfg.RcloneBinary} {
		if strings.ContainsAny(binary, "\x00\r\n") {
			return errors.New("binary paths must not contain control characters")
		}
	}
	if cfg.RcloneConfigFile != "" {
		info, err := os.Lstat(cfg.RcloneConfigFile)
		if ValidateDestination(cfg.RcloneConfigFile) != nil || err != nil || !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return errors.New("rclone_config_file must be an existing absolute private regular file")
		}
	}
	if len(cfg.Projects) == 0 {
		return errors.New("at least one project is required")
	}
	seen := make(map[string]bool)
	for _, project := range cfg.Projects {
		if !safeName.MatchString(project.Name) {
			return errors.New("project names must be 1-64 safe letters, numbers, dots, underscores or hyphens, starting with a letter or number")
		}
		if seen[project.Name] {
			return errors.New("project names must be unique")
		}
		seen[project.Name] = true
		if project.CaptureRequired && (len(project.Paths) != 1 || len(project.Databases) != 1 || project.Databases[0].Type != "sqlite") {
			return errors.New("capture_required projects need one SQLite DB and one media directory")
		}
		if len(project.Paths) == 0 && len(project.Databases) == 0 {
			return fmt.Errorf("project %s requires at least one path", project.Name)
		}
		dbNames := map[string]bool{}
		for _, db := range project.Databases {
			if !safeName.MatchString(db.Name) || dbNames[db.Name] {
				return errors.New("database names must be safe and unique within a project")
			}
			dbNames[db.Name] = true
			switch db.Type {
			case "sqlite":
				if ValidateDestination(db.Source) != nil || db.Host != "" || db.Port != 0 || db.Database != "" || db.User != "" || db.PasswordFile != "" {
					return errors.New("sqlite requires an absolute source and no PostgreSQL settings")
				}
			case "postgres":
				if db.Source != "" || db.Host == "" || strings.ContainsAny(db.Host, "\x00\r\n") || db.Port < 1 || db.Port > 65535 || !safeName.MatchString(db.Database) || !safeName.MatchString(db.User) || !filepath.IsAbs(db.PasswordFile) {
					return errors.New("postgres requires explicit host, port, safe database/user and absolute password_file; URLs are unsupported")
				}
			default:
				return errors.New("database type must be sqlite or postgres")
			}
		}
		for _, source := range project.Paths {
			if err := ValidateDestination(source); err != nil {
				return fmt.Errorf("project %s source: %w", project.Name, err)
			}
			// Sources may be missing after a disk failure. Restore and repository
			// inspection must remain available; Backup checks source availability.
			for _, protected := range []string{cfg.StateDir, cfg.PasswordFile, cfg.DiscordWebhookFile, cfg.RcloneConfigFile} {
				if protected == "" {
					continue
				}
				inside, err := Contains(source, protected)
				if err != nil {
					return errors.New("cannot resolve configured paths")
				}
				reverse, err := Contains(protected, source)
				if err != nil {
					return errors.New("cannot resolve configured paths")
				}
				if inside || reverse {
					return fmt.Errorf("project %s source must not overlap state_dir or password_file", project.Name)
				}
			}
			if filepath.IsAbs(cfg.Repository) {
				inside, err := Contains(source, cfg.Repository)
				if err != nil {
					return errors.New("cannot resolve repository path")
				}
				reverse, err := Contains(cfg.Repository, source)
				if err != nil {
					return errors.New("cannot resolve source path")
				}
				if inside || reverse {
					return fmt.Errorf("project %s source and local repository must not overlap", project.Name)
				}
			}
		}
	}
	// Protect every database source/credential across all projects, including aliases.
	for _, project := range cfg.Projects {
		for _, db := range project.Databases {
			p := db.Source
			if db.Type == "postgres" {
				p = db.PasswordFile
			}
			for _, protected := range []string{cfg.StateDir, cfg.PasswordFile, cfg.Repository, cfg.DiscordWebhookFile, cfg.RcloneConfigFile} {
				if !filepath.IsAbs(protected) {
					continue
				}
				a, e1 := Contains(p, protected)
				b, e2 := Contains(protected, p)
				if e1 != nil || e2 != nil || a || b {
					return errors.New("database path overlaps protected storage")
				}
			}
			for _, other := range cfg.Projects {
				for _, source := range other.Paths {
					a, e1 := Contains(source, p)
					b, e2 := Contains(p, source)
					if e1 != nil || e2 != nil || a || b {
						return errors.New("file sources must not overlap live SQLite databases or database credentials")
					}
				}
			}
		}
	}
	return nil
}

func validateRepository(repository string) error {
	if strings.ContainsAny(repository, "\x00\r\n") {
		return errors.New("repository contains control characters")
	}
	if filepath.IsAbs(repository) {
		if err := ValidateDestination(repository); err != nil {
			return fmt.Errorf("repository: %w", err)
		}
		return nil
	}
	if strings.HasPrefix(repository, "rclone:") {
		parts := strings.SplitN(strings.TrimPrefix(repository, "rclone:"), ":", 2)
		if len(parts) == 2 && safeName.MatchString(parts[0]) && strings.TrimSpace(parts[1]) != "" && !strings.ContainsAny(parts[1], "\x00\r\n@?#:") {
			remotePath := path.Clean(strings.ReplaceAll(parts[1], "\\", "/"))
			if remotePath != "." && remotePath != "/" && remotePath != ".." && !strings.HasPrefix(remotePath, "../") {
				return nil
			}
		}
	}
	return errors.New("repository must be an absolute local path or rclone:<remote>:<path>; embedded credentials and URL repositories are unsupported")
}

// ValidateDestination rejects blank, relative and filesystem-root paths, including
// paths that resolve to a root through symlinks. The destination need not yet exist.
func ValidateDestination(path string) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		return errors.New("path must be absolute and nonempty, without control characters")
	}
	resolved, err := canonical(path)
	if err != nil {
		return errors.New("path cannot be resolved")
	}
	if filepath.Dir(resolved) == resolved {
		return errors.New("filesystem root paths are not allowed")
	}
	return nil
}

// Contains compares resolved paths, following existing ancestor symlinks even when
// the final destination does not exist. Equality counts as containment.
func Contains(parent, child string) (bool, error) {
	parent, err := canonical(parent)
	if err != nil {
		return false, err
	}
	child, err = canonical(child)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil { // Different Windows volumes cannot contain one another.
		if runtime.GOOS == "windows" && !strings.EqualFold(filepath.VolumeName(parent), filepath.VolumeName(child)) {
			return false, nil
		}
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func canonical(path string) (string, error) {
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = canonical(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}
