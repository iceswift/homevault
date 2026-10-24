# HomeVault development direction

Chosen direction (24 October 2026): make backup and verified recovery easier for Docker home-server applications. Do not claim that TUI, encrypted cloud backup, scheduling or rclone integration are novel, or that this prototype is faster/more reliable than mature alternatives without comparative evidence.

## Working foundation

CLI/TUI, file backups through Restic, local/cloud snapshots, guarded restore, process locking/cancellation, and a synthetic Google Drive round trip. Current limits and verification evidence live in TESTING.md.

## Next development milestones

1. Database-aware preparation: experimental Linux PostgreSQL exports and SQLite online backup implemented, with private staging and failure before upload. See DATABASE-BACKUPS.md. Remaining: application-specific coordinated write pause, real-project onboarding and broader compatibility tests. No blind copy of live database storage.
2. Application recovery rehearsal: isolated app recovery and optional ephemeral PostgreSQL import implemented, with health/database/media/login reports. Python fixture, actual DealDoor image with new demo data, and a synthetic PostgreSQL+app stack passed. Invalid SQL import fails closed with cleanup. See RECOVERY-REHEARSAL.md and POSTGRES-RECOVERY.md. Remaining: production-data recovery onboarding, broader app/version compatibility and crash recovery. Production stacks remain unchanged.
3. Guided onboarding: project/source selection, storage/OAuth setup, recovery-key instructions, preflight checks and a clear first restore drill. Production installation remains manual.
4. Durable job state: implemented bounded job history, distinct backup/check outcomes, last-success preservation, freshness monitoring and optional deduplicated Discord failure/overdue/recovery delivery. Restore/rehearsal proof remains separate. See SCHEDULING-ALERTS.md.
5. Scheduled operations and retention: fresh SQLite capture, maintenance-window gate, guardian/rescue helpers and dedicated systemd templates implemented and fixture-tested. DealDoor live-source metadata preflight passed without stopping it. No services/timers enabled (owner chose preparation only). Live capture rehearsal, private webhook provisioning, source permissions and owner activation remain prerequisites. Retention/deletion remains unimplemented.

Public website must label these as planned until implemented and tested. Google OAuth Production status is separate from product maturity or public launch readiness.
