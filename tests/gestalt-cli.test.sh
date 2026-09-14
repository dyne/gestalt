#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gestalt-cli-test.XXXXXXXX")
test_relay_pid=''
cleanup() {
  if [[ -n $test_relay_pid ]] && kill -0 "$test_relay_pid" 2>/dev/null; then
    kill -TERM "$test_relay_pid" 2>/dev/null || true
    wait "$test_relay_pid" 2>/dev/null || true
  fi
  rm -rf -- "$test_root"
}
trap cleanup EXIT

fake_bin=$test_root/bin
test_home=$test_root/home
command_log=$test_root/commands.log
real_node=$(command -v node)
mkdir -p -- "$fake_bin" "$test_home"

cat > "$fake_bin/node" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ ${1:-} == -p && ${2:-} == 'process.versions.node' ]]; then
  printf '24.1.0\n'
  exit 0
fi
if [[ ${1:-} == -e || ${1:-} == -p || ${1:-} == *.mjs ]]; then
  exec "${GESTALT_TEST_REAL_NODE:?}" "$@"
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
mkdir -p -- "$prefix/node_modules/.bin" "$prefix/node_modules/gestalt-mobile"
cat > "$prefix/node_modules/.bin/gestalt-mobile" <<'MOBILE'
#!/usr/bin/env bash
set -euo pipefail
{
  printf 'mobile|CODEX_HOME=%s|GESTALT_HOME=%s' "${CODEX_HOME:-}" "${GESTALT_HOME:-}"
  printf '|GESTALT_MANAGER_VERSION=%s|GESTALT_AGENTS_VERSION=%s|GESTALT_CONTEXT_MODE_VERSION=%s' \
    "${GESTALT_MANAGER_VERSION:-}" "${GESTALT_AGENTS_VERSION:-}" \
    "${GESTALT_CONTEXT_MODE_VERSION:-}"
  printf '|%s' "$@"
  printf '|PATH=%s|GESTALT_MOBILE_PID=%s|GESTALT_MOBILE_RESTART_STATE=%s\n' \
    "$PATH" "${GESTALT_MOBILE_PID:-}" "${GESTALT_MOBILE_RESTART_STATE:-}"
} >> "${GESTALT_TEST_LOG:?}"
if [[ ${1:-} == --version ]]; then printf '0.1.0\n'; fi
MOBILE
cat > "$prefix/node_modules/gestalt-mobile/gestalt-supervision-capabilities.json" <<'CAPABILITIES'
{"schemaVersion":1,"component":"mobile","supervisionContract":1,"capabilities":["supervision-start","wait-lease-tool","checkpoint-tool","agent-capacity-recovery","controller-status","canonical-agent-identity","session-verdict","acknowledgement-safe-composer","org-plan-contract"]}
CAPABILITIES
chmod 0755 "$prefix/node_modules/.bin/gestalt-mobile"
EOF

cat > "$fake_bin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
{
  printf 'codex|CODEX_HOME=%s' "${CODEX_HOME:-}"
  printf '|%s' "$@"
  printf '|GESTALT_HOME=%s|PATH=%s\n' "${GESTALT_HOME:-}" "$PATH"
} >> "${GESTALT_TEST_LOG:?}"
if [[ ${1:-} == --version ]]; then
  printf 'codex-cli 1.0.0\n'
  exit 0
fi
if [[ ${1:-} == doctor ]]; then
  printf 'Codex doctor passed\n'
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
capability_manifest="${CODEX_HOME:?}/.tmp/marketplaces/dyne-gestalt-agents/plugins/gestalt/gestalt-supervision-capabilities.json"
mkdir -p -- "$(dirname -- "$capability_manifest")"
cat > "$capability_manifest" <<'CAPABILITIES'
{"schemaVersion":1,"component":"agents","supervisionContract":1,"capabilities":["supervision-start","wait-lease-tool","checkpoint-tool","agent-capacity-recovery","canonical-agent-identity","org-plan-contract"]}
CAPABILITIES
SETUP
  chmod 0755 "$setup"
  exit 0
