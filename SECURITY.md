# Security policy

HomeVault handles repository keys, cloud authorization and recovery/deletion
operations. Report suspected vulnerabilities **privately**, including path/command
injection, unsafe restore overwrite, unintended data deletion, permission bypass,
credential exposure and backup-success reporting that hides failed capture/checks.

## Report privately

Use [GitHub private vulnerability reporting](https://github.com/iceswift/homevault/security/advisories/new).
Sign in to GitHub and choose the private report workflow. Do not put exploit details,
credentials or sensitive backup data into public issues, PRs or Actions logs.

Include only the minimum needed to reproduce:

- Affected version or full commit, OS/architecture and relevant tool versions.
- Expected boundary, observed behavior and potential impact.
- Steps or a minimal generated-data proof of concept.
- Redacted error text and any safe mitigation you found.

Never send real repository passwords, age identities, OAuth tokens/client secrets,
webhook URLs, private configuration or database dumps, even in a private report.
Use synthetic examples. If the private channel is unavailable, open a public issue
asking only for a private security contact, with no vulnerability details.

## Supported versions and response expectations

The latest public preview and current main branch are the active review targets.
There is no stable/LTS branch or guaranteed backport/support period. Check the
[release list](https://github.com/iceswift/homevault/releases) for the current preview;
older preview fixes may require updating rather than receiving a backport.

This is a small volunteer-maintained project. There is no guaranteed response time,
patch deadline or paid bug bounty. Maintainers will assess reproducible reports,
coordinate remediation/disclosure with the reporter where possible, and publish
an advisory when appropriate. Do not interpret preview status as a security audit
or a promise that all supported configurations are safe.

## If a credential was exposed

Treat it as compromised and revoke/rotate it through the issuing service. Deleting
an issue, message or commit does not revoke a credential. Stop sharing sensitive
artifacts and report the software issue privately with synthetic evidence. Preserve
your original repository key in independent custody: changing a config value alone
does not safely migrate encryption keys or restore lost access.

## Scope and boundaries

Restic supplies repository encryption and integrity primitives; rclone supplies
cloud transport. Those tools have their own security policies. Report a suspected
HomeVault integration problem here, and upstream-specific issues to the relevant
project privately. SHA256SUMS detects corruption but is not a signature.

A successful repository check is not an application recovery guarantee. Full host
reconstruction, production cutover and arbitrary app capture are outside the
current validated scope. Use separate generated-data restore drills, least-needed
permissions and independent key custody before relying on real backups.
