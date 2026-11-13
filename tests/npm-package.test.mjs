import assert from 'node:assert/strict';
import { execFile, spawnSync } from 'node:child_process';
import { copyFile, lstat, mkdir, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

import { buildNpmPackage } from '../scripts/build-npm-package.mjs';

const exec = promisify(execFile);
const repository = resolve(import.meta.dirname, '..');

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt npm package '));
  t.after(() => rm(root, { recursive: true, force: true }));
  for (const file of ['public/gestalt', 'npm/package.json', 'npm/gestalt', 'npm/README.md', 'npm/LICENSE']) {
    await mkdir(dirname(join(root, file)), { recursive: true });
    await copyFile(join(repository, file), join(root, file));
  }
  await exec('git', ['init', '--quiet', root]);
  await exec('git', ['-c', 'user.name=Package test', '-c', 'user.email=package-test@example.invalid',
    'commit', '--quiet', '--allow-empty', '-m', 'fixture'], { cwd: root });
  return root;
}

test('builds a versioned executable package containing the canonical manager and no dependencies or hooks', async t => {
  const root = await fixture(t);
  const destination = await buildNpmPackage(root);
  const manifest = JSON.parse(await readFile(join(destination, 'package.json'), 'utf8'));
  const manager = await readFile(join(root, 'public/gestalt'), 'utf8');
  const version = /^readonly GESTALT_CLI_VERSION='([^']+)'$/m.exec(manager)[1];
  assert.equal(manifest.name, '@jaromil/gestalt');
  assert.equal(manifest.version, version);
  assert.equal(manifest.gitHead, (await exec('git', ['rev-parse', 'HEAD'], { cwd: root })).stdout.trim());
  assert.equal(manifest.publishConfig.access, 'public');
  assert.equal(manifest.dependencies, undefined);
  assert.equal(manifest.scripts, undefined);
  assert.equal(await readFile(join(destination, 'lib/gestalt'), 'utf8'), manager);
  assert.equal((await stat(join(destination, 'bin/gestalt'))).mode & 0o777, 0o755);
  const files = (await readdir(destination, { recursive: true })).filter(file => !['bin', 'lib'].includes(file)).sort();
  assert.deepEqual(files, ['LICENSE', 'README.md', 'bin/gestalt', 'lib/gestalt', 'package.json']);
  await writeFile(join(destination, 'stale-file'), 'not part of the next release');
  await buildNpmPackage(root);
  assert.equal(await readFile(join(destination, 'lib/gestalt'), 'utf8'), manager);
  await assert.rejects(stat(join(destination, 'stale-file')), { code: 'ENOENT' });
});

test('installs the actual npm tarball offline and preserves symlinks, arguments, stdin and exit status', async t => {
  const root = await fixture(t);
  const destination = await buildNpmPackage(root);
  const manifest = JSON.parse(await readFile(join(destination, 'package.json'), 'utf8'));
  const npmOptions = ['--cache', join(root, 'cache'), '--ignore-scripts'];
  const packResult = JSON.parse((await exec('npm', ['pack', destination, '--json', '--pack-destination', root,
    ...npmOptions], { cwd: root, timeout: 30_000 })).stdout);
  const packed = Array.isArray(packResult) ? packResult[0] : packResult[manifest.name];
  assert.ok(packed, 'npm pack did not return metadata for the expected package');
  assert.deepEqual(packed.files.map(file => file.path).sort(),
    ['LICENSE', 'README.md', 'bin/gestalt', 'lib/gestalt', 'package.json']);
  const prefix = join(root, 'global prefix');
  await exec('npm', ['install', '--global', '--prefix', prefix, '--offline', '--no-audit', '--no-fund',
    ...npmOptions, join(root, packed.filename)], { cwd: root, timeout: 30_000 });
  const binary = join(prefix, 'bin/gestalt');
  assert.ok((await lstat(binary)).isSymbolicLink());
  assert.match((await exec(binary, ['--version'])).stdout, new RegExp(`^gestalt ${manifest.version.replaceAll('.', '\\.')}\\n`));
  assert.match((await exec(binary, ['--help'])).stdout, /Usage: gestalt/);
  const installedManager = join(prefix, 'lib/node_modules/@jaromil/gestalt/lib/gestalt');
  await writeFile(installedManager, '#!/usr/bin/env bash\nprintf "%s\\n" "$GESTALT_MANAGER_DISTRIBUTION" "$@"\ncat\nexit 23\n');
  const forwarding = spawnSync(binary, ['a b', '--literal=$HOME', ''], { input: 'standard input\n', encoding: 'utf8', timeout: 10_000 });
  assert.equal(forwarding.status, 23);
  assert.equal(forwarding.stdout, 'npm\na b\n--literal=$HOME\n\nstandard input\n');
});

test('npm updates leave the package and executable symlinks intact and print the npm update command', async t => {
  const root = await fixture(t);
  const destination = await buildNpmPackage(root);
  const binary = join(root, 'gestalt');
  await symlink(join(destination, 'bin/gestalt'), binary);
  const managerPath = join(destination, 'lib/gestalt');
  const source = await readFile(managerPath, 'utf8');
  const instrumented = source.replace(/\nmain "\$@"\s*$/, '\nupdate_manager\n');
  assert.notEqual(instrumented, source);
  await writeFile(managerPath, instrumented);
  const result = await exec(binary, []);
  assert.match(result.stderr, /npm install -g @jaromil\/gestalt@latest/);
  assert.equal(await readFile(managerPath, 'utf8'), instrumented);
  assert.ok((await lstat(binary)).isSymbolicLink());
});

test('rejects invalid manager versions before replacing an existing package', async t => {
  const root = await fixture(t);
  const destination = await buildNpmPackage(root);
  const original = await readFile(join(destination, 'package.json'), 'utf8');
  await writeFile(join(root, 'public/gestalt'), "readonly GESTALT_CLI_VERSION='invalid'\n");
  await assert.rejects(buildNpmPackage(root), /strict manager version/);
  assert.equal(await readFile(join(destination, 'package.json'), 'utf8'), original);
});
