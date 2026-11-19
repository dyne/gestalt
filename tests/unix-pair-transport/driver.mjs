import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { tmpdir } from 'node:os';
import { closeSync, openSync } from 'node:fs';
import { chmod, copyFile, lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { captureCopyTree } from '../../scripts/native-copy-provenance.mjs';
import { digest, pinnedCodex } from '../fixtures/native-runtime/codex.mjs';
import { checkPack, gitInventory, inputs, verifySameRun } from '../final-current-transport/common.mjs';

const managerRoot = path.resolve(import.meta.dirname, '../..');
const [agentsInput, evidenceInput, helperInput] = process.argv.slice(2);
assert.ok(agentsInput && evidenceInput && helperInput, 'Expected frozen Agents checkout, evidence and producer directory');
const agentsRoot = await realpath(agentsInput);
const evidence = path.resolve(evidenceInput);
const helperDirectory = path.resolve(helperInput);
const now = () => new Date().toISOString();
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const actualMode = stat => stat.mode & 0o777;
const save = (file, value) => writeFile(path.join(evidence, file), JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });
const exists = async file => !!await lstat(file).catch(error => { if (error.code !== 'ENOENT') throw error; });
const git = (cwd, args) => {
  const result = spawnSync('git', args, { cwd, shell: false, encoding: 'utf8', timeout: 10000, maxBuffer: 16 * 1024 * 1024 });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  return result.stdout;
};

await mkdir(evidence, { recursive: true, mode: 0o700 });
const commands = [], cases = [], fixtures = [];
const expectedCases = ['fresh-022-direct', 'repeat-direct', 'preexisting-private-operator-update', 'npm-global-lifecycle-and-repeat'];
const proof = { schemaVersion: 2, startedAt: now(), platform: process.platform, architecture: process.arch, node: process.version,
  abi: process.versions.modules, uid: process.getuid(), runnerImage: process.env.ImageVersion ?? null, inputScope: 'actual paired Manager and Agents with local marketplace selection only',
  commands, cases, state: 'started', failure: null };
let fixtureRoot, parse;
const selected = process.platform + '-' + process.arch;
const scope = { 'linux-x64': 'ubuntu-latest', 'darwin-arm64': 'macos-latest' };

async function run(name, executable, argv, environment, cwd = managerRoot, timeout = 360_000) {
  const stdout = path.join(evidence, name + '.stdout');
  const stderr = path.join(evidence, name + '.stderr');
  const out = openSync(stdout, 'wx', 0o600), err = openSync(stderr, 'wx', 0o600);
  const startedAt = now();
  let result;
  try { result = spawnSync(executable, argv, { cwd, env: environment, shell: false, timeout, stdio: ['ignore', out, err] }); }
  finally { closeSync(out); closeSync(err); }
  commands.push({ name, argv: [executable, ...argv], cwd, timeout, exit: result.status, signal: result.signal, error: result.error?.message ?? null,
    startedAt, observedAt: now(), stdout: await digest(stdout), stderr: await digest(stderr) });
  await save('commands.json', commands);
  assert.equal(result.status, 0, name + ' failed; see captured streams');
  return (await readFile(stdout, 'utf8')).trim();
}

function cleanEnvironment() {
  return Object.fromEntries(Object.entries(process.env).filter(([key]) => ['PATH', 'LANG', 'LC_ALL', 'TZ', 'TERM'].includes(key)));
}

