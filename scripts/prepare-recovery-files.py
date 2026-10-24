#!/usr/bin/env python3
"""Stage an explicit list of deployment files; never discover or upload secrets.

Usage: prepare-recovery-files.py PLAN.json NEW_DIRECTORY
Plan: {"version":1,"files":[{"source":"/absolute/file","target":"app/compose.yml"}]}
Only regular files with non-symlink ancestors are accepted. No application pause,
database capture, cloud upload or permission escalation is performed.
"""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import stat
import sys


def regular_source(source):
    p = Path(source)
    if not p.is_absolute() or '..' in p.parts:
        raise ValueError('source must be an absolute path without traversal')
    for ancestor in (p, *p.parents):
        if ancestor.is_symlink():
            raise ValueError('symlink sources or ancestors are not supported')
    if not stat.S_ISREG(p.stat().st_mode):
        raise ValueError('source must be a regular file')
    return p


def prepare(plan, destination):
    if set(plan) != {'version', 'files'} or plan['version'] != 1:
        raise ValueError('unsupported plan')
    if not isinstance(plan['files'], list) or not 1 <= len(plan['files']) <= 10000:
        raise ValueError('expected 1-10000 explicit files')
    target_root = Path(destination)
    if not target_root.is_absolute() or '..' in target_root.parts:
        raise ValueError('destination must be absolute without traversal')
    parent = target_root.parent
    for p in (parent, *parent.parents):
        if p.is_symlink():
            raise ValueError('destination ancestors must not be symlinks')
    if os.name != 'nt' and parent.stat().st_mode & 0o077:
        raise ValueError('destination parent must be private (0700)')
    entries, seen = [], set()
    for entry in plan['files']:
        if set(entry) != {'source', 'target'} or not isinstance(entry['target'], str):
            raise ValueError('invalid file entry')
        name = entry['target']
        rel = PurePosixPath(name)
        if (not name or '\\' in name or ':' in name or any(ord(c) < 32 for c in name)
                or rel.is_absolute() or '..' in rel.parts or str(rel) != name
                or name == 'manifest.json' or name in seen):
            raise ValueError('invalid or duplicate target')
        seen.add(name)
        entries.append((regular_source(entry['source']), name))
    target_root.mkdir(mode=0o700)  # never reuse or overwrite an existing directory
    try:
        manifest = {'version': 1, 'consistency': 'stable individual deployment files; not an atomic application snapshot', 'files': []}
        for source, name in entries:
            out = target_root / name
            out.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
            before = source.stat()
            fd = os.open(source, os.O_RDONLY | getattr(os, 'O_NOFOLLOW', 0))
            with os.fdopen(fd, 'rb') as src:
                opened = os.fstat(src.fileno())
                if (before.st_dev, before.st_ino) != (opened.st_dev, opened.st_ino):
                    raise ValueError('source changed while opening')
                digest = hashlib.sha256()
                with os.fdopen(os.open(out, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'wb') as dst:
                    while True:
                        chunk = src.read(128 * 1024)
                        if not chunk:
                            break
                        digest.update(chunk)
                        dst.write(chunk)
                    dst.flush()
                    os.fsync(dst.fileno())
                after = os.fstat(src.fileno())
            current = source.stat()
            signature = lambda s: (s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns, s.st_ctime_ns)
            if signature(before) != signature(after) or signature(after) != signature(current):
                raise ValueError('source changed during copy')
            manifest['files'].append({'target': name, 'sha256': digest.hexdigest(), 'bytes': after.st_size,
                                      'mode': stat.S_IMODE(after.st_mode), 'uid': after.st_uid, 'gid': after.st_gid})
        with os.fdopen(os.open(target_root / 'manifest.json', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
            json.dump(manifest, f, indent=2)
        return len(entries)
    except BaseException:
        shutil.rmtree(target_root)  # only the new directory owned by this invocation
        raise


if __name__ == '__main__':
    try:
        if len(sys.argv) != 3:
            raise ValueError('expected PLAN.json NEW_DIRECTORY')
        with open(sys.argv[1], encoding='utf-8') as f:
            plan = json.load(f)
        count = prepare(plan, sys.argv[2])
        print(f'PASS staged {count} files; no upload performed')
    except Exception:
        # OS/JSON errors may contain private source names or values.
        print('STOP recovery-file staging failed; no upload performed', file=sys.stderr)
        sys.exit(1)
