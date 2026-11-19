// Test transport only. Record dispatch and actual results without changing
// arguments, environment, stdio, callbacks, timeouts or command results.
import childProcess from 'node:child_process';
import { createHash } from 'node:crypto';
import { appendFileSync } from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
import path from 'node:path';
import { promisify } from 'node:util';

const log = process.env.FINAL_WINDOWS_PHASE_LOG;
const digest = value => value == null ? null : createHash('sha256').update(value).digest('hex');
let next = 0;
function record(event, fields = {}) {
  if (!log) return;
  try {
    appendFileSync(log, JSON.stringify({ observedAt: new Date().toISOString(), monotonicMs: performance.now(),
      pid: process.pid, event, ...fields }) + '\n');
  } catch (error) { console.error('Transport receipt write failed:', error.code ?? error.name); }
}
function describe(file, argv = [], options) {
  const executable = typeof file === 'string' ? path.basename(file) : '(unavailable)';
  if (!Array.isArray(argv)) { options = argv; argv = []; }
  if (argv[0] === '--storage-check-v1')
    return { executable, nativeRead: argv[1], pathCount: argv.length - 3 };
  const env = options?.env;
  const encoded = env?.GESTALT_NATIVE_REQUEST ?? env?.GESTALT_STORAGE_PARITY_REQUEST;
  if (executable.toLowerCase() === 'powershell.exe' && typeof encoded === 'string' && encoded.length <= 131072) {
    try {
      const request = JSON.parse(Buffer.from(encoded, 'base64').toString('utf8'));
      return { executable, adapter: request.operation, fixtureKind: request.kind ?? null };
    } catch { return { executable, adapter: 'unparsed' }; }
  }
  // Command classes only. Never log passed environments or arbitrary argv.
  return { executable, entry: path.basename(typeof argv[0] === 'string' ? argv[0] : '').slice(0, 80),
    verbs: argv.slice(1, 4).filter(arg => /^(--version|--help|agents|install|update|gestalt|skills|doctor|status|--check)$/.test(arg)) };
}
function started(api, args) {
  const id = ++next;
  record('command-dispatched', { id, api, ...describe(...args) });
  return id;
}
function completed(id, result) {
  record('command-observed', {
    id, status: result.status ?? result.code ?? null, signal: result.signal ?? null,
    error: typeof result.code === 'string' ? result.code : result.error?.code ?? null,
    stdoutSha256: digest(result.stdout), stderrSha256: digest(result.stderr),
  });
}
const installKey = Symbol.for('gestalt.final-current.phase-observer');
if (log && !globalThis[installKey]) {
  globalThis[installKey] = true;
  record('node-entry', { script: path.basename(process.argv[1] ?? '') });
  for (const name of ['spawnSync', 'execFileSync']) {
    const original = childProcess[name];
    childProcess[name] = function (...args) {
      const id = started(name, args);
      try {
        const result = Reflect.apply(original, this, args);
        completed(id, name === 'spawnSync' ? result : { status: 0, stdout: result });
        return result;
      } catch (error) { completed(id, error); throw error; }
    };
  }
  const spawn = childProcess.spawn;
  childProcess.spawn = function (...args) {
    const id = started('spawn', args);
    const child = Reflect.apply(spawn, this, args);
    child.once('close', (code, signal) => completed(id, { code, signal }));
    return child;
  };
  const execFile = childProcess.execFile;
  const execute = promisify(execFile);
  function observedExecFile(...args) { return Reflect.apply(execFile, this, args); }
  Object.defineProperty(observedExecFile, promisify.custom, { value: function (...args) {
    const id = started('execFile', args);
    const result = Reflect.apply(execute, this, args);
    void result.then(reply => completed(id, { status: 0, ...reply }), error => completed(id, error));
    return result;
  } });
  childProcess.execFile = observedExecFile;
  syncBuiltinESMExports();
}
