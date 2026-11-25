import assert from 'node:assert/strict';
import { chmod, copyFile, mkdtemp, mkdir, readFile, readdir, realpath, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import test from 'node:test';
import { createDiagnosticHandlers, pluginVersion, verifyContextRuntime, DiagnosticContext } from '../src/cli/runtime/diagnostics.ts';
import { setupPaths } from '../src/cli/setup/context.ts';
import { diagnosticEnvironment } from '../src/cli/runtime/commands.ts';
import { runCli } from '../src/cli/dispatch.ts';
import { sessionScratch } from './session-scratch-fixture.mjs';

const metadata = { version: '0.16.1', help: 'help\n' };
async function fixture(t) {
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt diagnostic ü ')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const environment = { HOME: root, USERPROFILE: root, PATH: '', CODEX_HOME: path.join(root, 'custom Codex'), GESTALT_HOME: path.join(root, 'custom Gestalt'),
    GESTALT_AGENTS_VERSION: '2.26.0', GESTALT_CONTEXT_MODE_VERSION: '2.26.0' };
  const out = [], err = [], streams = { stdin: (async function* () {})(), stdout: async b => out.push(Buffer.from(b)), stderr: async b => err.push(Buffer.from(b)) };
  return { root, environment, streams, output: () => Buffer.concat(out).toString(), error: () => Buffer.concat(err).toString() };
}
async function inventory(root) {
  const records = [];
  async function visit(file, relative) {
    const { lstat } = await import('node:fs/promises'); const info = await lstat(file);
    records.push([relative, info.mode, info.mtimeMs, info.isFile() ? (await readFile(file)).toString('base64') : null]);
    if (info.isDirectory()) for (const child of (await readdir(file)).sort()) await visit(path.join(file, child), path.join(relative, child));
  }
  await visit(root, '.'); return records;
}
test('Node path handler resolves isolated native paths without creating missing homes or caches', async t => {
  const f = await fixture(t), before = await inventory(f.root);
  const code = await runCli({ argv: ['path', '--json'], cwd: f.root, environment: f.environment }, f.streams, metadata, createDiagnosticHandlers(metadata));
  assert.equal(code, 0, f.error()); const output = JSON.parse(f.output());
  assert.equal(output.codexHome, f.environment.CODEX_HOME); assert.equal(output.gestaltHome, f.environment.GESTALT_HOME);
  assert.equal(output.gestaltPlugin, path.join(f.environment.CODEX_HOME, 'plugins/cache/dyne-gestalt-agents/gestalt/2.26.0'));
  assert.equal(output.contextModeRuntime, path.join(f.environment.GESTALT_HOME, 'runtime/context-mode'));
  assert.deepEqual(await inventory(f.root), before);
});
test('version aliases route to runtime diagnostics with no implicit setup or metadata mutation', async t => {
  for (const token of ['version', '--version', '-V']) {
    const f = await fixture(t), before = await inventory(f.root);
    assert.equal(await runCli({ argv: [token], cwd: f.root, environment: f.environment }, f.streams, metadata, createDiagnosticHandlers(metadata)), 0);
    assert.match(f.output(), /^Gestalt Manager 0\.16\.1\n/); assert.match(f.output(), process.platform === 'win32' ? /DEFERRED \(native optional feature\)/ : /NOT INSTALLED \(optional\)/);
    assert.match(f.output(), /^Gestalt Skills +NOT INSTALLED \(optional\)$/m);
    assert.deepEqual(await inventory(f.root), before); assert.equal(f.error(), '');
  }
});
test('default diagnostics read exact installed metadata without launching commands or changing profiles, claims or TMP', async t => {
  const f = await fixture(t), bin = path.join(f.root, 'bin'); await mkdir(bin);
  for (const name of ['codex', 'npm']) {
    const file = path.join(bin, name + (process.platform === 'win32' ? '.exe' : ''));
    await copyFile(process.execPath, file); if (process.platform !== 'win32') await chmod(file, 0o700);
  }
  f.environment.PATH = bin;
  const paths = await setupPaths(f.environment), runtime = path.join(paths.gestaltHome, 'runtime/context-mode');
  await mkdir(runtime, { recursive: true }); await mkdir(paths.codexHome, { recursive: true });
  await writeFile(path.join(runtime, '.context-mode-prepared.json'), JSON.stringify({ packageVersion: '2.26.0', platform: process.platform, arch: process.arch, nodeModulesAbi: process.versions.modules }));
  for (const name of ['gestalt', 'context-mode']) {
    const plugin = path.join(paths.codexHome, 'plugins/cache', paths.marketplaceName, name, '2.26.0/.codex-plugin');
    await mkdir(plugin, { recursive: true }); await writeFile(path.join(plugin, 'plugin.json'), JSON.stringify({ name, version: '2.26.0' }));
  }
  await writeFile(path.join(paths.codexHome, 'config.toml'), `[plugins."context-mode@${paths.marketplaceName}"]\nenabled=false\n[mcp_servers.context-mode]\ncommand="node"\nargs=${JSON.stringify([path.join(paths.codexHome, 'bin/context-mode-mcp.mjs')])}\n`);
  await writeFile(path.join(paths.codexHome, 'claims.json'), '{"retained":true}\n');
  const selected = diagnosticEnvironment({ ...f.environment, TMPDIR: 'operator-tmp', TMP: 'operator-tmp', TEMP: 'operator-tmp' });
  assert.equal(selected.TMPDIR, 'operator-tmp'); assert.equal(selected.TMP, 'operator-tmp'); assert.equal(selected.TEMP, 'operator-tmp');
  let launches = 0;
  const port = { launch: async () => { launches++; throw new Error('read-only diagnostic launched a subprocess'); } };
  const context = new DiagnosticContext(paths, { argv: [], cwd: f.root, environment: selected }, f.streams, port);
  const before = await inventory(f.root);
  assert.equal(await verifyContextRuntime(context), runtime);
  const handlers = createDiagnosticHandlers(metadata, port);
  for (const argv of [['path', '--json'], ['version'], ['doctor']]) {
    await runCli({ argv, cwd: f.root, environment: selected }, f.streams, metadata, handlers);
    assert.deepEqual(await inventory(f.root), before);
  }
  assert.match(f.output(), /^Gestalt Agents +2\.26\.0$/m);
  assert.match(f.output(), /^Context Mode +2\.26\.0$/m);
  assert.equal(launches, 0); assert.match(f.output(), /Codex sandbox\s+NOT PROBED.*--probe-sandbox/);
  assert.doesNotMatch(f.output(), /Codex sandbox\s+ready/);
});
test('diagnostic version selectors cannot escape the plugin cache', () => {
  for (const version of ['../escape', '..', '.', 'a/b', 'a\\b', 'a\n']) assert.equal(pluginVersion([{ name: 'gestalt', version }], 'gestalt'), null);
  assert.equal(pluginVersion({ installed: [{ name: 'gestalt', installed: false, version: '1.2.3' }] }, 'gestalt'), null);
  assert.equal(pluginVersion({ installed: [{ name: 'gestalt', version: '2.26.0' }] }, 'gestalt'), '2.26.0');
});
test('Windows diagnostics keep unsupported npm launchers out of the setup command owner',
  { skip: process.platform !== 'win32' }, async t => {
    const f = await fixture(t), bin = path.join(f.root, 'bin');
    await mkdir(bin);
    await writeFile(path.join(bin, 'npm.cmd'), '@echo off\r\nexit /b 99\r\n');
    f.environment.PATH = bin;
    let launches = 0;
    const port = { launch: async () => { launches++; throw new Error('unsupported launcher reached the command owner'); } };
    const context = new DiagnosticContext(await setupPaths(f.environment), { argv: [], cwd: f.root, environment: f.environment }, f.streams, port, true);
    assert.equal(await context.executableVersion('npm'), null);
    assert.equal(launches, 0);

    const bundled = path.join(bin, 'node_modules/npm');
    await mkdir(path.join(bundled, 'bin'), { recursive: true });
    await writeFile(path.join(bundled, 'package.json'), JSON.stringify({ name: 'npm', version: '11.19.1', bin: { npm: 'bin/npm-cli.js' } }));
    await writeFile(path.join(bundled, 'bin/npm-cli.js'), '#!/usr/bin/env node\nprocess.exit(99);\n');
    assert.equal(await context.executableVersion('npm'), '11.19.1 (package metadata; command not run)');
    assert.equal(launches, 0);
  });
