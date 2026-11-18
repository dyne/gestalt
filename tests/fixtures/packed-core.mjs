// Actual npm installation in a disposable prefix. No host installation changes.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFile, realpath, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { commandTarget, NativeExecutables } from '../../src/cli/platform/executables.ts';

const hash = bytes => createHash('sha256').update(bytes).digest('hex');
// Exact bundled npm11 prerequisite wrapper, also observed on the failed
// ordinary-user runner. This is not npm's generated package cmd-shim.
const bundledNpm11 = '21b46c69ad6e2f231f02a9e120f4ba6c8e75fef5a45637103002eab99f888ab8';
export async function npmPrerequisiteTarget(executable, receipt, nodeExecutable = process.execPath) {
  receipt.selected = executable;
  const bytes = await readFile(executable);
  receipt.selectedBytes = { bytes: bytes.length, sha256: hash(bytes) };
  if (/\.(cmd|bat)$/i.test(executable)) receipt.batchSource = bytes.toString('utf8');
  if (!/^npm\.cmd$/i.test(path.basename(executable)) || hash(bytes) !== bundledNpm11) {
    const target = await commandTarget(executable);
    receipt.kind = 'existing-generic-command-target'; receipt.target = target;
    return target;
  }
  const node = await realpath(nodeExecutable), shim = await realpath(executable);
  assert.equal(path.relative(path.dirname(node), path.dirname(shim)), '', 'Bundled npm must be adjacent to the selected Node runtime');
  const expectedRoot = path.join(path.dirname(node), 'node_modules/npm');
  const npmRoot = await realpath(expectedRoot);
  assert.equal(path.relative(expectedRoot, npmRoot), '', 'Bundled npm package must remain inside the selected Node distribution');
  const manifestBytes = await readFile(path.join(npmRoot, 'package.json'));
  const manifest = JSON.parse(manifestBytes);
  receipt.package = { root: npmRoot, name: manifest.name, version: manifest.version, bytes: manifestBytes.length, sha256: hash(manifestBytes) };
  assert.equal(manifest.name, 'npm');
  assert.match(manifest.version, /^11\./, 'This correction uses the primary npm11 lane');
  assert.equal(manifest.bin?.npm, 'bin/npm-cli.js');
  const entry = await realpath(path.join(npmRoot, 'bin/npm-cli.js'));
  assert.equal(path.relative(npmRoot, entry), path.join('bin', 'npm-cli.js'), 'npm entry must remain inside its package');
  const entryBytes = await readFile(entry);
  receipt.entry = { path: entry, bytes: entryBytes.length, sha256: hash(entryBytes) };
  assert.match(entryBytes.toString('utf8').split(/\r?\n/, 1)[0], /^#!\/usr\/bin\/env node$/);
  // Execute the verified bundled CLI directly. Never evaluate npm.cmd's
  // prefix-discovery batch program or generalize the product batch recognizer.
  const target = { executable: node, prefix: [entry] };
  receipt.kind = 'verified-bundled-npm11-node-entry'; receipt.target = target;
  return target;
}

export async function installPackedCore(tarball, fixture, environment, agents, evidence) {
  const prefix = path.join(fixture, 'npm prefix');
  const cache = path.join(fixture, 'npm cache');
  let env = { ...environment }, failure = null;
  const prerequisite = { schemaVersion: 1, observedAt: new Date().toISOString() };
  const commands = [];
  const run = (executable, argv, options = {}) => {
    const result = spawnSync(executable, argv, { cwd: fixture, env, encoding: 'utf8', timeout: 900000, shell: false, ...options });
    commands.push({ command: [executable, ...argv], cwd: fixture, exit: result.status, signal: result.signal,
      error: result.error?.message ?? null, stdout: result.stdout, stderr: result.stderr, observedAt: new Date().toISOString() });
    return result;
  };
  try {
    const npm = await new NativeExecutables().resolve('npm', environment);
    assert.ok(npm, 'The primary supported npm lane is required');
    const target = await npmPrerequisiteTarget(npm, prerequisite);
    env = { ...environment, GESTALT_AGENTS_SOURCE: agents,
      GESTALT_AGENTS_SETUP_SHA256: hash(await readFile(path.join(agents, 'gestalt-setup.mjs'))),
      GESTALT_AGENTS_SETUP_OWNER_SHA256: hash(await readFile(path.join(agents, 'scripts/setup-agents.mjs'))) };
    // Unix's accepted standalone setup fixture is prepared separately below;
    // its historical marketplace bootstrap is unchanged. Windows exercises the
    // real global lifecycle against the immutable accepted Node setup owner.
    const install = run(target.executable, [...target.prefix, 'install', '--global', '--prefix', prefix, '--cache', cache,
      '--no-audit', '--no-fund', ...(process.platform === 'win32' ? [] : ['--offline']), '--foreground-scripts',
      process.platform === 'win32' ? '--ignore-scripts=false' : '--ignore-scripts', tarball]);
    assert.equal(install.status, 0, install.stderr);
    const commandDirectory = process.platform === 'win32' ? prefix : path.join(prefix, 'bin');
    env.PATH = [commandDirectory, env.PATH].join(path.delimiter);
    // Fixed test commands exercise npm's literal global shim from cmd or PATH.
    // No caller-controlled argument enters shell text.
    for (const argv of [['--help'], ['--version'], ['doctor'], ['skills', '--help'], ['scratch', '--help']]) {
      const result = process.platform === 'win32'
        ? run(path.join(env.SystemRoot, 'System32/cmd.exe'), ['/d', '/s', '/c', 'gestalt ' + argv.join(' ')])
        : run('gestalt', argv);
      if (argv[0] === 'doctor') {
        // doctor reports missing host/optional prerequisites with status1.
        // Retain the actual diagnostic result; this is command execution proof,
        // never a claim that every optional tool or sandbox is available.
        assert.ok([0, 1].includes(result.status), result.stderr);
        assert.match(result.stdout, /Gestalt|gestalt|Codex/);
      } else assert.equal(result.status, 0, result.stderr);
    }
    const packageRoot = process.platform === 'win32' ? path.join(prefix, 'node_modules/@jaromil/gestalt') : path.join(prefix, 'lib/node_modules/@jaromil/gestalt');
    const entry = path.join(packageRoot, 'bin/gestalt');
    const manifest = JSON.parse(await readFile(path.join(packageRoot, 'package.json')));
    return { entry, packageRoot, environment: env, receipt: { tarball, sha256: hash(await readFile(tarball)), manifest,
      prefix, actualGlobalShim: path.join(commandDirectory, process.platform === 'win32' ? 'gestalt.cmd' : 'gestalt'),
      lifecycleRequested: process.platform === 'win32', observedAt: new Date().toISOString() } };
  } catch (error) {
    failure = { message: error.message, stack: error.stack }; throw error;
  } finally {
    await writeFile(path.join(evidence, 'packed-install-commands.json'), JSON.stringify(commands, null, 2) + '\n');
    await writeFile(path.join(evidence, 'packed-prerequisite.json'), JSON.stringify({ ...prerequisite, failure }, null, 2) + '\n');
  }
}
