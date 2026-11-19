import assert from 'node:assert/strict';
import cp from 'node:child_process';
import { createHash } from 'node:crypto';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { lstat, mkdir, mkdtemp, realpath, rmdir } from 'node:fs/promises';
import { syncBuiltinESMExports } from 'node:module';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { pathToFileURL } from 'node:url';

const source = path.resolve(import.meta.dirname, '../..');
const evidence = path.join(source, 'native-evidence', 'windows-agents-storage-diagnostic');
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const fileSha = file => sha256(readFileSync(file));
const json = name => JSON.parse(readFileSync(path.join(evidence, name), 'utf8').replace(/^\uFEFF/, ''));
const began = performance.now();
const cutoff = Date.parse(process.env.PROBE_GATE_DEADLINE_UTC ?? '');
const budget = Math.min(80_000, cutoff - Date.now());
const stamp = () => ({ observedAt: new Date().toISOString(), elapsedMs: performance.now() - began });
let observationError;
function record(event, data = {}) {
  try { appendFileSync(path.join(evidence, 'phases.ndjson'), JSON.stringify({ ...stamp(), event, ...data }) + '\n'); }
  catch (error) { observationError = error; }
}
function capture(name, value) {
  if (value == null) return { present: false };
  const bytes = Buffer.from(value);
  const bounded = bytes.subarray(0, 65536);
  try { writeFileSync(path.join(evidence, name), bounded, { flag: 'wx', mode: 0o600 }); }
  catch (error) { observationError = error; }
  return { present: true, file: name, originalBytes: bytes.length, capturedBytes: bounded.length,
    sha256: sha256(bounded), truncated: bounded.length !== bytes.length };
}
function errorDetails(error) {
  const keys = ['code','errno','status','signal','killed','syscall'];
  return { name: error.name, fields: Object.fromEntries(keys.map(key => [key, error[key] ?? null])),
    present: keys.filter(key => Object.hasOwn(error, key)),
    cause: error.cause == null ? null : { name: error.cause.name ?? null, code: error.cause.code ?? null,
      status: error.cause.status ?? null, signal: error.cause.signal ?? null, killed: error.cause.killed ?? null } };
}
function requireBudget(minimum) {
  if (!(budget - (performance.now() - began) >= minimum)) throw new Error('Insufficient overall observation budget');
}

const outcome = { scope: 'One isolated Agents storage call. No Manager installed or release acceptance.',
  startedAt: stamp().observedAt, guard: 'unexecuted', fixture: null, cleanup: 'unexecuted', call: null, fixtureCreate: 'unexecuted' };
