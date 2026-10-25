# Your first backup and verified restore

This Ubuntu/Linux walkthrough uses **generated data and a local encrypted
repository**. It does not read your application data, connect Google Drive, stop
containers, send notifications, enable timers or delete snapshots.

## 1. Check dependencies and build

Use Bash, Git, Go 1.24.5+, Restic and Python 3. Docker and rclone are not needed.
Install dependencies first using their official instructions:
[Go](https://go.dev/doc/install), [Restic](https://restic.readthedocs.io/en/stable/020_installation.html).
The pinned fixture tool used by CI is Restic 0.19.1; other versions need their own
validation. Installing dependencies may require administrator access; the demo does not.

```sh
go version
restic version
python3 --version
git clone https://github.com/iceswift/homevault.git
cd homevault
go build -trimpath -o bin/homevault ./cmd/homevault
./bin/homevault version
```

A source build may report `dev`. Run the following blocks in the same Bash session
from the repository root. A new random private directory avoids colliding with an
existing repository or configuration.

## 2. Back up and restore sample data

The exact walkthrough below is also [scripts/quickstart-smoke.sh](../scripts/quickstart-smoke.sh),
run in Linux CI with real Restic. Either paste the block **or** run
`bash scripts/quickstart-smoke.sh`; do not run both expecting the same directory.
`set -euo pipefail` stops this shell on failure, so use a dedicated terminal session.

```bash
#!/usr/bin/env bash
# Generated data only. Keep the sandbox for inspection; never remove it automatically.
set -euo pipefail
umask 077

HV="${HOMEVAULT_QUICKSTART_BINARY:-$(pwd)/bin/homevault}"
test -x "$HV"
command -v restic >/dev/null
command -v python3 >/dev/null
LAB_PARENT="$(mktemp -d "${TMPDIR:-/tmp}/homevault-demo.XXXXXXXX")"
LAB="$LAB_PARENT/lab"
RESTORE="$LAB_PARENT/restored"
printf 'Demo files stay in: %s\n' "$LAB_PARENT"

"$HV" demo-setup "$LAB"
"$HV" --config "$LAB/homevault.json" doctor
"$HV" --config "$LAB/homevault.json" init
"$HV" --config "$LAB/homevault.json" job-backup synthetic
"$HV" --config "$LAB/homevault.json" status synthetic
"$HV" --config "$LAB/homevault.json" snapshots synthetic > "$LAB_PARENT/snapshots.json"

SNAPSHOT_ID="$(python3 - "$LAB_PARENT/snapshots.json" <<'PY'
import json, re, sys
snapshots = json.load(open(sys.argv[1], encoding="utf-8"))
assert len(snapshots) == 1, "Expected exactly one snapshot in the new demo"
identity = snapshots[0]["id"]
assert re.fullmatch(r"[0-9a-f]{64}", identity), "Expected a full snapshot ID"
print(identity)
PY
)"
"$HV" --config "$LAB/homevault.json" restore synthetic "$SNAPSHOT_ID" "$RESTORE" --confirm

python3 - "$LAB/source/proof.txt" "$RESTORE" <<'PY'
from pathlib import Path
import sys
original, restored = Path(sys.argv[1]), Path(sys.argv[2])
matches = list(restored.rglob("proof.txt"))
assert len(matches) == 1, "Expected exactly one restored proof.txt"
assert matches[0].read_bytes() == original.read_bytes(), "Restored bytes differ"
print("PASS: restored proof.txt matches the generated source byte for byte")
PY
```

The script creates a random sample repository key and a UTF-8 `proof.txt`, records
a backup plus repository metadata check, lists the single snapshot, restores its
full ID into a separate target and compares the restored bytes. Snapshot JSON
contains identifiers, not the repository password; no key contents are printed.
Restic preserves directory structure under the restore target, which is why the
verification locates the restored file recursively.

Expected final output:

```text
PASS: restored proof.txt matches the generated source byte for byte
```

Repository checks and intermediate tool output may vary by version. If the final
line is missing or any command fails, the walkthrough did not pass. This proves
local sample-file recovery, not cloud reachability or application recovery.

## 3. Explore the terminal interface

In the same shell after pasting the block above:

```sh
"$HV" --config "$LAB/homevault.json" tui
```

If you ran the script instead, use the directory it printed:

```sh
./bin/homevault --config /printed/demo/directory/lab/homevault.json tui
```

| Key | Result |
| --- | --- |
| Up / Down | Select project |
| `h` | Read recorded job history |
| `s` | List snapshots |
| `b` | Open backup confirmation; Enter starts, another key cancels |
| `c` | Check repository metadata |
| `p` | Preview retention; no deletion |
| `d` | Inspect local Drive credential state; not a live connection test |
| `g` / `?` | Recovery guidance / getting started |
| Page Up / Page Down | Scroll details |
| `q` | Quit; active interactive work is cancelled and cleaned up first |

A second demo backup is optional and adds another snapshot. The walkthrough's
single-snapshot assertion is intended for a brand-new sandbox, not a repeat run
against an existing repository.

## Troubleshooting

| Symptom | Next action |
| --- | --- |
| `restic executable not found` | Install Restic and check `command -v restic` in this shell |
| `bin/homevault` missing | Run the build command from the repository root |
| Directory already exists | Use a new sandbox; do not initialize over an existing repository |
| Restore target rejected | Use a fresh target outside source, state, key and repository paths |
| A lock remains after interruption | Check for active HomeVault/Restic processes before following lock recovery guidance; do not blindly delete locks |
| A command failed | Keep the sandbox, inspect the error and retry in a new sandbox after fixing the cause |

The demo is intentionally retained at the printed path, including its sample key.
No cleanup command runs automatically. Inspect and remove that specific demo
folder manually when finished. Never reuse its sample configuration for real data.

## Next: a real project

Continue with [onboarding](ONBOARDING.md), then [Drive OAuth](DRIVE-OAUTH.md) if using
Google Drive. Keep the original repository key independently recoverable before
uploading real data. Use [database preparation](DATABASE-BACKUPS.md) for databases,
and validate an isolated application restore before scheduling anything.
