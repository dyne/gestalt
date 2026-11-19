import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { copyFile, mkdir, realpath, writeFile } from 'node:fs/promises';
import { release } from 'node:os';
import path from 'node:path';
import { pinnedCodex } from '../fixtures/native-runtime/codex.mjs';
import { capturedNodeCommand } from '../fixtures/native-node-command.mjs';
import { checkPack, digest, errorInfo, gitInventory, inputs, json, now, phases, root, save, npm12Inventory, verifyAcceptedPrHelper } from './common.mjs';

const lane = process.argv[2];
assert.equal(lane, 'windows-consumer');
const output = path.join(root, 'native-evidence/final-current');
const evidence = path.join(output, lane);
const agents = path.join(root, '.native-dependencies/agents');
const supplied = path.join(output, 'input');
const state = { schemaVersion: 1, lane, state: 'started', startedAt: now(), platform: process.platform, arch: process.arch,
  node: process.version, abi: process.versions.modules, imageVersion: process.env.ImageVersion ?? null, imageOS: process.env.ImageOS ?? null,
  kernel: release(), head: process.env.GITHUB_SHA ?? null, run: process.env.GITHUB_RUN_ID ?? null, attempt: process.env.GITHUB_RUN_ATTEMPT ?? null,
  groups: [], commands: [], failure: null };
await mkdir(evidence, { recursive: true });
const allowed = /^(PATH|PATHEXT|SYSTEMROOT|WINDIR|COMSPEC|PROGRAMFILES(?:\(X86\))?|PROGRAMW6432|PROGRAMDATA|SYSTEMDRIVE|USERPROFILE|LOCALAPPDATA|APPDATA|HOMEDRIVE|HOMEPATH|TMP|TEMP|TMPDIR|LANG|LC_ALL|TERM|TZ|CI|GITHUB_ACTIONS)$/i;
const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => allowed.test(key)));
const { group, command, record } = phases(evidence, state, environment);
const toolchain = path.join(evidence, 'private-toolchain');
const packageSource = path.join(toolchain, 'source');
let npm, helper, packageBinding;

async function verifySources() {
  const managerHead = (await command('manager-head', 'git', ['rev-parse', 'HEAD'])).text.trim();
  assert.equal(managerHead, state.head);
  assert.equal((await command('manager-parent', 'git', ['rev-parse', 'HEAD^'])).text.trim(), inputs.managerBase);
  state.tree = (await command('manager-tree', 'git', ['rev-parse', 'HEAD^{tree}'])).text.trim();
  const paths = (await command('manager-test-paths', 'git', ['diff', '--name-only', inputs.managerBase, managerHead])).text.trim().split(/\r?\n/);
  assert.deepEqual(paths.sort(), [...inputs.overlayPaths].sort());
  const entries = await gitInventory(root, 'manager', command, evidence);
  const replacedWorkflow = '.github/workflows/native-core-slice.yml';
  assert.ok(inputs.overlayPaths.includes(replacedWorkflow));
  assert.equal(entries.length, inputs.sourceLeafCount + inputs.overlayPaths.length - 1);
  assert.equal((await command('agents-head', 'git', ['rev-parse', 'HEAD'], { cwd: agents })).text.trim(), inputs.agentsSnapshot);
  assert.equal((await command('agents-tree', 'git', ['rev-parse', 'HEAD^{tree}'], { cwd: agents })).text.trim(), inputs.agentsTree);
  assert.equal((await gitInventory(agents, 'agents', command, evidence)).length, 755);
  await command('exact-source-worktree', 'git', ['-c', 'core.autocrlf=false', 'worktree', 'add', '--detach', packageSource, inputs.managerBase]);
  assert.equal((await command('package-source-head', 'git', ['rev-parse', 'HEAD'], { cwd: packageSource })).text.trim(), inputs.managerBase);
  assert.equal((await command('package-source-tree', 'git', ['rev-parse', 'HEAD^{tree}'], { cwd: packageSource })).text.trim(), inputs.sourceTree);
  const source = await gitInventory(packageSource, 'package-source', command, evidence);
  assert.equal(source.length, inputs.sourceLeafCount);
  for (const [name, digest] of Object.entries(inputs.sourceDigests)) {
    assert.equal(source.find(row => row.name === name)?.sha256, digest, name);
  }
  assert.deepEqual(source.filter(row => row.name !== replacedWorkflow),
    entries.filter(row => !inputs.overlayPaths.includes(row.name)));
  assert.equal((await command('package-source-clean', 'git', ['status', '--porcelain'], { cwd: packageSource })).text.trim(), '');
  return { managerLeaves: entries.length, packageSourceLeaves: source.length, sourceHead: inputs.managerBase,
    sourceTree: inputs.sourceTree, agentsLeaves: 755, agentsTree: inputs.agentsTree };
}

