import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, openSync } from 'node:fs';
import { chmod, copyFile, lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { parse } from 'smol-toml';
import { captureCopyTree } from '../../scripts/native-copy-provenance.mjs';
import { digest, pinnedCodex } from '../fixtures/native-runtime/codex.mjs';

const managerRoot = path.resolve(import.meta.dirname, '../..');
const [agentsInput, evidenceInput] = process.argv.slice(2);
assert.ok(agentsInput && evidenceInput, 'Expected frozen Agents checkout and evidence directory');
const agentsRoot = await realpath(agentsInput);
const evidence = path.resolve(evidenceInput);
const inputs = JSON.parse(await readFile(path.join(import.meta.dirname, 'inputs.json'), 'utf8'));
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

assert.equal(process.platform + '-' + process.arch, 'linux-x64');
assert.notEqual(process.getuid(), 0);
await mkdir(evidence, { recursive: true, mode: 0o700 });
const commands = [], cases = [], fixtures = [];
const expectedCases = ['fresh-022-direct', 'repeat-direct', 'preexisting-private-operator-update', 'npm-global-lifecycle-and-repeat'];
const proof = { schemaVersion: 1, startedAt: now(), platform: process.platform, architecture: process.arch, node: process.version,
  abi: process.versions.modules, uid: process.getuid(), runnerImage: process.env.ImageVersion ?? null, inputScope: 'actual paired Manager and Agents with local marketplace selection only',
  commands, cases, state: 'started', failure: null };
let fixtureRoot;

async function run(name, executable, argv, environment, cwd = managerRoot) {
  const stdout = path.join(evidence, name + '.stdout');
  const stderr = path.join(evidence, name + '.stderr');
  const out = openSync(stdout, 'wx', 0o600), err = openSync(stderr, 'wx', 0o600);
  const startedAt = now();
  let result;
  try { result = spawnSync(executable, argv, { cwd, env: environment, shell: false, timeout: 360000, stdio: ['ignore', out, err] }); }
  finally { closeSync(out); closeSync(err); }
  commands.push({ name, argv: [executable, ...argv], cwd, exit: result.status, signal: result.signal, error: result.error?.message ?? null,
    startedAt, observedAt: now(), stdout: await digest(stdout), stderr: await digest(stderr) });
  await save('commands.json', commands);
  assert.equal(result.status, 0, name + ' failed; see captured streams');
  return (await readFile(stdout, 'utf8')).trim();
}

function cleanEnvironment() {
  return Object.fromEntries(Object.entries(process.env).filter(([key]) => ['PATH', 'LANG', 'LC_ALL', 'TZ', 'TERM'].includes(key)));
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

async function fixture(name, codex, inventory) {
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
  const environment = { ...cleanEnvironment(), PATH: bin + path.delimiter + process.env.PATH, HOME: home,
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
  return { configFile: await digest(configFile), marker, artifacts, orgHelper: await digest(helper), marketplace: await realpath(marketplace) };
}

async function exercise(name, execute) {
  const test = { name, startedAt: now(), status: 'started' };
  cases.push(test);
  try { test.result = await execute(); test.status = 'pass'; }
  catch (error) { test.status = 'fail'; test.error = error.message; throw error; }
  finally { test.observedAt = now(); await save('cases.json', cases); }
}

try {
  assert.equal(process.version, inputs.node);
  assert.equal(typeof process.execve, 'function', 'The Linux executable handoff is required');
  assert.ok(!Object.hasOwn(process.env, 'GESTALT_SESSION_TMP_CONTEXT'), 'Pairing fixtures must not replace an inherited scratch owner');
  proof.manager = { head: git(managerRoot, ['rev-parse', 'HEAD']).trim(), tree: git(managerRoot, ['rev-parse', 'HEAD^{tree}']).trim(), inputs: [] };
  assert.equal(proof.manager.head, process.env.GITHUB_SHA);
  assert.equal(git(managerRoot, ['rev-parse', 'HEAD^']).trim(), inputs.managerBase);
  for (const [relative, expected] of Object.entries(inputs.sources)) {
    const item = await digest(path.join(managerRoot, relative));
    assert.equal(item.sha256, expected, relative);
    proof.manager.inputs.push(item);
  }
  proof.agents = { head: git(agentsRoot, ['rev-parse', 'HEAD']).trim(), tree: git(agentsRoot, ['rev-parse', 'HEAD^{tree}']).trim(), acceptedCommit: inputs.agentsAccepted };
  assert.equal(proof.agents.head, inputs.agentsTransport);
  assert.equal(proof.agents.tree, inputs.agentsTree);
  const inventory = await agentsInventory();
  assert.equal(inventory.find(entry => entry.relative === 'plugins/context-mode/package-lock.json')?.sha256, inputs.agentsRuntimeLock);
  proof.agents.inventoryCount = inventory.length;
  await save('agents-inventory.json', inventory);
  fixtureRoot = await realpath(await mkdtemp('/tmp/gestalt-unix-pair-'));
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
  const builderUserConfig = path.join(fixtureRoot, 'builder.npmrc');
  await writeFile(builderUserConfig, '', { mode: 0o600 });
  const buildEnv = { ...cleanEnvironment(), HOME: fixtureRoot, npm_config_cache: path.join(fixtureRoot, 'build-cache'),
    npm_config_userconfig: builderUserConfig, npm_config_globalconfig: '/dev/null', npm_config_update_notifier: 'false' };
  assert.equal(await run('canonical-npm', 'npm', ['--version'], buildEnv), inputs.npm);
  const deliveryNative = path.join(managerRoot, 'dist/cli/native');
  await mkdir(deliveryNative, { recursive: true });
  for (const name of ['gestalt-owned-process.exe', 'gestalt-owned-process.exe.provenance.json']) {
    await copyFile(path.join(import.meta.dirname, 'frozen', name), path.join(deliveryNative, name));
  }
  await run('build-package', 'npm', ['run', 'build:package'], buildEnv);
  const packed = JSON.parse(await run('pack-current', 'npm', ['pack', './dist/npm', '--json', '--pack-destination', fixtureRoot], buildEnv));
  assert.equal(packed.length, 1);
  const tarball = path.join(fixtureRoot, packed[0].filename);
  proof.package = { ...await digest(tarball), pack: packed[0] };
  await copyFile(tarball, path.join(evidence, packed[0].filename));

  const fresh = await fixture('fresh', artifact.binary, inventory);
  await exercise('fresh-022-direct', async () => {
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
  });
  await exercise('preexisting-private-operator-update', async () => {
    const f = await fixture('existing', artifact.binary, inventory);
    for (const home of [f.codexHome, f.gestaltHome]) await mkdir(home, { mode: 0o700 });
    await writeFile(path.join(f.codexHome, 'config.toml'), 'model = "gpt-5" # operator setting\n', { mode: 0o640 });
    const sentinel = path.join(f.codexHome, 'AGENTS.md');
    await writeFile(sentinel, 'Operator data remains intact.\n', { mode: 0o600 });
    const before = await digest(sentinel);
    await run('existing-manager-update', 'bash', ['public/gestalt', 'agents', 'update'], f.environment);
    const result = await installed(f, 'existing', inventory);
    assert.equal(result.configFile.mode, '640');
    assert.match(await readFile(result.configFile.path, 'utf8'), /model = "gpt-5" # operator setting/);
    assert.deepEqual(await digest(sentinel), before);
    return result;
  });
  await exercise('npm-global-lifecycle-and-repeat', async () => {
    const f = await fixture('npm', artifact.binary, inventory), prefix = path.join(f.root, 'prefix');
    assert.equal(await exists(f.codexHome), false);
    assert.equal(await exists(f.gestaltHome), false);
    await run('npm-global-install', 'npm', ['install', '--global', '--prefix', prefix, '--no-audit', '--no-fund', '--foreground-scripts', '--ignore-scripts=false', tarball], f.environment, f.root);
    const setup = await installed(f, 'npm-installed', inventory);
    const pkg = path.join(prefix, 'lib/node_modules/@jaromil/gestalt');
    const manifest = JSON.parse(await readFile(path.join(pkg, 'package.json'), 'utf8'));
    assert.ok(manifest.os.includes('linux'));
    assert.equal(manifest.gitHead, proof.manager.head);
    assert.equal((await digest(path.join(pkg, 'lib/legacy/gestalt'))).sha256, inputs.sources['public/gestalt']);
    const before = await captureCopyTree(pkg), shim = path.join(prefix, 'bin/gestalt');
    assert.match(await run('npm-installed-version', shim, ['--version'], f.environment, f.root), /^gestalt 0\.16\.1/m);
    await run('npm-installed-update', shim, ['agents', 'update'], f.environment, f.root);
    const result = await installed(f, 'npm-repeat', inventory);
    assert.equal(result.configFile.sha256, setup.configFile.sha256);
    assert.deepEqual(await captureCopyTree(pkg), before);
    await save('installed-package-map.json', before);
    return { ...result, manifest, before, installedShim: await realpath(shim) };
  });
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
  proof.coverage = { expected: expectedCases, executed: cases.map(test => test.name),
    unexecuted: expectedCases.filter(name => !cases.some(test => test.name === name)) };
  await save('result.json', proof);
  if (proof.failure) console.error(proof.failure.message);
}
