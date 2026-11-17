import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { once } from 'node:events';
import { createServer, request } from 'node:http';
import { createHash } from 'node:crypto';
import { authorizeFixtureHooks } from './fixtures/native-cli-hook-trust.mjs';
import { assertMetadataProbesOnly, captureProviderProbes, providerProbeViolation } from './fixtures/native-provider-probes.mjs';
import { terminalControls } from './fixtures/native-cli-slice-controls.mjs';
import { createReadinessObserver } from './fixtures/native-cli-spawn-capture.mjs';
import { selectedSliceMode, readinessArgv, finalCapturedInvocation, readinessObservations,
  selectedReadinessComparison, observerFreeEnvironment, readinessComparison } from './native-core-slice.mjs';
import { EventEmitter } from 'node:events';

const wire = (kind, data = '') => {
  const bytes = Buffer.from(data), packet = Buffer.alloc(5 + bytes.length);
  packet.writeUInt32LE(1 + bytes.length); packet[4] = kind.charCodeAt(0); bytes.copy(packet, 5); return packet;
};
test('readiness selection is explicit Windows-only and exercises nonadministrative exec before help', () => {
  assert.equal(selectedSliceMode({}), 'terminal'); assert.equal(selectedSliceMode({ GESTALT_CORE_SLICE_READINESS_ONLY: '0' }), 'terminal');
  assert.equal(selectedSliceMode({ GESTALT_CORE_SLICE_READINESS_ONLY: '1' }, 'win32'), 'readiness');
  for (const flag of ['true', 'yes', '2']) assert.throws(() => selectedSliceMode({ GESTALT_CORE_SLICE_READINESS_ONLY: flag }, 'win32'));
  assert.throws(() => selectedSliceMode({ GESTALT_CORE_SLICE_READINESS_ONLY: '1' }, 'linux'));
  assert.deepEqual(readinessArgv('/fixture/gestalt.mjs'), ['/fixture/gestalt.mjs', 'cli', '--', 'exec', '--help']);
  assert.throws(() => finalCapturedInvocation([{ argv: ['--help'] }], 'readiness'));
  const final = { argv: ['--no-daemon', 'exec', '--help'] };
  assert.equal(finalCapturedInvocation([{ argv: ['app-server', '--stdio'] }, final], 'readiness'), final);
  assert.throws(() => finalCapturedInvocation([final, final], 'readiness'));
});
test('comparator selection requires its own explicit Windows readiness flag and stays off normally', () => {
  assert.equal(selectedReadinessComparison({}, 'readiness', 'win32'), false);
  assert.equal(selectedReadinessComparison({ GESTALT_CORE_SLICE_READINESS_COMPARISON: '0' }, 'readiness', 'win32'), false);
  assert.equal(selectedReadinessComparison({ GESTALT_CORE_SLICE_READINESS_COMPARISON: '1' }, 'readiness', 'win32'), true);
  for (const [flag, mode, platform] of [['yes', 'readiness', 'win32'], ['1', 'terminal', 'win32'], ['1', 'readiness', 'linux']])
    assert.throws(() => selectedReadinessComparison({ GESTALT_CORE_SLICE_READINESS_COMPARISON: flag }, mode, platform));
});
test('comparator removes only the exact observer import and capture inputs while preserving owner TMP and policy environment', () => {
  const observerImport = 'file:///fixture/spawn-capture.mjs', observed = {
    NODE_OPTIONS: '--import=' + observerImport, GESTALT_SLICE_CODEX_BINARY: '/fixture/codex', GESTALT_SLICE_SPAWN_CAPTURE: '/fixture/argv.jsonl', GESTALT_SLICE_READINESS_CAPTURE: '/fixture/observation.json',
    GESTALT_SESSION_TMP_CONTEXT: 'exact context bytes', TMPDIR: '/fixture/tmp', TMP: '/fixture/tmp', TEMP: '/fixture/tmp',
    CODEX_HOME: '/fixture/profile', GESTALT_HOME: '/fixture/install', GESTALT_SLICE_CONFIGURATION: '/fixture/configuration.json', PATH: '/fixture/native', RETAINED: 'literal ü',
  }, before = { ...observed }, { environment, changes } = observerFreeEnvironment(observed, observerImport);
  assert.deepEqual(observed, before); assert.equal(environment.NODE_OPTIONS, '');
  assert.deepEqual(changes.map(change => change.key), ['GESTALT_SLICE_CODEX_BINARY', 'GESTALT_SLICE_READINESS_CAPTURE', 'GESTALT_SLICE_SPAWN_CAPTURE', 'NODE_OPTIONS']);
  for (const [key, value] of Object.entries(observed)) if (!changes.some(change => change.key === key)) assert.equal(environment[key], value);
  assert.throws(() => observerFreeEnvironment({ ...observed, NODE_OPTIONS: '--require=other --import=' + observerImport }, observerImport));
});
test('comparator structured failure is retained and observed readiness still executes exactly once afterwards', async () => {
  const order = [], saved = [];
  const result = await readinessComparison(async control => { order.push(control); if (control === 'comparator') throw Error('controlled measurement failure'); },
    async record => { saved.push(structuredClone(record)); });
  assert.deepEqual(order, ['comparator', 'readiness']); assert.equal(result.pass, false);
  assert.match(saved[0].failure.message, /controlled measurement/); assert.equal(saved[0].completed, false); assert.equal(saved[1].completed, true);
  assert.equal(result.plannedQueryInvocations, 2); assert.equal(result.submittedTurns, 0); assert.equal(result.terminalCasesExecuted, 0);
});
test('comparator missing result capture cannot pass or prevent the unchanged observed readiness failure from being retained', async () => {
  const order = [];
  const result = await readinessComparison(async control => { order.push(control); if (control === 'readiness') throw Error('controlled original readiness gate'); },
    async record => { if (record.control === 'comparator') throw Error('controlled missing capture'); });
  assert.deepEqual(order, ['comparator', 'readiness']); assert.equal(result.pass, false);
  assert.equal(result.cases[0].completed, false); assert.match(result.cases[0].captureFailure.message, /missing capture/);
  assert.equal(result.cases[1].completed, false); assert.match(result.cases[1].failure.message, /original readiness gate/);
});
test('observer-free comparator child has no test import or capture files and preserves actual output status and inherited bytes', async t => {
  const fixture = await mkdtemp(path.join(tmpdir(), 'readiness-comparator-')); t.after(() => rm(fixture, { recursive: true, force: true }));
  const observerImport = pathToFileURL(path.resolve(import.meta.dirname, 'fixtures/native-cli-spawn-capture.mjs')).href;
  const capture = path.join(fixture, 'argv.jsonl'), observation = path.join(fixture, 'observation.json');
  const { environment } = observerFreeEnvironment({ ...process.env, NODE_OPTIONS: '--import=' + observerImport,
    GESTALT_SLICE_CODEX_BINARY: process.execPath, GESTALT_SLICE_SPAWN_CAPTURE: capture, GESTALT_SLICE_READINESS_CAPTURE: observation,
    GESTALT_SESSION_TMP_CONTEXT: 'controlled exact context', TMPDIR: fixture, TMP: fixture, TEMP: fixture }, observerImport);
  const program = `const p=process.env;if(p.NODE_OPTIONS!==''||p.GESTALT_SLICE_CODEX_BINARY!==undefined||p.GESTALT_SLICE_SPAWN_CAPTURE!==undefined||p.GESTALT_SLICE_READINESS_CAPTURE!==undefined)throw Error('observer enabled');process.stdout.write(Buffer.from([0,255,13,10]));process.stderr.write(JSON.stringify([p.GESTALT_SESSION_TMP_CONTEXT,p.TMPDIR,p.TMP,p.TEMP]));process.exitCode=23;`;
  const child = spawnSync(process.execPath, ['-e', program], { env: environment, timeout: 10000 });
  assert.equal(child.status, 23); assert.deepEqual(child.stdout, Buffer.from([0, 255, 13, 10]));
  assert.deepEqual(JSON.parse(child.stderr), ['controlled exact context', fixture, fixture, fixture]);
  await assert.rejects(readFile(capture), { code: 'ENOENT' }); await assert.rejects(readFile(observation), { code: 'ENOENT' });
});
test('readiness observer preserves read/write/emit identities, callbacks, bytes and listener counts without extra reads', () => {
  const observer = createReadinessObserver(), child = new EventEmitter(), chunks = []; let reads = 0, callbackCalls = 0;
  const callback = () => { callbackCalls++; }, written = [];
  child.stdout = { read(...args) { assert.equal(this, child.stdout); assert.deepEqual(args, [17]); reads++; return chunks.shift() ?? null; } };
  child.stdin = { write(...args) { assert.equal(this, child.stdin); written.push(args); args.at(-1)(); return false; } };
  let closes = 0; child.on('close', (code, signal) => { assert.equal(code, 0); assert.equal(signal, null); closes++; });
  const listeners = child.listenerCount('close');
  observer.observeHelper(child, { argv: ['app-server', '--stdio'], target: '/fixture/codex', transport: 'accepted-owned-helper' }, {});
  assert.equal(reads, 0); assert.equal(child.listenerCount('close'), listeners);
  const input = wire('I', '{"id":1,"method":"initialize"}\n');
  assert.equal(child.stdin.write(input, 'utf8', callback), false);
  assert.equal(written[0][0], input); assert.equal(written[0][2], callback); assert.equal(callbackCalls, 1);
  const output = Buffer.concat([wire('R', 'windows-job'), wire('O', '{"id":1,"result":{"text":"東京"}}\n'), wire('F', 'exit:0'), wire('D', 'windows-job')]);
  for (let i = 0; i < output.length; i++) { const chunk = output.subarray(i, i + 1); chunks.push(chunk); assert.equal(child.stdout.read(17), chunk); }
  assert.equal(reads, output.length);
  assert.equal(child.emit('close', 0, null), true); assert.equal(closes, 1); assert.equal(child.listenerCount('close'), listeners);
  const report = observer.report(); assert.deepEqual(report.errors, []); assert.equal(report.overflow, false);
  assert.deepEqual(report.channels[0].incompleteFrameBytes, [0, 0]); assert.deepEqual(report.channels[0].incompleteRpcBytes, [0, 0]);
  assert.deepEqual(report.events.filter(e => e.type === 'native-frame').map(e => e.kind), ['R', 'F', 'D']);
  assert.equal(report.events.filter(e => e.type === 'rpc' && e.direction === 'output').length, 1);
});
test('readiness observer preserves synchronous return/throw and native timer callback/handle/delay', () => {
  const observer = createReadinessObserver(), receiver = {}, payload = Buffer.from([0, 255]);
  const sync = observer.sync(function (...args) { assert.equal(this, receiver); assert.deepEqual(args, ['native', ['literal', '']]); return payload; }, 'controlled');
  assert.equal(sync.call(receiver, 'native', ['literal', '']), payload);
  const failure = { get message() { throw Error('observer getter must not replace original exception'); } };
  assert.throws(() => observer.sync(() => { throw failure; }, 'controlled')('native'), e => e === failure);
  const handle = Object.freeze({ native: true }), token = Symbol('clear'), callback = () => {};
  const set = observer.setTimeout(function (...args) { assert.equal(this, receiver); assert.equal(args[0], callback); assert.deepEqual(args.slice(1), [5000, payload]); return handle; });
  assert.equal(set.call(receiver, callback, 5000, payload), handle);
  const clear = observer.clearTimeout(function (actual) { assert.equal(this, receiver); assert.equal(actual, handle); return token; });
  assert.equal(clear.call(receiver, handle), token);
  assert.equal(observer.report().events.find(e => e.type === 'timer-cleared').provesNonexpiry, false);
});
test('readiness observer follows existing async-iterator reads on real Node pipes with a controlled frame producer', async t => {
  const fixture = await mkdtemp(path.join(tmpdir(), 'readiness-pipe-')); t.after(() => rm(fixture, { recursive: true, force: true }));
  const file = path.join(fixture, 'input.bin'), input = wire('I', '{"id":1,"method":"initialize"}\n');
  const output = Buffer.concat([wire('R', 'windows-job'), wire('O', '{"id":1,"result":{}}\n'), wire('F', 'exit:0'), wire('D', 'windows-job')]);
  const program = `const chunks=[];for await(const b of process.stdin)chunks.push(b);(await import('node:fs')).writeFileSync(${JSON.stringify(file)},Buffer.concat(chunks));process.stderr.write('controlled diagnostic\\n');process.stdout.write(Buffer.from(${JSON.stringify(output.toString('base64'))},'base64'));`;
  const child = spawn(process.execPath, ['--input-type=module', '-e', program], { stdio: ['pipe', 'pipe', 'pipe'] }), closed = once(child, 'close');
  const diagnostics = []; child.stderr.on('data', bytes => diagnostics.push(bytes)); const listeners = child.stderr.listenerCount('data');
  const observer = createReadinessObserver(); observer.observeHelper(child, { argv: ['app-server', '--stdio'], target: 'controlled-node-producer', transport: 'controlled-fixture' }, {});
  assert.equal(child.stderr.listenerCount('data'), listeners);
  child.stdin.write(input); child.stdin.end(); const chunks = []; for await (const chunk of child.stdout) chunks.push(chunk);
  assert.deepEqual(await closed, [0, null]); assert.deepEqual(Buffer.concat(chunks), output); assert.deepEqual(await readFile(file), input);
  const report = observer.report(); assert.deepEqual(report.errors, []);
  assert.deepEqual(report.events.filter(e => e.type === 'native-frame').map(e => e.kind), ['R', 'F', 'D']);
  assert.equal(report.events.filter(e => e.type === 'rpc').length, 2); assert.equal(report.events.filter(e => e.type === 'helper-close').length, 1);
  assert.deepEqual(report.channels[0].incompleteFrameBytes, [0, 0]);
  assert.equal(Buffer.concat(diagnostics).toString(), 'controlled diagnostic\n');
  assert.equal(Buffer.concat(report.events.filter(e => e.type === 'helper-stderr').map(e => Buffer.from(e.payloadBase64, 'base64'))).toString(), 'controlled diagnostic\n');
});
test('readiness observer overflow is bounded and cannot become complete positive evidence', () => {
  const observer = createReadinessObserver({ maxBytes: 64, maxRecords: 2 });
  observer.argv({ argv: ['x'.repeat(1000)] }); observer.observeHelper({}, { argv: [] }, {});
  const report = observer.report(); assert.equal(report.overflow, true); assert.ok(report.usedBytes <= 64);
  assert.equal(report.argvCalls.length, 0); assert.equal(report.channels.length, 0);
  assert.equal(readinessObservations(report, {}, '/fixture/codex').pass, false);
});

