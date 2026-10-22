import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import test from 'node:test';

async function sample(t) {
  const root = await mkdtemp('/tmp/impeccable-ci-');
  t.after(() => rm(root, { recursive: true, force: true }));
  return { root, xml: join(root, 'results.xml'), summary: join(root, 'summary.json') };
}
test('source preparation rejects a changed vendored patch before fetching or executing source', async t => {
  const f = await sample(t), scripts = join(f.root, 'scripts'), vendor = join(f.root, 'vendor/impeccable');
  await mkdir(scripts); await mkdir(vendor, { recursive: true });
  const prepare = join(scripts, 'prepare-impeccable-runtime.sh');
  await writeFile(prepare, await readFile('scripts/prepare-impeccable-runtime.sh'));
  await writeFile(join(vendor, 'provenance.json'), await readFile('vendor/impeccable/provenance.json'));
  await writeFile(join(vendor, 'remote-public-url.patch'), await readFile('vendor/impeccable/remote-public-url.patch'));
  assert.equal(spawnSync('bash', [prepare, '--verify-inputs']).status, 0);
  await writeFile(join(vendor, 'remote-public-url.patch'), 'corrupted source patch');
  assert.notEqual(spawnSync('bash', [prepare, '--verify-inputs']).status, 0);
});
