import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { once } from 'node:events';
import { createServer } from 'node:http';
import { lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { promisify } from 'node:util';
import { gunzipSync } from 'node:zlib';
import test from 'node:test';
import { managerVersion } from '../scripts/manager-metadata.mjs';

const exec = promisify(execFile);
const root = resolve(import.meta.dirname, '..');
const fixtureRoot = join(import.meta.dirname, 'fixtures/released-managers');
const manifest = JSON.parse(await readFile(join(fixtureRoot, 'manifest.json')));
const candidate = await readFile(join(root, 'public/gestalt'));
const candidateVersion = managerVersion(candidate.toString('utf8'));
const unix = ['linux', 'darwin'].includes(process.platform);
const options = { skip: unix ? false : 'Released Bash updater is a Unix compatibility contract', timeout: 20000 };
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
async function released(tag, path) {
  const entry = manifest.find(value => value.tag === tag && value.path === path);
  assert.ok(entry);
  const bytes = gunzipSync(await readFile(join(fixtureRoot, entry.fixture)));
  assert.equal(bytes.length, entry.bytes);
  assert.equal(digest(bytes), entry.sha256);
  return bytes;
}

async function fixture(t, tag = 'v0.15.1') {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'gestalt released upgrade ')));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const home = join(directory, 'home'), bin = join(home, 'bin');
  await mkdir(bin, { recursive: true });
  const manager = join(bin, 'gestalt'), events = join(directory, 'events');
  const original = await released(tag, 'public/gestalt');
  await writeFile(manager, original, { mode: 0o755 });
  const requests = [];
  let asset = candidate;
  const server = createServer((request, response) => {
    requests.push(request.url);
    response.writeHead(request.url === '/gestalt' ? 200 : 404);
    response.end(request.url === '/gestalt' ? asset : 'not found');
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  t.after(() => new Promise(resolve => { server.closeAllConnections(); server.close(resolve); }));
  const base = `http://127.0.0.1:${server.address().port}`;
  // BASH_ENV observes the unmodified release at its final main invocation.
  // Only component download/launch edges are replaced. Parser, updater, traps,
  // atomic promotion and exec continuation remain the exact released bytes.
  const hook = join(directory, 'external-edges.bash');
  await writeFile(hook, `
upgrade_edges() {
  trap - DEBUG
  printf '%s\\0' entry "$GESTALT_CLI_VERSION" "$0" "$@" >> "$UPGRADE_EVENTS"
  printf '\\0' >> "$UPGRADE_EVENTS"
  run_marketplace_setup() { printf 'agents:%s:%s\\0' "$1" "$2" >> "$UPGRADE_EVENTS"; }
  command_skills() { printf 'skills:%s\\0' "$1" >> "$UPGRADE_EVENTS"; }
  install_mobile() { printf 'mobile\\0' >> "$UPGRADE_EVENTS"; return "\${UPGRADE_COMPONENT_EXIT:-0}"; }
  command_serena() { printf 'serena:%s\\0' "$1" >> "$UPGRADE_EVENTS"; }
  command_playwright() { printf 'playwright:%s\\0' "$1" >> "$UPGRADE_EVENTS"; }
  install() {
    printf 'stage:%s\\0' "\${!#}" >> "$UPGRADE_EVENTS"
    if [[ \${UPGRADE_STAGE_FAIL:-} == 1 ]]; then return 41; fi
    command install "$@"
  }
  mv() {
    if [[ \${UPGRADE_HOLD_PROMOTION:-} == 1 ]]; then
      printf 'promotion-ready\\0' >> "$UPGRADE_EVENTS"
      while :; do sleep 0.05; done
    fi
    command mv "$@"
  }
}
upgrade_main_command='main "$@"'
trap 'if [[ $BASH_COMMAND == "$upgrade_main_command" ]]; then upgrade_edges "$@"; fi' DEBUG
`);
  const env = { ...process.env, HOME: home, CODEX_HOME: join(home, '.codex'), GESTALT_HOME: join(home, '.gestalt'),
    TMPDIR: directory, BASH_ENV: hook, UPGRADE_EVENTS: events, GESTALT_INSTALL_BASE_URL: base };
  delete env.GESTALT_MANAGER_DISTRIBUTION;
  // This is a separate consumer installation, not a child of the runner's
  // managed session. Match the isolated recovery fixtures' environment boundary.
  delete env.GESTALT_SESSION_TMP_CONTEXT;
  const run = (args, extra = {}, target = manager) => exec('bash', [target, ...args], { env: { ...env, ...extra }, timeout: 15000 });
  const operations = async () => (await readFile(events, 'utf8').catch(error => { if (error.code === 'ENOENT') return ''; throw error; })).split('\0');
  return { directory, home, bin, manager, original, requests, env, run, operations, setAsset: value => { asset = value; } };
}

for (const tag of ['v0.14.2', 'v0.15.1']) {
  test(`${tag} actual downloaded manager resumes literal argv once and preserves data and optional tool routes`, options, async t => {
    const f = await fixture(t, tag);
    const args = ['update', '--component', 'all', '--extra-skills'];
    for (const tool of ['serena', 'playwright']) {
      const path = join(f.home, '.gestalt', tool); await mkdir(path, { recursive: true }); await writeFile(join(path, 'active.json'), '{}');
    }
    const sentinel = join(f.home, 'user data é'); await writeFile(sentinel, 'retained');
    await f.run(args);
    assert.deepEqual(f.requests, ['/gestalt']);
    assert.deepEqual(await readFile(f.manager), candidate);
    assert.equal((await stat(f.manager)).mode & 0o777, 0o755);
    assert.equal(await readFile(sentinel, 'utf8'), 'retained');
    const events = await f.operations();
    const continuation = events.indexOf('__updated-manager');
    assert.ok(continuation > 0); assert.deepEqual(events.slice(continuation + 1, continuation + 1 + args.length), args);
    assert.equal(events.filter(value => value === '__updated-manager').length, 1);
    for (const edge of ['skills:update', 'agents:upgrade:true', 'mobile', 'serena:update', 'playwright:update']) assert.equal(events.filter(value => value === edge).length, 1);
    assert.equal(events.find(value => value.startsWith('stage:')).slice(6).startsWith(`${f.bin}/.gestalt.`), true);
    assert.equal((await f.run(['version'])).stdout.split('\n')[0], `Gestalt Manager ${candidateVersion}`);
    assert.deepEqual((await readdir(f.directory)).filter(value => value.startsWith('gestalt-update.')), []);
  });
}

for (const failure of ['invalid syntax', 'missing version', 'stage failure']) test(`released upgrade preserves old bytes on ${failure}`, options, async t => {
  const f = await fixture(t);
  if (failure === 'invalid syntax') f.setAsset(Buffer.from('if then\n'));
  if (failure === 'missing version') f.setAsset(Buffer.from('#!/bin/bash\nexit 0\n'));
  await assert.rejects(f.run(['update', 'gestalt'], { UPGRADE_STAGE_FAIL: failure === 'stage failure' ? '1' : '' }));
  assert.deepEqual(f.requests, ['/gestalt']);
  assert.deepEqual(await readFile(f.manager), f.original);
  assert.equal((await f.operations()).includes('mobile'), false);
  assert.deepEqual((await readdir(f.bin)).filter(value => value.startsWith('.gestalt.')), []);
});

test('released upgrade interrupted before promotion retains old executable and cleans destination staging', options, async t => {
  const f = await fixture(t);
  const child = spawn('bash', [f.manager, 'update', 'gestalt'], { env: { ...f.env, UPGRADE_HOLD_PROMOTION: '1' }, stdio: ['ignore', 'pipe', 'pipe'] });
  child.stdout.resume(); child.stderr.resume(); const closed = once(child, 'close');
  t.after(async () => { if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL'); await closed; });
  const deadline = Date.now() + 10000;
  while (!(await f.operations()).includes('promotion-ready')) {
    assert.ok(Date.now() < deadline, 'promotion checkpoint not reached');
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  assert.deepEqual(await readFile(f.manager), f.original);
  child.kill('SIGTERM'); const [code, signal] = await closed;
  assert.ok(code !== 0 || signal);
  assert.deepEqual(await readFile(f.manager), f.original);
  assert.deepEqual((await readdir(f.bin)).filter(value => value.startsWith('.gestalt.')), []);
});

test('released npm-owned manager keeps npm ownership and does not download or replace itself', options, async t => {
  const f = await fixture(t);
  await f.run(['update', 'gestalt'], { GESTALT_MANAGER_DISTRIBUTION: 'npm' });
  assert.deepEqual(f.requests, []); assert.deepEqual(await readFile(f.manager), f.original);
  assert.equal((await f.operations()).includes('__updated-manager'), false);
});

test('component failure after promotion propagates from new code without replaying the old updater', options, async t => {
  const f = await fixture(t);
  await assert.rejects(f.run(['update', 'gestalt'], { UPGRADE_COMPONENT_EXIT: '42' }), { code: 42 });
  assert.deepEqual(await readFile(f.manager), candidate);
  assert.deepEqual(f.requests, ['/gestalt']);
  assert.equal((await f.operations()).filter(value => value === 'mobile').length, 1);
});

test('released upgrade through a symlinked directory promotes in its canonical destination', options, async t => {
  const f = await fixture(t), alias = join(f.directory, 'bin alias'); await symlink(f.bin, alias);
  await f.run(['update', 'gestalt'], {}, join(alias, 'gestalt'));
  assert.deepEqual(await readFile(f.manager), candidate);
  const events = await f.operations();
  assert.equal(events.find(value => value.startsWith('stage:')).slice(6).startsWith(`${f.bin}/.gestalt.`), true);
});

test('released executable symlink layout replaces the command link without modifying its previous target', options, async t => {
  const f = await fixture(t), target = join(f.directory, 'previous release');
  await writeFile(target, f.original, { mode: 0o755 }); await rm(f.manager); await symlink(target, f.manager);
  await f.run(['update', 'gestalt']);
  assert.equal((await lstat(f.manager)).isSymbolicLink(), false);
  assert.deepEqual(await readFile(f.manager), candidate);
  assert.deepEqual(await readFile(target), f.original);
});

test('already-current manager downloads once without a continuation and retains the same component routes', options, async t => {
  const f = await fixture(t); await writeFile(f.manager, candidate);
  await f.run(['update', 'gestalt']);
  assert.deepEqual(f.requests, ['/gestalt']);
  const events = await f.operations();
  assert.equal(events.includes('__updated-manager'), false);
  assert.equal(events.includes('skills:update'), true);
  assert.equal(events.includes('agents:upgrade:false'), true);
  assert.equal(events.includes('mobile'), true);
});

test('released bootstrap installs the complete current standalone asset using a real local download', options, async t => {
  const f = await fixture(t), installer = join(f.directory, 'install.sh');
  await writeFile(installer, await released('v0.15.1', 'public/install.sh'));
  await exec('bash', [installer, '--no-setup'], { env: { ...f.env, GESTALT_BIN_DIR: f.bin }, timeout: 15000 });
  assert.deepEqual(f.requests, ['/gestalt']); assert.deepEqual(await readFile(f.manager), candidate);
  assert.equal((await stat(f.manager)).mode & 0o777, 0o755);
  assert.equal((await f.run(['version'])).stdout.split('\n')[0], `Gestalt Manager ${candidateVersion}`);
});
