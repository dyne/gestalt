import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { mkdir, mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const exec = promisify(execFile);
const source = await readFile(resolve('public/gestalt'), 'utf8');
assert.match(source, /\nmain "\$@"\s*$/);

async function fixture(t, installed = true) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt update components '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, 'managed');
  const log = join(root, 'operations');
  const manager = join(root, 'gestalt');
  await mkdir(join(home, 'run'), { recursive: true });
  if (installed) {
    await mkdir(join(home, 'serena'));
    await writeFile(join(home, 'serena/active.json'), '{}');
    await mkdir(join(home, 'xerj'));
    await writeFile(join(home, 'xerj/xerj'), 'installed fixture');
  }
  // Retain the production parser, orchestration, detached worker and restart
  // lifecycle. Replace only release installers/launchers so these tests cannot
  // download software, update the real installation, or start Mobile.
  await writeFile(manager, source.replace(/\nmain "\$@"\s*$/, `
record_update() {
  printf '%s\\n' "$1" >> "$GESTALT_TEST_UPDATE_LOG"
  [[ \${GESTALT_TEST_UPDATE_FAIL:-} != "$1" ]] || return 42
}
update_manager() { record_update manager; }
run_marketplace_setup() { record_update "agents:$1:$2"; }
install_mobile() { record_update mobile; }
command_serena() { [[ $1 == update ]]; record_update serena; }
command_xerj() {
  [[ $1 == update ]]; record_update xerj
  if [[ -n \${GESTALT_TEST_RESOLVED_HOME:-} ]]; then gestalt_home=$GESTALT_TEST_RESOLVED_HOME; fi
}
command_mobile() { record_update "restart:$1:$2:$3"; }
main "$@"
`), { mode: 0o755 });
  const env = {
    ...process.env,
    CODEX_HOME: join(root, 'codex'),
    GESTALT_HOME: home,
    GESTALT_TEST_UPDATE_LOG: log,
    GESTALT_TEST_UPDATE_FAIL: '',
  };
  const operations = async () => readFile(log, 'utf8').then(
    text => text.trim().split('\n').filter(Boolean),
    error => { if (error.code === 'ENOENT') return []; throw error; },
  );
  const run = (args, extra = {}) => exec('bash', [manager, ...args], {
    env: { ...env, ...extra }, timeout: 15_000,
  });
  return { root, home, log, env, run, operations };
}

test('update help lists component groups without invoking any installer', async t => {
  const f = await fixture(t, false);
  for (const args of [['update', '--help'], ['update-restart', '--help']]) {
    const { stdout } = await f.run(args);
    for (const name of ['all', 'gestalt', 'serena', 'xerj'])
      assert.match(stdout, new RegExp(`^  ${name} +`, 'm'));
    assert.match(stdout, /context-mode/);
    assert.match(stdout, /private uv\/Python/);
  }
  assert.deepEqual(await f.operations(), []);
});

test('default update includes all installed managed components in order', async t => {
  const f = await fixture(t);
  await f.run(['update', '--extra-skills']);
  assert.deepEqual(await f.operations(), ['manager', 'agents:upgrade:true', 'mobile', 'serena', 'xerj']);
});

