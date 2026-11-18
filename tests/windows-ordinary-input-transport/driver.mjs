import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, openSync } from 'node:fs';
import { appendFile, copyFile, mkdir, mkdtemp, readFile, realpath, rename, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { inspectPackedPackage } from './package-proof.mjs';

const repository = path.resolve(import.meta.dirname, '../..');
const evidence = path.resolve(process.argv[2] ?? '');
const relativeEvidence = path.relative(repository, evidence);
assert.equal(relativeEvidence, path.join('native-evidence', 'windows-ordinary-input'));
const inputs = JSON.parse(await readFile(path.join(import.meta.dirname, 'inputs.json'), 'utf8'));
const nativeNames = [
  'src/cli/platform/native/owned-process.c',
  'src/cli/platform/native/windows-storage-check.c',
  'src/cli/platform/native/windows-storage-check.h',
];
const expectedCases = ['empty-eof', 'small-binary-sequence'];
const excluded = [
  'unread or full-pipe input', 'controller crash or descendant lifetime',
  'forced stop, fault injection or terminal stress', 'full native contract suites',
  'installed lifecycle, Codex sessions, deployment or publication',
];
const now = () => new Date().toISOString();
const hash = (bytes, algorithm = 'sha256') => createHash(algorithm).update(bytes).digest('hex');
const record = async name => {
  const bytes = await readFile(name);
  return { path: name, bytes: bytes.length, sha256: hash(bytes) };
};
const save = async (name, value) => {
  const file = path.join(evidence, name), temp = file + '.tmp';
  await writeFile(temp, JSON.stringify(value, null, 2) + '\n');
  await rename(temp, file);
};
const proof = {
  schemaVersion: 1, state: 'started', startedAt: now(), platform: process.platform, arch: process.arch,
  node: process.version, runnerImage: process.env.ImageVersion ?? null,
  commands: [], cases: [], expectedCases, excluded, failure: null,
};
let work;
let keepWork = false;
await mkdir(evidence, { recursive: true });

function runtimeEnvironment() {
  const include = new Set(['PATH', 'PATHEXT', 'SYSTEMROOT', 'WINDIR', 'SYSTEMDRIVE', 'COMSPEC',
    'PROGRAMFILES', 'PROGRAMFILES(X86)', 'PROGRAMW6432', 'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH',
    'LOCALAPPDATA', 'APPDATA', 'TMP', 'TEMP']);
  return Object.fromEntries(Object.entries(process.env).filter(([name]) => include.has(name.toUpperCase())));
}

async function command(name, executable, args, { env = runtimeEnvironment(), cwd = repository, timeout = 90_000 } = {}) {
  const outPath = path.join(evidence, name + '.stdout');
  const errPath = path.join(evidence, name + '.stderr');
  const out = openSync(outPath, 'wx'), err = openSync(errPath, 'wx');
  const startedAt = now();
  let result;
  try { result = spawnSync(executable, args, { cwd, env, shell: false, windowsHide: true, timeout, stdio: ['ignore', out, err] }); }
  finally { closeSync(out); closeSync(err); }
  proof.commands.push({ name, argv: [executable, ...args], cwd, timeout, startedAt, observedAt: now(),
    exit: result.status, signal: result.signal, error: result.error?.message ?? null,
    stdout: await record(outPath), stderr: await record(errPath) });
  await save('commands.json', proof.commands);
  assert.equal(result.status, 0, name + ' failed; see retained command logs');
  return (await readFile(outPath, 'utf8')).trim();
}

async function npmTool() {
  const response = await fetch(inputs.npm.url, { redirect: 'error', signal: AbortSignal.timeout(45_000) });
  assert.ok(response.ok && response.body);
  const chunks = [];
  let size = 0;
  for await (const chunk of response.body) {
    size += chunk.byteLength;
    assert.ok(size <= inputs.npm.bytes, 'Pinned npm archive exceeds its recorded size');
    chunks.push(chunk);
  }
  const bytes = Buffer.concat(chunks);
  assert.equal(bytes.length, inputs.npm.bytes);
  assert.equal(hash(bytes, 'sha512'), inputs.npm.sha512);
  const archive = path.join(work, 'npm.tgz');
  await writeFile(archive, bytes);
  const root = path.join(work, 'npm-toolchain');
  await mkdir(root);
  assert.ok(path.isAbsolute(process.env.SystemRoot ?? ''));
  await command('extract-pinned-npm', path.join(process.env.SystemRoot, 'System32', 'tar.exe'), ['-xzf', archive, '-C', root]);
  const cli = path.join(root, 'package/bin/npm-cli.js');
  const npmrc = path.join(work, 'npmrc'), global = path.join(work, 'global-npmrc');
  await writeFile(npmrc, '');
  await writeFile(global, '');
  const env = { ...runtimeEnvironment(), npm_config_userconfig: npmrc, npm_config_globalconfig: global,
    npm_config_cache: path.join(work, 'npm-cache'), npm_config_update_notifier: 'false',
    npm_config_registry: 'https://registry.npmjs.org/' };
  const installedVersion = await command('pinned-npm-version', process.execPath, [cli, '--version'], { env });
  assert.equal(installedVersion, inputs.npm.version);
  proof.npm = { version: installedVersion, bytes: bytes.length, sha512: hash(bytes, 'sha512'), url: inputs.npm.url, cli: await record(cli) };
  return { cli, env };
}

async function readHelper(binary, sourceRoot) {
  const artifact = await record(binary);
  const bytes = await readFile(binary);
  assert.equal(bytes.toString('ascii', 0, 2), 'MZ');
  const offset = bytes.readUInt32LE(0x3C);
  assert.equal(bytes.readUInt32LE(offset), 0x4550, 'Windows PE signature');
  assert.equal(bytes.readUInt16LE(offset + 4), 0x8664, 'Windows x64 machine field');
  const provenanceFile = binary + '.provenance.json';
  const provenance = JSON.parse(await readFile(provenanceFile, 'utf8'));
  assert.equal(provenance.artifactSha256, artifact.sha256);
  assert.equal(provenance.platform, 'win32');
  assert.equal(provenance.architecture, 'x64');
  assert.equal(provenance.node, inputs.node);
  assert.equal(provenance.compiler, 'cl.exe');
  assert.ok(provenance.compilerVersion);
  assert.deepEqual(provenance.sourceInputs.map(item => item.path), nativeNames);
  for (const item of provenance.sourceInputs) {
    const source = await record(path.join(sourceRoot, item.path));
    assert.equal(item.sha256, source.sha256, item.path);
    assert.equal(item.bytes, source.bytes, item.path);
    if (sourceRoot === repository || item.path !== nativeNames[0]) assert.equal(item.sha256, inputs.sources[item.path]);
  }
  assert.equal(provenance.sourceSha256, provenance.sourceInputs[0].sha256);
  return { artifact, provenanceFile: await record(provenanceFile), provenance, machine: '8664' };
}

async function saveBuildProgress(source, name) {
  await copyFile(source, path.join(evidence, name)).catch(error => {
    if (error.code !== 'ENOENT') throw error;
  });
}

async function builds(npm) {
  const options = { env: { ...runtimeEnvironment(), GESTALT_NATIVE_BUILD_DIAGNOSTICS: '1' } };
  const directory = path.join(repository, 'dist/cli/native');
  const helper = path.join(directory, 'gestalt-owned-process.exe');
  try { await command('build-delivery-helper', process.execPath, ['scripts/build-owned-process.mjs'], options); }
  finally { await saveBuildProgress(path.join(directory, 'build-progress.jsonl'), 'delivery-compiler-progress.jsonl'); }
  proof.helper = await readHelper(helper, repository);
  assert.notEqual(proof.helper.artifact.sha256, inputs.formerWindowsHelperSha256);
  await copyFile(helper, path.join(evidence, 'gestalt-owned-process.exe'));
  await copyFile(helper + '.provenance.json', path.join(evidence, 'gestalt-owned-process.exe.provenance.json'));
  proof.helper.retained = await record(path.join(evidence, 'gestalt-owned-process.exe'));
  await save('result.json', proof);

  const fixture = path.join(work, 'compile-only');
  try { await command('build-only-copy-fixture', process.execPath, ['tests/windows-ordinary-input-transport/build-fixture.mjs', fixture], options); }
  finally { await saveBuildProgress(path.join(fixture, 'bin/build-progress.jsonl'), 'fixture-compiler-progress.jsonl'); }
  const binary = path.join(fixture, 'bin/gestalt-owned-process.exe');
  proof.buildOnly = await readHelper(binary, fixture);
  assert.notEqual(proof.buildOnly.provenance.sourceSha256, inputs.sources[nativeNames[0]]);
  proof.buildOnly.runtimeInvoked = false;
  await copyFile(binary + '.provenance.json', path.join(evidence, 'build-only-source-copy.provenance.json'));

  await command('typecheck-cli', process.execPath, ['node_modules/typescript/bin/tsc', '--project', 'tsconfig.cli.json']);
  await command('build-standalone-cli', process.execPath, ['scripts/build-cli.mjs']);
  await command('build-npm-package', process.execPath, ['scripts/build-npm-package.mjs']);
  const pack = JSON.parse(await command('pack-with-pinned-npm', process.execPath,
    [npm.cli, 'pack', './dist/npm', '--json', '--offline', '--ignore-scripts', '--pack-destination', evidence], { env: npm.env }));
  assert.equal(pack.length, 1);
  assert.equal(pack[0].filename, 'jaromil-gestalt-0.16.1.tgz');
  const tarball = path.join(evidence, pack[0].filename);
  const packageRoot = path.join(repository, 'dist/npm');
  const members = await inspectPackedPackage(tarball, npm.cli, packageRoot);
  const manifest = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));
  assert.equal(manifest.name, '@jaromil/gestalt');
  assert.equal(manifest.version, '0.16.1');
  assert.equal(manifest.gitHead, proof.source.head);
  assert.ok(manifest.os.includes('win32'));
  const expectedBytes = {
    LICENSE: inputs.sources['npm/LICENSE'],
    'README.md': inputs.sources['npm/README.md'],
    'postinstall.mjs': inputs.sources['npm/postinstall.mjs'],
    'bin/gestalt': inputs.sources['npm/gestalt'],
    'lib/legacy/gestalt': inputs.sources['public/gestalt'],
    'lib/gestalt.mjs': inputs.normalCliSha256,
    'lib/native/gestalt-owned-process.exe': proof.helper.artifact.sha256,
    'lib/native/gestalt-owned-process.exe.provenance.json': proof.helper.provenanceFile.sha256,
  };
  for (const member of members) if (Object.hasOwn(expectedBytes, member.name)) assert.equal(member.sha256, expectedBytes[member.name], member.name);
  const standalone = await record(path.join(repository, 'dist/cli/gestalt.mjs'));
  assert.equal(standalone.sha256, inputs.normalCliSha256);
  const source = await readFile(path.join(repository, nativeNames[0]), 'utf8');
  const license = source.match(/^\/\* Original Gestalt implementation\. SPDX-License-Identifier: MIT[\s\S]*?\*\//)?.[0];
  assert.ok(license);
  assert.equal(members.find(item => item.name === 'lib/native/LICENSE.txt').sha256, hash(license + '\n'));
  proof.package = { tarball: await record(tarball), integrity: pack[0].integrity, manifest, standalone, members };
  await save('result.json', proof);
  return helper;
}

