# Frequently asked questions

HomeVault is an experimental Linux backup/recovery orchestrator. Start with
[installation](RELEASING.md) and the [generated-data demo](QUICKSTART.md).

## Does HomeVault back up my whole server?

It backs up explicitly selected files and prepared application data. It does not
make a bootable disk image, discover every volume or recreate your operating
system, users, network routes and services automatically. Include deployment
files, required secrets and compatible application images deliberately if you
need to rebuild an application later.

Selected applications have been recovered in an isolated VM, but that VM ran on
the original physical server. This is not a complete replacement-hardware or
production-cutover test. See [rebuild scope](REBUILD.md) and [VM evidence](VM-RECOVERY-DRILL.md).

## Do I need a second disk in the server?

Not for cloud backups. Restic can store the encrypted repository on Google Drive
through rclone. You still need local working space for caches, database exports,
coordinated capture stages and any restore you perform. Required space depends on
the selected data and workflow; the cloud destination does not remove this need.

The local demo deliberately stores source and repository on the same machine.
It teaches the workflow, but does not protect against losing that machine/disk.

## Is Google Drive required?

No. The local demo needs neither a Google account nor rclone. For Drive, install
rclone separately and authorize the intended account through the reviewed
[Drive setup](DRIVE-OAUTH.md). Restic encrypts repository contents before upload;
HomeVault does not turn a normal Drive folder into a browsable copy of your files.
Use the restore command to recover the selected snapshot's original data.

Opening a Drive browser tab does not configure HomeVault or select its account.
Cloud storage quota and provider account access remain your responsibility.

## What does HomeVault add to Restic and rclone?

Restic supplies encryption, snapshots, deduplication and repository checks. rclone
supplies cloud transport. HomeVault coordinates those tools with project config,
a terminal UI, job history, reviewed capture workflows and recovery guidance.

If your Restic/rclone scripts already meet your needs, you may not need HomeVault.
It is not a replacement encryption engine, and there is no comparative evidence
that it is faster or more reliable than established tools.

## Do I need Go, Docker or sudo to try it?

A prebuilt Linux package does not require Go. The generated-data demo needs
Restic, Bash and Python 3.11+; it needs neither Docker nor sudo after dependencies
are installed. Building from source also requires Git and Go 1.24.5+.

Real application capture/rehearsals may need Docker/database tools and reviewed
permissions. Optional systemd installation is a separate administrator operation.
See the [installation guide](RELEASING.md).

## Which platforms are validated?

Ubuntu/Linux amd64 is the initial runtime target. CI executes an extracted amd64
package and completes generated-data backup/check/restore. ARM64 packages are
cross-built and structurally checked, but not runtime-tested. Windows unit builds
are separate from deployment support; Windows restore/deployment is not validated.
See [testing](TESTING.md) for exact evidence and skipped checks.

## Can I back up a running database like an ordinary folder?

Do not treat a file copy of live SQLite/PostgreSQL storage as a consistent backup.
Use the supported [database preparation](DATABASE-BACKUPS.md) or a reviewed
application capture plan. A correct database export alone does not make the DB
and associated uploads/media a simultaneous application snapshot.

The pinned [SQLite capture workflow](FRESH-CAPTURE.md) can use an explicitly
approved bounded application pause. Prepared workflows can also use a verified
completed local export bundle. Both need application-specific review.

## Will opening the TUI stop my application or start scheduled backups?

No. Opening the UI does not activate timers, delete snapshots or stop applications.
Pressing `b` opens a backup confirmation; it is not a scheduler. Capture-required
projects use their reviewed capture workflow instead of that generic operation.

An explicitly invoked capture workflow may pause its pinned application when its
activation and maintenance-window gates permit it. Review that plan before use.
See [prepared workflows](PLANNED-WORKFLOWS.md) and [jobs/alerts](SCHEDULING-ALERTS.md).

## Does a successful backup or check mean my application will recover?

It establishes the outcome of that operation, not end-to-end application recovery.
Restore into a separate target, validate file bytes and database contents, and run
appropriate application health/media/login checks in an isolated environment.

Job success, source freshness and application recovery evidence are distinct.
See [recovery rehearsal](RECOVERY-REHEARSAL.md); do not restore over production to
find out whether a backup works.

## What if the server fails, or I lose the repository key?

To recover after server loss, you need the original repository key, the selected
backup data and working transport/account access. Applications may also require
compatible images, configuration and secrets. Prepare these before a failure.

If every recoverable copy of the repository key is lost, HomeVault cannot decrypt
the repository for you. Keep an independently recoverable copy outside the server.
An optional [encrypted recovery kit](RECOVERY-KIT.md) can package selected original
credentials, but its decryption identity must also be independently available.
Creating a kit is not proof: open it and test a restore separately.

## Can HomeVault automatically delete backups after a number of days?

Automatic age-based deletion is not implemented. Current [retention tools](RETENTION.md)
preview calendar daily/weekly/monthly selections and support separately reviewed,
explicit manual snapshot removal. This is not a rolling-day expiry scheduler.
[Prune](PRUNE.md) is a separate reviewed operation that reclaims unused repository
packs; removing a snapshot does not imply proportional immediate space savings.

Do not delete encrypted repository objects manually in Drive. Configure and test
retention through the supported repository tools, preserving required recovery
points. No retention policy is enabled just by downloading HomeVault.

## Is HomeVault a hosted backup service?

No. You run the CLI/TUI on your own machine/server, with your chosen storage and
credentials. A public HomeVault web service is not required. The optional desktop
OAuth callback is temporary and loopback-only. There is no HomeVault-operated
storage service or managed account recovery.

## Where should I ask for help or report a problem?

Use the [issue forms](https://github.com/iceswift/homevault/issues/new/choose) for
ordinary bugs, feature requests and usage questions. Include the version,
OS/architecture and minimal redacted steps using generated data.

Use [private vulnerability reporting](../SECURITY.md) for security-sensitive bugs.
Never attach keys, OAuth JSON/tokens, webhook URLs, .env files, database dumps or
real backup archives, even when requesting private help. See [contributing](../CONTRIBUTING.md).