let outer, home, codex;
try {
  assert.equal(process.platform, 'win32');
  assert.equal(process.arch, 'x64');
  assert.equal(process.version, 'v24.22.3');
  const inputs = json('inputs.json'), token = json('token.json');
  assert.equal(token.administrator, false);
  for (const [relative, digest] of Object.entries(inputs.files)) assert.equal(fileSha(path.join(source, relative)), digest);
  assert.equal(fileSha(process.execPath), inputs.nodeSha256);
  const powerShell = path.join(process.env.SystemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
  const powerShellSha = fileSha(powerShell);
  assert.equal(powerShellSha, token.powershell.sha256);
  const modulePath = path.join(source, inputs.agents.module);
  assert.equal(fileSha(modulePath), inputs.agents.sha256);
  const storedSource = readFileSync(modulePath, 'utf8');
  const tick = String.fromCharCode(96);
  const marker = 'const privateDirectoryScript = String.raw' + tick;
  const script = storedSource.split(marker)[1]?.split(tick + ';')[0];
  assert.ok(script);
  const scriptSha256 = sha256(Buffer.from(script, 'utf16le'));
  outcome.artifacts = { head: inputs.head, agents: inputs.agents, node: inputs.nodeSha256, powerShellSha, scriptSha256 };
  record('inputs-verified', { ...outcome.artifacts, budget, cutoff: new Date(cutoff).toISOString() });
  requireBudget(65_000);

  outer = await realpath(await mkdtemp(path.join(await realpath(process.env.TEMP), 'gestalt core installed ü & ')));
  home = path.join(outer, 'home ü &');
  await mkdir(home);
  codex = path.join(home, 'Codex ü &');
  outcome.fixture = { outer, home, codex, pathClass: 'existing Codex root below owned Unicode/space Temp fixture' };
  record('fixture-create-started', { pathClass: outcome.fixture.pathClass });
  const { windowsSystem } = await import('../../src/cli/platform/windows-system.ts');
  outcome.fixtureCreate = 'started';
  const timer = new AbortController();
  try {
    await Promise.race([
      windowsSystem({ operation: 'create-directory', path: codex }),
      delay(50_000, null, { signal: timer.signal }).then(() => {
        throw Object.assign(new Error('Fixture creation observation expired'), { code: 'OBSERVATION_DEADLINE' });
      }),
    ]);
    assert.ok((await lstat(codex)).isDirectory());
    outcome.fixtureCreate = 'pass';
    record('fixture-create-observed', { state: 'pass' });
  } catch (error) {
    outcome.fixtureCreate = error.code === 'OBSERVATION_DEADLINE' ? 'unknown' : 'failed';
    record('fixture-create-observed', { state: outcome.fixtureCreate, ...errorDetails(error) });
    throw error;
  } finally { timer.abort(); }
  requireBudget(12_000);

  const original = cp.execFileSync;
  let calls = 0;
  cp.execFileSync = function (...args) {
    const [exe, argv, options] = args;
    const id = ++calls;
    const setupPath = options?.env?.GESTALT_SETUP_PATH;
    const providedScriptSha = Array.isArray(argv) && typeof argv[4] === 'string'
      ? sha256(Buffer.from(argv[4], 'base64')) : null;
    const matched = exe === powerShell &&
      Array.isArray(argv) && argv.length === 5 && argv.slice(0, 4).join('|') === '-NoLogo|-NoProfile|-NonInteractive|-EncodedCommand' &&
      providedScriptSha === scriptSha256 &&
      typeof setupPath === 'string' && Buffer.from(setupPath, 'base64').toString('utf8') === codex &&
      options.env.GESTALT_STORAGE_PRIVATE === '1' && options.timeout === 10_000 &&
      options.shell === false && options.windowsHide === true && JSON.stringify(options.stdio) === '["ignore","ignore","pipe"]';
    const started = stamp();
    const call = { id, ...started, executable: exe, powerShellSha, scriptSha256: providedScriptSha,
      flags: Array.isArray(argv) ? argv.slice(0, 4) : null, exactSourceInvocation: matched,
      setupPathClass: matched ? outcome.fixture.pathClass : 'unverified',
      options: { timeoutMs: options?.timeout ?? null, shell: options?.shell ?? null, windowsHide: options?.windowsHide ?? null,
        stdio: options?.stdio ?? null, privateMode: options?.env?.GESTALT_STORAGE_PRIVATE ?? null,
        encoding: options?.encoding ?? null, maxBuffer: options?.maxBuffer ?? null, killSignal: options?.killSignal ?? null } };
    record('accepted-storage-command-started', call);
    try {
      const result = Reflect.apply(original, this, args);
      outcome.call = { ...call, completed: stamp(), result: 'returned-without-throw',
        stdout: capture('call-' + id + '.stdout', result), stderr: 'unavailable on successful execFileSync return' };
      record('accepted-storage-command-observed', outcome.call);
      return result;
    } catch (error) {
      outcome.call = { ...call, completed: stamp(), result: 'threw', error: errorDetails(error),
        stdout: capture('call-' + id + '.stdout', error.stdout), stderr: capture('call-' + id + '.stderr', error.stderr) };
      record('accepted-storage-command-observed', outcome.call);
      throw error;
    }
  };
  syncBuiltinESMExports();
  let failure;
  try {
    const { ensureManagedRoot } = await import(pathToFileURL(modulePath));
    record('ensure-managed-root-started', { create: false, privateMode: 'default true' });
    outcome.guard = 'started';
    ensureManagedRoot(codex, { create: false });
    outcome.guard = 'returned';
  } catch (error) {
    failure = error;
    outcome.guard = 'threw';
    outcome.outerError = { ...errorDetails(error), message: error.message };
  } finally {
    cp.execFileSync = original;
    syncBuiltinESMExports();
  }
  assert.equal(calls, 1, 'Exactly one accepted Agents storage process invocation is required');
  assert.ok(outcome.call?.exactSourceInvocation, 'Captured invocation differs from frozen Agents setup guard');
  assert.equal(observationError, undefined, 'Original process outcome was not durably recorded');
  outcome.diagnosticReceiptComplete = true;
  if (failure) throw failure;
  await rmdir(codex);
  await rmdir(home);
  await rmdir(outer);
  outcome.cleanup = 'owned fixture removed after the successful guard return';
  outcome.state = 'diagnostic-complete';
} catch (error) {
  outcome.state = 'diagnostic-failure';
  outcome.failure = { ...errorDetails(error), message: error.message };
  outcome.cleanup = outer ? 'retained for disposable runner if present' : 'not attempted';
  process.exitCode = 1;
} finally {
  outcome.finishedAt = stamp().observedAt;
  outcome.ownershipScope = 'No child lifetime inference from blank stderr, wrapper timeout, or a direct harness receipt';
  writeFileSync(path.join(evidence, 'result.json'), JSON.stringify(outcome, null, 2) + '\n');
}
