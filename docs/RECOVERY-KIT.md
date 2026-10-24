# Portable encrypted recovery kit (experimental)

This kit preserves selected recovery credentials in a standard age-encrypted JSON
document. It does not contain backup data or automatically copy anything off-server.
It is independent of Windows account-bound encryption. Initial automated disaster
drills used generated data. Later owner-authorized deployment-key custody and
kit-to-cloud operator checks are recorded in OPERATOR-VALIDATION.md and
VM-RECOVERY-DRILL.md; they are separate from the fixture evidence below.

## Prepare independent custody

Install reviewed **age 1.3.2** tools. Generate an X25519 identity on a separate trusted
computer, not on the server that may be lost:

```sh
age-keygen -o /private/off-server-identity.txt
age-keygen -y /private/off-server-identity.txt
```

Only the resulting **public** `age1...` recipient is needed on the original server.
Keep the private identity independently recoverable, for example in your password
manager/offline custody. Do not paste it into chat/issues/Git. This first mode
accepts one ordinary X25519 identity, not age plugins, SSH or passphrase identities.
Losing that identity can make the kit inaccessible. Keeping it only on the original
server defeats the purpose.

## Export selected credentials, offline

Choose an existing complete snapshot ID from the selected project:

```sh
homevault --config /absolute/homevault.json snapshots PROJECT
homevault --config /absolute/homevault.json kit-export PROJECT FULL_SNAPSHOT_ID /outside-backup-sources/new-kit.age --recipient AGE_PUBLIC_RECIPIENT --confirm
```

Optionally include an explicitly selected private application plan using
`--recovery-plan /private/recovery-plan.json`. Validate image availability/compatibility
and keep the selected snapshot independently retained/pinned. Export validates the
ID's syntax but does not access the repository to prove that ID is still present.
It does not initialize a repository, rotate the password or change retention. The
repository password can unlock all snapshots in that repository, and the Drive
token can access other files authorized for its app. Project selection is guidance,
not an access-control boundary. Share a kit only with someone authorized for that
whole credential scope. Export/test a new kit after relevant key/client/repository changes.

The encrypted payload contains the **original** Restic password bytes, repository
location, selected project/full snapshot ID, database artifact labels and optional
plan. For Drive it requires an explicit `rclone_config_file`; ambient/default account
settings are not exported. Only one matching dedicated Drive remote with file scope
is accepted; other-account sections, arbitrary auth endpoints, service-account files
and unsupported settings fail closed. Required token/client fields and optional
root/team IDs are normalized; unrelated token fields are not copied. Trash is enabled
in recovered Drive config. This does not reduce provider-side OAuth permissions.

There is no automatic .env/Firebase/database-login/OS/Docker-image collection. An
explicit plan may itself contain sensitive command values; treat it privately.
Secrets are passed to age through stdin, not argv or a temporary plaintext bundle.
The new encrypted file is 0600 and must be outside configured sources, state,
credentials and local repository. Existing output is never overwritten. Encryption
failure can leave partial encrypted output: inspect it and retry with a new name.
Successful export is not proof the kit/backup can be recovered.

Copy the encrypted kit to independent off-server storage yourself and preserve the
reported SHA256 out of band. The tool does not upload it. Encryption to a public
recipient does **not** authenticate who created a kit: use your own trusted copy and
checksum, not an unsolicited kit. Do not keep all recovery material solely inside
the same encrypted repository or on the same machine.

## Open on a new machine

No original HomeVault config/state/password is required to open the kit:

An operator later verified kit-to-cloud access on an independent Windows PC using
direct Restic/rclone and restored a small bundle with matching hashes; see
VM-RECOVERY-DRILL.md. This does not certify the HomeVault Windows engine. Transport
executable paths containing spaces failed in that setup; task-local PATH plus a
reviewed executable basename worked. The Linux bootstrap remains the primary
application recovery route.

```sh
homevault kit-open /off-server/new-kit.age /absolute/new-private-recovery --identity-file /private/off-server-identity.txt --confirm
```

Use `--repository /new-location/repository` when a local repository has moved.
`--restic-binary`, `--rclone-binary` and `--age-binary` select reviewed local tools.
The target must not exist. Wrong identity/tampered ciphertext fails before any
credential tree is created. Decrypted JSON is size-bounded and validated. An optional
plan is validated in memory, then preserved for manual review;
it is never executed on open. Partial authenticated output after a disk/write failure
is kept private for inspection and not silently reused.

Output includes private original credentials, `homevault.json`, nonsecret
`recovery-info.json`, `RESTORE-FIRST.md` and an optional `recovery-plan.json`.
Secrets are not printed. Unix 0700/0600 modes do not validate Windows ACLs. Opening
produces plaintext credentials in this private directory by design; protect it.

The generated config sets **recovery_only=true**, uses new state/cache and unavailable
source placeholders, and does not import old green job history. Init, backup/capture,
snapshot removal and prune are refused. Listing/checking/restoring and isolated
rehearsal remain available; normal cache/lock and token-refresh writes still occur.
This is a HomeVault safety guard, not a provider IAM restriction or sandbox. Do not
remove it to fix an authorization/restore error. Existing legacy configs omit the
flag and retain their behavior.

Inspect config/repository/account, list snapshots, run metadata check, then restore
the explicit ID into a new empty target and verify DB/media/files. Review any
included image/commands before `rehearse ... --confirm`. App images, missing secrets,
production cutover, DNS/proxy and schedules must be recovered separately. A Drive
refresh token may need renewed Google consent using the original client/account.

The format is age v1 encryption around UTF-8 JSON schema version 1; byte fields
`restic_password` and `rclone_config` use standard JSON base64 encoding. An independent
age implementation can decrypt it without HomeVault, but doing so exposes plaintext
credentials: use a protected location. HomeVault currently pins age 1.3.2 for tested
tool behavior. Full-size inputs are capped, tool time is bounded, and child output
is withheld. The original machine/repository key is never regenerated.

## What was verified

- Local real Restic/age fixture: source/config/state/original password removed;
  repository relocated; encrypted off-server-model kit and separately kept identity
  recreated a new config and restored byte-identical files. Wrong key, damaged kit,
  overwrite and write-operation guards were refused.
- A fresh Docker Python fixture, no original source/state/key mounts, no external
  network, no Docker socket, read-only root/capability drops and bounded resources:
  recovered credentials/SQLite/media/app source, then passed health/database/media/
  login checks and removed its container.
- Unit checks prevent repository mutations and unsafe multi-account Drive export;
  authenticated malformed kit metadata is rejected before credential output.

These are generated-data simulations, not a physical server-loss/cloud-account-loss
certification or proof that DealDoor/Saan production can recover. Actual independent
key custody and a real approved capture/rehearsal remain activation prerequisites.

Reference: [age format/tool](https://github.com/FiloSottile/age).