function bounded(operation, label) {
  let timer;
  return Promise.race([operation, new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error(label + ' exceeded the ordinary observation deadline')), 30_000);
  })]).finally(() => clearTimeout(timer));
}

async function ordinaryCase(adapter, configuration) {
  const { name, chunks, argv, exitCode } = configuration;
  const input = Buffer.concat(chunks);
  assert.ok(input.length <= 4096 && chunks.length <= 3);
  const directory = path.join(work, name + ' cwd é');
  await mkdir(directory);
  const cwd = await realpath(directory);
  const env = { ...runtimeEnvironment(), ORDINARY_INPUT_OPERATOR: 'kept é' };
  const target = path.join(import.meta.dirname, 'target.mjs');
  const stdout = path.join(evidence, name + '.stdout.bin'), stderr = path.join(evidence, name + '.stderr.bin');
  const inputFile = path.join(evidence, name + '.stdin.bin');
  await writeFile(inputFile, input); await writeFile(stdout, ''); await writeFile(stderr, '');
  const row = {
    name, status: 'started', startedAt: now(), cwd, command: [process.execPath, target, String(exitCode), ...argv],
    chunkSizes: chunks.map(chunk => chunk.length), expectedExit: exitCode, stdin: await record(inputFile), runtimeSettled: false,
  };
  proof.cases.push(row);
  await save('cases.json', proof.cases);
  let outSize = 0, errSize = 0;
  try {
    const lease = await bounded(adapter.launch(process.execPath, {
      argv: [target, String(exitCode), ...argv], cwd, environment: env,
    }, {
      stdin: (async function* () { for (const bytes of chunks) yield bytes; })(),
      stdout: async bytes => { outSize += bytes.length; assert.ok(outSize <= 4096); await appendFile(stdout, bytes); },
      stderr: async bytes => { errSize += bytes.length; assert.ok(errSize <= 4096); await appendFile(stderr, bytes); },
    }), name + ' launch');
    [row.result, row.claim] = await bounded(Promise.all([lease.result, lease.completed]), name + ' settlement');
    row.runtimeSettled = true;
    assert.deepEqual(row.result, { kind: 'exit', code: exitCode });
    assert.equal(row.claim.kind, 'released');
    assert.equal(row.claim.backend, 'windows-job');
    assert.deepEqual(row.claim.result, row.result);
    assert.deepEqual(await readFile(stdout), input);
    const metadata = JSON.parse(await readFile(stderr, 'utf8'));
    assert.deepEqual(metadata, { eof: true, bytes: input.length, argv, cwd, operator: env.ORDINARY_INPUT_OPERATOR });
    row.status = 'pass';
  } catch (error) {
    keepWork ||= !row.runtimeSettled || row.claim?.kind !== 'released';
    row.status = 'fail';
    row.failure = { name: error.name, message: error.message, stack: error.stack };
  } finally {
    row.stdout = await record(stdout); row.stderr = await record(stderr); row.observedAt = now();
    await save('cases.json', proof.cases);
  }
}

