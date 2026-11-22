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
import { parse } from 'smol-toml';
import { createHash } from 'node:crypto';

async function fixture(t, discovery = true, terminal = false, managedHooks = false, refuseHooks = false, timing) {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt capability host ü ')); t.after(() => rm(root, { recursive: true, force: true }));
  const bin = path.join(root, 'bin'); await mkdir(bin); const body = '#!/usr/bin/env node\n';
  await writeFile(path.join(bin, 'codex.mjs'), body, { mode: 0o755 });
  if (process.platform === 'win32') {
    const captured = JSON.parse(await readFile(new URL('./fixtures/native-npm-node-shim.json', import.meta.url), 'utf8'));
    await writeFile(path.join(bin, 'codex.cmd'), capturedNodeCommand(captured, 'codex.mjs'));
  } else await writeFile(path.join(bin, 'codex'), body, { mode: 0o755 });
  const calls = [], nativeInput = [], out = [], err = [], input = Buffer.from([0, 255, 13, 10, 128]), terminalEvents = [];
  const nativeConfig = { features: { hooks: managedHooks }, approval_policy: 'on-request', ...(managedHooks ? { mcp_servers: {
    'context-mode': { command: process.execPath, args: [path.join(root, 'bin/context-mode-mcp.mjs')], enabled: false,
      env: { RETAINED: 'literal ü' }, tools: { selected: { approval_mode: 'prompt' } } },
    operator: { url: 'https://operator.invalid/private', enabled: false },
  } } : {}) };
  const port = { async launch(executable, invocation, streams) {
    timing?.beforeLaunch?.(invocation);
    calls.push(invocation); let stopped = false, resolveResult, resolveComplete;
    const result = new Promise(r => resolveResult = r), completed = new Promise(r => resolveComplete = r);
    const claim = { kind: 'released', lease: String(calls.length), backend: 'controlled', result: { kind: 'exit', code: 23 } };
    const lease = { id: claim.lease, result, completed, claim: () => stopped ? claim : { kind: 'active', lease: claim.lease, backend: 'controlled' },
      stop: async () => { if (!stopped) { stopped = true; timing?.stop?.(invocation); } resolveResult(claim.result); resolveComplete(claim); return claim; } };
    if (invocation.argv[0] === 'app-server') {
      timing?.launch?.(invocation);
      if (!discovery) { resolveResult({ kind: 'exit', code: 1 }); return lease; }
      const lines = createInterface({ input: Readable.from(streams.stdin) });
      void (async () => { for await (const line of lines) {
        const message = JSON.parse(line); if (message.id === undefined) continue;
        timing?.reply?.(invocation, message);
        if (message.method === 'hooks/list' && refuseHooks) {
          await streams.stdout(Buffer.from(JSON.stringify({ id: message.id, error: { message: 'controlled hook query refusal' } }) + '\n')); continue;
        }
        const hooks = [];
        for (const argument of invocation.argv) for (const event of ['SessionStart', 'SubagentStart']) if (argument.startsWith(`hooks.${event}=`)) {
          const groups = parse('value=' + argument.slice(argument.indexOf('=') + 1)).value;
          for (const [index, group] of groups.entries()) for (const hook of group.hooks)
            hooks.push({ ...hook, source: 'sessionFlags', eventName: event, key: `C:\\<session-flags>\\config.toml:${event}:${index}:0`,
              currentHash: 'sha256:' + createHash('sha256').update(event + hook.command).digest('hex') });
        }
        const value = message.method === 'initialize' ? { userAgent: 'codex/0.160.0 controlled' } : message.method === 'config/read' ?
          { config: nativeConfig, layers: [] } : message.method === 'hooks/list' ? { data: [{ cwd: root, hooks }] } : { data: [] };
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
  return { invocation, input, calls, nativeInput, out, err, terminalEvents, nativeConfig, commands: new RuntimeCommands(invocation, streams, process.execPath, port,
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

test('rejected final native launch reports its bounded cause and preserves127/unknown ownership', { timeout: 10000 }, async t => {
  const f = await fixture(t, true, false, false, false, {
    beforeLaunch(invocation) { if (invocation.argv[0] !== 'app-server') throw Error('controlled launch failure\n' + 'x'.repeat(1024)); },
  });
  assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 127);
  const diagnostic = Buffer.concat(f.err).toString().split('\n').find(line => line.startsWith('gestalt: native command launch failed'));
  assert.ok(diagnostic.includes('controlled launch failure ')); assert.ok(diagnostic.length < 560);
  assert.equal(f.commands.unknown.length, 1); assert.equal(f.commands.unknown[0].kind, 'unknown'); assert.equal(f.commands.active.size, 0);
  assert.equal(f.calls.filter(call => call.argv[0] !== 'app-server').length, 0);
});

for (const refusal of [false, true]) test(`managed native hook trust ${refusal ? 'refusal omits generated hooks' : 'preserves transport and scopes exact trust'}`, { timeout: 10000 }, async t => {
  const f = await fixture(t, true, false, true, refusal);
  assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 23);
  const final = f.calls.find(call => call.argv[0] !== 'app-server'), values = final.argv.filter((_, i, a) => i > 0 && a[i - 1] === '-c');
  const server = JSON.parse(JSON.stringify(parse('value=' + values.find(v => v.startsWith('mcp_servers.context-mode=')).slice('mcp_servers.context-mode='.length)).value));
  assert.deepEqual(server, { ...f.nativeConfig.mcp_servers['context-mode'],
    env_vars: ['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'], default_tools_approval_mode: 'approve', tools: { selected: { approval_mode: 'approve' } } });
  assert.equal(values.some(v => v.startsWith('mcp_servers.context-mode.')), false, 'Partial policy override must not replace native transport');
  assert.equal(values.some(v => v.includes('operator.invalid')), false, 'Unrelated inherited server must not enter overrides');
  assert.equal(values.includes('approval_policy="on-request"'), true);
  if (refusal) {
    assert.equal(values.some(v => /^hooks\.(SessionStart|SubagentStart|state)=/.test(v)), false);
    assert.match(Buffer.concat(f.err).toString(), /guidance-hooks-unavailable/);
  } else {
    const state = parse('value=' + values.find(v => v.startsWith('hooks.state=')).slice('hooks.state='.length)).value;
    const query = f.calls.find(call => call.argv.some(v => v.startsWith('hooks.SessionStart=')));
    for (const event of ['SessionStart', 'SubagentStart']) {
      const groups = parse('value=' + query.argv.find(v => v.startsWith(`hooks.${event}=`)).slice(`hooks.${event}=`.length)).value;
      for (const [index, group] of groups.entries()) for (const hook of group.hooks)
        assert.deepEqual({ ...state[`C:\\<session-flags>\\config.toml:${event}:${index}:0`] }, { enabled: true,
          trusted_hash: 'sha256:' + createHash('sha256').update(event + hook.command).digest('hex') });
    }
    assert.equal(Object.keys(state).length, 2);
  }
  assert.equal(f.commands.unknown.length, 0); assert.equal(f.commands.active.size, 0);
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

test('a selected skill profile produces one session override and stays out of the Codex environment', async t => {
  const f = await fixture(t, true, true, true);
  const rule = { path: path.resolve('skills/work/SKILL.md'), enabled: false };
  f.invocation.environment.GESTALT_LAUNCH_SKILL_RULES = JSON.stringify([rule]);
  assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 23);
  assert.ok(f.calls.every(call => !Object.hasOwn(call.environment, 'GESTALT_LAUNCH_SKILL_RULES')));
  const launch = f.calls.find(call => call.argv[0] !== 'app-server');
  const values = launch.argv.filter(value => value.startsWith('skills.config='));
  assert.equal(values.length, 1);
  assert.deepEqual({ ...parse(values[0]).skills.config[0] }, rule);
  assert.ok(launch.argv.some(value => value.startsWith('hooks.state=')));
});

function capabilityClock(t) {
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'], now: 100000 });
  const original = globalThis.setTimeout, snapshots = [];
  t.mock.method(globalThis, 'setTimeout', function (callback, delay, ...args) {
    if (/\bat snapshot\s+\(/.test(new Error().stack)) snapshots.push({ delay, createdAt: Date.now() });
    return Reflect.apply(original, this, [callback, delay, ...args]);
  });
  return { snapshots, advance: milliseconds => t.mock.timers.tick(milliseconds) };
}
test('shared capability deadline charges both query startups and cleanup without resetting the query cap', { timeout: 10000 }, async t => {
  let clock, queries = 0;
  const stopped = new Set();
  const f = await fixture(t, true, false, true, false, {
    launch() { queries++; clock.advance(1800); },
    reply(_invocation, message) { if (queries === 1 && message.method === 'config/read') clock.advance(200); },
    stop(invocation) {
      stopped.add(invocation);
      if (invocation.argv[0] === 'app-server' && !invocation.argv.some(value => value.startsWith('hooks.SessionStart='))) clock.advance(1500);
    },
  });
  clock = capabilityClock(t);
  assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 23);
  const windows = process.platform === 'win32', final = f.calls.find(call => call.argv[0] !== 'app-server');
  assert.deepEqual(clock.snapshots.map(timer => timer.delay), windows ? [5000, 5000] : [5000, 1500]);
  assert.deepEqual(clock.snapshots.map(timer => timer.createdAt), [100000, 103500]);
  assert.equal(final.argv.some(value => value.startsWith('hooks.state=')), windows);
  assert.equal(Buffer.concat(f.err).toString().includes('guidance-hooks-unavailable'), !windows);
  assert.equal(queries, 2); assert.equal(stopped.size, f.calls.length);
  assert.equal(f.commands.active.size, 0); assert.equal(f.commands.unknown.length, 0);
});
test('shared capability deadline retains the 5000ms individual query cap during startup', { timeout: 10000 }, async t => {
  let clock;
  const stopped = new Set();
  const f = await fixture(t, true, false, true, false, {
    launch() { clock.advance(5100); }, stop(invocation) { stopped.add(invocation); },
  });
  clock = capabilityClock(t);
  assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 23);
  assert.deepEqual(clock.snapshots.map(timer => timer.delay), [5000]);
  assert.equal(f.calls.filter(call => call.argv[0] === 'app-server').length, 1);
  assert.match(Buffer.concat(f.err).toString(), /native-config-unavailable/);
  assert.equal(f.calls.find(call => call.argv[0] !== 'app-server').argv.some(value => value.startsWith('hooks.state=')), false);
  assert.equal(stopped.size, f.calls.length);
  assert.equal(f.commands.active.size, 0); assert.equal(f.commands.unknown.length, 0);
});
test('shared capability deadline rejects the second query before launch after global exhaustion', { timeout: 10000 }, async t => {
  let clock;
  const stopped = new Set();
  const f = await fixture(t, true, false, true, false, {
    stop(invocation) {
      stopped.add(invocation);
      if (invocation.argv[0] === 'app-server') clock.advance(process.platform === 'win32' ? 11000 : 6000);
    },
  });
  clock = capabilityClock(t);
  assert.equal(await launchCapabilities(f.commands, f.invocation, process.execPath), 23);
  assert.deepEqual(clock.snapshots.map(timer => timer.delay), [5000]);
  assert.equal(f.calls.filter(call => call.argv[0] === 'app-server').length, 1);
  assert.match(Buffer.concat(f.err).toString(), /guidance-hooks-unavailable/);
  assert.equal(f.calls.find(call => call.argv[0] !== 'app-server').argv.some(value => value.startsWith('hooks.state=')), false);
  assert.equal(stopped.size, f.calls.length);
  assert.equal(f.commands.active.size, 0); assert.equal(f.commands.unknown.length, 0);
});
