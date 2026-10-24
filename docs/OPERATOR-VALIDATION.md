# Operator validation and recovery scope

Updated 24 October 2026. These are observations from one owner-operated Linux
deployment, not automated acceptance tests or a general compatibility guarantee.
Personal paths, account identifiers, credentials, snapshot IDs and deployment
scripts are intentionally excluded from this repository.

## Completed on one SQLite application

- Exported an encrypted recovery kit containing the original repository key and
  dedicated Drive credentials. Kept the age identity separately on an independent
  Windows PC, opened the kit there, compared the recovered repository key with the
  existing protected copy, and removed temporary decrypted output. Both custody
  folders are on the same PC; an additional independent copy is still advisable.
- Captured current SQLite DB/WAL and media from a reviewed single-container app
  during an owner-approved bounded pause. The app resumed healthy before cloud
  upload. Used the existing repository and key; metadata check passed.
- Restored that snapshot from Google Drive and checked SQLite integrity. A separate
  isolated application rehearsal passed health, database, media/HTTP and demo-login
  checks, then removed its test container. Fingerprints compared the first restore
  with the rehearsal restore; they were not independent pre-upload source hashes.
- Sent real Discord messages labelled as simulated tests through HomeVault's
  notifier/monitor. Discord accepted test, success, failure and recovery events.
  Failure/success deduplication and quiet routine-success policy passed. HTTP
  acceptance does not establish visual confirmation of the intended channel.
  With routine success enabled, a healthy poll following recovery can emit one
  success notification before deduplication; quiet success was also tested.
- Manually installed three systemd oneshot units with root-owned executables and
  configuration. Source-preflight and freshness-monitor units completed under
  systemd with exit 0. The capture unit was installed but not started: the actual
  capture above ran through the CLI. No timer was installed or enabled.

These operator-specific units use an existing Docker-authorized account. This
does not make Docker access an isolation boundary. Generic templates in `deploy/`
remain examples requiring a separate review of paths, permissions and sandboxing.
No reboot/power-loss test or capture-under-systemd test has been completed.

## What this backup does not cover

Initial cloud validation covered one application's database and media; selected
coverage was later expanded as recorded below. It is not a bootable disk image or
a complete server backup. It does not automatically
include every application, Compose files, environment/secrets, locally built
images, operating-system configuration, proxy/tunnel settings or other services.
The credential kit unlocks repository data; it does not supply missing application
images or reconstruct the host by itself.

## Next scope: rebuild a failed server from cloud backup

The intended destination is encrypted Google Drive storage, without requiring a
second backup disk in the source server. This direction is planned, not implemented:

1. Inventory all applications, persistent mounts, databases, host configuration
   and dependencies. Review explicit inclusion/exclusion rules with the owner.
2. Add consistent database/file capture for each application; do not blindly copy
   running database directories or assume the single-container SQLite adapter
   supports multi-container PostgreSQL stacks.
3. Preserve deployment definitions, required environment/secrets, pinned image
   references and locally built images that cannot be retrieved elsewhere.
4. Record the package/runtime, users/permissions, filesystem layout and host-service
   configuration needed to rebuild onto a fresh Ubuntu installation.
5. Provide a reviewed recovery checklist/bootstrap flow with off-server keys and
   Google account recovery requirements. Reauthentication may still be necessary.
6. Prove recovery on an isolated replacement environment before claiming full-server
   recovery. A replacement machine still needs working local storage and networking.

Scheduling remains a separate explicit owner decision. No real Drive retention
deletion or prune has been performed as part of these operator checks.

## Incremental server-recovery coverage

A subsequent read-only inventory found multiple applications with bind-mounted
data and host-service dependencies. This is a baseline, not an exhaustive
privileged audit. Curated deployment/static files were subsequently staged,
uploaded to the existing encrypted repository and separately restored; all
pre-upload manifest hashes matched. See RECOVERY-FILES.md for the staging helper.

After explicit owner authorization, one application's existing scheduled
PostgreSQL dump/uploads bundle and required deployment configuration/secrets were
also uploaded encrypted. Completion marker, source checksums, archive member
safety, dump readability and current image/config compatibility were checked;
the separate cloud restore matched staged hashes. The recovery point is the
original scheduled capture time, not upload time. No new application pause was
required. This check did not import the restored database or run the restored app.

Selected root-owned host/application configuration was prepared privately by the
owner. Following separate explicit authorization for that secrets payload, its
encrypted cloud backup and independently restored manifest hashes passed.

A separate JSON-state application was captured during an explicitly authorized
brief stop of its pinned container, with an independent resume guardian. Both
active state paths were checked against the captured mount without displaying
environment values. The app returned healthy before upload after approximately
7.3 seconds. Both restored state files and the manifest matched their pre-upload
hashes; unrelated application health checks passed. This was not an application
restore rehearsal.

Completion of privileged inventory gaps, complete host reconstruction and a full
replacement-machine drill remain pending. These selected backups do not cover
every host file or make a bootable image. No new cloud schedule was enabled.

## Recovery bundle preparation

One operator exported seven pinned runtime image references used by eight running
containers. The OCI archive's top-level image indexes and 110 included blob hashes
were verified; the archive and reviewed recovery tools were backed up encrypted
and separately restored with matching pre-upload file hashes. No image was loaded
into Docker during this check. Container identities/start times were unchanged
through the successful validation/transfer run; subsequent health checks passed.

A catalog maps six selections to exact project tags, full snapshot IDs and nested
restore paths. A bootstrap archive includes Linux amd64 tools, the catalog,
metadata-only host inventory and the manual REBUILD.md runbook. It does not install
Ubuntu/Docker, promote secrets, start applications or configure ingress.

The bootstrap, final catalog and a newly encrypted credential kit were copied to
the operator's independent Windows PC. Checksums and owner/SYSTEM-only ACLs passed.
The kit opened there, its original repository key matched the independently
protected copy, and the catalog generated six recovery-only configurations.
Temporary decrypted verification files were removed. The private age identity
was not sent to the source server. Kit and identity remain on the same independent
PC in separate private folders; an additional independent custody copy is not
established by this test.

This preparation step proved selected archive bytes and off-server bootstrap
custody; it did not itself load/run images. A subsequent isolated Ubuntu VM drill
loaded all seven image references and exercised SQLite, PostgreSQL, JSON-state
and static-site recovery with restart checks. See VM-RECOVERY-DRILL.md for precise
scope, preserved failed attempts, cleanup and remaining host/hardware/cutover gaps.
No physical disk-loss test or production cutover has been performed.
