# HomeVault

A small, open-source Go CLI/TUI that orchestrates encrypted Restic backups. No web server, listening port, or always-running HomeVault daemon. Early development software: use generated fixtures first, keep independent backups, and prove restore before trusting it with an application.

## Implemented scope

- Explicit repository initialization, project backup, snapshot listing, integrity check and restore.
- Local repositories or an already-configured rclone remote (Google Drive transport).
- One operation per state directory, context timeout/cancellation and argument-array subprocess execution.
- Restore requires an explicit full snapshot ID and a new/empty destination; configured source, state, secret and repository paths are protected.
- Keyboard TUI: arrows select project; `b` backup+check job, `s` snapshots, `c` repository check, `q` quit. Quitting cancels interactive work; scheduled CLI work runs independently via systemd.
- TUI job history (`h`), read-only retention preview (`p`), recovery guide (`g`) and Page Up/Down scrolling. `b` runs a recorded backup+metadata-check job; capture-required projects use the separate reviewed CLI workflow.

New users: follow [onboarding and new-machine recovery](docs/ONBOARDING.md). `setup` generates a private starter config/checklist using an existing repository key; it never initializes, uploads or enables services. Linux preview packages and draft-release workflow are described in [releasing](docs/RELEASING.md).

Experimental Linux database preparation now supports SQLite online backup and PostgreSQL custom dumps before Restic runs. See [database configuration and limits](docs/DATABASE-BACKUPS.md). Database exports and attached files are not a shared atomic application snapshot; coordinate application writes externally when needed. Never point file paths directly at running PostgreSQL storage or copy live SQLite files as ordinary files.

Experimental [backup jobs and alerts](docs/SCHEDULING-ALERTS.md) provide persisted backup/check status, the latest 32 attempts, freshness checks, optional minimal Discord notifications and reviewable systemd templates. They are not installed/enabled automatically. Plain backup and rehearsal remain distinct from a scheduled job's status.

[Retention](docs/RETENTION.md) shows daily/weekly/monthly keep reasons and candidates without deleting anything. A separate experimental manual apply command requires a saved matching review, a preserved kept ID and explicit confirmation; it removes snapshot records only. Pruning, OAuth wizard and automatic scheduler installation are **not implemented** yet. A real application still needs a reviewed live-capture plan; do not schedule repeated backups of a frozen one-time copy.

The experimental Linux [fresh SQLite capture adapter](docs/FRESH-CAPTURE.md) supports a pinned single-container deployment, maintenance-window gating, DB+WAL/media capture under the job lock, independent resume attempts and a config-free recovery helper. A live plan, exclusive-writer review, private webhook and explicit service activation are still required; no production timer is enabled automatically.

Experimental Linux [recovery rehearsal](docs/RECOVERY-REHEARSAL.md) can restore a snapshot, start a constrained application with an optional [ephemeral PostgreSQL container](docs/POSTGRES-RECOVERY.md), run explicit health/database/media/login checks and write a cleanup-aware report. Arbitrary multi-service stacks and production-project onboarding are not implemented; synthetic proof is not evidence that your production app can recover.

## Build and verify

Requires Go 1.24+, an official Restic executable, and rclone for cloud repositories. Dependencies are pinned in go.mod/go.sum.

Ubuntu/Linux is the initial supported target. Windows builds and unit tests work, but real Windows restore encountered ACL/access-denied failures in this development environment and is not validated for use.

```sh
go test ./...
go build -trimpath -o bin/homevault ./cmd/homevault
# Integration test uses only generated data under the test temporary directory:
HOMEVAULT_TEST_RESTIC=/absolute/path/to/restic go test ./integration -v
```

Create a private sandbox with separate `source`, `state`, `repository`, `restore` and `secrets` paths. Put only generated sample files in `source`. Store the repository password outside sources with mode 0600 on Linux. Keep a separate recoverable copy of that password: losing it loses access to the backups. Windows users must restrict file ACLs themselves; POSIX permission bits are not a Windows ACL audit.

`homevault demo-setup /absolute/new-lab` generates a brand-new synthetic source, random repository password and homevault.json; it refuses to adopt an existing directory. The parent directory must exist. No cloud connection is made.

Example configuration (replace every path with an absolute path appropriate to your system):

```json
{
  "version": 1,
  "state_dir": "/home/ice/homevault-lab/state",
  "repository": "/home/ice/homevault-lab/repository",
  "password_file": "/home/ice/homevault-lab/secrets/restic-password",
  "projects": [{"name": "synthetic", "paths": ["/home/ice/homevault-lab/source"]}]
}
```

Commands (global flags go before the command):

```sh
homevault --config /absolute/homevault.json doctor
homevault --config /absolute/homevault.json init
homevault --config /absolute/homevault.json backup synthetic
homevault --config /absolute/homevault.json snapshots synthetic
homevault --config /absolute/homevault.json check
homevault --config /absolute/homevault.json restore synthetic FULL_SNAPSHOT_ID /absolute/new-restore --confirm
homevault --config /absolute/homevault.json tui
```

`doctor` checks configuration and executable availability, not OAuth, repository health or restore correctness. Every nonzero Restic exit fails the operation, including partial backups. The application lock is intentionally not removed automatically after a crash; verify no HomeVault/Restic/rclone work remains before manually recovering it. Different state directories are not a global machine lock: use one state directory for the deployment. Do not concurrently manage the repository with other tools.

## Google Drive

HomeVault does not require a Drive connector in the chat application. Configure rclone locally via its browser OAuth flow; do not paste passwords, OAuth tokens, rclone.conf or repository keys into chat or Git. Current rclone documentation recommends a dedicated Google OAuth client because the shared client is being retired during 2026. Prefer the narrow `drive.file` scope for a dedicated backup folder if its limitations suit the setup.

After separately authorizing the intended account and testing rclone, set `repository` to `rclone:gdrive:HomeVault/synthetic`. Restic supplies encryption; an additional rclone crypt layer is unnecessary here. Cloud transfers are not automatically initiated by opening a Drive browser tab. Start with synthetic fixtures and perform a complete cloud restore before enabling real data.

Official references: [Restic rclone backend](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html), [rclone Google Drive setup](https://rclone.org/drive/).

## Scheduling and recovery

The units in `deploy/` are reviewable templates, not installed automatically. Provision the service user, executable and protected configuration first, and adapt ReadWritePaths for the repository/cache/staging paths. The sample unit protects home directories; the example home-directory sandbox therefore cannot be used unchanged with that unit. Validate actual permissions and run a manual service test before enabling the timer. systemd supplies scheduling and process lifecycle so SSH does not need to stay open.

Do not delete failed restore output and claim success. Investigate it and restore again into a fresh target. Restic's normal check is not a complete application restore drill; compare file hashes and validate the database/application after restoring prepared exports.

## Project status

MIT licensed. Source: [iceswift/homevault](https://github.com/iceswift/homevault). No production service, database, cloud token or existing Drive file is required for local development. See [verification notes](docs/TESTING.md) and [roadmap](docs/ROADMAP.md) for tested behavior and remaining work.
