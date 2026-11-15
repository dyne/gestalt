import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import test from 'node:test';
import { borrowedProcessInput } from '../src/cli/platform/process-input.ts';

const events = ['readable', 'end', 'error', 'close'];
const listeners = input => events.map(event => input.listeners(event));
function bounded(promise) {
  let timer;
  return Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(Error('Input wait did not cancel')), 1000); })]).finally(() => clearTimeout(timer));
}

test('ending a borrowed Readable wait removes only owned listeners and preserves the caller stream and future bytes', async () => {
  const input = new PassThrough(), caller = () => {};
  for (const event of events) input.on(event, caller);
  const baseline = listeners(input), cursor = borrowedProcessInput(input);
  const waiting = cursor.next();
  cursor.close(); cursor.close();
  assert.deepEqual(await bounded(waiting), { done: true, value: undefined });
  assert.deepEqual(listeners(input), baseline);
  assert.equal(input.destroyed, false); assert.equal(input.isPaused(), true);
  input.write(Buffer.from('still usable é東京'));
  assert.equal(input.read().toString(), 'still usable é東京');
  input.destroy();
});

test('borrowed input preserves queued bytes while an earlier chunk is in flight and drains EOF in order', async () => {
  const input = new PassThrough(), cursor = borrowedProcessInput(input);
  input.write(Buffer.from('first')); assert.equal((await cursor.next()).value.toString(), 'first');
  input.write(Buffer.from('unread')); cursor.close();
  assert.equal(input.destroyed, false); assert.equal(input.read().toString(), 'unread');
  const later = borrowedProcessInput(input);
  input.end(Buffer.from('last'));
  assert.equal((await later.next()).value.toString(), 'last');
  assert.equal((await bounded(later.next())).done, true);
  later.close(); assert.deepEqual(events.map(event => input.listenerCount(event)), [0, 0, 0, 0]);
});

test('input errors and premature close reject the pending read and cleanup leaves caller error handling intact', async () => {
  for (const error of [new Error('input failed'), undefined]) {
    const input = new PassThrough(), caller = () => {}; input.on('error', caller);
    const baseline = listeners(input), cursor = borrowedProcessInput(input), pending = cursor.next();
    const rejected = assert.rejects(bounded(pending), error ? /input failed/ : /closed before EOF/);
    input.destroy(error); await rejected; cursor.close();
    assert.deepEqual(listeners(input), baseline);
  }
});

test('cooperative generic iterator return cancels pending input exactly once without forwarding a late value', async () => {
  let finish, returns = 0, reads = 0;
  const input = { [Symbol.asyncIterator]() { return {
    next() { reads++; return new Promise(resolve => { finish = resolve; }); },
    return() { returns++; finish({ done: false, value: Buffer.from('late') }); return Promise.resolve({ done: true }); },
  }; } };
  const cursor = borrowedProcessInput(input), pending = cursor.next(); cursor.close(); cursor.close();
  assert.deepEqual(await bounded(pending), { done: true, value: undefined });
  assert.equal((await cursor.next()).done, true); assert.equal(reads, 1); assert.equal(returns, 1);
});

test('generic input retains normal byte order and EOF, and failed input requests cooperative cleanup', async () => {
  const output = [], cursor = borrowedProcessInput((async function* () { yield Buffer.from('é'); yield Buffer.from('東京'); })());
  for (;;) { const next = await cursor.next(); if (next.done) break; output.push(next.value); }
  cursor.close(); assert.equal(Buffer.concat(output).toString(), 'é東京');
  let returned = false;
  const failed = borrowedProcessInput({ [Symbol.asyncIterator]() { return {
    next: () => Promise.reject(Error('generic input failed')),
    return: () => { returned = true; return Promise.reject(Error('foreign return failed')); },
  }; } });
  await assert.rejects(failed.next(), /generic input failed/); failed.close(); assert.equal(returned, true);
});

test('an unbounded foreign iterator return cannot keep the adapter-owned input wait pending', async () => {
  let requested = false;
  const cursor = borrowedProcessInput({ [Symbol.asyncIterator]() { return {
    next: () => new Promise(() => {}),
    return: () => { requested = true; return new Promise(() => {}); },
  }; } });
  const pending = cursor.next(); cursor.close();
  assert.deepEqual(await bounded(pending), { done: true, value: undefined }); assert.equal(requested, true);
});
