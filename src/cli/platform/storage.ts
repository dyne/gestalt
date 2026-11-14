import { constants } from 'node:fs';
import { lstat, mkdir, open, realpath, rename, unlink } from 'node:fs/promises';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import type { Bytes, PrivateStoragePort } from '../contracts.ts';
import { NativePaths, validateNativePath } from './paths.ts';
import { windowsSystem } from './windows-system.ts';

const absent = (error: unknown) => (error as NodeJS.ErrnoException).code === 'ENOENT';
const execute = promisify(execFile);

/** All mutating methods are confined to one prevalidated private directory. */
export class NativePrivateStorage implements PrivateStoragePort {
  readonly root: string;
  private readonly paths = new NativePaths();
  private readonly rootIdentity: { dev: bigint; ino: bigint };
  private readonly unixProbe: string | undefined;
  private constructor(root: string, identity: { dev: bigint; ino: bigint }, unixProbe?: string) {
    this.root = root; this.rootIdentity = identity; this.unixProbe = unixProbe;
  }

  static async open(root: string, unixProbe?: string): Promise<NativePrivateStorage> {
    validateNativePath(root);
    // Resolve parent aliases (e.g. macOS /var) before creation; never follow a root link.
    const destination = path.join(await realpath(path.dirname(path.resolve(root))), path.basename(root));
    if (destination === path.parse(destination).root) throw new Error('A filesystem root cannot be private managed storage');
    try { await lstat(destination); }
    catch (error) {
      if (!absent(error)) throw error;
      if (process.platform === 'win32') await windowsSystem({ operation: 'create-directory', path: destination });
      else await mkdir(destination, { mode: 0o700 });
    }
    const storage = new NativePrivateStorage(destination, await lstat(destination, { bigint: true }), unixProbe);
    await storage.assertPrivate(destination, 'directory');
    return storage;
  }

  private async checked(value: string, allowMissing = false): Promise<string> {
    const destination = path.resolve(value);
    const currentRoot = await lstat(this.root, { bigint: true });
    if (currentRoot.dev !== this.rootIdentity.dev || currentRoot.ino !== this.rootIdentity.ino) throw new Error('Managed root identity changed; refusing redirected operation');
    await this.paths.assertContained(this.root, destination);
    const relative = path.relative(this.root, destination);
    let cursor = this.root;
    // Validate every existing component, including the root on every operation.
    for (const component of ['', ...relative.split(path.sep).filter(Boolean)]) {
      cursor = component ? path.join(cursor, component) : cursor;
      try {
        const entry = await lstat(cursor);
        if (entry.isSymbolicLink()) throw new Error('Redirected storage path refused');
        if (entry.isFile() && entry.nlink !== 1) throw new Error('Hardlinked storage path refused');
        if (process.platform === 'win32') await windowsSystem({ operation: 'assert-private', path: cursor });
        else if (entry.uid !== process.getuid?.() || (entry.mode & 0o077) !== 0) throw new Error('Storage is not private to current user');
        if (process.platform === 'darwin') {
          if (!this.unixProbe || !path.isAbsolute(this.unixProbe)) throw new Error('Target-built native macOS ACL probe is required');
          try { await execute(this.unixProbe, ['--private', cursor], { timeout: 5000, maxBuffer: 16384 }); }
          catch (error) { throw new Error('Native macOS ACL validation failed', { cause: error }); }
        }
      } catch (error) { if (!allowMissing || !absent(error)) throw error; }
    }
    return destination;
  }

  async assertPrivate(value: string, kind: 'file' | 'directory' | 'socket'): Promise<void> {
    const destination = await this.checked(value);
    const entry = await lstat(destination);
    if (kind === 'file' ? !entry.isFile() : kind === 'socket' ? !entry.isSocket() : !entry.isDirectory()) throw new Error(`Expected private ${kind}`);
  }

  async createDirectory(value: string): Promise<void> {
    const destination = await this.checked(value, true);
    try { await this.assertPrivate(destination, 'directory'); return; } catch (error) { if (!absent(error)) throw error; }
    await this.assertPrivate(path.dirname(destination), 'directory');
    try {
      if (process.platform === 'win32') await windowsSystem({ operation: 'create-directory', path: destination });
      else await mkdir(destination, { mode: 0o700 });
    } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error; }
    await this.assertPrivate(destination, 'directory');
  }

  async acquireLock(value: string): Promise<{ release(): Promise<void> }> {
    const destination = await this.checked(value, true);
    await this.assertPrivate(path.dirname(destination), 'directory');
    const handle = await open(destination, constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY, 0o600);
    const identity = await handle.stat({ bigint: true });
    let released = false;
    try {
      await handle.writeFile(JSON.stringify({ version: 1, token: randomUUID() }));
      await handle.sync();
      await this.assertPrivate(destination, 'file');
    } catch (error) { await handle.close(); throw error; }
    return { release: async () => {
      if (released) return;
      // Never unlink a replaced lock or infer a stale lock from a PID/deadline.
      const current = await lstat(await this.checked(destination), { bigint: true });
      if (current.dev !== identity.dev || current.ino !== identity.ino) throw new Error('Lock identity changed; retained');
      await unlink(destination);
      await handle.close();
      released = true;
    } };
  }

  async replaceFile(value: string, bytes: Bytes): Promise<void> {
    const destination = await this.checked(value, true);
    if (destination === this.root) throw new Error('Cannot replace storage root');
    await this.assertPrivate(path.dirname(destination), 'directory');
    const lock = await this.acquireLock(`${destination}.lock`);
    const stage = path.join(path.dirname(destination), `.${path.basename(destination)}.${randomUUID()}.stage`);
    let committed = false;
    try {
      // Revalidate after acquiring the cooperative lock. Arbitrary same-user
      // mutations are outside this boundary; untrusted principals cannot enter root.
      await this.checked(destination, true);
      try { await this.assertPrivate(destination, 'file'); } catch (error) { if (!absent(error)) throw error; }
      const handle = await open(stage, constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY, 0o600);
      try { await handle.writeFile(bytes); await handle.sync(); } finally { await handle.close(); }
      await this.assertPrivate(stage, 'file');
      await this.checked(destination, true);
      if (process.platform === 'win32') await windowsSystem({ operation: 'replace', source: stage, path: destination });
      else await rename(stage, destination);
      committed = true;
      if (process.platform !== 'win32') {
        const directory = await open(path.dirname(destination), constants.O_RDONLY);
        try { await directory.sync(); } finally { await directory.close(); }
      }
    } finally {
      if (!committed) { try { await unlink(stage); } catch (error) { if (!absent(error)) throw error; } }
      await lock.release();
    }
  }
}
