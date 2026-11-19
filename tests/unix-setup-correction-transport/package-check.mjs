import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { copyFile, mkdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { digest } from '../fixtures/native-runtime/codex.mjs';
import { inspectPackedPackage } from './package-proof.mjs';

export async function checkLinuxPackage({ manager, npmCli, inputs, head, destination, command, env }) {
  assert.equal(process.platform, 'linux');
  const native = path.join(manager, 'dist/cli/native');
  await mkdir(native, { recursive: true });
  for (const [name, expected] of Object.entries(inputs.helper)) {
    const frozen = path.join(import.meta.dirname, 'frozen', name);
    assert.equal((await digest(frozen)).sha256, expected);
    await copyFile(frozen, path.join(native, name));
  }
  const helper = await digest(path.join(native, 'gestalt-owned-process.exe'));
  const provenance = JSON.parse(await readFile(helper.path + '.provenance.json', 'utf8'));
  assert.equal(provenance.artifactSha256, helper.sha256);
  assert.equal(provenance.platform + '-' + provenance.architecture, 'win32-x64');
  for (const item of provenance.sourceInputs) {
    const file = await digest(path.join(manager, item.path));
    assert.equal(file.bytes, item.bytes);
    assert.equal(file.sha256, item.sha256);
    assert.equal(file.sha256, inputs.sourceDigests[item.path]);
  }
  // This invocation uses the product's ordinary source-bound package producer.
  await command('linux-package-build', process.execPath, ['scripts/build-npm-package.mjs'], { env });
  const pack = JSON.parse(await command('linux-package-pack', process.execPath,
    [npmCli, 'pack', './dist/npm', '--json', '--offline', '--ignore-scripts', '--pack-destination', destination], { env }));
  assert.equal(pack.length, 1);
  assert.equal(pack[0].filename, 'jaromil-gestalt-0.16.1.tgz');
  const packageRoot = path.join(manager, 'dist/npm'), tarball = path.join(destination, pack[0].filename);
  const members = await inspectPackedPackage(tarball, npmCli, packageRoot);
  const manifest = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));
  const prior = inputs.reviewedLinuxR1Package;
  assert.equal(manifest.gitHead, head);
  assert.deepEqual(manifest, { ...prior.manifest, gitHead: head });
  const executable = new Set(['bin/gestalt', 'lib/gestalt.mjs', 'lib/legacy/gestalt']);
  const changed = [];
  for (const member of members) {
    assert.equal(member.mode, executable.has(member.name) ? '755' : '644', member.name);
    const old = prior.members.find(row => row.name === member.name);
    assert.ok(old, member.name);
    if (member.sha256 !== old.sha256) changed.push(member.name);
    if (member.name === 'README.md') assert.equal(member.sha256, inputs.sourceDigests['npm/README.md']);
    if (member.name === 'lib/gestalt.mjs') assert.equal(member.sha256, inputs.normalCliSha256);
    if (member.name === 'lib/legacy/gestalt') assert.equal(member.sha256, inputs.sourceDigests['public/gestalt']);
  }
  assert.deepEqual(changed.sort(), ['lib/legacy/gestalt', 'package.json']);
  assert.equal(pack[0].integrity, 'sha512-' + createHash('sha512').update(await readFile(tarball)).digest('base64'));
  return { tarball: await digest(tarball), manifest, members, changedFromLinuxR1: changed,
    inertWindowsHelper: helper, provenance, windowsHelperExecuted: false, packageInstalled: false, integrity: pack[0].integrity };
}
