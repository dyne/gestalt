import assert from 'node:assert/strict';
import childProcess from 'node:child_process';
import { spawn as namedSpawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { observeOwnedHelpers } from './fixtures/native-owned-observer.mjs';

test('private helper observer preserves bytes/arguments/exit without adding a data reader', { timeout: 10000 }, async () => {
  const directory = await mkdtemp(path.join(tmpdir(), 'observe-')), original = childProcess.spawn;
  const observer = observeOwnedHelpers(process.execPath, directory, 'test'); let child;
  try {
    const argv = ['-e', 'const chunks=[];process.stdin.on("data",b=>chunks.push(b));process.stdin.on("end",()=>{process.stdout.write(Buffer.concat(chunks));process.exitCode=23;});', '', 'é東京'];
    child = namedSpawn(process.execPath, argv, { shell: false, stdio: ['pipe', 'pipe', 'pipe'] }); const closed = once(child, 'close');
    assert.equal(child.stdout.listenerCount('data'), 0); assert.equal(child.stdout.readableFlowing, null);
    const chunks = [], collected = (async () => { for await (const b of child.stdout) chunks.push(b); })();
    const bytes = Buffer.from([0, 255, 13, 10, ...Buffer.from('é東京')]); child.stdin.write(bytes); child.stdin.end();
    await collected; assert.deepEqual(await closed, [23, null]); assert.deepEqual(Buffer.concat(chunks), bytes);
    assert.deepEqual(await readFile(observer.traces[0].stdout), bytes); assert.deepEqual(await readFile(observer.traces[0].stdin), bytes);
    assert.deepEqual(observer.traces[0].argv, argv); assert.deepEqual(observer.traces[0].errors, []);
  } finally { observer.restore(); if (child?.exitCode === null && child.signalCode === null) { child.kill(); await once(child, 'close'); } await rm(directory, { recursive: true, force: true }); }
  assert.equal(childProcess.spawn, original);
});
