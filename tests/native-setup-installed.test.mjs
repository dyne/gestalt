/** Hosted integration: the compiled manager invokes the exact accepted Agents
 * Node owner and actual npm/Codex, in isolated homes. No live installation. */
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { buildCli } from '../scripts/build-cli.mjs';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { commandTarget, NativeExecutables } from '../src/cli/platform/executables.ts';

const root = path.resolve(import.meta.dirname, '..');
const evidence = process.env.GESTALT_NATIVE_SETUP_EVIDENCE;
const dependency = process.env.GESTALT_NATIVE_AGENTS_SOURCE;
if (!evidence || !dependency) throw new Error('native installed setup requires exact dependency and evidence paths');
await mkdir(evidence, { recursive: true });
const digest = bytes => createHash('sha256').update(bytes).digest('hex');

test('actual compiled manager provisions accepted native Agents and repeats offline', { timeout: 600_000 }, async t => {
  const base = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt core installed ü & ')));
  t.after(() => rm(base, { recursive: true, force: true }));
  const manager = await buildCli(root, path.join(base, 'gestalt.mjs'));
  const probe = await buildNativePlatform(root, path.join(base, 'native'));
  const home = path.join(base, 'home ü &'), codex = path.join(home, 'Codex ü &'), gestalt = path.join(home, 'Gestalt ü &');
  await mkdir(home, { mode: 0o700 });
  const environment = { ...process.env, HOME: home, USERPROFILE: home, CLAUDE_CONFIG_DIR: path.join(home, '.claude'), CONTEXT_MODE_DIR: path.join(home, 'context-mode-data'),
    CODEX_HOME: codex, GESTALT_HOME: gestalt, GESTALT_MANAGER_DISTRIBUTION: 'npm', GESTALT_AGENTS_SOURCE: await realpath(dependency),
    GESTALT_AGENTS_SETUP_SHA256: digest(await readFile(path.join(dependency, 'gestalt-setup.mjs'))), GESTALT_NATIVE_UNIX_PROBE: probe || '',
    CONTEXT_MODE_PACKAGE_MANAGER: 'npm', npm_config_ignore_scripts: 'true', CONTEXT_MODE_PLATFORM: 'codex', CONTEXT_MODE_WORKSPACE: base };
  for (const key of Object.keys(environment)) if (key.startsWith('GESTALT_MOBILE_ORG_PLAN_')) delete environment[key];
  const checks = [];
  const run = async (label, argv, extra = {}) => {
    const observedAt = new Date().toISOString();
    const result = spawnSync(process.execPath, [manager, ...argv], { env: { ...environment, ...extra }, cwd: base, encoding: 'utf8', timeout: 540_000, maxBuffer: 16 * 1024 * 1024, shell: false });
    const log = path.join(evidence, `${label}.log`), bytes = Buffer.from((result.stdout ?? '') + (result.stderr ?? ''));
    await writeFile(log, bytes);
    checks.push({ label, command: [process.execPath, manager, ...argv], cwd: base, status: result.status, signal: result.signal, error: result.error?.message ?? null,
      observedAt, log, sha256: digest(bytes) });
    await writeFile(path.join(evidence, 'installed-checks.json'), JSON.stringify(checks, null, 2));
    assert.equal(result.status, 0, `${label}: ${result.error?.message ?? ''}; see ${log}: ${(result.stderr ?? '').slice(-3000)}`);
  };
  await run('manager-agents-fresh', ['agents', 'install']);
  const outputs = ['config.toml', 'hooks.json', 'bin/context-mode-mcp.mjs', 'bin/context-mode-hook.mjs', '.gestalt-setup-provenance.json'];
  const first = await Promise.all(outputs.map(async file => digest(await readFile(path.join(codex, file)))));
  await run('manager-agents-repeat-offline', ['agents', 'update'], { npm_config_offline: 'true' });
  assert.deepEqual(await Promise.all(outputs.map(async file => digest(await readFile(path.join(codex, file))))), first);
  const prepared = JSON.parse(await readFile(path.join(gestalt, 'runtime/context-mode/.context-mode-prepared.json'), 'utf8'));
  assert.equal(prepared.platform, process.platform); assert.equal(prepared.arch, process.arch); assert.equal(prepared.nodeModulesAbi, process.versions.modules);
  const provenance = JSON.parse(await readFile(path.join(codex, '.gestalt-setup-provenance.json'), 'utf8'));
  assert.equal(provenance.manager.sha256, digest(await readFile(manager)));
  assert.equal(provenance.delegatedSetupSha256, environment.GESTALT_AGENTS_SETUP_SHA256);
  const codexExecutable = await new NativeExecutables().resolve('codex', environment);
  assert.ok(codexExecutable);
  const target = await commandTarget(codexExecutable);
  const version = spawnSync(target.executable, [...target.prefix, '--version'], { env: environment, encoding: 'utf8', shell: false });
  await writeFile(path.join(evidence, 'installed-identity.json'), JSON.stringify({ platform: process.platform, architecture: process.arch, node: process.version,
    abi: process.versions.modules, managerSha256: digest(await readFile(manager)), dependencySource: environment.GESTALT_AGENTS_SOURCE,
    delegatedSetupSha256: provenance.delegatedSetupSha256, codexVersion: version.stdout?.trim(), codexStatus: version.status,
    runtime: prepared, observedAt: new Date().toISOString() }, null, 2));
});
