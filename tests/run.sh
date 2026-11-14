#!/usr/bin/env bash
set -Eeuo pipefail

test_shard=all
if (($# > 0)); then
  if [[ $# != 2 || $1 != --shard || ! $2 =~ ^[12]/2$ ]]; then
    printf 'Usage: npm test [-- --shard 1/2|2/2]\n' >&2
    exit 2
  fi
  test_shard=$2
fi

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)

if [[ $test_shard != 2/2 ]]; then
node --test "$repo_root/tests/session-scratch.test.mjs"
node --test "$repo_root/tests/session-scratch-lifetimes.test.mjs"
node --test "$repo_root/tests/session-scratch-containment.test.mjs"
node --test "$repo_root/tests/session-scratch-recovery.test.mjs"
node --test "$repo_root/tests/session-scratch-tool-recovery.test.mjs"
node --test "$repo_root/tests/session-scratch-mobile.test.mjs"
node --test "$repo_root/tests/session-scratch-manager.test.mjs"
node --test "$repo_root/tests/tool-approvals.test.mjs"
node --test "$repo_root/tests/managed-mcp-approvals.test.mjs"
node --test "$repo_root/tests/codex-app-server-lifecycle.test.mjs"
node --test "$repo_root/tests/node-program.test.mjs"
node --test "$repo_root/tests/supervision-capabilities.test.mjs"
node --test "$repo_root/tests/setup-contract.test.mjs"
node --test "$repo_root/tests/packaged-supervision.test.mjs"
node --test "$repo_root/tests/update-components.test.mjs"
node --test "$repo_root/tests/xerj-retirement.test.mjs"
node --test "$repo_root/tests/manager-compatibility.test.mjs"
node --test "$repo_root/tests/playwright-lifecycle.test.mjs" "$repo_root/tests/playwright-stdio.test.mjs"
node --test "$repo_root/tests/playwright-chrome-install.test.mjs"
node --test "$repo_root/tests/playwright-browser-config.test.mjs"
node --test "$repo_root/tests/playwright-child-transport.test.mjs"
node --test "$repo_root/tests/playwright-cli-startup.test.mjs"
fi

if [[ $test_shard != 1/2 ]]; then
bash -n "$repo_root/public/gestalt"
bash -n "$repo_root/public/install.sh"
bash -n "$repo_root/npm/gestalt"
bash "$repo_root/tests/gestalt-cli.test.sh"
bash "$repo_root/tests/install.test.sh"
node --test "$repo_root/tests/cli-capabilities.test.mjs"
node --test "$repo_root/tests/cli-startup-args.test.mjs"
node --test "$repo_root/tests/serena-authority.test.mjs" "$repo_root/tests/serena-storage.test.mjs" \
  "$repo_root/tests/serena-lifecycle.test.mjs" "$repo_root/tests/serena-doctor.test.mjs"
node --test "$repo_root/tests/skills-lifecycle.test.mjs"
bash "$repo_root/tests/skills-transfer.test.sh"
node --test "$repo_root/tests/versioning.test.mjs"
node --test "$repo_root/tests/npm-package.test.mjs" "$repo_root/tests/npm-publication.test.mjs"

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck \
    "$repo_root/public/gestalt" \
    "$repo_root/public/install.sh" \
    "$repo_root/tests/gestalt-cli.test.sh" \
    "$repo_root/tests/install.test.sh" \
    "$repo_root/tests/skills-transfer.test.sh"
else
  printf 'tests: shellcheck unavailable; static shell lint skipped\n' >&2
fi
fi

printf 'tests: focused shell tests passed (shard %s)\n' "$test_shard"
