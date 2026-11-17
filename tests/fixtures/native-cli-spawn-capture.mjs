// Passive, test-only argv observation. No stream listener, shell, ownership
// replacement or process result synthesis. execve discards the initial import;
// the unchanged Unix bootstrap loads it again through this fixture's NODE_OPTIONS.
import childProcess from 'node:child_process';
import { appendFileSync, writeFileSync } from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
import { performance } from 'node:perf_hooks';

const ownerNames = ['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'];
const stamp = () => ({ observedAt: new Date().toISOString(), monotonicMs: performance.now() });
export function queryPhase(argv) {
  if (argv[0] === 'plugin' && argv[1] === 'list') return 'required-inventory';
  if (argv[0] === 'mcp' && argv[1] === 'get') return 'required-mcp';
  if (argv[0] === 'app-server') return argv.some(arg => arg.startsWith('hooks.SessionStart=')) ? 'capability-hooks' : 'capability-config';
  return argv.includes('exec') && argv.includes('--help') ? 'final-help' : 'unexpected';
}

/** Diagnostic tee only. No stream listener, read, callback replacement, timer,
 * retry or ownership result is introduced. Units exercise these wrappers with
 * controlled objects; native correctness is established only by the target run. */
export function createReadinessObserver({ maxBytes = 8 * 1024 * 1024, maxRecords = 10000 } = {}) {
  const events = [], channels = [], timers = new WeakMap(), argvCalls = [], errors = [];
  let used = 0, overflow = false, dropped = 0, nextTimer = 0;
  const error = reason => { if (errors.length < 16) errors.push(String(reason).slice(0, 512)); };
  const reserve = bytes => {
    if (overflow || used + bytes > maxBytes || events.length + argvCalls.length >= maxRecords) { overflow = true; dropped++; return false; }
    used += bytes; return true;
  };
  const add = value => { try { if (reserve(Buffer.byteLength(JSON.stringify(value)))) events.push(value); } catch { error('Observation record could not be serialized'); } };
  const rpc = (channel, direction) => {
    let buffered = Buffer.alloc(0);
    const receive = bytes => {
      if (!channel.phase.startsWith('capability-')) return;
      buffered = Buffer.concat([buffered, bytes]);
      if (buffered.length > 4 * 1024 * 1024) { error('RPC observation exceeded bound'); overflow = true; buffered = Buffer.alloc(0); return; }
      let boundary;
      while ((boundary = buffered.indexOf(10)) >= 0) {
        const text = buffered.subarray(0, boundary).toString('utf8'); buffered = buffered.subarray(boundary + 1);
        try { const message = JSON.parse(text);
          if (message.id === undefined) continue;
          add({ type: 'rpc', channel: channel.id, direction, ...stamp(), id: message.id,
            ...(direction === 'input' ? { method: message.method } : { response: true, hasResult: Object.hasOwn(message, 'result'), error: message.error ?? null }), bytes: Buffer.byteLength(text) });
        } catch { error('Invalid observed RPC line'); }
      }
    };
    receive.pending = () => buffered.length; return receive;
  };
  const frames = (channel, direction) => {
    let buffered = Buffer.alloc(0); const receive = rpc(channel, direction);
    const state = { get pending() { return buffered.length; }, get rpcPending() { return receive.pending(); } }; channel.parsers.push(state);
    return value => {
      if (overflow) { buffered = Buffer.alloc(0); return; }
      if (!Buffer.isBuffer(value) && !(value instanceof Uint8Array)) { error('Nonbinary helper observation'); return; }
      if (!reserve(value.byteLength)) { buffered = Buffer.alloc(0); return; }
      buffered = Buffer.concat([buffered, value]);
      while (buffered.length >= 4) {
        const length = buffered.readUInt32LE();
        if (length < 1 || length > 1024 * 1024) { error('Invalid observed helper frame'); buffered = Buffer.alloc(0); return; }
        if (buffered.length < length + 4) return;
        const kind = String.fromCharCode(buffered[4]), payload = buffered.subarray(5, length + 4); buffered = buffered.subarray(length + 4);
        if (direction === 'output' && ['R', 'F', 'D', 'U', 'X'].includes(kind)) {
          if (payload.length > 4096) { error('Control observation exceeded bound'); overflow = true; return; }
          add({ type: 'native-frame', channel: channel.id, ...stamp(), kind, payloadBase64: payload.toString('base64') });
        }
        if (direction === 'input' && ['L', 'E', 'S', 'W'].includes(kind)) add({ type: 'native-control', channel: channel.id, ...stamp(), kind, bytes: payload.length });
        if (kind === (direction === 'input' ? 'I' : 'O')) receive(payload);
      }
    };
  };
  return {
    argv(call) { if (reserve(Buffer.byteLength(JSON.stringify(call)))) argvCalls.push(call); },
    observeHelper(child, call, environment) {
      try {
        if (overflow) return;
        if (channels.length >= 32) { overflow = true; return; }
        const channel = { id: channels.length + 1, phase: queryPhase(call.argv), call,
          environment: Object.fromEntries(ownerNames.map(key => [key, environment?.[key] ?? null])), parsers: [] };
        channels.push(channel); add({ type: 'helper-start', channel: channel.id, ...stamp() });
        const input = frames(channel, 'input'), output = frames(channel, 'output');
        const read = child.stdout.read, write = child.stdin.write, emit = child.emit;
        child.stdout.read = function (...args) {
          const result = Reflect.apply(read, this, args);
          try { if (result !== null) output(result); } catch (e) { error(e.message); }
          return result;
        };
        child.stdin.write = function (...args) {
          const result = Reflect.apply(write, this, args);
          try { input(args[0]); } catch (e) { error(e.message); }
          return result;
        };
        child.emit = function (event, ...args) {
          const result = Reflect.apply(emit, this, [event, ...args]);
          try {
            if (event === 'close' || event === 'exit') add({ type: 'helper-' + event, channel: channel.id, ...stamp(), code: args[0], signal: args[1] });
            if (event === 'error') error('Helper error: ' + args[0]?.message);
          } catch { error('Helper event observation failed'); }
          return result;
        };
        if (child.stderr?.emit) {
          const stderrEmit = child.stderr.emit;
          child.stderr.emit = function (event, ...args) {
            const result = Reflect.apply(stderrEmit, this, [event, ...args]);
            try { if (event === 'data') add({ type: 'helper-stderr', channel: channel.id, ...stamp(),
              encoding: typeof args[0] === 'string' ? 'utf8-decoded-by-existing-consumer' : 'raw', payloadBase64: Buffer.from(args[0]).toString('base64') }); }
            catch { error('Helper stderr observation failed'); }
            return result;
          };
        }
      } catch (e) { error('Observer installation: ' + e.message); }
    },
    sync(original, operation) { return function (...args) {
      const start = stamp(); let failure;
      try { return Reflect.apply(original, this, args); }
      catch (e) { failure = e; throw e; }
      finally {
        let request;
        try { const encoded = args[2]?.env?.GESTALT_NATIVE_REQUEST;
          if (encoded && encoded.length <= 131072) request = JSON.parse(Buffer.from(encoded, 'base64')).operation;
        } catch { error('Invalid private-operation observation'); }
        try { add({ type: 'sync-command', operation, executable: String(args[0]), privateOperation: request ?? null, ...start,
          completed: stamp(), callsite: new Error().stack, error: failure ? { message: failure.message, status: failure.status ?? null, signal: failure.signal ?? null } : null }); }
        catch { error('Synchronous result observation failed'); }
      }
    }; },
    setTimeout(original) { return function (callback, delay, ...args) {
      const handle = Reflect.apply(original, this, [callback, delay, ...args]);
      try { const id = ++nextTimer, callsite = new Error().stack;
        if (handle && typeof handle === 'object') timers.set(handle, id);
        add({ type: 'timer-created', id, delay, ...stamp(), callsite, callbackName: callback?.name ?? '' });
      } catch { error('Timer observation failed'); }
      return handle;
    }; },
    clearTimeout(original) { return function (handle, ...args) {
      const result = Reflect.apply(original, this, [handle, ...args]);
      const id = handle && typeof handle === 'object' ? timers.get(handle) : undefined;
      if (id) add({ type: 'timer-cleared', id, ...stamp(), provesNonexpiry: false });
      return result;
    }; },
    report() {
      return { schemaVersion: 1, observerPid: process.pid, managerArgv: process.argv, ...stamp(), overflow, dropped, errors, usedBytes: used, maxBytes, maxRecords,
        channels: channels.map(({ parsers, ...channel }) => ({ ...channel, incompleteFrameBytes: parsers.map(parser => parser.pending), incompleteRpcBytes: parsers.map(parser => parser.rpcPending) })),
        events, argvCalls, scope: 'Pass-through diagnostic observation, not invented inner lease claims or an uninstrumented timing proof' };
    },
  };
}

