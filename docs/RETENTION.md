# Retention preview and reviewed snapshot removal (experimental)

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
- Protect the newest snapshot per hostname and its newest nonfuture snapshot.
  Snapshots shared by multiple HomeVault project tags are kept. Policy otherwise
  spans the project's path sets; it is not Restic's default host/path grouping.
- Counts may be 0–10000; all zero still preserves newest/protected snapshots.
  Missing timestamps, invalid IDs and duplicate IDs fail the plan closed.

Output contains `dry_run: true`, `timezone: UTC`, policy, `keep` entries with
reasons, and `candidates`. Candidate means outside this policy, **not safe to
delete**: this does not check application recovery reports or external dependencies.
It does not estimate reclaimable space because encrypted deduplicated snapshots
share data. Snapshot counts do not imply proportional Drive storage savings.

The plain preview is not an executable review file. To prepare a reviewable file:

```sh
homevault --config /absolute/homevault.json retention-save PROJECT /absolute/new-private-review.json --keep-daily 7 --keep-weekly 4 --keep-monthly 12
```

The file must be new; it is written as 0600 and contains the configured repository
fingerprint, creation time, project/policy, full IDs/reasons and a hash of snapshot
metadata. It contains no repository URL, key or file contents. Use private storage
outside backup sources. A plain preview cannot be passed to apply. This document
is not signed: anyone who controls the configuration/key/review can authorize
changes. It protects against stale/mismatched plans, not a compromised account.

## Explicit manual apply

Review every candidate and independently restore/rehearse at least one snapshot
in the keep set. Preserve portable off-server key custody before deletion.

```sh
homevault --config /absolute/homevault.json retention-apply PROJECT /absolute/new-private-review.json --preserve VERIFIED_KEPT_FULL_ID --confirm
```

**This command removes snapshot records.** `--preserve` must be a full ID from
the keep set; it is the owner's acknowledgment, not automatic proof of a restore.
The tool does not run a new application recovery drill or parse recovery reports.
It is never called by the TUI or existing timers.

Apply requires the same configured repository/project, a matching recomputed
policy and exact snapshot metadata set, and a review no older than 24 hours.
At most 64 candidates are accepted per apply; no automatic batching is supplied.
A preflight metadata check must pass, then snapshots are re-read before forgetting
the explicit full IDs. Policy options are not delegated to Restic during deletion.
Protected/latest/shared/other-host snapshots cannot be moved into candidates by
editing the JSON alone. New backups or tag/host/path changes invalidate the review.

The HomeVault operation lock spans all phases. **Do not run other tools/state
directories against this repository concurrently.** Restic uses its own command
locks, but the list/check/forget sequence is not one distributed transaction;
an external client can race the final metadata check. No global lock bypass or
automatic stale-lock removal is used.

Before removal, a private latest-attempt audit is written under
`state_dir/retention/PROJECT.json`. Afterward, all kept IDs and absence of candidates
are checked, followed by another repository metadata check. Audit status is
success/failed with a verification flag; no child output/credentials are stored.
Backup freshness is not advanced. Failure/cancellation may leave some snapshot
records removed: inspect actual state, create a fresh review, and do not assume
rollback or blindly retry. The audit is latest-only, not an immutable history.

## Space reclamation

Apply never calls `prune` or `forget --prune`. Snapshot removal alone does not
remove unreferenced data packs or guarantee reclaimed Drive space. Separate
[reviewed unused-pack prune](PRUNE.md) is implemented; Drive trash handling and
automatic retention are still unimplemented.
See [Restic's removal documentation](https://restic.readthedocs.io/en/stable/060_forget.html).

Unit tests cover project-filtered listing with no mutation commands, occupied
calendar union, ISO year boundaries, duplicate/invalid metadata, pinned/future
snapshots, zero policy and empty repositories. No real Drive data was deleted.
Apply tests cover exact IDs, local locking, stale/edited/wrong-repository reviews,
late metadata changes and failed checks with no forget call, plus private audit
status. An opt-in real Restic fixture removes one generated local snapshot,
preserves another project, restores the retained version byte-for-byte and confirms
data packs were not pruned. No real project or cloud snapshot was removed.
