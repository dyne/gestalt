import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import test from 'node:test';

const checker = resolve('scripts/check-impeccable-e2e.py');
const escape = s => s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('"', '&quot;');
async function sample(t) {
  const root = await mkdtemp('/tmp/impeccable-ci-');
  t.after(() => rm(root, { recursive: true, force: true }));
  return { root, xml: join(root, 'results.xml'), summary: join(root, 'summary.json') };
}
function xml({ newFailure = false, missing = false, newSkip = false } = {}) {
  const astro = 'live-e2e · astro-vite7 (plain-css)', react = 'live-e2e · vite8-react-plain (plain-css)';
  const failure = (name, message) => `<testcase name="${escape(name)}"><failure message="${escape(message)}"/></testcase>`;
  let document = '<testsuites><testsuite name="' + astro + '">' +
    failure('drives the full click → Go → cycle → accept cycle', newFailure ? 'new unrelated failure' : '500 astro&type=style&index=1') +
    `<testcase name="${newSkip ? 'new skip' : 'recovers when the preflight reload makes the browser miss the done broadcast'}"><skipped/></testcase></testsuite>`;
  document += '<testsuite name="' + react + '">' + failure('Edit copy → Save → Apply/commit: React headless manual Apply hard batch',
    'visible text span.secondary-action did not include Secondary duplicate action') + '</testsuite><testsuite name="other">';
  for (let n = 0; n < (missing ? 43 : 44); n++) document += `<testcase name="pass-${n}"/>`;
  return document + '</testsuite></testsuites>';
}

test('E2E result gate retains the two exact failures and rejects new errors, skips and incomplete evidence', async t => {
  const f = await sample(t);
  await writeFile(f.xml, xml());
  let result = spawnSync('python3', [checker, f.xml, f.summary, '1'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  const summary = JSON.parse(await readFile(f.summary, 'utf8'));
  assert.equal(summary.tests, 47); assert.equal(summary.failures, 2); assert.equal(summary.skipped, 1);
  assert.equal(summary.state, 'known-upstream-limitations');
  for (const change of [{ newFailure: true }, { missing: true }, { newSkip: true }]) {
    await writeFile(f.xml, xml(change));
    result = spawnSync('python3', [checker, f.xml, f.summary, '1'], { encoding: 'utf8' });
    assert.notEqual(result.status, 0);
  }
  await writeFile(f.xml, xml());
  assert.notEqual(spawnSync('python3', [checker, f.xml, f.summary, '0']).status, 0);
});

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
