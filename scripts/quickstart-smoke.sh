#!/usr/bin/env bash
# Generated data only. Keep the sandbox for inspection; never remove it automatically.
set -euo pipefail
umask 077

HV="$(pwd)/bin/homevault"
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