async function prepareNpm() {
  const bin = path.join(toolchain, 'bin'), archive = path.join(toolchain, 'npm.tgz');
  await mkdir(bin, { recursive: true, mode: 0o700 });
  const response = await fetch(inputs.npm.url, { redirect: 'error', signal: AbortSignal.timeout(60_000) });
  assert.ok(response.ok && response.body);
  const chunks = []; let size = 0;
  for await (const chunk of response.body) { size += chunk.byteLength; assert.ok(size <= inputs.npm.bytes); chunks.push(chunk); }
  assert.equal(size, inputs.npm.bytes);
  const bytes = Buffer.concat(chunks);
  assert.equal(createHash('sha512').update(bytes).digest('hex'), inputs.npm.sha512);
  await writeFile(archive, bytes, { mode: 0o600 });
  const unpacked = path.join(toolchain, 'unpacked');
  await mkdir(unpacked, { mode: 0o700 });
  const win = process.platform === 'win32';
  const tar = win ? path.join(process.env.SystemRoot, 'System32/tar.exe') : '/usr/bin/tar';
  await command('extract-frozen-npm', tar, ['-xzf', archive, '-C', unpacked]);
  const npmRoot = win ? path.join(bin, 'node_modules/npm') : path.join(unpacked, 'package');
  if (win) {
    const { rename } = await import('node:fs/promises');
    await mkdir(path.dirname(npmRoot), { recursive: true });
    await rename(path.join(unpacked, 'package'), npmRoot);
    await copyFile(process.execPath, path.join(bin, 'node.exe'));
    const capture = await json(path.join(root, 'tests/fixtures/native-npm-node-shim.json'));
    await writeFile(path.join(bin, 'npm.cmd'), capturedNodeCommand(capture, 'npm-entry.mjs'));
    await writeFile(path.join(bin, 'npm-entry.mjs'), "#!/usr/bin/env node\nimport { fileURLToPath } from 'node:url';\nconst cli = new URL('./node_modules/npm/bin/npm-cli.js', import.meta.url);\nprocess.argv[1] = fileURLToPath(cli);\nawait import(cli);\n");
  }
  npm = path.join(npmRoot, 'bin/npm-cli.js');
  if (!win) await symlink(npm, path.join(bin, 'npm'));
  const userConfig = path.join(toolchain, 'npmrc');
  await writeFile(userConfig, '', { mode: 0o600 });
  Object.assign(environment, { HOME: toolchain, PATH: [bin, path.dirname(process.execPath), environment.PATH ?? environment.Path].join(path.delimiter),
    npm_config_cache: path.join(toolchain, 'npm-cache'), npm_config_userconfig: userConfig,
    npm_config_globalconfig: win ? 'NUL' : '/dev/null', npm_config_update_notifier: 'false',
    npm_config_registry: 'https://registry.npmjs.org/', FORCE_COLOR: '0', GIT_TERMINAL_PROMPT: '0', GIT_CONFIG_NOSYSTEM: '1' });
  for (const key of Object.keys(environment)) if (key !== 'PATH' && key.toLowerCase() === 'path') delete environment[key];
  const ver = await npmCommand('pinned-npm-version', ['--version']);
  assert.equal(ver.text.trim(), inputs.npm.version);
  await npmCommand('lockfile-local-npm-ci', ['ci', '--ignore-scripts', '--no-audit', '--no-fund'], { cwd: packageSource });
  assert.equal((await digest(path.join(packageSource, 'package-lock.json'))).sha256, inputs.sourceDigests['package-lock.json']);
  const lookup = "import{realpathSync}from'node:fs';import{fileURLToPath}from'node:url';console.log(JSON.stringify(['esbuild','smol-toml','typescript','@types/node/package.json','@esbuild/'+process.platform+'-'+process.arch+'/package.json'].map(x=>realpathSync(fileURLToPath(import.meta.resolve(x))))));";
  const modules = JSON.parse((await command('lockfile-module-realpaths', process.execPath, ['--input-type=module', '-e', lookup], { cwd: packageSource })).text);
  const modulesRoot = await realpath(path.join(packageSource, 'node_modules'));
  for (const file of modules) assert.ok(!path.relative(modulesRoot, file).startsWith('..') && !path.isAbsolute(path.relative(modulesRoot, file)));
  return { npmCli: await digest(npm), archive: await digest(archive), sha512: inputs.npm.sha512, modulePaths: modules };
}
const npmCommand = (name, args, options) => command(name, process.execPath, [npm, ...args], options);

