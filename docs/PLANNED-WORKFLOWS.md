# Prepared workflow coordination (Linux, experimental)

Requires Python 3.11+ with system timezone data and a reviewed current HomeVault
binary. A disabled file-only starter is in deploy/workflow-plan.example.json;
the three deploy/homevault-workflow-*.service files are reviewable examples, not
installed units or a complete application-specific plan.

`scripts/planned-workflow.py` coordinates reviewed capture/staging with HomeVault's
existing encrypted `job-backup`/`capture-backup`, durable job records and redacted
Discord sender. It does not create timers, initialize repositories, select a
backup time, prune snapshots or install services. Plans are trusted operator
configuration, not a sandbox for untrusted commands, images or local users.

## Supported scope

| Adapter | Preparation and consistency | Gate |
|---|---|---|
| `sqlite` | Existing pinned single-container SQLite/media fresh-capture adapter; independent guardian, healthy resume before upload | Existing reviewed capture plan/window; new verified Go job ID and capture flag required |
| `sealed` | Consume the latest completed Saan-format local PostgreSQL/uploads bundle under its existing scheduler lock; verify checksums, archive safety, pinned images and deployment compatibility | Original capture time must be recent; configuration newer than its completion marker blocks upload |
| `json` | Stop one pinned healthy container, copy an explicit small JSON-file set, then restore healthy running state before upload; independent guard and stop hook rescue | Workflow explicitly enabled **and** a reviewed window selected/open; no window means no stop |
| `files` | Explicit regular deployment files and reviewed static trees, with per-file hashes and release-link checks | Stable individual files; not an atomic database or multi-application checkpoint |

The sealed format is currently specific to Saan's `.saan-scheduled-complete`,
SHA256SUMS, custom dump/uploads/images/manifest files and dated bundle layout.
It is not a general PostgreSQL scheduler. It does not call Saan's backup script,
stop its app/worker again or change its existing local backup/retention schedule.
The source must already have a private, operator-owned lock file. A locked/busy
source fails closed; no tight retry or forced unlock is performed.
The adapter checks completed-source age, not every intermediate failure flag from
the independent local producer. If that producer fails while an older completed
bundle is still within the allowed age, it is not immediately a stale cloud point.
Producer-specific immediate failure alerts remain separate from coordinator errors.

Do not use file-tree capture for live databases. Container captures assume the
reviewed container is the sole application writer; host-side/unknown writers are
not automatically excluded. JSON archive size is bounded, unexpected state files
are refused, and resume failure retains the guard for investigation. Hardware or
Docker failure can prevent prompt resume; the deadline is not a hard availability
guarantee. Never point a scheduled job at a previous frozen one-time staging tree.

## Prepared plan and commands

A plan contains version 1, an absolute private `root`, `base_config`, a reviewed
`homevault_binary`, `notifications_enabled` and explicit per-adapter `workflows`.
The base is a normal existing HomeVault config with the preserved repository/key,
shared state, explicit rclone configuration and private Discord webhook **file
path**. No URL, token or key bytes belong in the workflow plan. `sqlite` reuses its
base project; other adapters create a per-run project config pointing at the newly
staged bundle while retaining the shared HomeVault operation/credential locks.
Keep the coordinator's staging root outside the base config's `state_dir`,
repository and credential paths. For example use `/var/lib/homevault-workflows`
beside an existing `/var/lib/homevault` engine state, not inside it. Go correctly
refuses backup sources that overlap protected state/keys.

All operator-prepared workflows start with `enabled: false` and notifications
disabled. `preflight` still checks metadata/access/identity while disabled. It is
not a live capture, backup, provider-auth test or delivery test. An unselected JSON
window is reported separately. Monitoring says `not-activated`, never `healthy`,
for a disabled workflow. It does not fabricate a successful first run.

```sh
python3 /reviewed/planned-workflow.py --plan /private/workflows.json preflight all
python3 /reviewed/planned-workflow.py --plan /private/workflows.json monitor all --no-send
# Only after explicit activation/window review:
python3 /reviewed/planned-workflow.py --plan /private/workflows.json run all --confirm
```

