"""Generated-data recovery in a fresh network-none container; never production."""
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import time
import urllib.request

def command(*args, fail=False):
    result = subprocess.run(['/tools/homevault', *args], capture_output=True, timeout=120)
    if (result.returncode != 0) != fail:
        raise RuntimeError('fixture recovery command failed; output withheld')
    return result.stdout

info = json.loads(command('kit-open', '/kit/fixture.age', '/data/recovered',
                          '--identity-file', '/custody/identity.txt',
                          '--repository', '/repository', '--restic-binary', '/tools/restic',
                          '--age-binary', '/tools/age', '--confirm'))
config = Path('/data/recovered/homevault.json')
assert json.loads(config.read_text())['recovery_only']
assert info['has_plan'] and Path('/data/recovered/recovery-plan.json').is_file()
assert not Path(os.environ['ORIGINAL_PATH']).exists()
def action(*args, **kwargs):
    return command('--config', str(config), *args, **kwargs)
action('doctor')
for mutation in [('init',), ('backup', 'fixture'), ('job-backup', 'fixture')]:
    action(*mutation, fail=True)
snapshots = json.loads(action('snapshots', 'fixture'))
assert len(snapshots) == 1 and snapshots[0]['id'] == info['snapshot']
action('check')
action('restore', 'fixture', info['snapshot'], '/data/restored', '--confirm')
restored = Path('/data/restored')
apps = list(restored.rglob('app.py'))
databases = list(restored.rglob('main.sqlite'))
assert len(apps) == len(databases) == 1
assert hashlib.sha256(apps[0].read_bytes()).hexdigest() == os.environ['APP_SHA256']
Path('/data/app').symlink_to(apps[0].parent, target_is_directory=True)
Path('/data/db').symlink_to(databases[0].parent, target_is_directory=True)
with sqlite3.connect('file:/data/db/main.sqlite?mode=ro', uri=True) as db:
    assert db.execute('SELECT value FROM proof').fetchone() == ('verified',)
app = subprocess.Popen(['python', '-I', '/data/app/app.py'],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    deadline = time.monotonic() + 15
    while True:
        try:
            with urllib.request.urlopen('http://127.0.0.1:8000/health', timeout=2) as response:
                assert json.load(response)['status'] == 'ok'
            break
        except Exception:
            if time.monotonic() >= deadline:
                raise RuntimeError('restored fixture health failed')
            time.sleep(0.2)
    with urllib.request.urlopen('http://127.0.0.1:8000/media', timeout=2) as response:
        assert response.read() == b'restored-media'
    with urllib.request.urlopen('http://127.0.0.1:8000/login', data=b'fixture-only', timeout=2) as response:
        assert json.load(response)['authenticated']
finally:
    app.terminate()
    try:
        app.wait(timeout=5)
    except subprocess.TimeoutExpired:
        app.kill()
        app.wait(timeout=5)
print(json.dumps({'original_absent': True, 'recovery_only': True,
                  'health': True, 'database': True, 'media': True, 'login': True,
                  'kit_sha256': info['kit_sha256']}))
