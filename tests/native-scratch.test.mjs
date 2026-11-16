import assert from 'node:assert/strict';
import path from 'node:path';
import test from 'node:test';
import { selectedWorkspace, scratchHandler } from '../src/cli/runtime/scratch.ts';
import { scratchContext, scratchSocketPath } from '../src/cli/runtime/scratch-owner.mjs';

test('native workspace selection preserves resume IDs, permissions, literal argv and explicit selector precedence', () => {
  const cwd = path.resolve('workspace'), argv = ['-m', 'operator', '--permissions', ':workspace', '-C', 'first ü', 'resume', 'native-thread-id', '--cd=second', '--', '-C', 'literal'];
  const before = [...argv]; assert.equal(selectedWorkspace({ argv, cwd, environment: {} }), path.resolve(cwd, 'first ü', 'second')); assert.deepEqual(argv, before);
  assert.equal(selectedWorkspace({ argv: ['resume', 'native-thread-id'], cwd, environment: {} }), undefined);
  assert.throws(() => selectedWorkspace({ argv: ['-C'], cwd, environment: {} }), /requires a directory/);
});
test('extracted scratch runtime rejects malformed inherited schemas before allocating or rewriting an owner', () => {
  for (const value of ['not JSON', '{}', '{"schemaVersion":3}', '{"schemaVersion":1,"ownerId":"../escape"}']) assert.throws(() => scratchContext({ GESTALT_SESSION_TMP_CONTEXT: value }));
  assert.equal(scratchContext({ TMPDIR: 'operator-root' }), null);
});
test('extracted socket budget counts encoded multibyte endpoints including the terminating NUL', () => {
  for (const capacity of [104, 108]) {
    const exact = '/' + 'é'.repeat((capacity - 2) / 2);
    assert.equal(Buffer.byteLength(exact) + 1, capacity); assert.equal(scratchSocketPath(exact, capacity), exact);
    assert.throws(() => scratchSocketPath(exact + 'a', capacity), /UNIX_SOCKET_PATH_TOO_LONG/);
  }
  assert.throws(() => scratchSocketPath('/tmp/zero\0endpoint', 108), /UNIX_SOCKET_PATH_TOO_LONG/);
});
test('native scratch recovery help states offline confirmation and invalid recovery never gains write authority', async () => {
  const output = [], errors = [], streams = { stdin: (async function* () {})(), stdout: async b => output.push(Buffer.from(b)), stderr: async b => errors.push(Buffer.from(b)) };
  assert.equal(await scratchHandler({ argv: ['--help'], cwd: process.cwd(), environment: {} }, streams), 0);
  assert.match(Buffer.concat(output).toString(), /confirm-quiescent/); assert.match(Buffer.concat(output).toString(), /never a process absence proof/);
  assert.equal(await scratchHandler({ argv: ['recover-publication', '--cwd', 'relative', '--expect', '0'.repeat(64)], cwd: process.cwd(), environment: {} }, streams), 1);
  assert.match(Buffer.concat(errors).toString(), /SESSION_SCRATCH_INVALID_CONTEXT/);
});
