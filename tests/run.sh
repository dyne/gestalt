#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)

bash -n "$repo_root/public/gestalt"
bash -n "$repo_root/public/install.sh"
bash "$repo_root/tests/gestalt-cli.test.sh"
bash "$repo_root/tests/install.test.sh"

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck \
    "$repo_root/public/gestalt" \
    "$repo_root/public/install.sh" \
    "$repo_root/tests/gestalt-cli.test.sh" \
    "$repo_root/tests/install.test.sh"
else
  printf 'tests: shellcheck unavailable; static shell lint skipped\n' >&2
fi

printf 'tests: all focused shell tests passed\n'