function controlledReadiness() {
  const inherited = { GESTALT_SESSION_TMP_CONTEXT: 'controlled-fixture', TMPDIR: '/fixture/tmp', TMP: '/fixture/tmp', TEMP: '/fixture/tmp' };
  const phases = ['required-inventory', 'required-mcp', 'capability-config', 'capability-hooks', 'final-help'];
  const observation = { overflow: false, errors: [], processExit: 0, channels: [], events: [] };
  for (const [index, phase] of phases.entries()) {
    const id = index + 1; observation.channels.push({ id, phase, call: { transport: 'accepted-owned-helper', target: '/fixture/codex' }, environment: { ...inherited }, incompleteFrameBytes: [0, 0], incompleteRpcBytes: [0, 0] });
    observation.events.push({ type: 'native-frame', channel: id, kind: 'R', payloadBase64: Buffer.from('windows-job').toString('base64') });
    if (phase.startsWith('capability-')) {
      const methods = phase === 'capability-config' ? ['initialize', 'config/read', 'skills/list'] : ['initialize', 'hooks/list'];
      for (const [rpcId, method] of methods.entries()) observation.events.push({ type: 'rpc', channel: id, direction: 'input', method, id: rpcId }, { type: 'rpc', channel: id, direction: 'output', id: rpcId, error: null, response: true, hasResult: true });
      observation.events.push({ type: 'native-control', channel: id, kind: 'S' });
    }
    observation.events.push({ type: 'native-frame', channel: id, kind: 'F', payloadBase64: Buffer.from(phase.startsWith('capability-') ? 'exit:137' : 'exit:0').toString('base64') },
      { type: 'native-frame', channel: id, kind: 'D', payloadBase64: Buffer.from('windows-job').toString('base64') }, { type: 'helper-close', channel: id, code: 0, signal: null });
  }
  for (let i = 0; i < 2; i++) observation.events.push({ type: 'timer-created', delay: 10000, callsite: 'at RuntimeCommands.capture (compiled-fixture:1)' },
    { type: 'timer-created', delay: 4990, callsite: 'at snapshot (compiled-fixture:2)' });
  return { observation, inherited };
}
test('controlled readiness gate distinguishes command captures, shared snapshots and raw records from lease claims', () => {
  const { observation, inherited } = controlledReadiness(), report = readinessObservations(observation, inherited, '/fixture/codex');
  assert.equal(report.pass, true); assert.equal(report.rawInnerRecordsAreLeaseClaims, false); assert.equal(report.timerClearProvesNonexpiry, false);
  assert.match(report.timerObservationScope, /Per-query5s cap only/);
  assert.match(report.timerObservationScope, /not an observed callback timestamp/);
});
for (const fault of ['missing-phase', 'context', 'missing-D', 'unknown', 'RPC', 'helper-close', 'timer-callsite', 'perquery-cap', 'missing-stop']) test('controlled readiness gate rejects ' + fault, () => {
  const { observation, inherited } = controlledReadiness();
  if (fault === 'missing-phase') observation.channels.pop();
  if (fault === 'context') observation.channels[2].environment.TMPDIR = '/ambient';
  if (fault === 'missing-D') observation.events = observation.events.filter(e => !(e.channel === 3 && e.kind === 'D'));
  if (fault === 'unknown') observation.events.push({ type: 'native-frame', channel: 3, kind: 'U' });
  if (fault === 'RPC') observation.events = observation.events.filter(e => !(e.channel === 3 && e.type === 'rpc' && e.direction === 'output'));
  if (fault === 'helper-close') observation.events.find(e => e.channel === 3 && e.type === 'helper-close').code = 2;
  if (fault === 'timer-callsite') for (const e of observation.events) if (e.type === 'timer-created') e.callsite = 'at launchNative (native-cli-spawn-capture.mjs:1)';
  if (fault === 'perquery-cap') for (const e of observation.events) if (e.type === 'timer-created' && e.callsite.includes('at snapshot ')) e.delay = 5001;
  if (fault === 'missing-stop') observation.events = observation.events.filter(e => e.type !== 'native-control');
  const report = readinessObservations(observation, inherited, '/fixture/codex');
  assert.equal(report.pass, false);
  if (fault === 'perquery-cap') assert.ok(report.failures.includes('per-query5s-snapshot-cap-timers-unobserved'));
});

