#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)

bash -n "$repo_root/public/gestalt"
bash -n "$repo_root/public/install.sh"
bash "$repo_root/tests/gestalt-cli.test.sh"
bash "$repo_root/tests/install.test.sh"
bash "$repo_root/tests/xerj.test.sh"
node --test "$repo_root/tests/xerj-maintenance.test.mjs"
node --test "$repo_root/tests/xerj-readiness.test.mjs"
node --test "$repo_root/tests/xerj-lifecycle.test.mjs"
node --test "$repo_root/tests/xerj-cli-startup.test.mjs"
node --test "$repo_root/tests/xerj-cli-native.test.mjs"
node --test "$repo_root/tests/serena-authority.test.mjs" "$repo_root/tests/serena-storage.test.mjs" \
  "$repo_root/tests/serena-native.test.mjs" "$repo_root/tests/serena-real.test.mjs"
bash "$repo_root/tests/skills-transfer.test.sh"
node --test "$repo_root/tests/versioning.test.mjs"

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck \
    "$repo_root/public/gestalt" \
    "$repo_root/public/install.sh" \
    "$repo_root/tests/gestalt-cli.test.sh" \
    "$repo_root/tests/install.test.sh" \
    "$repo_root/tests/xerj.test.sh" \
    "$repo_root/tests/skills-transfer.test.sh"
else
  printf 'tests: shellcheck unavailable; static shell lint skipped\n' >&2
fi

printf 'tests: all focused shell tests passed\n'
