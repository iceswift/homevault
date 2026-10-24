# Verification status

HomeVault is an early file-backup MVP, not a production application-backup system. Ubuntu/Linux is the initial validated platform.

## Checks performed

- Go unit tests and go vet passed on Windows; Linux configuration, engine and cross-process locking tests also passed.
- Real Restic local initialization, backup, snapshot listing, repository check and byte-identical restore passed with generated fixtures.
- Restore with the original source absent passed. Nonempty restore destinations were refused.
- TUI cancellation was exercised with a controlled long-running child: quitting terminated the child and released the operation lock before returning.
- Google Drive synthetic encrypted backup, repository check and byte-identical restore passed through rclone with drive.file scope, including after OAuth Production reauthorization.
- A simulated network failure returned an error, released the operation lock, and recovered on a normal read.
- A Linux isolated Compose application rehearsal passed after encrypted restore: health, SQLite query, media bytes over HTTP and synthetic login. Generated Compose isolation fields and container cleanup were checked. A failing check/timeout produced a failed report and cleanup. This is a bundled fixture, not evidence of recovery for a real project; PostgreSQL extension results are recorded below.
- The actual DealDoor 84c478b application image also passed a fresh-demo-only encrypted backup/recovery rehearsal: random database canary survived original fixture DB loss; HTTP media bytes matched; login issued a cookie accepted by the dashboard; anonymous dashboard access redirected. Production data/accounts were never used, and production health remained HTTP 200. This validates that image against generated demo data, not the current production dataset. See RECOVERY-REHEARSAL.md for the opt-in test.

The default `go test ./...` run skips real Restic integration unless HOMEVAULT_TEST_RESTIC is provided. These checks are not a sustained performance benchmark or application/database recovery drill. No production data was used.

## Known limits

Windows unit tests pass, but actual restore encountered ACL failures; Windows is not a validated backup/restore target. Experimental database preparation is Linux-only; SQLite WAL export, encrypted recovery after source loss, PostgreSQL 16 dump/restore and export-failure gating passed synthetic tests. See DATABASE-BACKUPS.md for boundaries, dependencies and application consistency limitations. Retention, alerts, durable history and automatic scheduler installation remain unimplemented. Review deploy templates and test restore independently before relying on them.

Personal deployment paths, account-specific evidence, credentials and generated data are intentionally excluded from the public repository.

## Jobs and alerts (prepared, not activated)

Fresh-capture Linux tests additionally verified two changed DB/media versions restored through Restic, committed WAL data, capture-failure resume with unchanged last success, cancellation, source/image pinning, stale-copy refusal and orphaned-guard rescue despite a missing key/stale operation lock. A separate DealDoor live-source check inspected identity/layout/read access without restarting it, writing job state, capturing data or activating a timer. This is not a production fresh-capture run or whole-system reboot/SIGKILL certification. See FRESH-CAPTURE.md.

Ubuntu synthetic tests passed durable job status/history, preserving last success on a failed metadata check, monitoring despite a simulated stale operation lock, notification dedup/recovery and retry after a failed delivery. Real Restic job backup/check and a later missing-source failure passed without advancing last success. Discord HTTP tests used mock transports only; no real webhook was used or message sent. Windows tests passed outside the restricted filesystem sandbox, where atomic rename otherwise failed; Linux remains the deployment target. No production timer or permission change was installed. See SCHEDULING-ALERTS.md.

## PostgreSQL multi-container rehearsal


On Ubuntu, a synthetic PostgreSQL 16 + Node/pg application passed HomeVault preparation, encrypted Restic backup/restore, automatic import into a fresh ephemeral cluster, exact database canary, HTTP health/media, database-backed login/session and both-container cleanup. An invalid SQL archive failed import before application checks and cleaned up correctly. No Saan or production PostgreSQL data was used. See POSTGRES-RECOVERY.md; arbitrary Compose stacks and actual production-project onboarding remain unvalidated.
