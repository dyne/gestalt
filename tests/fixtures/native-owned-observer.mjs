// Private diagnostic observations only: no data listeners, reads, resync,
// buffering substitution, PID discovery or changed native ownership decisions.
import childProcess from 'node:child_process';
import { syncBuiltinESMExports } from 'node:module';
import { closeSync, fsyncSync, openSync, writeSync } from 'node:fs';
import path from 'node:path';

export function journal(file, phase, detail = {}) {
  const fd = openSync(file, 'a', 0o600);
  try { writeSync(fd, JSON.stringify({ observedAt: new Date().toISOString(), pid: process.pid, phase, ...detail }) + '\n'); fsyncSync(fd); }
  finally { closeSync(fd); }
}
export function resources() {
  return { activeResources: process.getActiveResourcesInfo(), stdin: { tty: Boolean(process.stdin.isTTY), raw: Boolean(process.stdin.isRaw), paused: process.stdin.isPaused(), flowing: process.stdin.readableFlowing,
    readableListeners: process.stdin.listenerCount('readable'), dataListeners: process.stdin.listenerCount('data') },
    handles: process._getActiveHandles().map(h => ({ type: h.constructor?.name, fd: h.fd, hasRef: typeof h.hasRef === 'function' ? h.hasRef() : null,
      nativeHasRef: typeof h._handle?.hasRef === 'function' ? h._handle.hasRef() : null, readable: h.readable, writable: h.writable, destroyed: h.destroyed })) };
}
export function observeOwnedHelpers(helper, directory, prefix) {
  const original = childProcess.spawn, traces = [];
  childProcess.spawn = function (file, argv, options) {
    const child = Reflect.apply(original, childProcess, arguments);
    if (path.resolve(String(file)) !== path.resolve(helper)) return child;
    const id = traces.length, trace = { id, argv, cwd: options?.cwd, stdout: path.join(directory, `${prefix}-${id}.stdout.bin`), stdin: path.join(directory, `${prefix}-${id}.stdin.bin`), stderr: path.join(directory, `${prefix}-${id}.stderr.bin`), bytes: 0, errors: [] }; traces.push(trace);
    const copy = (destination, bytes) => {
      if (!bytes) return;
      try {
        const data = Buffer.isBuffer(bytes) ? bytes : Buffer.from(bytes); trace.bytes += data.length;
        if (trace.bytes > 2 * 1024 * 1024) { trace.errors.push('trace limit'); return; }
        const fd = openSync(destination, 'a', 0o600); try { writeSync(fd, data); fsyncSync(fd); } finally { closeSync(fd); }
      } catch (error) { trace.errors.push(error.message); }
    };
    // Observe push without starting/pausing a reader or adding a data listener.
    for (const [stream, destination] of [[child.stdout, trace.stdout], [child.stderr, trace.stderr]]) {
      const push = stream.push; stream.push = function (bytes, encoding) { copy(destination, bytes); return Reflect.apply(push, this, arguments); };
    }
    const write = child.stdin.write; child.stdin.write = function (bytes) { copy(trace.stdin, bytes); return Reflect.apply(write, this, arguments); };
    child.on('exit', (code, signal) => { trace.exit = { code, signal }; });
    child.on('close', (code, signal) => { trace.close = { code, signal }; });
    return child;
  };
  syncBuiltinESMExports();
  return { traces, restore() { childProcess.spawn = original; syncBuiltinESMExports(); } };
}
