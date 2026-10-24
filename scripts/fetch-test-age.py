"""Fetch official pinned age tools into a NEW private Linux amd64 lab directory."""
import hashlib
import io
import os
from pathlib import Path
import sys
import tarfile
import urllib.request

VERSION = '1.3.2'
SHA256 = 'cbe24006683f8eb669266162894b9a522a1af52f2665fbc63a4bb032ed26ac10'

def main():
    if len(sys.argv) != 2:
        raise SystemExit('usage: fetch-test-age.py NEW_ABSOLUTE_DIRECTORY')
    target = Path(sys.argv[1])
    if not target.is_absolute():
        raise SystemExit('target must be absolute')
    target.mkdir(mode=0o700)
    url = f'https://github.com/FiloSottile/age/releases/download/v{VERSION}/age-v{VERSION}-linux-amd64.tar.gz'
    with urllib.request.urlopen(url, timeout=60) as response:
        data = response.read(32 * 1024 * 1024 + 1)
    if len(data) > 32 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != SHA256:
        raise SystemExit('age archive checksum/size mismatch')
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:gz') as archive:
        for name in ['age', 'age-keygen']:
            member = archive.getmember('age/' + name)
            if not member.isfile() or member.size > 32 * 1024 * 1024:
                raise SystemExit('unexpected age fixture member')
            with archive.extractfile(member) as source, (target / name).open('xb') as out:
                out.write(source.read())
            os.chmod(target / name, 0o700)
    print('PASS official pinned age fixture checksum; no system installation')

if __name__ == '__main__':
    main()
