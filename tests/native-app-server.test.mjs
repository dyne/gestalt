import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import { createInterface } from 'node:readline';
import { proxyAppServer } from '../src/cli/runtime/app-server.ts';
import test from 'node:test';

function deferred() { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; }
function controlledPort(mode = 'released') {
  const calls = [], stopped = [], received = [], command = deferred(), complete = deferred(), stopStarted = deferred(), release = deferred();
  return { calls, stopped, received, command, stopStarted, release, port: { async launch(executable, invocation, streams) {
    calls.push({ executable, invocation }); let claim = { kind: 'active', lease: 'controlled', backend: 'controlled-native-fixture' };
    const lines = createInterface({ input: PassThrough.from(streams.stdin) });
    const config = { model: 'operator-model', mcp_servers: { 'gestalt-serena': { command: 'gestalt', args: ['serena', 'mcp'], enabled: false, default_tools_approval_mode: 'prompt' },
      custom: { command: 'custom', default_tools_approval_mode: 'prompt' } } };
    void (async () => {
      for await (const line of lines) {
        const message = JSON.parse(line); received.push(message);
        if (message.method === 'exit') { command.resolve({ kind: 'exit', code: 7 }); break; }
        if (mode === 'baseline-error' && String(message.id).startsWith('gestalt-approvals-')) {
          await streams.stdout(Buffer.from(JSON.stringify({ id: message.id, error: { code: -1, message: 'controlled config unavailable' } }) + '\n')); continue;
        }
        const result = message.method === 'config/read' ? { config } : message.method === 'thread/read' ? { history: 'x'.repeat(2 * 1024 * 1024) } : { params: message.params };
        if (message.id !== undefined) await streams.stdout(Buffer.from(JSON.stringify({ id: message.id, result }) + '\n'));
      }
    })().catch(() => {});
    const lease = { id: 'controlled', result: command.promise, completed: complete.promise, claim: () => claim, stop: async () => {
      stopped.push('stop'); stopStarted.resolve();
      if (mode === 'held-release') await release.promise;
      lines.close(); command.resolve({ kind: 'exit', code: 0 });
      claim = mode !== 'unknown' ? { kind: 'released', lease: 'controlled', backend: 'controlled-native-fixture', result: await command.promise } : { kind: 'unknown', lease: 'controlled', reason: 'controlled absence unknown' };
      complete.resolve(claim); return claim;
    } }; return lease;
  } } };
}
async function run(t, mode = 'released') {
  const control = controlledPort(mode), input = new PassThrough(), out = new PassThrough(), errors = [], messages = [], waiters = new Map();
  const lines = createInterface({ input: out }); lines.on('line', line => { const value = JSON.parse(line); messages.push(value); waiters.get(value.id)?.(value); });
  const response = id => new Promise(resolve => waiters.set(id, resolve));
  const invocation = { argv: ['--stdio', 'space ü', '', '--literal=$HOME'], cwd: process.cwd(), environment: { PATH: '', CODEX_HOME: process.cwd(), RETAINED: 'literal' } };
  const result = proxyAppServer(invocation, { stdin: input, stdout: async bytes => { out.write(bytes); }, stderr: async bytes => errors.push(Buffer.from(bytes)) }, process.execPath, process.execPath, control.port);
  t.after(() => { input.destroy(); out.destroy(); lines.close(); });
  return { control, input, messages, response, result, invocation, errors };
}
test('host AppServer policy proxy preserves exact argv and unrelated protocol/config options, settling owned work', { timeout: 10000 }, async t => {
  const f = await run(t), reply = f.response(1);
  f.input.write(JSON.stringify({ id: 1, method: 'thread/resume', params: { threadId: 'native-ID', model: 'chosen-model', permissions: { selected: ':workspace' }, config: { 'mcp_servers.custom.default_tools_approval_mode': 'prompt' } } }) + '\n');
  const result = await reply;
  assert.equal(result.result.params.threadId, 'native-ID'); assert.equal(result.result.params.model, 'chosen-model');
  assert.deepEqual(result.result.params.permissions, { selected: ':workspace' });
  assert.equal(result.result.params.config.mcp_servers['gestalt-serena'].enabled, false);
  assert.equal(result.result.params.config.mcp_servers['gestalt-serena'].default_tools_approval_mode, 'approve');
  assert.equal(result.result.params.config['mcp_servers.custom.default_tools_approval_mode'], 'prompt');
  f.input.end(); assert.equal(await f.result, 0); assert.ok(f.control.stopped.length);
  assert.deepEqual(f.control.calls[0].invocation.argv, ['app-server', ...f.invocation.argv]);
  assert.equal(f.control.calls[0].invocation.environment.RETAINED, 'literal');
  assert.equal(f.errors.length, 0);
});
test('host AppServer proxy drains a large history response and exits when command ends with caller input held open', { timeout: 10000 }, async t => {
  const f = await run(t), reply = f.response('history');
  f.input.write(JSON.stringify({ id: 'history', method: 'thread/read', params: { threadId: 'native-ID' } }) + '\n');
  assert.equal((await reply).result.history.length, 2 * 1024 * 1024);
  f.input.write('{"method":"exit"}\n'); assert.equal(await f.result, 7); assert.ok(f.control.stopped.length);
});
test('host AppServer proxy returns policy error without forwarding a thread when baseline cannot be prepared', { timeout: 10000 }, async t => {
  const f = await run(t, 'baseline-error'), reply = f.response(2);
  f.input.write('{"id":2,"method":"thread/start","params":{}}\n');
  assert.equal((await reply).error.code, -32603);
  assert.ok(f.control.received.every(message => message.method !== 'thread/start'));
  f.input.end(); assert.equal(await f.result, 0);
});
test('host AppServer proxy preserves unknown lifetime separately from a nonzero command outcome', { timeout: 10000 }, async t => {
  const f = await run(t, 'unknown'); f.input.write('{"method":"exit"}\n');
  assert.equal(await f.result, 7); assert.match(Buffer.concat(f.errors).toString(), /unconfirmed.*protected/);
});
test('controlled AppServer interruption waits for owned settlement and preserves the interrupt result', { timeout: 10000 }, async t => {
  const f = await run(t, 'held-release'), reply = f.response(1);
  f.input.write('{"id":1,"method":"initialize","params":{}}\n'); await reply;
  let settled = false; void f.result.then(() => { settled = true; });
  try {
    process.emit('SIGTERM'); // Controlled listener injection, not an OS signal or active-model cancellation.
    await f.control.stopStarted.promise; await new Promise(resolve => setImmediate(resolve));
    assert.equal(settled, false, 'Proxy returned before its owned stop settled');
  } finally { f.control.release.resolve(); }
  assert.equal(await f.result, 143); assert.ok(f.control.stopped.length > 0); assert.equal(f.errors.length, 0);
});
