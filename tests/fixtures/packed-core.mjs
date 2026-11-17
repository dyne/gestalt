// Actual npm installation in a disposable prefix. No host installation changes.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { commandTarget, NativeExecutables } from '../../src/cli/platform/executables.ts';

export async function installPackedCore(tarball, fixture, environment, agents, evidence) {
  const prefix = path.join(fixture, 'npm prefix');
  const cache = path.join(fixture, 'npm cache');
  const npm = await new NativeExecutables().resolve('npm', environment);
  assert.ok(npm, 'The primary supported npm lane is required');
  const target = await commandTarget(npm);
  const hash = bytes => createHash('sha256').update(bytes).digest('hex');
  const env = { ...environment, GESTALT_AGENTS_SOURCE: agents,
    GESTALT_AGENTS_SETUP_SHA256: hash(await readFile(path.join(agents, 'gestalt-setup.mjs'))),
    GESTALT_AGENTS_SETUP_OWNER_SHA256: hash(await readFile(path.join(agents, 'scripts/setup-agents.mjs'))) };
  const commands = [];
  const run = (executable, argv, options = {}) => {
    const result = spawnSync(executable, argv, { cwd: fixture, env, encoding: 'utf8', timeout: 900000, shell: false, ...options });
    commands.push({ command: [executable, ...argv], cwd: fixture, exit: result.status, signal: result.signal,
      error: result.error?.message ?? null, stdout: result.stdout, stderr: result.stderr, observedAt: new Date().toISOString() });
    return result;
  };
  try {
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
      lifecycleExecuted: process.platform === 'win32', observedAt: new Date().toISOString() } };
  } finally {
    await writeFile(path.join(evidence, 'packed-install-commands.json'), JSON.stringify(commands, null, 2) + '\n');
  }
}
