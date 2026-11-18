"""Retain executed macOS package and released-upgrade observations."""
import datetime
import json
import os
from pathlib import Path
import platform
import re
import subprocess

out = Path("native-evidence/unix-package")
out.mkdir(parents=True, exist_ok=True)
assert platform.system() == "Darwin", "macOS job requires an actual Darwin host"
assert subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip() == os.environ["GITHUB_SHA"]
def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()

commands = []
for name, argv in [
    ("package", ["node", "scripts/test-unix-package.mjs", os.environ["GESTALT_RELEASE_TARBALL"],
                 os.environ["GESTALT_RELEASE_TARBALL_SHA256"], ".native-dependencies/agents", str(out)]),
    ("upgrade", ["node", "--test", "--test-reporter=tap", "tests/unix-upgrade.test.mjs",
                 "tests/update-components.test.mjs", "tests/manager-compatibility.test.mjs"]),
    ("installer", ["bash", "tests/install.test.sh"]),
]:
    record = {"name": name, "argv": argv, "cwd": os.getcwd(), "startedAt": now()}
    with (out / (name + ".stdout")).open("wb") as stdout, (out / (name + ".stderr")).open("wb") as stderr:
        result = subprocess.run(argv, stdout=stdout, stderr=stderr)
    record.update(exit=result.returncode, observedAt=now())
    commands.append(record)
    (out / "target-commands.json").write_text(json.dumps(commands, indent=2) + "\n")

assert all(item["exit"] == 0 for item in commands), "See failed command and retained streams"
report = json.loads((out / "unix-installed-package.json").read_text())
assert report["package"]["sha256"] == os.environ["GESTALT_RELEASE_TARBALL_SHA256"]
assert report["package"]["manifest"]["gitHead"] == os.environ["GESTALT_RELEASE_SOURCE_HEAD"]
assert report["tests"] == report["passed"] == 4 and report["failed"] == 0
upgrade = dict(re.findall(r"^# (tests|pass|fail|skipped|cancelled|todo) (\d+)$",
                          (out / "upgrade.stdout").read_text(), re.M))
assert upgrade == {"tests": "60", "pass": "60", "fail": "0", "skipped": "0", "cancelled": "0", "todo": "0"}
assert "install.test: PASS" in (out / "installer.stdout").read_text()
(out / "target-result.json").write_text(json.dumps({
    "observedAt": now(), "testHead": os.environ["GITHUB_SHA"], "platform": platform.platform(),
    "bash": subprocess.check_output(["bash", "--version"], text=True).splitlines()[0],
    "package": {"tests": 4, "pass": 4, "fail": 0}, "upgrade": upgrade, "installer": "pass",
}, indent=2) + "\n")
