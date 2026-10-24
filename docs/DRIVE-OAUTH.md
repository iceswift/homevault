# Google Drive OAuth wizard (experimental)

The wizard prepares a new private rclone config, guides Google Desktop consent,
and reports local/explicit online status. It does not create a Google Cloud
project/client, change consent-screen publishing status, select the Google account
for you, initialize a Restic repository, upload data or activate services.

## Prerequisites

Enable the Google Drive API in your own Cloud project and download an OAuth client
of type **Desktop app**. Keep the JSON private (0600 on Linux; protect Windows ACLs
separately). Web/service-account clients and nonofficial auth/token endpoints are
refused. The Desktop client secret is not treated as an embedded application secret;
it and refresh tokens stay in private files, never command-line values.

The authorization uses the official Go OAuth library with S256 PKCE, random
single-use state, fixed Google HTTPS endpoints, offline access and account-selection
consent. It requests only `https://www.googleapis.com/auth/drive.file`. Existing
server-side grants are not automatically revoked or reduced. Other granted scopes
in a token response are refused rather than silently accepted.

## 1. Prepare, without network access

```sh
homevault drive-setup /absolute/new-private-drive --client-json /absolute/private/desktop-client.json --remote gdrive --project-id your-cloud-project
homevault drive-status /absolute/new-private-drive
```

`--project-id` is optional but recommended to catch the wrong downloaded client.
The output directory must be brand new with an existing parent. It contains
`connection.json` and `rclone.conf` (0600; directory 0700). The config fixes Drive
type, `drive.file` scope and `use_trash=true`. No existing config, consent or token
is changed. Partial setup is retained for inspection if a write fails.

The Drive file scope covers app-created/authorized files; it is not arbitrary
whole-Drive access. Reusing the original OAuth client matters when reconnecting to
an existing repository. A different client/account may not see old files. The
wizard does not migrate or delete them.

## 2. Explicitly authorize in your browser

```sh
homevault drive-auth /absolute/new-private-drive --confirm
```

Open the printed **Google** authorization URL in your system browser, select the
intended account and review the consent. Do not paste the URL/callback code into
chat/issues/logs. The terminal reports success only after a usable refresh token
was received and saved atomically. No password/token is typed into HomeVault.

During this command only, a temporary listener binds **127.0.0.1:53682**, validates
callback host, path, method, issuer when supplied, unique state and one-time use,
then closes on completion/cancel/10-minute timeout. Port conflicts fail without
stopping existing listeners. `--port 0` chooses a random port; an explicit port must
be 1024–65535. The callback page contains no token/code and asks you to check the
terminal; it is not a public web UI, Tailscale Serve route or persistent daemon.

### A headless SSH home server

Run drive-auth on the server. In a **second local terminal**, create the loopback
forward shown by the wizard, substituting your SSH user/host:

```sh
ssh -N -L 127.0.0.1:53682:127.0.0.1:53682 USER@SERVER
```

Keep both terminals open and open the Google URL in your **local** browser. Use the
actual selected port if changed. Do not forward externally or expose the callback
through a public proxy. Close the SSH forwarding session after authorization.

Existing saved tokens are never overwritten by drive-auth. To reconnect, prepare
another new private directory using the same intended client, authorize it, verify
access, then explicitly review/update the deployment's config path. Failed consent
does not automatically revoke any grant Google may already have created; manage
revocation privately in Google Account settings if required.

## 3. Check and connect to HomeVault

```sh
homevault drive-status /absolute/new-private-drive --online
homevault setup /absolute/new-project-config --project example --source /srv/example/files --repository rclone:gdrive:HomeVault/example --password-file /absolute/private/original-restic-key --rclone-config /absolute/new-private-drive/rclone.conf
```

`drive-status` without --online is local only: authorization_required,
authorized_cached or refresh_needed. It exposes only remote/project labels,
requested scope, access-token expiry and refresh availability, never token values
or account email. Cached/refresh availability is not proof Google will accept it.
The TUI `d` key displays this local status for wizard-prepared configs.

`--online` explicitly runs a shallow read-only rclone directory listing, discards
all names/output and reports reachability. It may refresh the private token file.
It uses a bounded context/retries and accepts `--rclone-binary` for an explicitly
installed executable. It proves current access, **not** the destination account's
identity, repository visibility, future refresh validity or restore correctness.
No identity/profile/Gmail scope is requested. Review the account in browser consent.

`rclone_config_file` is an optional top-level HomeVault setting. With it, Restic's
rclone child uses exactly that file and ambient RCLONE_* overrides are cleared to
avoid accidentally using another remote/token. Use file configuration for desired
settings; encrypted rclone configs requiring RCLONE_CONFIG_PASS are not supported
by this explicit plaintext wizard path. Without this setting, existing deployments
retain their legacy environment/config behavior. Source/restore paths cannot
overlap the credential file. For explicit rclone configs, job/review repository
fingerprints also bind the config path (not token contents), so changing that path
does not silently reuse old proof. Use a new reviewed state directory when adopting
the new path; inspect any identity mismatch rather than editing history to green.

Wizard/status and HomeVault child operations share a per-config credential lock.
The private config parent must be writable for lock files and rclone token refresh.
Adapt service source access/ReadWritePaths explicitly before production use; the
wizard changes no systemd service or permissions outside newly created files.
Do not run external rclone writers against the same config concurrently. A crashed
operation can leave a stale lock; inspect running processes before manual recovery.

## Refresh lifetime and verification limits

rclone handles normal token refresh during transport. No lifetime grant is promised.
Google testing-mode refresh limits, revocation, account policy, inactivity and other
provider rules may require renewed consent. Review the Google Auth Platform
audience/publishing state yourself; the wizard does not change or certify it.
Access expiry is not the refresh token's lifetime.

Tests use fake Desktop clients, loopback callbacks and mocked Google token exchange:
PKCE/state/replay/host/issuer checks, denial, cancellation, missing refresh token,
unexpected scope, concurrent config edits, redaction and refusal to overwrite.
Linux fake-rclone tests verify explicit config, clean environment and hidden child
output. An opt-in real-rclone check reads generated config offline via listremotes.
These tests do not certify a new live Google consent/refresh round trip; no actual
account or production token was changed by developing/testing this wizard.

References: [Google Desktop OAuth](https://developers.google.com/identity/protocols/oauth2/native-app),
[rclone Drive](https://rclone.org/drive/), [headless SSH setup](https://rclone.org/remote_setup/).
