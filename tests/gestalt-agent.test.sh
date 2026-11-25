#!/usr/bin/env bash
# Covers `gestalt agent`: provider listing, validation, argv forwarding and the
# provider environment mapping. Uses stub binaries so no real agent CLI or
# network is needed.
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
manager=$repo_root/public/gestalt
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-agent-test.XXXXXXXX")
cleanup() { rm -rf -- "$test_root"; }
trap cleanup EXIT

fake_bin=$test_root/bin
mkdir -p -- "$fake_bin"
for provider in claude ollama opencode agy; do
  cat > "$fake_bin/$provider" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
printf 'stub=%s\n' "$(basename "$0")"
printf 'ANTHROPIC_BASE_URL=%s\n' "${ANTHROPIC_BASE_URL:-<unset>}"
printf 'ANTHROPIC_AUTH_TOKEN=%s\n' "${ANTHROPIC_AUTH_TOKEN:-<unset>}"
printf 'ANTHROPIC_API_KEY=%s\n' "${ANTHROPIC_API_KEY:-<unset>}"
printf 'OLLAMA_HOST=%s\n' "${OLLAMA_HOST:-<unset>}"
printf 'GESTALT_HOME=%s\n' "${GESTALT_HOME:-<unset>}"
printf 'args=%s\n' "$*"
STUB
  chmod +x "$fake_bin/$provider"
done

failures=0
check() {
  local label=$1 expected=$2 actual=$3
  if [[ $actual == *"$expected"* ]]; then
    printf 'ok   %s\n' "$label"
  else
    printf 'FAIL %s\n       expected to contain: %s\n       actual: %s\n' "$label" "$expected" "$actual"
    failures=$((failures + 1))
  fi
}

run() { PATH="$fake_bin:$PATH" bash "$manager" "$@" 2>&1 || true; }

# Help and listing
check 'agent help names every provider' 'opencode' "$(run agent)"
check 'agent help names antigravity' 'antigravity' "$(run agent)"
list_out=$(run agent list)
check 'agent list reports claude' 'claude' "$list_out"
check 'agent list marks stubs installed' 'INSTALLED' "$list_out"
check 'agent list reports every provider' 'opencode' "$list_out"

# Validation
check 'unknown provider is rejected' 'unknown agent provider: bogus' "$(run agent bogus)"

# Arguments are forwarded unchanged, with and without the -- separator.
check 'forwards args after --' 'args=run qwen3:4b' "$(run agent ollama -- run qwen3:4b)"
check 'forwards args without --' 'args=run qwen3:4b' "$(run agent ollama run qwen3:4b)"
check 'launches with no args' 'stub=opencode' "$(run agent opencode)"

# The provider name and the executable name differ for antigravity.
check 'antigravity runs the agy executable' 'stub=agy' "$(run agent antigravity)"
check 'the agy alias resolves to antigravity' 'stub=agy' "$(run agent agy)"
check 'antigravity forwards its own flags' 'args=--dangerously-skip-permissions -p hello' \
  "$(run agent antigravity -- --dangerously-skip-permissions -p hello)"
check 'antigravity is listed as installed' 'antigravity' "$(run agent list)"
# No endpoint override is invented for a provider that documents none.
check 'antigravity gets no anthropic variables' 'ANTHROPIC_BASE_URL=<unset>' \
  "$(PATH="$fake_bin:$PATH" GESTALT_CLAUDE_BASE_URL=http://x bash "$manager" agent antigravity 2>&1 || true)"

# Provider environment mapping, only when the operator set it.
check 'ollama host maps to OLLAMA_HOST' 'OLLAMA_HOST=http://box:11434' \
  "$(PATH="$fake_bin:$PATH" GESTALT_OLLAMA_HOST=http://box:11434 bash "$manager" agent ollama 2>&1 || true)"
check 'ollama host unset stays unset' 'OLLAMA_HOST=<unset>' "$(run agent ollama)"
check 'claude base url maps to ANTHROPIC_BASE_URL' 'ANTHROPIC_BASE_URL=http://127.0.0.1:8080' \
  "$(PATH="$fake_bin:$PATH" GESTALT_CLAUDE_BASE_URL=http://127.0.0.1:8080 bash "$manager" agent claude 2>&1 || true)"
check 'claude base url unset stays unset' 'ANTHROPIC_BASE_URL=<unset>' "$(run agent claude)"
check 'claude auth token maps to ANTHROPIC_AUTH_TOKEN' 'ANTHROPIC_AUTH_TOKEN=tok-123' \
  "$(PATH="$fake_bin:$PATH" GESTALT_CLAUDE_AUTH_TOKEN=tok-123 bash "$manager" agent claude 2>&1 || true)"
check 'claude auth token unset stays unset' 'ANTHROPIC_AUTH_TOKEN=<unset>' "$(run agent claude)"
# The bearer path must not be turned into the x-api-key path.
check 'auth token does not become ANTHROPIC_API_KEY' 'ANTHROPIC_API_KEY=<unset>' \
  "$(PATH="$fake_bin:$PATH" GESTALT_CLAUDE_AUTH_TOKEN=tok-123 bash "$manager" agent claude 2>&1 || true)"
check 'claude auth token does not reach ollama' 'ANTHROPIC_AUTH_TOKEN=<unset>' \
  "$(PATH="$fake_bin:$PATH" GESTALT_CLAUDE_AUTH_TOKEN=tok-123 bash "$manager" agent ollama 2>&1 || true)"

# A provider variable must not leak into a different provider.
check 'ollama host does not reach claude' 'OLLAMA_HOST=<unset>' \
  "$(PATH="$fake_bin:$PATH" GESTALT_OLLAMA_HOST=http://box:11434 bash "$manager" agent claude 2>&1 || true)"

# GESTALT_HOME is always exported.
check 'exports GESTALT_HOME' 'GESTALT_HOME=/' "$(run agent claude)"

# A missing binary must fail cleanly rather than exec nothing. Keep the system
# directories on PATH (bash itself lives there) but drop the stub directory.
check 'missing binary is reported' 'claude' \
  "$(PATH=/usr/bin:/bin /bin/bash "$manager" agent claude 2>&1 || true)"

if ((failures > 0)); then
  printf '\n%d check(s) failed\n' "$failures" >&2
  exit 1
fi
printf '\nall agent checks passed\n'
