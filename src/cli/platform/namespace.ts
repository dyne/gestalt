import { lstat } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import path from 'node:path';
import { windowsSystem, type WindowsReadHelper } from './windows-system.ts';

const execute = promisify(execFile);

/** Refuse namespaces in which another unprivileged principal can replace an
 * ancestor entry. This is a prerequisite for path-based operations, not an
 * assertion that repeated inode checks themselves make those operations atomic.
 */
export async function assertStableParents(root: string, unixProbe?: string, windowsReadHelper?: WindowsReadHelper): Promise<void> {
  const parents: string[] = [];
  for (let current = path.dirname(root);; current = path.dirname(current)) {
    parents.push(current);
    if (path.dirname(current) === current) break;
  }
  if (process.platform === 'win32') {
    await windowsSystem({ operation: 'assert-namespace', paths: parents.map(value => path.toNamespacedPath(value)) }, windowsReadHelper);
    return;
  }
  for (const current of parents) {
    const entry = await lstat(current);
    // A mapped filesystem root owner is not evidence of system authority.
    // UID 65534 may represent unrelated unmapped principals; refuse it even
    // when the caller itself has an unverifiable overflow identity.
    const trustedOwner = entry.uid !== 65534 && (entry.uid === 0 || entry.uid === process.getuid?.());
    if (!entry.isDirectory() || !trustedOwner || ((entry.mode & 0o022) !== 0 && (entry.mode & 0o1000) === 0))
      throw new Error(`Untrusted namespace parent can redirect managed storage: ${current}`);
    if (process.platform === 'darwin') {
      if (!unixProbe || !path.isAbsolute(unixProbe)) throw new Error('Target-built native macOS namespace ACL probe is required');
      try { await execute(unixProbe, ['--namespace', current], { timeout: 5000, maxBuffer: 16384 }); }
      catch (error) { throw new Error(`Native macOS namespace ACL validation failed: ${current}`, { cause: error }); }
    }
  }
}
