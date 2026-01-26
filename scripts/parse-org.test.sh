#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target="${root_dir}/internal/plan/testdata/sample.org"

node "${root_dir}/scripts/parse-org.js" "${target}" > /dev/null
echo "parse-org test passed"
