# Isolated application recovery rehearsal (experimental, Linux)

The new `rehearse` command restores an explicit snapshot into a **brand-new** directory, launches a constrained Docker Compose project (one app, optionally one PostgreSQL database), runs four reviewed checks, removes its containers, and saves a private JSON report. It does not replace a production application or run its Compose file.

```sh
homevault --config /absolute/homevault.json --timeout 3m \
  rehearse PROJECT FULL_64_CHARACTER_SNAPSHOT_ID /absolute/new-restore \
  /absolute/recovery-plan.json --confirm
```

Requires a local Docker Engine socket at /var/run/docker.sock and Docker Compose v2. The operator needs Docker access (root-equivalent access to the host). Only already-installed, trusted images may be used: the command never pulls/builds images. Do not treat a container as a security boundary for hostile code or malicious database dumps.

## Plan, not a production Compose file

The strict JSON plan accepts only image, command, mounts, checks and an optional postgres block. Review the images and commands before `--confirm`. Never copy production credentials, webhook tokens or provider integrations into the plan; generated Compose files retain the command arguments on disk. No implicit production .env file is loaded.

Example structure (replace the commands with real, meaningful application assertions):

```json
{
  "image": "your-reviewed-app:exact-version",
  "command": ["/app/start-recovery-mode"],
  "mounts": [
    {"prepared": true, "target": "/data/db", "writable": true},
    {"source": "srv/example/media", "target": "/data/media"}
  ],
  "checks": [
    {"kind": "health", "command": ["/app/recovery-check", "health"]},
    {"kind": "database", "command": ["/app/recovery-check", "database"]},
    {"kind": "media", "command": ["/app/recovery-check", "media"]},
    {"kind": "login", "command": ["/app/recovery-check", "login"]}
  ]
}
```

`source` is a path relative to the restored tree, not a current host source. For an original Linux path /srv/example/media, Restic restores it under TARGET/srv/example/media. `prepared` selects the unique prepared-* directory containing a manifest for the chosen project. Ambiguity fails closed. Mount targets must be /data or descendants and cannot overlap; no source paths outside the restored tree are accepted. Symlinks and special files in the restored tree are refused.

## Fixed isolation policy

- Random `homevault-drill-` Compose project name; no production project/container names.
- Network disabled, no published ports, no access to production Docker networks. With PostgreSQL enabled, the app shares the database container's network namespace, which has no external interface; only their common loopback is available.
- Read-only container root, all capabilities dropped, no-new-privileges, host operator UID/GID.
- Only restored-data bind mounts; no Docker socket, production volumes, secrets directory or arbitrary host mount.
- Per-container limits: 256 MiB RAM, 0.5 CPU, 64 processes, /tmp tmpfs 32 MiB. PostgreSQL also has a 128 MiB data tmpfs and a 1 MiB socket tmpfs. No restart policy or container logs retained.
- Commands use argument arrays, with Compose dollar interpolation escaped. Child output is discarded to avoid logging secrets.
- All four check kinds are mandatory and must exit zero; each has bounded readiness retries and per-attempt timeout. Check scripts must assert actual behavior, not merely return success. Generic tooling cannot know whether a user-supplied check is meaningful.

The report records snapshot/project, restore result, PostgreSQL import result when applicable, passed checks, cleanup result and timing, not child output or credentials. It is saved under state_dir/recovery-*/report.json even on ordinary failures after report initialization. Partial restore files are preserved for diagnosis. Containers and image-created anonymous volumes belonging to this generated Compose project are removed on success, failure and cancellation using an independent cleanup timeout. Restored bind-mounted files are preserved. Cleanup errors make the run fail. Power loss/SIGKILL can leave resources; inspect the report/Compose project before manual cleanup. There is no automatic broad prune or cleanup of other projects.

## Current boundaries

The current implementation supports a single-container app, optionally paired with [one ephemeral PostgreSQL cluster](POSTGRES-RECOVERY.md). Arbitrary Compose stacks, Redis/other sidecars, multiple databases, automatic application migrations, application write-pause orchestration, production credential substitution and real-project onboarding are **not implemented**. Do not claim a real application's recovery is validated because a synthetic fixture passed.

No production app is stopped or reconfigured automatically. Plaintext restored files and reports remain in private directories; retain/remove them deliberately after review. This is not secure erasure or a replacement for encrypted local storage. Keep the original repository and recovery password separately.

## Verified fixture

The Linux opt-in integration test constructs a SQLite database and a tiny bundled Python HTTP application with generated media. It performs encrypted Restic backup, deletes the original DB, restores the snapshot, starts the app from restored code/data, checks health, a DB row, HTTP media bytes and a synthetic login response, and checks that no container remains for that project. It also exercises a failing check/timeout and independent cleanup. The fixture is not DealDoor, Saan or production authentication.

```sh
HOMEVAULT_TEST_RESTIC=/absolute/restic \
HOMEVAULT_TEST_RECOVERY_IMAGE=python:3.12-alpine \
go test ./integration -run TestIsolatedApplicationRehearsal -v
```

Install/review the official fixture image separately before running; the test does not pull it. Docker isolation fields follow the [Docker Compose service reference](https://docs.docker.com/reference/compose-file/services/).

## DealDoor image rehearsal (24 October 2026)

An additional opt-in test passed on Ubuntu using the already-installed DealDoor image tagged 84c478b. This uses the **actual application image**, but only its newly generated demo database and media, never production volumes, .env or accounts.

1. Start a private no-network, no-host-port fixture container with Demo restrictions enabled and a newly created data directory.
2. Seed the demo through the image's normal startup, add a random database canary not present in its seeder, stop only the fixture container, and add a generated media proof file.
3. Back up through HomeVault's SQLite Online Backup preparation and encrypted Restic repository, then move the original fixture DB away.
4. Restore and run the same DealDoor image in an isolated generated Compose project. Assert healthy DB/API, the original random canary and seeded users, identical media bytes through the app's upload route, login using the public seed account, session-cookie issuance, and authenticated dashboard access. Anonymous dashboard access must redirect.
5. Verify the recovery report passed and container cleanup completed. The test's temporary files and seed container are cleaned up; the production DealDoor health endpoint remained HTTP 200.

```sh
HOMEVAULT_TEST_RESTIC=/absolute/restic \
HOMEVAULT_TEST_DEALDOOR_IMAGE=dealdoor:84c478b \
go test ./integration -run TestDealDoorDemoRecovery -v
```

This test requires that the operator already has that reviewed image; HomeVault does not download a DealDoor image or fetch the project. The demo credentials are public seed fixtures from the application source, not a server owner's credentials. The random canary guards against mistakenly passing by reseeding a new empty database. This is not proof of recovery for current production data or multi-container PostgreSQL applications. No Cloudflare/Tailscale route or production service is changed.
