import { lstat, open, readFile, rmdir, stat, unlink } from 'node:fs/promises';
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
async function relayContext(context: SetupContext, selectedPid: string, descriptor: string): Promise<number> {
  if (!/^[1-9]\d*$/.test(selectedPid) || !Number.isSafeInteger(Number(selectedPid)))
    throw new Error('update-restart must be run from a session managed by Gestalt Mobile');
  const pid = Number(selectedPid);
  if (descriptor !== path.join(context.paths.gestaltHome, 'run', `mobile-${pid}.restart`) || !(await readProfile(descriptor)))
    throw new Error('the Gestalt Mobile restart descriptor is unavailable or invalid');
  return pid;
}

export async function scheduleRestart(context: SetupContext, component: string, extra: boolean): Promise<void> {
  const support = mobileHostSupport();
  if (!support.supported) throw new Error(`Mobile restart unavailable: ${support.prerequisite}`);
  const relay = context.environment.GESTALT_MOBILE_PID || '', descriptor = context.environment.GESTALT_MOBILE_RESTART_STATE || '';
  await relayContext(context, relay, descriptor);
  // A descriptor/PID is context, never restart authority. The Mobile native
  // host milestone supplies the live authenticated handoff. Do not detach an
  // unowned host worker or move its filesystem owner into a user namespace.
  throw new Error('native Mobile restart requires an authenticated live host handoff with owned command lifetimes; the running relay and descriptor are preserved');
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
    worker = await context.withStreams({ ...context.streams, stdout: write, stderr: write });
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
    await worker.log(`update succeeded; native restart authority for the configured relay ${pid} is unavailable; the running relay and descriptor are preserved`);
    throw new Error('native Mobile restart requires an authenticated live host handoff; no PID or liveness value grants termination authority');
  } catch (error) {
    retainLock = worker !== null && !worker.canRollback();
    await handle.write(`gestalt: ${error instanceof Error ? error.message : error}\n`);
    return error instanceof CommandFailure ? error.status : 1;
  } finally {
    await handle.close();
    if (!retainLock && await optionalStat(lock)) { await checkLock(); await unlink(path.join(lock, 'owner.json')); await rmdir(lock); }
  }
}
