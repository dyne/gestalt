/** Hosted integration: the compiled manager invokes the exact accepted Agents
 * Node owner and actual npm/Codex, in isolated homes. No live installation. */
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, copyFile, mkdir, mkdtemp, readFile, readdir, realpath, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { buildCli } from '../scripts/build-cli.mjs';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { commandTarget, NativeExecutables } from '../src/cli/platform/executables.ts';
import { NativeOwnedProcesses } from '../src/cli/platform/processes.ts';
import { captureSetupRetention } from './fixtures/setup-retention.mjs';
import { compositionInputs } from './fixtures/setup-composition-inputs.mjs';

const root = path.resolve(import.meta.dirname, '..');
const evidence = process.env.GESTALT_NATIVE_SETUP_EVIDENCE;
const dependency = process.env.GESTALT_NATIVE_AGENTS_SOURCE;
if (!evidence || !dependency) throw new Error('native installed setup requires exact dependency and evidence paths');
await mkdir(evidence, { recursive: true });
const digest = bytes => createHash('sha256').update(bytes).digest('hex');

if (process.platform === 'win32') test('actual compiled Windows manager provisions accepted native Agents and repeats offline', { timeout: 1_200_000 }, async t => {
  const base = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt core installed ü & ')));
  let retainFixture = true;
  t.after(() => retainFixture ? undefined : rm(base, { recursive: true, force: true }));
  const manager = await buildCli(root, path.join(base, 'gestalt.mjs'));
  const ownershipHelper = await buildOwnedProcess(root, path.join(base, 'native'));
  await writeFile(path.join(evidence, 'installed-ownership.provenance.json'), await readFile(`${ownershipHelper}.provenance.json`));
  const home = path.join(base, 'home ü &'), codex = path.join(home, 'Codex ü &'), gestalt = path.join(home, 'Gestalt ü &');
  await mkdir(home, { mode: 0o700 });
  const { collection, mobilePackage, release, skill } = await compositionInputs(base);
  const previousManagerSha256 = digest(await readFile(manager));
  const replacement = Buffer.concat([await readFile(manager), Buffer.from('\n// hosted native standalone replacement fixture\n')]);
  await writeFile(path.join(release, 'gestalt.mjs'), replacement);
  const environment = { ...process.env, HOME: home, USERPROFILE: home, CLAUDE_CONFIG_DIR: path.join(home, '.claude'), CONTEXT_MODE_DIR: path.join(home, 'context-mode-data'),
    CODEX_HOME: codex, GESTALT_HOME: gestalt, GESTALT_MANAGER_DISTRIBUTION: 'npm', GESTALT_AGENTS_SOURCE: await realpath(dependency),
    GESTALT_AGENTS_SETUP_SHA256: digest(await readFile(path.join(dependency, 'gestalt-setup.mjs'))),
    GESTALT_AGENTS_SETUP_OWNER_SHA256: digest(await readFile(path.join(dependency, 'scripts/setup-agents.mjs'))),
    CONTEXT_MODE_PACKAGE_MANAGER: 'npm', npm_config_ignore_scripts: 'true', CONTEXT_MODE_PLATFORM: 'codex', CONTEXT_MODE_WORKSPACE: base };
  for (const key of Object.keys(environment)) if (key.startsWith('GESTALT_MOBILE_ORG_PLAN_')) delete environment[key];
  const hostAncestors = [];
  for (let parent = home; ; parent = path.dirname(parent)) {
    const info = await stat(parent); hostAncestors.push({ path: parent, uid: info.uid, gid: info.gid, mode: info.mode & 0o7777 });
    if (parent === path.dirname(parent)) break;
  }
  await writeFile(path.join(evidence, 'host-setup-namespace.json'), JSON.stringify({ observedAt: new Date().toISOString(),
    uid: process.getuid?.() ?? null, gid: process.getgid?.() ?? null, umask: process.umask(), ancestors: hostAncestors, setupApiVersion: 1 }, null, 2));
  // Capture the actual filesystem UID view inside the same ownership backend.
  // This diagnostic never substitutes for the real accepted Agents setup below.
  const namespaceProbe = `const fs=require('node:fs'),path=require('node:path');const roots=process.argv.slice(1),ancestors=[];for(let p=path.dirname(roots[0]);;p=path.dirname(p)){try{const s=fs.statSync(p);ancestors.push({path:p,uid:s.uid,gid:s.gid,mode:s.mode&511});}catch(e){ancestors.push({path:p,error:e.code});}if(p===path.dirname(p))break;}let uidMap=null,gidMap=null;for(const [key,file] of [['uid','/proc/self/uid_map'],['gid','/proc/self/gid_map']])try{const value=fs.readFileSync(file,'utf8');if(key==='uid')uidMap=value;else gidMap=value;}catch{}console.log(JSON.stringify({platform:process.platform,uid:process.getuid?.()??null,gid:process.getgid?.()??null,uidMap,gidMap,ancestors}));`;
  const diagnosticOutput = [], diagnosticError = [];
  const diagnostic = await new NativeOwnedProcesses(ownershipHelper).launch(process.execPath,
    { argv: ['-e', namespaceProbe, codex, gestalt], cwd: base, environment },
    { stdin: (async function* () {})(), stdout: async bytes => { diagnosticOutput.push(Buffer.from(bytes)); }, stderr: async bytes => { diagnosticError.push(Buffer.from(bytes)); } });
  const diagnosticResult = await diagnostic.result, diagnosticClaim = await diagnostic.stop();
  await writeFile(path.join(evidence, 'owned-setup-namespace.json'), JSON.stringify({ observedAt: new Date().toISOString(), result: diagnosticResult, claim: diagnosticClaim,
    stdout: Buffer.concat(diagnosticOutput).toString('utf8'), stderr: Buffer.concat(diagnosticError).toString('utf8'), ownershipHelperSha256: digest(await readFile(ownershipHelper)) }, null, 2));
  const checks = [];
  const run = async (label, argv, extra = {}) => {
    const observedAt = new Date().toISOString();
    const result = spawnSync(process.execPath, [manager, ...argv], { env: { ...environment, ...extra }, cwd: base, encoding: 'utf8', timeout: 1_080_000, maxBuffer: 16 * 1024 * 1024, shell: false });
    const roots = await Promise.all([codex, gestalt].map(async selected => {
      try { const info = await stat(selected); return { path: selected, uid: info.uid, gid: info.gid, mode: info.mode & 0o7777, directory: info.isDirectory() }; }
      catch (error) { return { path: selected, error: error.code }; }
    }));
    await writeFile(path.join(evidence, `${label}-managed-roots.json`), JSON.stringify({ observedAt: new Date().toISOString(), roots }, null, 2));
    const log = path.join(evidence, `${label}.log`), bytes = Buffer.from((result.stdout ?? '') + (result.stderr ?? ''));
    await writeFile(log, bytes);
    checks.push({ label, command: [process.execPath, manager, ...argv], cwd: base, status: result.status, signal: result.signal, error: result.error?.message ?? null,
      observedAt, log, sha256: digest(bytes) });
    await writeFile(path.join(evidence, 'installed-checks.json'), JSON.stringify(checks, null, 2));
    assert.equal(result.status, 0, `${label}: ${result.error?.message ?? ''}; see ${log}: ${(result.stderr ?? '').slice(-3000)}`);
  };
  for (const selected of [codex, gestalt]) await assert.rejects(stat(selected), error => error.code === 'ENOENT', 'fresh composition must begin with both managed roots absent');
  await run('manager-standalone-update-fresh', ['update', 'gestalt'], {
    GESTALT_MANAGER_DISTRIBUTION: '', GESTALT_INSTALL_BASE_URL: pathToFileURL(release).href,
    GESTALT_SKILLS_SOURCE: collection, GESTALT_MOBILE_PACKAGE: mobilePackage, npm_config_install_links: 'true',
  });
  retainFixture = false;
  assert.deepEqual(await readFile(manager), replacement, 'compiled updated-manager API must run the actual installed replacement');
  assert.equal(await readFile(path.join(codex, 'skills/native-update/SKILL.md'), 'utf8'), skill);
  const updateLock = path.join(base, '.gestalt-manager-update.lock');
  assert.equal(await stat(updateLock).then(() => true, error => { if (error.code === 'ENOENT') return false; throw error; }), false);
  await writeFile(path.join(evidence, 'standalone-compiled-update.json'), JSON.stringify({ observedAt: new Date().toISOString(),
    previousManagerSha256, replacementSha256: digest(replacement), installedManagerSha256: digest(await readFile(manager)),
    updateApiVersion: 1, hostFilesystem: true, actualAcceptedAgentsSource: environment.GESTALT_AGENTS_SOURCE,
    skillsFixtureSha256: digest(Buffer.from(skill)), mobileFixtureManifestSha256: digest(await readFile(path.join(mobilePackage, 'package.json'))),
    updateLockRetained: false, updateReceipt: null }, null, 2));
  const outputs = ['config.toml', 'hooks.json', 'bin/context-mode-mcp.mjs', 'bin/context-mode-hook.mjs', '.gestalt-setup-provenance.json'];
  const first = await Promise.all(outputs.map(async file => digest(await readFile(path.join(codex, file)))));
  await run('manager-agents-repeat-offline', ['agents', 'update'], { npm_config_offline: 'true' });
  assert.deepEqual(await Promise.all(outputs.map(async file => digest(await readFile(path.join(codex, file))))), first);
  const prepared = JSON.parse(await readFile(path.join(gestalt, 'runtime/context-mode/.context-mode-prepared.json'), 'utf8'));
  assert.equal(prepared.platform, process.platform); assert.equal(prepared.arch, process.arch); assert.equal(prepared.nodeModulesAbi, process.versions.modules);
  const provenance = JSON.parse(await readFile(path.join(codex, '.gestalt-setup-provenance.json'), 'utf8'));
  assert.equal(provenance.manager.sha256, digest(await readFile(manager)));
  assert.equal(provenance.delegatedSetupSha256, environment.GESTALT_AGENTS_SETUP_SHA256);
  assert.equal(provenance.delegatedApi.apiVersion, 1); assert.equal(provenance.delegatedApi.mode, 'managed');
  assert.equal(provenance.delegatedApi.ownerSha256, environment.GESTALT_AGENTS_SETUP_OWNER_SHA256);
  assert.equal(provenance.delegatedApi.ownership, 'released');
  const codexExecutable = await new NativeExecutables().resolve('codex', environment);
  assert.ok(codexExecutable);
  const target = await commandTarget(codexExecutable);
  const version = spawnSync(target.executable, [...target.prefix, '--version'], { env: environment, encoding: 'utf8', shell: false });
  await writeFile(path.join(evidence, 'installed-identity.json'), JSON.stringify({ platform: process.platform, architecture: process.arch, node: process.version,
    abi: process.versions.modules, managerSha256: digest(await readFile(manager)), dependencySource: environment.GESTALT_AGENTS_SOURCE,
    delegatedSetupSha256: environment.GESTALT_AGENTS_SETUP_SHA256, delegatedApiOwnerSha256: environment.GESTALT_AGENTS_SETUP_OWNER_SHA256,
    setupApiVersion: 1, managedOwnership: 'released', retained: null,
    repeat: 'offline-success', codexVersion: version.stdout?.trim(), codexStatus: version.status,
    runtime: prepared, ownershipHelperSha256: digest(await readFile(ownershipHelper)), observedAt: new Date().toISOString() }, null, 2));
});

