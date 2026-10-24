# HomeVault Personal — Privacy policy

Effective date: 24 October 2026. This document describes the personal self-hosted installation, not a hosted service accepting other users' backups.

## Data access and purpose

HomeVault Personal uses Google Drive solely as a destination for backups selected by the operator and as a source when listing, checking or restoring those backups. The current integration requests `https://www.googleapis.com/auth/drive.file`. This permits access to app-created files and other files explicitly authorized for the app, rather than unrestricted access to the whole Drive.

The installation also reads the authorized account's email address through Drive's account-information endpoint to confirm the intended destination account. It does not add Google profile or Gmail permissions for this check.

## Processing and storage

Selected source files are read on the operator's machine. Restic encrypts backup repository data before it is transferred to Google Drive. During restoration, selected backup data is downloaded and decrypted on the operator's machine. Google still processes storage metadata such as folder names, repository file sizes, timestamps and normal service/request metadata; encryption does not conceal all metadata.

OAuth client credentials and access/refresh tokens are stored in a private rclone configuration on the operator's machine. The repository recovery password is also stored locally in a separate private file. The personal deployment restricts access using operating-system permissions. Backup repository data is stored in the Google account authorized by the operator. Protecting the account, server and recovery material remains essential.

The optional new desktop OAuth wizard performs consent directly with Google using PKCE and a temporary loopback callback. It saves credentials locally, does not request an identity/profile/Gmail scope, and does not read the account's email for its status display. Account selection occurs in the browser. Its optional online status check performs a shallow directory listing through rclone but does not display or retain directory names. It does not change existing authorization automatically or send tokens to the public information website.

Optional recovery-kit export encrypts operator-selected original recovery credentials and a dedicated Drive configuration into an age-encrypted local file for independent custody. It does not automatically upload that file or export unrelated accounts. Opening a kit writes plaintext credentials only into a new private recovery directory. The operator chooses storage/custody and must protect the decryption identity separately. No production recovery kit has been created by the new generated-data tests described in the project documentation.

## Sharing and other uses

This installation does not send backup contents or OAuth tokens to a HomeVault-operated central service. Its Google API access is for the backup/restore functions described here, not advertising, data resale or training AI models. HomeVault currently implements no advertising or analytics collection. Google processes requests under its own applicable terms and privacy practices. If these pages are hosted publicly, the hosting provider may process ordinary web access logs separately from the backup application.

The software also supports optional operator-configured Discord notifications. When enabled, these send only the configured project label and generic job/health events to the operator's selected Discord channel; backup contents, OAuth tokens, recovery passwords and session cookies are not included. Notifications are not enabled automatically and are not a HomeVault-operated telemetry service. The personal installation described here has not enabled this new notification integration.

## Retention, revocation and deletion

The current installation does not enable automatic backup pruning. Backup data remains in the chosen storage until the operator removes it. The operator can revoke HomeVault's access in Google Account third-party connections; revocation does not itself delete existing backup files. To remove backups, delete the dedicated backup folder from Drive and manage Trash according to Google's interface. Local credentials, caches, source exports and restored files are managed separately on the operator's machine.

The operator should keep an independent secure copy of repository recovery material. Losing the recovery password can make encrypted backups inaccessible. Revoked/expired OAuth authorization may require a new consent flow.

## Contact and changes

Questions about this personal application: contacticev1@gmail.com. This disclosure must be reviewed before changing the requested scopes, adding a hosted service, collecting additional data or using Google data for another purpose. Updates will be published at https://homevault.suwiwat.com/privacy with an updated effective date.
