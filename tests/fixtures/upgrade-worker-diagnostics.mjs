import { execFile } from 'node:child_process';
import { readFile, readdir, realpath, mkdir, writeFile } from 'node:fs/promises';
import { join, basename } from 'node:path';
import { promisify } from 'node:util';
import { randomUUID } from 'node:crypto';
const execute = promisify(execFile);
async function boundedFile(path, maximum = 32768) {
  try { const bytes = await readFile(path); return { bytes: bytes.length, tail: bytes.subarray(-maximum).toString('utf8') }; }
  catch (error) { return { error: error.code ?? error.message }; }
}
async function identity(pid) {
  try {
    const { stdout } = await execute('ps', ['-o', 'pid=,ppid=,pgid=,stat=,comm=', '-p', String(pid)]);
    return { pid, ps: stdout.trim(), live: !/^\s*\d+\s+\d+\s+\d+\s+Z/.test(stdout) };
  } catch (error) { return { pid, live: false, exit: error.code }; }
}
export async function captureWorkerFixture(root, name) {
  const destination = process.env.GESTALT_UPGRADE_DIAGNOSTICS;
  if (!destination) return;
  const snapshot = { name, observedAt: new Date().toISOString(), root, canonicalRoot: await realpath(root),
    operations: await boundedFile(join(root, 'operations')), workerStartup: await boundedFile(join(root, 'operations.worker-startup')), workerLog: await boundedFile(join(root, 'managed/update-restart.log')),
    invocation: await boundedFile(join(root, 'invocation.json')), ownership: await boundedFile(join(root, 'owned-processes.json')), states: [] };
  for (const file of await readdir(join(root, 'managed/run')).catch(() => [])) {
    if (/^mobile-[1-9][0-9]*\.restart$/.test(file)) snapshot.states.push({ file, descriptor: await boundedFile(join(root, 'managed/run', file), 4096) });
    else if (file === 'update-restart.lock') snapshot.lockPresent = true;
  }
  let owned = {};
  try { owned = JSON.parse(await readFile(join(root, 'owned-processes.json'), 'utf8')); } catch {}
  snapshot.processes = [];
  // Only fixture relays and the explicit detached worker PID returned by this
  // invocation are eligible. Verify executable/group before cleanup signals.
  try {
  for (const [kind, pid] of Object.entries(owned)) {
    const before = await identity(pid); let signal = null;
    if (before.live) {
      const fields = before.ps.split(/\s+/), command = basename(fields.slice(4).join(' '));
      const permitted = kind === 'relay' ? command === 'sleep' : Number(fields[2]) === pid && ['bash', 'gestalt'].includes(command);
      if (!permitted) throw Error(`owned fixture process identity changed: ${kind}`);
      signal = 'SIGTERM'; process.kill(kind === 'worker' ? -pid : pid, signal);
      const deadline = Date.now() + 2000;
      while ((await identity(pid)).live && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 20));
      if ((await identity(pid)).live) { signal = 'SIGKILL'; process.kill(kind === 'worker' ? -pid : pid, signal); }
    }
    const deadline = Date.now() + 2000;
    while ((await identity(pid)).live && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 20));
    const after = await identity(pid); snapshot.processes.push({ kind, before, cleanupSignal: signal, after });
    if (after.live) throw Error(`owned fixture process did not settle: ${kind}`);
  }
  } catch (error) { snapshot.cleanupError = { message: error.message, code: error.code ?? null }; }
  await mkdir(destination, { recursive: true });
  await writeFile(join(destination, randomUUID() + '.json'), JSON.stringify(snapshot, null, 2) + '\n', { flag: 'wx' });
  if (snapshot.cleanupError) throw Error(snapshot.cleanupError.message);
}