const acceptedAgents = JSON.parse(await readFile(new URL('./fixtures/native-runtime/agents-source.json', import.meta.url), 'utf8'));

async function copyAcceptedAgents(destination, evidenceDirectory) {
  const checkout = await realpath(dependency);
  const git = (...argv) => {
    const result = spawnSync('git', ['--no-optional-locks', '-C', checkout, ...argv],
      { encoding: 'utf8', shell: false, timeout: 30_000, maxBuffer: 16 * 1024 * 1024 });
    assert.equal(result.status, 0, result.error?.message ?? result.stderr);
    return result.stdout;
  };
  const head = git('rev-parse', 'HEAD').trim(), tree = git('rev-parse', 'HEAD^{tree}').trim();
  assert.equal(head, acceptedAgents.acceptedCommit);
  assert.equal(tree, acceptedAgents.acceptedTree);
  const files = [];
  for (const entry of git('ls-tree', '-rz', 'HEAD').split('\0').filter(Boolean)) {
    const match = /^(100644|100755) blob ([0-9a-f]{40})\t(.+)$/.exec(entry);
    assert.ok(match, 'Expected a pinned regular Agents file');
    const [, mode, blob, relative] = match;
    const source = path.join(checkout, relative);
    assert.equal(await realpath(source), source);
    const bytes = await readFile(source);
    assert.equal(createHash('sha1').update('blob ' + bytes.length + '\0').update(bytes).digest('hex'), blob, relative);
    assert.equal(Boolean((await stat(source)).mode & 0o111), mode === '100755', relative);
    const target = path.join(destination, relative);
    await mkdir(path.dirname(target), { recursive: true, mode: 0o700 });
    await writeFile(target, bytes, { flag: 'wx', mode: mode === '100755' ? 0o755 : 0o644 });
    files.push({ relative, mode, sha256: digest(bytes) });
  }
  assert.equal(files.length, 755);
  assert.ok(files.some(file => file.relative === 'gestalt-setup.sh'));
  await writeFile(path.join(evidenceDirectory, 'agents-inventory.json'), JSON.stringify({ checkout, head, tree, files }, null, 2));
  return { checkout, head, tree, files };
}