fi
if [[ ${1:-} == plugin && ${2:-} == list ]]; then
  context_version=${GESTALT_TEST_CONTEXT_PLUGIN_VERSION:-2.1.0}
  context_enabled=${GESTALT_TEST_CONTEXT_PLUGIN_ENABLED:-false}
  cat <<JSON
{
  "installed": [
    {
      "pluginId": "gestalt@dyne-gestalt-agents",
      "name": "gestalt",
      "marketplaceName": "dyne-gestalt-agents",
      "version": "2.1.0",
      "installed": true,
      "enabled": true
    },
    {
      "pluginId": "context-mode@dyne-gestalt-agents",
      "name": "context-mode",
      "marketplaceName": "dyne-gestalt-agents",
      "version": "$context_version",
      "installed": true,
      "enabled": $context_enabled
    }
  ],
  "available": []
}
JSON
  exit 0
fi
if [[ ${1:-} == mcp && ${2:-} == get && ${3:-} == context-mode && ${4:-} == --json ]]; then
  context_mcp_enabled=${GESTALT_TEST_CONTEXT_MCP_ENABLED:-true}
  cat <<JSON
{
  "name": "context-mode",
  "enabled": $context_mcp_enabled,
  "transport": {
    "type": "stdio",
    "command": "node",
    "args": ["${CODEX_HOME:?}/bin/context-mode-mcp.mjs"]
  }
}
JSON
  exit 0
fi
exit 0
EOF

chmod 0755 "$fake_bin/node" "$fake_bin/npm" "$fake_bin/codex"

export HOME=$test_home
export PATH=$fake_bin:/usr/bin:/bin
export GESTALT_TEST_LOG=$command_log
export GESTALT_TEST_REAL_NODE=$real_node
export CODEX_HOME=$test_home/.codex-gestalt
export GESTALT_HOME=$test_home/.gestalt
export GESTALT_INSTALL_BASE_URL=file://$repo_root/public

runtime_identity=$($real_node -p '[process.platform, process.arch, "node-" + process.versions.modules].join("-")')
prepared_runtime=$GESTALT_HOME/runtime/context-mode/2.1.0/$runtime_identity
mkdir -p -- "$prepared_runtime"
cat > "$prepared_runtime/cli.bundle.mjs" <<'EOF'
import { appendFileSync } from 'node:fs';
appendFileSync(process.env.GESTALT_TEST_LOG, `context-mode|${process.argv.slice(2).join('|')}\n`);
process.stdout.write('Context-mode doctor passed\n');
EOF
touch "$prepared_runtime/server.bundle.mjs"
cat > "$prepared_runtime/.context-mode-prepared.json" <<EOF
{"packageVersion":"2.1.0","nodeModulesAbi":"$($real_node -p 'process.versions.modules')","platform":"$($real_node -p 'process.platform')","arch":"$($real_node -p 'process.arch')"}
EOF

agents_capability_manifest=$CODEX_HOME/.tmp/marketplaces/dyne-gestalt-agents/plugins/gestalt/gestalt-supervision-capabilities.json
mobile_capability_manifest=$GESTALT_HOME/mobile/node_modules/gestalt-mobile/gestalt-supervision-capabilities.json
write_capability_manifests() {
  mkdir -p -- "$(dirname -- "$agents_capability_manifest")" "$(dirname -- "$mobile_capability_manifest")"
  cat > "$agents_capability_manifest" <<'EOF'
{"schemaVersion":1,"component":"agents","supervisionContract":1,"capabilities":["supervision-start","wait-lease-tool","checkpoint-tool","agent-capacity-recovery","canonical-agent-identity","org-plan-contract"]}
EOF
  cat > "$mobile_capability_manifest" <<'EOF'
{"schemaVersion":1,"component":"mobile","supervisionContract":1,"capabilities":["supervision-start","wait-lease-tool","checkpoint-tool","agent-capacity-recovery","controller-status","canonical-agent-identity","session-verdict","acknowledgement-safe-composer","org-plan-contract"]}
EOF
}

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
[[ -f $agents_capability_manifest && -f $mobile_capability_manifest ]]
assert_log "codex|CODEX_HOME=$CODEX_HOME|plugin|marketplace|add|dyne/gestalt-agents"
assert_log "setup|CODEX_HOME=$CODEX_HOME|GESTALT_HOME=$GESTALT_HOME"

