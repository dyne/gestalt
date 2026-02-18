#!/usr/bin/env bash
set -euo pipefail

if ! command -v rg >/dev/null 2>&1; then
  echo "rg (ripgrep) is required for temporal guard checks." >&2
  exit 1
fi

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

declare -a patterns=(
  "go.temporal.io"
  "internal/temporal"
  "temporal-dev-server"
  "GESTALT_TEMPORAL"
  "/api/workflows"
  "/workflow/resume"
  "/workflow/history"
  "temporal_ui_url"
  "temporal_host"
  "temporal.max-output-bytes"
  "use_workflow"
)

declare -a targets=(
  cmd
  internal
  frontend/src
  config
  scripts
  .github
  GNUmakefile
)

for pattern in "${patterns[@]}"; do
  if rg -n -S --fixed-strings --glob '!**/*_test.go' --glob '!scripts/check-no-temporal.sh' "${pattern}" "${targets[@]}"; then
    echo "Temporal guard failed: found disallowed pattern '${pattern}'." >&2
    exit 1
  fi
done
