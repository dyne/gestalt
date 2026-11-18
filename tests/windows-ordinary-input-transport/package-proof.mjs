import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';

const layout = {
  'bin/gestalt': [0o755],
  'lib/gestalt.mjs': [0o644, 0o755],
  'lib/legacy/gestalt': [0o644, 0o755],
  'lib/native/gestalt-owned-process.exe': [0o644],
  'lib/native/gestalt-owned-process.exe.provenance.json': [0o644],
  'lib/native/LICENSE.txt': [0o644],
  'LICENSE': [0o644],
  'package.json': [0o644],
  'postinstall.mjs': [0o644],
  'README.md': [0o644],
};

export async function inspectPackedPackage(tarball, npmCli, directory) {
  const tar = createRequire(npmCli)('tar');
  const entries = new Map();
  const reads = [];
  await tar.t({
    file: tarball,
    strict: true,
    onReadEntry(entry) {
      assert.match(entry.path, /^package\//);
      const name = entry.path.slice('package/'.length);
      assert.ok(Object.hasOwn(layout, name) && !entries.has(name), name);
      assert.equal(entry.type, 'File', name);
      // The Windows pack is verification only. Windows chmod does not supply
      // POSIX execute bits to non-bin files as the Linux publish build does.
      assert.ok(layout[name].includes(entry.mode & 0o7777), name + ': ' + entry.mode.toString(8));
      assert.ok(entry.size > 0 && entry.size <= 4 * 1024 * 1024, name);
      const chunks = [];
      const info = { name, mode: (entry.mode & 0o7777).toString(8), size: entry.size };
      entries.set(name, info);
      reads.push(new Promise((resolve, reject) => {
        entry.on('data', chunk => chunks.push(chunk));
        entry.on('end', () => resolve([info, Buffer.concat(chunks)]));
        entry.on('error', reject);
      }));
    },
  });
  assert.deepEqual([...entries.keys()].sort(), Object.keys(layout).sort());
  for (const [info, bytes] of await Promise.all(reads)) {
    assert.equal(bytes.length, info.size);
    assert.deepEqual(bytes, await readFile(path.join(directory, info.name)), info.name);
    info.sha256 = createHash('sha256').update(bytes).digest('hex');
  }
  return [...entries.values()].sort((a, b) => a.name.localeCompare(b.name));
}
