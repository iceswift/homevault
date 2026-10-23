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
)

const CurrentVersion = 1

type Config struct {
	Version      int       `json:"version"`
	StateDir     string    `json:"state_dir"`
	Repository   string    `json:"repository"`
	PasswordFile string    `json:"password_file"`
	ResticBinary string    `json:"restic_binary,omitempty"`
	RcloneBinary string    `json:"rclone_binary,omitempty"`
	Projects     []Project `json:"projects"`
}

type Project struct {
	Name  string   `json:"name"`
	Paths []string `json:"paths"`
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
		if len(project.Paths) == 0 {
			return fmt.Errorf("project %s requires at least one path", project.Name)
		}
		for _, source := range project.Paths {
			if err := ValidateDestination(source); err != nil {
				return fmt.Errorf("project %s source: %w", project.Name, err)
			}
			// Sources may be missing after a disk failure. Restore and repository
			// inspection must remain available; Backup checks source availability.
			for _, protected := range []string{cfg.StateDir, cfg.PasswordFile} {
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
