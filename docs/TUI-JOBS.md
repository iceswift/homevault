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

## Local attempt history

Press `h` from a project list or details to open that project's native engine
history. Newest recorded attempts appear first; the current attempt is not repeated
when it also appears in the completed history. The engine retains up to 32 completed
attempts; a running record can add one more entry. This is not a cloud snapshot list
or a complete audit log.

Each entry shows its attempt ID, start/finish timestamps, completed duration and
separate backup/check results. `not recorded passed` does not assert that a step ran
and failed: it might not have been reached. A running record does not prove that its
process is alive. Cancellation is shown separately from failure at a backup/check
phase. The last recorded successful backup/check remains visible above the list even
if the latest attempt failed. Source freshness and application recovery stay unknown.

- Up/Down or `j`/`k`, Page Up/Page Down: scroll.
- `r`: explicitly reload local records; no cloud lookup or engine job is started.
- Esc or `h`: return to the same project; `q`/Ctrl+C: quit.
- `b` and Enter do not start jobs from history; return to the project first.

Missing/unreadable history is distinguished from a successfully read empty history.
On refresh failure, cached attempts are hidden until a successful refresh, rather
than presented as current evidence. The selected history stays bound to its project
ID across refreshed list reordering. Navigation, scrolling and refresh create no
history files and send no notifications. `tui-status` and `tui-demo` remain separate
and do not expose this manual-job history panel.
