import assert from 'node:assert/strict';
import { access, chmod, mkdtemp, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

import { setManagerVersion } from '../scripts/set-manager-version.mjs';

async function fixture(manager) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt-versioning-'));
  await mkdir(join(root, 'public'));
  await writeFile(join(root, 'public', 'gestalt'), manager, { mode: 0o755 });
  return root;
}

test('updates the manager version without a checksum or changing executable mode', async (t) => {
  const root = await fixture("#!/usr/bin/env bash\nreadonly GESTALT_CLI_VERSION='0.1.0'\n");
  t.after(() => rm(root, { recursive: true, force: true }));

  await chmod(join(root, 'public', 'gestalt'), 0o751);
  await setManagerVersion(root, '0.2.0');

  const manager = await readFile(join(root, 'public', 'gestalt'), 'utf8');
  const mode = await stat(join(root, 'public', 'gestalt'));

  assert.match(manager, /GESTALT_CLI_VERSION='0\.2\.0'/);
  await assert.rejects(access(join(root, 'public', 'gestalt.sha256')), { code: 'ENOENT' });
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

test('release version replaces an ahead-of-tag source version and is idempotent', async (t) => {
  const root = await fixture("readonly GESTALT_CLI_VERSION='0.5.1'\n");
  t.after(() => rm(root, { recursive: true, force: true }));
  await setManagerVersion(root, '0.6.0');
  const first = await readFile(join(root, 'public', 'gestalt'), 'utf8');
  assert.equal(first, "readonly GESTALT_CLI_VERSION='0.6.0'\n");
  await setManagerVersion(root, '0.6.0');
  assert.equal(await readFile(join(root, 'public', 'gestalt'), 'utf8'), first);
});
