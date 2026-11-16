import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { once } from 'node:events';
import { createServer, request } from 'node:http';
import { createHash } from 'node:crypto';
import { authorizeFixtureHooks } from './fixtures/native-cli-hook-trust.mjs';
import { assertMetadataProbesOnly, captureProviderProbes, providerProbeViolation } from './fixtures/native-provider-probes.mjs';

const expected = [
  { kind: 'verified-installed-context-mode', sourcePath: '/isolated/Codex/hooks.json', eventName: 'sessionStart', command: 'exact accepted managed command' },
  { kind: 'explicit-blocking-fixture', sourcePath: '/isolated/Codex/config.toml', eventName: 'sessionStart', command: 'exact isolated fixture command' },
];
const hooks = () => expected.map((entry, index) => ({ ...entry, source: 'user', enabled: true, key: entry.sourcePath + ':' + index,
  currentHash: 'sha256:' + String(index).repeat(64), trustStatus: 'untrusted' }));

test('fixture authorization selects only the exact verified commands, events and source files', () => {
  const selected = authorizeFixtureHooks(hooks(), expected);
  assert.deepEqual(selected.map(value => value.kind), expected.map(value => value.kind));
  assert.deepEqual(selected.map(value => value.key), hooks().map(value => value.key));
});
for (const fault of ['extra', 'command', 'source', 'event', 'disabled', 'duplicate-key']) test('fixture authorization refuses ' + fault, () => {
  const actual = hooks();
  if (fault === 'extra') actual.push({ ...actual[0], key: 'foreign', command: 'unverified' });
  if (fault === 'command') actual[0].command += ' changed';
  if (fault === 'source') actual[0].sourcePath = '/operator/live/hooks.json';
  if (fault === 'event') actual[0].eventName = 'userPromptSubmit';
  if (fault === 'disabled') actual[0].enabled = false;
  if (fault === 'duplicate-key') actual[1].key = actual[0].key;
  assert.throws(() => authorizeFixtureHooks(actual, expected));
});

for (const writable of [true, false]) test(`passive spawn capture preserves literal argv, binary streams and exit with capture ${writable ? 'available' : 'unavailable'}`, async t => {
  const fixture = await mkdtemp(path.join(tmpdir(), 'core-slice-capture-'));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const capture = writable ? path.join(fixture, 'calls.jsonl') : path.join(fixture, 'missing-parent/calls.jsonl');
  const preload = pathToFileURL(path.resolve(import.meta.dirname, 'fixtures/native-cli-spawn-capture.mjs')).href;
  const child = `const input=[];for await(const b of process.stdin)input.push(b);process.stdout.write(Buffer.concat(input));process.stderr.write(JSON.stringify(process.argv.slice(1)));process.exitCode=23;`;
  const program = `import{spawn}from'node:child_process';const child=spawn(process.execPath,['--input-type=module','-e',${JSON.stringify(child)},'literal ü & $HOME',''],{stdio:['pipe','pipe','pipe']});process.stdin.pipe(child.stdin);child.stdout.pipe(process.stdout);child.stderr.pipe(process.stderr);child.on('exit',code=>process.exitCode=code);`;
  const input = Buffer.from([0, 255, 13, 10, 128]);
  const result = spawnSync(process.execPath, ['--import=' + preload, '--input-type=module', '-e', program],
    { env: { ...process.env, GESTALT_SLICE_CODEX_BINARY: process.execPath, GESTALT_SLICE_SPAWN_CAPTURE: capture }, input, timeout: 10000 });
  assert.equal(result.status, 23, result.stderr.toString()); assert.deepEqual(result.stdout, input);
  assert.deepEqual(JSON.parse(result.stderr), ['literal ü & $HOME', '']);
  if (writable) {
    const calls = (await readFile(capture, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
    assert.equal(calls.length, 1); assert.equal(calls[0].target, process.execPath); assert.deepEqual(calls[0].argv.slice(-2), ['literal ü & $HOME', '']);
  }
});

const probe = (method, path, extra = {}) => ({ control: 'eof', method, path, bodyBytes: 0, bodyComplete: true,
  contentLength: null, transferEncoding: null, blockedHeaderNames: [], ...extra });
test('provider fixture permits only the pinned empty doctor metadata sequence', () => {
  const records = [probe('HEAD', '/responses'), probe('GET', '/models')];
  assert.deepEqual(assertMetadataProbesOnly(records, 'eof'), { control: 'eof', metadataProbes: 2, generationOrOtherRequests: 0 });
  assert.deepEqual(assertMetadataProbesOnly(records, 'interrupt'), { control: 'interrupt', metadataProbes: 0, generationOrOtherRequests: 0 });
});
for (const [name, record] of [
  ['generation POST', probe('POST', '/responses')], ['query', probe('GET', '/models?prompt=synthetic')],
  ['body', probe('GET', '/models', { bodyBytes: 1 })], ['partial', probe('HEAD', '/responses', { bodyComplete: false })],
  ['declared body', probe('HEAD', '/responses', { contentLength: '1' })], ['chunked', probe('HEAD', '/responses', { transferEncoding: 'chunked' })],
  ['prompt header', probe('HEAD', '/responses', { blockedHeaderNames: ['x-prompt'] })], ['outside case', probe('HEAD', '/responses', { control: null })],
]) test('provider fixture refuses ' + name, () => { assert.notEqual(providerProbeViolation(record), null); assert.throws(() => assertMetadataProbesOnly([record], 'eof')); });
test('provider fixture refuses repeated probes and a catalog request without the doctor HEAD', () => {
  assert.throws(() => assertMetadataProbesOnly([probe('HEAD', '/responses'), probe('HEAD', '/responses')], 'eof'));
  assert.throws(() => assertMetadataProbesOnly([probe('GET', '/models')], 'eof'));
});
test('inert provider captures actual method/path/body bytes/hash/times and rejects synthetic prompt data', { timeout: 10000 }, async () => {
  const records = [], server = createServer(captureProviderProbes(records, () => 'eof'));
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const send = (method, path, body = '') => new Promise((resolve, reject) => {
    const req = request({ hostname: '127.0.0.1', port: server.address().port, method, path, headers: { 'content-length': Buffer.byteLength(body) } },
      res => { res.resume(); res.once('end', resolve); }); req.once('error', reject); req.end(body);
  });
  try {
    await send('HEAD', '/responses'); await send('GET', '/models'); assertMetadataProbesOnly(records, 'eof');
    await send('POST', '/responses', 'synthetic prompt fixture');
    assert.equal(records[2].bodyBytes, Buffer.byteLength('synthetic prompt fixture'));
    assert.equal(records[2].bodySha256, createHash('sha256').update('synthetic prompt fixture').digest('hex'));
    assert.equal(records[2].bodyComplete, true); assert.ok(Date.parse(records[2].receivedAt) <= Date.parse(records[2].completedAt));
    assert.throws(() => assertMetadataProbesOnly(records, 'eof'));
  } finally { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); }
});
