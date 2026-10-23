"""Provision private rclone config from a downloaded Desktop OAuth JSON.

Run only on the owner's home server. Never prints credential values.
Existing configuration is never overwritten.
"""
import argparse
import json
import os
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("credentials", type=Path)
parser.add_argument("output", type=Path)
parser.add_argument("--project-id", required=True, help="Expected Google Cloud project ID")
args = parser.parse_args()
try:
    client = json.loads(args.credentials.read_text())["installed"]
    if client.get("project_id") != args.project_id:
        raise ValueError("unexpected Google Cloud project")
    client_id, secret = client["client_id"], client["client_secret"]
    if not client_id.endswith(".apps.googleusercontent.com") or not secret:
        raise ValueError("invalid Desktop client")
    if any(ord(char) < 32 for char in client_id + secret):
        raise ValueError("credential contains control characters")
    os.chmod(args.credentials, 0o600)
    args.output.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(args.output.parent, 0o700)
    contents = f"[gdrive]\ntype = drive\nclient_id = {client_id}\nclient_secret = {secret}\nscope = drive.file\n"
    fd = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as config:
        config.write(contents)
    print("Private Desktop OAuth configuration prepared; Google authorization still required.")
except Exception:
    raise SystemExit("Could not provision credentials (existing output, invalid client/project or file permissions). No credential values printed.")
