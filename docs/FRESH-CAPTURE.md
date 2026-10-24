# Fresh SQLite application capture (Linux, experimental)

This adapter prepares a new DB/media copy for every job, resumes the application **before** Restic upload, and holds the same engine operation lock across capture, upload and repository check. No timer is installed or enabled automatically. The current owner chose preparation/testing only, not another production stop or a real Discord test.

## Supported deployment

Exactly one trusted container, one /data bind mount, one configured SQLite file and one media directory beneath that data root. The container must be the **sole writer** of both resources and have a meaningful Docker HEALTHCHECK. Host processes/other containers writing the same data are not quiesced by this adapter. PostgreSQL/live multi-container capture, other mounts and arbitrary hooks are not supported here.

The application must tolerate its bounded Docker stop or provide a reviewed graceful shutdown. SQLite recovers committed WAL transactions, but a forced stop cannot guarantee completion of an in-flight logical operation spanning DB and filesystem. A stopped copy is not a distributed/application transaction protocol; meaningful application restore checks are still required.

The normal HomeVault project configuration must reference the actual live DB/media, not the old one-time frozen capture. Keep repository, repository password and state_dir stable between runs. Never generate a new repository/key per night, and do not claim data is fresh just because a new snapshot was uploaded from an old copy.

Set `"capture_required": true` on this project. Ordinary backup/job-backup are then refused; only capture-backup can advance its job state. Monitoring also refuses to treat a legacy ordinary success as a healthy capture result. This guards against accidentally wiring the generic file-backup timer to a live-app configuration.

## Explicit capture plan

```json
{
  "container_id": "REPLACE_WITH_REVIEWED_64_HEX_CONTAINER_ID",
  "image_id": "sha256:REPLACE_WITH_REVIEWED_64_HEX_IMAGE_ID",
  "data_root": "/srv/example/data",
  "window_start": "03:15",
  "window_minutes": 15,
  "timezone": "Asia/Bangkok",
  "max_pause_seconds": 60
}
```

These placeholders intentionally do not validate. Inspect the deployment privately and review/pin actual identities. Container names are not accepted. A new deployment/container/image requires plan review; it must not silently capture a different release. Review the configured DB/media paths too: this is not automatic application discovery. The optional guardian_binary must be an absolute path to the same reviewed HomeVault CLI; normally the current executable is used.

```sh
homevault --config /absolute/live-homevault.json --timeout 2h \
  capture-backup PROJECT /absolute/capture-plan.json --confirm
```

For a metadata/read-access check only, without stopping, writing job state or backing up:

```sh
homevault --config /absolute/live-homevault.json capture-check PROJECT /absolute/capture-plan.json
```

This checks source identity/layout/read access; it does not validate a live capture, upload, recovery, source exclusivity or future scheduler permissions.

Outside the reviewed local maintenance window, the CLI skips without stopping the app or advancing last success. Windows crossing midnight are supported. A missed timer does not justify an unexpected daytime stop. The engine lock is acquired before any stop, preventing overlapping captures/jobs/restores that share state_dir.

## Capture sequence

1. Validate pinned container/image, healthy/running state, exact bind root and live configured source paths. Refuse missing/unreadable DB/media, media symlinks/special files, old frozen paths and a previous unresolved guard. Docker metadata inspection deliberately excludes production environment/secrets.
2. Create a new private captured-* directory under state_dir. Persist a private guard with the exact container/image, nonce and short deadline. Launch the independent guardian and wait for its nonce-bound readiness acknowledgment. If it cannot start, do not stop the app.
3. Stop only the pinned container with a bounded grace period. While it remains stopped, copy the SQLite file and any WAL into private staging (not the shared-memory sidecar). Reconstruct/validate a standalone SQLite export from that **offline DB+WAL pair**, then copy media. The original database is never reconfigured/checkpointed by the adapter. This avoids needing to create sidecars in a read-only production directory and includes committed data still present in WAL.
4. Reject the capture if the source restarted or changed start identity during copying. Use an independent bounded context to start the exact same container and await its Docker healthy state even if the main operation was cancelled. Only a successful capture AND healthy resume proceed to upload.
5. Back up the fresh media, private capture metadata and prepared DB to the existing encrypted repository. Perform the metadata check; last success advances only after the complete job succeeds. A previous success is preserved on capture/upload/check failure. CapturePassed is recorded separately in job history.
6. Remove this owned temporary capture after success. Failed captures are retained privately for diagnosis, never silently reused on the next run. Retention/staging garbage collection remains manual; inspect disk usage. The encrypted snapshot remains restorable after staging is removed.

