#!/usr/bin/env node
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, mkdtemp, readFile, realpath, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { installPackedCore } from '../tests/fixtures/packed-core.mjs';
import { isolatedEnvironment } from '../tests/fixtures/native-runtime/codex.mjs';
import { captureCopyTree } from './native-copy-provenance.mjs';

assert.ok(['linux', 'darwin'].includes(process.platform), 'Existing Unix package gate only');
const [tarballInput, expectedHash, agentsInput, evidenceInput] = process.argv.slice(2);
assert.ok(tarballInput && agentsInput && evidenceInput && /^[a-f0-9]{64}$/.test(expectedHash));
const tarball = await realpath(tarballInput), agents = await realpath(agentsInput), evidence = path.resolve(evidenceInput);
await mkdir(evidence, { recursive: true });
assert.equal(createHash('sha256').update(await readFile(tarball)).digest('hex'), expectedHash);
const fixture = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt-packed-unix-'))), home = path.join(fixture, 'home');
await mkdir(home, { mode: 0o700 });
const environment = { ...isolatedEnvironment(path.join(fixture, 'Codex')), HOME: home, USERPROFILE: home, GESTALT_HOME: path.join(fixture, 'Gestalt') };
const commands = []; let failure;
try {
  const installed = await installPackedCore(tarball, fixture, environment, agents, evidence);
  assert.ok(installed.receipt.manifest.os.includes(process.platform));
  const before = await captureCopyTree(installed.packageRoot);
  const selected = { ...installed.environment, GESTALT_TEST_INSTALLED_ENTRY: installed.receipt.actualGlobalShim };
  const root = path.resolve(import.meta.dirname, '..');
  const run = async (name, executable, argv) => {
    const result = spawnSync(executable, argv, { cwd: root, env: selected, encoding: 'utf8', shell: false, timeout: 180000, maxBuffer: 32 * 1024 * 1024 });
    await writeFile(path.join(evidence, name + '.stdout'), result.stdout || '');
    await writeFile(path.join(evidence, name + '.stderr'), result.stderr || '');
    commands.push({ command: [executable, ...argv], cwd: root, exit: result.status, signal: result.signal, error: result.error?.message ?? null, observedAt: new Date().toISOString() });
    assert.equal(result.status, 0, result.stderr);
    return result.stdout;
  };
  const tap = await run('installed-unix-contracts', process.execPath, ['--test', '--test-reporter=tap',
    '--test-name-pattern=^(fresh and repeated installs|update adds|recovery requires explicit quiescence)',
    'tests/skills-lifecycle.test.mjs', 'tests/session-scratch-recovery.test.mjs']);
  assert.match(tap, /^# pass 3$/m); assert.match(tap, /^# fail 0$/m);
  const transfer = await run('installed-unix-transfer', 'bash', ['tests/skills-transfer.test.sh']);
  assert.match(transfer, /skills-transfer\.test: PASS/);
  const after = await captureCopyTree(installed.packageRoot); assert.deepEqual(after, before);
  await writeFile(path.join(evidence, 'unix-installed-package.json'), JSON.stringify({ observedAt: new Date().toISOString(),
    platform: process.platform, architecture: process.arch, node: process.version, package: installed.receipt,
    before, after, tests: 4, passed: 4, failed: 0, scope: 'Actual installed global package: three Unix skill/recovery contracts plus executable-resource transfer; no new Unix supervisor or native optional port.' }, null, 2) + '\n');
} catch (error) { failure = { message: error.message, stack: error.stack }; throw error; }
finally { await writeFile(path.join(evidence, 'unix-package-commands.json'), JSON.stringify({ commands, failure: failure ?? null, fixture, observedAt: new Date().toISOString() }, null, 2) + '\n'); }
