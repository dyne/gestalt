import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

const digest = bytes => createHash('sha256').update(bytes).digest('hex');

export async function compositionInputs(base) {
  // Local package inputs exercise the actual update composition without a
  // floating optional-skills/Mobile release or a running Mobile service.
  const collection = path.join(base, 'skills source'), mobilePackage = path.join(base, 'mobile package'), release = path.join(base, 'release');
  await mkdir(path.join(collection, 'skills/native-update'), { recursive: true });
  const skill = '---\nname: native-update\ndescription: Native update fixture.\n---\n# Fixture\n';
  await writeFile(path.join(collection, 'skills/native-update/SKILL.md'), skill);
  await writeFile(path.join(collection, 'LICENSE'), 'MIT fixture\n');
  await writeFile(path.join(collection, 'manifest.json'), JSON.stringify({ schemaVersion: 1, repository: 'dyne/gestalt-skills', source: 'https://github.com/dyne/gestalt-skills', version: '0.1.0',
    skills: [{ name: 'native-update', entry: 'skills/native-update/SKILL.md', files: [{ path: 'skills/native-update/SKILL.md', sha256: digest(Buffer.from(skill)) }] }] }));
  for (const argv of [['init', '--initial-branch=main', '--template='], ['add', '--all'], ['commit', '-m', 'native standalone update fixture']]) {
    const result = spawnSync('git', ['-c', 'core.hooksPath='+path.join(collection, '.empty-hooks'), '-c', 'commit.gpgSign=false', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test', ...argv],
      { cwd: collection, encoding: 'utf8', shell: false });
    assert.equal(result.status, 0, result.stderr);
  }
  await mkdir(mobilePackage); await mkdir(release);
  await writeFile(path.join(mobilePackage, 'package.json'), JSON.stringify({ name: 'gestalt-mobile', version: '0.0.0', bin: { 'gestalt-mobile': 'cli.mjs' } }));
  await writeFile(path.join(mobilePackage, 'cli.mjs'), '#!/usr/bin/env node\nthrow new Error("Fixture Mobile must never launch");\n', { mode: 0o755 });
  return { collection, mobilePackage, release, skill };
}
