// Private diagnostic observations only: no data listeners, reads, resync,
// buffering substitution, PID discovery or changed native ownership decisions.
import childProcess from 'node:child_process';
import { syncBuiltinESMExports } from 'node:module';
import { closeSync, fsyncSync, openSync, writeSync } from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { readFile, readdir } from 'node:fs/promises';

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

// Native test observation only. These identities never authorize product
// signalling, stale-lock recovery or complete-tree release.
export async function until(check, label, milliseconds = 15000) {
  const deadline = Date.now() + milliseconds;
  while (!await check()) {
    if (Date.now() >= deadline) throw new Error(label);
    await new Promise(resolve => setTimeout(resolve, 25));
  }
}
export async function linuxIdentities(environmentEntry) {
  const records = [];
  for (const pid of (await readdir('/proc')).filter(name => /^\d+$/.test(name))) {
    try {
      if (!(await readFile(`/proc/${pid}/environ`, 'utf8')).split('\0').includes(environmentEntry)) continue;
      const text = await readFile(`/proc/${pid}/stat`, 'utf8'); records.push({ pid: Number(pid), started: text.slice(text.lastIndexOf(')') + 2).split(' ')[19] });
    } catch (error) { if (!['ENOENT', 'ESRCH', 'EACCES', 'EPERM'].includes(error.code)) throw error; }
  }
  return records;
}
export async function assertLinuxAbsent(records) {
  await until(async () => {
    for (const record of records) {
      try { const text = await readFile(`/proc/${record.pid}/stat`, 'utf8'); if (text.slice(text.lastIndexOf(')') + 2).split(' ')[19] === record.started) return false; }
      catch (error) { if (!['ENOENT', 'ESRCH'].includes(error.code)) throw error; }
    }
    return true;
  }, 'captured native process creation identities remain after cleanup');
}
export async function windowsObserver(pid) {
  const script = "$ErrorActionPreference='Stop';$p=[Diagnostics.Process]::GetProcessById([int]$env:GESTALT_TEST_OBSERVE_PID);[void]$p.Handle;[Console]::Out.WriteLine('held:'+ $p.Id + ':' + $p.StartTime.ToUniversalTime().Ticks);[Console]::In.ReadLine()|Out-Null;if(-not $p.WaitForExit(15000)){throw 'Captured native descendant remains alive'};[Console]::Out.WriteLine('absent')";
  const child = spawn(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'), ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')],
    { env: { ...process.env, GESTALT_TEST_OBSERVE_PID: String(pid) }, shell: false, stdio: ['pipe', 'pipe', 'pipe'] });
  let stdout = '', stderr = '', exit;
  const completion = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', status => { exit = status; resolve(status); }); });
  void completion.catch(() => {}); child.stdin.on('error', () => {});
  child.stdout.on('data', bytes => stdout += bytes); child.stderr.on('data', bytes => stderr += bytes);
  try { await until(() => { if (exit !== undefined) throw new Error(stderr); return /held:\d+:\d+/.test(stdout); }, 'native observer did not capture a process handle'); }
  catch (error) { if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL'); await completion.catch(() => {}); throw error; }
  let verified = false;
  return { identity: stdout.match(/held:(\d+):(\d+)/).slice(1), async verify() {
    if (verified) return; child.stdin.end('\n'); assert.equal(await completion, 0, stderr); assert.match(stdout, /absent/); verified = true;
  } };
}
