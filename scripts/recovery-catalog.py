#!/usr/bin/env python3
"""Create recovery-only project configs from an opened kit and reviewed catalog.
No network, installation, restore, image load or application start is performed.
"""
import json
import os
from pathlib import Path
import re
import shutil
import sys


def prepare(catalog, base, destination):
    if catalog.get('version') != 1 or not base.get('recovery_only'):
        raise ValueError('version 1 catalog and recovery-only kit config required')
    if not base.get('repository') or catalog.get('repository') != base['repository']:
        raise ValueError('catalog and opened kit repositories differ')
    rows = catalog.get('projects')
    if not isinstance(rows, list) or not rows:
        raise ValueError('projects required')
    names = set()
    for row in rows:
        name = row.get('name', '')
        if not re.fullmatch(r'[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}', name) or name in names:
            raise ValueError('invalid or duplicate project name')
        names.add(name)
        if not re.fullmatch(r'[0-9a-f]{64}', row.get('snapshot', '')):
            raise ValueError('full snapshot ID required')
    root = Path(destination)
    if not root.is_absolute() or '..' in root.parts:
        raise ValueError('absolute destination required')
    for p in (root.parent, *root.parent.parents):
        if p.is_symlink():
            raise ValueError('symlink destination parent refused')
    if os.name != 'nt' and root.parent.stat().st_mode & 0o077:
        raise ValueError('private parent required')
    root.mkdir(mode=0o700)
    try:
        for row in rows:
            name = row['name']
            # Copy only tool/credential references from the trusted opened kit.
            cfg = {k: base[k] for k in ['version', 'repository', 'password_file',
                   'restic_binary', 'rclone_binary', 'rclone_config_file'] if k in base}
            cfg.update(recovery_only=True, state_dir=str(root / name / 'state'),
                       projects=[{'name': name, 'paths': [str(root / name / 'unused-source')]}])
            project = root / name
            project.mkdir(mode=0o700)
            for filename, data in [('homevault.json', cfg), ('selection.json', row)]:
                with os.fdopen(os.open(project / filename, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), 'w') as f:
                    json.dump(data, f, indent=2)
        return len(rows)
    except BaseException:
        shutil.rmtree(root)
        raise


if __name__ == '__main__':
    try:
        if len(sys.argv) != 4:
            raise ValueError('CATALOG.json OPENED_KIT_CONFIG.json NEW_DIRECTORY required')
        with open(sys.argv[1]) as f:
            catalog = json.load(f)
        with open(sys.argv[2]) as f:
            base = json.load(f)
        count = prepare(catalog, base, sys.argv[3])
        print(f'PASS prepared {count} recovery-only selections; no commands executed')
    except Exception:
        print('STOP recovery catalog preparation failed; private values withheld', file=sys.stderr)
        sys.exit(1)
