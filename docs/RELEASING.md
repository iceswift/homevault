# Linux preview packages and verification

HomeVault is early development software. The first public preview is
[v0.1.0-preview.1](https://github.com/iceswift/homevault/releases/tag/v0.1.0-preview.1).
Its public archives and checksums were downloaded without GitHub authentication
and verified after publication. This is a prerelease, not a stable release.
Future candidates remain drafts until a separate maintainer publication step.

## Install a published preview without Go

Ubuntu/Linux amd64 is the initial runtime target. ARM64 is cross-built and
structurally checked, but not runtime-validated. Windows/macOS deployment packages
are not supplied. You need Restic separately; cloud storage additionally needs
rclone. See the official [Restic installation guide](https://restic.readthedocs.io/en/stable/020_installation.html).
Python 3.11+ and Bash are used by the bundled sample walkthrough.

To download this published preview, run in Bash (requires curl, tar and sha256sum):

```sh
set -euo pipefail
test "$(uname -s)" = Linux
VERSION=v0.1.0-preview.1
case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "No package for this architecture"; exit 1 ;;
esac
DOWNLOAD="$(mktemp -d)"
cd "$DOWNLOAD"
ARCHIVE="homevault_${VERSION}_linux_${ARCH}.tar.gz"
BASE="https://github.com/iceswift/homevault/releases/download/$VERSION"
curl --fail --location --output "$ARCHIVE" "$BASE/$ARCHIVE"
curl --fail --location --output SHA256SUMS "$BASE/SHA256SUMS"
sha256sum --check --ignore-missing SHA256SUMS
# Continue only when the downloaded archive reports OK.
tar -xzf "$ARCHIVE"
cd "homevault_${VERSION}_linux_${ARCH}"
./homevault version
cat RELEASE.json
```

Use a dedicated Bash session and stop on any failed command. If the tag is still a
draft, download returns 404; do not substitute an untrusted mirror or bypass checks.
Checksums detect corrupted downloads but do not authenticate a publisher. Obtain
both the archive and checksums from the official release and review its tag/CI.
No signature/provenance attestation is supplied in this preview.

Run the bundled demo from the extracted directory (Restic must be on PATH):

```sh
HOMEVAULT_QUICKSTART_BINARY="$PWD/homevault" bash scripts/quickstart-smoke.sh
```

Expected: `PASS: restored proof.txt matches the generated source byte for byte`.
This uses generated files in a new private temporary directory and a local
encrypted repository; no Google account, Docker, sudo or production data is used.
The generated sandbox is retained for inspection. See [Quick Start](QUICKSTART.md).
You can keep the executable in this directory; global installation is optional.

## What is in an archive?

Only the HomeVault executable, MIT license, README, `RELEASE.json`, the generated-data
demo script and an explicit allowlist of tracked public documents. No private
configuration, credentials, database, repository contents or operator notes are
included. Restic/rclone/Python/age/database tools are not bundled.

`RELEASE.json` records the source commit, version, architecture, CGO setting,
Go build toolchain and tested external-tool versions. A tagged package requires
clean tracked source at that tag's commit. This metadata is descriptive, not a
cryptographic attestation or proof that a local custom build passed CI.

## Maintainer checks and draft creation

CI runs on main pushes/PRs using pinned action commits and read-only permissions.
It runs Go race tests/vet, systemd syntax checks, generated-data Restic/age fixtures,
the public walkthrough and extracted-package smoke validation on Ubuntu amd64.
Default CI does not test live Drive consent, Docker app recovery, PostgreSQL
rehearsals, physical loss or ARM64 execution.

For a local development package, use a new absolute output directory:

```sh
python3 scripts/verify-quickstart.py
python3 scripts/test_verify_release.py
python3 scripts/package-release.py --version dev-0123456789ab --output /absolute/new-dist
python3 scripts/verify-release.py /absolute/new-dist --version dev-0123456789ab --commit "$(git rev-parse HEAD)" --smoke
```

The final command requires Linux amd64 and Restic on PATH. Without `--smoke`, it
checks both checksums, archive safety, required files, metadata and ELF architecture
without executing binaries. Negative tests cover corruption, unsafe members,
source mismatch and duplicate checksum entries. This is not a malware scanner.

For a candidate, commit version-specific notes under `docs/releases/VERSION.md`,
complete main CI, then create/push the reviewed semantic version tag. The tag
workflow repeats verification, builds and tests the **exact packaged binary**, and
creates a **draft prerelease** with those notes/assets. Existing drafts are not
overwritten. No public release is automatically published.

Before publishing, inspect the exact tag/commit, successful workflow, assets,
checksums, `RELEASE.json`, recovery limits and notes. Publishing remains a separate
maintainer action. Main CI artifacts expire after 14 days; release assets belong
to the separately created draft/published release.

`scripts/fetch-test-restic.py` and `fetch-test-age.py` fetch pinned official tools
with pinned SHA-256 checks for Linux amd64 CI/labs, not system installation.
Dependency updates require review of the version and checksum together.
