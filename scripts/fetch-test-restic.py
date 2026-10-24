"""Fetch the pinned official Linux amd64 fixture tool; never install system-wide."""
import bz2
import hashlib
import os
from pathlib import Path
import sys
import urllib.request

VERSION = '0.19.1'
SHA256 = 'f415415624dcc452f2a02b8c33641791a8c6d6d3b65bbb3543fcf9a25151585c'

def main():
    if len(sys.argv) != 2:
        raise SystemExit('usage: fetch-test-restic.py NEW_DIRECTORY')
    target = Path(sys.argv[1])
    if not target.is_absolute():
        raise SystemExit('target must be absolute and new')
    target.mkdir(mode=0o700)
    url = f'https://github.com/restic/restic/releases/download/v{VERSION}/restic_{VERSION}_linux_amd64.bz2'
    with urllib.request.urlopen(url, timeout=60) as response:
        data = response.read(32 * 1024 * 1024 + 1)
    if len(data) > 32 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != SHA256:
        raise SystemExit('official fixture archive checksum/size mismatch')
    with (target / 'restic').open('xb') as out:
        out.write(bz2.decompress(data))
    os.chmod(target / 'restic', 0o700)
    print('PASS pinned Restic fixture archive checksum; tool written in new private directory')

if __name__ == '__main__':
    main()
