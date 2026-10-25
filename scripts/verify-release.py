"""Validate release archives before extraction; optionally run the packaged amd64 demo."""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import platform
import re
import subprocess
import tarfile
import tempfile
import os


def verify_archive(path, version, arch, commit):
    prefix = f"homevault_{version}_linux_{arch}"
    files = {}
    with tarfile.open(path, "r:gz") as archive:
        total = 0
        for member in archive:
            parts = PurePosixPath(member.name).parts
            if (not member.isfile() or member.name.startswith("/") or
                    ".." in parts or len(parts) < 2 or parts[0] != prefix or
                    member.name != "/".join(parts) or len(files) >= 1024 or
                    member.name in files or member.mode & 0o7000):
                raise ValueError("unsafe or duplicate archive member")
            total += member.size
            if member.size < 0 or total > 64 * 1024 * 1024:
                raise ValueError("archive exceeds size limit")
            relative = "/".join(parts[1:])
            if relative not in {"homevault", "RELEASE.json", "README.md", "LICENSE", "CONTRIBUTING.md", "SECURITY.md", "scripts/quickstart-smoke.sh", "docs/assets/README.md", "docs/assets/tui-welcome.txt", "docs/assets/tui-welcome.png", "docs/assets/homevault-banner.svg"} and not re.fullmatch(r"docs/[A-Z][A-Z0-9-]*\.(md|txt)", relative):
                raise ValueError("unexpected archive content")
            files[member.name] = (archive.extractfile(member).read(), member.mode)
    for name in ["homevault", "RELEASE.json", "README.md", "LICENSE", "docs/QUICKSTART.md", "scripts/quickstart-smoke.sh"]:
        if prefix + "/" + name not in files:
            raise ValueError("required package file missing")
    binary, mode = files[prefix + "/homevault"]
    machine = 62 if arch == "amd64" else 183
    if len(binary) < 20 or binary[:6] != b"\x7fELF\x02\x01" or int.from_bytes(binary[18:20], "little") != machine or mode != 0o755:
        raise ValueError("binary is not an executable ELF for the advertised architecture")
    metadata = json.loads(files[prefix + "/RELEASE.json"][0])
    if any(metadata.get(k) != v for k, v in {"schema": 1, "version": version, "source_commit": commit, "os": "linux", "arch": arch, "cgo_enabled": False}.items()):
        raise ValueError("package metadata differs from expected source/version/architecture")
    if version.startswith("v") and metadata.get("tracked_source_dirty") is not False:
        raise ValueError("tagged package has dirty source")
    return prefix, files


def verify_dist(directory, version, commit):
    expected = {f"homevault_{version}_linux_{arch}.tar.gz": arch for arch in ["amd64", "arm64"]}
    lines = (directory / "SHA256SUMS").read_text(encoding="ascii").splitlines()
    seen = set()
    packages = {}
    for line in lines:
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] not in expected or match[2] in seen:
            raise ValueError("unexpected checksum entry")
        digest, name = match.groups()
        seen.add(name)
        archive = directory / name
        if archive.is_symlink() or archive.stat().st_size > 64 * 1024 * 1024:
            raise ValueError("invalid archive file")
        if hashlib.sha256(archive.read_bytes()).hexdigest() != digest:
            raise ValueError("archive checksum mismatch")
        packages[expected[name]] = verify_archive(archive, version, expected[name], commit)
    if seen != set(expected):
        raise ValueError("expected both architecture checksums")
    return packages


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--smoke", action="store_true")
    args = parser.parse_args()
    if not re.fullmatch(r"(?:v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?|dev-[0-9a-f]{7,40})", args.version) or not re.fullmatch(r"[0-9a-f]{40}", args.commit):
        raise SystemExit("invalid version or full source commit")
    packages = verify_dist(args.directory, args.version, args.commit)
    print("PASS both archive checksums, safe contents, metadata and ELF architectures", flush=True)
    if args.smoke:
        if platform.system() != "Linux" or platform.machine() not in {"x86_64", "amd64"}:
            raise SystemExit("packaged runtime smoke currently requires Linux amd64")
        prefix, files = packages["amd64"]
        # Only validated regular members are written; never use tar.extractall.
        with tempfile.TemporaryDirectory(prefix="homevault-package-") as directory:
            root = Path(directory)
            for name, (data, mode) in files.items():
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(data)
                target.chmod(mode)
            package = root / prefix
            binary = package / "homevault"
            actual = subprocess.check_output([str(binary), "version"], text=True).strip()
            if actual != "homevault " + args.version:
                raise ValueError("packaged binary version mismatch")
            env = dict(os.environ, HOMEVAULT_QUICKSTART_BINARY=str(binary))
            subprocess.run(["bash", str(package / "scripts/quickstart-smoke.sh")], cwd=package, env=env, check=True, timeout=180)
        print("PASS extracted Linux amd64 package version and real local backup/check/restore")
        print("ARM64 archive checked structurally only; runtime remains unverified")


if __name__ == "__main__":
    main()
