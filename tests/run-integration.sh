#!/usr/bin/env bash
# Opt-in native runtime and terminal coverage. Use the documented host runner
# when these suites need a nested Codex sandbox; they do not gate publication.
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)

cd -- "$repo_root"
node --test tests/playwright-cli-control.test.mjs tests/playwright-cli-native.test.mjs
node --test tests/cli-startup-native.test.mjs tests/xerj-cli-native.test.mjs
node --test tests/serena-cli-native.test.mjs
SERENA_NATIVE_TESTS=1 node --test tests/serena-native.test.mjs
node --test tests/serena-real.test.mjs
