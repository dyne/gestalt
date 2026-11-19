import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFile, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { commandTarget, NativeExecutables } from '../src/cli/platform/executables.ts';
import { installPackedCore, npmPrerequisiteTarget, windowsPackedShimEnvironment } from './fixtures/packed-core.mjs';
import { capturedNodeCommand } from './fixtures/native-node-command.mjs';

// Authentic npm11.19.1 bin/npm.cmd, read from the installed npm package on
// 2026-11-18. Its538 bytes/SHA match failed Windows run44145068080 exactly.
const original = await readFile(new URL('./fixtures/npm-prerequisite-11.cmd', import.meta.url));
const expectedHash = '21b46c69ad6e2f231f02a9e120f4ba6c8e75fef5a45637103002eab99f888ab8';
async function fixture(t) {
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'npm prerequisite ü ')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const node = path.join(root, 'node.exe'), npm = path.join(root, 'npm.cmd');
  const npmRoot = path.join(root, 'node_modules/npm'), entry = path.join(npmRoot, 'bin/npm-cli.js');
  await mkdir(path.dirname(entry), { recursive: true });
  await writeFile(node, 'resolution fixture only; never executed');
  await copyFile(new URL('./fixtures/npm-prerequisite-11.cmd', import.meta.url), npm);
  const manifest = { name: 'npm', version: '11.19.1', bin: { npm: 'bin/npm-cli.js' } };
  await writeFile(path.join(npmRoot, 'package.json'), JSON.stringify(manifest));
  await writeFile(entry, '#!/usr/bin/env node\nthrow Error("Resolution fixture must never execute");\n');
  return { root, node, npm, npmRoot, entry, manifest };
}

test('authentic failing bundled npm wrapper reproduces generic rejection and resolves only to its verified sibling Node entry', async t => {
  assert.equal(original.length, 538);
  assert.equal(createHash('sha256').update(original).digest('hex'), expectedHash);
  const f = await fixture(t), receipt = {};
  await assert.rejects(commandTarget(f.npm), /Unsupported batch shim/);
  assert.deepEqual(await npmPrerequisiteTarget(f.npm, receipt, f.node), { executable: f.node, prefix: [f.entry] });
  assert.equal(receipt.kind, 'verified-bundled-npm11-node-entry');
  assert.equal(receipt.selectedBytes.sha256, expectedHash);
  assert.equal(receipt.batchSource, original.toString('utf8'));
  assert.equal(receipt.package.version, '11.19.1');
  assert.equal(receipt.entry.path, f.entry);
});

test('modified bundled batch programs remain rejected by the unchanged generic recognizer', async t => {
  const f = await fixture(t);
  for (const change of [Buffer.concat([original, Buffer.from('echo injected\r\n')]), Buffer.from(original.toString().replace('NPM_PREFIX_JS', 'OTHER_PREFIX_JS'))]) {
    await writeFile(f.npm, change);
    await assert.rejects(npmPrerequisiteTarget(f.npm, {}, f.node), /Unsupported batch shim/);
  }
});

test('generated package Node shims retain the existing generic target contract', async t => {
  const f = await fixture(t);
  const captured = JSON.parse(await readFile(new URL('./fixtures/native-npm-node-shim.json', import.meta.url)));
  const shim = path.join(f.root, 'tool.cmd'), entry = path.join(f.root, 'tool.mjs');
  await writeFile(shim, capturedNodeCommand(captured, 'tool.mjs'));
  await writeFile(entry, '#!/usr/bin/env node\n');
  const receipt = {};
  assert.deepEqual(await npmPrerequisiteTarget(shim, receipt), { executable: process.execPath, prefix: [entry] });
  assert.equal(receipt.kind, 'existing-generic-command-target');
});

test('known wrapper cannot select a Node runtime from another distribution', async t => {
  const f = await fixture(t);
  await assert.rejects(npmPrerequisiteTarget(f.npm, {}, process.execPath), /adjacent to the selected Node runtime/);
});

test('bundled target requires npm11 identity and its fixed npm-cli entry', async t => {
  const f = await fixture(t);
  for (const change of [{ name: 'other' }, { version: '12.0.0' }, { bin: { npm: '../arbitrary.js' } }]) {
    await writeFile(path.join(f.npmRoot, 'package.json'), JSON.stringify({ ...f.manifest, ...change }));
    await assert.rejects(npmPrerequisiteTarget(f.npm, {}, f.node));
  }
});

test('bundled npm entry must exist and carry a plain Node shebang', async t => {
  const f = await fixture(t);
  await writeFile(f.entry, '#!/bin/sh\nexit 0\n');
  await assert.rejects(npmPrerequisiteTarget(f.npm, {}, f.node));
  await rm(f.entry);
  await assert.rejects(npmPrerequisiteTarget(f.npm, {}, f.node), { code: 'ENOENT' });
});

test('Windows literal packed shim keeps the toolchain after the owned prefix for PATH and Path', () => {
  const prefix = String.raw`C:\Owned fixture ü &\npm prefix`;
  const configured = String.raw`D:\tools;C:\Program Files\Git\cmd;C:\Windows\System32`;
  for (const key of ['PATH', 'Path', 'path']) {
    const original = { [key]: configured, PATHEXT: '.COM;.EXE;.BAT;.CMD', TEMP: 'unchanged' };
    const child = windowsPackedShimEnvironment(original, prefix);
    assert.deepEqual(child, { PATH: prefix + ';' + configured, PATHEXT: '.COM;.EXE;.BAT;.CMD', TEMP: 'unchanged' });
    assert.deepEqual(original, { [key]: configured, PATHEXT: '.COM;.EXE;.BAT;.CMD', TEMP: 'unchanged' });
  }
});

test('Windows literal packed shim rejects ambiguous and missing path values', () => {
  const prefix = String.raw`C:\Owned fixture\npm prefix`;
  assert.throws(() => windowsPackedShimEnvironment({}, prefix), /exactly one PATH key/);
  assert.throws(() => windowsPackedShimEnvironment({ PATH: 'tools', Path: 'tools' }, prefix), /exactly one PATH key/);
  for (const value of [undefined, '', '  ']) {
    assert.throws(() => windowsPackedShimEnvironment({ Path: value }, prefix), /nonempty PATH/);
  }
});

test('prerequisite discovery failure is preserved before any installation command runs', async t => {
  const f = await fixture(t);
  await assert.rejects(installPackedCore('never-installed.tgz', f.root, { PATH: '' }, 'never-read-agents', f.root), /primary supported npm lane/);
  assert.deepEqual(JSON.parse(await readFile(path.join(f.root, 'packed-install-commands.json'))), []);
  const report = JSON.parse(await readFile(path.join(f.root, 'packed-prerequisite.json')));
  assert.match(report.failure.message, /primary supported npm lane/);
  assert.match(report.failure.stack, /installPackedCore/);
});

test('existing Unix npm prerequisite still executes directly with literal arguments', { skip: process.platform === 'win32' }, async () => {
  const selected = await new NativeExecutables().resolve('npm', process.env);
  assert.ok(selected); const npm = await realpath(selected), receipt = {};
  const target = await npmPrerequisiteTarget(npm, receipt);
  const result = spawnSync(target.executable, [...target.prefix, '--version'], { encoding: 'utf8', shell: false });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /^\d+\.\d+\.\d+/);
  assert.equal(receipt.kind, 'existing-generic-command-target');
});
