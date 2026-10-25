# Verification status

HomeVault is an experimental backup/recovery orchestrator, not a certified production application-backup system. Ubuntu/Linux is the initial validated platform.

## Checks performed

- Portable-kit drills used generated credentials and official pinned age 1.3.2. Source/state/original key removal and repository relocation still allowed byte-identical recovery with a separately kept identity. A fresh network-none container also passed SQLite/media/app health/login checks with no original-machine mounts. Read-only mutation/wrong-key/tamper/overwrite guards passed. No actual user key was exported or real service stopped. See RECOVERY-KIT.md.

- Experimental Drive wizard tests use loopback callbacks and mocked Google token exchange: S256 PKCE, state/issuer/host/replay checks, consent failures, cancellation, redaction and no overwrite. Config/source/restore isolation and clearing ambient rclone account overrides are tested. No real Google account was authorized or refreshed during these tests; full live consent remains separate verification. See DRIVE-OAUTH.md.

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

Reviewed prune tests use Restic 0.19.1, full-repository snapshot/pack/index binding,
fixed no-repack flags, private audit, dry-run summary parsing and pre/post metadata
checks. Unit cases refuse stale/changed/unsupported/repacking plans and cover
cancellation/post-check failure without false verification. On Ubuntu, generated
local data-pack bytes decreased after apply and both retained projects restored
byte-identically. Dry-run itself did not alter data packs. See PRUNE.md. No real
Drive/production prune or quota-reclamation claim is made.

Reviewed retention tests cover explicit confirmation, exact forget IDs, protected
hosts/shared project tags, stale/edited/mismatched reviews, preflight failure,
late metadata changes, failure audit and unchanged backup freshness. On Ubuntu,
the real Restic local fixture removed only a generated candidate, preserved an
unrelated project and retained data packs, then restored the kept version exactly.
No production/cloud snapshots were removed. Reviewed unused-pack pruning is tested separately above.

Windows unit tests pass, but actual restore encountered ACL failures; Windows is not a validated backup/restore target. Experimental database preparation is Linux-only; SQLite WAL export, encrypted recovery after source loss, PostgreSQL 16 dump/restore and export-failure gating passed synthetic tests. See DATABASE-BACKUPS.md for boundaries, dependencies and application consistency limitations. Retention preview/reviewed snapshot removal, reviewed unused-pack pruning, optional alerts and durable history are implemented; automatic scheduler installation and data repacking remain unimplemented. Review deploy templates and test restore independently before relying on them.

Personal deployment paths, account-specific evidence, credentials and generated data are intentionally excluded from the public repository.

## Jobs and alerts (prepared, not activated)

Fresh-capture Linux tests additionally verified two changed DB/media versions restored through Restic, committed WAL data, capture-failure resume with unchanged last success, cancellation, source/image pinning, stale-copy refusal and orphaned-guard rescue despite a missing key/stale operation lock. A separate DealDoor live-source check inspected identity/layout/read access without restarting it, writing job state, capturing data or activating a timer. This is not a production fresh-capture run or whole-system reboot/SIGKILL certification. See FRESH-CAPTURE.md.

Ubuntu synthetic tests passed durable job status/history, preserving last success on a failed metadata check, monitoring despite a simulated stale operation lock, notification dedup/recovery and retry after a failed delivery. Real Restic job backup/check and a later missing-source failure passed without advancing last success. Discord HTTP tests used mock transports only; no real webhook was used or message sent. Windows tests passed outside the restricted filesystem sandbox, where atomic rename otherwise failed; Linux remains the deployment target. No production timer or permission change was installed. See SCHEDULING-ALERTS.md.

## PostgreSQL multi-container rehearsal


On Ubuntu, a synthetic PostgreSQL 16 + Node/pg application passed HomeVault preparation, encrypted Restic backup/restore, automatic import into a fresh ephemeral cluster, exact database canary, HTTP health/media, database-backed login/session and both-container cleanup. An invalid SQL archive failed import before application checks and cleaned up correctly. No Saan or production PostgreSQL data was used. See POSTGRES-RECOVERY.md; arbitrary Compose stacks and actual production-project onboarding remain unvalidated.

## Onboarding, TUI and distribution preparation

The guarded setup command is tested for existing-key reuse, refusal to overwrite,
and validation before output creation. TUI tests cover paging/resize bounds,
history presentation without repository fingerprints, and the recovery guide.
Linux amd64/arm64 archives were cross-built and SHA256SUMS verified locally.
The amd64 package passed an Ubuntu synthetic CLI smoke test: version, setup,
doctor, encrypted job backup, status, retention preview and byte-identical restore.
This did not stop any production app or access Drive/Discord. Arm64 runtime has
not been tested. CI/draft-release workflows require their own hosted-run evidence;
local packaging alone does not prove a GitHub workflow run or published release.

## Independent first-user validation

The [first-user trial](FIRST-USER-TRIAL.md) and dedicated issue form are ready.
Independent participant evidence remains pending. CI package/restore checks and
operator terminal sessions are automated/maintainer rehearsals, not independent
usability studies. Record the exact version, restore result, help needed and
retest outcome before claiming successful first-user onboarding.

## Packaged redesigned TUI checks

Current CI and draft-release verification use `verify-release.py --smoke --tui-smoke`.
After the extracted amd64 binary passes the generated-data backup/check/byte-restore
walkthrough, its actual `tui-demo`, `tui-status` and `tui-jobs` render commands run
with an empty PATH. All metadata is generated locally; tool-less manual jobs stay
blocked. Fixture hashes, mode bits, modification times and tree membership are
checked for changes, and no repository may be initialized by the views.

This covers packaged entry points and display-only boundaries. It does not replace
PTY navigation/cancellation checks, a new-user trial, real cloud verification,
application consistency or ARM64 runtime validation. `--tui-smoke` is opt-in so the
older published Preview can still be verified with the original `--smoke` command.