test('readiness observer normal-off import leaves methods unchanged and readiness capture flushes only after critical work', async t => {
  const fixture = await mkdtemp(path.join(tmpdir(), 'readiness-observer-')); t.after(() => rm(fixture, { recursive: true, force: true }));
  const observer = pathToFileURL(path.resolve(import.meta.dirname, 'fixtures/native-cli-spawn-capture.mjs')).href;
  const off = `import cp from 'node:child_process';const before=[cp.spawn,cp.execFileSync,cp.spawnSync,setTimeout,clearTimeout];await import(${JSON.stringify(observer)});if(before.some((v,i)=>v!==[cp.spawn,cp.execFileSync,cp.spawnSync,setTimeout,clearTimeout][i]))throw Error('normal-off changed method');`;
  assert.equal(spawnSync(process.execPath, ['--input-type=module', '-e', off], { env: { ...process.env, GESTALT_SLICE_CODEX_BINARY: '', GESTALT_SLICE_SPAWN_CAPTURE: '', GESTALT_SLICE_READINESS_CAPTURE: '' } }).status, 0);
  const capture = path.join(fixture, 'argv.jsonl'), report = path.join(fixture, 'readiness.json'), bytes = Buffer.from([0, 255, 13, 10, 128]);
  const child = `const chunks=[];for await(const b of process.stdin)chunks.push(b);process.stdout.write(Buffer.concat(chunks));process.stderr.write(JSON.stringify(process.argv.slice(1)));process.exitCode=23;`;
  const program = `import{spawn}from'node:child_process';import{existsSync}from'node:fs';const c=spawn(process.execPath,['--input-type=module','-e',${JSON.stringify(child)},'literal ü',''],{stdio:['pipe','pipe','pipe']});process.stdin.pipe(c.stdin);c.stdout.pipe(process.stdout);c.stderr.pipe(process.stderr);c.on('close',code=>{if(existsSync(${JSON.stringify(capture)})||existsSync(${JSON.stringify(report)}))throw Error('observer wrote during critical work');process.exitCode=code});`;
  const result = spawnSync(process.execPath, ['--import=' + observer, '--input-type=module', '-e', program], { input: bytes, timeout: 10000,
    env: { ...process.env, GESTALT_SLICE_CODEX_BINARY: process.execPath, GESTALT_SLICE_SPAWN_CAPTURE: capture, GESTALT_SLICE_READINESS_CAPTURE: report } });
  assert.equal(result.status, 23, result.stderr.toString()); assert.deepEqual(result.stdout, bytes); assert.deepEqual(JSON.parse(result.stderr), ['literal ü', '']);
  const actual = JSON.parse(await readFile(report)); assert.equal(actual.processExit, 23); assert.equal(actual.overflow, false);
  assert.equal((await readFile(capture, 'utf8')).trim().split('\n').length, 1);
});
test('readiness observer abrupt termination leaves missing capture unknown rather than passing', async t => {
  const fixture = await mkdtemp(path.join(tmpdir(), 'readiness-abrupt-')); t.after(() => rm(fixture, { recursive: true, force: true }));
  const observer = pathToFileURL(path.resolve(import.meta.dirname, 'fixtures/native-cli-spawn-capture.mjs')).href;
  const capture = path.join(fixture, 'argv.jsonl'), report = path.join(fixture, 'readiness.json');
  const result = spawnSync(process.execPath, ['--import=' + observer, '-e', 'process.kill(process.pid,"SIGKILL")'], { timeout: 10000,
    env: { ...process.env, GESTALT_SLICE_CODEX_BINARY: process.execPath, GESTALT_SLICE_SPAWN_CAPTURE: capture, GESTALT_SLICE_READINESS_CAPTURE: report } });
  assert.notEqual(result.status, 0); await assert.rejects(readFile(report), { code: 'ENOENT' });
  assert.equal(readinessObservations(undefined, {}, '/fixture/codex').pass, false);
});

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
test('readiness provider label preserves the exact empty metadata sequence and rejects generation', () => {
  const records = [probe('HEAD', '/responses', { control: 'readiness' }), probe('GET', '/models', { control: 'readiness' })];
  assert.deepEqual(assertMetadataProbesOnly(records, 'readiness'), { control: 'readiness', metadataProbes: 2, generationOrOtherRequests: 0 });
  assert.throws(() => assertMetadataProbesOnly([...records, probe('POST', '/responses', { control: 'readiness' })], 'readiness'));
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

test('terminal controls wait for a fresh submitted-turn settled status before their only UI-quit Ctrl-C', () => {
  const controls = terminalControls('interrupt');
  const startup = '\x1b]7501;state=idle\x07 Native core slice no-model barrier';
  assert.equal(controls.observe(startup, true), null, 'Startup idle is not this submitted turn completion');
  const working = startup + '\x1b]7501;state=working\x07';
  assert.equal(controls.observe(working, true), null, 'Working must never trigger this UI-quit case');
  assert.equal(controls.observe(working + ' Conversation interrupted', true), null, 'Text is not a settled status');
  const done = working + '\x1b]7501;state=done\x1b\\';
  assert.equal(controls.observe(done, false), null, 'Actual authority and no-model barrier remain mandatory');
  assert.equal(controls.observe(done, true), '\x03', 'Separate Working and Done callbacks produce one quit only after Done');
  assert.equal(controls.observe(done + '\x1b]7501;state=idle\x1b\\\x1b]7501;state=clear\x1b\\', true), null);
  assert.equal(controls.events.length, 1); assert.equal(controls.events[0].intent, 'settled-ui-quit');
  assert.equal(controls.events[0].activeCancellationClaimed, false);
  assert.ok(controls.events[0].settledStatusOffset > controls.events[0].workingStatusOffset);
});

for (const state of ['idle', 'done']) test(`terminal controls quit once when the submitted turn is already ${state} in the same callback`, () => {
  const controls = terminalControls('interrupt');
  const before = '\x1b]7501;state=working\x07Native core slice no-model barrier\x1b]7501;state=' + state + '\x1b\\';
  assert.equal(controls.observe(before, true), '\x03');
  assert.equal(controls.events[0].stateBeforeControl, state);
  assert.equal(controls.events[0].awaitingSettledControl, false);
  assert.equal(controls.observe(before + '\x1b]7501;state=idle\x1b\\', true), null);
  assert.equal(controls.observe(before + '\x1b]7501;state=idle\x1b\\\x1b]7501;state=clear\x1b\\', true), null);
  assert.equal(controls.events.length, 1);
});

test('terminal controls preserve immediate EOF04 after authority/barrier without requiring settled status', () => {
  const eof = terminalControls('eof'), working = '\x1b]7501;state=working Native core slice no-model barrier';
  assert.equal(eof.observe(working, false), null); assert.equal(eof.observe(working, true), '\x04');
  assert.equal(eof.observe(working + '\x1b]7501;state=done', true), null);
  assert.equal(eof.events.length, 1); assert.equal(eof.events[0].action, 'eof');
});

for (const control of ['eof', 'interrupt']) test(`actual POSIX PTY fixture sends ${control} controls according to observed child state`,
  { skip: process.platform === 'win32' ? 'Windows uses the same tested JS state gate with ConPTY; no Python prerequisite' : false, timeout: 15000 }, async t => {
    const fixture = await mkdtemp(path.join(tmpdir(), 'core-slice-control-')); t.after(() => rm(fixture, { recursive: true, force: true }));
    const report = path.join(fixture, 'authority.json'), result = path.join(fixture, 'terminal.json');
    const program = `import{writeFileSync}from'node:fs';process.stdin.setRawMode(true);let submitted=false,interrupted=false;
      setTimeout(()=>process.exit(97),5000).unref();process.stdout.write('\\x1b]7501;state=idle\\x07');
      process.stdin.on('data',chunk=>{const s=chunk.toString();if(s.includes('\\r')){submitted=true;writeFileSync(${JSON.stringify(report)},'{}');process.stdout.write('Native core slice no-model barrier');}
      if(s.includes('\\x04'))process.exit(submitted?0:96);if(s.includes('\\x03\\x03'))process.exit(95);
      if(s.includes('\\x03')){if(!submitted)process.exit(94);if(interrupted)process.exit(0);interrupted=true;process.stdout.write('Conversation interrupted\\x1b]7501;state=idle\\x07');}});`;
    const request = path.join(fixture, 'request.json');
    await writeFile(request, JSON.stringify({ command: [process.execPath, '--input-type=module', '-e', program], cwd: fixture, environment: process.env, report, result, control }));
    const child = spawnSync('python3', [path.join(import.meta.dirname, 'fixtures/native-cli-slice-pty.py'), request], { timeout: 10000 });
    assert.equal(child.status, 0, child.stderr.toString());
    const record = JSON.parse(await readFile(result, 'utf8')); assert.equal(record.exit, 0); assert.equal(record.timedOut, false);
    assert.deepEqual(record.controlEvents.map(event => event.action), control === 'eof' ? ['eof'] : ['ctrl-c-after-barrier', 'ctrl-c-after-settled']);
    assert.equal(record.submittedBlockedFixtureTurn, true); assert.equal(record.controlSent, true);
  });
