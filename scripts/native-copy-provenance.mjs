import { createHash } from 'node:crypto';
import { lstat, readFile, readdir, readlink } from 'node:fs/promises';
import path from 'node:path';

/** Inventory the installed toolchain, including nested dist trees and native
 * binaries. Read links as links; do not traverse them into another installation. */
export async function captureCopyTree(root) {
  const entries = [];
  async function visit(directory, prefix = '') {
    for (const name of (await readdir(directory)).sort()) {
      const relative = prefix ? `${prefix}/${name}` : name, file = path.join(directory, name), info = await lstat(file);
      if (info.isSymbolicLink()) entries.push({ path: relative, kind: 'link', target: await readlink(file) });
      else if (info.isDirectory()) { entries.push({ path: relative, kind: 'directory' }); await visit(file, relative); }
      else if (info.isFile()) entries.push({ path: relative, kind: 'file', executable: process.platform === 'win32' ? null : Boolean(info.mode & 0o111),
        bytes: info.size, sha256: createHash('sha256').update(await readFile(file)).digest('hex') });
      else throw new Error(`Unsupported installed dependency entry: ${relative}`);
    }
  }
  await visit(root);
  return entries;
}

/** Preserve every mismatch for evidence, while callers print a bounded cause. */
export function compareCopyTrees(expected, actual) {
  const before = new Map(expected.map(entry => [entry.path, entry])), after = new Map(actual.map(entry => [entry.path, entry]));
  const differences = [];
  for (const name of [...new Set([...before.keys(), ...after.keys()])].sort()) {
    const original = before.get(name), copied = after.get(name);
    if (JSON.stringify(original) !== JSON.stringify(copied)) differences.push({ path: name, expected: original ?? null, actual: copied ?? null });
  }
  return differences;
}