async function unixInstalledFixture(t, lane) {
  const logs = path.join(evidence, lane);
  await mkdir(logs, { recursive: true });
  assert.ok(!Object.hasOwn(process.env, 'GESTALT_SESSION_TMP_CONTEXT'), 'Run installed setup with no inherited scratch owner');
  const base = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt core installed ü & ')));
  let retained = true;
  t.after(() => retained ? undefined : rm(base, { recursive: true, force: true }));
  const home = path.join(base, 'home ü &'), codex = path.join(home, 'Codex ü &'), gestalt = path.join(home, 'Gestalt ü &');
  const bin = path.join(base, 'bin'), temp = path.join(base, 'tmp'), localAgents = path.join(base, 'accepted local Agents');
  for (const directory of [home, bin, temp]) await mkdir(directory, { mode: 0o700 });
  const agents = await copyAcceptedAgents(localAgents, logs);
  const { ensureManagedRoot } = await import(pathToFileURL(path.join(localAgents, 'scripts/setup-storage.mjs')).href);
  try { ensureManagedRoot(base, { create: false }); }
  catch (error) {
    await writeFile(path.join(logs, 'precondition.json'), JSON.stringify({ platform: process.platform, base, error: error.message }, null, 2));
    throw error;
  }
  const manager = await buildCli(root, path.join(base, 'gestalt.mjs'));
  const fixture = await compositionInputs(base);
  const executables = new NativeExecutables();
  const tools = Object.fromEntries(Object.entries(process.env).filter(([key]) => ['PATH', 'LANG', 'LC_ALL', 'TZ', 'TERM'].includes(key)));
  const [actualCodex, actualNpm] = await Promise.all([executables.resolve('codex', tools), executables.resolve('npm', tools)]);
  assert.ok(actualCodex && actualNpm);
  for (const name of ['codex', 'npm']) {
    const wrapper = path.join(bin, name);
    await copyFile(path.join(root, 'tests/fixtures/unix-setup-external.mjs'), wrapper);
    await chmod(wrapper, 0o700);
  }
  const npmrc = path.join(base, 'fixture.npmrc'), trace = path.join(logs, 'local-external-edges.jsonl');
  await writeFile(npmrc, '');
  const environment = { ...tools, PATH: [bin, path.dirname(process.execPath), tools.PATH].join(path.delimiter),
    HOME: home, USERPROFILE: home, CODEX_HOME: codex, GESTALT_HOME: gestalt, TMPDIR: temp,
    CONTEXT_MODE_PACKAGE_MANAGER: 'npm', GESTALT_SKILLS_SOURCE: fixture.collection,
    npm_config_cache: path.join(base, 'npm-cache'), npm_config_userconfig: npmrc, npm_config_globalconfig: '/dev/null',
    npm_config_ignore_scripts: 'true', npm_config_install_links: 'true', npm_config_update_notifier: 'false',
    UNIX_SETUP_CODEX: actualCodex, UNIX_SETUP_NPM: actualNpm, UNIX_SETUP_AGENTS: localAgents,
    UNIX_SETUP_MOBILE_PACKAGE: fixture.mobilePackage, UNIX_SETUP_TRACE: trace };
  const checks = [];
  const run = async (label, executable, argv, extra = {}, expected = 0) => {
    const startedAt = new Date().toISOString();
    const result = spawnSync(executable, argv, { env: { ...environment, ...extra }, cwd: base, shell: false,
      timeout: 720_000, maxBuffer: 16 * 1024 * 1024 });
    for (const key of ['stdout', 'stderr']) await writeFile(path.join(logs, label + '.' + key), result[key] ?? '');
    checks.push({ label, argv: [executable, ...argv], cwd: base, startedAt, observedAt: new Date().toISOString(), status: result.status,
      signal: result.signal, error: result.error?.message ?? null, stdoutSha256: digest(result.stdout ?? ''), stderrSha256: digest(result.stderr ?? '') });
    await writeFile(path.join(logs, 'installed-checks.json'), JSON.stringify(checks, null, 2));
    assert.equal(result.status, expected, result.error?.message ?? result.stderr?.toString());
    return { stdout: result.stdout, stderr: result.stderr?.toString() ?? '' };
  };
  const version = await run('pinned-codex-version', path.join(bin, 'codex'), ['--version']);
  assert.equal(version.stdout.toString().trim(), 'codex-cli 0.164.0');
  return { base, codex, gestalt, manager, localAgents, agents, logs, trace, run, ...fixture,
    allowCleanup: () => { retained = false; } };
}