`run all` processes workflows sequentially. A separate nonblocking suite lock
prevents overlapping coordinator runs; the original Go operation and rclone
credential locks still protect the repository. Do not clear stale locks blindly.
Policy/repository binding prevents an old status from silently becoming healthy
under changed workflow settings. Preserve old evidence and review state migration
when changing a bound policy; do not edit success timestamps to make it green.

## Freshness and alerts

Coordinator status is separate from Go backup status. Its last success advances
only after preparation/capture and the actual Go backup+metadata check succeed.
A failed preparation before Go starts is still persisted as a workflow failure;
the previous successful capture/upload timestamps remain intact. History is
bounded to 32 attempts. A running attempt older than two hours is unhealthy.

Monitoring checks **both original capture time and verified upload time**. Uploading
an old sealed dump cannot hide stale source data. An already uploaded identical
sealed source is skipped without advancing freshness. A future timestamp is not
accepted as healthy. Status/dedup state is atomic and private, and project messages
contain no rows, paths, keys or child output.

The new CLI primitive is deliberately narrow:

```sh
homevault --config /private/notification-config.json \
  notify-event PROJECT failed --confirm
```

Only `failed`, `stale` and `recovered` events are accepted. It reuses the existing
official-HTTPS Discord endpoint validation, disabled mentions, timeout and redaction.
It does not change backup history/freshness, accepts no custom message body, and
is refused by recovery-only configs. Operators should normally let the coordinator
call it. Do not wire a second independent notifier to the same job events.

Failure/overdue events are deduplicated until the incident kind changes; healthy
state after a delivered failure/overdue sends recovery. Routine success is quiet.
Delivery failure is not marked delivered and cannot erase a completed backup;
CLI exits nonzero so it remains visible. A later monitor can retry; there is no
inline retry loop or exactly-once guarantee. `--no-send` suppresses delivery.
A host that is off cannot send its own outage alert.

## Service preparation and activation gates

One operator prepared three **static** oneshot units: backup, preflight and monitor.
Only preflight/monitor are to be started during installation; the backup unit is
not started and no timer file is created. The worker account's existing Docker
access is root-equivalent; root-owned code does not make that an isolation boundary.

When installing code/config directories under a private umask, explicitly set
the reviewed final directory permissions (for example code 0755, configuration
0750 with the worker's read group). Merely passing mkdir a mode is insufficient.
With `ProtectHome=read-only`/`ProtectSystem=strict`, allow writes to only state,
staging, rclone refresh credentials and the **exact existing sealed-source lock
file**. Read-only application data is not enough if the coordinator opens its
shared lock read/write. Do not make the whole application directory writable.

Before activating any schedule, review windows/overdue limits, run one explicitly
approved real cycle, verify suite-specific Discord delivery, and review source
permissions/pins again after a deployment. Rehearsal evidence is separate from a
new backup's metadata check. Late catch-up must never stop apps outside the window.
Repository keys are reused, not regenerated for each run.

Staging is capped at 5 GiB of selected file inputs with at least 1 GiB free margin.
Per-run plaintext stages/configs are currently retained privately for inspection;
there is **no automatic local-stage cleanup or aggregate disk quota**. Review a
cleanup policy before enabling a recurring schedule. Root-owned host secrets and
image/bootstrap refresh remain reviewed maintenance tasks, not part of this
nightly four-adapter suite. Off-server keys/catalogs must be maintained separately.
No real Drive retention deletion or prune is introduced by this coordinator.

## Evidence limits

Linux fixtures covered activation/window guards, original capture-age monitoring,
failed preparation preserving last success, failed delivery retry/dedup/recovery,
repository/policy mismatch, lock contention, bounded command output, duplicate
sealed source and JSON-copy failure resuming the exact fake container. A separate
generated-data test passed real local Restic job backup/check/restore and capture
metadata validation, then failed preparation without losing the prior success.
Neither test stopped a real app, used Google Drive or sent real Discord.

Real source preflight is not certification of a first live coordinator run under
systemd. No coordinator live pause/upload, suite-specific live message delivery,
reboot/cancellation test or active cloud schedule is implied.
