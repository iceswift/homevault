"""Validate archives before extraction; optionally run packaged amd64 recovery/TUI checks."""
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
            if relative not in {"homevault", "RELEASE.json", "README.md", "LICENSE", "CONTRIBUTING.md", "SECURITY.md", "CHANGELOG.md", "scripts/quickstart-smoke.sh", "docs/assets/README.md", "docs/assets/tui-welcome.txt", "docs/assets/tui-welcome.png", "docs/assets/homevault-banner.svg", "docs/assets/tui-prototype-wide.png", "docs/assets/tui-prototype-wide.txt", "docs/assets/tui-prototype-compact.png", "docs/assets/tui-prototype-compact.txt"} and not re.fullmatch(r"docs/[A-Z][A-Z0-9-]*\.(md|txt)", relative):
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


def verify_tui_views(binary, root):
    """Exercise actual packaged UI entry points, with generated metadata only."""
    def run(*args, env=None):
        result = subprocess.run([str(binary), *map(str, args)], env=env,
                                capture_output=True, text=True, timeout=30, cwd=root)
        if result.returncode:
            raise ValueError("packaged TUI command failed; output withheld")
        return result.stdout

    lab = root / "tui-generated"
    run("demo-setup", lab)
    cfg = lab / "homevault.json"
    base = json.loads(cfg.read_text(encoding="utf-8"))
    if base.get("discord_webhook_file") or base.get("rclone_config_file"):
        raise ValueError("synthetic UI fixture unexpectedly configures external credentials")
    for key in ["repository", "state_dir", "password_file"]:
        if not Path(base[key]).resolve().is_relative_to(lab.resolve()):
            raise ValueError("synthetic UI fixture escaped its generated root")
    if any(not Path(path).resolve().is_relative_to(lab.resolve())
           for project in base["projects"] for path in project["paths"]):
        raise ValueError("synthetic UI source escaped its generated root")
    plan = root / "tui-plan.json"
    plan.write_text(json.dumps({
        "version": 1, "root": str(root / "absent-coordinator"),
        "base_config": str(cfg), "homevault_binary": str(binary),
        "notifications_enabled": False, "workflows": [{
            "name": "synthetic", "project": "synthetic", "kind": "files",
            "enabled": False, "max_age_hours": 36, "files": [], "trees": []}]
    }), encoding="utf-8")
    plan.chmod(0o600)

    def inventory():
        # Only generated fixtures; credential contents never leave this process.
        result = {str(p.relative_to(root)): (
            p.stat().st_mode, p.stat().st_mtime_ns,
            hashlib.sha256(p.read_bytes()).hexdigest() if p.is_file() else None)
            for p in [lab, *lab.rglob("*"), plan]}
        result["root_entries"] = tuple(sorted(p.name for p in root.iterdir()))
        return result

    before = inventory()
    env = dict(os.environ, PATH="", NO_COLOR="1")
    demo = run("tui-demo", "--render", env=env)
    status = run("tui-status", "--plan", plan, "--context", "lab", "--render", "--width", "156", "--height", "42", env=env)
    jobs = run("--config", cfg, "tui-jobs", "--context", "lab", "--render", "--width", "156", "--height", "42", env=env)
    for output, marker in [(demo, "DESIGN PREVIEW"), (status, "WORKFLOWS - READ ONLY"), (jobs, "MANUAL FILE JOBS")]:
        if marker not in output:
            raise ValueError("packaged TUI mode marker missing")
    if "LAB (operator label)" not in status or "LAB (operator label)" not in jobs:
        raise ValueError("packaged TUI context label missing")
    if "Blocked in this mode" not in jobs:
        raise ValueError("tool-less packaged job view is not blocked")
    if inventory() != before or Path(base["repository"]).exists():
        raise ValueError("packaged display-only UI changed generated fixture")
    print("PASS extracted TUI demo/status/jobs render with empty PATH and unchanged generated metadata")
    print("Display-only check; interactive terminal behavior and application recovery are separate")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--smoke", action="store_true")
    parser.add_argument("--tui-smoke", action="store_true",
                        help="with --smoke: verify redesigned packaged TUI entry points (current builds only)")
    args = parser.parse_args()
    if args.tui_smoke and not args.smoke:
        parser.error("--tui-smoke requires --smoke")
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
            if args.tui_smoke:
                verify_tui_views(binary, root)
        print("PASS extracted Linux amd64 package version and real local backup/check/restore")
        print("ARM64 archive checked structurally only; runtime remains unverified")


if __name__ == "__main__":
    main()
