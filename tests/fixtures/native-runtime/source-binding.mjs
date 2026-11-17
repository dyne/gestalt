import assert from 'node:assert/strict';
import { readFile, lstat } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import path from 'node:path';

export async function verifyOwnershipBuildInputs(provenance, root) {
  const expected = ['src/cli/platform/native/owned-process.c', ...(provenance.platform === 'win32' ?
    ['src/cli/platform/native/windows-storage-check.c', 'src/cli/platform/native/windows-storage-check.h'] : [])];
  if (provenance.platform === 'win32') assert.deepEqual(provenance.sourceInputs?.map(record => record.path), expected, 'Every native Windows translation/header input must be bound');
  const inputs = [];
  for (const [index, relative] of expected.entries()) {
    const file = path.join(root, relative), bytes = await readFile(file), info = await lstat(file), sha256 = createHash('sha256').update(bytes).digest('hex');
    assert.ok(info.isFile() && !info.isSymbolicLink());
    if (!index) assert.equal(sha256, provenance.sourceSha256);
    if (provenance.platform === 'win32') { assert.equal(bytes.length, provenance.sourceInputs[index].bytes); assert.equal(sha256, provenance.sourceInputs[index].sha256); }
    inputs.push({ path: relative, bytes: bytes.length, sha256 });
  }
  return inputs;
}

/** Source-export permissions are provenance. Git checkout permissions are
 * canonical; a documented group-write umask difference cannot change bytes,
 * blob identity, regular-file type or any executable bit. This is a test-source
 * integrity check, never a replacement for real Org/storage ancestor guards. */
export function verifySourceRecord(expected, actual, tree) {
  assert.equal(actual.regularFile, true, 'Expected a regular source file');
  assert.equal(actual.sha256, expected.sha256, 'Source byte digest differs');
  assert.equal(actual.bytes, expected.bytes, 'Source byte length differs');
  assert.equal(tree, `${expected.gitMode} blob ${expected.blobOid}\t${expected.path}`, 'Git source blob or mode differs');
  assert.ok(['100644', '100755'].includes(expected.gitMode), 'Unsupported canonical Git source mode');
  const canonical = Number.parseInt(expected.gitMode.slice(-3), 8), physical = Number.parseInt(actual.mode, 8);
  assert.ok(Number.isInteger(physical) && physical >= 0 && physical <= 0o7777, 'Invalid physical source mode');
  assert.equal(physical & 0o7000, 0, 'Special source permission bits refused');
  assert.equal(physical & 0o002, 0, 'World-writable source refused');
  assert.equal(physical & 0o111, canonical & 0o111, 'Source executable semantics differ');
  assert.equal((physical ^ canonical) & ~0o020, 0, 'Only the documented group-write umask difference is permitted');
  return { ...actual, originPhysicalMode: expected.physicalMode, canonicalGitMode: expected.gitMode,
    canonicalPhysicalMode: canonical.toString(8), physicalModeDelta: (physical ^ canonical).toString(8) };
}
