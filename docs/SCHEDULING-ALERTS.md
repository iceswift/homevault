# Backup jobs, freshness monitoring and Discord (experimental)

This release prepares the software and reviewable systemd templates. It does **not** install/enable services, stop applications, select a real webhook or send Discord messages automatically. The owner explicitly chose preparation only.

## Commands

```sh
homevault --config /absolute/homevault.json --timeout 2h job-backup PROJECT
homevault --config /absolute/homevault.json status PROJECT
homevault --config /absolute/homevault.json --timeout 30s monitor PROJECT --max-age 36h
homevault --config /absolute/homevault.json --timeout 15s notify-test PROJECT
```

`job-backup` holds the existing operation lock across backup and repository metadata check. It atomically persists a running record before work, backup/check phase results, completion time and the latest 32 attempts under state_dir/jobs. LastSuccess advances only when both phases pass. A failed/cancelled job preserves the previous success. The record contains no child output, repository URL, webhook, database rows or session tokens; a repository fingerprint prevents using another repository's status by mistake.

A successful job proves backup upload and metadata check, **not** application recovery. Rehearsal reports remain separate. Plain `backup` keeps its previous behavior and does not advance job freshness. A missing/corrupt status file must not become a green success. Generated status files are private (0600), with private directories (0700 on Linux); atomic replacement and file synchronization are used. Abrupt power loss can still leave an incomplete running record/lock. HomeVault never automatically removes stale operation locks.

`monitor` reads atomic status without acquiring the backup lock, so a crashed process's stale lock cannot disable freshness checks forever. It reports never-completed/overdue jobs, last failed attempts and running jobs. It exits nonzero for stale/failed health or monitoring/delivery errors. Alert delivery uses a separate per-project lock and dedup state. An interrupted job is detected by age; this is not a process supervisor or immediate SIGKILL detector. A server that is off cannot send its own alert: use an independent external monitor if immediate off-host outage detection is required.

## Optional Discord configuration

Add only a private **file path**, never the URL/token itself, to configuration:

```json
{
  "discord_webhook_file": "/etc/homevault/discord-webhook",
  "notify_success": false,
  "backup_max_age": "36h"
}
```

These are optional top-level fields merged into the ordinary complete HomeVault configuration. The webhook file must be a regular private file (0600 on Linux) readable by the service user. It cannot overlap a configured file source/live database. Do not use a previously exposed webhook without rotating it privately. Windows users must enforce ACLs themselves; Unix mode bits are not Windows ACL validation.

Notifications contain only the safe configured project name and a generic event: failure, overdue, recovery, optional success or explicit test. Mentions are disabled. Database content, paths, snapshots, child output and credentials are never included. Only official HTTPS discord.com webhook URLs are accepted; redirects are not followed and network errors are sanitized so they cannot echo a token-bearing URL. A request is bounded to eight seconds. Discord rejection/rate limit/network failure is reported, not silently ignored or retried in a tight loop.

Failure and overdue events are deduplicated until the event changes; a later healthy job emits recovery. Successful routine jobs stay quiet by default. Failed delivery is not marked delivered, so the next monitor call retries. Network ambiguity can still duplicate a message; exactly-once delivery is not promised. A notification error does not change a completed backup into a failed backup in persisted status, but the CLI exits nonzero to expose the delivery problem. Job notification is attempted after releasing the operation lock, with an independent bounded context even when backup was cancelled.

## systemd templates and reboot behavior

The files in deploy/ are **examples**, not installed configuration. They use a dedicated homevault user, protected configuration under /etc/homevault and writable private state under /var/lib/homevault. Provision/review the user, executable, source permissions, paths and credentials before use. Rclone's writable refresh-token file belongs under /var/lib/homevault/private/rclone.conf, not a read-only /etc path; protect it as 0600 and never include it in sources.

The backup timer example is 03:15 Asia/Bangkok, with persistent catch-up. The monitor example runs hourly with persistent catch-up. Enabling system timer units makes them available after reboot, but that has not been activated here. Validate calendar syntax/unit paths, run each service manually, exercise a controlled failure and inspect status/notification before enabling a timer. Review ReadWritePaths for local repositories or staging outside /var/lib/homevault. ProtectHome means home-directory fixtures do not work unchanged.

For application-stopping workflows, persistent catch-up could otherwise stop an app during daytime after a missed night run. Add an explicit maintenance-window gate or choose a reviewed non-catch-up policy before deployment. Do not copy the generic template into production unchanged. No reboot or production stop is needed to validate these prepared templates.

## DealDoor activation gates

The previously verified DealDoor configuration points to a **frozen one-time capture**. Scheduling job-backup against that file would repeatedly back up old data and create misleading freshness timestamps. Do not do that.

The separate live project should set capture_required=true, preventing accidental plain backup/job-backup and requiring a successful capture result for healthy monitoring.

The [fresh-capture adapter](FRESH-CAPTURE.md) is now implemented and fixture-tested with a stable repository/password, a shared capture/upload/check lock, bounded stop/resume, an independent guard, durable capture-failure status and a maintenance-window gate. A metadata-only preflight and separate private live-source plan have been prepared for DealDoor; this did not stop/capture the real application or activate a schedule.

Before any DealDoor timer is enabled, review that plan and the exclusive-writer assumption, confirm portable recovery-key custody, rehearse a new real-source capture in an explicitly approved window, and verify webhook delivery privately. Reuse the preserved recovery key; do not generate a new key per night. Installation on this M70q also requires owner sudo authorization; noninteractive sudo is not available and user lingering is not enabled. No permissions were widened as a workaround.

## Verified with fixtures

Unit tests use fake Restic and HTTP transports, never real Discord. They cover backup/check phase locking, history bounds, preserving last success on failure, overdue detection despite a stale operation lock, webhook endpoint restrictions, mention suppression, redaction, notification dedup/recovery and failed-delivery retry. An opt-in real Restic test validates encrypted job backup/check, durable state and failed-source preservation with generated files. Linux is the supported deployment target; restricted Windows filesystem environments may reject atomic rename.

These checks passed on Ubuntu, including the real Restic job test. Windows unit tests and go vet passed outside the restricted atomic-rename sandbox. The systemd templates passed offline syntax/dependency verification using temporary copies with an existing executable/user stub; the Asia/Bangkok calendar expression parsed correctly. This does not validate production source permissions, webhook delivery, an application-stopping adapter, real service execution or reboot behavior. Nothing was installed/enabled or rebooted, and no Discord message was sent.

Official references: [systemd.timer](https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html), [Discord webhooks](https://docs.discord.com/developers/resources/webhook).