test('diagnostics reject an invalid inherited scratch context before any probe or mutation', async t => {
  const f = await fixture(t), before = await inventory(f.root);
  assert.throws(() => diagnosticEnvironment({ ...f.environment, GESTALT_SESSION_TMP_CONTEXT: '{}' }), /SESSION_SCRATCH_INVALID_CONTEXT/);
  const handlers = createDiagnosticHandlers(metadata, { launch: async () => { throw new Error('invalid context must not launch'); } });
  assert.equal(await runCli({ argv: ['path', 'codex-home'], cwd: f.root, environment: { ...f.environment, GESTALT_SESSION_TMP_CONTEXT: '{}' } }, f.streams, metadata, handlers), 1);
  assert.deepEqual(await inventory(f.root), before); assert.match(f.error(), /SESSION_SCRATCH_INVALID_CONTEXT/);
});
test('validated runtime-query environment preserves the real inherited v1 context and all TMP values without metadata writes',
  { skip: process.platform !== 'linux' ? 'Source-only Linux metadata validation; native ACL/compiled-peer integration is a separate target gate' : false }, async t => {
    const f = await fixture(t), scratch = await sessionScratch(f.root), environment = { ...f.environment, ...scratch.environment };
    const before = await inventory(f.root), selected = diagnosticEnvironment(environment);
    assert.deepEqual(JSON.parse(selected.GESTALT_SESSION_TMP_CONTEXT), scratch.context);
    for (const key of ['TMPDIR', 'TMP', 'TEMP']) assert.equal(selected[key], environment[key]);
    assert.deepEqual(await inventory(f.root), before); assert.deepEqual(environment, { ...f.environment, ...scratch.environment });
    // Context validation supplies no metadata-write or descendant-absence proof.
  });
