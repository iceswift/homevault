#!/usr/bin/env python3
"""Verify an OCI Docker-save gzip archive against reviewed tag -> image ID pins.
Streams every blob; does not load images or display config/environment contents.
"""
import gzip
import hashlib
import json
import re
import tarfile
from pathlib import PurePosixPath
import sys


def verify(archive, images):
    blobs, names, index = set(), set(), None
    with tarfile.open(archive, 'r|gz') as tar:
        for member in tar:
            p = PurePosixPath(member.name)
            if p.is_absolute() or '..' in p.parts or member.name in names:
                raise ValueError('unsafe or duplicate archive entry')
            names.add(member.name)
            if member.isdir():
                continue
            if not member.isfile():
                raise ValueError('special archive entry refused')
            if member.name == 'index.json':
                if member.size > 1024*1024:
                    raise ValueError('oversized OCI index')
                index = json.load(tar.extractfile(member))
            elif member.name.startswith('blobs/'):
                if not re.fullmatch(r'blobs/sha256/[0-9a-f]{64}', member.name):
                    raise ValueError('unsupported blob digest')
                digest = hashlib.sha256()
                f = tar.extractfile(member)
                for chunk in iter(lambda: f.read(1024*1024), b''):
                    digest.update(chunk)
                expected = member.name.split('/')[-1]
                if digest.hexdigest() != expected:
                    raise ValueError('blob hash mismatch')
                blobs.add('sha256:' + expected)
    # tar readers may stop before the gzip footer; consume the full stream to
    # verify its CRC/length too, without retaining image contents in memory.
    with gzip.open(archive, 'rb') as f:
        for _ in iter(lambda: f.read(1024*1024), b''):
            pass
    if not index or index.get('schemaVersion') != 2:
        raise ValueError('OCI index required')
    descriptors = {d.get('annotations', {}).get('io.containerd.image.name'): d.get('digest')
                   for d in index.get('manifests', [])}
    for ref, info in images.items():
        full = ref if ref.startswith('docker.io/') else 'docker.io/' + (ref if '/' in ref else 'library/' + ref)
        if descriptors.get(full) != info['id'] or info['id'] not in blobs:
            raise ValueError('pinned image index missing or different')
    return len(images), len(blobs)


if __name__ == '__main__':
    try:
        with open(sys.argv[2]) as f:
            images = json.load(f)['images']
        refs, blobs = verify(sys.argv[1], images)
        print(f'PASS {refs} OCI image pins and {blobs} blob hashes; no images loaded')
    except Exception:
        print('STOP OCI archive verification failed; private values withheld', file=sys.stderr)
        sys.exit(1)