managed_bin=$test_root/managed-bin
mkdir -p -- "$managed_bin"
cp "$repo_root/public/gestalt" "$managed_bin/gestalt"
printf '\n# stale local manager copy\n' >> "$managed_bin/gestalt"
chmod 0755 "$managed_bin/gestalt"

bash "$managed_bin/gestalt" update --extra-skills
cmp "$repo_root/public/gestalt" "$managed_bin/gestalt"
assert_log "codex|CODEX_HOME=$CODEX_HOME|plugin|marketplace|upgrade|dyne-gestalt-agents"
grep -F 'setup|' "$command_log" | grep -F -- '--extra-skills' >/dev/null

bad_update_source=$test_root/bad-update-source
bad_managed_bin=$test_root/bad-managed-bin
mkdir -p -- "$bad_update_source" "$bad_managed_bin"
cp "$repo_root/public/gestalt" "$bad_update_source/gestalt"
printf '%064d  gestalt\n' 0 > "$bad_update_source/gestalt.sha256"
cp "$repo_root/public/gestalt" "$bad_managed_bin/gestalt"
printf '\n# manager that must survive a rejected update\n' >> "$bad_managed_bin/gestalt"
chmod 0755 "$bad_managed_bin/gestalt"
cp "$bad_managed_bin/gestalt" "$test_root/manager-before-rejected-update"

if GESTALT_INSTALL_BASE_URL=file://$bad_update_source \
  bash "$bad_managed_bin/gestalt" update > /dev/null 2>&1; then
  printf 'expected manager update with an invalid checksum to fail\n' >&2
  exit 1
fi
cmp "$test_root/manager-before-rejected-update" "$bad_managed_bin/gestalt"

bash "$repo_root/public/gestalt" cli -- --help
assert_log "codex|CODEX_HOME=$CODEX_HOME|--help"
assert_log "codex|CODEX_HOME=$CODEX_HOME|doctor|--summary|--ascii|--no-color"
assert_log "context-mode|doctor"
grep -F "codex|CODEX_HOME=$CODEX_HOME|--help|GESTALT_HOME=$GESTALT_HOME|PATH=$CODEX_HOME/bin:" \
  "$command_log" >/dev/null

bash "$repo_root/public/gestalt" mobile -- --cwd "$test_home/workspace"
assert_log "mobile|CODEX_HOME=$CODEX_HOME|GESTALT_HOME=$GESTALT_HOME"
grep -F '|GESTALT_MANAGER_VERSION=0.1.0|GESTALT_AGENTS_VERSION=2.1.0|GESTALT_CONTEXT_MODE_VERSION=2.1.0' "$command_log" >/dev/null
grep -F "|--cwd|$test_home/workspace" "$command_log" | grep -F 'mobile|' >/dev/null
grep -F "|PATH=$CODEX_HOME/bin:" "$command_log" | grep -F 'mobile|' >/dev/null
mobile_restart_state=$(find "$GESTALT_HOME/run" -maxdepth 1 -type f -name 'mobile-*.restart' -print -quit)
[[ -n $mobile_restart_state && -r $mobile_restart_state ]]
$real_node -e '
  const fs = require("node:fs");
  const fields = fs.readFileSync(process.argv[1]).toString("utf8").split("\0");
  fields.pop();
  const [cwd, ...args] = fields;
  if (cwd !== process.argv[2] || JSON.stringify(args) !== JSON.stringify(["--cwd", process.argv[3]])) {
    process.exit(1);
  }
' "$mobile_restart_state" "$repo_root" "$test_home/workspace"
grep -F "GESTALT_MOBILE_RESTART_STATE=$mobile_restart_state" "$command_log" >/dev/null

if bash "$repo_root/public/gestalt" update-restart > "$test_root/unmanaged-restart.out" 2>&1; then
  printf 'update-restart unexpectedly accepted an unmanaged shell\n' >&2
  exit 1
fi
grep -F 'must be run from a session managed by Gestalt Mobile' \
  "$test_root/unmanaged-restart.out" >/dev/null

