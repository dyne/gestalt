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
  for (const file of ['public/gestalt', 'npm/package.json', 'npm/gestalt', 'npm/postinstall.mjs', 'npm/README.md', 'npm/LICENSE']) {
    await mkdir(dirname(join(root, file)), { recursive: true });
    await copyFile(join(repository, file), join(root, file));
  }
  await exec('git', ['init', '--quiet', root]);
  await exec('git', ['-c', 'user.name=Package test', '-c', 'user.email=package-test@example.invalid',
    'commit', '--quiet', '--allow-empty', '-m', 'fixture'], { cwd: root });
  return root;
}

test('builds a versioned executable package with the canonical manager and automatic Agents setup', async t => {
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
  assert.deepEqual(manifest.scripts, { postinstall: 'node postinstall.mjs' });
  assert.equal(await readFile(join(destination, 'lib/gestalt'), 'utf8'), manager);
  assert.equal((await stat(join(destination, 'bin/gestalt'))).mode & 0o777, 0o755);
  const files = (await readdir(destination, { recursive: true })).filter(file => !['bin', 'lib'].includes(file)).sort();
  assert.deepEqual(files, ['LICENSE', 'README.md', 'bin/gestalt', 'lib/gestalt', 'package.json', 'postinstall.mjs']);
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
    ['LICENSE', 'README.md', 'bin/gestalt', 'lib/gestalt', 'package.json', 'postinstall.mjs']);
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

async function agentsBootstrap(t) {
  const root = await fixture(t);
  const sourcePath = join(root, 'public/gestalt');
  const source = await readFile(sourcePath, 'utf8');
  // Retain the production command parser and npm lifecycle. Substitute only
  // network/install adapters so tests cannot change a developer's installation.
  await writeFile(sourcePath, source.replace(/\nmain "\$@"\s*$/, `
run_marketplace_setup() {
  printf '%s|%s|%s|%s|%s|%s\\n' "$1" "$2" "$codex_home" "$gestalt_home" \\
    "\${npm_config_global:-unset}" "\${npm_config_prefix:-unset}" >> "$AGENTS_TEST_LOG"
  [[ \${AGENTS_TEST_FAIL:-false} != true ]] || return 42
  mkdir -p -- "$codex_home/agents"
  printf 'installed\\n' > "$codex_home/agents/fixture.toml"
}
command_skills() { printf 'unexpected skills install\\n' >&2; exit 85; }
install_mobile() { printf 'unexpected Mobile install\\n' >&2; exit 86; }
main "$@"
`));
  const destination = await buildNpmPackage(root);
  const prefix = join(root, 'global prefix');
  const log = join(root, 'agents.log');
  const env = {
    ...process.env,
    CODEX_HOME: join(root, 'codex home'),
    GESTALT_HOME: join(root, 'gestalt home'),
    AGENTS_TEST_LOG: log,
  };
  const packedOutput = JSON.parse((await exec('npm', ['pack', destination, '--json', '--ignore-scripts',
    '--pack-destination', root, '--cache', join(root, 'cache')], { cwd: root, env })).stdout);
  const packed = Array.isArray(packedOutput) ? packedOutput[0] : packedOutput['@jaromil/gestalt'];
  const tarball = join(root, packed.filename);
  const npmMajor = Number((await exec('npm', ['--version'])).stdout.split('.')[0]);
  // npm 12 approves file dependencies by resolved file identity, rather than
  // the name declared inside the tarball. Never enable unrelated scripts.
  const approval = npmMajor >= 12 ? [`--allow-scripts=file:${tarball}`] : [];
  const install = (extra = {}, options = []) => exec('npm', ['install', '--global', '--prefix', prefix, '--offline',
    '--no-audit', '--no-fund', '--ignore-scripts=false', '--foreground-scripts',
    '--cache', join(root, 'cache'), ...approval, ...options, tarball], {
    cwd: root, env: { ...env, ...extra }, timeout: 30_000,
  });
  return { root, log, env, install, destination, binary: join(prefix, 'bin/gestalt') };
}

test('global npm installation automatically reconciles Agents without installing Mobile or optional skills', async t => {
  const f = await agentsBootstrap(t);
  await f.install();
  const expected = `install|false|${f.env.CODEX_HOME}|${f.env.GESTALT_HOME}|false|unset\n`;
  assert.equal(await readFile(f.log, 'utf8'), expected);
  assert.equal(await readFile(join(f.env.CODEX_HOME, 'agents/fixture.toml'), 'utf8'), 'installed\n');
  await f.install();
  assert.equal(await readFile(f.log, 'utf8'), expected.repeat(2));
  const beforeHelp = await readFile(f.log, 'utf8');
  assert.match((await exec(f.binary, ['agents', '--help'], { env: f.env })).stdout, /agents install\|update/);
  assert.equal(await readFile(f.log, 'utf8'), beforeHelp);
  await exec(f.binary, ['agents', 'update'], { env: f.env });
  assert.match(await readFile(f.log, 'utf8'), /\nupgrade\|false\|/);
});

test('an automatic Agents failure fails npm installation and supplies a recovery command', async t => {
  const f = await agentsBootstrap(t);
  await assert.rejects(f.install({ AGENTS_TEST_FAIL: 'true' }), error => {
    assert.notEqual(error.code, 0);
    assert.match(error.stderr, /Agents setup failed/);
    assert.match(error.stderr, /gestalt agents install/);
    return true;
  });
  assert.equal((await readFile(f.log, 'utf8')).split('\n').filter(Boolean).length, 1);
});

test('disabling lifecycle scripts leaves setup explicit and the installed command usable', async t => {
  const f = await agentsBootstrap(t);
  await f.install({}, ['--ignore-scripts']);
  await assert.rejects(stat(f.log), { code: 'ENOENT' });
  assert.match((await exec(f.binary, ['--help'], { env: f.env })).stdout, /Usage: gestalt/);
  await exec(f.binary, ['agents', 'install'], { env: f.env });
  assert.match(await readFile(f.log, 'utf8'), /^install\|false\|/);
});

test('non-global package setup does not provision a user profile', async t => {
  const f = await agentsBootstrap(t);
  await exec(process.execPath, [join(f.destination, 'postinstall.mjs')], {
    env: { ...f.env, npm_config_global: 'false', NPM_CONFIG_GLOBAL: 'false' },
  });
  await assert.rejects(stat(f.log), { code: 'ENOENT' });
});
