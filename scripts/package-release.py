"""Cross-build source-only Linux packages into a new directory, with SHA256SUMS."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]
DOCS = ['QUICKSTART.md', 'ONBOARDING.md', 'DATABASE-BACKUPS.md', 'FRESH-CAPTURE.md',
        'RECOVERY-REHEARSAL.md', 'POSTGRES-RECOVERY.md', 'SCHEDULING-ALERTS.md',
        'RETENTION.md', 'PRUNE.md', 'DRIVE-OAUTH.md', 'GO-OAUTH-LICENSE.txt',
        'RECOVERY-KIT.md', 'TESTING.md', 'RELEASING.md', 'VM-RECOVERY-DRILL.md',
        'OPERATOR-VALIDATION.md', 'PLANNED-WORKFLOWS.md', 'REBUILD.md', 'ROADMAP.md',
        'RECOVERY-FILES.md', 'assets/README.md', 'assets/tui-welcome.txt', 'assets/tui-welcome.png']


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
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    dirty = subprocess.run(['git', 'diff', '--quiet', 'HEAD', '--'], cwd=ROOT).returncode != 0
    if args.version.startswith('v'):
        tagged = subprocess.check_output(['git', 'rev-list', '-n', '1', args.version], cwd=ROOT, text=True).strip()
        if dirty or tagged != commit:
            raise SystemExit('tagged packages require clean tracked source at the exact tag commit')
    go_version = subprocess.check_output(['go', 'version'], text=True).strip()
    # Explicit allowlist: never archive working directories, credentials or data.
    files = {name: (ROOT / name).read_bytes() for name in ['LICENSE', 'README.md', 'CONTRIBUTING.md', 'SECURITY.md']}
    for doc in DOCS:
        files['docs/' + doc] = (ROOT / 'docs' / doc).read_bytes()
    files['scripts/quickstart-smoke.sh'] = (ROOT / 'scripts/quickstart-smoke.sh').read_bytes()
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
                    metadata = {
                        'schema': 1, 'version': args.version, 'source_commit': commit,
                        'tracked_source_dirty': dirty, 'go_toolchain': go_version,
                        'os': 'linux', 'arch': arch, 'cgo_enabled': False,
                        'external_tools': {
                            'restic': '0.19.1 tested in generated-data CI',
                            'rclone': '1.75.2 operator-tested; optional for cloud',
                            'python': '3.11+ for optional workflows and this demo',
                            'age': '1.3.2 tested; optional for recovery kits'},
                        'runtime_validation_target': 'Linux amd64 generated-data local restore; inspect matching CI run' if arch == 'amd64' else 'cross-build only; runtime unverified'
                    }
                    add(tar, name + '/RELEASE.json', (json.dumps(metadata, indent=2) + '\n').encode())
                    for file, data in sorted(files.items()):
                        add(tar, name + '/' + file, data)
        binary.unlink()
        checksums.append(hashlib.sha256(archive.read_bytes()).hexdigest() + '  ' + archive.name)
    (args.output / 'SHA256SUMS').write_text('\n'.join(checksums) + '\n', encoding='ascii')
    print('PASS Linux amd64/arm64 packages and checksums built; no upload or installation')

if __name__ == '__main__':
    main()
