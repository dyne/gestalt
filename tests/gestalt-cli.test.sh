#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-cli-test.XXXXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

fake_bin=$test_root/bin
test_home=$test_root/home
command_log=$test_root/commands.log
mkdir -p -- "$fake_bin" "$test_home"

cat > "$fake_bin/node" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ ${1:-} == -p ]]; then
  printf '24.1.0\n'
  exit 0
fi
printf 'unexpected node invocation\n' >&2
exit 1
EOF

cat > "$fake_bin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
{
  printf 'npm'
  printf '|%s' "$@"
  printf '\n'
} >> "${GESTALT_TEST_LOG:?}"
if [[ ${1:-} == --version ]]; then
  printf '10.9.0\n'
  exit 0
fi
prefix=''
while (($# > 0)); do
  if [[ $1 == --prefix ]]; then
    prefix=$2
    shift 2
    continue
  fi
  shift
done
[[ -n $prefix ]]
mkdir -p -- "$prefix/node_modules/.bin"
cat > "$prefix/node_modules/.bin/gestalt-mobile" <<'MOBILE'
#!/usr/bin/env bash
set -euo pipefail
{
  printf 'mobile|CODEX_HOME=%s|GESTALT_HOME=%s' "${CODEX_HOME:-}" "${GESTALT_HOME:-}"
  printf '|%s' "$@"
  printf '\n'
} >> "${GESTALT_TEST_LOG:?}"
if [[ ${1:-} == --version ]]; then printf '0.1.0\n'; fi
MOBILE
chmod 0755 "$prefix/node_modules/.bin/gestalt-mobile"
EOF

cat > "$fake_bin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
{
  printf 'codex|CODEX_HOME=%s' "${CODEX_HOME:-}"
  printf '|%s' "$@"
  printf '\n'
} >> "${GESTALT_TEST_LOG:?}"
if [[ ${1:-} == --version ]]; then
  printf 'codex-cli 1.0.0\n'
  exit 0
fi
if [[ ${1:-} == plugin && ${2:-} == marketplace && ( ${3:-} == add || ${3:-} == upgrade ) ]]; then
  setup="${CODEX_HOME:?}/.tmp/marketplaces/dyne-gestalt-agents/gestalt-setup.sh"
  mkdir -p -- "$(dirname -- "$setup")"
  cat > "$setup" <<'SETUP'
#!/usr/bin/env bash
set -euo pipefail
{
  printf 'setup|CODEX_HOME=%s|GESTALT_HOME=%s' "${CODEX_HOME:-}" "${GESTALT_HOME:-}"
  printf '|%s' "$@"
  printf '\n'
} >> "${GESTALT_TEST_LOG:?}"
SETUP
  chmod 0755 "$setup"
  exit 0
fi
if [[ ${1:-} == plugin && ${2:-} == list ]]; then
  printf '[]\n'
  exit 0
fi
exit 0
EOF

chmod 0755 "$fake_bin/node" "$fake_bin/npm" "$fake_bin/codex"

export HOME=$test_home
export PATH=$fake_bin:/usr/bin:/bin
export GESTALT_TEST_LOG=$command_log
export CODEX_HOME=$test_home/.codex-gestalt
export GESTALT_HOME=$test_home/.gestalt

assert_log() {
  local -r expected=$1
  if ! grep -F -- "$expected" "$command_log" >/dev/null; then
    printf 'missing command log entry: %s\n' "$expected" >&2
    sed -n '1,200p' "$command_log" >&2
    return 1
  fi
}

bash "$repo_root/public/gestalt" install
[[ -x $GESTALT_HOME/mobile/node_modules/.bin/gestalt-mobile ]]
assert_log "codex|CODEX_HOME=$CODEX_HOME|plugin|marketplace|add|dyne/gestalt-agents"
assert_log "setup|CODEX_HOME=$CODEX_HOME|GESTALT_HOME=$GESTALT_HOME"

bash "$repo_root/public/gestalt" update --extra-skills
assert_log "codex|CODEX_HOME=$CODEX_HOME|plugin|marketplace|upgrade|dyne-gestalt-agents"
grep -F 'setup|' "$command_log" | grep -F -- '--extra-skills' >/dev/null

bash "$repo_root/public/gestalt" cli -- --help
assert_log "codex|CODEX_HOME=$CODEX_HOME|--help"

bash "$repo_root/public/gestalt" mobile -- --cwd "$test_home/workspace"
assert_log "mobile|CODEX_HOME=$CODEX_HOME|GESTALT_HOME=$GESTALT_HOME|--cwd|$test_home/workspace"

bash "$repo_root/public/gestalt" doctor > "$test_root/doctor.out"
grep -F 'All manager checks passed.' "$test_root/doctor.out" >/dev/null

if CODEX_HOME=relative bash "$repo_root/public/gestalt" version > /dev/null 2>&1; then
  printf 'expected relative CODEX_HOME to be rejected\n' >&2
  exit 1
fi

printf 'gestalt-cli.test: PASS\n'
