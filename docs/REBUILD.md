# Replacement-machine recovery (experimental, manual gates)

This runbook prepares recovery on a **new isolated x86_64 Linux machine**.
Do not execute it on the original server. The catalog describes selected
application recovery points, not a bootable disk image or a simultaneous host
checkpoint. A full replacement-machine drill is still required.

One operator completed the selected-application [isolated VM drill](VM-RECOVERY-DRILL.md).
That evidence does not cover every host service or authorize production cutover.

## 1. Independent material before touching the replacement

Obtain the reviewed bootstrap tools, encrypted credential kit, age private
identity, catalog and their independently recorded checksums. Keep the identity
off the source server. The kit contains repository/Drive credentials; the backup
contains deployment secrets. Treat both as private. Google/Tailscale/Cloudflare
account access and OAuth reauthentication may still be necessary.

Install a fresh supported Ubuntu installation and Docker/Compose using the
distribution/operator's reviewed instructions. Use a fresh isolated network;
do not attach production hostnames, tunnels or UPS actions. Create a private
0700 recovery directory on a disk with enough room for restored images and data.
Choose new partitions deliberately; never replay old UUIDs or an LVM layout
blindly. Record the actual new user UID/GID.

Bootstrap binaries are Linux amd64 copies of tools used for this backup. Verify
their checksums against the off-server receipt before use. Set executable bits
on the reviewed copies only. Python 3 standard library is required for catalog
selection; the OS, Docker daemon and Compose are not supplied by this bundle.

## 2. Unlock and select snapshots

Use absolute paths appropriate to the replacement; examples below assume
`/recovery` exists, is private and is on the replacement disk.

```sh
/recovery/tools/homevault kit-open /recovery/server-recovery.age /recovery/opened \
  --identity-file /offline/identity.agekey \
  --age-binary /recovery/tools/age \
  --restic-binary /recovery/tools/restic \
  --rclone-binary /recovery/tools/rclone --confirm
python3 /recovery/recovery-catalog.py /recovery/catalog.json \
  /recovery/opened/homevault.json /recovery/selections
```

The helper creates recovery-only configs referencing the opened kit credentials.
It does not install/start/load/restore anything. Confirm catalog repository matches
the opened kit, project names and full snapshot IDs before proceeding. The final
off-server catalog includes the images snapshot; its copy inside that snapshot
cannot refer to its own snapshot ID and intentionally lists earlier selections.

For each selection, read `selection.json` and use its exact full ID:

```sh
/recovery/tools/homevault --config /recovery/selections/PROJECT/homevault.json \
  check
/recovery/tools/homevault --config /recovery/selections/PROJECT/homevault.json \
  restore PROJECT FULL_SNAPSHOT_ID /recovery/restored-PROJECT --confirm
```

Original absolute source paths are nested under each new restore target. Follow
`snapshot_paths` in the catalog to locate them. Compare all manifest hashes before
using files; do not promote files if any mismatch. Never restore onto `/`, an
existing app data directory or production DB.

Locate the backup manifest at the exact catalog-selected snapshot root. A PWA
may also contain `manifest.json`; a filename search alone is ambiguous.

## 3. Images, definitions and data

Verify the restored image archive's SHA-256 and archive manifest against the
catalog. Only on the isolated replacement:

The OCI verifier checks selected top-level image pins and every included blob's
SHA-256 without loading images. It does not prove complete multi-platform graph
availability or runtime compatibility. Use the Linux amd64 platform used by the
source and record the actual load result during the replacement drill.

```sh
python3 /recovery/verify-image-archive.py \
  /recovery/REVIEWED/docker-images.tar.gz /recovery/catalog.json
gzip -dc /recovery/REVIEWED/docker-images.tar.gz | docker load
docker image inspect REVIEWED_IMAGE_TAG --format '{{.Id}}'
```

Compare **each** loaded ID with the catalog. Archive hash/manifest verification on
the source does not prove Docker can load/run the images; record this separately.
Review image platform and local disk/RAM before starting anything.

- **SQLite app:** locate prepared DB and captured media from its data snapshot.
  Use a copy in a fresh data directory; run SQLite integrity check. Review the
  original rehearsal plan and expected volume destinations. Preserve DB/media
  pairing, Compose build arguments, demo restrictions and resource limits.
  Make both the DB file and its mounted parent directory accessible to the
  runtime image's numeric UID; changing the file owner alone is insufficient.
- **PostgreSQL app:** use its dump and uploads archive from the same scheduled
  bundle. Check SHA256SUMS and archive paths before extraction. Create a new empty
  DB/data root and separate Compose project/loopback port. Restore only trusted
  SQL archives into that isolated DB. Review the included restore.sh manually.
  Preserve role/bootstrap config; do not copy live PostgreSQL physical storage.
  Wait for a real SQL connection over TCP to the final database before import;
  a temporary init server's Unix-socket readiness is insufficient.
  Remove gateway credentials, disable payment/reconciliation workers, scheduled
  tasks, real SMS/notifications and external mutations before starting the app.
  Start operations health state empty; old success/heartbeat files are not fresh.
- **JSON-state API:** pair captured JSON state with its code/image/config. Restore
  private file permissions and original container destination mapping. Disable
  Firebase/other outbound notifications for the drill. Avoid running two writers.
- **Static sites:** restore reviewed release assets, Nginx/Compose definitions
  and release symlink intent. The captured resolved release may need a new symlink;
  never link back to a missing old server path.

Do not start application Compose files wholesale with restored secrets during a
drill. They may launch workers or connect to external services. Prepare a reviewed
drill override and new data roots first. Exact image archives preserve runtime;
source repositories/build inputs are separate long-term maintainability work.

## 4. Host reconstruction and cutover gates

Treat saved host files as reference material, not an overwrite script. Recreate
users/groups, SSH access, filesystem mounts, ownership, service definitions,
firewall and network addressing for the new machine. Review Netplan/fstab against
new hardware/UUIDs. Do not enable Cloudflared, Tailscale ingress, UPS shutdown,
alert helpers or backup schedules until reviewed. Reauthenticate/recreate remote
routes separately; a tunnel token is not an export of Cloudflare account rules.

Run isolated app health, login, data/media and persistence checks. For PostgreSQL
also compare schema/data/financial invariants without replaying transfers. Record
restored image IDs and failures. Production cutover is a separate owner decision:
take an approved final consistent data capture, prevent dual writers, then switch
ingress and monitor. Do not replay older financial history over newer accepted
transactions. Keep a reviewed rollback strategy.

This bundle does not certify BIOS/UPS reboot behavior, account loss, every cron
entry, all host secrets or all root-owned files. Inventory gaps remain explicit.
No schedule is activated by following the selection/bootstrap helper.
