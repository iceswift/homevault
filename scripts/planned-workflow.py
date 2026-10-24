#!/usr/bin/env python3
"""Linux operator coordinator. Explicit activation, no timer installation.

Uses HomeVault for encrypted backup/check and redacted Discord delivery. Plans
are trusted operator configuration, not a sandbox for untrusted containers/code.
"""
import argparse
from contextlib import contextmanager
from datetime import datetime, timedelta, timezone
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import secrets
import shutil
import signal
import stat
import subprocess
import sys
import tarfile
import threading
import time
from zoneinfo import ZoneInfo


class Stop(Exception):
    pass


def error_code(error):
    value = str(error) if isinstance(error, Stop) else ''
    return value if re.fullmatch(r'[a-z0-9-]{1,80}', value) else 'unexpected-error'


def now():
    return datetime.now(timezone.utc)


def stamp():
    return now().isoformat()


def parsed(value):
    d = datetime.fromisoformat(value)
    if d.tzinfo is None:
        raise Stop('invalid-time')
    return d.astimezone(timezone.utc)


def digest(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def private_dir(path):
    path = Path(path)
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    s = path.lstat()
    if not stat.S_ISDIR(s.st_mode) or s.st_mode & 0o077:
        raise Stop('private-directory-required')
    return path


def save(path, data):
    path = Path(path)
    private_dir(path.parent)
    tmp = path.parent / ('.pending-' + secrets.token_hex(12))
    try:
        with os.fdopen(os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
            json.dump(data, f, indent=2)
            f.flush()
            os.fsync(f.fileno())
        if path.is_symlink():
            raise Stop('unsafe-state')
        os.replace(tmp, path)
        fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        tmp.unlink(missing_ok=True)


def read(path):
    path = Path(path)
    s = path.lstat()
    if not stat.S_ISREG(s.st_mode) or s.st_size > 1024 * 1024:
        raise Stop('invalid-state')
    return json.loads(path.read_text())


@contextmanager
def locked(path):
    import fcntl
    path = Path(path)
    fd = os.open(path, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        s = os.fstat(fd)
        if not stat.S_ISREG(s.st_mode) or s.st_mode & 0o077 or s.st_uid != os.getuid():
            raise Stop('unsafe-lock')
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise Stop('source-or-workflow-busy') from None
        yield
    finally:
        os.close(fd)


def command(args, timeout=60, output=False, limit=8 * 1024 * 1024):
    proc = None
    data = bytearray()
    overflow = threading.Event()
    try:
        proc = subprocess.Popen([str(x) for x in args], stdout=subprocess.PIPE if output else subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, start_new_session=True)
        def receive():
            while True:
                block = proc.stdout.read(64 * 1024)
                if not block:
                    break
                if len(data) + len(block) > limit:
                    overflow.set()
                    try:
                        os.killpg(proc.pid, signal.SIGTERM)
                    except ProcessLookupError:
                        pass
                    break
                data.extend(block)
        reader = threading.Thread(target=receive, daemon=True) if output else None
        if reader:
            reader.start()
        proc.wait(timeout=timeout)
        if reader:
            reader.join(timeout=5)
            if reader.is_alive():
                raise Stop('child-output-did-not-close')
        if proc.returncode or overflow.is_set():
            raise Stop('child-failed-or-output-limit')
        return bytes(data) if output else None
    except (OSError, subprocess.TimeoutExpired):
        raise Stop('child-failed-or-timed-out') from None
    finally:
        if proc is not None:
            try:
                os.killpg(proc.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait()
            if output:
                proc.stdout.close()


def safe_name(value):
    return isinstance(value, str) and re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,63}', value)


def absolute(value):
    return isinstance(value, str) and Path(value).is_absolute() and '..' not in Path(value).parts and not any(c in value for c in '\x00\r\n')


def window_open(window, at=None):
    if not window:
        return False
    local = (at or now()).astimezone(ZoneInfo(window['timezone']))
    start = datetime.strptime(window['start'], '%H:%M')
    delta = (local.hour * 60 + local.minute - start.hour * 60 - start.minute) % 1440
    return 1 <= window['minutes'] <= 120 and delta < window['minutes']


def health(state, workflow, at=None):
    at = at or now()
    if not workflow['enabled']:
        return 'not-activated'
    latest = state.get('latest', {})
    if latest.get('result') == 'failed':
        return 'failed'
    if latest.get('result') == 'running':
        started = parsed(latest['started'])
        if started > at + timedelta(minutes=5):
            raise Stop('future-status-time')
        if at - started > timedelta(hours=2):
            return 'failed'
        if not state.get('last_success'):
            return 'running'
    if not state.get('last_success') or not state.get('captured_at'):
        return 'never-completed'
    captured = parsed(state['captured_at'])
    uploaded = parsed(state['last_success'])
    if max(captured, uploaded) > at + timedelta(minutes=5):
        raise Stop('future-status-time')
    if at - min(captured, uploaded) > timedelta(hours=workflow['max_age_hours']):
        return 'overdue'
    if state.get('latest', {}).get('result') == 'running':
        started = parsed(state['latest']['started'])
        return 'failed' if at - started > timedelta(hours=2) else 'running'
    return 'healthy'


def alert_event(current, previous):
    if current in ['not-activated', 'running']:
        return None
    event = 'stale' if current in ['overdue', 'never-completed'] else 'failed' if current == 'failed' else None
    if current == 'healthy' and previous in ['failed', 'stale']:
        return 'recovered'
    return None if event == previous else event


class Workflow:
    def __init__(self, plan_path, runner=command):
        self.plan = read(plan_path)
        p = self.plan
        if set(p) != {'version', 'root', 'base_config', 'homevault_binary', 'notifications_enabled', 'workflows'} or p['version'] != 1:
            raise Stop('invalid-plan')
        if not all(absolute(p[k]) for k in ['root', 'base_config', 'homevault_binary']) or type(p['notifications_enabled']) is not bool:
            raise Stop('invalid-plan-paths')
        self.root = Path(p['root'])
        self.state_dir = private_dir(self.root / 'state')
        self.stages = private_dir(self.root / 'stages')
        self.guards = private_dir(self.state_dir / 'guards')
        self.runner = runner
        self.base = read(p['base_config'])
        if self.base.get('recovery_only'):
            raise Stop('recovery-only-base-refused')
        self.repo_id = hashlib.sha256((self.base['repository'] + '\0' + self.base.get('rclone_config_file', '')).encode()).hexdigest()
        self.workflows = {}
        projects = set()
        common = {'name', 'project', 'kind', 'enabled', 'max_age_hours'}
        kinds = {'sqlite': {'capture_plan'}, 'json': {'container_id', 'image_id', 'mount_source', 'mount_target', 'files', 'window', 'max_pause_seconds'},
                 'sealed': {'source_root', 'source_lock', 'deployment_files', 'image_pins'}, 'files': {'files', 'trees'}}
        for w in p['workflows']:
            if w.get('kind') not in kinds or set(w) != common | kinds[w['kind']]:
                raise Stop('invalid-workflow')
            if not safe_name(w['name']) or not safe_name(w['project']) or w['name'] in self.workflows or w['project'] in projects or type(w['enabled']) is not bool or not 1 <= w['max_age_hours'] <= 720:
                raise Stop('invalid-workflow-policy')
            self.workflows[w['name']] = w
            projects.add(w['project'])
        if not self.workflows:
            raise Stop('no-workflows')

    def binding(self, w):
        return hashlib.sha256(json.dumps({'workflow': w, 'repo': self.repo_id}, sort_keys=True).encode()).hexdigest()

    def status(self, w):
        path = self.state_dir / (w['name'] + '.json')
        if not path.exists():
            return {'version': 1, 'binding': self.binding(w), 'latest': {}, 'history': []}
        s = read(path)
        if s.get('version') != 1 or s.get('binding') != self.binding(w):
            raise Stop('status-plan-or-repository-changed')
        return s

    def persist(self, w, s):
        save(self.state_dir / (w['name'] + '.json'), s)

    def cli(self, cfg, *args, output=False, timeout=5500):
        return self.runner([self.plan['homevault_binary'], '--config', cfg, '--timeout', '90m', *args], timeout=timeout, output=output)

    def inspect(self, cid):
        if not re.fullmatch(r'[0-9a-f]{64}', cid):
            raise Stop('unpinned-container')
        fmt = '{"id":{{json .Id}},"image":{{json .Image}},"running":{{json .State.Running}},"started":{{json .State.StartedAt}},"health":{{with index .State "Health"}}{{json .Status}}{{else}}"none"{{end}},"mounts":{{json .Mounts}}}'
        return json.loads(self.runner(['docker', 'inspect', '--format', fmt, cid], output=True))

    def verify_json(self, w):
        d = self.inspect(w['container_id'])
        if d['image'] != w['image_id'] or not d['running'] or d['health'] != 'healthy':
            raise Stop('capture-identity-or-health-changed')
        mounts = [m for m in d['mounts'] if m['Destination'] == w['mount_target']]
        if len(mounts) != 1 or mounts[0]['Type'] != 'bind' or mounts[0]['Source'] != w['mount_source']:
            raise Stop('capture-mount-changed')
        if not w['files'] or any(not safe_name(x) or not x.endswith('.json') for x in w['files']) or not 5 <= w['max_pause_seconds'] <= 30:
            raise Stop('invalid-json-capture')
        window_open(w['window'])  # validate even when disabled/outside window
        return d

    def sealed_source(self, w):
        root = Path(w['source_root'])
        if not absolute(str(root)) or root.is_symlink():
            raise Stop('invalid-sealed-root')
        bundles = sorted(x for x in root.iterdir() if x.is_dir() and not x.is_symlink() and (x / '.saan-scheduled-complete').is_file())
        if not bundles:
            raise Stop('no-completed-source')
        source = bundles[-1]
        if not re.fullmatch(r'\d{8}T\d{6}Z-\d+', source.name):
            raise Stop('invalid-source-name')
        captured = datetime.strptime(source.name.split('-')[0], '%Y%m%dT%H%M%SZ').replace(tzinfo=timezone.utc)
        if captured > now() + timedelta(minutes=5) or now() - captured > timedelta(hours=w['max_age_hours']):
            raise Stop('sealed-source-overdue')
        marker = source / '.saan-scheduled-complete'
        permitted = {'database.dump', 'uploads.tar.gz', 'images.json', 'manifest.txt', 'SHA256SUMS', '.saan-scheduled-complete'}
        if {p.name for p in source.iterdir()} != permitted or any(p.is_symlink() or not p.is_file() for p in source.iterdir()):
            raise Stop('unexpected-sealed-files')
        if marker.read_text() != 'format=1\n':
            raise Stop('invalid-completion-marker')
        expected = {'database.dump', 'uploads.tar.gz', 'images.json', 'manifest.txt'}
        entries = {}
        for line in (source / 'SHA256SUMS').read_text().splitlines():
            parts = line.split()
            if len(parts) != 2 or parts[1].lstrip('*') not in expected or not re.fullmatch('[0-9a-f]{64}', parts[0]):
                raise Stop('invalid-source-checksums')
            name = parts[1].lstrip('*')
            if name in entries or (source / name).is_symlink() or digest(source / name) != parts[0]:
                raise Stop('source-checksum-mismatch')
            entries[name] = parts[0]
        if set(entries) != expected:
            raise Stop('incomplete-sealed-source')
        pins = json.loads((source / 'images.json').read_text())
        for pin in w['image_pins']:
            current = self.inspect(pin['container_id'])
            if current['image'] != pin['image_id'] or not any(p['ID'] == pin['image_id'] and p['ContainerName'] == pin['name'] for p in pins):
                raise Stop('sealed-image-deployment-mismatch')
        for item in w['deployment_files']:
            if Path(item['source']).stat().st_mtime_ns > marker.stat().st_mtime_ns:
                raise Stop('deployment-newer-than-sealed-source')
        with tarfile.open(source / 'uploads.tar.gz', 'r:gz') as t:
            for m in t:
                p = PurePosixPath(m.name)
                if p.is_absolute() or '..' in p.parts or not (m.isfile() or m.isdir()):
                    raise Stop('unsafe-media-archive')
        identity = hashlib.sha256(json.dumps({'source': source.name, 'checksums': entries}, sort_keys=True).encode()).hexdigest()
        return source, captured.isoformat(), identity

    def file_entries(self, w):
        files = list(w['files'])
        links = []
        for tree in w['trees']:
            source = Path(tree['source'])
            resolved = source.resolve(strict=True)
            allowed = Path(tree['allowed_root']).resolve(strict=True)
            if not resolved.is_relative_to(allowed):
                raise Stop('tree-outside-reviewed-root')
            links.append((source, resolved))
            for p in sorted(resolved.rglob('*')):
                if p.is_symlink():
                    raise Stop('nested-tree-symlink')
                if p.is_file():
                    files.append({'source': str(p), 'target': tree['target'] + '/' + str(p.relative_to(resolved))})
        return files, links

    def stage_files(self, entries, dest):
        # Reuse the strict file staging helper, not shell copy commands.
        module_path = Path(__file__).with_name('prepare-recovery-files.py')
        spec = importlib.util.spec_from_file_location('homevault_staging', module_path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        sizes = sum(Path(e['source']).stat().st_size for e in entries)
        if sizes > 5 * 1024 ** 3 or shutil.disk_usage(dest.parent).free < sizes + 1024 ** 3:
            raise Stop('insufficient-staging-budget')
        module.prepare({'version': 1, 'files': entries}, str(dest))

    def preflight(self, w):
        if w['kind'] == 'sqlite':
            self.cli(self.plan['base_config'], 'capture-check', w['project'], w['capture_plan'], timeout=60)
        elif w['kind'] == 'json':
            self.verify_json(w)
        elif w['kind'] == 'sealed':
            with locked(w['source_lock']):
                self.sealed_source(w)
        elif w['kind'] == 'files':
            files, _ = self.file_entries(w)
            if not files or any(not os.access(f['source'], os.R_OK) for f in files):
                raise Stop('unreadable-deployment-files')
        hook = Path(self.base['discord_webhook_file'])
        s = hook.lstat()
        if not stat.S_ISREG(s.st_mode) or s.st_mode & 0o077 or not 0 < s.st_size <= 4096:
            raise Stop('private-webhook-metadata-invalid')
        return {'name': w['name'], 'preflight': 'passed', 'enabled': w['enabled'], 'notifications_enabled': self.plan['notifications_enabled'],
                'capture_window_selected': bool(w.get('window')) if w['kind'] == 'json' else None}

    def json_capture(self, w, stage):
        before = self.verify_json(w)
        guard = self.guards / (w['name'] + '.json')
        if guard.exists():
            raise Stop('existing-guard-requires-review')
        nonce = secrets.token_hex(12)
        deadline = now() + timedelta(seconds=w['max_pause_seconds'])
        save(guard, {'version': 1, 'container_id': w['container_id'], 'image_id': w['image_id'], 'nonce': nonce,
                     'deadline': deadline.isoformat(), 'owner_pid': os.getpid()})
        guardian = subprocess.Popen([self.plan['homevault_binary'], 'pause-guardian', str(guard)], stdin=subprocess.DEVNULL,
                                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        end = time.monotonic() + 3
        while not Path(str(guard) + '.ready').exists():
            if guardian.poll() is not None or time.monotonic() > end:
                raise Stop('guardian-not-ready-no-stop')
            time.sleep(.025)
        if read(str(guard) + '.ready')['nonce'] != nonce:
            raise Stop('guardian-identity-mismatch')
        stage.mkdir(mode=0o700)
        started = time.monotonic()
        try:
            self.runner(['docker', 'stop', '--time', '5', w['container_id']], timeout=8)
            if self.inspect(w['container_id'])['running']:
                raise Stop('container-not-stopped')
            archive = self.runner(['docker', 'cp', w['container_id'] + ':' + w['mount_target'] + '/.', '-'], timeout=5, output=True, limit=2*1024*1024)
            copied = []
            with tarfile.open(fileobj=io.BytesIO(archive), mode='r:') as t:
                for m in t:
                    p = PurePosixPath(m.name)
                    if p.is_absolute() or '..' in p.parts:
                        raise Stop('unsafe-state-archive')
                    if m.isdir() or m.name.endswith('.lock'):
                        continue
                    if not m.isfile() or len(p.parts) != 1 or p.name not in w['files'] or m.size > 1024*1024:
                        raise Stop('unexpected-state-file')
                    content = t.extractfile(m).read()
                    json.loads(content)
                    dest = stage / p.name
                    with os.fdopen(os.open(dest, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'wb') as f:
                        f.write(content)
                    copied.append({'target': p.name, 'sha256': hashlib.sha256(content).hexdigest(), 'bytes': len(content)})
            if {x['target'] for x in copied} != set(w['files']) or now() >= deadline or self.inspect(w['container_id'])['running']:
                raise Stop('capture-incomplete-or-window-expired')
            captured_at = stamp()
        finally:
            self.runner(['docker', 'start', w['container_id']], timeout=10)
            end = time.monotonic() + 15
            while True:
                d = self.inspect(w['container_id'])
                if d['image'] == before['image'] and d['running'] and d['health'] == 'healthy':
                    break
                if time.monotonic() > end:
                    raise Stop('resume-health-failed-guard-retained')
                time.sleep(.25)
            if guard.exists() and read(guard)['nonce'] == nonce:
                guard.unlink()
        save(stage / 'manifest.json', {'version': 1, 'captured_at': captured_at, 'files': copied,
                                      'pause_seconds': round(time.monotonic()-started, 3), 'image_id': before['image']})
        return captured_at, hashlib.sha256((nonce + captured_at).encode()).hexdigest()

    def monitor(self, w, no_send=False):
        s = self.status(w)
        current = health(s, w)
        result = {'name': w['name'], 'health': current, 'notification': 'disabled'}
        if no_send or not self.plan['notifications_enabled'] or not w['enabled']:
            return result
        with locked(self.state_dir / (w['name'] + '-alert.lock')):
            path = self.state_dir / (w['name'] + '-alert.json')
            previous = read(path) if path.exists() else {}
            if previous and previous.get('binding') != self.binding(w):
                raise Stop('alert-policy-changed')
            event = alert_event(current, previous.get('event'))
            if not event:
                result['notification'] = 'deduplicated-or-not-needed'
                return result
            cfg = dict(self.base)
            cfg['projects'] = [{'name': w['project'], 'paths': [str(self.stages / 'notification-unused-source')]}]
            config = self.state_dir / (w['name'] + '-notification-config.json')
            save(config, cfg)
            self.cli(config, 'notify-event', w['project'], event, '--confirm', timeout=20)
            save(path, {'binding': self.binding(w), 'event': event, 'sent_at': stamp()})
            result['notification'] = 'sent'
        return result

    def run(self, w):
        if not w['enabled']:
            return {'name': w['name'], 'result': 'not-activated', 'application_stopped': False}
        if w['kind'] == 'json' and not window_open(w['window']):
            return {'name': w['name'], 'result': 'outside-maintenance-window', 'application_stopped': False}
        with locked(self.state_dir / 'workflow.lock'):
            s = self.status(w)
            attempt = {'id': secrets.token_hex(12), 'started': stamp(), 'result': 'running', 'phase': 'preflight'}
            s['latest'] = attempt
            self.persist(w, s)
            try:
                self.preflight(w)
                run_root = self.stages / (w['name'] + '-' + attempt['id'])
                run_root.mkdir(mode=0o700)
                stage = run_root / 'bundle'
                cfg = dict(self.base)
                cfg.pop('discord_webhook_file', None)
                cfg['notify_success'] = False
                cfgpath = run_root / 'homevault.json'
                if w['kind'] == 'sqlite':
                    save(cfgpath, cfg)
                    old = json.loads(self.cli(cfgpath, 'status', w['project'], output=True))['latest'].get('id')
                    self.cli(cfgpath, 'capture-backup', w['project'], w['capture_plan'], '--confirm')
                    job = json.loads(self.cli(cfgpath, 'status', w['project'], output=True))
                    if job['latest'].get('id') == old:
                        s['latest'] = {'id': attempt['id'], 'started': attempt['started'], 'result': 'skipped', 'phase': 'maintenance-window'}
                        self.persist(w, s)
                        return {'name': w['name'], 'result': 'outside-maintenance-window', 'application_stopped': False}
                    if not job['latest'].get('capture_passed'):
                        raise Stop('fresh-capture-not-verified')
                    captured, source_id = job['latest']['started'], job['latest']['id']
                else:
                    attempt['phase'] = 'capture-or-stage'
                    self.persist(w, s)
                    if w['kind'] == 'json':
                        captured, source_id = self.json_capture(w, stage)
                    elif w['kind'] == 'sealed':
                        with locked(w['source_lock']):
                            source, captured, source_id = self.sealed_source(w)
                            if s.get('source_id') == source_id:
                                s['latest'] = {'id': attempt['id'], 'started': attempt['started'], 'result': 'skipped', 'phase': 'already-uploaded'}
                                self.persist(w, s)
                                return {'name': w['name'], 'result': 'already-uploaded-no-freshness-advance'}
                            entries = [{'source': str(p), 'target': 'backup/' + p.name} for p in source.iterdir() if p.is_file()]
                            entries += w['deployment_files']
                            self.stage_files(entries, stage)
                            self.sealed_source(w)  # source/config remains compatible under lock
                    else:
                        entries, links = self.file_entries(w)
                        self.stage_files(entries, stage)
                        if any(src.resolve(strict=True) != target for src, target in links):
                            raise Stop('release-link-changed')
                        captured, source_id = stamp(), digest(stage / 'manifest.json')
                    if (stage / 'capture-info.json').exists():
                        raise Stop('reserved-capture-metadata-path')
                    save(stage / 'capture-info.json', {'version': 1, 'captured_at': captured, 'source_id': source_id,
                                                       'adapter': w['kind'], 'consistent_across_applications': False})
                    cfg['projects'] = [{'name': w['project'], 'paths': [str(stage)]}]
                    save(cfgpath, cfg)
                    attempt['phase'] = 'backup-check'
                    self.persist(w, s)
                    self.cli(cfgpath, 'job-backup', w['project'])
                    job = json.loads(self.cli(cfgpath, 'status', w['project'], output=True))
                j = job['latest']
                if j['status'] != 'success' or not j['backup_passed'] or not j['check_passed'] or parsed(j['started']) < parsed(attempt['started']):
                    raise Stop('backup-check-not-verified')
                attempt.update(result='success', phase='completed', finished=stamp(), captured_at=captured,
                               capture_verified=True, upload_verified=True, check_verified=True)
                s.update(last_success=job['last_success'], captured_at=captured, source_id=source_id, last_job_id=j['id'])
            except BaseException as error:
                attempt.update(result='failed', phase='cancelled' if isinstance(error, (KeyboardInterrupt, SystemExit)) else 'failed', finished=stamp(), error_code=error_code(error))
                s['history'] = (s.get('history', []) + [dict(attempt)])[-32:]
                self.persist(w, s)
                raise Stop('workflow-failed-previous-success-preserved') from None
            s['history'] = (s.get('history', []) + [dict(attempt)])[-32:]
            self.persist(w, s)
        return {'name': w['name'], 'result': 'success'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--plan', required=True)
    parser.add_argument('command', choices=['preflight', 'run', 'monitor'])
    parser.add_argument('name', nargs='?', default='all')
    parser.add_argument('--confirm', action='store_true')
    parser.add_argument('--no-send', action='store_true')
    args = parser.parse_args()
    if args.command == 'run' and not args.confirm:
        raise Stop('run-requires-confirmation')
    if os.name != 'posix':
        raise Stop('linux-required')
    os.umask(0o077)
    suite = Workflow(args.plan)
    selected = list(suite.workflows.values()) if args.name == 'all' else [suite.workflows[args.name]]
    failed = False
    for w in selected:
        try:
            if args.command == 'preflight':
                result = suite.preflight(w)
            elif args.command == 'monitor':
                result = suite.monitor(w, args.no_send)
                failed |= result['health'] not in ['healthy', 'running', 'not-activated']
            else:
                try:
                    result = suite.run(w)
                except Stop:
                    # Includes preparation/capture failures before Go's job begins.
                    suite.monitor(w, args.no_send)
                    raise
                else:
                    # A delivery error here is not retried inline and never
                    # changes the already-persisted backup success.
                    result['monitor'] = suite.monitor(w, args.no_send)
            print(json.dumps(result), flush=True)
        except Exception as error:
            print(json.dumps({'name': w['name'], 'result': 'failed', 'code': error_code(error), 'details': 'private errors withheld'}), flush=True)
            failed = True
    return 1 if failed else 0


if __name__ == '__main__':
    signal.signal(signal.SIGTERM, lambda *_: (_ for _ in ()).throw(KeyboardInterrupt()))
    try:
        sys.exit(main())
    except BaseException as error:
        if isinstance(error, SystemExit):
            raise
        print('STOP workflow incomplete; credentials/child output withheld', file=sys.stderr)
        sys.exit(1)
