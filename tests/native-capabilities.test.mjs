import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { createInterface } from 'node:readline';
import { Readable } from 'node:stream';
import test from 'node:test';
import { launchCapabilities } from '../src/cli/runtime/capabilities.mjs';
import { RuntimeCommands } from '../src/cli/runtime/commands.ts';
import { capturedNodeCommand } from './fixtures/native-node-command.mjs';

async function fixture(t, discovery = true, terminal = false) {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt capability host ü ')); t.after(() => rm(root, { recursive: true, force: true }));
  const bin = path.join(root, 'bin'); await mkdir(bin); const body = '#!/usr/bin/env node\n';
  await writeFile(path.join(bin, 'codex.mjs'), body, { mode: 0o755 });
  if (process.platform === 'win32') {
    const captured = JSON.parse(await readFile(new URL('./fixtures/native-npm-node-shim.json', import.meta.url), 'utf8'));
    await writeFile(path.join(bin, 'codex.cmd'), capturedNodeCommand(captured, 'codex.mjs'));
  } else await writeFile(path.join(bin, 'codex'), body, { mode: 0o755 });
  const calls = [], nativeInput = [], out = [], err = [], input = Buffer.from([0, 255, 13, 10, 128]), terminalEvents = [];
  const port = { async launch(executable, invocation, streams) {
    calls.push(invocation); let stopped = false, resolveResult, resolveComplete;
    const result = new Promise(r => resolveResult = r), completed = new Promise(r => resolveComplete = r);
    const claim = { kind: 'released', lease: String(calls.length), backend: 'controlled', result: { kind: 'exit', code: 23 } };
    const lease = { id: claim.lease, result, completed, claim: () => stopped ? claim : { kind: 'active', lease: claim.lease, backend: 'controlled' },
      stop: async () => { stopped = true; resolveResult(claim.result); resolveComplete(claim); return claim; } };
    if (invocation.argv[0] === 'app-server') {
      if (!discovery) { resolveResult({ kind: 'exit', code: 1 }); return lease; }
      const lines = createInterface({ input: Readable.from(streams.stdin) });
      void (async () => { for await (const line of lines) {
        const message = JSON.parse(line); if (message.id === undefined) continue;
        const value = message.method === 'initialize' ? { userAgent: 'codex/0.160.0 controlled' } : message.method === 'config/read' ? { config: { features: { hooks: false }, approval_policy: 'on-request' }, layers: [] } : { data: [] };
        await streams.stdout(Buffer.from(JSON.stringify({ id: message.id, result: value }) + '\n'));
      } })().catch(() => {});
    } else void (async () => {
      for await (const bytes of streams.stdin) nativeInput.push(Buffer.from(bytes));
      await streams.stdout(Buffer.from([255, 0, 13, 10])); await streams.stderr(Buffer.from('native-stderr\n')); resolveResult(claim.result);
    })();
    return lease;
  } };
  if (terminal) port.launchTerminal = async (executable, invocation, streams, size) => {
    terminalEvents.push({ kind: 'launch', size });
    const lease = await port.launch(executable, invocation, { stdin: streams.input, stdout: streams.output, stderr: streams.output });
    return { ...lease, resize: async size => { terminalEvents.push({ kind: 'resize', size }); } };
  };
  const argv = ['-m', 'operator-model', '-c', 'approval_policy="on-request"', 'exec', '--', 'literal $HOME', '', '東京'];
  const invocation = { argv, cwd: root, environment: { PATH: bin, PATHEXT: '.CMD;.EXE', CODEX_HOME: root, GESTALT_HOME: root, TMPDIR: 'operator-tmp-value', RETAINED: 'kept' } };
  const streams = { stdin: (async function* () { yield input; })(), stdout: async bytes => out.push(Buffer.from(bytes)), stderr: async bytes => err.push(Buffer.from(bytes)) };
  return { invocation, input, calls, nativeInput, out, err, terminalEvents, commands: new RuntimeCommands(invocation, streams, process.execPath, port,
    terminal ? { size: () => ({ columns: 113, rows: 39 }), enter: () => { terminalEvents.push({ kind: 'enter' }); return () => terminalEvents.push({ kind: 'restore' }); },
      onResize: listener => { terminalEvents.push({ kind: 'subscribe' }); listener(); return () => terminalEvents.push({ kind: 'unsubscribe' }); } } : undefined) };
}
for (const discovery of [true, false]) test(`host capability composition preserves native argv/streams/defaults with discovery ${discovery ? 'available' : 'unavailable'}`, { timeout: 10000 }, async t => {
  const f = await fixture(t, discovery); assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 23);
  const launched = f.calls.find(call => call.argv[0] !== 'app-server'); assert.ok(launched);
  const actual = launched.argv, original = f.invocation.argv;
  // Inserted owned flags may occur only before the native '--' boundary. Every
  // original token remains literal, including an empty token and shell syntax.
  const originalTail = original.slice(original.indexOf('--'));
  assert.deepEqual(actual.slice(actual.indexOf('--')), originalTail);
  assert.deepEqual(actual.slice(1, 5), original.slice(0, 4));
  assert.deepEqual(Buffer.concat(f.nativeInput), f.input); assert.deepEqual(Buffer.concat(f.out), Buffer.from([255, 0, 13, 10]));
  assert.equal(launched.environment.RETAINED, 'kept'); assert.equal(launched.environment.TMPDIR, 'operator-tmp-value');
  assert.match(Buffer.concat(f.err).toString(), /native-stderr/); assert.equal(f.commands.unknown.length, 0);
  assert.equal(actual.filter(value => value === 'operator-model').length, 1);
});

test('interactive capability launch uses the owned terminal and restores it after settlement', { timeout: 10000 }, async t => {
  const f = await fixture(t, true, true);
  assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 23);
  assert.equal(f.calls.filter(call => call.argv[0] !== 'app-server').length, 1);
  assert.deepEqual(f.terminalEvents.map(event => event.kind), ['enter', 'launch', 'subscribe', 'resize', 'unsubscribe', 'restore']);
  assert.deepEqual(f.terminalEvents.find(event => event.kind === 'launch').size, { columns: 113, rows: 39 });
  assert.deepEqual(Buffer.concat(f.nativeInput), f.input);
  assert.deepEqual(Buffer.concat(f.out), Buffer.concat([Buffer.from([255, 0, 13, 10]), Buffer.from('native-stderr\n')]));
  assert.equal(f.commands.active.size, 0); assert.equal(f.commands.unknown.length, 0);
});
