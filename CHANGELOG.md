# Changelog

User-facing changes, compatibility notes and release boundaries for HomeVault.
`Unreleased` describes main-branch changes not included in a published release.
Dates use `YYYY-MM-DD`; release history links point to the exact tag or comparison.

## [Unreleased]

### Added

- Local readiness panel (`d`) in manual-job TUI: selected config/destination, executable lookup, bounded path/credential metadata and next actions. Missing tools keep diagnostics open with jobs blocked; no tool is run, credential content opened, write probe or cloud request made.
- Read-only local attempt history inside `tui-jobs` (`h`), with newest-first deduplication, recorded durations, separate backup/check results and conservative refresh-error states.
- Separate `tui-jobs` mode for explicitly configured file-only manual backup/check jobs, typed confirmation, recorded progress and cancellation cleanup. Configuration and previous-job guards run under the engine lock; database/capture/recovery scopes remain blocked.
- Native `tui-status --plan FILE` local-record view with manual async refresh, explicit coordinator/legacy provenance, source/upload age separation and conservative Unknown states. It performs no engine actions, credential-file reads, cloud/service checks or state writes.
- Isolated `tui-demo` design prototype with mock Overview/Projects/details, filtering, responsive layouts, ASCII/monochrome support and deterministic frame rendering. It never loads live config or starts backup operations.
- Contributor guide, security policy, private vulnerability reporting and structured issue/PR templates.
- Captured synthetic TUI preview with plain-text provenance, a static vector banner and clearer README navigation.
- FAQ covering backup scope, cloud storage, key custody, database consistency, scheduling and retention.
- This changelog and release-maintenance guidance.

### Changed

- Manual-job TUI separates backup/check evidence, unknown source freshness and uninspected application recovery; action blocks do not erase backup results. Explicit `--context lab|production|unknown` labels replace guessing deployment scope from project names.
- README prioritizes the downloadable preview and verified demo; advanced feature details can be expanded separately.
- Roadmap distinguishes delivered features, current verification work and future milestones.
- GitHub About/links/topics describe the project and direct new users to the public Quick Start.
- Future packages include contribution/security guidance, FAQ, changelog and reviewed preview assets. Existing published assets are unchanged.

### Upgrade notes

- These changes are on main and subsequent CI artifacts; they are **not** inside `v0.1.0-preview.1`.
- New display-only `tui-demo` and read-only `tui-status` commands are available on main. The existing live CLI/TUI entry points, configuration schemas and Go dependencies are unchanged; a separate guarded engine entry point supports reviewed file jobs; no application-data migration is introduced.
- These documentation/packaging updates do not enable timers, change repository keys, activate retention or update a running server automatically.

## [v0.1.0-preview.1] - 2026-10-25

First public experimental Linux preview, built from commit
[`acb6fdb`](https://github.com/iceswift/homevault/commit/acb6fdb8da5327694549deda5e0af2fc76b1b442).

### Added

- Go CLI/TUI around Restic for encrypted backups, snapshot listing, repository checks and explicit restore into a separate destination.
- Getting-started guidance, backup confirmation, job history and width-aware detail scrolling.
- Optional rclone/Google Drive transport and desktop OAuth setup with explicit authorization.
- Experimental database preparation, reviewed capture workflows, Discord alerts, retention review, recovery kits and application rehearsals.
- Linux amd64/arm64 archives, `SHA256SUMS`, embedded source/toolchain metadata and a local generated-data walkthrough.
- CI checks for Go race tests/vet, real local recovery fixtures and backup/check/restore using an extracted amd64 package.

### Compatibility and limits

- Linux amd64 is runtime-tested; ARM64 is cross-built/structurally checked only. Windows/macOS deployment packages are not supplied.
- Restic/rclone and optional workflow tools are separate dependencies. See [version-specific release notes](https://github.com/iceswift/homevault/blob/v0.1.0-preview.1/docs/releases/v0.1.0-preview.1.md).
- Automatic retention and complete host reconstruction are not implemented. Selected operator VM recovery evidence is not a physical disk-loss or production-cutover guarantee.

### Upgrade notes

- This is the first public release; there is no earlier public release migration path.
- When adopting an existing repository, preserve its original key/configuration and validate recovery. Do not initialize over existing backup data or replace its key to follow a demo.
- Downloading or opening the preview does not install systemd services, activate schedules or delete backups. Keep independently recoverable key custody and test a restore before real use.

[Unreleased]: https://github.com/iceswift/homevault/compare/v0.1.0-preview.1...main
[v0.1.0-preview.1]: https://github.com/iceswift/homevault/releases/tag/v0.1.0-preview.1
