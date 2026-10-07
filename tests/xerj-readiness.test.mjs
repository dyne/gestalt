import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

const manager = new URL('../public/gestalt', import.meta.url).pathname;
const secret = 'do-not-print-fixture-secret';
async function snapshot(root) {
  const result = {};
  async function visit(path) {
    for (const item of await readdir(path, { withFileTypes: true })) {
      const child = join(path, item.name);
      if (item.isDirectory()) await visit(child);
      else result[child] = (await readFile(child)).toString('base64');
    }
  }
  await visit(root);
  return result;
}
async function fixture(t, mode = 'healthy') {
  const temp = await mkdtemp(join(tmpdir(), 'gestalt readiness '));
  const root = join(temp, 'managed home'), pidFile = join(temp, 'probe.pid');
  await mkdir(join(root, 'xerj'), { recursive: true });
  await mkdir(join(root, 'runtime', 'xerj'), { recursive: true });
  await writeFile(join(root, 'runtime', 'xerj', 'admin.key'), secret);
  const binary = join(root, 'xerj', 'xerj');
  await writeFile(binary, `#!${process.execPath}
const fs = require('node:fs');
const mode = ${JSON.stringify(mode)};
fs.writeFileSync(${JSON.stringify(pidFile)}, String(process.pid));
if (process.argv[2] === '--version') {
  if (mode === 'version-timeout') setInterval(() => {}, 1000);
  else console.log(mode === 'unsupported' ? 'xerj v0.0.0' : 'xerj v1.0.0-rc.87');
} else {
  console.error(${JSON.stringify(secret)});
  if (mode === 'eof') process.exit(0);
  const readline = require('node:readline').createInterface({ input: process.stdin });
  readline.on('line', line => {
    const request = JSON.parse(line);
    if (!request.id) return;
    if (['timeout', 'cancel'].includes(mode)) return;
    if (mode === 'malformed') return console.log('invalid ${secret}');
    if (mode === 'partial') return process.stdout.write('{');
    const result = request.method === 'initialize'
      ? { protocolVersion: mode === 'protocol' ? 'bad' : '2025-03-26', capabilities: { tools: {} }, serverInfo: { name: 'xerj-mcp', version: '1.0.0-rc.87' } }
      : { tools: (mode === 'tools' ? ['xerj_map'] : ['xerj_search', 'xerj_map', 'xerj_code_search']).map(name => ({ name, inputSchema: { type: 'object' } })) };
    console.log(JSON.stringify({ jsonrpc: '2.0', id: request.id, result }));
  });
  readline.on('close', () => { if (mode === 'hang-eof') setInterval(() => {}, 1000); });
}
`, { mode: 0o755 });
  const reads = [];
  const server = createServer((request, response) => {
    reads.push({ method: request.method, url: request.url, auth: request.headers.authorization });
    if (mode === 'backend-timeout') return;
    if (mode === 'bad-auth' || request.headers.authorization !== `ApiKey ${secret}`) {
      response.writeHead(401); response.end(secret); return;
    }
    response.setHeader('Content-Type', 'application/json');
    if (mode === 'wrong-service') response.end('{"hello":"world"}');
    else if (request.url === '/_cluster/health') response.end(JSON.stringify({ cluster_name: 'xerj', status: 'green', timed_out: false, number_of_nodes: 1 }));
    else if (request.url === '/_cat/indices?format=json&h=index') response.end(JSON.stringify(mode === 'empty' ? [] : [{ index: 'fixture-index' }]));
    else { response.writeHead(404); response.end('{}'); }
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const endpoint = `http://127.0.0.1:${server.address().port}`;
  const before = await snapshot(root);
  t.after(async () => { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); await rm(temp, { recursive: true, force: true }); });
  async function run(overrides = {}, cancel = false) {
    const env = { HOME: process.env.HOME, PATH: process.env.PATH, GESTALT_HOME: root, XERJ_URL: endpoint, XERJ_READY_TIMEOUT_MS: '750', ...overrides };
    const child = spawn('bash', [manager, 'xerj', 'probe'], { env });
    let stdout = '', stderr = '';
    child.stdout.on('data', chunk => stdout += chunk);
    child.stderr.on('data', chunk => stderr += chunk);
    if (cancel) {
      for (let attempt = 0; attempt < 100; attempt++) {
        try { await readFile(pidFile); break; } catch { await new Promise(resolve => setTimeout(resolve, 5)); }
      }
      child.kill('SIGTERM');
    }
    const began = performance.now();
    const code = await new Promise(resolve => child.on('close', resolve));
    assert.equal(code, 0, stderr);
    assert.equal(stderr, '');
    assert.ok(!stdout.includes(secret));
    assert.ok(performance.now() - began < 1600, 'overall deadline exceeded');
    const result = JSON.parse(stdout);
    assert.equal(result.schemaVersion, 1);
    assert.deepEqual(await snapshot(root), before, 'probe wrote managed state');
    try {
      const pid = Number(await readFile(pidFile, 'utf8'));
      assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' }, 'retained MCP probe');
    } catch (error) { if (error.code !== 'ENOENT') throw error; }
    return result;
  }
  return { root, binary, run, reads, before, endpoint };
}

for (const mode of ['healthy', 'empty']) test(`verified ${mode} backend, private read-only probe and paths with spaces`, async t => {
  const f = await fixture(t, mode);
  const result = await f.run();
  assert.equal(result.status, 'ready');
  assert.equal(result.endpoint, f.endpoint);
  assert.equal(result.binary, f.binary);
  assert.equal(result.auth.keyFile, join(f.root, 'runtime', 'xerj', 'admin.key'));
  assert.deepEqual(f.reads.map(r => [r.method, r.url]), [['GET', '/_cluster/health'], ['GET', '/_cat/indices?format=json&h=index']]);
  assert.ok(f.reads.every(r => r.auth === `ApiKey ${secret}`));
});
for (const [mode, reason] of [
  ['unsupported', 'unsupported-version'], ['version-timeout', 'timeout'], ['wrong-service', 'invalid-backend'],
  ['bad-auth', 'authentication-failed'], ['malformed', 'invalid-mcp'], ['partial', 'timeout'],
  ['eof', 'invalid-mcp'], ['timeout', 'timeout'], ['hang-eof', 'timeout'],
  ['protocol', 'incompatible-mcp'], ['tools', 'incompatible-mcp'], ['backend-timeout', 'timeout']
]) test(`bounded unavailable: ${mode}`, async t => {
  const f = await fixture(t, mode);
  assert.deepEqual(await f.run(), { schemaVersion: 1, status: 'unavailable', reason });
});
test('tool discovery alone cannot prove backend readiness', async t => {
  const f = await fixture(t);
  assert.deepEqual(await f.run({ XERJ_URL: 'http://127.0.0.1:1' }), { schemaVersion: 1, status: 'unavailable', reason: 'backend-unavailable' });
});
test('missing credential never falls back to user home or cwd', async t => {
  const f = await fixture(t);
  await rm(join(f.root, 'runtime', 'xerj', 'admin.key'));
  delete f.before[join(f.root, 'runtime', 'xerj', 'admin.key')];
  assert.deepEqual(await f.run(), { schemaVersion: 1, status: 'unavailable', reason: 'credentials-unavailable' });
});
test('missing managed binary is optional absence', async t => {
  const f = await fixture(t);
  await rm(f.binary); delete f.before[f.binary];
  assert.deepEqual(await f.run(), { schemaVersion: 1, status: 'absent' });
});
test('unsafe endpoint and invalid deadline are bounded structured failures', async t => {
  const f = await fixture(t);
  assert.equal((await f.run({ XERJ_URL: `http://${secret}@remote.example:9200` })).reason, 'unsafe-endpoint');
  assert.equal((await f.run({ XERJ_READY_TIMEOUT_MS: 'oops' })).reason, 'invalid-deadline');
});
test('cancellation reaps its probe without secret diagnostics', async t => {
  const f = await fixture(t, 'cancel');
  assert.deepEqual(await f.run({}, true), { schemaVersion: 1, status: 'unavailable', reason: 'cancelled' });
});