async function prepareNpm12() {
  const frozen = inputs.npm12;
  const archive = path.join(toolchain, 'npm12.tgz'), bin = path.join(toolchain, 'bin'), npmRoot = path.join(bin, 'npm12');
  const response = await fetch(frozen.url, { redirect: 'error', signal: AbortSignal.timeout(60_000) });
  assert.ok(response.ok && response.body);
  const chunks = [];
  let length = 0;
  for await (const chunk of response.body) {
    length += chunk.byteLength;
    assert.ok(length <= frozen.bytes);
    chunks.push(chunk);
  }
  const payload = Buffer.concat(chunks);
  assert.equal(length, frozen.bytes);
  assert.equal(createHash('sha512').update(payload).digest('hex'), frozen.sha512);
  await writeFile(archive, payload, { flag: 'wx', mode: 0o600 });
  const artifact = await digest(archive);
  assert.equal(artifact.sha256, frozen.sha256);
  await mkdir(npmRoot);
  const tar = path.join(process.env.SystemRoot, 'System32/tar.exe');
  await command('extract-frozen-npm12', tar, ['-xzf', archive, '-C', npmRoot]);
  const inventory = await npm12Inventory(await realpath(npmRoot));
  await save(path.join(evidence, 'npm12-inventory.json'), inventory);
  const entry = await digest(path.join(bin, frozen.relativeEntry));
  assert.equal(entry.sha256, frozen.entrySha256);
  const version = await command('builder-npm12-version', process.execPath, [entry.path, '--version']);
  assert.equal(version.text.trim(), frozen.version);
  return { archive: artifact, sha512: frozen.sha512, entry, version: version.text.trim(),
    inventory: { leaves: inventory.leaves, sha256: inventory.sha256 } };
}

