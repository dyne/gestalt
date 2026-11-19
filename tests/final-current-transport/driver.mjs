import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { copyFile, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir, release } from 'node:os';
import path from 'node:path';
import { pinnedCodex } from '../fixtures/native-runtime/codex.mjs';
import { capturedNodeCommand } from '../fixtures/native-node-command.mjs';
import { checkPack, digest, errorInfo, gitInventory, inputs, json, now, phases, root, save, testCounts, verifyHelper, verifySameRun } from './common.mjs';

const lane = process.argv[2];
assert.ok(['windows-producer', 'linux-full', 'windows-consumer'].includes(lane));
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
let npm, toolchain, helper, packageBinding;

async function verifySources() {
  const managerHead = (await command('manager-head', 'git', ['rev-parse', 'HEAD'])).text.trim();
  assert.equal(managerHead, state.head);
  assert.equal((await command('manager-parent', 'git', ['rev-parse', 'HEAD^'])).text.trim(), inputs.managerBase);
  state.tree = (await command('manager-tree', 'git', ['rev-parse', 'HEAD^{tree}'])).text.trim();
  const paths = (await command('manager-test-paths', 'git', ['diff', '--name-only', inputs.managerBase, managerHead])).text.trim().split(/\r?\n/);
  assert.deepEqual(paths.sort(), [...inputs.sourcePaths, ...inputs.overlayPaths].sort());
  const entries = await gitInventory(root, 'manager', command, evidence);
  assert.equal(entries.length, 303 + inputs.overlayPaths.length);
  for (const [name, digest] of Object.entries(inputs.sourceDigests)) assert.equal(entries.find(row => row.name === name)?.sha256, digest, name);
  assert.equal((await command('agents-head', 'git', ['rev-parse', 'HEAD'], { cwd: agents })).text.trim(), inputs.agentsSnapshot);
  assert.equal((await command('agents-tree', 'git', ['rev-parse', 'HEAD^{tree}'], { cwd: agents })).text.trim(), inputs.agentsTree);
  assert.equal((await gitInventory(agents, 'agents', command, evidence)).length, 755);
  return { managerLeaves: entries.length, agentsLeaves: 755, agentsTree: inputs.agentsTree };
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
  await npmCommand('lockfile-local-npm-ci', ['ci', '--ignore-scripts', '--no-audit', '--no-fund']);
  assert.equal((await digest(path.join(root, 'package-lock.json'))).sha256, inputs.sourceDigests['package-lock.json']);
  const lookup = "import{realpathSync}from'node:fs';import{fileURLToPath}from'node:url';console.log(JSON.stringify(['esbuild','smol-toml','typescript','@types/node/package.json','@esbuild/'+process.platform+'-'+process.arch+'/package.json'].map(x=>realpathSync(fileURLToPath(import.meta.resolve(x))))));";
  const modules = JSON.parse((await command('lockfile-module-realpaths', process.execPath, ['--input-type=module', '-e', lookup])).text);
  const modulesRoot = await realpath(path.join(root, 'node_modules'));
  for (const file of modules) assert.ok(!path.relative(modulesRoot, file).startsWith('..') && !path.isAbsolute(path.relative(modulesRoot, file)));
  return { npmCli: await digest(npm), archive: await digest(archive), sha512: inputs.npm.sha512, modulePaths: modules };
}
const npmCommand = (name, args, options) => command(name, process.execPath, [npm, ...args], options);

