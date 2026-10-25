## Problem and resulting behavior

Describe the concrete problem, what changes for the user and any related issue.

## Validation

List commands/checks actually run and their results. State skipped checks and why.
For backup/recovery changes, include relevant failure/cancellation coverage.
For TUI changes, include a sanitized terminal example or screenshot if useful.

## Compatibility and operational effects

Describe config/state-format changes, dependencies, permissions, app pauses,
network activity or changes to scheduling/retention. Write "none" where applicable.

## Checklist

- [ ] I reviewed the diff for credentials, private config, logs and real backup data.
- [ ] Tests/examples use generated data or mock transports.
- [ ] I updated relevant help/docs, or explained why none were needed.
- [ ] I reported actual test coverage and remaining limits without claiming unrun checks.

Suspected vulnerabilities belong in the private channel in [SECURITY.md](https://github.com/iceswift/homevault/blob/main/SECURITY.md),
not in a public PR. Issue links are optional for small fixes.
