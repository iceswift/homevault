# Contributing to HomeVault

Thanks for helping make backups easier to operate and recovery easier to verify.
Documentation fixes, reproducible bug reports, usability feedback and focused code
changes are welcome. Start with the [local Quick Start](docs/QUICKSTART.md).
HomeVault is early development software; Ubuntu/Linux amd64 is the initial target.

## Choose the right channel

- Bug: use the [bug report form](https://github.com/iceswift/homevault/issues/new?template=bug_report.yml).
- Feature or UX idea: describe the task and expected outcome in the [feature form](https://github.com/iceswift/homevault/issues/new?template=feature_request.yml).
- Setup question: use the [help form](https://github.com/iceswift/homevault/issues/new?template=help.yml).
- Vulnerability, key exposure or unsafe restore/deletion: follow [SECURITY.md](SECURITY.md) privately, not a public issue or PR.

Search existing issues first. For larger changes to capture, locking, permissions,
retention or recovery formats, discuss the design before investing in implementation.
Small documentation and bug fixes can go directly to a PR; an issue is not required.

## Develop locally

Fork the repository on GitHub, clone your fork and create a focused branch.
Requirements: Git, Go 1.24.5+ and Python 3.11+ for Python tooling. Restic is required
for the real local fixtures, rclone only for cloud tests, and Docker/database tools
only for the corresponding opt-in rehearsals. Do not use a production server as
your development sandbox.

```sh
git clone https://github.com/YOUR_USERNAME/homevault.git
cd homevault
git switch -c fix/short-description
go build -trimpath -o bin/homevault ./cmd/homevault
go test ./...
go vet ./...
python3 scripts/verify-quickstart.py
python3 scripts/test_verify_release.py
```

Format changed Go files with `gofmt -w PATH_TO_CHANGED_FILE.go`. On Linux, run
`go test -race ./...` before submitting Go changes. Default Go tests skip real
Restic/age integration when the opt-in tool variables are absent; a green default
run is not proof of encrypted backup/restore.

For Linux amd64 generated-data integration tests, the pinned lab download helpers
do not install tools globally. They fetch official binaries with pinned checksums:

```sh
TOOLS_PARENT="$(mktemp -d)"
python3 scripts/fetch-test-restic.py "$TOOLS_PARENT/restic"
python3 scripts/fetch-test-age.py "$TOOLS_PARENT/age"
HOMEVAULT_TEST_RESTIC="$TOOLS_PARENT/restic/restic" HOMEVAULT_TEST_AGE="$TOOLS_PARENT/age/age" go test ./internal/recovery ./integration -count=1
```

These tests do not need real cloud accounts, production credentials or app data.
Review [testing and limits](docs/TESTING.md) for opt-in Docker/PostgreSQL/Drive checks.
Never claim those checks passed because default CI is green. CI additionally runs
systemd syntax checks, the public walkthrough and extracted Linux package tests.
ARM64 runtime and Windows deployment remain unvalidated.

## Keep changes reviewable

- Explain the concrete problem and resulting behavior. Keep unrelated refactors separate.
- Use generated fixtures and fake transports for tests. Do not add real webhooks, tokens, account exports or application databases.
- For backup/recovery changes, test relevant failures and cancellation as well as success. A failed check must not advance verified-success timestamps.
- Preserve explicit confirmation for destructive operations, restore destination isolation, original-key reuse and operation locks.
- Do not introduce automatic app stops, scheduling, deletion, network calls or credential discovery into read-only commands.
- Keep dependencies small; explain why a new dependency is needed and update its lock/checksum files together.
- Update user-facing help, docs and fixtures when behavior changes. The Quick Start Bash block must match its tested script.

Use the PR template to describe validation that actually ran, including skipped
checks and known limitations. Screenshots or terminal examples help TUI changes,
but remove hostnames, paths, account names and secret-bearing output first.
AI-assisted contributions follow the same review and evidence requirements; explain
the change and verify it yourself. Generated output alone is not validation.

## Before pushing

Inspect `git diff` and `git diff --cached`. Keep these out of commits and reports:
repository keys, age identities, OAuth client/token JSON, rclone.conf, webhook URLs,
.env files, backups/dumps, private deployment plans and logs containing those values.
A `.gitignore` entry is not a substitute for reviewing the diff.

No CLA or DCO sign-off is currently required. Submit only work you have the right
to contribute; contributions are distributed under the project's [MIT license](LICENSE).
Maintainers may ask for changes or defer scope. Review/reply times are not guaranteed.
Keep discussion respectful, specific and focused on the problem.