if (process.env.GESTALT_SLICE_CODEX_BINARY && process.env.GESTALT_SLICE_SPAWN_CAPTURE) {
const original = childProcess.spawn;
const readinessFile = process.env.GESTALT_SLICE_READINESS_CAPTURE;
const observer = readinessFile ? createReadinessObserver() : undefined;
childProcess.spawn = function (executable, argv, ...options) {
  const child = Reflect.apply(original, this, [executable, argv, ...options]);
  const target = process.env.GESTALT_SLICE_CODEX_BINARY, file = process.env.GESTALT_SLICE_SPAWN_CAPTURE;
  if (target && file && Array.isArray(argv)) {
    const boundary = argv.indexOf('--'), supervised = /gestalt-owned-process(?:\.exe)?$/i.test(String(executable)) && boundary >= 0;
    const selected = supervised ? argv[boundary + 1] : executable, selectedArgv = supervised ? argv.slice(boundary + 2) : argv;
    if (selected === 'codex' || String(selected).toLowerCase() === target.toLowerCase()) {
      const call = { observedAt: new Date().toISOString(), observer: process.pid, spawnedChildPid: child.pid,
        executable, target: selected, argv: selectedArgv, cwd: options[0]?.cwd ?? process.cwd(),
        transport: supervised ? 'accepted-owned-helper' : 'unchanged-Unix-spawn' };
      try {
        if (observer) { observer.argv(call); if (supervised) observer.observeHelper(child, call, options[0]?.env); }
        else appendFileSync(file, JSON.stringify(call) + '\n'); }
      catch { /* Missing capture fails the test; never lose the real child handle. */ }
    }
  }
  return child;
};
if (observer) {
  childProcess.execFileSync = observer.sync(childProcess.execFileSync, 'execFileSync');
  childProcess.spawnSync = observer.sync(childProcess.spawnSync, 'spawnSync');
  globalThis.setTimeout = observer.setTimeout(globalThis.setTimeout);
  globalThis.clearTimeout = observer.clearTimeout(globalThis.clearTimeout);
  const emit = process.emit; let flushed = false;
  process.emit = function (event, ...args) {
    const result = Reflect.apply(emit, this, [event, ...args]);
    if (event === 'exit' && !flushed) {
      flushed = true;
      try { const report = observer.report(); report.processExit = args[0];
        writeFileSync(readinessFile, JSON.stringify(report));
        writeFileSync(process.env.GESTALT_SLICE_SPAWN_CAPTURE, report.argvCalls.map(call => JSON.stringify(call) + '\n').join(''));
      } catch { /* Missing records are unknown/fail in the parent; results stay original. */ }
    }
    return result;
  };
}
syncBuiltinESMExports();
}
