# Retention preview (experimental; no deletion)

```sh
homevault --config /absolute/homevault.json retention-plan PROJECT --keep-daily 7 --keep-weekly 4 --keep-monthly 12
```

This command lists only the selected project's snapshots and prints a JSON plan.
It does not run Restic forget/prune, remove snapshots, empty Drive trash, send
notifications, stop an app or enable a timer. The repository client may use its
ordinary local cache/locks while listing. No application data is downloaded.

The policy is HomeVault's preview algorithm, not a Restic forget invocation:

- Sort by snapshot time descending; break timestamp ties by full ID ascending.
- Keep the newest snapshot, plus the newest nonfuture snapshot when a clock error
  puts a future snapshot first. Always protect future-dated snapshots.
- Keep the newest snapshot in each of the last N **occupied** UTC days,
  ISO weeks (Monday start, ISO week-year) and UTC months. Empty periods do not
  consume slots. These are calendar groups, not rolling 24-hour durations.
- Combine these reasons as a union. A snapshot tagged exactly `homevault:keep`
  is also protected. Tags must be maintained externally; this command adds none.
- Counts may be 0–10000; all zero still preserves newest/protected snapshots.
  Missing timestamps, invalid IDs and duplicate IDs fail the plan closed.

Output contains `dry_run: true`, `timezone: UTC`, policy, `keep` entries with
reasons, and `candidates`. Candidate means outside this policy, **not safe to
delete**: this does not check application recovery reports or external dependencies.
It does not estimate reclaimable space because encrypted deduplicated snapshots
share data. Snapshot counts do not imply proportional Drive storage savings.

This is a point-in-time preview, not a signed approval or an executable deletion
manifest. No apply command exists. Review recovery evidence, key custody and
project ownership before designing deletion. Multiple project tags and shared
repositories require additional protections before any future deletion feature.

Unit tests cover project-filtered listing with no mutation commands, occupied
calendar union, ISO year boundaries, duplicate/invalid metadata, pinned/future
snapshots, zero policy and empty repositories. No real Drive data was deleted.
