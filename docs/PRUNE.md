# Reviewed unused-pack cleanup (experimental)

Prune is repository-wide: it considers **all** retained snapshots, including other
projects and untagged snapshots. It does not choose/delete snapshots. First review
[retention](RETENTION.md) if removing old snapshot records is intended.

## Preview and save

```sh
homevault --config /absolute/homevault.json prune-plan
homevault --config /absolute/homevault.json prune-save /absolute/new-private-prune.json
```

Both run a repository metadata check and Restic dry-run while holding the HomeVault
operation lock. No destructive prune runs. A preview can be expensive: Restic must
inspect snapshot trees/indexes and remote objects; this is not an instant quota query.
Local cache and transient repository lock files may be used normally.

The saved private 0600 JSON review includes the configured repository fingerprint,
UTC creation time, all full retained snapshot IDs and metadata hash, pack/index
inventory hash, fixed mode/version and estimated blob counts/approximate human sizes.
It contains no repository URL, key, source file contents or raw child output. Save
outside backup sources in a private location. Existing files are never overwritten.
It is not signed; anyone controlling the repository credentials/config/review can
authorize changes. The review is a consistency guard, not proof against a compromised
account. Unix mode checks do not validate Windows ACLs.

## Explicit apply

Before use, independently verify recovery of a retained snapshot and preserve the
original repository key off-server. A metadata check does not prove app recovery.

```sh
homevault --config /absolute/homevault.json prune-apply /absolute/new-private-prune.json --repository-wide --preserve VERIFIED_FULL_ID --confirm
```

`--repository-wide` and `--confirm` acknowledge the scope/destructive operation.
`--preserve` must identify a snapshot in the unchanged repository; it is the owner's
acknowledgment of recovery testing, not an automatic restore test. Apply:

1. Refuses wrong repository/mode/version, future/older-than-24-hour reviews, empty
   repositories, changed snapshots, and changed pack/index inventories.
2. Writes a private latest-attempt audit under `state_dir/prune/latest.json`.
3. Checks repository metadata, repeats dry-run and compares the estimate. It then
   rechecks snapshot/inventory identity and review age immediately before prune.
4. Runs only `prune --max-repack-size=0 --max-unused=unlimited`.
5. Checks repository metadata again and verifies **all** snapshot metadata remains
   unchanged. Success means these checks passed, not a complete application drill.

No `forget`, unsafe recovery, no-lock bypass, automatic retries, timer, notification,
Drive trash cleanup or production cutover is included. Backup freshness is unchanged.
An interrupted/failed operation records failure and may have partially cleaned data;
inspect/check the repository and create a fresh plan before further maintenance.
The audit stores only the latest attempt; it is not immutable event history.

## Resource and compatibility boundaries

The first mode removes fully unreferenced packs without repacking blobs. Partly used
packs may retain unused space; this deliberately avoids downloading/reuploading used
data. Restic still scans metadata, can rewrite indexes, and holds its own exclusive
prune lock, so budget CPU/memory/network/time and run outside backup hours. No exact
resource ceiling or transfer-byte guarantee is enforced. Context timeout/cancellation
uses the normal child-process cleanup path.

Only tested Restic **0.19.1** is accepted. Its prune output is text, not stable JSON;
the parser accepts the tested dry-run summary and refuses unknown/incomplete/duplicate
fields or any repacking estimate. Future versions require explicit compatibility
testing. Estimates are rounded Restic summaries, not guaranteed reclaimed bytes or
an exact deletion manifest. Pack/index command output is bounded to 8 MiB per list;
larger inventories fail closed rather than silently truncate.

The local HomeVault lock spans each operation, but separate Restic commands are not
one distributed transaction. Do not use other clients/state directories against
the repository concurrently. A final external-writer race cannot be eliminated by
these checks. Remote eventual consistency/backup-store behavior is not certified.

For Google Drive, rclone normally moves deleted objects to trash. HomeVault does not
change `use_trash`, permanently delete files, empty trash or promise an immediate
quota reduction. Review that separately; no real Drive prune has been run here.

Official references: [Restic cleanup/options](https://restic.readthedocs.io/en/stable/060_forget.html),
[Restic stable scripting limits](https://restic.readthedocs.io/en/stable/075_scripting.html),
[rclone Drive deletion behavior](https://rclone.org/drive/#deleting-files).

## Verification

Unit tests cover repository-wide snapshot scope, local locking, private review
round-trip, audit, stale/mismatched/changed reviews, late inventory/snapshot changes,
failed checks, unsupported versions/summary/repacking output and failed prune status.
An opt-in real Restic fixture creates a temporary local repository, forgets only a
generated obsolete snapshot, previews without changing data packs, prunes unused
packs, measures reduced local data bytes, restores **both** retained projects exactly
and rejects reuse of the old storage review. No cloud/production data is used.
