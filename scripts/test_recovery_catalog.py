import importlib.util
from pathlib import Path
import json
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('catalog', Path(__file__).with_name('recovery-catalog.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class CatalogTest(unittest.TestCase):
    def test_recovery_only_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            root.chmod(0o700)
            catalog = {'version': 1, 'repository':'/repo', 'projects': [{'name': 'app', 'snapshot': 'a'*64}]}
            base = {'version': 1, 'recovery_only': True, 'repository': '/repo',
                    'password_file': '/private/key', 'discord_webhook_file': '/excluded'}
            out = root / 'out'
            self.assertEqual(module.prepare(catalog, base, str(out)), 1)
            cfg = json.loads((out/'app/homevault.json').read_text())
            self.assertTrue(cfg['recovery_only'])
            self.assertNotIn('discord_webhook_file', cfg)
            self.assertEqual(cfg['projects'][0]['name'], 'app')
            with self.assertRaises(FileExistsError):
                module.prepare(catalog, base, str(out))
            with self.assertRaises(ValueError):
                module.prepare(dict(catalog, repository='/other'), base, str(root/'wrong-repo'))

    def test_unsafe_selections_fail_before_output(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            root.chmod(0o700)
            for row in [{'name': '../bad', 'snapshot': 'a'*64}, {'name':'app', 'snapshot':'short'}]:
                with self.assertRaises(ValueError):
                    module.prepare({'version':1, 'repository':'/repo', 'projects':[row]}, {'recovery_only':True,'repository':'/repo'}, str(root/'out'))
            self.assertFalse((root/'out').exists())
            with self.assertRaises(ValueError):
                module.prepare({'version':1,'projects':[{'name':'app','snapshot':'a'*64}]}, {}, str(root/'out'))


if __name__ == '__main__':
    unittest.main()