test('default update skips absent optional components with an explicit message', async t => {
  const f = await fixture(t, false);
  const { stderr } = await f.run(['update']);
  assert.deepEqual(await f.operations(), ['manager', 'agents:upgrade:false', 'mobile']);
  assert.match(stderr, /skipping Serena \(not installed/);
  assert.match(stderr, /skipping XERJ \(not installed/);
});

for (const component of ['all', 'gestalt', 'serena', 'xerj']) {
  for (const selector of [[component], ['--component', component]]) {
    test(`update ${selector.join(' ')} changes only the selected component group`, async t => {
      const f = await fixture(t, component === 'all' || component === 'gestalt');
      await f.run(['update', ...selector]);
      const core = ['manager', 'agents:upgrade:false', 'mobile'];
      assert.deepEqual(await f.operations(), component === 'all' ? [...core, 'serena', 'xerj']
        : component === 'gestalt' ? core : [component]);
    });
  }
}

for (const args of [
  ['unknown'], ['--component'], ['--component', 'unknown'], ['serena', 'xerj'],
  ['--component', 'xerj', 'xerj'], ['--unexpected'], ['serena', '--extra-skills'],
]) {
  test(`invalid update arguments are rejected before any change: ${args.join(' ')}`, async t => {
    const f = await fixture(t);
    await assert.rejects(f.run(['update', ...args]));
    assert.deepEqual(await f.operations(), []);
  });
}

for (const failed of ['manager', 'agents:upgrade:false', 'mobile', 'serena', 'xerj']) {
  test(`a failed ${failed} update stops subsequent components and never claims completion`, async t => {
    const f = await fixture(t);
    const order = ['manager', 'agents:upgrade:false', 'mobile', 'serena', 'xerj'];
    await assert.rejects(f.run(['update'], { GESTALT_TEST_UPDATE_FAIL: failed }), error => {
      assert.doesNotMatch(error.stderr, /update complete/);
      return true;
    });
    assert.deepEqual(await f.operations(), order.slice(0, order.indexOf(failed) + 1));
  });
}

async function until(predicate) {
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    if (await predicate()) return;
    await new Promise(resolve => setTimeout(resolve, 25));
  }
  assert.fail('detached update worker did not settle within its deadline');
}

for (const scenario of ['all', 'xerj', 'all-with-symlink']) {
  const component = scenario === 'xerj' ? 'xerj' : 'all';
  test(`GUI-compatible detached update-restart forwards ${scenario} before restarting`, async t => {
    const f = await fixture(t);
    const home = scenario === 'all-with-symlink' ? join(f.root, 'managed-alias') : f.home;
    if (home !== f.home) await symlink(f.home, home);
    const relay = spawn('sleep', ['30']);
    const stopped = new Promise(resolve => relay.once('exit', resolve));
    t.after(async () => { relay.kill(); await stopped; });
    const state = join(home, `run/mobile-${relay.pid}.restart`);
    await writeFile(state, [f.root, '--port', '4321', ''].join('\0'));
    await f.run(['update-restart', ...(component === 'all' ? [] : [component])], {
      GESTALT_MOBILE_PID: String(relay.pid), GESTALT_MOBILE_RESTART_STATE: state,
      GESTALT_HOME: home, GESTALT_TEST_RESOLVED_HOME: f.home,
    });
    await until(async () => (await f.operations()).some(item => item.startsWith('restart:')));
    await stopped;
    const expected = component === 'all'
      ? ['manager', 'agents:upgrade:false', 'mobile', 'serena', 'xerj'] : ['xerj'];
    assert.deepEqual(await f.operations(), [...expected, 'restart:--:--port:4321']);
  });
}

test('failed external GUI update leaves the managed relay running and releases the worker lock', async t => {
  const f = await fixture(t);
  const relay = spawn('sleep', ['30']);
  const stopped = new Promise(resolve => relay.once('exit', resolve));
  t.after(async () => { relay.kill(); await stopped; });
  const state = join(f.home, `run/mobile-${relay.pid}.restart`);
  await writeFile(state, [f.root, '--port', '4321', ''].join('\0'));
  await f.run(['update-restart'], {
    GESTALT_MOBILE_PID: String(relay.pid), GESTALT_MOBILE_RESTART_STATE: state,
    GESTALT_TEST_UPDATE_FAIL: 'serena',
  });
  await until(async () => {
    try { return (await readFile(join(f.home, 'update-restart.log'), 'utf8')).includes('exit 42'); }
    catch { return false; }
  });
  await until(() => readFile(join(f.home, 'run/update-restart.lock')).then(() => false,
    error => error.code === 'ENOENT'));
  assert.equal(relay.exitCode, null);
  assert.equal(relay.signalCode, null);
  assert.deepEqual(await f.operations(), ['manager', 'agents:upgrade:false', 'mobile', 'serena']);
  assert.equal(await readFile(state, 'utf8'), [f.root, '--port', '4321', ''].join('\0'));
});