## Guard and recovery limitations

The guardian is a short-lived separate Linux session/process, not an idle daemon. After the deadline it attempts to restart only the recorded container/image and waits for health, with one bounded retry. Parent/SSH process failure alone does not intentionally cancel this independent process. Wrong identity, unavailable Docker or health failures leave the guard for inspection instead of deleting evidence or restarting another app. A stop-command timeout also retains the guard to cover a late Docker stop completion.

```sh
# Expired guard only; works without repository password/config/operation lock:
homevault resume-paused /absolute/state/guards/PROJECT.json
# Operator or reviewed systemd ExecStopPost, only after the job has stopped:
homevault resume-paused /absolute/state/guards/PROJECT.json --force
```

Before the deadline, forced recovery refuses to interrupt a still-running recorded owner (or an unknown legacy owner). Expired recovery remains available even when a stuck owner is still alive. PID reuse may conservatively delay an early forced resume until the short deadline; it never authorizes restarting a different container.

This is **best-effort recovery**, not an uptime guarantee. Linux sessions do not escape systemd control groups: systemd may kill every service process, including the guardian. The prepared capture service therefore includes ExecStopPost recovery, and its monitor includes an expired-guard recovery step after boot. These are examples only, not installed or reboot-tested. Power loss/filesystem damage/Docker failure can still prevent recovery. An off host cannot notify/restart itself; use an independent monitor where required. Inspect a stale backup operation lock manually; the resume helper never removes it automatically.

## Prepared systemd integration

Review homevault-capture.service/timer and homevault-capture-monitor.service/timer under deploy/. The capture timer uses no persistent daytime catch-up; the plan additionally gates the window. The monitor can catch up and only resumes an **expired** guard. Match project name/state_dir in every guard path. Provision the service user, paths, source permissions, stable key/repository and writable private rclone config; explicitly review Docker access (root-equivalent). No group membership or privileges are granted by this code. ProtectHome means a home-directory installation needs reviewed unit changes.

## Verification scope

Linux unit tests exercise operation-lock coverage, rejecting frozen sources, missing/cancelled capture, healthy resume, preserved last success and image-mismatch refusal. The opt-in real integration fixture runs two jobs with changed DB/media, restores both versions through Restic, verifies committed WAL-only data, tests a capture failure and resumes the fixture, then simulates an orphaned expired pause with a missing repository key and stale lock. This is not a production stop, whole-system reboot test or proof against every SIGKILL/systemd failure scenario. No real webhook is used.

```sh
HOMEVAULT_TEST_RESTIC=/absolute/restic \
HOMEVAULT_TEST_CAPTURE_IMAGE=python:3.12-alpine \
HOMEVAULT_TEST_CAPTURE_BINARY=/absolute/new-homevault-cli \
go test ./integration -run TestFreshCaptureJobsAndOrphanRecovery -v
```

Only local reviewed fixture images/binaries are used; tests do not pull images. Companion instructions: SCHEDULING-ALERTS.md and DATABASE-BACKUPS.md. Official lifecycle references: [systemd ExecStopPost](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html), [systemd control-group termination](https://www.freedesktop.org/software/systemd/man/latest/systemd.kill.html).
