"""Syntax-check templates using temporary ExecStart/user stubs, never install/start."""
from pathlib import Path
import getpass
import grp
import os
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
templates = root / 'deploy'
with tempfile.TemporaryDirectory(prefix='homevault-unit-verify-') as tmp:
    paths = []
    for name in ['homevault-backup.service', 'homevault-backup.timer', 'homevault-monitor.service', 'homevault-monitor.timer', 'homevault-capture.service', 'homevault-capture.timer', 'homevault-capture-monitor.service', 'homevault-capture-monitor.timer']:
        lines = (templates / name).read_text().splitlines()
        # This validates syntax/dependencies with existing test executable/user,
        # not installation permissions, source access or a real service run.
        lines = [x.split('=', 1)[0]+'=/usr/bin/true' if x.startswith(('ExecStart=', 'ExecStartPre=', 'ExecStopPost=')) else
                 'User='+getpass.getuser() if x.startswith('User=') else
                 'Group='+grp.getgrgid(os.getgid()).gr_name if x.startswith('Group=') else x for x in lines]
        path = Path(tmp) / name
        path.write_text('\n'.join(lines)+'\n')
        paths.append(str(path))
    result = subprocess.run(['systemd-analyze', 'verify', *paths], capture_output=True, timeout=30)
    if result.returncode:
        raise SystemExit('systemd template verification failed; inspect templates locally (not installed).')
print('PASS systemd template syntax/dependencies with stub executable/user; nothing installed or started')
