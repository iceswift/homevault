import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('prepare', Path(__file__).with_name('prepare-recovery-files.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RecoveryFilesTest(unittest.TestCase):
    def test_changed_source_fails_and_removes_new_stage(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            root.chmod(0o700)
            source = root / 'source'
            source.write_bytes(b'original')
            real_fsync = os.fsync
            def change_source(fd):
                source.write_bytes(b'changed-and-longer')
                real_fsync(fd)
            with patch.object(module.os, 'fsync', side_effect=change_source):
                with self.assertRaises(ValueError):
                    module.prepare({'version': 1, 'files': [{'source': str(source), 'target': 'file'}]}, str(root / 'out'))
            self.assertFalse((root / 'out').exists())

    def test_copy_manifest_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            root.chmod(0o700)
            source = root / 'source'
            source.write_bytes(b'fixture deployment\n')
            plan = {'version': 1, 'files': [{'source': str(source), 'target': 'app/compose.yml'}]}
            dest = root / 'bundle'
            self.assertEqual(module.prepare(plan, str(dest)), 1)
            self.assertEqual((dest / 'app/compose.yml').read_bytes(), source.read_bytes())
            manifest = json.loads((dest / 'manifest.json').read_text())
            self.assertEqual(len(manifest['files'][0]['sha256']), 64)
            with self.assertRaises(FileExistsError):
                module.prepare(plan, str(dest))

    def test_traversal_and_collisions_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            root.chmod(0o700)
            source = root / 'source'
            source.write_text('fixture')
            for target in ['../escape', '/absolute', 'a/../b', 'a\\b', 'manifest.json', 'a//b']:
                with self.assertRaises(ValueError):
                    module.prepare({'version': 1, 'files': [{'source': str(source), 'target': target}]}, str(root / 'out'))
            collision = {'version': 1, 'files': [{'source': str(source), 'target': p} for p in ['a', 'a/b']]}
            with self.assertRaises(OSError):
                module.prepare(collision, str(root / 'out'))
            self.assertFalse((root / 'out').exists())

    @unittest.skipIf(os.name == 'nt', 'Linux source permissions/symlink contract')
    def test_symlink_and_shared_parent_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            root.chmod(0o700)
            source = root / 'source'
            source.write_text('fixture')
            alias = root / 'alias'
            alias.symlink_to(source)
            plan = {'version': 1, 'files': [{'source': str(alias), 'target': 'file'}]}
            with self.assertRaises(ValueError):
                module.prepare(plan, str(root / 'out'))
            plan['files'][0]['source'] = str(source)
            root.chmod(0o755)
            with self.assertRaises(ValueError):
                module.prepare(plan, str(root / 'out'))


if __name__ == '__main__':
    unittest.main()