async function frozenToolchain() {
  const response = await fetch(inputs.npm.url, { redirect: 'error', signal: AbortSignal.timeout(60_000) });
  assert.ok(response.ok && response.body);
  const chunks = [];
  let bytes = 0;
  for await (const chunk of response.body) {
    bytes += chunk.byteLength;
    assert.ok(bytes <= inputs.npm.bytes, 'Pinned npm archive exceeds recorded size');
    chunks.push(chunk);
  }
  assert.equal(bytes, inputs.npm.bytes);
  const archive = path.join(fixtureRoot, 'npm.tgz'), contents = Buffer.concat(chunks);
  assert.equal(createHash('sha512').update(contents).digest('hex'), inputs.npm.sha512);
  await writeFile(archive, contents, { mode: 0o600 });
  const npmRoot = path.join(fixtureRoot, 'npm');
  await mkdir(npmRoot, { mode: 0o700 });
  await run('extract-pinned-npm', '/usr/bin/tar', ['-xzf', archive, '-C', npmRoot], cleanEnvironment());
  const cli = path.join(npmRoot, 'package/bin/npm-cli.js');
  const bin = path.join(fixtureRoot, 'bin');
  await mkdir(bin, { mode: 0o700 });
  await symlink(cli, path.join(bin, 'npm'));
  if (selected === 'darwin-arm64') await symlink('/bin/bash', path.join(bin, 'bash'));
  const configuration = path.join(fixtureRoot, 'npmrc');
  await writeFile(configuration, '', { mode: 0o600 });
  const toolEnv = { ...cleanEnvironment(), HOME: fixtureRoot, PATH: [bin, path.dirname(process.execPath), process.env.PATH].join(path.delimiter),
    npm_config_cache: path.join(fixtureRoot, 'npm-cache'), npm_config_userconfig: configuration, npm_config_globalconfig: '/dev/null',
    npm_config_registry: 'https://registry.npmjs.org/', npm_config_update_notifier: 'false' };
  const version = await run('pinned-npm-version', process.execPath, [cli, '--version'], toolEnv);
  assert.equal(version, inputs.npm.version);
  proof.npm = { version, source: inputs.npm.url, bytes, sha512: inputs.npm.sha512, cli: await digest(cli) };
  return { cli, bin, env: toolEnv };
}


async function agentsInventory() {
  const records = [];
  for (const entry of git(agentsRoot, ['ls-tree', '-r', '-z', 'HEAD']).split('\0').filter(Boolean)) {
    const match = /^(100644|100755) blob ([0-9a-f]{40})\t([^\n]+)$/.exec(entry);
    assert.ok(match, 'Expected a regular pinned Agents Git leaf');
    const [, mode, blob, relative] = match;
    const source = path.join(agentsRoot, relative);
    const data = await readFile(source), stat = await lstat(source);
    assert.ok(stat.isFile() && !stat.isSymbolicLink());
    const observedBlob = createHash('sha1').update('blob ' + data.length + '\0').update(data).digest('hex');
    assert.equal(observedBlob, blob, relative);
    assert.equal(Boolean(stat.mode & 0o111), mode === '100755', relative);
    records.push({ relative, mode, blob, bytes: data.length, sha256: sha(data) });
  }
  return records;
}

async function copyAgents(destination, inventory) {
  for (const entry of inventory) {
    const target = path.join(destination, entry.relative);
    await mkdir(path.dirname(target), { recursive: true, mode: 0o700 });
    await copyFile(path.join(agentsRoot, entry.relative), target);
    await chmod(target, entry.mode === '100755' ? 0o755 : 0o644);
  }
  await verifyAgentsCopy(destination, inventory);
}

async function verifyAgentsCopy(source, inventory) {
  for (const entry of inventory) {
    const result = await digest(path.join(source, entry.relative));
    assert.equal(result.bytes, entry.bytes, entry.relative);
    assert.equal(result.sha256, entry.sha256, entry.relative);
    assert.equal(result.mode, entry.mode === '100755' ? '755' : '644', entry.relative);
  }
}

