import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {randomUUID} from 'node:crypto';
import {createRequire} from 'node:module';
import {mkdtemp, mkdir, readFile, writeFile, rm, rename, symlink, chmod} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import test from 'node:test';
import {sessionScratch, mobileResourceLease} from './session-scratch-fixture.mjs';

const manager = resolve('public/gestalt');
const program = (await readFile(manager, 'utf8')).match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const api = new Function('require', program + '\nreturn {scratchAllocate};')(createRequire(import.meta.url));
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'tool lease recovery '));
  t.after(() => rm(root, {recursive: true, force: true}));
  const workspace = join(root, 'workspace with spaces');
  await mkdir(workspace);
  const scratch = await sessionScratch(workspace);
  const runtime = await mobileResourceLease(scratch);
  await writeFile(runtime.path, JSON.stringify({...runtime.lease, state: 'released', consumers: []}));
  const borrowers = [];
  for (const consumer of ['serena-mcp', 'playwright-mcp']) {
    const lease = {...runtime.lease, leaseId: randomUUID(), processIdentity: randomUUID(), consumers: [consumer]};
    const path = join(scratch.context.directory, 'leases', lease.leaseId + '.json');
    await writeFile(path, JSON.stringify(lease), {mode: 0o600});
    borrowers.push({path, lease});
  }
  const payload = join(scratch.context.directory, 'keep');
  await writeFile(payload, 'disposable but retained until Forget');
  const env = {...process.env};
  for (const key of ['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP']) delete env[key];
  const args = ['--cwd', workspace, '--owner', scratch.context.ownerId];
  const run = (...options) => spawnSync('bash', [manager, 'scratch', ...options], {env, encoding: 'utf8', timeout: 5000});
  const inspect = () => {
    const result = run('inspect-tool-leases', ...args);
    assert.equal(result.status, 0, result.stderr);
    return JSON.parse(result.stdout);
  };
  const recover = token => run('recover-tool-leases', ...args, '--expect', token, '--confirm-quiescent');
  return {...scratch, root, workspace, runtime, borrowers, payload, args, run, inspect, recover};
}

test('offline recovery retires inspected tool borrowers while retaining owner, released leases and payload', async t => {
  const f = await fixture(t);
  const before = await Promise.all([f.context.markerPath, f.runtime.path, f.payload].map(p => readFile(p, 'utf8')));
  const snapshot = f.inspect();
  assert.equal(snapshot.activeToolLeases, 2);
  assert.equal(JSON.parse(await readFile(f.borrowers[0].path, 'utf8')).state, 'active', 'inspection is read-only');
  assert.notEqual(f.run('recover-tool-leases', ...f.args, '--expect', snapshot.token).status, 0);
  const recovered = f.recover(snapshot.token);
  assert.equal(recovered.status, 0, recovered.stderr);
  for (const {path, lease} of f.borrowers) {
    const current = JSON.parse(await readFile(path, 'utf8'));
    assert.deepEqual(current, {...lease, state: 'released', consumers: [], updatedAt: current.updatedAt});
  }
  assert.deepEqual(await Promise.all([f.context.markerPath, f.runtime.path, f.payload].map(p => readFile(p, 'utf8'))), before);
  assert.notEqual(f.recover(snapshot.token).status, 0, 'a completed snapshot cannot be replayed');
});

for (const change of ['content', 'replacement', 'new lease', 'owner', 'controller', 'association', 'containment', 'symlink', 'permissions']) {
  test('tool recovery rejects changed ' + change + ' before releasing any borrower', async t => {
    const f = await fixture(t), token = f.inspect().token;
    const first = f.borrowers[0];
    if (change === 'content') await writeFile(first.path, JSON.stringify({...first.lease, processIdentity: randomUUID()}));
    if (change === 'replacement') {
      await rename(first.path, join(f.root, 'original-lease'));
      await writeFile(first.path, JSON.stringify(first.lease), {mode: 0o600});
    }
    if (change === 'new lease') await mobileResourceLease(f);
    if (change === 'owner') {
      await rename(f.context.directory, f.context.directory + '-original');
      await sessionScratch(f.workspace);
    }
    if (['controller', 'association', 'containment'].includes(change))
      await mkdir(join(f.context.directory, '.' + change + '-lock'), {mode: 0o700});
    if (change === 'symlink') {
      const outside = join(f.root, 'external-lease');
      await rename(first.path, outside);
      await symlink(outside, first.path);
    }
    if (change === 'permissions') await chmod(first.path, 0o644);
    assert.notEqual(f.recover(token).status, 0);
    const directory = change === 'owner' ? f.context.directory + '-original' : f.context.directory;
    for (const {lease} of f.borrowers) {
      const current = JSON.parse(await readFile(join(directory, 'leases', lease.leaseId + '.json'), 'utf8'));
      assert.equal(current.state, 'active');
    }
  });
}

for (const consumers of [['runtime-unknown'], [], ['serena-mcp', 'unknown']]) {
  test('unknown active borrower authority is retained: ' + JSON.stringify(consumers), async t => {
    const f = await fixture(t);
    const first = f.borrowers[0];
    await writeFile(first.path, JSON.stringify({...first.lease, consumers}));
    assert.notEqual(f.run('inspect-tool-leases', ...f.args).status, 0);
    assert.equal(JSON.parse(await readFile(first.path, 'utf8')).state, 'active');
  });
}

for (const kind of ['cli-session', 'workspace-service']) {
  test('tool recovery rejects validated ' + kind + ' owners', async t => {
    const f = await fixture(t);
    const owner = api.scratchAllocate(f.workspace, kind, 'serena-mcp');
    assert.notEqual(f.run('inspect-tool-leases', '--cwd', f.workspace, '--owner', owner.context.ownerId).status, 0);
    assert.equal(JSON.parse(await readFile(join(owner.context.directory, 'leases', owner.lease.leaseId + '.json'), 'utf8')).state, 'active');
  });
}

test('tool recovery rejects invalid or duplicate arguments', async t => {
  const f = await fixture(t);
  for (const args of [[], ['--expect'], ['--force'], ['--confirm-quiescent'], ['--owner', 'cli-' + randomUUID()]])
    assert.notEqual(f.run('recover-tool-leases', ...f.args, ...args).status, 0);
  assert.match(f.run('--help').stdout, /inspect-tool-leases/);
});
