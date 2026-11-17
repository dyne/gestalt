import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';

/** Retarget the complete captured npm Node entry; never edit a partial '..'
 * path component or relax the production cmd-shim recognizer. */
export function capturedNodeCommand(captured, entry) {
  assert.match(entry, /^[A-Za-z0-9_-]+\.mjs$/);
  const bytes = Buffer.from(captured.base64, 'base64');
  assert.equal(bytes.length, captured.bytes);
  assert.equal(createHash('sha256').update(bytes).digest('hex'), captured.sha256);
  const source = bytes.toString('utf8');
  const target = '"%dp0%\\..\\native-forward-fixture\\entry.mjs"';
  assert.equal(source.split(target).length, 2, 'capture must contain exactly one complete fixture entry');
  return source.replace(target, `"%dp0%\\${entry}"`);
}