async function fixture(name, codex, inventory, npm) {
  const root = path.join(fixtureRoot, name);
  const home = path.join(root, 'home'), bin = path.join(root, 'bin');
  const codexHome = path.join(home, '.codex-gestalt'), gestaltHome = path.join(home, '.gestalt');
  const localAgents = path.join(root, 'agents');
  fixtures.push({ name, root, codexHome, gestaltHome });
  for (const directory of [home, bin, path.join(root, 'tmp')]) await mkdir(directory, { mode: 0o700, recursive: true });
  await copyAgents(localAgents, inventory);
  const wrapper = path.join(bin, 'codex');
  await copyFile(path.join(import.meta.dirname, 'codex-wrapper.mjs'), wrapper);
  await chmod(wrapper, 0o700);
  const npmUserConfig = path.join(root, '.npmrc');
  await writeFile(npmUserConfig, '', { mode: 0o600 });
  const environment = { ...cleanEnvironment(), PATH: bin + path.delimiter + npm.env.PATH, HOME: home,
    CODEX_HOME: codexHome, GESTALT_HOME: gestaltHome, TMPDIR: path.join(root, 'tmp'), CONTEXT_MODE_PACKAGE_MANAGER: 'npm',
    npm_config_userconfig: npmUserConfig, npm_config_globalconfig: '/dev/null', npm_config_registry: 'https://registry.npmjs.org/',
    npm_config_cache: path.join(root, 'cache'), npm_config_update_notifier: 'false',
    PAIR_CODEX: codex, PAIR_LOCAL_AGENTS: localAgents, PAIR_MARKETPLACE_TRACE: path.join(evidence, name + '.marketplace.jsonl') };
  return { name, root, home, codexHome, gestaltHome, localAgents, environment };
}

async function installed(f, name, inventory) {
  for (const home of [f.codexHome, f.gestaltHome]) {
    assert.equal(actualMode(await lstat(home)), 0o700, home);
    assert.equal((await readdir(home)).filter(entry => entry.startsWith('.gestalt-setup.') || entry === '.gestalt-manager-setup.lock').length, 0, home);
  }
  const runtime = path.join(f.gestaltHome, 'runtime/context-mode');
  const marker = JSON.parse(await readFile(path.join(runtime, '.context-mode-prepared.json'), 'utf8'));
  assert.equal(marker.platform, process.platform);
  assert.equal(marker.arch, process.arch);
  assert.equal(marker.nodeModulesAbi, process.versions.modules);
  const artifacts = [];
  for (const relative of ['server.bundle.mjs', 'cli.bundle.mjs']) artifacts.push(await digest(path.join(runtime, relative)));
  const configFile = path.join(f.codexHome, 'config.toml');
  const config = parse(await readFile(configFile, 'utf8'));
  assert.equal(config.default_permissions, 'workspace-git');
  assert.equal(config.mcp_servers['context-mode'].command, 'node');
  const helper = path.join(f.codexHome, 'bin/org-plan');
  assert.match(await run(name + '-installed-org', helper, ['--help'], f.environment, f.root), /org-plan/i);
  await verifyAgentsCopy(f.localAgents, inventory);
  const marketplace = path.join(f.codexHome, '.tmp/marketplaces/dyne-gestalt-agents');
  assert.equal(await realpath(marketplace), f.localAgents);
  const provenance = JSON.parse(await readFile(path.join(f.codexHome, '.gestalt-setup-provenance.json'), 'utf8'));
  const shell = inventory.find(item => item.relative === 'gestalt-setup.sh');
  assert.equal(provenance.delegatedSetupSha256, shell.sha256);
  assert.equal(provenance.manager.sha256, inputs.sourceDigests['public/gestalt']);
  const configDigest = await digest(configFile);
  assert.equal(provenance.outputs['config.toml'], configDigest.sha256);
  assert.equal(Object.hasOwn(provenance, 'delegatedApi'), false);
  const caller = await readFile(helper, 'utf8');
  const token = /\/bundles\/([0-9a-f-]{36})\/org-plan-cli\.mjs/.exec(caller)?.[1];
  assert.match(token, /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/);
  const bundle = path.join(f.codexHome, 'lib/gestalt-org-plan/bundles', token);
  const publication = JSON.parse(await readFile(path.join(bundle, '.publication.json'), 'utf8'));
  assert.equal(publication.schemaVersion, 1);
  assert.equal(publication.token, token);
  assert.deepEqual((await readdir(bundle)).sort(), ['.publication.json', ...publication.files.map(file => file.name)].sort());
  const scriptEntries = inventory.filter(file => file.relative.startsWith('plugins/gestalt/skills/org-plan/scripts/'));
  const expectedEntries = ['LICENSE', ...scriptEntries.map(file => path.basename(file.relative))].sort();
  assert.deepEqual(publication.files.map(file => file.name).sort(), expectedEntries);
  const bundleFiles = [];
  for (const item of publication.files) {
    assert.match(item.name, /^[A-Za-z0-9._-]+$/);
    const candidate = await digest(path.join(bundle, item.name));
    const relative = item.name === 'LICENSE' ? 'plugins/gestalt/LICENSE' : 'plugins/gestalt/skills/org-plan/scripts/' + item.name;
    assert.equal(candidate.sha256, item.sha256);
    assert.equal(candidate.sha256, inventory.find(file => file.relative === relative)?.sha256);
    assert.equal(candidate.mode, item.name === 'org-plan' ? '755' : '644');
    assert.equal(candidate.mode, item.mode.toString(8));
    bundleFiles.push(candidate);
  }
  assert.ok(bundleFiles.some(item => path.basename(item.path) === 'org-plan-storage.mjs' && item.sha256 === inputs.agentsStorageSha256));
  return { configFile: configDigest, marker, artifacts, orgHelper: await digest(helper), marketplace: await realpath(marketplace),
    orgPublication: await digest(path.join(bundle, '.publication.json')), bundleFiles, provenance };
}

