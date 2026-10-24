# Starting on a new home server

HomeVault is experimental and Linux is the supported deployment target. Start with
generated data and prove recovery before including an application. The CLI/TUI
does not need a web server or inbound port.

## 1. Install and practice

Build from reviewed source with Go 1.24+, or obtain a Linux archive from a reviewed
release/CI run. Verify `sha256sum -c SHA256SUMS` before extracting/running it.
Checksums detect file corruption; they do not authenticate the publisher. Use the
official repository and review its source/build provenance separately.

Restic is a separate executable, and rclone is needed for a Drive repository.
Review their official installation instructions. HomeVault does not download tools
during ordinary backup. The fixture download script is for Linux amd64 CI/labs only.

Use a new directory with an existing parent:

```sh
homevault demo-setup /absolute/new-lab
homevault --config /absolute/new-lab/homevault.json doctor
homevault --config /absolute/new-lab/homevault.json init
homevault --config /absolute/new-lab/homevault.json job-backup synthetic
homevault --config /absolute/new-lab/homevault.json snapshots synthetic
homevault --config /absolute/new-lab/homevault.json restore synthetic FULL_SNAPSHOT_ID /absolute/new-restore --confirm
```

Compare the restored proof.txt bytes with the source. This exercises a local
encrypted repository with generated data, not your cloud credentials or real app.

## 2. Prepare an actual project configuration

Create/preserve the repository key privately first. If adopting an existing
repository, use its **original** key; never replace it with a newly generated key.
Keep an independently recoverable copy in a password manager/offline location.
Do not paste passwords, tokens, webhook URLs or private JSON into issues/chat/Git.

The `setup` command runs before executable discovery. It creates one new private
configuration directory, a state subdirectory and a checklist. It references the
existing key without reading/printing/copying its contents. It does not initialize
a repository or authorize Google.

```sh
homevault setup /absolute/new-project-config --project photos --source /srv/photos --repository rclone:gdrive:HomeVault/photos --password-file /absolute/private/original-restic-key
```

For a supported single-container SQLite app, select media separately from the DB:

```sh
homevault setup /absolute/new-app-config --project example --source /srv/example/media --sqlite /srv/example/app.db --capture-required --repository rclone:gdrive:HomeVault/example --password-file /absolute/private/original-restic-key
```

All paths must be absolute, source/key must exist, the output directory must be
new, and output must not overlap source/database/key/local repository. No existing
configuration is overwritten. Failed partial output is retained for inspection;
retry in a new directory. Linux permissions are 0700 directories/0600 files;
Windows users must independently protect ACLs.

This is a guided project starter, not an automatic database detector. The separate
[Drive OAuth wizard](DRIVE-OAUTH.md) prepares authorization before project setup;
use --rclone-config to attach its private config. Files-only sources must exclude live databases. Review
[database preparation](DATABASE-BACKUPS.md) for PostgreSQL settings and credentials.
Capture-required projects additionally need a reviewed pinned [capture plan](FRESH-CAPTURE.md)
and approved pause window; setup does not create or activate that plan.

For Drive, configure the intended rclone account/remotes privately first. Prefer a
dedicated folder and appropriate narrow OAuth scope where suitable. HomeVault uses
the configured transport and does not inspect your open browser or select an account.
See [rclone Drive documentation](https://rclone.org/drive/).

## 3. Validate before scheduling

Read the generated ONBOARDING.md. Run doctor, then list snapshots for an existing
repository; use init only for an intentionally new repository. Verify backup/check,
restore files into a new empty target, validate the prepared DB and media, then
perform an isolated [application rehearsal](RECOVERY-REHEARSAL.md).

TUI keys: `h` job history, `s` snapshots, `p` read-only retention preview, `g` recovery
guide, Page Up/Down scroll. `b` now runs job-backup followed by metadata check and
records status; capture-required projects refuse this generic operation. The UI
does not stop an application, restore over production or delete snapshots.

Only after recovery and key custody are verified should you provision private
alerts, review source permissions and manually test [systemd templates](SCHEDULING-ALERTS.md).
Opening the TUI/setup does not enable any timers.

## Rebuilding after losing the server

1. Obtain a reviewed HomeVault build plus Restic/rclone and compatible database tools.
2. Recover the original repository key from off-server custody. Restore transport
   authorization privately or reauthorize the intended Drive account. A key stored
   only in machine/account-bound encryption may not be portable to the new server.
3. Recreate private configuration pointing at the **existing repository**. Create
   a new private state directory. Do not initialize/overwrite the repository.
4. List snapshots and restore a selected complete ID into new storage. Verify DB
   exports and all required media/files. Obtain the app's exact compatible image
   independently; HomeVault snapshots do not automatically contain container images.
5. Rehearse in isolation, validate health/database/media/login, then plan a separate
   production recovery. Recreate service accounts, DNS/proxy and schedules only
   after recovery is verified. No automatic production cutover is supplied.
