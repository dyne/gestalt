"""Reject incomplete ordinary-account package captures and retain the coverage."""
import datetime
import json
import os
from pathlib import Path
import re

def load(file):
    # PowerShell 5.1 redirection writes UTF-16LE with a BOM, while Node
    # receipts are UTF-8. Decode the JSON from bytes using its actual encoding.
    return json.loads(file.read_bytes())

def counts(text):
    pairs = re.findall(r"^# (tests|pass|fail|cancelled|skipped|todo) (\d+)$", text, re.M)
    if len(pairs) != 12:
        raise ValueError("Expected complete installed-runtime and setup transaction TAP summaries")
    blocks = []
    for index, expected in enumerate((7, 4)):
        result = {name: int(value) for name, value in pairs[index * 6 : index * 6 + 6]}
        if result != {"tests": expected, "pass": expected, "fail": 0, "cancelled": 0, "skipped": 0, "todo": 0}:
            raise ValueError(f"Selected test coverage differs: {result}")
        blocks.append(result)
    return blocks

def verify(root, env):
    slice_root = root / "core-slice"
    token = load(slice_root / "consumer-token.json")
    assert token["standardUser"] is True and token["administrator"] is False
    assert token["noBashPath"] is True and token["noConsumerCompilerPath"] is True
    assert token["developerModeKind"] == "DWord" and token["developerMode"] == 0
    observation = load(root / "harness-observation.json")
    assert observation["directExitObserved"] is True and observation["timedOut"] is False
    copied = load(root / "setup/copied-source-verification.json")
    assert copied["status"] == "matched" and copied["manager"] == env["GITHUB_SHA"]
    npm = {}
    for major, location in [(11, slice_root), (12, slice_root / "npm12")]:
        receipt = load(location / "packed-install.json")
        assert receipt["lifecycleRequested"] is True and receipt["npmVersion"].startswith(f"{major}.")
        assert receipt["sha256"] == env["GESTALT_RELEASE_TARBALL_SHA256"]
        assert receipt["manifest"]["gitHead"] == env["GESTALT_RELEASE_SOURCE_HEAD"]
        assert receipt["manifest"]["os"] == ["linux", "darwin", "win32"]
        commands = load(location / "packed-install-commands.json")
        install = [command for command in commands if "install" in command["command"] and "--global" in command["command"]]
        assert len(install) == 1 and install[0]["exit"] == 0
        if major == 12:
            assert "--allow-scripts=file:" + receipt["tarball"] in install[0]["command"]
            smoke = load(location / "lifecycle.json")
            assert smoke["pass"] is True and smoke["tarballSha256"] == receipt["sha256"]
            assert smoke["npmVersion"] == receipt["npmVersion"]
        npm[str(major)] = receipt["npmVersion"]
    executions = (root / "slice.stdout").read_text(encoding="utf-8-sig")
    terminal = [json.loads(line) for line in executions.splitlines()
                if line.startswith('{"kind":"actual-compiled-cli-codex-installed-org-authenticated-ipc"')]
    assert len(terminal) == 1
    assert terminal[0] == {"kind": "actual-compiled-cli-codex-installed-org-authenticated-ipc",
                           "platform": "win32", "arch": "x64", "tests": 2, "pass": 2, "fail": 0, "skip": 0}
    installed, setup = counts(executions)
    return {"testHead": copied["manager"], "packageSourceHead": env["GESTALT_RELEASE_SOURCE_HEAD"],
            "packageSha256": env["GESTALT_RELEASE_TARBALL_SHA256"], "npm": npm,
            "terminal": terminal[0], "installedRuntime": installed, "setupTransactions": setup}

if __name__ == "__main__":
    root = Path("native-evidence")
    result = verify(root, os.environ)
    result["observedAt"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    (root / "windows-package-result.json").write_text(json.dumps(result, indent=2) + "\n")
