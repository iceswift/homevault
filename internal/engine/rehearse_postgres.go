package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func recoveryPostgresService(root, project string, pg RecoveryPostgres) (map[string]any, error) {
	mounts, err := recoveryMounts(root, project, RecoveryPlan{Mounts: []RecoveryMount{{Prepared: true, Target: "/data/prepared"}}})
	if err != nil {
		return nil, err
	}
	source := filepath.Join(mounts[0].(map[string]any)["source"].(string), pg.Artifact+".dump")
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, errors.New("restored PostgreSQL archive is missing or unsafe")
	}
	return map[string]any{
		"image": pg.Image, "pull_policy": "never", "entrypoint": []string{"docker-entrypoint.sh"},
		"command":     []string{"postgres", "-c", "listen_addresses=127.0.0.1", "-c", "unix_socket_directories=/var/run/postgresql", "-c", "shared_buffers=16MB", "-c", "max_connections=20"},
		"environment": map[string]string{"POSTGRES_HOST_AUTH_METHOD": "trust", "POSTGRES_USER": "postgres", "POSTGRES_DB": "recovery", "PGDATA": "/var/lib/postgresql/data/pgdata"},
		"user":        strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "network_mode": "none", "read_only": true, "cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"},
		"mem_limit": "256m", "cpus": 0.5, "pids_limit": 64, "restart": "no", "logging": map[string]any{"driver": "none"}, "healthcheck": map[string]any{"disable": true},
		"tmpfs":   []string{"/tmp:rw,noexec,nosuid,size=32m,mode=1777", "/var/run/postgresql:rw,noexec,nosuid,size=1m,mode=1777", "/var/lib/postgresql/data:rw,noexec,nosuid,size=128m,mode=1777"},
		"volumes": []any{map[string]any{"type": "bind", "source": source, "target": "/data/restore.dump", "read_only": true, "bind": map[string]any{"create_host_path": false}}},
	}, nil
}

func retryRecoveryCommand(ctx context.Context, timeout time.Duration, args []string) error {
	deadline := time.Now().Add(timeout)
	for {
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := dockerCommand(attempt, args...)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return errors.New("recovery readiness timeout")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