async function exercise(name, execute, prerequisite) {
  const test = { name, startedAt: now(), status: 'started' };
  cases.push(test);
  if (prerequisite && prerequisite.status !== 'pass') {
    test.status = 'blocked';
    test.blockedBy = prerequisite.name;
  } else {
    try { test.result = await execute(test); test.status = 'pass'; }
    catch (error) {
      test.status = 'fail';
      test.error = { name: error.name, message: error.message, stack: error.stack };
    }
  }
  test.observedAt = now();
  await save('cases.json', cases);
  return test;
}

try {
  await save('result.json', proof);
  assert.ok(Object.hasOwn(scope, selected));
  assert.equal(process.env.R9_RUNNER_ENVIRONMENT, 'github-hosted');
  assert.equal(process.env.R9_RUNNER_LABEL, scope[selected]);
  assert.equal(process.env.R9_RUNNER_TARGET, selected);
  assert.notEqual(process.getuid(), 0);
  assert.equal(process.version, inputs.node);
  assert.equal(typeof process.execve, 'function', 'The Unix executable handoff is required');
  proof.kernel = await run('kernel', '/usr/bin/uname', ['-srm'], cleanEnvironment());
  proof.os = selected === 'darwin-arm64' ? await run('os-release', '/usr/bin/sw_vers', [], cleanEnvironment())
    : await readFile('/etc/os-release', 'utf8');
  if (selected === 'linux-x64') assert.match(proof.os, /^VERSION_ID="?26\.04"?$/m);
  assert.ok(!Object.hasOwn(process.env, 'GESTALT_SESSION_TMP_CONTEXT'), 'Pairing fixtures must not replace an inherited scratch owner');
  proof.manager = { head: git(managerRoot, ['rev-parse', 'HEAD']).trim(), tree: git(managerRoot, ['rev-parse', 'HEAD^{tree}']).trim(), inputs: [] };
  assert.equal(proof.manager.head, process.env.GITHUB_SHA);
  assert.equal(git(managerRoot, ['rev-parse', 'HEAD^']).trim(), inputs.managerBase);
  const sourceArgs = async (name, executable, argv, { cwd }) => ({ text: await run(name, executable, argv, cleanEnvironment(), cwd) });
  const managerFiles = await gitInventory(managerRoot, 'manager', sourceArgs, evidence);
  assert.equal(managerFiles.length, inputs.sourceLeafCount + inputs.overlayPaths.length);
  assert.deepEqual(git(managerRoot, ['diff', '--name-only', 'HEAD^', 'HEAD']).trim().split('\n').sort(),
    [...inputs.sourcePaths, ...inputs.overlayPaths].sort());
  proof.manager.inventoryCount = managerFiles.length;
  for (const [relative, expected] of Object.entries(inputs.sourceDigests)) {
    const item = await digest(path.join(managerRoot, relative));
    assert.equal(item.sha256, expected, relative);
    proof.manager.inputs.push(item);
  }
  proof.agents = { head: git(agentsRoot, ['rev-parse', 'HEAD']).trim(), tree: git(agentsRoot, ['rev-parse', 'HEAD^{tree}']).trim(), historicalAcceptedParent: inputs.agentsParent };
  assert.equal(proof.agents.head, inputs.agentsSnapshot);
  assert.equal(proof.agents.tree, inputs.agentsTree);
  const inventory = await agentsInventory();
  assert.equal(inventory.find(entry => entry.relative === 'plugins/context-mode/package-lock.json')?.sha256, inputs.agentsRuntimeLock);
  assert.equal(inventory.length, 755);
  assert.equal(inventory.find(entry => entry.relative === 'plugins/gestalt/skills/org-plan/scripts/org-plan-storage.mjs')?.sha256, inputs.agentsStorageSha256);
  proof.agents.inventoryCount = inventory.length;
  await save('agents-inventory.json', inventory);
  const base = await realpath(tmpdir());
  fixtureRoot = await realpath(await mkdtemp(path.join(base, 'gestalt-unix-pair-')));
  const { ensureManagedRoot } = await import(pathToFileURL(path.join(agentsRoot, 'scripts/setup-storage.mjs')));
  const ancestors = [];
  for (let current = fixtureRoot;; current = path.dirname(current)) {
    const stat = await lstat(current);
    ancestors.push({ path: current, mode: stat.mode & 0o7777, uid: stat.uid });
    if (path.dirname(current) === current) break;
  }
  proof.preconditions = { ancestors, acceptedGuard: 'pending', ordinaryUmask: '022' };
  ensureManagedRoot(fixtureRoot, { create: false });
  proof.preconditions.acceptedGuard = 'pass';
  process.umask(0o022);
  const artifact = await pinnedCodex(path.join(fixtureRoot, 'codex-artifact'));
  proof.codex = artifact.record;
  const npm = await frozenToolchain();
  await run('lockfile-local-npm-ci', process.execPath, [npm.cli, 'ci', '--ignore-scripts', '--no-audit', '--no-fund'], npm.env);
  const modules = await realpath(path.join(managerRoot, 'node_modules'));
  assert.equal(modules, path.join(managerRoot, 'node_modules'));
  const renderer = "import { realpathSync } from 'node:fs'; import { fileURLToPath } from 'node:url';" +
    "console.log(JSON.stringify(['esbuild','smol-toml','typescript','@types/node/package.json','@esbuild/'+process.platform+'-'+process.arch+'/package.json'].map(name=>({name,physical:realpathSync(fileURLToPath(import.meta.resolve(name)))}))));";
  proof.lockfileModules = JSON.parse(await run('lockfile-local-modules', process.execPath, ['--input-type=module', '-e', renderer], npm.env));
  for (const module of proof.lockfileModules) assert.ok(module.physical.startsWith(modules + path.sep), module.name);
  ({ parse } = await import('smol-toml'));
  proof.bash = await run('bash-identity', 'bash', ['-c', 'printf "%s\n" "$BASH_VERSION"; command -v bash'], npm.env);
  if (selected === 'darwin-arm64') {
    const [version, bash] = proof.bash.split('\n');
    assert.match(version, /^3\.2\./);
    assert.equal(await realpath(bash), await realpath('/bin/bash'));
  }

  const helper = await verifySameRun(helperDirectory, proof.manager.head, proof.manager.tree);
  proof.inertWindowsHelper = helper;
  const deliveryNative = path.join(managerRoot, 'dist/cli/native');
  await mkdir(deliveryNative, { recursive: true });
  for (const name of ['gestalt-owned-process.exe', 'gestalt-owned-process.exe.provenance.json']) {
    await copyFile(path.join(helperDirectory, name), path.join(deliveryNative, name));
  }
  await run('build-package', process.execPath, [npm.cli, 'run', 'build:package'], npm.env);
  const packed = JSON.parse(await run('pack-current', process.execPath, [npm.cli, 'pack', './dist/npm', '--json', '--pack-destination', fixtureRoot], npm.env));
  assert.equal(packed.length, 1);
  const tarball = path.join(fixtureRoot, packed[0].filename);
  proof.package = await checkPack(tarball, npm.cli, path.join(managerRoot, 'dist/npm'), proof.manager.head, helper);
  await save('package-binding.json', proof.package);
  await copyFile(tarball, path.join(evidence, packed[0].filename));

  let fresh;
  const freshCase = await exercise('fresh-022-direct', async () => {
    fresh = await fixture('fresh', artifact.binary, inventory, npm);
    assert.equal(await exists(fresh.codexHome), false);
    assert.equal(await exists(fresh.gestaltHome), false);
    await run('fresh-manager-install', 'bash', ['public/gestalt', 'agents', 'install'], fresh.environment);
    return installed(fresh, 'fresh', inventory);
  });
  await exercise('repeat-direct', async () => {
    const before = await digest(path.join(fresh.codexHome, 'config.toml'));
    await run('fresh-manager-update', 'bash', ['public/gestalt', 'agents', 'update'], fresh.environment);
    const result = await installed(fresh, 'repeat', inventory);
    assert.equal(result.configFile.sha256, before.sha256);
    return result;
  }, freshCase);
  await exercise('preexisting-private-operator-update', async test => {
    const f = await fixture('existing', artifact.binary, inventory, npm);
    for (const home of [f.codexHome, f.gestaltHome]) await mkdir(home, { mode: 0o700 });
    const configFile = path.join(f.codexHome, 'config.toml');
    const operatorLine = 'model = "gpt-5" # operator setting';
    await writeFile(configFile, operatorLine + '\n', { mode: 0o640 });
    const sentinel = path.join(f.codexHome, 'AGENTS.md');
    await writeFile(sentinel, 'Operator data remains intact.\n', { mode: 0o600 });
    const before = { config: await digest(configFile), sentinel: await digest(sentinel) };
    const operatorProfile = { before };
    test.operatorProfile = operatorProfile;
    assert.equal(before.config.mode, '640');
    await run('existing-manager-update', 'bash', ['public/gestalt', 'agents', 'update'], f.environment);
    const result = await installed(f, 'existing', inventory);
    const config = await readFile(configFile, 'utf8');
    operatorProfile.after = { config: result.configFile, sentinel: await digest(sentinel),
      model: parse(config).model, operatorLineRetained: config.split(/\r?\n/).includes(operatorLine) };
    // Pinned Codex 0.164.0 rewrites the config privately when enabling a plugin.
    assert.equal(operatorProfile.after.config.mode, '600');
    assert.equal(operatorProfile.after.model, 'gpt-5');
    assert.equal(operatorProfile.after.operatorLineRetained, true);
    assert.deepEqual(operatorProfile.after.sentinel, before.sentinel);
    return result;
  });
  await exercise('npm-global-lifecycle-and-repeat', async () => {
    const f = await fixture('npm', artifact.binary, inventory, npm), prefix = path.join(f.root, 'prefix');
    assert.equal(await exists(f.codexHome), false);
    assert.equal(await exists(f.gestaltHome), false);
    await run('npm-global-install', 'npm', ['install', '--global', '--prefix', prefix, '--no-audit', '--no-fund', '--foreground-scripts', '--ignore-scripts=false', tarball], f.environment, f.root);
    const setup = await installed(f, 'npm-installed', inventory);
    const pkg = path.join(prefix, 'lib/node_modules/@jaromil/gestalt');
    const manifest = JSON.parse(await readFile(path.join(pkg, 'package.json'), 'utf8'));
    assert.ok(manifest.os.includes(process.platform));
    assert.equal(manifest.gitHead, proof.manager.head);
    assert.equal((await digest(path.join(pkg, 'lib/legacy/gestalt'))).sha256, inputs.sourceDigests['public/gestalt']);
    const before = await captureCopyTree(pkg), shim = path.join(prefix, 'bin/gestalt');
    assert.equal(Object.keys(proof.package.members).length, 10);
    for (const [name, expected] of Object.entries(proof.package.members)) {
      const actual = await digest(path.join(pkg, name));
      assert.equal(actual.sha256, expected.sha256, name);
      assert.equal(actual.bytes, expected.size, name);
      assert.equal(actual.mode, expected.mode, name);
    }
    assert.match(await run('npm-installed-version', shim, ['--version'], f.environment, f.root), /^gestalt 0\.16\.1/m);
    await run('npm-installed-update', shim, ['agents', 'update'], f.environment, f.root);
    const result = await installed(f, 'npm-repeat', inventory);
    assert.equal(result.configFile.sha256, setup.configFile.sha256);
    assert.deepEqual(await captureCopyTree(pkg), before);
    await save('installed-package-map.json', before);
    return { ...result, manifest, before, installedShim: await realpath(shim) };
  });
  assert.deepEqual(cases.filter(test => test.status === 'pass').map(test => test.name), expectedCases,
    'All four actual pairing cases must pass; see cases.json for failed or blocked checks');
  proof.state = 'pass';
} catch (error) {
  proof.state = 'fail';
  proof.failure = { name: error.name, message: error.message, stack: error.stack };
  process.exitCode = 1;
} finally {
  try {
    if (fixtureRoot && proof.state === 'pass') {
      await rm(fixtureRoot, { recursive: true });
      proof.fixtureCleanup = { attempted: true, removed: true, path: fixtureRoot };
    } else {
      const retained = [];
      for (const f of fixtures) {
        const homes = [];
        for (const home of [f.codexHome, f.gestaltHome]) {
          if (!await exists(home)) continue;
          const info = await lstat(home);
          if (!info.isDirectory() || info.isSymbolicLink()) continue;
          const setupState = (await readdir(home)).filter(entry => entry.startsWith('.gestalt-setup') || entry === '.gestalt-manager-setup.lock');
          const config = path.join(home, 'config.toml');
          const file = await exists(config) ? await lstat(config) : null;
          homes.push({ home, mode: actualMode(info), setupState, config: file?.isFile() ? await digest(config) : null });
        }
        retained.push({ name: f.name, homes });
      }
      proof.fixtureCleanup = { attempted: false, removed: false, path: fixtureRoot ?? null, retained, expiresWithHostedRunner: !!fixtureRoot };
    }
  } catch (error) {
    proof.fixtureCleanup = { attempted: true, removed: false, path: fixtureRoot, error: error.message };
    proof.state = 'fail';
    process.exitCode = 1;
  }
  proof.finishedAt = now();
  const executed = cases.filter(test => test.status === 'pass' || test.status === 'fail').map(test => test.name);
  proof.coverage = { expected: expectedCases, executed,
    passed: cases.filter(test => test.status === 'pass').map(test => test.name),
    failed: cases.filter(test => test.status === 'fail').map(test => test.name),
    blocked: cases.filter(test => test.status === 'blocked').map(test => ({ name: test.name, blockedBy: test.blockedBy })),
    unexecuted: expectedCases.filter(name => !executed.includes(name)) };
  await save('result.json', proof);
  if (proof.failure) console.error(proof.failure.message);
}