try {
  assert.equal(process.platform + '-' + process.arch, 'win32-x64');
  assert.equal(process.version, inputs.node);
  const head = await command('checkout-head', 'git', ['rev-parse', 'HEAD']);
  assert.equal(head, process.env.GITHUB_SHA);
  assert.equal(await command('checkout-parent', 'git', ['rev-parse', 'HEAD^']), inputs.managerBase);
  const tree = await command('checkout-tree', 'git', ['rev-parse', 'HEAD^{tree}']);
  const files = [];
  for (const [relative, expected] of Object.entries(inputs.sources)) {
    const file = await record(path.join(repository, relative));
    assert.equal(file.sha256, expected, relative);
    files.push({ relative, ...file });
  }
  proof.source = { head, tree, parent: inputs.managerBase, files };
  const physicalTemporary = await realpath(tmpdir());
  work = await mkdtemp(path.join(physicalTemporary, 'gestalt-ordinary-input-'));
  const npm = await npmTool();
  await command('lockfile-local-npm-ci', process.execPath, [npm.cli, 'ci', '--ignore-scripts', '--no-audit', '--no-fund'], { env: npm.env, timeout: 180_000 });
  assert.equal((await record(path.join(repository, 'package-lock.json'))).sha256, inputs.sources['package-lock.json']);
  const helper = await builds(npm);
  const { NativeOwnedProcesses } = await import('../../src/cli/platform/processes.ts');
  const adapter = new NativeOwnedProcesses(helper);
  await ordinaryCase(adapter, { name: expectedCases[0], chunks: [], argv: [], exitCode: 0 });
  if (!keepWork) await ordinaryCase(adapter, {
    name: expectedCases[1], chunks: [Buffer.from([0, 255, 13, 10]), Buffer.from('é 東京'), Buffer.from([129, 0, 50, 254])],
    argv: ['', 'space é 東京', '"quoted"', 'last\\'], exitCode: 23,
  });
  assert.deepEqual(proof.cases.filter(row => row.status === 'pass').map(row => row.name), expectedCases);
  proof.state = 'pass';
} catch (error) {
  proof.state = 'fail';
  proof.failure = { name: error.name, message: error.message, stack: error.stack };
  process.exitCode = 1;
} finally {
  proof.cleanup = { attempted: false, removed: false, path: work ?? null, normalReleaseUnverified: keepWork };
  if (work && !keepWork) {
    proof.cleanup.attempted = true;
    try { await rm(work, { recursive: true }); proof.cleanup.removed = true; }
    catch (error) {
      proof.cleanup.error = error.message;
      proof.state = 'fail'; process.exitCode = 1;
    }
  }
  proof.finishedAt = now();
  proof.unexecutedCases = expectedCases.filter(name => !proof.cases.some(row => row.name === name));
  await save('result.json', proof);
  if (proof.failure) console.error(proof.failure.message);
}
