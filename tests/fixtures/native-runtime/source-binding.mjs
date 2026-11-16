import assert from 'node:assert/strict';

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
