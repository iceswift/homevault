# Isolated Ubuntu recovery drill — 25 October 2026

This is operator evidence for selected applications, not a certification of every
host service or a production cutover. Six pinned snapshots were freshly restored
from Google Drive into new private targets. A new Ubuntu 24.04 cloud-image VM had
its own kernel, 24 GiB virtual disk, 4 GiB RAM and two vCPUs. It ran on the same
physical source server; no replacement hardware or physical disk failure occurred.

## Isolation and image proof

QEMU ran in a nonprivileged, capability-dropped container as the operator UID,
with KVM access, only test-disk/output mounts and a read-only mount of the fresh
restore inputs. It had no source application mounts or host Docker socket. Guest
Docker was installed before any restored app was started. The VM NIC was then
disabled and the outer test container's network disconnected; an external HTTPS
request failed before application checks. Application containers also used no
external network, with the PostgreSQL app sharing only its isolated DB namespace.

Seven recovered image references loaded into the guest's initially empty Docker
daemon. All loaded image IDs matched the original pins; rootfs layer identities
also matched the archived config blobs. Guest Docker was 29.1.3. An initial
manifest-selector failure occurred after image load; later application attempts
reused that same private VM disk and rechecked image IDs. The reports preserve
this distinction rather than claiming every retry started from an empty daemon.

## Checks that passed

| Target | Observed result |
|---|---|
| SQLite application | integrity_check; health; database/media checks; media bytes through HTTP; public demo login/session access; container restart checks |
| PostgreSQL application | trusted dump imported into a new DB; 28 restored tables fingerprinted; fingerprints unchanged by initial app reads; selected financial table fingerprints unchanged during auth probe; all-table persistence across DB restart |
| PostgreSQL application media/auth | two restored image files matched through HTTP; anonymous access redirected to login; a new test-only DB session for an existing synthetic allowlisted account accessed its profile; the same session worked after app/DB restart |
| JSON-state API | restored JSON hashes; API health and board HTTP response; Firebase disabled; background scraper lifespan disabled; restart with JSON bytes unchanged |
| Two static sites | recovered index bytes matched through local HTTP; all staged-file manifests were validated against their catalog-selected roots |
| Cleanup and source | guest test containers removed; QEMU runner stopped/removed; original source container IDs, images and start times unchanged; live health endpoints responded; no HomeVault timers installed/enabled |

The PostgreSQL auth probe did **not** run the OTP form/browser flow. It created a
random session only in the isolated restored DB, then tested real HTTP access and
persistence for the restored account. No payment worker, gateway secrets, SMS,
public tunnel, production UPS action or production credential file was promoted.
Static HTTP checks did not exercise browser JavaScript or public TLS ingress.

Database fingerprints compare restored test data before/after operations; they
are not independent pre-backup production fingerprints. Snapshot recovery times
differ by application and do not form one simultaneous host checkpoint. Local
test disks/restored data remain private plaintext under restricted filesystem
permissions; removal of test containers is not secure erasure of those files.

## Lessons incorporated into the runbook

- Select a backup manifest at the exact `snapshot_paths` root in the catalog.
  A recursive search for `manifest.json` can select a PWA manifest instead.
- Preserve permissions of both a SQLite file **and its parent directory** for the
  image's numeric UID. A correct file owner cannot overcome a root-only directory.
- Wait for PostgreSQL TCP plus an actual SQL query before import. Its temporary
  initialization server can respond on a Unix socket before the final DB is ready.
- Authentication checks must recognize Next.js streaming redirect meta tags and
  verify authenticated content, not infer access from HTTP 200 alone.

These were test-harness corrections; failed attempts were retained and were not
reported as successful app recovery. No original deployment was changed to make
the drill pass.

## Remaining gates

Host users, root cron/inventory gaps, real hardware/BIOS, UPS behavior, network
reconstruction, account loss and production ingress/cutover remain unproven. VM
cloud downloads were orchestrated by the host; an independent off-server kit/cloud
credential check is recorded separately. This drill does not start backup schedules.

## Independent kit-to-cloud check

On the owner's separate Windows PC, the encrypted kit and private age identity
alone opened a new private credential tree. No source-server credential file was
read. Restic 0.19.1 with independently downloaded rclone 1.75.2 reached Google Drive,
verified all six catalog selections by full ID/project tag, restored the small
PostgreSQL application's twelve-file bundle and matched every manifest hash.
The tools and all decrypted/restored files stayed under the protected PC custody
directory; owner/SYSTEM-only ACLs were checked and the temporary plaintext tree
was removed. The primary age identity was not sent to the source server or VM.

This was a direct Restic/rclone transport check, not certification of the entire
HomeVault Windows engine. Absolute Windows `rclone.program` paths containing
spaces failed in this setup; selecting the verified executable by basename through
a task-local PATH succeeded. Global PATH was not changed. rclone's Windows ZIP
matched the published SHA-256 from its official release download; its detached
signature was not verified. This check does not simulate loss of the Google account
or future credential revocation.

References: [Canonical cloud images](https://cloud-images.ubuntu.com/releases/noble/release/),
[QEMU invocation](https://www.qemu.org/docs/master/system/invocation.html),
[QMP network controls](https://www.qemu.org/docs/master/interop/qemu-qmp-ref.html),
[Next.js streaming redirects](https://nextjs.org/docs/app/api-reference/functions/redirect).
The Ubuntu download matched SHA256SUMS fetched over HTTPS; the detached GPG
signature was not checked in this operator run.
