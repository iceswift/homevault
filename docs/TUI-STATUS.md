# Read-only local status TUI

`tui-status` connects the new layout to **local metadata**, separately from the
synthetic `tui-demo` and the existing operational `tui`. Available on main and
subsequent CI builds; not in the published `v0.1.0-preview.1`.

## Open an explicitly selected plan

Build current main using the README's source instructions, then use your existing
reviewed coordinator plan:

```sh
./bin/homevault tui-status --plan /absolute/reviewed-workflows.json
./bin/homevault tui-status --plan /absolute/reviewed-workflows.json --context production --timezone Asia/Bangkok
./bin/homevault tui-status --plan /absolute/reviewed-workflows.json --render --width 120 --height 32 --ascii
```

The plan supplies `base_config`, coordinator root, workflow names and activation
flags. The global `--config` flag is not used by this mode. No plan is discovered
or installed automatically. Initial load failures stop with a redacted error;
there is no fallback to sample data.

No Restic, rclone, Python, Docker or systemd executable is required by this command.
The reader is native Go. For application status it reads only the selected JSON
plan, base-configuration metadata and local coordinator/legacy job records. It does not open repository-key,
OAuth/rclone-credential or webhook file contents, inspect app files, list cloud
snapshots, query running processes or run backup/monitor/preflight commands.

## Controls and refresh

Use the same Overview/Projects/detail layouts as the prototype: Tab or `1`/`2`,
arrows, Enter, `/` filter, Esc back/clear, Page Up/Down and `?` help. `q` or Ctrl+C
quits. ASCII and monochrome modes use `--ascii` / `--no-color`; NO_COLOR and
TERM=dumb are honored. `--project ID`, `--page projects` and `--details` select an
initial view. The default display timezone is Local; use UTC or an installed IANA
zone explicitly if preferred.

**`r` reloads local files manually.** There is no periodic polling or daemon.
Reads run outside the input/update loop; overlapping refresh requests are ignored.
The last-read timestamp is shown persistently. Ages are calculated at that read,
not a continuously updated heartbeat.

While refreshing, old values are labelled cached/Unknown. If refresh fails, no raw
error payload is displayed, the last successful read time is preserved and all
cached rows remain Unknown until a successful reload. Selection is preserved by
workflow ID across reordering; the search context remains in place.

Backup, restore, checks, deletion, scheduling and credential actions are unavailable
in this mode. Their keys explain the read-only boundary. No file/lock creation,
state update, notification, service restart, app pause or timer activation occurs.

## Separate workflow evidence

The Overview and Projects screens show every workflow from the selected plan in
one view. Backup/check, source freshness, application recovery and the plan gate
are separate fields. Disabled gates do not erase earlier successful backup records.
No coordinator verification point means unknown source freshness, even when a
validated legacy backup/check record exists.

Source freshness describes the recorded capture time against that workflow's age
limit **at the last local read**. It is independent of the gate and latest attempt:
a recorded upload may have passed while its source capture is stale. The latest
backup/check attempt can be passed, failed, running, cancelled (legacy), skipped
(coordinator), missing from this metadata set or unknown. Last successful upload
and its timestamp are shown separately in details. Recovery always says
`Not inspected`; rehearsal receipts are not read by this view.

Use `--context production` for an explicitly selected real-data plan or `--context
lab` for a test plan. The header says `(operator label)`; the default is `CONTEXT
UNKNOWN`. No plan is selected based on the label and it does not grant permissions.
All action keys remain unavailable. There is no Jobs capability, readiness engine,
credential-content lookup or tool discovery in this view.

## Coordinator assessment (full details)

| State | Interpretation |
| --- | --- |
| Passed | A local coordinator record matches this plan/repository reference, has stored capture/upload/check proof and satisfies both age limits; not a live cloud or recovery check |
| Stale source | Captured data or verified upload is older than the plan's freshness limit |
| Failed | A failure is recorded, or a recorded run exceeds the coordinator's 2h monitoring limit; no process was inspected |
| Running record | Stored running attempt; actual process state is unverified |
| Disabled in plan | The workflow's activation gate is false; earlier job history may still exist |
| Unverified | No coordinator verification point is available; legacy context does not establish source freshness |
| Unknown | Metadata is unreadable, inconsistent, unsupported or from a failed/cached refresh |

An enabled plan gate does not prove that a timer is installed/enabled. Schedule
always says **Unknown - systemd not inspected**. Recovery evidence similarly says
**Unknown - not inspected**, not "untested" or "passed". Notification settings in
help describe the plan flag only; the view never sends a message.

## Coordinator versus legacy scope

Coordinator records couple a verified upload with its source capture time. The
reader validates version, policy/repository binding, timestamp consistency and
stored success proof before deriving freshness. A failed attempt preserves the
previous verified point; a new upload of old data cannot become fresh merely by
having a recent upload timestamp.

When a coordinator record is absent, the view may show a version/identity-checked
legacy Go job record as **Legacy engine job only**. Its recorded backup/check and
upload time are context; source age remains Unknown. Legacy context never makes
a workflow Passed. A missing record means **no record in this metadata set**, not
that cloud backups are absent: earlier plain backups, catalogs or other state
roots may have independent recovery points.

The view does not read rehearsal receipts or the repository itself. A green local
record is not a new integrity check, cloud reachability check or recovery proof.

## Supported metadata and failure handling

The reader supports coordinator plan/state version 1 and legacy Go job-state
version 1 for the four existing workflow kinds. Freshness policies must use whole
hours (1-720). Unknown versions/policies are refused rather than guessed.
Workflow binding matches the coordinator's Python JSON canonicalization for this
integer-only policy schema, with Unicode/nested test vectors.

Input is bounded to 1 MiB per file, with structural limits and duplicate-key
rejection. Symlinks, special files, unsafe names and identity/binding mismatches
are not trusted. Plan/base semantics are checked again after the read to reject
mixed policy generations. Per-project records are independent local observations,
not a simultaneous whole-server checkpoint. Binding checks are consistency checks,
not cryptographic authentication against a malicious local writer.

Diagnostics use fixed codes without dumping paths, JSON, credentials or arbitrary
exception text. Fix/read the metadata independently when needed; the UI does not
repair files, remove locks or change permissions.

## Validation

Generated fixtures cover stale sources despite recent upload, failed attempts
preserving older success, legacy-source uncertainty, incorrect identities,
malformed/oversized/duplicate JSON, unsafe links, policy changes during reads,
refresh failures, timezone display and terminal bounds. UI rendering/refresh and
CLI routing work without backup tools or readable credential files.

One operator also opened the native view against existing metadata, exercised a
real PTY refresh/quit, and compared file hashes/modes/mtimes and directory entries
before/after. Metadata was unchanged; workflow/notification gates stayed disabled.
No actual repository, app health, systemd schedule or recovery drill was checked
in that observation. Do not use it as evidence that a backup can currently restore.
