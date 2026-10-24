"""Cross-build source-only Linux packages into a new directory, with SHA256SUMS."""
import argparse
import gzip
import hashlib
import io
import os
from pathlib import Path
import re
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]
DOCS = ['ONBOARDING.md', 'DATABASE-BACKUPS.md', 'FRESH-CAPTURE.md',
        'RECOVERY-REHEARSAL.md', 'POSTGRES-RECOVERY.md', 'SCHEDULING-ALERTS.md',
        'RETENTION.md', 'TESTING.md']

def add(tar, name, data, mode=0o644):
    info = tarfile.TarInfo(name)
    info.size, info.mode, info.mtime = len(data), mode, 0
    info.uid = info.gid = 0
    tar.addfile(info, io.BytesIO(data))

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'(?:v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?|dev-[0-9a-f]{7,40})', args.version):
        raise SystemExit('version must be a semantic v-tag or dev-commithex')
    if not args.output.is_absolute():
        raise SystemExit('output must be a new absolute directory')
    # Explicit allowlist: never archive working directories, credentials or data.
    files = {'LICENSE': (ROOT / 'LICENSE').read_bytes(), 'README.md': (ROOT / 'README.md').read_bytes()}
    for doc in DOCS:
        files['docs/' + doc] = (ROOT / 'docs' / doc).read_bytes()
    args.output.mkdir(mode=0o700)
    checksums = []
    for arch in ['amd64', 'arm64']:
        binary = args.output / ('homevault-' + arch)
        env = dict(os.environ, GOOS='linux', GOARCH=arch, CGO_ENABLED='0')
        subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false',
                        '-ldflags=-s -w -X main.version=' + args.version,
                        '-o', str(binary), './cmd/homevault'], cwd=ROOT, env=env, check=True)
        name = f'homevault_{args.version}_linux_{arch}'
        archive = args.output / (name + '.tar.gz')
        with archive.open('xb') as out:
            with gzip.GzipFile(filename='', mode='wb', fileobj=out, mtime=0) as zipped:
                with tarfile.open(fileobj=zipped, mode='w') as tar:
                    add(tar, name + '/homevault', binary.read_bytes(), 0o755)
                    for file, data in sorted(files.items()):
                        add(tar, name + '/' + file, data)
        binary.unlink()
        checksums.append(hashlib.sha256(archive.read_bytes()).hexdigest() + '  ' + archive.name)
    (args.output / 'SHA256SUMS').write_text('\n'.join(checksums) + '\n', encoding='ascii')
    print('PASS Linux amd64/arm64 packages and checksums built; no upload or installation')

if __name__ == '__main__':
    main()
