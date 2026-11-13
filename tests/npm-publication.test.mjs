import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import { shouldStage } from '../scripts/inspect-npm-publication.mjs';

const release = { name: '@jaromil/gestalt', version: '0.14.0', gitHead: 'a'.repeat(40) };

test('stages a new version and skips the exact approved release', () => {
  assert.equal(shouldStage(release, null), true);
  assert.equal(shouldStage(release, { ...release }), false);
});

for (const change of [
  { name: 'another-package' },
  { version: '0.14.1' },
  { gitHead: 'b'.repeat(40) },
  { gitHead: undefined },
]) {
  test(`refuses a conflicting publication: ${Object.keys(change)[0]}`, () => {
    assert.throws(() => shouldStage(release, { ...release, ...change }), /refusing to replace/);
  });
}

test('release workflow submits for review and never publishes or approves directly', async () => {
  const workflow = await readFile(new URL('../.github/workflows/deploy-pages.yml', import.meta.url), 'utf8');
  const npmJob = workflow.slice(workflow.indexOf('\n  publish-npm:'));
  assert.match(npmJob, /npm stage publish \.\/dist\/npm --access public --tag latest/);
  assert.doesNotMatch(npmJob, /npm publish\b|npm stage approve\b/);
  assert.match(npmJob, /outputs\.stage == 'true'/);
});
