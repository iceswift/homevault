import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('images', Path(__file__).with_name('verify-image-archive.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ArchiveTest(unittest.TestCase):
    def test_pins_and_corrupted_blob(self):
        blob = b'{"fixture":true}'
        digest = hashlib.sha256(blob).hexdigest()
        images = {'demo:fixture': {'id': 'sha256:'+digest}}
        index = json.dumps({'schemaVersion': 2, 'manifests': [{'digest':'sha256:'+digest,
            'annotations': {'io.containerd.image.name':'docker.io/library/demo:fixture'}}]}).encode()
        with tempfile.TemporaryDirectory() as tmp:
            for corrupt in [False, True]:
                p = Path(tmp)/str(corrupt)
                with tarfile.open(p, 'w:gz') as t:
                    for name, data in [('index.json',index), ('blobs/sha256/'+digest, b'bad' if corrupt else blob)]:
                        m=tarfile.TarInfo(name);m.size=len(data);t.addfile(m,io.BytesIO(data))
                if corrupt:
                    with self.assertRaises(ValueError):module.verify(p,images)
                else:
                    self.assertEqual(module.verify(p,images),(1,1))
                    with self.assertRaises(ValueError):module.verify(p,{'demo:fixture':{'id':'sha256:'+'a'*64}})


if __name__ == '__main__':
    unittest.main()
