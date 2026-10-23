# HomeVault Personal

HomeVault Personal is an experimental, self-hosted command-line and terminal-interface tool for encrypted backups. It runs on the operator's computer or home server, using Restic for backup repositories and rclone to connect to a storage destination such as Google Drive.

The current version can initialize a repository, back up selected file directories, list snapshots, check repository metadata and restore an explicit snapshot to a separate destination. Database-aware exports, unattended production scheduling and retention management are not yet enabled in the personal installation.

The personal Google Drive integration requests `drive.file` access for backup files created or explicitly authorized for this application. It does not request full-drive access. Files are encrypted by Restic before upload; repository recovery passwords must be kept separately and securely by the operator.

This is self-hosted software, not a developer-operated cloud backup service. Current testing uses generated sample data. Do not rely on it as your only backup or assume a successful upload proves an application can be recovered.

- [Privacy policy](https://homevault.suwiwat.com/privacy)
- [Usage and limitations](https://homevault.suwiwat.com/terms)
- Contact: contacticev1@gmail.com

Official information site: https://homevault.suwiwat.com/. Planned features are separate from the working file-backup MVP.