async function stageHelper() {
  helper = await verifyAcceptedPrHelper(supplied);
  const native = path.join(packageSource, 'dist/cli/native');
  await mkdir(native, { recursive: true });
  await copyFile(helper.helper.path, path.join(native, 'gestalt-owned-process.exe'));
  await copyFile(helper.provenance.path, path.join(native, 'gestalt-owned-process.exe.provenance.json'));
  assert.equal((await verifyAcceptedPrHelper(native)).helper.sha256, helper.helper.sha256);
  return helper;
}
async function preparePackage() {
  await npmCommand('build-package', ['run', 'build:package'], { cwd: packageSource });
  const destination = path.join(evidence, 'package'); await mkdir(destination, { recursive: true });
  const output = JSON.parse((await npmCommand('pack-draft-package', ['pack', './dist/npm', '--offline', '--ignore-scripts', '--json',
    '--pack-destination', destination], { cwd: packageSource })).text);
  assert.equal(output.length, 1); assert.equal(output[0].filename, 'jaromil-gestalt-0.16.1.tgz');
  packageBinding = await checkPack(path.join(destination, output[0].filename), npm, path.join(packageSource, 'dist/npm'), inputs.managerBase, helper);
  await save(path.join(evidence, 'package-binding.json'), packageBinding);
  return packageBinding;
}

try {
  assert.equal(process.env.FINAL_RUNNER_ENVIRONMENT, 'github-hosted');
  assert.equal(process.env.GITHUB_ACTIONS, 'true');
  assert.equal(process.platform, 'win32');
  assert.equal(process.arch, 'x64');
  assert.equal(process.version, inputs.node);
  assert.ok(state.imageVersion && state.run && state.attempt);
  await mkdir(toolchain, { recursive: true, mode: 0o700 });
  await group('source', [], verifySources);
  await group('frozen-npm-and-lockfile', ['source'], prepareNpm);
  await group('frozen-npm12', ['frozen-npm-and-lockfile'], prepareNpm12);
  await group('accepted-pr-helper', ['frozen-npm12'], stageHelper);
  await group('package-build', ['frozen-npm-and-lockfile', 'accepted-pr-helper'], preparePackage);
  await group('prepare-consumer', ['package-build', 'frozen-npm12'], async () => {
    const codexDir = path.join(toolchain, 'codex-pinned');
    const codex = await pinnedCodex(codexDir);
    const toolBin = path.join(toolchain, 'bin');
    const absoluteCodex = path.join(toolBin, 'codex.exe');
    await copyFile(codex.binary, absoluteCodex);
    assert.equal((await digest(absoluteCodex)).sha256, codex.record.binary.sha256);
    await copyFile(path.join(codexDir, 'artifact.json'), path.join(evidence, 'codex-artifact.json'));
    const output = path.join(evidence, 'prebuilt');
    await command('prepare-prebuilt-consumer', process.execPath,
      ['tests/native-core-slice.mjs', '--prepare', output, '--agents', agents], { timeout: 240_000, cwd: packageSource,
        env: { ...environment, GESTALT_CORE_SLICE_NATIVE_ARTIFACTS: supplied, GESTALT_CORE_SLICE_PACKAGE: '1',
          GESTALT_CORE_SLICE_NPM_TARBALL: packageBinding.tarball.path, GESTALT_CORE_SLICE_NPM_SOURCE_HEAD: inputs.managerBase,
          GESTALT_CORE_SLICE_NPM_SHA256: packageBinding.tarball.sha256 } });
    const artifacts = await json(path.join(output, 'artifacts.json'));
    assert.equal(artifacts.package.sha256, packageBinding.tarball.sha256);
    assert.equal(artifacts.artifacts.find(item => item.relative === artifacts.helper).sha256, helper.helper.sha256);
    return { prebuilt: await digest(path.join(output, 'artifacts.json')), codex: await digest(absoluteCodex),
      node: await digest(path.join(toolBin, 'node.exe')), npm: await digest(npm),
      npmShim: await digest(path.join(toolBin, 'npm.cmd')), npmEntry: await digest(path.join(toolBin, 'npm-entry.mjs')) };
  });
  assert.ok(state.groups.every(row => row.state === 'pass'), 'Required verification group failed or remained unexecuted');
  state.state = 'pass';
} catch (error) { state.state = 'fail'; state.failure = errorInfo(error); process.exitCode = 1; }
finally {
  state.finishedAt = now();
  await record();
  if (state.failure) console.error(state.failure.message);
}
