# HomeVault

**Encrypted home-server backups, with a terminal interface and a recovery-first workflow.**

[![CI](https://github.com/iceswift/homevault/actions/workflows/ci.yml/badge.svg)](https://github.com/iceswift/homevault/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

HomeVault is an open-source Go CLI/TUI for backing up selected files and prepared
application data. Restic provides encryption, snapshots and deduplication; rclone
connects cloud storage such as Google Drive. HomeVault adds project configuration,
job history, capture workflows and recovery checks around those tools.

**Early development · Ubuntu/Linux first.** Start with the local demo below.
A successful backup is not proof that your application can recover.

**[Download Linux Preview v0.1.0-preview.1](https://github.com/iceswift/homevault/releases/tag/v0.1.0-preview.1)**
· [Install and verify checksums](docs/RELEASING.md)
· [Try the local demo](docs/QUICKSTART.md)

Linux amd64 has extracted-package restore tests. ARM64 is cross-built; runtime
remains unverified. This is a prerelease, not a stable release.

## What using it looks like

![Actual HomeVault welcome screen with a synthetic project, keyboard shortcuts and getting-started guidance](docs/assets/tui-welcome.png)

Actual TUI output captured on Linux with generated data, rendered as a terminal
image. [Plain-text capture and provenance](docs/assets/README.md). No real backup
history, account identifiers or credentials are shown.

Press `b` to review a backup confirmation, then Enter to start or any other key to
cancel. Capture-required applications use their reviewed workflow instead.
Opening the TUI does not enable schedules or delete snapshots.

## Try it without a cloud account

The [prebuilt-package guide](docs/RELEASING.md) covers download, checksum verification
and running the bundled local demo without Go. You need Restic separately; the
demo uses Bash and Python 3.11+. rclone is only needed for cloud repositories.
No Docker, Google account, sudo or application data is needed for the demo once
these dependencies are installed.

<details>
<summary>Prefer to build from source?</summary>

You also need Git and Go 1.24.5+:

```sh
git clone https://github.com/iceswift/homevault.git
cd homevault
go build -trimpath -o bin/homevault ./cmd/homevault
./bin/homevault version
```

Follow the [copy-and-paste local Quick Start](docs/QUICKSTART.md) to create sample
data, back it up, restore it into a separate directory and compare the bytes.
The guide includes expected results and troubleshooting.

</details>

Complete the sample restore before adding real data or setting up
[Google Drive](docs/DRIVE-OAUTH.md). Then [prepare a real project](docs/ONBOARDING.md).

## Choose the right scope

HomeVault suits Linux home-server owners comfortable with a terminal who want
selected files or application data backed up to a local repository or cloud
storage. It does not create a bootable disk image, discover every Docker volume,
or automatically reconstruct your operating system and public routes.

| Capability | Current status |
| --- | --- |
| Local encrypted backup, snapshots, check and guarded restore | Implemented; generated-data integration tests |
| CLI/TUI, backup confirmation, job history and detail scrolling | Implemented |
| Google Drive through rclone | Implemented; manual OAuth client setup; operator cloud restore evidence |
| SQLite online export / PostgreSQL custom dump | Experimental; DB and media need coordinated capture |
| Pinned container capture and prepared multi-project workflows | Experimental; explicit configuration and activation |
| Discord alerts and systemd templates | Experimental; installed/tested per deployment, no automatic activation |
| Retention preview, manual apply and separate prune | Experimental; explicit review and confirmation |
| Recovery kits, image catalogs and isolated app recovery | Experimental; selected operator drills, not complete host recovery |
| Automatic full-machine restore or production cutover | Not implemented |

Linux amd64 is the operator-tested deployment target. Linux arm64 packages are
cross-built but runtime compatibility is unverified. Windows builds/unit tests
work; Windows deployment and restore are not supported as a validated target.

## How the pieces fit

```text
Selected files / coordinated application exports
                      ↓
            HomeVault CLI / TUI
                      ↓
        Restic encrypted repository
                      ↓
      Local storage OR rclone → Google Drive
```

There is no always-running HomeVault daemon or required public web service.
Optional desktop OAuth uses a temporary loopback callback. Optional systemd jobs
run independently of an open SSH connection. HomeVault does not read your open
browser to choose a Google account.

## What has actually been verified?

Automated CI covers Go race tests/static analysis, template syntax and real local
encrypted Restic fixtures. These tests use generated data, not production secrets.
See [testing](docs/TESTING.md) and the [CI workflow](.github/workflows/ci.yml).

One owner-operated deployment also restored selected encrypted Google Drive
snapshots and rehearsed SQLite, PostgreSQL, JSON-state and static-site recovery
in an isolated Ubuntu VM. That VM ran on the original physical server: this was
not a physical disk-loss test, a full host rebuild or an independent-user trial.
See the [VM drill and its limits](docs/VM-RECOVERY-DRILL.md) and
[operator validation](docs/OPERATOR-VALIDATION.md).

Schedules and retention depend on each deployment. Prepared templates and a
successful manual test do not mean an automatic schedule is active.

## Before backing up real data

- Keep the original repository key recoverable outside the server. Losing it loses access to encrypted backups.
- Prepare databases correctly; ordinary file copies of a running database are not a consistent backup.
- Restore into a new target and validate the application, not only a repository check.
- Keep passwords, OAuth tokens, webhook URLs and private configuration out of Git and issues.

## Documentation

| Goal | Guide |
| --- | --- |
| Complete a local backup and verified restore | [Quick Start](docs/QUICKSTART.md) |
| Configure a real project | [Onboarding](docs/ONBOARDING.md) |
| Authorize Drive | [Drive OAuth](docs/DRIVE-OAUTH.md) |
| Prepare databases / container captures | [Databases](docs/DATABASE-BACKUPS.md) · [Fresh capture](docs/FRESH-CAPTURE.md) |
| Schedule and monitor | [Jobs and alerts](docs/SCHEDULING-ALERTS.md) · [Prepared workflows](docs/PLANNED-WORKFLOWS.md) |
| Review deletion and storage reclamation | [Retention](docs/RETENTION.md) · [Prune](docs/PRUNE.md) |
| Recover on another machine | [Recovery kit](docs/RECOVERY-KIT.md) · [Rebuild runbook](docs/REBUILD.md) |
| Develop / inspect plans | [Testing](docs/TESTING.md) · [Roadmap](docs/ROADMAP.md) |

## Contribute and get help

Read [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, test commands and PR scope.
Use the [issue forms](https://github.com/iceswift/homevault/issues/new/choose) for
bugs, feature/UX requests and usage questions. Small docs fixes are welcome.

Report vulnerabilities through the private channel in [SECURITY.md](SECURITY.md).
Never put keys, OAuth responses, webhook URLs, private config or real backup data
in issues, PRs or screenshots. Preview status does not imply a security audit.

[MIT License](LICENSE). Built around [Restic](https://restic.net/) and
[rclone](https://rclone.org/); their encryption and transport work is credited to
those projects. HomeVault is an independent project.
