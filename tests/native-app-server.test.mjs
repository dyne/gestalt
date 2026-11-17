import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import { createInterface } from 'node:readline';
import { proxyAppServer } from '../src/cli/runtime/app-server.ts';
import test from 'node:test';
import { spawn } from 'node:child_process';

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

test('standalone AppServer naturally exits after result7 with actual caller stdin held open', { timeout: 10000 }, async t => {
  const program = `import {PassThrough} from 'node:stream';import {createInterface} from 'node:readline';
import {proxyAppServer} from ${JSON.stringify(import.meta.resolve('../src/cli/runtime/app-server.ts'))};
${deferred.toString()}
${controlledPort.toString()}
const control=controlledPort();const write=(stream,bytes)=>new Promise((resolve,reject)=>stream.write(bytes,error=>error?reject(error):resolve()));
const status=await proxyAppServer({argv:['--stdio'],cwd:process.cwd(),environment:{PATH:'',CODEX_HOME:process.cwd()}},
{stdin:process.stdin,stdout:bytes=>write(process.stdout,bytes),stderr:bytes=>write(process.stderr,bytes)},process.execPath,process.execPath,control.port);
const observe=phase=>process.stdout.write(JSON.stringify({fixtureSettled:status,phase,
resources:process.getActiveResourcesInfo(),stdinDestroyed:process.stdin.destroyed,stdinPaused:process.stdin.isPaused(),stdinReadableFlowing:process.stdin.readableFlowing,
stdinListeners:Object.fromEntries(['readable','data','end','close','error','pause','resume'].map(event=>[event,process.stdin.listenerCount(event)])),
stdinInternalObservation:{handleHasRef:process.stdin._handle?.hasRef?.(),handleReading:process.stdin._handle?.reading,readableStateReading:process.stdin._readableState?.reading},
scope:'Passive local Node stdin/resource diagnosis; internal fields are version-specific observations, not a product API'})+'\\n');
observe('proxy-returned');setImmediate(()=>observe('next-event-loop-turn'));process.exitCode=status;`;
  const child = spawn(process.execPath, ['--input-type=module', '-e', program], { stdio: ['pipe', 'pipe', 'pipe'], shell: false });
  const errors = [], lines = createInterface({ input: child.stdout }), observed = [], waiters = new Map();
  child.stderr.on('data', bytes => errors.push(Buffer.from(bytes)));
  lines.on('line', line => { const value = JSON.parse(line); observed.push(value); waiters.get(value.id ?? 'settled')?.(value); });
  const closed = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', (code, signal) => resolve({ code, signal })); });
  let deadline = false;
  const timer = setTimeout(() => { deadline = true; child.kill('SIGKILL'); }, 4000);
  t.after(async () => { clearTimeout(timer); if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL'); await closed; child.stdin.destroy(); lines.close(); });
  const history = new Promise(resolve => waiters.set('history', resolve));
  child.stdin.write('{"id":"history","method":"thread/read","params":{}}\n');
  assert.equal((await history).result.history.length, 2 * 1024 * 1024);
  child.stdin.write('{"method":"exit"}\n'); // Deliberately keep the caller's real pipe open.
  const result = await closed;
  assert.equal(deadline, false, JSON.stringify({ result, observed: observed.map(v => v.fixtureSettled !== undefined ? v : { id: v.id }), stderr: Buffer.concat(errors).toString(), callerStdinEnded: child.stdin.writableEnded }));
  assert.equal(child.stdin.writableEnded, false); assert.deepEqual(result, { code: 7, signal: null });
});

test('AppServer cancels a cooperative generic input iterator after owned result without waiting for caller EOF', { timeout: 10000 }, async () => {
  const control = controlledPort(), waiting = deferred(), history = deferred();
  let delivered = false, returned = 0, pending, cancelled = false;
  const iterator = {
    next() {
      if (!delivered) { delivered = true; return Promise.resolve({ done: false, value: Buffer.from('{"id":"history","method":"thread/read","params":{}}\n') }); }
      waiting.resolve(); return new Promise(resolve => { pending = resolve; });
    },
    return() { returned++; cancelled = true; pending?.({ done: true, value: undefined }); return Promise.resolve({ done: true, value: undefined }); },
  };
  const result = proxyAppServer({ argv: ['--stdio'], cwd: process.cwd(), environment: { PATH: '', CODEX_HOME: process.cwd() } }, {
    stdin: { [Symbol.asyncIterator]: () => iterator },
    stdout: async bytes => { const value = JSON.parse(Buffer.from(bytes)); if (value.id === 'history') history.resolve(value); },
    stderr: async bytes => { throw Error(Buffer.from(bytes).toString()); },
  }, process.execPath, process.execPath, control.port);
  assert.equal((await history.promise).result.history.length, 2 * 1024 * 1024);
  await waiting.promise; control.command.resolve({ kind: 'exit', code: 7 });
  assert.equal(await result, 7); assert.equal(returned, 1); assert.equal(cancelled, true);
  assert.ok(control.stopped.length > 0); assert.equal(control.received.filter(value => value.id === 'history').length, 1);
});
