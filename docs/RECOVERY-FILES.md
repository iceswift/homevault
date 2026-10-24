# Curated deployment-file staging (experimental)

Database/media snapshots alone cannot recreate an application. Preserve its
deployment definitions, required configuration and exact build/image references
alongside a consistent data capture.

`python3 scripts/prepare-recovery-files.py PLAN.json NEW_DIRECTORY` prepares an
explicit file list in a new private directory. It does not discover files, stop
services, export databases, upload anything or install timers. Linux is the
operator-tested platform. Python 3 standard library is sufficient.

```json
{
  "version": 1,
  "files": [
    {"source": "/srv/example/deploy/compose.yml", "target": "deploy/compose.yml"},
    {"source": "/srv/example/deploy/app.env", "target": "deploy/app.env"}
  ]
}
```

The destination parent must already exist and be private (0700 on Linux). The
helper refuses existing destinations, symlink sources/ancestors, special files,
target traversal and duplicate targets. Newly staged files use 0600 and directories
0700. It records SHA-256, original mode/UID/GID and byte count in manifest.json;
the manifest deliberately omits original absolute source paths. The original
plan remains sensitive. Restore ownership is a separate reviewed operation.

Source identity, size and timestamps are checked around copying. This detects
ordinary concurrent changes, not malicious privileged writers or all filesystem
races. Use trusted paths and a quiet deployment interval. It is a set of stable
individual files, **not an atomic database/application snapshot**. It provides no
timeout or capacity quota: bound selected input size and runtime externally.

Use the resulting directory as a separate ordinary file project with the existing
Restic encrypted backup flow. Keep repository credentials and state outside that
directory and retain the existing operation/credential locking domain. Restore
to a new separate destination and compare every manifest hash against the
pre-upload manifest. A repository metadata check alone does not prove file bytes.

Secrets in staging/restored directories are plaintext protected by OS permissions.
Restic encrypts the cloud copy; it does not encrypt local staging. Restrict access,
review cleanup after use, and do not commit plans, populated bundles or private
operator inventories. File deletion is not secure erasure.

For an existing application-produced backup bundle, verify its completion marker,
checksums, archive safety and database dump readability before staging. Record its
original capture time, verify deployment/image compatibility and do not describe
the later cloud upload time as the database recovery point. Full app restore
testing remains separate. Preserve image availability and off-server credentials
before claiming replacement-machine recovery.
