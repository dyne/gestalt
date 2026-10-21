import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { parse } from 'smol-toml';

const manager = new URL('../public/gestalt', import.meta.url).pathname;
const source = await readFile(manager, 'utf8');
const helpers = source.slice(source.indexOf('configure_codex_permissions() {'),
  source.indexOf('\nparse_setup_options() {')).replaceAll('${BASH_SOURCE[0]}', manager);
const version = source.match(/^readonly GESTALT_CLI_VERSION='([^']+)'/m)[1];
const digest = text => createHash('sha256').update(text).digest('hex');
const cleanupInitializers = source.slice(0, source.indexOf('log() {')).split('\n')
  .filter(line => /^[a-z_]+=''$/.test(line)).join('\n');
const productionCleanup = source.match(/^cleanup\(\) \{[\s\S]*?^\}/m)[0]
  .replace('cleanup() {', 'production_cleanup() {');

const custom = `model = "gpt-6-luna" # keep the user's model
model_reasoning_effort = "low"
[features]
hooks = true
[[skills.config]]
name = "custom-skill"
enabled = true
[agents.custom]
config_file = "agents/custom.toml"
[mcp_servers.custom]
command = "custom"
default_tools_approval_mode = "prompt"
`;

async function fixture(t, config = custom) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt-setup-contract-'));
  const home = join(root, 'home'), codex = join(home, '.codex-gestalt'), managed = join(home, '.gestalt');
  await mkdir(codex, { recursive: true });
  await mkdir(managed);
  if (config !== null) await writeFile(join(codex, 'config.toml'), config, { mode: 0o640 });
  await mkdir(join(codex, 'agents'));
  await writeFile(join(codex, 'agents/custom.toml'), 'model="gpt-6-luna"\n');
  await writeFile(join(codex, 'AGENTS.md'), 'User instructions remain untouched.\n');
  const plugin = join(root, 'plugin', '0.0.1');
  await mkdir(join(plugin, 'hooks'), { recursive: true });
  await writeFile(join(plugin, 'package.json'), '{"version":"0.0.1"}');
  await writeFile(join(plugin, 'start.mjs'), '');
  await writeFile(join(plugin, 'hooks/runtime-hook.mjs'), '');
  const owner = join(root, 'delegated-setup.sh');
  // A deterministic package stand-in tests the manager boundary. Actual pinned
  // package generation is checked separately; the manager must never generate roles.
  await writeFile(owner, `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'setup' >> "$FIXTURE_LOG"
printf '|%s' "$@" >> "$FIXTURE_LOG"
printf '\\n' >> "$FIXTURE_LOG"
if [[ \${FIXTURE_WAIT:-} == 1 ]]; then
  touch "$FIXTURE_READY"
  for ((attempt=0; attempt<400; attempt++)); do
    [[ ! -f $FIXTURE_RELEASE ]] || break
    sleep 0.025
  done
  [[ -f $FIXTURE_RELEASE ]] || exit 8
fi
[[ \${FIXTURE_FAIL:-} != 1 ]] || exit 9
mkdir -p -- "$CODEX_HOME/agents"
printf 'model="fixture-provider-default"\\n' > "$CODEX_HOME/agents/org-plan-executor.toml"
printf 'model="fixture-provider-default"\\n' > "$CODEX_HOME/agents/org-plan-reviewer.toml"
node "$FIXTURE_CONFIGURATOR" --codex-home "$CODEX_HOME" --gestalt-home "$GESTALT_HOME" --plugin-root "$FIXTURE_PLUGIN" --plugin-id context-mode@fixture
`);
  const script = join(root, 'manager-setup.sh');
  await writeFile(script, `set -Eeuo pipefail
log() { :; }
${cleanupInitializers}
${productionCleanup}
cleanup() {
  printf 'cleanup\\n' >> "$FIXTURE_CLEANUP_LOG"
  production_cleanup
  [[ \${FIXTURE_CLEANUP_FAIL:-} != 1 ]]
}
trap cleanup EXIT
if [[ \${FIXTURE_PREEXISTING_STATE:-} == 1 ]]; then
  update_restart_lock=$FIXTURE_RESTART_LOCK
  manager_update_temp_root=$FIXTURE_UPDATE_SCRATCH
  manager_update_staged_target=$FIXTURE_UPDATE_FILE
fi
die() { printf '%s\\n' "$*" >&2; exit 1; }
check_node_version() { :; }
require_command() { command -v "$1" >/dev/null; }
codex() { printf 'codex|%s\\n' "$*" >> "$FIXTURE_LOG"; [[ \${FIXTURE_MARKETPLACE_FAIL:-} != 1 ]]; }
verify_context_mode_installation() { [[ \${FIXTURE_VERIFY_FAIL:-} != 1 ]]; }
codex_home=$CODEX_HOME
gestalt_home=$GESTALT_HOME
marketplace_setup=$FIXTURE_OWNER
marketplace=fixture-marketplace
marketplace_name=fixture-marketplace
GESTALT_CLI_VERSION=${version}
${helpers}
run_marketplace_setup install "\${FIXTURE_EXTRA:-false}"
`);
  const env = { ...process.env, HOME: home, CODEX_HOME: codex, GESTALT_HOME: managed,
    FIXTURE_OWNER: owner, FIXTURE_PLUGIN: plugin,
    FIXTURE_CLEANUP_LOG: join(root, 'cleanup-calls'),
    FIXTURE_RESTART_LOCK: join(managed, 'run/update-restart.lock'),
    FIXTURE_UPDATE_SCRATCH: join(root, 'owned-update-scratch'),
    FIXTURE_UPDATE_FILE: join(root, 'owned-update-file'),
    FIXTURE_CONFIGURATOR: new URL('./fixtures/supervision-packages/agents/configure-codex.mjs', import.meta.url).pathname, FIXTURE_LOG: join(root, 'calls.log'), FIXTURE_READY: join(root, 'ready'),
    FIXTURE_RELEASE: join(root, 'release') };
  function start(overrides = {}) {
    const child = spawn('bash', [script], { env: { ...env, ...overrides }, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.on('data', data => stdout += data);
    child.stderr.on('data', data => stderr += data);
    const result = new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('exit', (code, signal) => resolve({ code, signal, stdout, stderr }));
    });
    return { child, result };
  }
  t.after(async () => { await chmod(codex, 0o700); await rm(root, { recursive: true, force: true }); });
  return { root, home, codex, owner, env, start,
    run: overrides => start(overrides).result,
    config: () => readFile(join(codex, 'config.toml'), 'utf8'),
    provenance: () => readFile(join(codex, '.gestalt-setup-provenance.json'), 'utf8') };
}

