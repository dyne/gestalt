import assert from 'node:assert/strict';
import cp from 'node:child_process';
import { promisify } from 'node:util';
import { createHash } from 'node:crypto';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { lstat, mkdir, mkdtemp, readFile, realpath, rmdir, unlink } from 'node:fs/promises';
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
const budget = Math.min(240_000, cutoff - Date.now());
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

const outcome = { scope: 'Two isolated positive Agents storage operations. No Manager installed or release acceptance.',
  startedAt: stamp().observedAt, guard: 'unexecuted', privateWrite: 'unexecuted', privacy: 'unexecuted', fixture: null, cleanup: 'unexecuted', calls: [], fixtureCreate: 'unexecuted' };
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
  const markers = ['const privateDirectoryScript = String.raw', 'const script = String.raw'];
  const scriptHashes = markers.map(marker => {
    const script = storedSource.split(marker + tick)[1]?.split(tick + ';')[0];
    assert.ok(script);
    return sha256(Buffer.from(script, 'utf16le'));
  });
  assert.deepEqual(scriptHashes, [
    'dc6463be4c626ad9c47a577a402d2afb7b3c5ab61e47d3076d2383505d746bc8',
    '3f8a1e9e3427dd670a5733d00e3a569303d2e27b448c385e66b3cf37c2e3dccf',
  ]);
  outcome.artifacts = { head: inputs.head, agents: inputs.agents, node: inputs.nodeSha256, powerShellSha, scriptHashes };
  record('inputs-verified', { ...outcome.artifacts, budget, cutoff: new Date(cutoff).toISOString() });
  requireBudget(235_000);

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
  requireBudget(183_000);
  const privateFile = path.join(codex, 'R8 private payload ü.txt');
  outcome.fixture.privateFile = privateFile;
  const payload = Buffer.from('owned Windows storage fixture ü\n', 'utf8');
  let expectedOperation;

  const original = cp.execFileSync;
  let calls = 0;
  cp.execFileSync = function (...args) {
    const [exe, argv, options] = args;
    const id = ++calls;
    const expectedTarget = expectedOperation === 'existing-root' ? codex : privateFile;
    const expectedScript = expectedOperation === 'existing-root' ? scriptHashes[0] : scriptHashes[1];
    const requestVariable = expectedOperation === 'existing-root' ? 'GESTALT_SETUP_PATH' : 'GESTALT_STORAGE_FILE';
    const setupPath = options?.env?.[requestVariable];
    const providedScriptSha = Array.isArray(argv) && typeof argv[4] === 'string'
      ? sha256(Buffer.from(argv[4], 'base64')) : null;
    const matched = expectedOperation !== undefined && exe === powerShell &&
      Array.isArray(argv) && argv.length === 5 && argv.slice(0, 4).join('|') === '-NoLogo|-NoProfile|-NonInteractive|-EncodedCommand' &&
      providedScriptSha === expectedScript &&
      typeof setupPath === 'string' && Buffer.from(setupPath, 'base64').toString('utf8') === expectedTarget &&
      (expectedOperation !== 'existing-root' || options.env.GESTALT_STORAGE_PRIVATE === '1') &&
      options.timeout === 60_000 && options.shell === false && options.windowsHide === true &&
      JSON.stringify(options.stdio) === '["ignore","ignore","pipe"]';
    const started = stamp();
    const call = { id, operation: expectedOperation ?? null, ...started, executable: exe, powerShellSha, scriptSha256: providedScriptSha,
      flags: Array.isArray(argv) ? argv.slice(0, 4) : null, exactSourceInvocation: matched,
      setupPathClass: matched ? (expectedOperation === 'existing-root' ? outcome.fixture.pathClass : 'owned absent fixture file') : 'unverified',
      options: { timeoutMs: options?.timeout ?? null, shell: options?.shell ?? null, windowsHide: options?.windowsHide ?? null,
        stdio: options?.stdio ?? null, privateMode: options?.env?.GESTALT_STORAGE_PRIVATE ?? null,
        encoding: options?.encoding ?? null, maxBuffer: options?.maxBuffer ?? null, killSignal: options?.killSignal ?? null } };
    record('accepted-storage-command-started', call);
    try {
      const result = Reflect.apply(original, this, args);
      const observed = { ...call, completed: stamp(), result: 'returned-without-throw',
        stdout: capture('call-' + id + '.stdout', result), stderr: 'unavailable on successful execFileSync return' };
      outcome.calls.push(observed);
      record('accepted-storage-command-observed', observed);
      return result;
    } catch (error) {
      const observed = { ...call, completed: stamp(), result: 'threw', error: errorDetails(error),
        stdout: capture('call-' + id + '.stdout', error.stdout), stderr: capture('call-' + id + '.stderr', error.stderr) };
      outcome.calls.push(observed);
      record('accepted-storage-command-observed', observed);
      throw error;
    }
  };
  syncBuiltinESMExports();
  try {
    const { ensureManagedRoot, writePrivateFile } = await import(pathToFileURL(modulePath));
    expectedOperation = 'existing-root';
    record('ensure-managed-root-started', { create: false, privateMode: 'default true' });
    outcome.guard = 'started';
    try { ensureManagedRoot(codex, { create: false }); }
    catch (error) {
      outcome.guard = 'threw';
      outcome.outerError = { ...errorDetails(error), message: error.message };
      throw error;
    }
    outcome.guard = 'returned';
    assert.equal(calls, 1);
    assert.ok(outcome.calls[0]?.exactSourceInvocation);
    assert.equal(observationError, undefined);
    requireBudget(122_000);

    await assert.rejects(lstat(privateFile), { code: 'ENOENT' });
    expectedOperation = 'exclusive-private-file';
    record('write-private-file-started', { bytes: payload.length, sha256: sha256(payload) });
    outcome.privateWrite = 'started';
    try { writePrivateFile(privateFile, payload); }
    catch (error) { outcome.privateWrite = 'threw'; throw error; }
    outcome.privateWrite = 'returned';
    assert.equal(calls, 2, 'Two exact existing Agents process invocations required');
    assert.ok(outcome.calls[1]?.exactSourceInvocation);
    const info = await lstat(privateFile);
    assert.ok(info.isFile() && !info.isSymbolicLink());
    const actual = await readFile(privateFile);
    assert.deepEqual(actual, payload);
    outcome.payload = { bytes: actual.length, sha256: sha256(actual), exact: true };
  } finally {
    cp.execFileSync = original;
    syncBuiltinESMExports();
  }

  requireBudget(60_000);
  const inspect = String.raw`
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$file = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:GESTALT_R8_INSPECT_PATH))
$item = Get-Item -LiteralPath $file -Force
$acl = $item.GetAccessControl()
@{ protected=$acl.AreAccessRulesProtected; owner=$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value;
  reparse=(($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0);
  rules=@($acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]) | ForEach-Object {
    @{sid=$_.IdentityReference.Value; mask=[int64]$_.FileSystemRights; type=$_.AccessControlType.ToString(); inherited=$_.IsInherited}
  }) } | ConvertTo-Json -Depth 5 -Compress
`;
  const inspectOptions = { env: { ...process.env, GESTALT_R8_INSPECT_PATH: Buffer.from(privateFile).toString('base64') },
    encoding: 'buffer', shell: false, windowsHide: true, timeout: 50_000, maxBuffer: 65536 };
  outcome.privacy = 'started';
  record('inspect-owned-private-file-started', { scriptSha256: sha256(Buffer.from(inspect, 'utf16le')), timeoutMs: 50000 });
  let inspection;
  try {
    inspection = await promisify(cp.execFile)(powerShell, ['-NoLogo', '-NoProfile', '-NonInteractive',
      '-EncodedCommand', Buffer.from(inspect, 'utf16le').toString('base64')], inspectOptions);
  } catch (error) {
    outcome.inspectionError = { ...errorDetails(error),
      stdout: capture('privacy.stdout', error.stdout), stderr: capture('privacy.stderr', error.stderr) };
    outcome.privacy = 'threw';
    throw error;
  }
  outcome.inspectionStreams = { stdout: capture('privacy.stdout', inspection.stdout), stderr: capture('privacy.stderr', inspection.stderr) };
  assert.equal(inspection.stderr.length, 0);
  const acl = JSON.parse(inspection.stdout.toString('utf8').replace(/^\uFEFF/, ''));
  outcome.fileAcl = acl;
  assert.equal(acl.owner, token.sid);
  assert.equal(acl.protected, true);
  assert.equal(acl.reparse, false);
  assert.ok(acl.rules.length > 0 && acl.rules.every(rule => rule.sid === token.sid && !rule.inherited && rule.type === 'Allow'));
  assert.ok(acl.rules.some(rule => (rule.mask & 2032127) === 2032127));
  outcome.privacy = 'confirmed';
  assert.equal(observationError, undefined, 'Original process outcomes were not durably recorded');
  outcome.diagnosticReceiptComplete = true;
  await unlink(privateFile);
  await rmdir(codex);
  await rmdir(home);
  await rmdir(outer);
  outcome.cleanup = 'owned fixture removed after both positive operations and privacy inspection';
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
