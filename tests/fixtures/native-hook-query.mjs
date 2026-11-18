// Isolated Codex hook fixture, never a product process or ownership adapter.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { writeFile } from 'node:fs/promises';
import { createInterface } from 'node:readline';
import { bounded } from './native-runtime/codex.mjs';

const errorRecord = error => error ? { name: error.name, message: error.message, code: error.code, stack: error.stack } : null;

export async function queryFixtureHooks(binary, environment, cwd, overrides, receipt, { cleanupMs = 20000 } = {}) {
  const argv = ['app-server', '--stdio', ...overrides], events = [], pending = new Map();
  const closed = Promise.withResolvers(), protocolFailure = Promise.withResolvers();
  let id = 0, phase = 'spawn', stderrBytes = 0, stderr = Buffer.alloc(0), directExit = null, close = null;
  let primaryFailure, cleanupFailure, result;
  const mark = (event, details = {}) => events.push({ event, at: new Date().toISOString(), phase, ...details });
  const child = spawn(binary, argv, { env: environment, cwd, stdio: ['pipe', 'pipe', 'pipe'], shell: false });
  const lines = createInterface({ input: child.stdout });
  mark('spawn-request', { pid: child.pid ?? null });
  child.once('spawn', () => mark('spawn', { pid: child.pid }));
  child.once('exit', (code, signal) => { directExit = { code, signal }; mark('exit', directExit); });
  child.once('close', (code, signal) => { close = { code, signal }; mark('close', close); closed.resolve(); });
  const streamError = (source, error) => {
    mark('error', { source, message: error.message, code: error.code });
    protocolFailure.resolve(error);
  };
  child.on('error', error => streamError('process', error));
  lines.on('error', error => streamError('readline', error));
  for (const [name, stream] of [['stdin', child.stdin], ['stdout', child.stdout], ['stderr', child.stderr]]) {
    stream.on('error', error => streamError(name, error));
    stream.once('close', () => mark('pipe-close', { pipe: name }));
    stream.once(name === 'stdin' ? 'finish' : 'end', () => mark('pipe-end', { pipe: name }));
  }
  child.stderr.on('data', bytes => {
    stderrBytes += bytes.length;
    if (stderr.length < 65536) stderr = Buffer.concat([stderr, bytes.subarray(0, 65536 - stderr.length)]);
  });
  lines.on('line', line => {
    try {
      const message = JSON.parse(line), resolve = pending.get(message.id);
      if (resolve) {
        mark('rpc-response', { id: message.id, hasError: message.error !== undefined });
        pending.delete(message.id);
        resolve(message);
      }
    } catch (error) { streamError('JSON response', error); }
  });
  async function rpc(method, params) {
    phase = method;
    const messageId = ++id, reply = Promise.withResolvers();
    pending.set(messageId, reply.resolve);
    mark('rpc-send', { id: messageId });
    try {
      child.stdin.write(JSON.stringify({ id: messageId, method, params }) + '\n');
      const failure = error => { throw error; };
      const response = await bounded(Promise.race([
        reply.promise,
        protocolFailure.promise.then(failure),
        closed.promise.then(() => failure(Error(`Fixture app-server closed before ${method} reply`))),
      ]), method);
      assert.equal(response.error, undefined, JSON.stringify(response));
      return response;
    } finally { pending.delete(messageId); }
  }
  try {
    await rpc('initialize', { clientInfo: { name: 'gestalt-core-slice-hook-fixture', version: '1' }, capabilities: {} });
    child.stdin.write('{"method":"initialized"}\n');
    result = await rpc('hooks/list', { cwds: [cwd] });
  } catch (error) {
    primaryFailure = error;
    mark('rpc-failure', { message: error.message });
  }
  phase = 'cleanup';
  // Keep the original termination sequence and deadline during diagnosis.
  // The kill return describes signal dispatch, never process or tree release.
  try {
    lines.close();
    child.stdin.end();
    mark('kill', { returned: child.kill() });
    await bounded(closed.promise, 'fixture config query cleanup', cleanupMs);
  } catch (error) {
    cleanupFailure = error;
    mark('cleanup-failure', { message: error.message });
  }

  const diagnostics = {
    observedAt: new Date().toISOString(), pid: child.pid ?? null, cwd, argv, events: [...events],
    primaryFailure: errorRecord(primaryFailure), cleanupFailure: errorRecord(cleanupFailure),
    directExit, close, stderr: stderr.toString(), stderrBytes, closedBeforeDeadline: close !== null,
    scope: 'Direct child and parent-side pipes only. No descendant or complete-tree settlement claim.',
  };
  if (!close) {
    // Release only our own stream/process references after recording the
    // failed boundary. This cannot promote missing closure to a pass.
    child.stdin.destroy();
    child.stdout.destroy();
    child.stderr.destroy();
    child.unref();
  }
  try { await writeFile(receipt, JSON.stringify(diagnostics, null, 2) + '\n'); }
  catch (error) {
    if (primaryFailure || cleanupFailure)
      throw new AggregateError([primaryFailure, cleanupFailure, error].filter(Boolean), 'Fixture query receipt failed');
    throw error;
  }
  if (primaryFailure) throw primaryFailure;
  if (cleanupFailure) throw cleanupFailure;
  return { result, stderr: diagnostics.stderr, overrides };
}
