"""Keep the published walkthrough identical to the Linux CI smoke script."""
from pathlib import Path
import re
import runpy
import subprocess

root = Path(__file__).resolve().parents[1]
text = (root / "docs/QUICKSTART.md").read_text(encoding="utf-8")
blocks = re.findall(r"```bash\n(.*?)```", text, re.S)
script = (root / "scripts/quickstart-smoke.sh").read_text(encoding="utf-8")
if blocks != [script]:
    raise SystemExit("FAIL Quick Start Bash block differs from tested smoke script")
for name in ["README.md", "docs/QUICKSTART.md", "docs/ONBOARDING.md", "docs/FIRST-USER-TRIAL.md"]:
    path = root / name
    for link in re.findall(r"\]\(([^)]+)\)", path.read_text(encoding="utf-8")):
        if "://" not in link and not link.startswith("#"):
            target = link.split("#", 1)[0]
            if not (path.parent / target).exists():
                raise SystemExit(f"FAIL broken local link in {name}: {target}")
print("PASS published Quick Start matches tested script; entry-point links exist")

# Local ignored operator notes must never become release inputs accidentally.
tracked = set(subprocess.check_output(
    ["git", "ls-files", "--", "docs"], cwd=root, text=True).splitlines())
package_docs = runpy.run_path(str(root / "scripts/package-release.py"))["DOCS"]
for name in package_docs:
    if "docs/" + name not in tracked:
        raise SystemExit("FAIL package input is not a tracked public document: " + name)
print("PASS package document allowlist contains only tracked public files")
