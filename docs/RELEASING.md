# CI and draft Linux packages

CI runs on main pushes, pull requests and manual dispatch using hosted Ubuntu
runners, read-only repository permissions and pinned official action commits.
It checks formatting, Go race tests/vet, systemd template syntax with stubs, then
real encrypted local Restic/SQLite/job fixtures. Docker application, PostgreSQL
rehearsal and Drive tests still need explicit lab dependencies and are not claimed
by the default CI job. No production credentials, SSH, webhook or Drive account is
used. Systemd templates are not installed/started.

`scripts/fetch-test-restic.py NEW_ABSOLUTE_DIRECTORY` downloads the pinned official
Restic Linux amd64 archive and verifies its embedded SHA256 before decompression.
It does not install a system binary. Update the pinned version/checksum together
after reviewing the official release. CI's Go modules remain pinned in go.sum.

```sh
python3 scripts/package-release.py --version dev-0123456789ab --output /absolute/new-dist
(cd /absolute/new-dist && sha256sum -c SHA256SUMS)
```

The packager cross-builds Linux amd64 and arm64 with CGO disabled, embeds the version,
and creates tar.gz archives plus SHA256SUMS. Only the binary, LICENSE, README and an
explicit public-document allowlist enter archives. It never archives the working
tree, private config, database, repository data or credentials. The output must be
new; archive ownership/timestamps are normalized. Cross-compilation is not proof
of runtime compatibility: amd64 is the M70q deployment target, arm64 runtime remains
unverified. Restic, rclone, Python and database tools are not bundled.

CI uploads preview artifacts for 14 days. A v-semver tag triggers checks/builds and
creates a **draft prerelease**, never automatically publishes it. No tag or public
release is created merely by committing these workflows. A maintainer must review
the tag, tests, artifact checksums, compatibility claims and release notes before
publishing. Only that tag workflow gets repository contents-write permission for
the draft. Existing drafts are not silently overwritten.

Checksums are integrity checks, not signatures. Signed provenance, automated
dependency vulnerability review and portable recovery-key onboarding remain future
work. Do not describe this as a stable production release.

Workflow permissions and syntax follow the
[official GitHub Actions reference](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax).
