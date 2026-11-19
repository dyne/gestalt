import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { closeSync, fsyncSync, openSync, readFileSync, writeSync } from 'node:fs';
import { lstat, mkdir, realpath, rmdir } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const source = path.resolve(import.meta.dirname, '../..');
const evidence = path.join(source, 'native-evidence', 'windows-native-read-pair');
const journal = path.join(evidence, 'phases.ndjson');
const start = performance.now();
const cutoff = Date.parse(process.env.PROBE_GATE_DEADLINE_UTC ?? '');
const budgetMs = Math.min(80_000, cutoff - Date.now());
const elapsed = () => Math.round(performance.now() - start);
function record(event, details = {}) {
  const fd = openSync(journal, 'a');
  try {
    writeSync(fd, JSON.stringify({observedAt: new Date().toISOString(), elapsedMs: elapsed(), event, ...details}) + '\n');
    fsyncSync(fd);
  } finally { closeSync(fd); }
}
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const fileSha = file => sha256(readFileSync(file));
const readJson = file => JSON.parse(readFileSync(file, 'utf8').replace(/^\uFEFF/, ''));

async function phase(name, ms, action) {
  const remaining = budgetMs - elapsed();
  if (remaining < ms) {
    record('phase-unexecuted', {phase: name, requiredMs: ms, remaining, reason: 'observation budget'});
    throw Object.assign(new Error('Insufficient remaining observation budget'), {code: 'PROBE_BUDGET'});
  }
  record('phase-started', {phase: name, limitMs: ms});
  const abort = new AbortController();
  try {
    const timer = delay(ms, null, {signal: abort.signal}).then(() => {
      throw Object.assign(new Error('Observation deadline'), {code: 'PROBE_DEADLINE'});
    });
    const result = await Promise.race([timer, action()]);
    record('phase-settled', {phase: name, state: 'pass'});
    return result;
  } catch (error) {
    record('phase-settled', {phase: name, state: error.code === 'PROBE_DEADLINE' ? 'unknown' : 'failed',
      message: error.message, code: error.code ?? null});
    throw error;
  } finally { abort.abort(); }
}

function observe(child, name, expectedDigest) {
  record('child-request', {phase: name, argv: child.spawnargs, executableSha256: expectedDigest});
  child.once('spawn', () => record('child-spawned', {phase: name, pid: child.pid}));
  child.stdin?.end();
  let stdout = Buffer.alloc(0), stderr = Buffer.alloc(0), truncated = false;
  const capture = (previous, chunk) => {
    if (previous.length + chunk.length > 4096) truncated = true;
    return Buffer.concat([previous, chunk.subarray(0, 4096 - previous.length)]);
  };
  child.stdout.on('data', chunk => { stdout = capture(stdout, chunk); });
  child.stderr.on('data', chunk => { stderr = capture(stderr, chunk); });
  return new Promise((resolve, reject) => {
    child.once('error', error => {
      record('child-error', {phase: name, code: error.code ?? null, message: error.message});
      reject(error);
    });
    child.once('close', (code, signal) => {
      record('child-closed', {phase: name, code, signal, stdout: stdout.toString('utf8'),
        stderr: stderr.toString('utf8'), truncated});
      if (code !== 0 || signal || truncated) reject(new Error('Unconfirmed child result'));
      else resolve({ stdout, stderr });
    });
  });
}

let outer;
try {
  assert.ok(Number.isFinite(budgetMs) && budgetMs > 0, 'Launch observation gate unavailable or expired');
  const inputs = readJson(path.join(evidence, 'inputs.json'));
  const token = readJson(path.join(evidence, 'token.json'));
  assert.equal(process.platform, 'win32');
  assert.equal(process.arch, 'x64');
  assert.equal(process.version, 'v24.22.3');
  assert.equal(token.administrator, false);
  assert.equal(fileSha(process.execPath), inputs.nodeSha256);
  for (const [relative, expected] of Object.entries(inputs.files)) assert.equal(fileSha(path.join(source, relative)), expected);
  const artifactRoot = path.join(source, 'verified-helper');
  for (const [name, expected] of Object.entries(inputs.helperArtifact.files)) assert.equal(fileSha(path.join(artifactRoot,name)), expected);
  const helper = path.join(artifactRoot, 'gestalt-owned-process.exe');
  const helperSha = inputs.helperArtifact.files['gestalt-owned-process.exe'];
  const powerShell = path.join(process.env.SystemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
  const psSha = fileSha(powerShell);
  assert.equal(psSha, token.powershell.sha256);
  const { startWindowsSystem } = await import('../../src/cli/platform/windows-system.ts');
  const { assertWindowsReadReply, windowsReadArguments } = await import('../../src/cli/runtime/windows-scratch-read.mjs');
  record('inputs-verified', {head: inputs.head, helperSha, powerShellSha: psSha,
    nodeSha: inputs.nodeSha256, helperArtifact: inputs.helperArtifact.id, budgetMs, gateDeadlineUtc: new Date(cutoff).toISOString()});
  const root = await phase('fixture-base', 5000, async () => {
    const scratch = await realpath(process.env.TEMP);
    const fresh = path.join(scratch, 'gestalt native read ü ' + randomUUID());
    await mkdir(fresh);
    outer = fresh;
    const root = path.join(outer, 'private ü root');
    record('fixture-created', {outer, root});
    return root;
  });

  // The existing fixed create operation also checks the resulting owner/DACL.
  await phase('powershell-create', 25_000, () =>
    observe(startWindowsSystem({operation: 'create-directory', path: root}), 'powershell-create', psSha));
  assert.ok((await lstat(root)).isDirectory());
  const parents = [];
  for (let next = path.dirname(root);; next = path.dirname(next)) {
    parents.push(path.toNamespacedPath(next));
    if (path.dirname(next) === next) break;
  }
  const namespace = {operation: 'assert-namespace', paths: parents};
  const privateRoot = {operation: 'assert-private', path: root};
  record('requests-fixed', {ancestorCount: parents.length, root});
  for (const [name, request] of [['native-namespace',namespace],['native-private',privateRoot]]) {
    await phase(name, 10_000, async () => {
      const argv = windowsReadArguments(request);
      const reply = await observe(spawn(helper, argv, {shell: false, windowsHide: true, stdio: ['ignore','pipe','pipe']}), name, helperSha);
      assert.equal(reply.stderr.length, 0);
      assertWindowsReadReply(reply.stdout);
    });
  }

  // The comparison is optional. Leave an explicit receipt if its two local
  // read-only counterparts and success cleanup cannot fit within this window.
  let compared = false;
  if (elapsed() + 25_000 + 10_000 + 5000 <= budgetMs) {
    for (const [name, request, limit] of [
      ['powershell-namespace',namespace,25_000],['powershell-private',privateRoot,10_000],
    ]) await phase(name, limit, () => observe(startWindowsSystem(request), name, psSha));
    compared = true;
  } else {
    record('comparison-unexecuted', {reason: 'insufficient remaining budget', remainingMs: budgetMs - elapsed()});
  }
  await phase('fixture-cleanup', 5000, async () => {
    await rmdir(root);
    await rmdir(outer);
  });
  record('terminal', {state: 'pass', nativeReads: 2, sameFixturePowerShellReads: compared ? 2 : 0, fixture: 'removed'});
} catch (error) {
  record('terminal', {state: 'fail', unresolved: error.code === 'PROBE_DEADLINE', fixture: outer ?? null,
    disposition: 'Retained for hosted runner disposal if present', message: error.message});
  process.exitCode = 1;
}
