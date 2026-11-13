import assert from 'node:assert/strict';
import test from 'node:test';
import { shouldPublish } from '../scripts/inspect-npm-publication.mjs';

const release = { name: '@jaromil/gestalt', version: '0.14.0', gitHead: 'a'.repeat(40) };

test('publishes a new version and skips a rerun of the exact published release', () => {
  assert.equal(shouldPublish(release, null), true);
  assert.equal(shouldPublish(release, { ...release }), false);
});

for (const change of [
  { name: 'another-package' },
  { version: '0.14.1' },
  { gitHead: 'b'.repeat(40) },
  { gitHead: undefined },
]) {
  test(`refuses a conflicting publication: ${Object.keys(change)[0]}`, () => {
    assert.throws(() => shouldPublish(release, { ...release, ...change }), /refusing to replace/);
  });
}
