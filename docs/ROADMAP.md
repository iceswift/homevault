# HomeVault roadmap

Updated 25 October 2026. The goal is to make selected Linux home-server backups
and verified application recovery easier to operate. This is a direction, not a
release-date promise. Implemented experimental features still require review and
restore testing for each deployment.

## Delivered

| Area | Available now | Evidence / limits |
| --- | --- | --- |
| Encrypted backup and guarded restore | Go CLI/TUI around Restic; local or rclone cloud transport; project locks and cancellation | [Testing](TESTING.md); local generated-data integration and separate operator Drive drills |
| Terminal UX | Getting-started guidance, backup confirmation, job history, retention preview and detail scrolling | [Quick Start](QUICKSTART.md); no inference that timers are enabled |
| Project and Drive onboarding | Existing-key starter config, desktop PKCE OAuth wizard, local and explicit online status checks | [Onboarding](ONBOARDING.md) / [OAuth](DRIVE-OAUTH.md); Google Cloud client creation remains manual |
| Database preparation and capture | Experimental SQLite/PostgreSQL exports and pinned single-container capture with resume guards | [Databases](DATABASE-BACKUPS.md) / [Capture](FRESH-CAPTURE.md); not arbitrary application consistency |
| Jobs and alerts | Durable backup/check status, source-age checks, optional Discord alerts and reviewed systemd templates | [Workflows](PLANNED-WORKFLOWS.md); installation/preflight is separate from live scheduled proof |
| Retention and storage reclamation | Read-only plans, explicit reviewed snapshot removal and separate no-repack prune | [Retention](RETENTION.md) / [Prune](PRUNE.md); automatic deletion is not implemented |
| Recovery preparation | Encrypted credential kits, selected image/config catalogs and isolated app rehearsals | [VM drill](VM-RECOVERY-DRILL.md); selected apps on the original physical host, not full host reconstruction |
| Public distribution | [v0.1.0-preview.1](https://github.com/iceswift/homevault/releases/tag/v0.1.0-preview.1), checksums and extracted-package amd64 restore tests | [Releasing](RELEASING.md); ARM64 is cross-built only; no stable release |
| Contributor entry points | Contribution/security policies, private vulnerability reporting and issue/PR templates | [Contributing](../CONTRIBUTING.md) / [Security](../SECURITY.md) |

## Current focus

| Work | Current state | Completion evidence needed |
| --- | --- | --- |
| TUI redesign | [Mock Overview/Projects/details prototype](TUI-PROTOTYPE.md) available on main; no live engine/data | Review navigation and narrow layouts before a separate read-only live-data stage |
| Independent onboarding trial | Documentation and local demo are ready; external-user trial pending | Another person follows the published package guide and completes byte-verified restore; record friction and fixes |
| Operational workflow readiness | Four operator workflows have passed systemd preflight and a disabled monitor run | Reviewed cleanup policy, maintenance windows, one real coordinated cycle and suite alert verification before timers |
| Recovery coverage review | Selected app/config/image backups and VM drill completed | Close host inventory gaps and document what must be recreated manually; keep scope tied to tested evidence |

The operator's preferred times and retention periods are private deployment
choices, not public defaults. No recurring cloud schedule or automatic deletion
is implied by these entries. See [operator validation](OPERATOR-VALIDATION.md).

## Next, after the current checks

1. Fix onboarding/TUI friction found by independent testers; make errors and next actions clearer.
2. Add bounded cleanup of local capture stages and more complete producer-failure monitoring.
3. Exercise crash/reboot/resume behavior under real systemd service execution using generated data.
4. Expand explicit platform/app compatibility tests, including native ARM64 runtime testing.
5. Improve guided PostgreSQL setup and Drive client onboarding while preserving explicit authorization.
6. Review automatic scheduling/retention separately, with dry-run plans, protection for recovery points and failure handling before activation.

## Longer-term boundaries

Full replacement-host reconstruction, hardware/cloud-account-loss scenarios and
reviewed production cutover remain unproven. The [rebuild runbook](REBUILD.md)
helps prepare selected recovery inputs; it does not automatically install an OS,
recreate every host service or supply a bootable disk image. Data repacking,
signed release provenance and broader multi-service compatibility need separate
work and evidence.

Do not describe TUI, encrypted cloud backup or rclone integration as novel, or
claim performance/reliability superiority over established tools without comparative
results. OAuth Production status is separate from product maturity.
