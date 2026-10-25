import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("verify_release", Path(__file__).with_name("verify-release.py"))
verify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)
VERSION = "v0.1.0-preview.1"
COMMIT = "a" * 40


class PackageTests(unittest.TestCase):
    def build(self, root, unsafe=False, wrong_metadata=False):
        sums = []
        for arch, machine in [("amd64", 62), ("arm64", 183)]:
            prefix = f"homevault_{VERSION}_linux_{arch}"
            binary = bytearray(20)
            binary[:6] = b"\x7fELF\x02\x01"
            binary[18:20] = machine.to_bytes(2, "little")
            metadata = dict(schema=1, version=VERSION, source_commit=COMMIT, os="linux", arch=arch, cgo_enabled=False, tracked_source_dirty=False)
            if wrong_metadata:
                metadata["source_commit"] = "b" * 40
            files = {"homevault": bytes(binary), "RELEASE.json": json.dumps(metadata).encode(), "README.md": b"readme", "LICENSE": b"MIT", "docs/QUICKSTART.md": b"demo", "scripts/quickstart-smoke.sh": b"demo"}
            if unsafe:
                files["../outside"] = b"bad"
            archive = root / (prefix + ".tar.gz")
            with tarfile.open(archive, "w:gz") as tar:
                for name, data in files.items():
                    info = tarfile.TarInfo(prefix + "/" + name)
                    info.size = len(data)
                    info.mode = 0o755 if name == "homevault" else 0o644
                    tar.addfile(info, io.BytesIO(data))
            sums.append(hashlib.sha256(archive.read_bytes()).hexdigest() + "  " + archive.name)
        (root / "SHA256SUMS").write_text("\n".join(sums) + "\n")

    def test_matching_archives(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.build(root)
            self.assertEqual(set(verify.verify_dist(root, VERSION, COMMIT)), {"amd64", "arm64"})

    def test_checksum_tampering(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.build(root)
            archive = next(root.glob("*.tar.gz"))
            archive.write_bytes(archive.read_bytes() + b"tampered")
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                verify.verify_dist(root, VERSION, COMMIT)

    def test_unsafe_member_even_with_matching_checksum(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.build(root, unsafe=True)
            with self.assertRaisesRegex(ValueError, "unsafe"):
                verify.verify_dist(root, VERSION, COMMIT)

    def test_metadata_source_mismatch(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.build(root, wrong_metadata=True)
            with self.assertRaisesRegex(ValueError, "metadata"):
                verify.verify_dist(root, VERSION, COMMIT)

    def test_duplicate_checksum_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.build(root)
            sums = root / "SHA256SUMS"
            sums.write_text(sums.read_text() + sums.read_text().splitlines()[0] + "\n")
            with self.assertRaisesRegex(ValueError, "checksum entry"):
                verify.verify_dist(root, VERSION, COMMIT)


if __name__ == "__main__":
    unittest.main()
