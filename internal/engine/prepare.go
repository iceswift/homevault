package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"homevault/internal/config"
)

// Python's stdlib wraps SQLite's online backup API; no SQL/path interpolation.
const sqliteBackup = `import sqlite3,sys,pathlib,os
src=sqlite3.connect(pathlib.Path(sys.argv[1]).as_uri()+'?mode=ro',uri=True,timeout=5)
dst=sqlite3.connect(sys.argv[2])
src.backup(dst,pages=256,sleep=0.05)
assert dst.execute('PRAGMA journal_mode=DELETE').fetchone()==('delete',)
assert dst.execute('PRAGMA integrity_check').fetchall()==[('ok',)]
dst.close()
src.close()
os.chmod(sys.argv[2],0o600)
`

func tool(configured, fallback string) string {
	if configured != "" {
		return configured
	}
	return fallback
}

// No child stderr or libpq ambient settings can leak credentials or change target.
func preparationCommand(ctx context.Context, binary string, args []string, extraEnv ...string) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "PG") && !strings.HasPrefix(strings.ToUpper(key), "PYTHON") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	configureCancellation(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("database export/validation failed (child output withheld)")
	}
	return nil
}

func (e *Engine) prepareDatabases(ctx context.Context, project config.Project) (stage string, err error) {
	if len(project.Databases) == 0 {
		return "", nil
	}
	if runtime.GOOS != "linux" {
		return "", errors.New("database preparation currently requires Linux")
	}
	// State exists under the operation lock. Refuse shared writable staging parents.
	info, err := os.Stat(e.cfg.StateDir)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("database preparation requires private state_dir (0700)")
	}
	stage, err = os.MkdirTemp(e.cfg.StateDir, "prepared-")
	if err != nil {
		return "", errors.New("cannot create private database staging")
	}
	owned := stage
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(owned))
		}
	}()
	for _, db := range project.Databases {
		output := filepath.Join(stage, db.Name)
		if db.Type == "sqlite" {
			info, statErr := os.Stat(db.Source)
			if statErr != nil || !info.Mode().IsRegular() {
				return "", errors.New("SQLite source must be an existing regular file")
			}
			output += ".sqlite"
			err = preparationCommand(ctx, tool(e.cfg.PythonBinary, "python3"), []string{"-I", "-c", sqliteBackup, db.Source, output})
		} else {
			info, statErr := os.Stat(db.PasswordFile)
			if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() == 0 {
				return "", errors.New("PostgreSQL password_file must be a nonempty private regular file (0600)")
			}
			output += ".dump"
			err = preparationCommand(ctx, tool(e.cfg.PgDumpBinary, "pg_dump"), []string{"--no-password", "--format=custom", "--host", db.Host, "--port", strconv.Itoa(db.Port), "--username", db.User, "--dbname", db.Database, "--file", output}, "PGPASSFILE="+db.PasswordFile)
			if err == nil {
				err = preparationCommand(ctx, tool(e.cfg.PgRestoreBinary, "pg_restore"), []string{"--file=/dev/null", output})
			}
		}
		if err != nil {
			return "", err
		}
		info, statErr := os.Stat(output)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return "", errors.New("database export produced no regular nonempty artifact")
		}
		if err = os.Chmod(output, 0600); err != nil {
			return "", errors.New("cannot protect database artifact")
		}
	}
	// Manifest is deliberately free of connection details and credentials.
	data, _ := json.MarshalIndent(struct {
		Project     string    `json:"project"`
		PreparedAt  time.Time `json:"prepared_at"`
		Consistency string    `json:"consistency"`
	}{project.Name, time.Now().UTC(), "independent database snapshots; attached files are not an atomic application snapshot"}, "", "  ")
	if err = os.WriteFile(filepath.Join(stage, "manifest.json"), data, 0600); err != nil {
		return "", errors.New("cannot write preparation manifest")
	}
	return stage, nil
}