async function stageHelper() {
  helper = await verifySameRun(supplied, state.head, state.tree);
  const native = path.join(root, 'dist/cli/native'); await mkdir(native, { recursive: true });
  await copyFile(helper.helper.path, path.join(native, 'gestalt-owned-process.exe'));
  await copyFile(helper.provenance.path, path.join(native, 'gestalt-owned-process.exe.provenance.json'));
  assert.deepEqual(await verifyHelper(native).then(x => x.source), helper.source);
  return helper;
}
async function verifyLinuxEnvironment() {
  const os = await readFile('/etc/os-release', 'utf8');
  await writeFile(path.join(evidence, 'os-release.txt'), os);
  assert.match(os, /^ID="?ubuntu"?$/m);
  assert.match(os, /^VERSION_ID="?22\.04"?$/m);
  const flags = {};
  for (const key of ['unprivileged_userns_clone', 'apparmor_restrict_unprivileged_userns'])
    flags[key] = await readFile('/proc/sys/kernel/' + key, 'utf8').then(s => s.trim(), err => { if (err.code === 'ENOENT') return null; throw err; });
  await save(path.join(evidence, 'host-policy.json'), { flags, altered: false, kernel: release(), observedAt: now() });
  assert.notEqual(flags.unprivileged_userns_clone, '0');
  assert.notEqual(flags.apparmor_restrict_unprivileged_userns, '1');
  return { flags, policyAltered: false };
}
async function verifySandbox() {
  const sandboxHome = await mkdtemp(path.join(toolchain, 'sandbox-home-'));
  const codex = path.join(root, 'node_modules/.bin/codex');
  const version = (await command('locked-codex-version', codex, ['--version'])).text.trim();
  assert.equal(version, 'codex-cli 0.160.0');
  const tested = await command('native-sandbox-preflight', codex,
    ['-c', 'default_permissions=":read-only"', 'sandbox', '--', '/bin/true'], { env: { ...environment, CODEX_HOME: sandboxHome } });
  return { executable: await digest(await realpath(codex)), version, command: tested.row.name };
}
async function fullShard(number) {
  const name = 'manager-shard-' + number, shard = number + '/2';
  const result = await npmCommand(name, ['test', '--', '--shard', shard], { timeout: 1_020_000, required: false });
  const counts = testCounts(result.text);
  const terminal = 'tests: focused shell tests passed (shard ' + shard + ')';
  const receipt = { blocks: counts, skippedCases: result.text.split(/\r?\n/).filter(line => /#\s*SKIP|^﹣ /.test(line)),
    completionMarker: result.text.includes(terminal), command: result.row.name };
  await save(path.join(evidence, name + '-coverage.json'), receipt);
  assert.equal(result.row.exit, 0); assert.equal(counts.length, inputs.linuxCommands[name]); assert.ok(receipt.completionMarker);
  assert.ok(counts.every(c => c.tests > 0 && c.pass > 0 && c.fail === 0 && c.cancelled === 0));
  return receipt;
}
async function exactNpmGroup(name, script, count) {
  const result = await npmCommand(name, ['run', script], { timeout: 300_000 });
  const counts = testCounts(result.text);
  assert.deepEqual(counts, [{ tests: count, pass: count, fail: 0, cancelled: 0, skipped: 0, todo: 0 }]);
  return { counts, command: result.row.name };
}
async function preparePackage() {
  await npmCommand('build-package', ['run', 'build:package']);
  const destination = path.join(evidence, 'package'); await mkdir(destination, { recursive: true });
  const output = JSON.parse((await npmCommand('pack-draft-package', ['pack', './dist/npm', '--offline', '--ignore-scripts', '--json',
    '--pack-destination', destination])).text);
  assert.equal(output.length, 1); assert.equal(output[0].filename, 'jaromil-gestalt-0.16.1.tgz');
  packageBinding = await checkPack(path.join(destination, output[0].filename), npm, path.join(root, 'dist/npm'), state.head, helper);
  await save(path.join(evidence, 'package-binding.json'), packageBinding);
  return packageBinding;
}

try {
  assert.equal(process.env.FINAL_RUNNER_ENVIRONMENT, 'github-hosted');
  assert.equal(process.arch, 'x64'); assert.equal(process.version, inputs.node);
  assert.equal(process.platform, lane === 'linux-full' ? 'linux' : 'win32');
  assert.ok(state.imageVersion && state.run && state.attempt);
  await group('source', [], verifySources);
  if (lane === 'windows-producer') {
    await group('build-source-bound-msvc-helper', ['source'], async () => {
      await command('build-helper', process.execPath, ['scripts/build-owned-process.mjs']);
      const target = await verifyHelper(path.join(root, 'dist/cli/native'));
      const producer = path.join(output, 'producer'); await mkdir(producer, { recursive: true });
      for (const item of [target.helper, target.provenance]) await copyFile(item.path, path.join(producer, path.basename(item.path)));
      const binding = { ...await verifyHelper(producer), head: state.head, tree: state.tree, run: state.run, attempt: state.attempt, observedAt: now() };
      await save(path.join(producer, 'helper-receipt.json'), binding); return binding;
    });
  } else {
    toolchain = lane === 'windows-consumer' ? path.join(evidence, 'private-toolchain') : await mkdtemp(path.join(await realpath(tmpdir()), 'gestalt-final-'));
    await mkdir(toolchain, { recursive: true, mode: 0o700 });
    await group('frozen-npm-and-lockfile', ['source'], prepareNpm);
    await group('same-run-helper', ['source'], stageHelper);
    if (lane === 'linux-full') {
      await group('host-policy', [], verifyLinuxEnvironment);
      await group('supported-sandbox-prerequisite', ['host-policy', 'frozen-npm-and-lockfile'], verifySandbox);
      await group('manager-shard-1', ['supported-sandbox-prerequisite', 'same-run-helper'], () => fullShard(1));
      await group('manager-shard-2', ['supported-sandbox-prerequisite', 'same-run-helper'], () => fullShard(2));
      await group('scratch-sandbox', ['supported-sandbox-prerequisite'], () => exactNpmGroup('scratch-sandbox', 'test:scratch:sandbox', 1));
      await group('package-tests', ['frozen-npm-and-lockfile'], () => exactNpmGroup('package-tests', 'test:package', 16));
      await group('cli-type-and-build', ['frozen-npm-and-lockfile'], async () => {
        await npmCommand('cli-typecheck', ['run', 'typecheck:cli']); await npmCommand('cli-build', ['run', 'build:cli']);
        const entry = await digest(path.join(root, 'dist/cli/gestalt.mjs')); assert.equal(entry.sha256, inputs.normalCliSha256); return entry;
      });
      await group('docs-build', ['frozen-npm-and-lockfile'], () => npmCommand('docs-build', ['run', 'build'], { env: { ...environment, BASE_PATH: '/gestalt/' } }).then(r => r.row));
      await group('package-build', ['frozen-npm-and-lockfile', 'same-run-helper'], preparePackage);
    } else {
      await group('package-build', ['frozen-npm-and-lockfile', 'same-run-helper'], preparePackage);
      await group('prepare-consumer', ['package-build'], async () => {
        const codexDir = path.join(toolchain, 'codex-pinned');
        const codex = await pinnedCodex(codexDir);
        const toolBin = path.join(toolchain, 'bin');
        const absoluteCodex = path.join(toolBin, 'codex.exe');
        await copyFile(codex.binary, absoluteCodex);
        assert.equal((await digest(absoluteCodex)).sha256, codex.record.binary.sha256);
        await copyFile(path.join(codexDir, 'artifact.json'), path.join(evidence, 'codex-artifact.json'));
        const output = path.join(evidence, 'prebuilt');
        await command('prepare-prebuilt-consumer', process.execPath,
          ['tests/native-core-slice.mjs', '--prepare', output, '--agents', agents], { timeout: 240_000,
            env: { ...environment, GESTALT_CORE_SLICE_NATIVE_ARTIFACTS: supplied, GESTALT_CORE_SLICE_PACKAGE: '1',
              GESTALT_CORE_SLICE_NPM_TARBALL: packageBinding.tarball.path, GESTALT_CORE_SLICE_NPM_SOURCE_HEAD: state.head,
              GESTALT_CORE_SLICE_NPM_SHA256: packageBinding.tarball.sha256 } });
        const artifacts = await json(path.join(output, 'artifacts.json'));
        assert.equal(artifacts.package.sha256, packageBinding.tarball.sha256);
        assert.equal(artifacts.artifacts.find(item => item.relative === artifacts.helper).sha256, helper.helper.sha256);
        return { prebuilt: await digest(path.join(output, 'artifacts.json')), codex: await digest(absoluteCodex),
          node: await digest(path.join(toolBin, 'node.exe')), npm: await digest(npm),
          npmShim: await digest(path.join(toolBin, 'npm.cmd')), npmEntry: await digest(path.join(toolBin, 'npm-entry.mjs')) };
      });
    }
  }
  assert.ok(state.groups.length > 0 && state.groups.every(row => row.state === 'pass'), 'Required verification group failed or remained unexecuted');
  state.state = 'pass';
} catch (error) { state.state = 'fail'; state.failure = errorInfo(error); process.exitCode = 1; }
finally {
  state.finishedAt = now();
  state.privateToolchain = { path: toolchain ?? null, cleaned: false, retained: Boolean(toolchain) };
  if (lane === 'linux-full' && toolchain && state.state === 'pass') {
    try { await rm(toolchain, { recursive: true }); state.privateToolchain.cleaned = true; state.privateToolchain.retained = false; }
    catch (error) { state.state = 'fail'; state.failure = errorInfo(error); process.exitCode = 1; }
  }
  await record();
  if (state.failure) console.error(state.failure.message);
}
