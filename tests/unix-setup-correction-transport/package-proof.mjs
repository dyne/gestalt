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

export async function inspectPackedPackage(tarball, npmCli, directory, { platform = process.platform } = {}) {
  const tar = createRequire(npmCli)('tar');
  const entries = new Map();
  let failure;
  await tar.t({
    file: tarball,
    strict: true,
    onReadEntry(entry) {
      try {
        assert.match(entry.path, /^package\//);
        const name = entry.path.slice('package/'.length);
        assert.ok(Object.hasOwn(layout, name) && !entries.has(name), name);
        assert.equal(entry.type, 'File', name);
        const mode = entry.mode & 0o7777;
        // npm's Windows verification tar reports 0644 even for the bin.
        // The POSIX bin requirement still applies to Unix publication packs.
        assert.ok(layout[name].includes(mode) || (platform === 'win32' && name === 'bin/gestalt' && mode === 0o644),
          name + ': ' + mode.toString(8));
        assert.ok(entry.size > 0 && entry.size <= 4 * 1024 * 1024, name);
        const chunks = [];
        const info = { name, mode: mode.toString(8), size: entry.size };
        entries.set(name, { info, chunks });
        entry.on('data', chunk => chunks.push(chunk));
        entry.on('error', error => { failure ??= error; });
      } catch (error) {
        // tar emits entries outside the caller's promise stack. Report the
        // validation error after the parser drains the archive.
        failure ??= error;
      }
    },
  });
  if (failure) throw failure;
  assert.deepEqual([...entries.keys()].sort(), Object.keys(layout).sort());
  const verified = [];
  for (const { info, chunks } of entries.values()) {
    const bytes = Buffer.concat(chunks);
    assert.equal(bytes.length, info.size);
    assert.deepEqual(bytes, await readFile(path.join(directory, info.name)), info.name);
    verified.push({ ...info, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  return verified.sort((a, b) => a.name.localeCompare(b.name));
}
