import { lstat } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import path from 'node:path';
import { windowsSystem } from './windows-system.ts';

const execute = promisify(execFile);

/** Refuse namespaces in which another unprivileged principal can replace an
 * ancestor entry. This is a prerequisite for path-based operations, not an
 * assertion that repeated inode checks themselves make those operations atomic.
 */
export async function assertStableParents(root: string, unixProbe?: string): Promise<void> {
  const parents: string[] = [];
  for (let current = path.dirname(root);; current = path.dirname(current)) {
    parents.push(current);
    if (path.dirname(current) === current) break;
  }
  if (process.platform === 'win32') {
    await windowsSystem({ operation: 'assert-namespace', paths: parents.map(value => path.toNamespacedPath(value)) });
    return;
  }
  const systemOwner = (await lstat(path.parse(root).root)).uid;
  for (const current of parents) {
    const entry = await lstat(current);
    const trustedOwner = entry.uid === process.getuid?.() || entry.uid === systemOwner;
    if (!entry.isDirectory() || !trustedOwner || ((entry.mode & 0o022) !== 0 && (entry.mode & 0o1000) === 0))
      throw new Error(`Untrusted namespace parent can redirect managed storage: ${current}`);
    if (process.platform === 'darwin') {
      if (!unixProbe || !path.isAbsolute(unixProbe)) throw new Error('Target-built native macOS namespace ACL probe is required');
      try { await execute(unixProbe, ['--namespace', current], { timeout: 5000, maxBuffer: 16384 }); }
      catch (error) { throw new Error(`Native macOS namespace ACL validation failed: ${current}`, { cause: error }); }
    }
  }
}
