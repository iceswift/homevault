# Verification status

HomeVault is an early file-backup MVP, not a production application-backup system. Ubuntu/Linux is the initial validated platform.

## Checks performed

- Go unit tests and go vet passed on Windows; Linux configuration, engine and cross-process locking tests also passed.
- Real Restic local initialization, backup, snapshot listing, repository check and byte-identical restore passed with generated fixtures.
- Restore with the original source absent passed. Nonempty restore destinations were refused.
- TUI cancellation was exercised with a controlled long-running child: quitting terminated the child and released the operation lock before returning.
- Google Drive synthetic encrypted backup, repository check and byte-identical restore passed through rclone with drive.file scope, including after OAuth Production reauthorization.
- A simulated network failure returned an error, released the operation lock, and recovered on a normal read.

The default `go test ./...` run skips real Restic integration unless HOMEVAULT_TEST_RESTIC is provided. These checks are not a sustained performance benchmark or application/database recovery drill. No production data was used.

## Known limits

Windows unit tests pass, but actual restore encountered ACL failures; Windows is not a validated backup/restore target. Database preparation, retention, alerts, durable history and automatic scheduler installation remain unimplemented. Review deploy templates and test restore independently before relying on them.

Personal deployment paths, account-specific evidence, credentials and generated data are intentionally excluded from the public repository.