mkdir -p -- "$test_home/restarted-workspace"
sleep 30 &
test_relay_pid=$!
failed_restart_state=$GESTALT_HOME/run/mobile-$test_relay_pid.restart
failed_restart_lock=$GESTALT_HOME/run/update-restart.lock
mkdir "$failed_restart_lock"
printf '%s\0%s\0%s\0' "$test_home/restarted-workspace" --port 3210 > "$failed_restart_state"
if GESTALT_INSTALL_BASE_URL=file://$bad_update_source \
  bash "$repo_root/public/gestalt" __update-restart-worker \
    "$test_relay_pid" "$failed_restart_state" false "$test_root/failed-update-restart.log" \
    "$failed_restart_lock"; then
  printf 'update-restart worker unexpectedly accepted a bad manager checksum\n' >&2
  exit 1
fi
kill -0 "$test_relay_pid"
kill -TERM "$test_relay_pid"
wait "$test_relay_pid" 2>/dev/null || true
test_relay_pid=''
grep -F 'manager update checksum verification failed' "$test_root/failed-update-restart.log" >/dev/null
[[ ! -e $failed_restart_lock ]]

sleep 30 &
test_relay_pid=$!
live_restart_state=$GESTALT_HOME/run/mobile-$test_relay_pid.restart
printf '%s\0%s\0%s\0' "$test_home/restarted-workspace" --port 4321 > "$live_restart_state"
mkdir "$GESTALT_HOME/run/update-restart.lock"
if GESTALT_MOBILE_PID=$test_relay_pid GESTALT_MOBILE_RESTART_STATE=$live_restart_state \
  bash "$repo_root/public/gestalt" update-restart > "$test_root/concurrent-restart.out" 2>&1; then
  printf 'update-restart unexpectedly accepted a concurrent update\n' >&2
  exit 1
fi
grep -F 'another update-restart is already running' "$test_root/concurrent-restart.out" >/dev/null
rmdir "$GESTALT_HOME/run/update-restart.lock"
GESTALT_MOBILE_PID=$test_relay_pid GESTALT_MOBILE_RESTART_STATE=$live_restart_state \
  bash "$repo_root/public/gestalt" update-restart > "$test_root/scheduled-restart.out" 2>&1
grep -F 'scheduled update and Mobile restart' "$test_root/scheduled-restart.out" >/dev/null
for ((attempt = 0; attempt < 200; attempt += 1)); do
  if ! kill -0 "$test_relay_pid" 2>/dev/null &&
    grep -F 'mobile|' "$command_log" | grep -F '|--port|4321|' >/dev/null; then
    break
  fi
  sleep 0.1
done
if kill -0 "$test_relay_pid" 2>/dev/null; then
  printf 'scheduled update-restart did not stop the managed relay\n' >&2
  exit 1
fi
wait "$test_relay_pid" 2>/dev/null || true
test_relay_pid=''
grep -F 'mobile|' "$command_log" | grep -F '|--port|4321|' >/dev/null
grep -F 'restarting Gestalt Mobile with its previous options' \
  "$GESTALT_HOME/update-restart.log" >/dev/null

bash "$repo_root/public/gestalt" doctor > "$test_root/doctor.out"
grep -E '^Gestalt plugins +2\.1\.0$' "$test_root/doctor.out" >/dev/null
grep -E '^Context-mode plugin +2\.1\.0$' "$test_root/doctor.out" >/dev/null
grep -E '^Context-mode runtime +' "$test_root/doctor.out" >/dev/null
grep -E '^Plugin MCP source +disabled \(expected\)$' "$test_root/doctor.out" >/dev/null
grep -E '^Native context MCP +enabled$' "$test_root/doctor.out" >/dev/null
grep -E '^Supervision contract +ready \(v1; offline manifests\)$' "$test_root/doctor.out" >/dev/null
grep -F 'All startup diagnostics passed.' "$test_root/doctor.out" >/dev/null

rm -f -- "$agents_capability_manifest"
if bash "$repo_root/public/gestalt" doctor > "$test_root/missing-capability-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted a missing Agents capability manifest\n' >&2
  exit 1
fi
grep -F 'Agents supervision       UNAVAILABLE (run gestalt update, then restart Mobile)' \
  "$test_root/missing-capability-doctor.out" >/dev/null
