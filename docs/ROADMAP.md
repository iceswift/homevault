# HomeVault development direction

Chosen direction (24 October 2026): make backup and verified recovery easier for Docker home-server applications. Do not claim that TUI, encrypted cloud backup, scheduling or rclone integration are novel, or that this prototype is faster/more reliable than mature alternatives without comparative evidence.

## Working foundation

CLI/TUI, file backups through Restic, local/cloud snapshots, guarded restore, process locking/cancellation, and a synthetic Google Drive round trip. Current limits and verification evidence live in TESTING.md.

## Next development milestones

1. Database-aware preparation: experimental Linux PostgreSQL exports and SQLite online backup implemented, with private staging and failure before upload. See DATABASE-BACKUPS.md. Remaining: application-specific coordinated write pause, real-project onboarding and broader compatibility tests. No blind copy of live database storage.
2. Application recovery rehearsal: isolated app recovery and optional ephemeral PostgreSQL import implemented, with health/database/media/login reports. Python fixture, actual DealDoor image with new demo data, and a synthetic PostgreSQL+app stack passed. Invalid SQL import fails closed with cleanup. See RECOVERY-REHEARSAL.md and POSTGRES-RECOVERY.md. Remaining: production-data recovery onboarding, broader app/version compatibility and crash recovery. Production stacks remain unchanged.
3. Guided onboarding: a guarded setup command generates a private starter config and restore checklist using an existing key, with separate file/SQLite inputs and optional capture-required policy. The Drive OAuth wizard adds private setup, PKCE browser consent and local/explicit online status; see DRIVE-OAUTH.md and ONBOARDING.md. Google Cloud client creation, PostgreSQL wizard and automatic production installation remain manual/future work.
4. Durable job state: implemented bounded job history, distinct backup/check outcomes, last-success preservation, freshness monitoring and optional deduplicated Discord failure/overdue/recovery delivery. Restore/rehearsal proof remains separate. See SCHEDULING-ALERTS.md.
5. Scheduled operations and retention: fresh SQLite capture, maintenance-window gate, guardian/rescue helpers and dedicated systemd templates implemented and fixture-tested. One owner-operated deployment also passed a fresh live capture/rehearsal, real Discord delivery tests and manually installed systemd preflight/monitor services. Its capture service has not been run under systemd and no timer is enabled. See OPERATOR-VALIDATION.md for exact scope. Reviewed manual retention/pruning is described below; automatic retention/pruning is not enabled or implemented.

Public website must label these as planned until implemented and tested. Google OAuth Production status is separate from product maturity or public launch readiness.

Disaster recovery update: age-encrypted portable credential kits and guarded
recovery-only configs are implemented. Generated-data drills recovered after the
original source/key/state were removed, including a fresh network-none container
with DB/media/app health/login checks. See RECOVERY-KIT.md. This does not certify
actual production key custody, physical machine/cloud-account loss or production
cutover. An actual owner-operated credential kit was subsequently opened off-server;
see OPERATOR-VALIDATION.md. No production timer has been activated.

Retention update: a project-scoped, read-only `retention-plan` preview is implemented
with calendar buckets and protected snapshots; `retention-save` and explicit manual
`retention-apply` now support reviewed snapshot-record removal with expiry, metadata
binding, preserved kept ID, pre/post checks and audit. See RETENTION.md. Separate
reviewed repository-wide unused-pack prune is implemented with dry-run estimates,
all-snapshot/pack/index binding, fixed no-repack mode and pre/post checks; see
PRUNE.md. Automatic retention/pruning and bounded data repacking remain future work.

Usability/distribution update: TUI history, retention preview, recovery guide and
paging are implemented. CI, Linux package/checksum generation and tag-triggered
draft prerelease workflows are prepared; see RELEASING.md. No stable release or
production activation is implied. One operator has completed key custody, approved
live capture/rehearsal, private webhook tests and manual preflight/monitor service
tests. Capture-service execution and explicit schedule activation remain pending.
Cloud-based whole-server rebuild is the next planned scope: inventory all apps and
host settings, protect deployment definitions/secrets/images, capture each database
consistently and prove recovery onto a replacement environment. It is not implemented
and does not require a second disk in the source server. Data repacking, crash/reboot
certification and broader platform/app compatibility remain future work.
