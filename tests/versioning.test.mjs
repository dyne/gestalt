import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { chmod, mkdtemp, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

import { setManagerVersion } from '../scripts/set-manager-version.mjs';

async function fixture(manager) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt-versioning-'));
  await mkdir(join(root, 'public'));
  await writeFile(join(root, 'public', 'gestalt'), manager, { mode: 0o755 });
  await writeFile(join(root, 'public', 'gestalt.sha256'), 'obsolete\n');
  return root;
}

test('updates the manager version and checksum without changing executable mode', async (t) => {
  const root = await fixture("#!/usr/bin/env bash\nreadonly GESTALT_CLI_VERSION='0.1.0'\n");
  t.after(() => rm(root, { recursive: true, force: true }));

  await chmod(join(root, 'public', 'gestalt'), 0o751);
  await setManagerVersion(root, '0.2.0');

  const manager = await readFile(join(root, 'public', 'gestalt'), 'utf8');
  const checksum = await readFile(join(root, 'public', 'gestalt.sha256'), 'utf8');
  const digest = createHash('sha256').update(manager).digest('hex');
  const mode = await stat(join(root, 'public', 'gestalt'));

  assert.match(manager, /GESTALT_CLI_VERSION='0\.2\.0'/);
  assert.equal(checksum, `${digest}  public/gestalt\n`);
  assert.equal(mode.mode & 0o777, 0o751);
});

test('rejects invalid versions without modifying files', async (t) => {
  const original = "readonly GESTALT_CLI_VERSION='0.1.0'\n";
  const root = await fixture(original);
  t.after(() => rm(root, { recursive: true, force: true }));

  await assert.rejects(setManagerVersion(root, 'v0.2.0'), /invalid strict semantic version/);
  assert.equal(await readFile(join(root, 'public', 'gestalt'), 'utf8'), original);
});

test('rejects ambiguous version declarations', async (t) => {
  const root = await fixture(
    "readonly GESTALT_CLI_VERSION='0.1.0'\nreadonly GESTALT_CLI_VERSION='0.1.0'\n",
  );
  t.after(() => rm(root, { recursive: true, force: true }));

  await assert.rejects(setManagerVersion(root, '0.2.0'), /expected exactly one/);
});
