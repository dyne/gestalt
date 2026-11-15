import { createHash, randomUUID } from 'node:crypto';
import { chmod, lstat, mkdir, open, readFile, rename, rmdir, unlink } from 'node:fs/promises';
import path from 'node:path';
import type { NativePrivateStorage } from '../platform/storage.ts';
import { windowsSystem } from '../platform/windows-system.ts';
import { optionalStat } from './context.ts';

export const digest = (bytes: Uint8Array): string => createHash('sha256').update(bytes).digest('hex');
export interface ProfileSnapshot { readonly bytes: Buffer; readonly mode: number; readonly dev: number; readonly ino: number }
export async function readProfile(file: string): Promise<ProfileSnapshot | null> {
  const info = await optionalStat(file);
  if (!info) return null;
  if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1 || (process.getuid && info.uid !== process.getuid()))
    throw new Error(`Codex configuration must be an owned regular file without links: ${file}`);
  return { bytes: await readFile(file), mode: info.mode & 0o777, dev: info.dev, ino: info.ino };
}
/** Config files can retain an operator's existing mode inside a private profile.
 * Keep that compatibility separate from the adapter's strictly private files. */
export async function replaceProfile(storage: NativePrivateStorage, file: string, bytes: Uint8Array,
  expected: ProfileSnapshot | null): Promise<void> {
  await storage.assertPrivate(path.dirname(file), 'directory');
  const current = await readProfile(file);
  if (Boolean(current) !== Boolean(expected) || (current && expected &&
      (current.dev !== expected.dev || current.ino !== expected.ino || !current.bytes.equals(expected.bytes))))
    throw new Error('configuration changed during setup; previous snapshot retained for inspection');
  if (current?.bytes.equals(bytes)) return;
  const temporary = path.join(path.dirname(file), `.gestalt-profile-${randomUUID()}`);
  try {
    const handle = await open(temporary, 'wx', 0o600);
    try { await handle.writeFile(bytes); await handle.sync(); } finally { await handle.close(); }
    await storage.assertPrivate(temporary, 'file');
    if (process.platform === 'win32') await windowsSystem({ operation: 'replace', source: temporary, path: file });
    else {
      if (current) await chmod(temporary, current.mode);
      await rename(temporary, file);
      const parent = await open(path.dirname(file), 'r');
      try { await parent.sync(); } finally { await parent.close(); }
    }
  } finally { await unlink(temporary).catch(error => { if (error.code !== 'ENOENT') throw error; }); }
}

/** Interoperate with the delegated owner's directory lock, without adopting
 * stale claims or holding its lock across delegation. */
export async function acquireSetupGate(storage: NativePrivateStorage): Promise<() => Promise<void>> {
  const directory = path.join(storage.root, '.gestalt-setup.lock');
  try { await mkdir(directory, { mode: 0o700 }); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'EEXIST')
      throw new Error(`Agents setup is already running or ownership is unknown: ${directory}`);
    throw error;
  }
  const identity = await lstat(directory, { bigint: true });
  const token = randomUUID();
  const owner = path.join(directory, `owner-${process.pid}-${token}.json`);
  try {
    await storage.assertPrivate(directory, 'directory');
    await storage.replaceFile(owner, Buffer.from(JSON.stringify({ schemaVersion: 1, pid: process.pid, token }) + '\n'));
  } catch (error) { await rmdir(directory).catch(() => {}); throw error; }
  return async () => {
    await storage.assertPrivate(directory, 'directory');
    const current = await lstat(directory, { bigint: true });
    if (current.dev !== identity.dev || current.ino !== identity.ino) throw new Error('setup lock identity changed; retained');
    const record = JSON.parse(await readFile(owner, 'utf8')) as { token?: string };
    if (record.token !== token) throw new Error('setup lock owner changed; retained');
    await unlink(owner); await rmdir(directory);
  };
}
