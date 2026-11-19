// Actual npm installation in a disposable prefix. No host installation changes.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstat, open, readFile, realpath, writeFile } from 'node:fs/promises';
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

// A plain JS environment object loses the case-insensitive lookup of
// Windows process.env. Node keeps only the first spelling when spawning.
export function windowsPackedShimEnvironment(environment, commandDirectory) {
  const pathKeys = Object.keys(environment).filter(key => key.toUpperCase() === 'PATH');
  assert.equal(pathKeys.length, 1, 'Windows packed shim requires exactly one PATH key');
  const key = pathKeys[0], configuredPath = environment[key];
  assert.ok(typeof configuredPath === 'string' && configuredPath.trim(), 'Windows packed shim requires a nonempty PATH');
  const childEnvironment = { ...environment };
  delete childEnvironment[key];
  childEnvironment.PATH = [commandDirectory, configuredPath].join(path.win32.delimiter);
  return childEnvironment;
}

export async function installPackedCore(tarball, fixture, environment, agents, evidence, { npm12Entry } = {}) {
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
    let target;
    if (npm12Entry) {
      assert.equal(process.platform, 'win32', 'This lifecycle smoke uses the Windows ordinary account');
      const entry = await realpath(npm12Entry);
      prerequisite.entry = { path: entry, sha256: hash(await readFile(entry)) };
      target = { executable: process.execPath, prefix: [entry] };
    } else {
      const npm = await new NativeExecutables().resolve('npm', environment);
      assert.ok(npm, 'The primary supported npm lane is required');
      target = await npmPrerequisiteTarget(npm, prerequisite);
    }
    const npmVersion = run(target.executable, [...target.prefix, '--version']);
    assert.equal(npmVersion.status, 0, npmVersion.stderr);
    prerequisite.version = npmVersion.stdout.trim();
    if (npm12Entry) assert.match(prerequisite.version, /^12\./);
    env = { ...environment, GESTALT_AGENTS_SOURCE: agents,
      GESTALT_AGENTS_SETUP_SHA256: hash(await readFile(path.join(agents, 'gestalt-setup.mjs'))),
      GESTALT_AGENTS_SETUP_OWNER_SHA256: hash(await readFile(path.join(agents, 'scripts/setup-agents.mjs'))) };
    // Unix's accepted standalone setup fixture is prepared separately below;
    // its historical marketplace bootstrap is unchanged. Windows exercises the
    // real global lifecycle against the immutable accepted Node setup owner.
    const install = run(target.executable, [...target.prefix, 'install', '--global', '--prefix', prefix, '--cache', cache,
      '--no-audit', '--no-fund', ...(process.platform === 'win32' ? [] : ['--offline']), '--foreground-scripts',
      process.platform === 'win32' ? '--ignore-scripts=false' : '--ignore-scripts',
      ...(npm12Entry ? [`--allow-scripts=file:${tarball}`] : []), tarball]);
    assert.equal(install.status, 0, install.stderr);
    const commandDirectory = process.platform === 'win32' ? prefix : path.join(prefix, 'bin');
    const actualGlobalShim = path.join(commandDirectory, process.platform === 'win32' ? 'gestalt.cmd' : 'gestalt');
    if (process.platform === 'win32') {
      const observed = prerequisite.literalShim = { path: actualGlobalShim, observedAt: new Date().toISOString(),
        inputPathKeys: Object.keys(env).filter(key => key.toUpperCase() === 'PATH') };
      env = windowsPackedShimEnvironment(env, commandDirectory);
      observed.pathKeys = Object.keys(env).filter(key => key.toUpperCase() === 'PATH');
      observed.pathEntries = env.PATH.split(path.win32.delimiter);
      const extensionKeys = Object.keys(env).filter(key => key.toUpperCase() === 'PATHEXT').sort();
      observed.pathExt = env[extensionKeys[0]] ?? null;

      const info = await lstat(actualGlobalShim);
      assert.ok(info.isFile() && !info.isSymbolicLink(), 'Packed Windows shim must be a regular file');
      const physical = await realpath(actualGlobalShim);
      assert.equal(path.relative(await realpath(commandDirectory), physical), 'gestalt.cmd', 'Packed Windows shim escaped its owned prefix');
      const limit = 64 * 1024;
      assert.ok(info.size > 0 && info.size <= limit, 'Packed Windows shim exceeds its bounded observation size');
      const handle = await open(physical, 'r');
      try {
        const buffer = Buffer.alloc(limit + 1);
        const { bytesRead } = await handle.read(buffer, 0, buffer.length, 0);
        assert.equal(bytesRead, info.size, 'Packed Windows shim changed during observation');
        const bytes = buffer.subarray(0, bytesRead);
        const saved = 'packed-installed-shim.cmd.raw';
        await writeFile(path.join(evidence, saved), bytes, { flag: 'wx', mode: 0o600 });
        observed.script = { file: saved, bytes: bytesRead, sha256: hash(bytes) };
      } finally { await handle.close(); }

      const expectedNode = await realpath(process.execPath);
      const resolvedNode = await new NativeExecutables().resolve('node', env);
      const expectedSha256 = hash(await readFile(expectedNode));
      observed.node = { expectedPath: expectedNode, expectedSha256, resolvedPath: resolvedNode, resolvedSha256: null };
      assert.equal(resolvedNode, expectedNode, 'Packed Windows shim must resolve the same Node executable');
      observed.node.resolvedSha256 = hash(await readFile(resolvedNode));
      assert.equal(observed.node.resolvedSha256, expectedSha256, 'Packed Windows shim Node bytes differ');
    } else {
      env.PATH = [commandDirectory, env.PATH].join(path.delimiter);
    }
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
    if (process.platform === 'win32') {
      const result = run(path.join(env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'),
        ['-NoLogo', '-NoProfile', '-NonInteractive', '-Command', 'gestalt.cmd --version']);
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.stdout.split(/\r?\n/, 1)[0], `gestalt ${manifest.version}`);
    }
    return { entry, packageRoot, environment: env, receipt: { tarball, sha256: hash(await readFile(tarball)), manifest,
      prefix, actualGlobalShim,
      npmVersion: prerequisite.version, lifecycleRequested: process.platform === 'win32', observedAt: new Date().toISOString() } };
  } catch (error) {
    failure = { message: error.message, stack: error.stack }; throw error;
  } finally {
    await writeFile(path.join(evidence, 'packed-install-commands.json'), JSON.stringify(commands, null, 2) + '\n');
    await writeFile(path.join(evidence, 'packed-prerequisite.json'), JSON.stringify({ ...prerequisite, failure }, null, 2) + '\n');
  }
}
