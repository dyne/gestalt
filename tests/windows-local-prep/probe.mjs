import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { closeSync, fsyncSync, openSync, readFileSync, writeSync } from 'node:fs';
import { lstat, mkdir, realpath, rmdir } from 'node:fs/promises';
import { createRequire, syncBuiltinESMExports } from 'node:module';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const sourceRoot = path.resolve(import.meta.dirname, '../..');
const evidence = path.join(sourceRoot, 'native-evidence', 'windows-local-prep');
const journal = path.join(evidence, 'phases.ndjson');
const started = performance.now();
function record(event, details = {}) {
  const fd = openSync(journal, 'a');
  try {
    writeSync(fd, JSON.stringify({
      observedAt: new Date().toISOString(), elapsedMs: Math.round(performance.now() - started),
      event, ...details,
    }) + '\n');
    fsyncSync(fd);
  } finally { closeSync(fd); }
}

const inputManifest = JSON.parse(readFileSync(path.join(evidence, 'inputs.json'), 'utf8').replace(/^\uFEFF/, ''));
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
assert.equal(process.platform, 'win32');
assert.equal(process.arch, 'x64');
assert.equal(process.version, 'v24.22.3');
for (const [relative, expected] of Object.entries(inputManifest.files)) {
  assert.equal(sha256(readFileSync(path.join(sourceRoot, relative))), expected, relative);
}
assert.equal(sha256(readFileSync(process.execPath)), inputManifest.nodeSha256);
record('inputs-verified', { head: inputManifest.head, files: inputManifest.files, nodeSha256: inputManifest.nodeSha256 });

const { windowsSystemScript } = await import('../../src/cli/platform/windows-system.ts');
const powerShell = path.join(process.env.SystemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
const fixedArguments = ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand',
  Buffer.from(windowsSystemScript, 'utf16le').toString('base64')];
const cp = createRequire(import.meta.url)('node:child_process');
const originalSpawn = cp.spawn;
const adapterRuns = [];
let phase;
let deadlineExpired = false;
cp.spawn = function (file, args, options) {
  if (deadlineExpired) throw new Error('Observation already ended; no further adapter started');
  assert.equal(file, powerShell, 'unexpected executable in observation scope');
  assert.deepEqual(args, fixedArguments, 'unexpected PowerShell adapter command');
  const request = JSON.parse(Buffer.from(options.env.GESTALT_NATIVE_REQUEST, 'base64').toString('utf8'));
  assert.ok(['assert-namespace', 'assert-private', 'create-directory'].includes(request.operation));
  const item = { id: adapterRuns.length + 1, phase, operation: request.operation };
  adapterRuns.push({ item, closed: false, code: null });
  const observation = adapterRuns.at(-1);
  record('adapter-started', { ...item, requestedPath: request.path ?? null, ancestorCount: request.paths?.length ?? null });
  const child = originalSpawn.apply(this, arguments);
  child.once('spawn', () => record('adapter-spawned', { ...item, pid: child.pid }));
  child.once('error', error => record('adapter-error', { ...item, code: error.code ?? null, message: error.message }));
  child.once('close', (code, signal) => {
    observation.closed = true;
    observation.code = code;
    record('adapter-closed', { ...item, code, signal });
  });
  return child;
};
syncBuiltinESMExports();

async function within(name, ms, action) {
  phase = name;
  record('phase-started', { phase, limitMs: ms });
  const abort = new AbortController();
  try {
    const value = await Promise.race([
      action(),
      delay(ms, null, { signal: abort.signal }).then(() => {
        deadlineExpired = true;
        throw Object.assign(new Error('Observation deadline'), { code: 'PROBE_DEADLINE' });
      }),
    ]);
    record('phase-settled', { phase, state: 'pass' });
    return value;
  } catch (error) {
    record('phase-settled', { phase, state: error.code === 'PROBE_DEADLINE' ? 'unknown' : 'failed',
      message: error.message, code: error.code ?? null });
    throw error;
  } finally { abort.abort(); }
}

const requestedScratch = process.env.TEMP;
let fixture;
try {
  const { NativePrivateStorage } = await import('../../src/cli/platform/storage.ts');
  const root = await within('fixture-base', 5000, async () => {
    const scratch = await realpath(requestedScratch);
    const fresh = path.join(scratch, 'gestalt native runtime ü ' + randomUUID());
    await mkdir(fresh);
    fixture = fresh;
    record('fixture-created', { fixture });
    return path.join(fresh, 'private');
  });
  const storage = await within('private-open', 30000, () => NativePrivateStorage.open(root));
  const nested = path.join(storage.root, 'nested ü leaf');
  await within('nested-create', 30000, async () => {
    await storage.createDirectory(nested);
    assert.ok((await lstat(nested)).isDirectory());
  });
  assert.ok(adapterRuns.length && adapterRuns.every(row => row.closed && row.code === 0));
  await within('fixture-cleanup', 5000, async () => {
    await rmdir(nested);
    await rmdir(storage.root);
    await rmdir(fixture);
  });
  record('terminal', { state: 'pass', fixture: 'removed', adapterCalls: adapterRuns.length });
} catch (error) {
  record('terminal', { state: 'fail', unresolved: error.code === 'PROBE_DEADLINE',
    fixture: fixture ?? null, disposition: 'Retained for hosted runner disposal if present',
    adapterCalls: adapterRuns.length, closed: adapterRuns.filter(row => row.closed).length, message: error.message });
  process.exitCode = 1;
  // No signal or alternate operation on timeout. Outstanding handles retain
  // their ordinary lifetime, with the direct runner observation bounded above us.
}
