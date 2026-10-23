# HomeVault development direction

Chosen direction (24 October 2026): make backup and verified recovery easier for Docker home-server applications. Do not claim that TUI, encrypted cloud backup, scheduling or rclone integration are novel, or that this prototype is faster/more reliable than mature alternatives without comparative evidence.

## Working foundation

CLI/TUI, file backups through Restic, local/cloud snapshots, guarded restore, process locking/cancellation, and a synthetic Google Drive round trip. Current limits and verification evidence live in TESTING.md.

## Next development milestones

1. Database-aware preparation: PostgreSQL exports and SQLite online backup, explicit credentials, private staging, validated output and failure before upload if export fails. No blind copy of live database storage.
2. Application recovery rehearsal: restore into an isolated new Compose project, validate database/media/health, produce a recovery report and leave the production stack unchanged.
3. Guided onboarding: project/source selection, storage/OAuth setup, recovery-key instructions, preflight checks and a clear first restore drill.
4. Durable job state: distinguish backup uploaded, repository checked, file restored and application recovery verified. Add failure notifications and recovery instructions.
5. Scheduled operations and retention: reviewable systemd installation, safe cancellation, resource/IO measurements and explicit retention policies after restore is proven.

Public website must label these as planned until implemented and tested. Google OAuth Production status is separate from product maturity or public launch readiness.