test('required context-runtime verification refuses ABI/skew/source conflicts without repair', async t => {
  const f = await fixture(t), paths = await setupPaths(f.environment), context = new DiagnosticContext(paths, { argv: [], cwd: f.root, environment: f.environment }, f.streams);
  const runtime = path.join(paths.gestaltHome, 'runtime/context-mode'); await mkdir(runtime, { recursive: true });
  const marker = path.join(runtime, '.context-mode-prepared.json'); await writeFile(marker, JSON.stringify({ packageVersion: '2.26.0', platform: process.platform, arch: process.arch, nodeModulesAbi: 'wrong' }));
  context.inventory = async () => [{ name: 'gestalt', version: '2.26.0' }, { name: 'context-mode', version: '2.26.0', enabled: false }];
  await assert.rejects(verifyContextRuntime(context), /Node ABI/);
  const before = await inventory(f.root); await assert.rejects(verifyContextRuntime(context), /Node ABI/); assert.deepEqual(await inventory(f.root), before);
  context.inventory = async () => [{ name: 'gestalt', version: 'different' }, { name: 'context-mode', version: '2.26.0' }];
  // Exported session versions intentionally identify inherited packages. Without
  // them, exact native inventory versions govern skew detection.
  const plain = new DiagnosticContext(paths, { argv: [], cwd: f.root, environment: { HOME: f.root } }, f.streams); plain.inventory = context.inventory;
  await assert.rejects(verifyContextRuntime(plain), /version skew/);
});