if (process.platform !== 'win32') test('actual compiled Unix Bash replaces its packaged asset, provisions pinned Agents and repeats offline', { timeout: 1_200_000 }, async t => {
  const f = await unixInstalledFixture(t, 'unix-public-installed');
  const backend = path.join(f.base, 'legacy/gestalt');
  const compiledSha256 = digest(await readFile(f.manager)), previous = await readFile(backend);
  const replacement = Buffer.concat([previous, Buffer.from('\n# installed Unix Bash replacement fixture\n')]);
  const asset = path.join(f.release, 'gestalt');
  await writeFile(asset, replacement);
  const release = pathToFileURL(asset);
  const readable = await f.run('controlled-release-curl', 'curl', ['--fail', '--silent', '--show-error', release.href]);
  assert.deepEqual(readable.stdout, replacement);
  for (const home of [f.codex, f.gestalt]) await assert.rejects(stat(home), { code: 'ENOENT' });

  const installed = await f.run('compiled-unix-standalone-update', process.execPath, [f.manager, 'update', 'gestalt'],
    { GESTALT_INSTALL_BASE_URL: pathToFileURL(f.release).href });
  assert.equal((installed.stderr.match(/checking for a manager update/g) ?? []).length, 1);
  assert.match(installed.stderr, /continuing update with the newly installed manager/);
  assert.deepEqual(await readFile(backend), replacement);
  assert.equal(digest(await readFile(f.manager)), compiledSha256);
  assert.equal((await stat(backend)).mode & 0o777, 0o755);
  for (const home of [f.codex, f.gestalt]) {
    const info = await stat(home);
    assert.equal(info.mode & 0o777, 0o700);
    assert.equal(info.uid, process.getuid());
    assert.deepEqual((await readdir(home)).filter(name => name.startsWith('.gestalt-setup.') || ['.gestalt-manager-setup.lock', '.gestalt-setup.lock'].includes(name)), []);
  }
  assert.equal(await readFile(path.join(f.codex, 'skills/native-update/SKILL.md'), 'utf8'), f.skill);
  const receipt = JSON.parse(await readFile(path.join(f.codex, 'gestalt-skills/receipt.json'), 'utf8'));
  assert.equal(receipt.verifiedSource, false);
  const mobile = JSON.parse(await readFile(path.join(f.gestalt, 'mobile/node_modules/gestalt-mobile/package.json'), 'utf8'));
  assert.equal(mobile.version, '0.0.0');
  const marketplace = path.join(f.codex, '.tmp', 'marketplaces', 'dyne-gestalt-agents');
  assert.equal(await realpath(marketplace), f.localAgents);
  const originalShell = f.agents.files.find(file => file.relative === 'gestalt-setup.sh');
  assert.equal(digest(await readFile(path.join(marketplace, 'gestalt-setup.sh'))), originalShell.sha256);

  const outputs = ['config.toml', 'hooks.json', 'bin/context-mode-mcp.mjs', 'bin/context-mode-hook.mjs', '.gestalt-setup-provenance.json'];
  const snapshot = async () => Object.fromEntries(await Promise.all(outputs.map(async name => [name, digest(await readFile(path.join(f.codex, name)))])));
  const before = await snapshot();
  const provenance = JSON.parse(await readFile(path.join(f.codex, '.gestalt-setup-provenance.json'), 'utf8'));
  assert.equal(provenance.manager.sha256, digest(replacement));
  assert.equal(provenance.delegatedSetupSha256, originalShell.sha256);
  assert.equal(provenance.outputs['config.toml'], before['config.toml']);
  assert.equal(provenance.outputs['hooks.json'], before['hooks.json']);
  assert.equal(provenance.scope, 'setup-artifacts-only');
  assert.equal(provenance.runtimeEnforcement, false);
  assert.equal(Object.hasOwn(provenance, 'delegatedApi'), false);
  const prepared = JSON.parse(await readFile(path.join(f.gestalt, 'runtime/context-mode/.context-mode-prepared.json'), 'utf8'));
  assert.equal(prepared.platform, process.platform); assert.equal(prepared.arch, process.arch); assert.equal(prepared.nodeModulesAbi, process.versions.modules);
  const org = await f.run('installed-org-help', path.join(f.codex, 'bin/org-plan'), ['--help']);
  assert.match(org.stdout.toString(), /org-plan/i);
  await f.run('compiled-unix-agents-repeat-offline', process.execPath, [f.manager, 'agents', 'update'], { npm_config_offline: 'true' });
  assert.deepEqual(await snapshot(), before);
  for (const item of f.agents.files) assert.equal(digest(await readFile(path.join(f.localAgents, item.relative))), item.sha256, item.relative);
  const edges = (await readFile(f.trace, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  assert.equal(edges.filter(edge => edge.event === 'local-mobile-package').length, 1);
  assert.equal(edges.filter(edge => edge.event === 'local-marketplace-add').length, 1);
  assert.equal(edges.filter(edge => edge.event === 'local-marketplace-verified').length, 2);
  await writeFile(path.join(f.logs, 'installed-identity.json'), JSON.stringify({ observedAt: new Date().toISOString(),
    backend: 'compiled-to-unix-bash', compiledSha256, previousBashSha256: digest(previous), replacementBashSha256: digest(replacement),
    agentsHead: f.agents.head, agentsTree: f.agents.tree, installedMarketplace: marketplace, provenance, outputs: before,
    runtime: prepared, localMarketplace: true, mobileFixtureVersion: mobile.version, releaseReadByCurl: true, repeat: 'offline-success' }, null, 2));
  f.allowCleanup();
});

if (process.platform === 'darwin') test('actual macOS managed JavaScript API retains unknown authority from pinned Agents', { timeout: 1_200_000 }, async t => {
  const f = await unixInstalledFixture(t, 'macos-managed-installed');
  await buildNativePlatform(root, path.join(f.base, 'native'));
  const ownershipHelper = await buildOwnedProcess(root, path.join(f.base, 'native'));
  await copyFile(ownershipHelper + '.provenance.json', path.join(f.logs, 'ownership-helper.provenance.json'));
  const managed = path.join(root, 'tests/fixtures/run-managed-setup.mjs');
  const source = path.join(f.localAgents, 'gestalt-setup.mjs');
  const owner = path.join(f.localAgents, 'scripts/setup-agents.mjs');
  const environment = { GESTALT_AGENTS_SOURCE: f.localAgents,
    GESTALT_AGENTS_SETUP_SHA256: digest(await readFile(source)), GESTALT_AGENTS_SETUP_OWNER_SHA256: digest(await readFile(owner)) };
  const outcome = path.join(f.logs, 'managed-outcome.json');
  const installed = await f.run('managed-agents-install', process.execPath, [managed, f.manager, ownershipHelper, '__updated-manager', 'agents', 'install'],
    { ...environment, FIXTURE_MANAGED_OUTCOME: outcome });
  assert.match(installed.stderr, /complete descendant absence is unknown/);
  assert.deepEqual(JSON.parse(await readFile(outcome, 'utf8')), { apiVersion: 1, status: 0, ownership: { kind: 'unknown' } });
  const retained = { codex: await captureSetupRetention(f.codex), gestalt: await captureSetupRetention(f.gestalt) };
  await writeFile(path.join(f.logs, 'retained-before-repeat.json'), JSON.stringify(retained, null, 2));
  const [stage] = retained.codex.managerSnapshots;
  assert.equal(retained.codex.managerSnapshots.length, 1);
  assert.equal(stage.snapshot.present, false);
  assert.equal(stage.ownership.schemaVersion, 1);
  assert.equal(stage.ownership.status, 'command-succeeded-cleanup-unknown');
  assert.equal(stage.ownership.delegated.mode, 'managed');
  assert.equal(stage.ownership.delegated.ownership.kind, 'unknown');
  assert.ok(stage.ownership.delegated.retained.length > 0);
  const prepared = JSON.parse(await readFile(path.join(f.gestalt, 'runtime/context-mode/.context-mode-prepared.json'), 'utf8'));
  assert.equal(prepared.platform, process.platform);
  assert.equal(prepared.arch, process.arch);
  assert.equal(prepared.nodeModulesAbi, process.versions.modules);
  assert.equal(retained.codex.managerLock.version, 1);
  assert.equal(typeof retained.codex.managerLock.token, 'string');
  assert.equal(retained.codex.entries.some(entry => entry.path === '.gestalt-setup-provenance.json'), false);
  for (const home of [retained.codex, retained.gestalt]) {
    assert.equal(home.agentsSnapshots.length, 1);
    const owner = home.agentsSnapshots[0].owner;
    assert.equal(owner.schemaVersion, 1); assert.equal(owner.root, home.root);
    assert.ok(Number.isInteger(owner.pid) && owner.pid > 0);
    assert.ok(owner.records.length > 0);
    assert.ok(owner.records.every(record => typeof record.relative === 'string' && typeof record.present === 'boolean' && Number.isInteger(record.index)));
    assert.ok(Array.isArray(home.agentsLock));
    assert.ok(home.agentsLock.some(entry => entry.name.startsWith('owner-') && entry.record.schemaVersion === 1 && entry.record.pid === owner.pid));
    assert.ok(home.agentsLock.some(entry => entry.name.startsWith('command-') && entry.record.schemaVersion === 1 && entry.record.mode === 'managed'));
  }
  const blocked = await f.run('managed-repeat-refused', process.execPath, [managed, f.manager, ownershipHelper, '__updated-manager', 'agents', 'update'], environment, 1);
  assert.match(blocked.stderr, /EEXIST|already running/);
  const after = { codex: await captureSetupRetention(f.codex), gestalt: await captureSetupRetention(f.gestalt) };
  await writeFile(path.join(f.logs, 'retained-after-repeat.json'), JSON.stringify(after, null, 2));
  assert.deepEqual(after, retained);
  // Verified unknown authority is deliberately retained for inspection.
});