for (const config of [null, custom]) {
  test(`${config === null ? 'fresh' : 'customized'} setup repeats identically with one managed permission block and provenance`, async t => {
    const f = await fixture(t, config);
    let result = await f.run(); assert.equal(result.code, 0, result.stderr);
    const first = await f.config(), provenance = await f.provenance();
    const parsed = parse(first);
    if (config) {
      assert.equal(parsed.model, 'gpt-6-luna');
      assert.equal(parsed.model_reasoning_effort, 'low');
      assert.ok(first.includes("# keep the user's model"));
      assert.equal(parsed.skills.config[0].enabled, true);
      assert.equal(parsed.mcp_servers.custom.default_tools_approval_mode, 'prompt');
      assert.equal((await stat(join(f.codex, 'config.toml'))).mode & 0o777, 0o640);
    }
    assert.equal(first.split('[permissions.workspace-git]').length - 1, 1);
    assert.equal(await readFile(join(f.codex, 'agents/custom.toml'), 'utf8'), 'model="gpt-6-luna"\n');
    assert.equal(await readFile(join(f.codex, 'AGENTS.md'), 'utf8'), 'User instructions remain untouched.\n');
    const metadata = JSON.parse(provenance);
    assert.equal(metadata.manager.version, version);
    assert.equal(metadata.manager.sha256, digest(source));
    assert.equal(metadata.delegatedSetupSha256, digest(await readFile(f.owner)));
    assert.equal(metadata.outputs['config.toml'], digest(first));
    assert.equal(metadata.scope, 'setup-artifacts-only');
    assert.equal(metadata.runtimeEnforcement, false);
    assert.equal(metadata.outputs['hooks.json'], digest(await readFile(join(f.codex, 'hooks.json'))));
    assert.equal(metadata.outputs['agents/org-plan-executor.toml'], digest('model="fixture-provider-default"\n'));
    // Explicit setup retains the provider's ownership of generated roles. Manual
    // edits there are outside the preservation contract; launches never regenerate.
    for (const name of ['org-plan-executor', 'org-plan-reviewer']) {
      await writeFile(join(f.codex, 'agents', `${name}.toml`), 'model="manual-generated-role-edit"\n');
    }
    result = await f.run(); assert.equal(result.code, 0, result.stderr);
    for (const name of ['org-plan-executor', 'org-plan-reviewer']) {
      assert.equal(await readFile(join(f.codex, 'agents', `${name}.toml`), 'utf8'),
        'model="fixture-provider-default"\n');
    }
    assert.equal(await f.config(), first);
    assert.equal(await f.provenance(), provenance);
    assert.deepEqual((await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.')), []);
  });
}

for (const failure of ['FIXTURE_FAIL', 'FIXTURE_VERIFY_FAIL', 'FIXTURE_MARKETPLACE_FAIL']) {
  for (const config of [null, custom]) {
    test(`${failure} restores ${config === null ? 'absent' : 'existing'} config without claiming successful provenance`, async t => {
      const f = await fixture(t, config);
      const result = await f.run({ [failure]: '1' });
      assert.notEqual(result.code, 0);
      if (config === null) await assert.rejects(f.config(), { code: 'ENOENT' });
      else {
        assert.equal(await f.config(), config);
        assert.equal((await stat(join(f.codex, 'config.toml'))).mode & 0o777, 0o640);
      }
      await assert.rejects(f.provenance(), { code: 'ENOENT' });
      assert.deepEqual((await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup')), []);
    });
  }
}

test('failed repeated setup retains last successful provenance', async t => {
  const f = await fixture(t);
  assert.equal((await f.run()).code, 0);
  const before = await f.config(), provenance = await f.provenance();
  assert.notEqual((await f.run({ FIXTURE_FAIL: '1' })).code, 0);
  assert.equal(await f.config(), before);
  assert.equal(await f.provenance(), provenance);
});

test('read-only Codex home fails before delegation and leaves user bytes untouched', async t => {
  const f = await fixture(t);
  await chmod(f.codex, 0o500);
  const result = await f.run();
  assert.notEqual(result.code, 0);
  assert.match(result.stderr, /not writable/);
  assert.equal(await f.config(), custom);
  await assert.rejects(readFile(f.env.FIXTURE_LOG), { code: 'ENOENT' });
});

test('concurrent setup rejects the competing writer and releases the lock', async t => {
  const f = await fixture(t);
  const first = f.start({ FIXTURE_WAIT: '1' });
  for (let attempt = 0; attempt < 200; attempt++) {
    try { await stat(f.env.FIXTURE_READY); break; }
    catch { await new Promise(resolve => setTimeout(resolve, 20)); }
  }
  try {
    await stat(f.env.FIXTURE_READY);
    const second = await f.run();
    assert.notEqual(second.code, 0);
    assert.match(second.stderr, /already running/);
  } finally { await writeFile(f.env.FIXTURE_RELEASE, ''); }
  const result = await first.result; assert.equal(result.code, 0, result.stderr);
  assert.equal((await f.run()).code, 0, 'completed setup did not release lock');
});

test('interruption restores configuration and releases the lock', async t => {
  const f = await fixture(t);
  const pending = f.start({ FIXTURE_WAIT: '1' });
  for (let attempt = 0; attempt < 200; attempt++) {
    try { await stat(f.env.FIXTURE_READY); break; }
    catch { await new Promise(resolve => setTimeout(resolve, 20)); }
  }
  await stat(f.env.FIXTURE_READY);
  pending.child.kill('SIGTERM');
  await writeFile(f.env.FIXTURE_RELEASE, '');
  const result = await pending.result;
  assert.equal(result.code, 143);
  assert.equal(await f.config(), custom);
  await assert.rejects(stat(join(f.codex, '.gestalt-setup.lock')), { code: 'ENOENT' });
});

test('only the existing supported extra-skills option is delegated', async t => {
  const f = await fixture(t);
  assert.equal((await f.run({ FIXTURE_EXTRA: 'true' })).code, 0);
  const calls = await readFile(f.env.FIXTURE_LOG, 'utf8');
  assert.match(calls, /setup\|--extra-skills\n/);
  assert.doesNotMatch(calls, /--(?:executor|reviewer|model|reasoning|policy)/);
});

test('unsupported policy defaults fail before public install can mutate user state', async t => {
  const f = await fixture(t);
  const result = await new Promise((resolve, reject) => {
    const child = spawn('bash', [manager, 'install', '--executor-model', 'gpt-6-luna'],
      { env: f.env, stdio: ['ignore', 'pipe', 'pipe'] });
    let stderr = '';
    child.stderr.on('data', data => stderr += data);
    child.once('error', reject);
    child.once('exit', code => resolve({ code, stderr }));
  });
  assert.notEqual(result.code, 0);
  assert.match(result.stderr, /unknown setup option/);
  assert.equal(await f.config(), custom);
  await assert.rejects(readFile(f.env.FIXTURE_LOG), { code: 'ENOENT' });
});

for (const mode of ['failure', 'signal', 'success', 'cleanup-error']) {
  test(`preexisting cleanup-owned state is released exactly once on ${mode}`, async t => {
    const f = await fixture(t);
    await mkdir(f.env.FIXTURE_RESTART_LOCK, { recursive: true });
    await mkdir(f.env.FIXTURE_UPDATE_SCRATCH);
    await writeFile(join(f.env.FIXTURE_UPDATE_SCRATCH, 'partial-download'), 'owned');
    await writeFile(f.env.FIXTURE_UPDATE_FILE, 'owned');
    const options = { FIXTURE_PREEXISTING_STATE: '1' };
    let result;
    if (mode === 'signal') {
      const pending = f.start({ ...options, FIXTURE_WAIT: '1' });
      for (let attempt = 0; attempt < 200; attempt++) {
        try { await stat(f.env.FIXTURE_READY); break; }
        catch { await new Promise(resolve => setTimeout(resolve, 20)); }
      }
      await stat(f.env.FIXTURE_READY);
      pending.child.kill('SIGTERM');
      await writeFile(f.env.FIXTURE_RELEASE, '');
      result = await pending.result;
      assert.equal(result.code, 143, result.stderr);
    } else {
      result = await f.run({ ...options,
        FIXTURE_FAIL: mode === 'success' ? '0' : '1',
        FIXTURE_CLEANUP_FAIL: mode === 'cleanup-error' ? '1' : '0' });
      assert.equal(result.code, mode === 'success' ? 0 : 9, result.stderr);
    }
    assert.equal(await readFile(f.env.FIXTURE_CLEANUP_LOG, 'utf8'), 'cleanup\n');
    for (const path of [f.env.FIXTURE_RESTART_LOCK, f.env.FIXTURE_UPDATE_SCRATCH,
      f.env.FIXTURE_UPDATE_FILE, join(f.codex, '.gestalt-setup.lock')]) {
      await assert.rejects(stat(path), { code: 'ENOENT' });
    }
    if (mode !== 'success') assert.equal(await f.config(), custom);
    assert.deepEqual((await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.')), []);
  });
}
