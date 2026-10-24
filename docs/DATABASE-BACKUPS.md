# Database preparation (Linux, experimental)

`backup PROJECT` now exports configured databases before invoking Restic, under the existing operation lock and timeout. No production application is configured automatically.

## SQLite

Requires python3 with its standard-library sqlite3 module. HomeVault opens the original in read-only mode, uses SQLite's online backup API (including committed WAL data), normalizes only the exported copy to DELETE journal mode so it can open on a read-only mount without WAL sidecars, then runs integrity_check against the exported database. The original database's journal mode is not changed. Missing sources are refused rather than silently creating empty databases.

Example project entry:

```json
{
  "name": "demo",
  "paths": ["/srv/demo/media"],
  "databases": [
    {"name": "main", "type": "sqlite", "source": "/srv/demo/database/app.db"}
  ]
}
```

## PostgreSQL

Requires pg_dump and pg_restore compatible with the server version. Exports one database in custom archive format, then reads the archive with pg_restore into /dev/null without connecting to or modifying a database. Any export/validation failure blocks Restic. This validates readability, not an application's ability to recover; perform a real restore drill too.

```json
{
  "name": "demo",
  "paths": ["/srv/demo/media"],
  "databases": [
    {
      "name": "main", "type": "postgres",
      "host": "127.0.0.1", "port": 5432,
      "database": "demo", "user": "backup",
      "password_file": "/etc/homevault/private/pgpass"
    }
  ]
}
```

Use a libpq password file (0600), not a DSN/password in JSON, arguments or chat. Provision a suitably restricted backup account. No roles/global objects are exported; preserve role/bootstrap configuration separately. Docker discovery/exec and remote TLS configuration are not automated; use reviewed local connectivity for now. HomeVault strips inherited PG settings so they cannot redirect the export. Existing production ports or containers must not be changed merely to use this example.

Optional top-level executable overrides: python_binary, pg_dump_binary, pg_restore_binary. `doctor` currently checks Restic/rclone only; database tooling and credentials are checked during preparation.

## Safety and recovery

- Database-only projects may use an empty paths array. Existing file-only configurations still work.
- File paths must not overlap configured live SQLite files or PostgreSQL password files, across any project. Never include a live PostgreSQL data directory in paths; HomeVault cannot automatically identify every database file.
- Exports are staged in a new private directory under state_dir (0700), artifacts are 0600, and only validated exports are passed to Restic. The stage is removed after success/failure; abrupt power loss/SIGKILL can leave plaintext remnants. Use encrypted local storage and review abandoned staging while no operation is running. Deletion is not secure erasure.
- Ordinary attached paths are backed up by Restic as before. Each database is independently consistent, but databases and media are NOT an atomic shared application snapshot. Arrange an application-specific write pause/maintenance window externally when cross-resource consistency matters. HomeVault does not stop applications automatically.
- Restore still requires a full snapshot ID and a separate empty destination. Prepared files appear under the saved state_dir/prepared-* path inside the restored tree. Missing original database/password files must not prevent restoring snapshots; export credentials are only checked at backup time.
- Restored dumps may contain privileged SQL; only restore trusted archives into isolated, reviewed targets. This feature never imports into production automatically.

## Evidence

Linux synthetic tests covered a live SQLite WAL connection, integrity check and query of the export; encrypted Restic backup/restore with the original DB removed; PostgreSQL 16 custom dump restored into a separate fixture DB and queried; failed export blocking Restic. The PostgreSQL fixture used an isolated no-network container, no host ports and no production volume, and was removed after testing. No real project data was exported or uploaded.

References: [SQLite Online Backup API](https://www.sqlite.org/backup.html), [PostgreSQL pg_dump](https://www.postgresql.org/docs/current/app-pgdump.html).
