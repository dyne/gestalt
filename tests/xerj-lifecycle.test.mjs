import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import net from 'node:net';
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm, stat, link } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

const manager = new URL('../public/gestalt', import.meta.url).pathname;
const secret = 'private-lifecycle-test-key';
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function freePort() {
  const server = createServer();
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
async function fixture(t, mode = 'healthy', port, automatic = false) {
  port ??= await freePort();
  const temp = await mkdtemp(join(tmpdir(), 'xerj life '));
  const root = join(temp, 'home space'), binary = join(root, 'xerj', 'xerj');
  const starts = join(temp, 'starts.ndjson');
  const endpoint = `http://127.0.0.1:${port}`;
  const name = createHash('sha256').update(endpoint).digest('hex').slice(0, 16);
  const directory = join(root, 'xerj-data', '.lifecycle');
  const lock = join(directory, `${name}.lock`), record = join(directory, `${name}.json`);
  await mkdir(join(root, 'xerj'), { recursive: true });
  await writeFile(binary, `#!${process.execPath}
const fs = require('node:fs');
const mode = ${JSON.stringify(mode)}, secret = ${JSON.stringify(secret)};
const args = process.argv.slice(2);
if (args[0] === '--version') { console.log('xerj v1.0.0-rc.98'); }
else if (args[0] === 'mcp') {
  const r = require('node:readline').createInterface({ input: process.stdin });
  r.on('line', line => {
    const q = JSON.parse(line);
    if (!q.id) return;
    const result = q.method === 'initialize'
      ? { protocolVersion: '2025-03-26', capabilities: { tools: {} }, serverInfo: { name: 'xerj-mcp', version: '1.0.0-rc.98' } }
      : { tools: ['xerj_search', 'xerj_map', 'xerj_code_search'].map(name => ({ name, inputSchema: { type: 'object' } })) };
    console.log(JSON.stringify({ jsonrpc: '2.0', id: q.id, result }));
  });
} else {
  fs.appendFileSync(${JSON.stringify(starts)}, JSON.stringify({ pid: process.pid, args, env: process.env }) + '\\n');
  const data = args[args.indexOf('--data-dir') + 1];
  fs.writeFileSync(require('node:path').join(data, 'admin.key'), secret, { mode: 0o600 });
  console.error(secret.repeat(10000));
  if (mode === 'exit') process.exit(9);
  if (mode === 'timeout') setInterval(() => {}, 1000);
  else setTimeout(() => {
    const server = require('node:http').createServer((request, response) => {
      if (request.headers.authorization !== 'ApiKey ' + secret) { response.writeHead(401); response.end(secret); return; }
      response.setHeader('Content-Type', 'application/json');
      response.end(request.url === '/_cluster/health'
        ? JSON.stringify({ cluster_name: 'xerj', status: 'green', timed_out: false, number_of_nodes: 1 }) : '[]');
    });
    server.listen(Number(args[args.indexOf('--port') + 1]), args[args.indexOf('--bind') + 1]);
    process.on('SIGTERM', () => { if (mode === 'ignore-term') return; server.close(() => process.exit(0)); });
  }, mode === 'bind-race' ? 400 : 100);
}
`, { mode: 0o755 });
  let commands = [];
  async function run(operation, overrides = {}, cancel = false, cwd = root) {
    const env = { HOME: process.env.HOME, PATH: process.env.PATH, GESTALT_HOME: root, CODEX_HOME: root, XERJ_URL: automatic ? undefined : endpoint, XERJ_READY_TIMEOUT_MS: '2500', ...overrides };
    const child = spawn('bash', [manager, 'xerj', operation], { env, cwd });
    commands.push(child.pid);
    let stdout = '', stderr = '';
    child.stdout.on('data', chunk => stdout += chunk);
    child.stderr.on('data', chunk => stderr += chunk);
    if (cancel) {
      for (let attempt = 0; attempt < 200; attempt++) {
        try { await readFile(starts); break; } catch { await pause(5); }
      }
      child.kill('SIGTERM');
    }
    const code = await new Promise(resolve => child.on('close', resolve));
    assert.equal(code, 0, stderr);
    assert.equal(stderr, '');
    assert.ok(!stdout.includes(secret));
    return JSON.parse(stdout);
  }
  async function launched() {
    try { return (await readFile(starts, 'utf8')).trim().split('\n').map(JSON.parse); }
    catch (error) { if (error.code === 'ENOENT') return []; throw error; }
  }
  async function noServers() {
    for (let attempt = 0; attempt < 200; attempt++) {
      if ((await launched()).every(({ pid }) => {
        try { process.kill(pid, 0); return false; } catch (error) { return error.code === 'ESRCH'; }
      })) return;
      await pause(5);
    }
    assert.fail('owned server was not reaped');
  }
  t.after(async () => {
    await run('stop').catch(() => {});
    await noServers();
    await rm(temp, { recursive: true, force: true });
  });
  return { root, binary, endpoint, directory, lock, record, run, launched, noServers, commands };
}

test('automatic endpoints avoid another installation and persist across concurrent clients and restarts', async t => {
  const first = await fixture(t);
  assert.equal((await first.run('ensure-ready')).status, 'ready');
  const second = await fixture(t, 'healthy', undefined, true);
  await mkdir(second.directory, { recursive: true });
  const endpointFile = join(second.directory, 'endpoint.json');
  await writeFile(endpointFile, JSON.stringify({ schemaVersion: 1, endpoint: first.endpoint }));
  // Different installations do not share the first backend's credentials.
  const results = await Promise.all(Array.from({ length: 4 }, () => second.run('ensure-ready')));
  assert.ok(results.every(r => r.status === 'ready'), JSON.stringify(results));
  const endpoint = results[0].endpoint;
  assert.notEqual(endpoint, first.endpoint);
  assert.ok(results.every(r => r.endpoint === endpoint));
  assert.equal((await second.launched()).length, 1);
  assert.equal((await second.run('probe')).endpoint, endpoint);
  assert.equal((await second.run('status')).endpoint, endpoint);
  assert.equal(JSON.parse(await readFile(endpointFile, 'utf8')).endpoint, endpoint);
  assert.equal((await stat(endpointFile)).mode & 0o777, 0o600);
  assert.equal((await second.run('stop')).status, 'stopped');
  assert.equal((await second.run('ensure-ready')).endpoint, endpoint);
  assert.equal((await first.run('probe')).status, 'ready');
});

test('a large retained snapshot history does not exhaust the default startup budget', async t => {
  const f = await fixture(t);
  const snapshots = join(f.root, 'xerj-data/autoindex/ax-fixture/sync-snapshots');
  // Match an accumulated managed store without copying source or index contents.
  // All paths still pass the same per-file lstat and link checks during startup.
  for (let generation = 0; generation < 75; generation++) {
    const blobs = join(snapshots, `run-${generation}`, 'blobs');
    mkdirSync(blobs, { recursive: true });
    for (let file = 0; file < 1200; file++) writeFileSync(join(blobs, String(file)), '');
  }
  const overrides = { XERJ_READY_TIMEOUT_MS: '5000' };
  const ready = await f.run('ensure-ready', overrides);
  assert.equal(ready.status, 'ready', JSON.stringify(ready));
  assert.equal((await f.run('probe', overrides)).status, 'ready');
  assert.equal((await f.launched()).length, 1);
  assert.equal((await f.run('stop', overrides)).status, 'stopped');
  await f.noServers();
});

test('new concurrent sessions reuse the running backend after autoindex links snapshot blobs', async t => {
  const f = await fixture(t, 'healthy', undefined, true);
  const first = await f.run('ensure-ready');
  assert.equal(first.status, 'ready');
  const snapshots = join(f.root, 'xerj-data/autoindex/ax-fixture/sync-snapshots');
  const blob = join(snapshots, 'run-g1/blobs/00000001');
  const reused = join(snapshots, 'run-g2/blobs/00000001');
  await mkdir(join(snapshots, 'run-g1/blobs'), { recursive: true });
  await mkdir(join(snapshots, 'run-g2/blobs'), { recursive: true });
  await writeFile(blob, 'immutable snapshot blob');
  await link(blob, reused);
  await mkdir(join(snapshots, 'run-g1/prepared'), { recursive: true });
  await mkdir(join(snapshots, 'run-g2/prepared'), { recursive: true });
  const prepared = join(snapshots, 'run-g1/prepared/00000001.ndjson');
  await writeFile(prepared, '{}\n');
  await link(prepared, join(snapshots, 'run-g2/prepared/00000001.ndjson'));
  const results = await Promise.all(Array.from({ length: 4 }, () => f.run('ensure-ready')));
  assert.ok(results.every(r => r.status === 'ready' && r.endpoint === first.endpoint), JSON.stringify(results));
  assert.equal((await f.launched()).length, 1, 'new sessions must not replace the running backend');
  assert.equal((await f.run('status')).status, 'ready');
  assert.equal((await f.run('probe')).status, 'ready');
  assert.equal((await stat(blob)).nlink, 2);
  assert.equal((await stat(blob)).ino, (await stat(reused)).ino);
});

test('automatic startup replaces an occupied saved endpoint without changing explicit endpoint policy', async t => {
  const f = await fixture(t, 'healthy', undefined, true);
  const foreign = createServer((request, response) => { response.writeHead(401); response.end(); });
  await new Promise(resolve => foreign.listen(0, '127.0.0.1', resolve));
  t.after(() => new Promise(resolve => foreign.close(resolve)));
  const endpoint = `http://127.0.0.1:${foreign.address().port}`;
  await mkdir(f.directory, { recursive: true });
  await writeFile(join(f.directory, 'endpoint.json'), JSON.stringify({ schemaVersion: 1, endpoint }));
  assert.equal((await f.run('ensure-ready', { XERJ_URL: endpoint })).reason, 'endpoint-in-use');
  const ready = await f.run('ensure-ready');
  assert.equal(ready.status, 'ready');
  assert.notEqual(ready.endpoint, endpoint);
  assert.equal(foreign.listening, true);
});

test('automatic startup retries when another listener wins the backend bind race', async t => {
  const f = await fixture(t, 'bind-race', undefined, true);
  await mkdir(f.directory, { recursive: true });
  await writeFile(join(f.directory, 'endpoint.json'), JSON.stringify({ schemaVersion: 1, endpoint: f.endpoint }));
  const foreign = createServer((request, response) => { response.writeHead(401); response.end(); });
  t.after(() => new Promise(resolve => foreign.close(resolve)));
  const startup = f.run('ensure-ready', { XERJ_READY_TIMEOUT_MS: '5000' });
  for (let i = 0; i < 200 && !(await f.launched()).length; i++) await pause(5);
  assert.equal((await f.launched()).length, 1, 'backend launch must precede competing bind');
  await new Promise((resolve, reject) => {
    foreign.once('error', reject);
    foreign.listen(Number(new URL(f.endpoint).port), '127.0.0.1', resolve);
  });
  const ready = await startup;
  assert.equal(ready.status, 'ready', JSON.stringify(ready));
  assert.notEqual(ready.endpoint, f.endpoint);
  assert.equal((await f.launched()).length, 2);
  assert.equal(foreign.listening, true);
});

test('concurrent first launches start exactly one shared server; repeated ensure and clients reuse it', async t => {
  const f = await fixture(t);
  const results = await Promise.all(Array.from({ length: 4 }, () => f.run('ensure-ready')));
  assert.ok(results.every(r => r.status === 'ready' && r.ownership === 'managed'), JSON.stringify(results));
  assert.equal((await f.launched()).length, 1);
  assert.equal((await f.run('ensure-ready')).status, 'ready');
  for (const pid of f.commands) assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' });
  assert.equal((await f.run('probe')).status, 'ready', 'backend survives client exit');
  const before = (await readdir(f.directory)).sort();
  const result = await f.run('status');
  assert.equal(result.ownership, 'managed');
  assert.deepEqual((await readdir(f.directory)).sort(), before);
  const start = (await f.launched())[0];
  assert.deepEqual(start.args.slice(-6), ['--port', String(new URL(f.endpoint).port), '--bind', '127.0.0.1', '--embed-mode', 'lexical']);
  assert.ok(!start.args.includes('--allow-insecure'));
  assert.ok(!start.args.some(arg => ['install', 'index', 'autoindex', 'init', 'service'].includes(arg)));
  const record = JSON.parse(await readFile(f.record, 'utf8'));
  assert.equal(record.root, f.root); assert.equal(record.endpoint, f.endpoint);
  assert.equal((await stat(f.record)).mode & 0o777, 0o600);
  assert.equal((await stat(f.directory)).mode & 0o777, 0o700);
  for (const file of await readdir(f.directory)) if (file.endsWith('.log')) {
    const log = await readFile(join(f.directory, file), 'utf8');
    assert.ok(log.length <= 32768 && !log.includes(secret));
    assert.equal((await stat(join(f.directory, file))).mode & 0o777, 0o600);
  }
  assert.equal((await f.run('stop')).status, 'stopped');
  await f.noServers();
  assert.equal((await f.run('ensure-ready')).status, 'ready');
  assert.equal((await f.launched()).length, 2);
});
test('different workspaces share one backend and global data directory', async t => {
  const f = await fixture(t);
  const workspaces = [join(f.root, 'a'), join(f.root, 'b')];
  await Promise.all(workspaces.map(cwd => mkdir(cwd)));
  const overrides = {};
  try {
    const results = await Promise.all(workspaces.map(cwd => f.run('ensure-ready', overrides, false, cwd)));
    assert.ok(results.every(result => result.status === 'ready'), JSON.stringify(results));
    assert.equal(results[0].endpoint, results[1].endpoint);
    assert.deepEqual(results.map(result => result.dataRoot), workspaces.map(() => join(f.root, 'xerj-data')));
    assert.equal((await f.launched()).length, 1);
    for (let i = 0; i < workspaces.length; i++) {
      const status = await f.run('status', overrides, false, workspaces[i]);
      assert.equal(status.endpoint, results[i].endpoint);
      assert.equal(status.ownership, 'managed');
      await assert.rejects(stat(join(workspaces[i], '.gestalt')), { code: 'ENOENT' });
    }
    assert.equal((await f.run('stop', overrides, false, workspaces[0])).status, 'stopped');
    assert.equal((await f.run('probe', overrides, false, workspaces[1])).status, 'unavailable');
    assert.equal((await f.run('ensure-ready', overrides, false, workspaces[1])).status, 'ready');
    await assert.rejects(stat(join(f.root, 'runtime', 'xerj')), { code: 'ENOENT' });
  } finally {
    await Promise.all(workspaces.map(cwd => f.run('stop', overrides, false, cwd)));
  }
});
test('stale lock is reclaimed by verified identity, without signaling its PID', async t => {
  const f = await fixture(t);
  await mkdir(f.lock, { recursive: true });
  await writeFile(join(f.lock, 'owner.json'), JSON.stringify({ root: f.root, endpoint: f.endpoint, pid: process.pid, identity: 'stale-birth-marker', token: 'a'.repeat(48) }));
  assert.equal((await f.run('ensure-ready')).status, 'ready');
  assert.ok(process.kill(process.pid, 0));
  assert.equal((await f.launched()).length, 1);
});
test('unknown lock ownership is preserved and deadline is bounded', async t => {
  const f = await fixture(t);
  await mkdir(f.lock, { recursive: true });
  const result = await f.run('ensure-ready', { XERJ_READY_TIMEOUT_MS: '450' });
  assert.equal(result.status, 'unavailable'); assert.equal(result.reason, 'timeout');
  assert.ok((await stat(f.lock)).isDirectory());
  assert.equal((await f.launched()).length, 0);
});
test('stale server PID record cannot stop a reused or unrelated process', async t => {
  const f = await fixture(t);
  await mkdir(f.directory, { recursive: true });
  await writeFile(f.record, JSON.stringify({ root: f.root, endpoint: f.endpoint, pid: process.pid, token: 'b'.repeat(48) }));
  assert.equal((await f.run('stop')).reason, 'not-managed');
  assert.ok(process.kill(process.pid, 0));
  assert.equal((await f.run('ensure-ready')).status, 'ready');
});
test('unrelated port listener is preserved, never replaced or stopped', async t => {
  const listener = createServer((_, response) => response.end('{"service":"other"}'));
  await new Promise(resolve => listener.listen(0, '127.0.0.1', resolve));
  t.after(() => { listener.closeAllConnections(); listener.close(); });
  const f = await fixture(t, 'healthy', listener.address().port);
  assert.equal((await f.run('ensure-ready')).reason, 'endpoint-in-use');
  assert.equal((await f.run('stop')).reason, 'not-managed');
  assert.equal((await f.launched()).length, 0);
  assert.ok(listener.listening);
});
test('compatible adopted backend is reused and explicit stop refuses it', async t => {
  const listener = createServer((request, response) => {
    assert.equal(request.headers.authorization, `ApiKey ${secret}`);
    response.end(request.url === '/_cluster/health'
      ? JSON.stringify({ cluster_name: 'xerj', status: 'green', timed_out: false, number_of_nodes: 1 }) : '[]');
  });
  await new Promise(resolve => listener.listen(0, '127.0.0.1', resolve));
  t.after(() => { listener.closeAllConnections(); listener.close(); });
  const f = await fixture(t, 'healthy', listener.address().port);
  const result = await f.run('ensure-ready', { XERJ_API_KEY: secret });
  assert.equal(result.status, 'ready'); assert.equal(result.ownership, 'adopted');
  assert.equal((await f.run('status', { XERJ_API_KEY: secret })).ownership, 'adopted');
  assert.equal((await f.run('stop')).reason, 'not-managed');
  assert.equal((await f.launched()).length, 0);
  assert.ok(listener.listening);
});
for (const mode of ['timeout', 'exit']) test(`failed ${mode} launch leaves no owned orphan`, async t => {
  const f = await fixture(t, mode);
  const began = performance.now();
  const result = await f.run('ensure-ready', { XERJ_READY_TIMEOUT_MS: '1200' });
  assert.equal(result.status, 'unavailable');
  assert.ok(performance.now() - began < 2200);
  assert.equal((await f.launched()).length, 1);
  await f.noServers();
  assert.ok(!(await readdir(f.directory)).some(p => p.endsWith('.sock') || p.endsWith('.lock') || p.endsWith('.json')));
});
test('cancelled startup cleans only its own attempted launch', async t => {
  const f = await fixture(t, 'timeout');
  assert.equal((await f.run('ensure-ready', {}, true)).reason, 'cancelled');
  await f.noServers();
});
test('independent managed roots keep lifecycle and shutdown isolated', async t => {
  const first = await fixture(t), second = await fixture(t);
  assert.equal((await first.run('ensure-ready')).status, 'ready');
  assert.equal((await second.run('ensure-ready')).status, 'ready');
  assert.equal((await first.run('stop')).status, 'stopped');
  await first.noServers();
  assert.equal((await second.run('probe')).status, 'ready');
});
test('status is read-only when no backend or managed data exists', async t => {
  const f = await fixture(t);
  assert.equal((await f.run('status')).status, 'unavailable');
  await assert.rejects(stat(join(f.root, 'runtime')), { code: 'ENOENT' });
  assert.equal((await f.launched()).length, 0);
});
test('failed authenticated startup is reaped rather than retained', async t => {
  const f = await fixture(t);
  assert.equal((await f.run('ensure-ready', { XERJ_AUTH: 'ApiKey wrong-test-key' })).reason, 'authentication-failed');
  await f.noServers();
  assert.equal((await f.launched()).length, 1);
});
test('explicit stop reaps an owned child that ignores graceful termination', async t => {
  const f = await fixture(t, 'ignore-term');
  assert.equal((await f.run('ensure-ready')).status, 'ready');
  assert.equal((await f.run('stop')).status, 'stopped');
  await f.noServers();
});
test('a second managed root cannot replace another roots listener', async t => {
  const first = await fixture(t);
  const second = await fixture(t, 'healthy', Number(new URL(first.endpoint).port));
  assert.equal((await first.run('ensure-ready')).status, 'ready');
  assert.equal((await second.run('ensure-ready')).reason, 'endpoint-in-use');
  assert.equal((await second.launched()).length, 0);
  assert.equal((await first.run('probe')).status, 'ready');
});
test('live lock identity is preserved without startup or signals', async t => {
  const f = await fixture(t);
  if (process.platform !== 'linux') { t.skip('Linux process identity fixture'); return; }
  const record = await readFile(`/proc/${process.pid}/stat`, 'utf8');
  const boot = await readFile('/proc/sys/kernel/random/boot_id', 'utf8');
  const identity = `${boot.trim()}:${record.slice(record.lastIndexOf(')') + 2).split(' ')[19]}`;
  await mkdir(f.lock, { recursive: true });
  await writeFile(join(f.lock, 'owner.json'), JSON.stringify({ root: f.root, endpoint: f.endpoint, pid: process.pid, identity, token: 'c'.repeat(48) }));
  assert.equal((await f.run('ensure-ready', { XERJ_READY_TIMEOUT_MS: '450' })).reason, 'timeout');
  assert.ok((await stat(f.lock)).isDirectory());
  assert.equal((await f.launched()).length, 0);
});
test('malformed or unauthorized control requests cannot disrupt a shared backend', async t => {
  const f = await fixture(t);
  assert.equal((await f.run('ensure-ready')).status, 'ready');
  const socketPath = join(f.directory, (await readdir(f.directory)).find(name => name.endsWith('.sock')));
  assert.equal((await stat(socketPath)).mode & 0o777, 0o600);
  for (const input of ['null\n', '{invalid\n', '{"token":"wrong","method":"stop"}\n']) {
    await new Promise((resolve, reject) => {
      const client = net.createConnection(socketPath);
      client.on('error', reject); client.on('close', resolve);
      client.on('connect', () => client.write(input));
    });
  }
  assert.equal((await f.run('status')).ownership, 'managed');
  assert.equal((await f.run('stop')).status, 'stopped');
  await f.noServers();
});
test('explicit stop waits for ownership acknowledgment within its overall deadline', async t => {
  const f = await fixture(t);
  await mkdir(f.directory, { recursive: true, mode: 0o700 });
  const token = 'd'.repeat(48);
  await writeFile(f.record, JSON.stringify({ root: f.root, endpoint: f.endpoint, token }), { mode: 0o600 });
  const owner = net.createServer(client => {
    let input = '';
    client.on('data', chunk => {
      input += chunk;
      if (!input.includes('\n')) return;
      const request = JSON.parse(input.trim());
      assert.equal(request.token, token);
      if (request.method === 'info') client.end(JSON.stringify({ root: f.root, endpoint: f.endpoint, serverAlive: true }) + '\n');
      else setTimeout(() => client.end('{"stopped":true}\n'), 300);
    });
  });
  await new Promise(resolve => owner.listen(f.record.replace(/\.json$/, '.sock'), resolve));
  try {
    const began = performance.now();
    assert.equal((await f.run('stop')).status, 'stopped');
    assert.ok(performance.now() - began >= 300);
  } finally { await new Promise(resolve => owner.close(resolve)); }
});