write_capability_manifests

printf '{not-json\n' > "$mobile_capability_manifest"
if bash "$repo_root/public/gestalt" doctor > "$test_root/malformed-capability-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted a malformed Mobile capability manifest\n' >&2
  exit 1
fi
grep -F 'Mobile supervision       INCOMPATIBLE (malformed capability manifest; run gestalt update, then restart Mobile)' \
  "$test_root/malformed-capability-doctor.out" >/dev/null
write_capability_manifests

sed -i 's/"supervision-start"/"bad,name"/' "$agents_capability_manifest"
if bash "$repo_root/public/gestalt" doctor > "$test_root/malformed-capability-name-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted an unsafe capability name\n' >&2
  exit 1
fi
grep -F 'Agents supervision       INCOMPATIBLE (malformed capability manifest; run gestalt update, then restart Mobile)' \
  "$test_root/malformed-capability-name-doctor.out" >/dev/null
write_capability_manifests

sed -i 's/"supervisionContract":1/"supervisionContract":2/' "$agents_capability_manifest"
if bash "$repo_root/public/gestalt" doctor > "$test_root/stale-capability-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted a stale supervision contract\n' >&2
  exit 1
fi
grep -F 'Supervision contract     INCOMPATIBLE (Agents=2 Mobile=1; run gestalt update, then restart Mobile)' \
  "$test_root/stale-capability-doctor.out" >/dev/null
write_capability_manifests

sed -i 's/,"session-verdict"//' "$mobile_capability_manifest"
if bash "$repo_root/public/gestalt" doctor > "$test_root/partial-capability-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted a partial Mobile capability manifest\n' >&2
  exit 1
fi
grep -F 'Mobile supervision       INCOMPATIBLE (missing session-verdict; run gestalt update, then restart Mobile)' \
  "$test_root/partial-capability-doctor.out" >/dev/null
write_capability_manifests

if GESTALT_TEST_CONTEXT_PLUGIN_ENABLED=true \
  bash "$repo_root/public/gestalt" doctor > "$test_root/enabled-plugin-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted the manifest MCP source as enabled\n' >&2
  exit 1
fi
grep -F 'ENABLED (disable it; native bridge owns startup)' \
  "$test_root/enabled-plugin-doctor.out" >/dev/null

if GESTALT_TEST_CONTEXT_PLUGIN_ENABLED=true \
  bash "$repo_root/public/gestalt" cli -- --help > "$test_root/enabled-plugin-cli.out" 2>&1; then
  printf 'cli unexpectedly started with the manifest MCP source enabled\n' >&2
  exit 1
fi
grep -F 'context-mode plugin source must remain disabled' \
  "$test_root/enabled-plugin-cli.out" >/dev/null

if GESTALT_TEST_CONTEXT_MCP_ENABLED=false \
  bash "$repo_root/public/gestalt" doctor > "$test_root/disabled-mcp-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted the native MCP as disabled\n' >&2
  exit 1
fi
grep -E '^Native context MCP +MISSING OR DISABLED$' \
  "$test_root/disabled-mcp-doctor.out" >/dev/null

if GESTALT_TEST_CONTEXT_PLUGIN_VERSION=9.9.9 \
  bash "$repo_root/public/gestalt" doctor > "$test_root/skew-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted plugin version skew\n' >&2
  exit 1
fi
grep -F 'MISMATCH (gestalt=2.1.0 context-mode=9.9.9)' "$test_root/skew-doctor.out" >/dev/null

rm -f -- "$prepared_runtime/.context-mode-prepared.json"
if bash "$repo_root/public/gestalt" doctor > "$test_root/runtime-doctor.out" 2>&1; then
  printf 'doctor unexpectedly accepted a missing prepared runtime marker\n' >&2
  exit 1
fi
grep -F "NOT PREPARED ($prepared_runtime)" "$test_root/runtime-doctor.out" >/dev/null

if CODEX_HOME=relative bash "$repo_root/public/gestalt" version > /dev/null 2>&1; then
  printf 'expected relative CODEX_HOME to be rejected\n' >&2
  exit 1
fi

printf 'gestalt-cli.test: PASS\n'
