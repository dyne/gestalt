import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { lstat, mkdir, open, readFile, rmdir, stat, unlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { mobileHostSupport } from './mobile.ts';
import { CommandFailure, optionalStat, SetupContext } from './context.ts';
import { readProfile } from './profile-files.ts';

export function retiredRestartArguments(args: readonly string[]): string[] {
  const result: string[] = [];
  for (let index = 0; index < args.length; index++) {
    const token = args[index]!;
    if (token === '--') return [...result, ...args.slice(index)];
    if (token !== '--xerj') { result.push(token); continue; }
    if (!['auto', 'manual', 'off'].includes(args[index + 1] ?? ''))
      throw new Error('legacy --xerj restart option is missing or ambiguous; review the restart descriptor before updating');
    index++;
  }
  return result;
}
function live(pid: number): boolean {
  try { process.kill(pid, 0); return true; }
  catch (error) { if ((error as NodeJS.ErrnoException).code === 'ESRCH') return false; throw error; }
}
async function relayContext(context: SetupContext, selectedPid: string, descriptor: string): Promise<number> {
  if (!/^[1-9]\d*$/.test(selectedPid) || !Number.isSafeInteger(Number(selectedPid)))
    throw new Error('update-restart must be run from a session managed by Gestalt Mobile');
  const pid = Number(selectedPid);
  if (descriptor !== path.join(context.paths.gestaltHome, 'run', `mobile-${pid}.restart`) || !(await readProfile(descriptor)))
    throw new Error('the Gestalt Mobile restart descriptor is unavailable or invalid');
  if (!live(pid)) throw new Error('the managing Gestalt Mobile process is not running');
  return pid;
}

export async function scheduleRestart(context: SetupContext, component: string, extra: boolean): Promise<void> {
  const support = mobileHostSupport();
  if (!support.supported) throw new Error(`Mobile restart unavailable: ${support.prerequisite}`);
  const relay = context.environment.GESTALT_MOBILE_PID || '', descriptor = context.environment.GESTALT_MOBILE_RESTART_STATE || '';
  await relayContext(context, relay, descriptor);
  const storage = await context.storage(context.paths.gestaltHome);
  const run = path.join(storage.root, 'run');
  await storage.assertPrivate(run, 'directory');
  const lock = path.join(run, 'update-restart.lock'), log = path.join(storage.root, 'update-restart.log');
  try { await mkdir(lock, { mode: 0o700 }); }
  catch (error) { if ((error as NodeJS.ErrnoException).code === 'EEXIST') throw new Error(`another update-restart is already running (${lock})`); throw error; }
  const identity = await lstat(lock, { bigint: true });
  const token = randomUUID();
  try {
    await storage.assertPrivate(lock, 'directory');
    await storage.replaceFile(path.join(lock, 'owner.json'), Buffer.from(JSON.stringify({ schemaVersion: 1, token, dev: String(identity.dev), ino: String(identity.ino) })));
    const child = spawn(process.execPath, [context.paths.manager, '__update-restart-worker', relay, descriptor, String(extra), log, lock, component, token],
      { detached: true, env: context.environment, stdio: 'ignore', windowsHide: true, shell: false });
    await new Promise<void>((resolve, reject) => { child.once('error', reject); child.once('spawn', resolve); });
    child.unref();
    await context.log(`scheduled update and Mobile restart (worker ${child.pid})`);
    await context.log(`progress will be written to ${log}; this session will reconnect after restart`);
  } catch (error) {
    await unlink(path.join(lock, 'owner.json')).catch(() => {}); await rmdir(lock);
    throw error;
  }
}

export async function restartWorker(context: SetupContext, args: readonly string[],
  update: (worker: SetupContext, component: string, extra: boolean) => Promise<boolean>): Promise<number> {
  const [relay = '', descriptor = '', extra = '', log = '', lock = '', component = '', token = ''] = args;
  if (args.length !== 7 || !['true', 'false'].includes(extra) || log !== path.join(context.paths.gestaltHome, 'update-restart.log') ||
      lock !== path.join(context.paths.gestaltHome, 'run/update-restart.lock')) throw new Error('the update-restart worker received an invalid context');
  const storage = await context.storage(context.paths.gestaltHome);
  const checkLock = async () => {
    await storage.assertPrivate(lock, 'directory');
    const identity = await lstat(lock, { bigint: true });
    const record = JSON.parse(await readFile(path.join(lock, 'owner.json'), 'utf8')) as Record<string, unknown>;
    if (record.schemaVersion !== 1 || record.token !== token || record.dev !== String(identity.dev) || record.ino !== String(identity.ino))
      throw new Error('the update-restart worker lock identity changed; retained');
  };
  await checkLock();
  const existingLog = await readProfile(log);
  if (existingLog) await storage.assertPrivate(log, 'file');
  const handle = await open(log, 'a', 0o600);
  let worker: SetupContext | null = null, retainLock = false;
  try {
    const write = async (bytes: Uint8Array) => { await handle.write(bytes); };
    worker = new SetupContext(context.invocation, { ...context.streams, stdout: write, stderr: write }, context.paths, context.version);
    await worker.log(`starting live update at ${new Date().toISOString()}`);
    // Preserve the endpoint's chance to flush its accepted response.
    await delay(1000);
    const pid = await relayContext(worker, relay, descriptor);
    const bytes = await readFile(descriptor);
    if (!bytes.length || bytes.at(-1) !== 0) throw new Error('the Gestalt Mobile restart descriptor is empty or incomplete');
    const record = bytes.toString('utf8').slice(0, -1).split('\0');
    const cwd = record.shift()!;
    const argv = retiredRestartArguments(record);
    if (!path.isAbsolute(cwd) || !(await stat(cwd)).isDirectory()) throw new Error(`the Gestalt Mobile restart directory is invalid: ${cwd}`);
    if (await update(worker, component, extra === 'true')) return 0;
    await relayContext(worker, relay, descriptor);
    if (!(await readFile(descriptor)).equals(bytes)) throw new Error('the Gestalt Mobile restart descriptor changed during update; relay preserved');
    await worker.log(`update succeeded; gracefully stopping Gestalt Mobile process ${pid}`);
    process.kill(pid, 'SIGTERM');
    for (let attempt = 0; attempt < 300 && live(pid); attempt++) await delay(100);
    if (live(pid)) throw new Error('Gestalt Mobile did not stop within 30 seconds; it was not force-killed');
    await unlink(descriptor);
    await checkLock(); await unlink(path.join(lock, 'owner.json')); await rmdir(lock);
    await worker.log('restarting Gestalt Mobile with its previous options');
    await worker.run(process.execPath, [context.paths.manager, 'mobile', '--', ...argv], { cwd });
    return 0;
  } catch (error) {
    retainLock = worker !== null && !worker.canRollback();
    await handle.write(`gestalt: ${error instanceof Error ? error.message : error}\n`);
    return error instanceof CommandFailure ? error.status : 1;
  } finally {
    await handle.close();
    if (!retainLock && await optionalStat(lock)) { await checkLock(); await unlink(path.join(lock, 'owner.json')); await rmdir(lock); }
  }
}
