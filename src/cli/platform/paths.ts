import { lstat, realpath, stat } from 'node:fs/promises';
import path from 'node:path';
import type { PathPort } from '../contracts.ts';

export function validateNativePath(value: string): void {
  if (!value || value.includes('\0')) throw new Error('Invalid native path');
  if (process.platform === 'win32') {
    // Permit ordinary drive/UNC paths, including extended paths from realpath.
    const plain = value.replace(/^\\\\\?\\UNC\\/i, '\\\\').replace(/^\\\\\?\\/, '');
    if (/^\\\\[.?]\\/.test(plain) || /(^|[\\/])[^\\/]*[:<>"|?*]/.test(plain.replace(/^[a-z]:/i, '')) ||
        plain.split(/[\\/]/).some(part => /[ .]$/.test(part) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(part)))
      throw new Error('Ambiguous or device Windows path');
    if (!path.isAbsolute(plain)) throw new Error('Windows paths must be absolute');
  }
}

async function canonicalWithMissingLeaf(value: string): Promise<string> {
  validateNativePath(value);
  let cursor = path.resolve(value);
  const suffix: string[] = [];
  for (;;) {
    try { return path.join(await realpath(cursor), ...suffix); }
    catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
      // A dangling link must not be mistaken for an absent ordinary component.
      try { await lstat(cursor); throw new Error('Dangling path redirect'); }
      catch (entryError) { if ((entryError as NodeJS.ErrnoException).code !== 'ENOENT') throw entryError; }
      const parent = path.dirname(cursor);
      if (parent === cursor) throw error;
      suffix.unshift(path.basename(cursor));
      cursor = parent;
    }
  }
}

/** Canonical ancestry uses file identities, not a case-folded string prefix. */
export class NativePaths implements PathPort {
  resolve(...parts: string[]): string { return path.resolve(...parts); }
  join(...parts: string[]): string { return path.join(...parts); }
  canonical(value: string): Promise<string> { return canonicalWithMissingLeaf(value); }
  async assertContained(root: string, candidate: string): Promise<void> {
    validateNativePath(root); validateNativePath(candidate);
    const lexical = path.relative(path.resolve(root), path.resolve(candidate));
    if (lexical === '..' || lexical.startsWith(`..${path.sep}`) || path.isAbsolute(lexical))
      throw new Error('Path is outside managed root');
    const rootIdentity = await stat(await realpath(root), { bigint: true });
    if (rootIdentity.ino === 0n) throw new Error('Filesystem identity is unavailable; containment is unverified');
    let cursor = await canonicalWithMissingLeaf(candidate);
    for (;;) {
      try {
        const identity = await stat(cursor, { bigint: true });
        if (identity.dev === rootIdentity.dev && identity.ino === rootIdentity.ino) return;
      } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
      const parent = path.dirname(cursor);
      if (parent === cursor) throw new Error('Canonical path is outside managed root');
      cursor = parent;
    }
  }
}
