import assert from 'node:assert/strict';
import {spawn, spawnSync} from 'node:child_process';
import {once} from 'node:events';
import fs from 'node:fs';
import {mkdtemp, mkdir, readFile, rm, stat, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import test from 'node:test';
import {createRequire} from 'node:module';
import {sessionScratch} from './session-scratch-fixture.mjs';

const manager = resolve('public/gestalt');
const program = (await readFile(manager, 'utf8')).match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const api = new Function('require', program + '\nreturn {scratchAllocate,scratchRelease};')(createRequire(import.meta.url));

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'scratch recovery '));
  t.after(() => rm(root, {recursive: true, force: true}));
  const workspace = join(root, 'workspace with spaces');
  await mkdir(workspace, {mode: 0o700});
  const {context} = await sessionScratch(workspace, 'crashed-publisher');
  const lock = join(context.directory, '.containment-lock');
  const selected = join(context.directory, 'nested workspace');
  await mkdir(selected, {mode: 0o700});
  await writeFile(join(context.directory, 'payload'), 'retained');
  const env = {...process.env};
  for (const key of ['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP']) delete env[key];
  const run = (...args) => spawnSync('bash', [manager, 'scratch', ...args], {env, encoding: 'utf8', timeout: 5000});
  const inspect = () => {
    const result = run('inspect', '--cwd', workspace, '--owner', context.ownerId);
    assert.equal(result.status, 0, result.stderr);
    return JSON.parse(result.stdout).token;
  };
  const recover = token => run('recover', '--cwd', workspace, '--owner', context.ownerId,
    '--expect', token, '--confirm-quiescent');
  return {root, workspace, context, lock, selected, run, inspect, recover};
}

test('killed publisher leaves a durable block; explicit offline recovery restores nested publication', {timeout: 10000}, async t => {
  const f = await fixture(t);
  // Kill a real publisher inside the protected callback: finally cannot run.
  const child = spawn(process.execPath, ['-e', program + '\nscratchWithContainment(process.argv[1],()=>{process.send(scratchManagedAncestors(process.argv[1]).map(a=>{const path=scratchPath.join(a.context.directory,".containment-lock");const s=scratchFs.lstatSync(path);return {path,dev:s.dev,ino:s.ino};}));Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0);});', f.selected],
    {stdio: ['ignore', 'ignore', 'pipe', 'ipc']});
  let stderr = ''; child.stderr.on('data', bytes => stderr += bytes);
  const exited = once(child, 'exit');
  let claims = [];
  try {
    [claims] = await Promise.race([once(child, 'message', {signal: AbortSignal.timeout(3000)}), exited.then(() => {throw Error('publisher exited before fencing: ' + stderr);})]);
  } finally {
    child.kill('SIGKILL'); await exited;
    // A fixture inside managed TMPDIR also claims the test runner's ancestors.
    // Retire only the child's positively observed instances outside our target;
    // leaving those behind would poison the runner rather than this fixture.
    for (const claim of claims.reverse()) {
      if (claim.path === f.lock) continue;
      const current = await stat(claim.path);
      assert.equal(current.dev, claim.dev); assert.equal(current.ino, claim.ino);
      await fs.promises.rmdir(claim.path);
    }
  }
  const before = await stat(f.lock);
  assert.throws(() => api.scratchAllocate(f.selected, 'cli-session', 'blocked'), /CONTAINMENT_BUSY/);
  const marker = await readFile(f.context.markerPath, 'utf8');
  const token = f.inspect();
  assert.equal((await stat(f.lock)).ino, before.ino, 'inspection never mutates the fence');
  assert.equal(f.recover(token).status, 0);
  await assert.rejects(stat(f.lock), {code: 'ENOENT'});
  const owner = api.scratchAllocate(f.selected, 'cli-session', 'recovered');
  assert.equal(api.scratchRelease(owner), true);
  assert.equal(await readFile(f.context.markerPath, 'utf8'), marker);
  assert.equal(await readFile(join(f.context.directory, 'payload'), 'utf8'), 'retained');
  assert.notEqual(f.recover(token).status, 0, 'replaying recovery cannot remove another fence');
});

test('recovery requires explicit quiescence and the inspected identity; active leases remain intact', async t => {
  const f = await fixture(t);
  await mkdir(f.lock, {mode: 0o700});
  const leases = join(f.context.directory, 'leases');
  await mkdir(leases, {mode: 0o700});
  const {randomUUID} = await import('node:crypto');
  const leaseId = randomUUID(), now = new Date().toISOString();
  const file = join(leases, leaseId + '.json');
  const lease = JSON.stringify({schemaVersion: 1, ownerId: f.context.ownerId, workspacePath: f.workspace,
    leaseId, state: 'active', processIdentity: randomUUID(), createdAt: now, updatedAt: now, consumers: ['retained']});
  await writeFile(file, lease, {mode: 0o600});
  const token = f.inspect();
  const args = ['recover', '--cwd', f.workspace, '--owner', f.context.ownerId, '--expect', token];
  assert.notEqual(f.run(...args).status, 0);
  const updatedLease = JSON.stringify({...JSON.parse(lease), consumers: ['changed-consumer']});
  await writeFile(file, updatedLease);
  assert.match(f.recover(token).stderr, /SNAPSHOT_CHANGED/);
  await writeFile(file, lease);
  await fs.promises.rename(f.lock, f.lock + '-original');
  await mkdir(f.lock, {mode: 0o700});
  assert.match(f.recover(token).stderr, /SNAPSHOT_CHANGED/);
  assert.equal(f.recover(f.inspect()).status, 0);
  assert.equal(await readFile(file, 'utf8'), lease);
  await stat(f.lock + '-original');
});

for (const change of ['payload', 'alias', 'controller', 'association', 'marker', 'permissions']) {
  test('recovery rejects ' + change + ' changes after inspection without clearing the fence', async t => {
    const f = await fixture(t);
    await mkdir(f.lock, {mode: 0o700});
    const token = f.inspect();
    if (change === 'payload') await writeFile(join(f.lock, 'unknown'), 'retained');
    if (change === 'alias') {
      await fs.promises.rename(f.lock, f.lock + '-original');
      await fs.promises.symlink(f.lock + '-original', f.lock);
    }
    if (change === 'controller' || change === 'association') await mkdir(join(f.context.directory, '.' + change + '-lock'));
    if (change === 'marker') {
      const marker = JSON.parse(await readFile(f.context.markerPath, 'utf8'));
      await writeFile(f.context.markerPath, JSON.stringify({...marker, createdAt: '2000-01-01T00:00:00Z'}));
    }
    if (change === 'permissions') await fs.promises.chmod(f.lock, 0o755);
    assert.notEqual(f.recover(token).status, 0);
    await fs.promises.lstat(f.lock);
    assert.equal(await readFile(join(f.context.directory, 'payload'), 'utf8'), 'retained');
  });
}

test('invalid, duplicate and unknown options fail without mutation; help explains the offline precondition', async t => {
  const f = await fixture(t);
  await mkdir(f.lock, {mode: 0o700});
  for (const args of [[], ['inspect', '--cwd'], ['inspect', '--cwd', f.workspace, '--owner', '../escape'],
    ['inspect', '--cwd', f.workspace, '--owner', f.context.ownerId, '--force'],
    ['inspect', '--cwd', f.workspace, '--cwd', f.workspace, '--owner', f.context.ownerId]]) {
    assert.notEqual(f.run(...args).status, 0);
  }
  assert.match(f.run('--help').stdout, /Stop Mobile and all runtimes\/tools/);
  await stat(f.lock);
});
