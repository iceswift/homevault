# Reviewed manual file jobs

Available on main; not included in `v0.1.0-preview.1`.

```sh
homevault --config /absolute/reviewed-files.json tui-jobs
```

This separate mode invokes native manual file jobs. It does not load coordinator
workflow enabled flags or maintenance windows. Use only a reviewed file-only
configuration whose source paths exclude live database storage. Database,
capture-required and recovery-only projects are blocked. An initialized repository
and the configured Restic executable are required; this screen cannot initialize it.

1. Select a project and press `b` to review its ID, source count, destination,
   configuration fingerprint and notification policy. Inspect the configuration
   separately for the complete source list.
2. Type the exact project ID, then Enter. Enter alone does nothing. Esc discards
   the review without starting a job. Configuration and previous job identity are
   checked again under the existing engine operation lock; stale reviews fail.
3. Watch recorded backup/check phases. No percentage is estimated. Repeated Enter
   does not queue another job. A successful backup plus metadata check is not proof
   of application recovery or source freshness.
4. Press `x` to cancel and stay, or `q`/Ctrl+C to cancel and wait for worker cleanup
   before exiting. Inspect local job status before retrying. Hard kills and power
   loss may require separate lock investigation; no automatic unlock occurs.

Configured Discord notifications may follow a confirmed job. Opening the screen,
reviewing, refreshing or rendering a frame does not start backup or send alerts.
No app pause, restore, snapshot deletion, prune, workflow activation or timer action
is provided. Cancellation does not promise rollback of an already written snapshot.

The global `--timeout` applies separately to each confirmed job (default `2h`):

```sh
homevault --config /absolute/reviewed-files.json --timeout 30m tui-jobs --ascii --no-color
```

Use `--timezone Asia/Bangkok` for local display. `--render --width 120 --height 32`
prints a frame without starting a job. Terminals smaller than 40 columns or 18 rows
must be enlarged before confirmation. Reviews can scroll; confirmation stays in
the footer. `r` refreshes local records only. Source freshness and recovery remain
unknown in this mode. For strictly read-only coordinator records use
[`tui-status`](TUI-STATUS.md); for synthetic design previews use
[`tui-demo`](TUI-PROTOTYPE.md).

Verification covers typed confirmation, blocked scopes, one-use reviews, stale
configuration/job refusal, cancellation cleanup, redacted completion warnings and
rendering without state creation. Generated-data Linux terminal testing is separate
from production cloud backups and application recovery.
