import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { randomUUID, createHash } from 'node:crypto';
import { chmod, copyFile, lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { createServer as createHttpServer } from 'node:http';
import { createInterface } from 'node:readline';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { createScratchOwner } from '../src/cli/runtime/scratch-owner.mjs';
import { capturedNodeCommand } from './fixtures/native-node-command.mjs';
import { NativeArchives } from '../src/cli/platform/archive.ts';
import { NativeExecutables } from '../src/cli/platform/executables.ts';
import { captureCopyTree } from '../scripts/native-copy-provenance.mjs';
import { until, windowsObserver } from './fixtures/native-owned-observer.mjs';
import { SetupContext } from '../src/cli/setup/context.ts';
import { restartWorker } from '../src/cli/setup/restart.ts';
import { parse, stringify } from 'smol-toml';
import { pinnedCodex } from './fixtures/native-runtime/codex.mjs';
import { authorizeFixtureHooks } from './fixtures/native-cli-hook-trust.mjs';
import { verifyOwnershipBuildInputs } from './fixtures/native-runtime/source-binding.mjs';

const repository = path.resolve(import.meta.dirname, '..');
assert.equal(process.platform, 'win32', 'Native Node installed runtime is a Windows gate; Unix exercises its unchanged legacy backend and the compiled compatibility entry');
assert.ok(process.env.GESTALT_NATIVE_RUNTIME_ARTIFACTS, 'Build artifacts outside the consumer token with native-core-slice.mjs --prepare');
const artifactRoot = await realpath(process.env.GESTALT_NATIVE_RUNTIME_ARTIFACTS);
const artifacts = JSON.parse(await readFile(path.join(artifactRoot, 'artifacts.json'), 'utf8'));
assert.equal(artifacts.platform, process.platform); assert.equal(artifacts.arch, process.arch); assert.equal(artifacts.abi, process.versions.modules);
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
for (const file of artifacts.artifacts) {
  assert.ok(!path.isAbsolute(file.relative) && !file.relative.split(/[\\/]/).includes('..'));
  const bytes = await readFile(path.join(artifactRoot, file.relative));
  assert.equal(bytes.length, file.bytes); assert.equal(hash(bytes), file.sha256);
}
const build = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt-runtime-build-')));
const entry = path.join(artifactRoot, 'gestalt.mjs'), peer = null;
assert.equal(artifacts.helper, path.join('native', 'gestalt-owned-process.exe'));
const helper = path.join(artifactRoot, artifacts.helper);
const helperProvenance = JSON.parse(await readFile(helper + '.provenance.json', 'utf8'));
assert.equal(helperProvenance.artifactSha256, hash(await readFile(helper)));
assert.equal(helperProvenance.sourceSha256, hash(await readFile(path.join(repository, 'src/cli/platform/native/owned-process.c'))));
const helperInputs = await verifyOwnershipBuildInputs(helperProvenance, repository);
assert.deepEqual(helperInputs, artifacts.nativeHelperInputs);
assert.ok(['e98a4cd15f945a7749d02e071126a854376b93c0', '8827347387ee32e2c5bd7eb912f9b80185cdad45'].includes(artifacts.orgSource.head));
assert.ok(process.env.GESTALT_NATIVE_AGENTS_SOURCE && path.isAbsolute(process.env.GESTALT_NATIVE_AGENTS_SOURCE), 'exact accepted Agents source is required');
const agentsRoot = await realpath(process.env.GESTALT_NATIVE_AGENTS_SOURCE);
for (const file of [...artifacts.orgSource.files, ...artifacts.orgSource.setupApi]) {
  const bytes = await readFile(path.join(agentsRoot, file.path)); assert.equal(bytes.length, file.bytes); assert.equal(hash(bytes), file.sha256);
}
if (process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE) {
  const directory = process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE; await mkdir(directory, { recursive: true });
  await copyFile(entry, path.join(directory, 'runtime-gestalt.mjs'));
  for (const binary of [helper, peer].filter(Boolean)) {
    await copyFile(binary, path.join(directory, path.basename(binary)));
    await copyFile(binary + '.provenance.json', path.join(directory, path.basename(binary) + '.provenance.json'));
  }
  await writeFile(path.join(directory, 'runtime-build.json'), JSON.stringify({ observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch,
    node: process.version, abi: process.versions.modules, managerSha256: hash(await readFile(entry)), helperSha256: hash(await readFile(helper)),
    peerSha256: peer ? hash(await readFile(peer)) : null, consumerCompilerRequired: false, sourceBuilderSeparateFromConsumer: true }, null, 2));
}
test.after(() => rm(build, { recursive: true, force: true }));
async function evidence(name, value) {
  if (!process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE) return;
  await mkdir(process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE, { recursive: true });
  await writeFile(path.join(process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE, name), JSON.stringify(value, null, 2));
}
async function fixture(t) {
  const outer = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt native runtime ü ')));
  let retained = process.platform === 'darwin';
  t.after(() => retained ? undefined : rm(outer, { recursive: true, force: true }));
  const storage = await NativePrivateStorage.open(path.join(outer, 'private'), peer), root = storage.root;
  for (const directory of ['home', 'Codex', 'Gestalt', 'bin', 'consumer', 'consumer/native', 'workspace']) await storage.createDirectory(path.join(root, directory));
  const manager = path.join(root, 'consumer/gestalt.mjs'); await copyFile(entry, manager);
  for (const file of [helper, peer].filter(Boolean)) { await copyFile(file, path.join(root, 'consumer/native', path.basename(file))); if (process.platform !== 'win32') await chmod(path.join(root, 'consumer/native', path.basename(file)), 0o755); }
  const environment = { ...process.env, HOME: path.join(root, 'home'), USERPROFILE: path.join(root, 'home'), CODEX_HOME: path.join(root, 'Codex'), GESTALT_HOME: path.join(root, 'Gestalt'),
    PATH: [path.join(root, 'bin'), path.dirname(process.execPath), path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0')].join(path.delimiter), PATHEXT: '.CMD;.EXE' };
  for (const key of Object.keys(environment)) if (key.toUpperCase() === 'PATH' && key !== 'PATH') delete environment[key];
  return { root, storage, manager, environment, retain: () => { retained = true; } };
}
async function command(f, name, program) {
  const file = path.join(f.root, 'bin', name + '.mjs'); await writeFile(file, '#!/usr/bin/env node\n' + program, { mode: 0o755 });
  if (process.platform !== 'win32') { const target = path.join(f.root, 'bin', name); await copyFile(file, target); await chmod(target, 0o755); return target; }
  const capture = JSON.parse(await readFile(path.join(repository, 'tests/fixtures/native-npm-node-shim.json'), 'utf8'));
  const target = path.join(f.root, 'bin', name + '.cmd'); await writeFile(target, capturedNodeCommand(capture, name + '.mjs')); return target;
}
async function appServer(t, f, native, environment = f.environment, argv = ['--stdio', '', 'literal ü & $HOME']) {
  const child = spawn(process.execPath, [f.manager, '__codex-app-server', native, ...argv],
    { cwd: f.root, env: environment, stdio: ['pipe', 'pipe', 'pipe'], shell: false });
  let stderr = ''; child.stderr.on('data', bytes => stderr += bytes);
  const closed = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', (status, signal) => resolve({ status, signal, stderr })); });
  const waiters = new Map(), lines = createInterface({ input: child.stdout });
  lines.on('line', line => { const message = JSON.parse(line); waiters.get(message.id)?.(message); });
  const lifecycle = { startedAt: new Date().toISOString(), watchdogAt: null };
  const timer = setTimeout(() => { lifecycle.watchdogAt = new Date().toISOString(); f.retain(); child.kill('SIGKILL'); }, process.platform === 'win32' ? 300000 : 60000);
  t.after(async () => { clearTimeout(timer); if (child.exitCode === null && child.signalCode === null) child.stdin.end(); await closed; lines.close(); });
  return { child, closed, lifecycle, request: (id, method, params = {}) => new Promise((resolve, reject) => {
    waiters.set(id, message => { waiters.delete(id); resolve(message); });
    void closed.then(result => { if (waiters.delete(id)) reject(new Error(`native AppServer exited before reply ${id}: ${JSON.stringify(result)}`)); }, reject);
    child.stdin.write(JSON.stringify({ id, method, params }) + '\n');
  }) };
}
async function managerCommand(f, argv, environment = f.environment, deadline = process.platform === 'win32' ? 300000 : 60000, boundary) {
  const startedAt = new Date().toISOString(), started = performance.now(); let observedStatus, observedFailure;
  const child = spawn(process.execPath, [f.manager, ...argv], { cwd: f.root, env: environment, stdio: ['ignore', 'pipe', 'pipe'], shell: false });
  const stdout = [], stderr = []; child.stdout.on('data', bytes => stdout.push(bytes)); child.stderr.on('data', bytes => stderr.push(bytes));
  let timer; try {
    const status = await new Promise((resolve, reject) => {
      timer = setTimeout(() => { f.retain(); child.kill('SIGKILL'); reject(new Error('installed runtime command did not settle')); }, deadline);
      child.once('error', reject); child.once('close', resolve);
    });
    observedStatus = status;
    return { status, stdout: Buffer.concat(stdout).toString(), stderr: Buffer.concat(stderr).toString() };
  } catch (error) { observedFailure = { message: error.message, code: error.code ?? null }; throw error; }
  finally {
    clearTimeout(timer);
    if (boundary) {
      const raw = chunks => { const bytes = Buffer.concat(chunks); return { totalBytes: bytes.length, truncated: bytes.length > 65536, base64: bytes.subarray(0, 65536).toString('base64') }; };
      await evidence(boundary, { startedAt, observedAt: new Date().toISOString(), elapsedMs: performance.now() - started, argv, executable: process.execPath,
        cwd: f.root, status: observedStatus ?? null, failure: observedFailure ?? null, stdout: raw(stdout), stderr: raw(stderr),
        inherited: Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].map(key => [key, environment[key] ?? null])),
        scope: 'Actual command boundary; nonzero/unknown preserved. No inferred inner result, model adoption or release.' });
    }
  }
}
async function profiles(f) {
  const result = [];
  for (const root of ['home', 'Codex', 'Gestalt']) {
    const directory = path.join(f.root, root), tree = await captureCopyTree(directory);
    for (const entry of [{ path: '', kind: 'directory' }, ...tree]) {
      const info = await lstat(path.join(directory, entry.path), { bigint: true });
      result.push({ root, ...entry, mode: String(info.mode), uid: String(info.uid), gid: String(info.gid), nlink: String(info.nlink), mtimeNs: String(info.mtimeNs) });
    }
  }
  return result;
}
async function nativeCodexIdentity() {
  if (!nativeCodexIdentity.pending) nativeCodexIdentity.pending = (async () => {
    const artifact = await pinnedCodex(path.join(build, 'codex-artifact'));
    const commandDirectory = path.join(build, 'codex-bin'); await mkdir(commandDirectory);
    const executable = path.join(commandDirectory, 'codex.exe'); await copyFile(artifact.binary, executable);
    assert.equal(hash(await readFile(executable)), artifact.record.binary.sha256);
    return { executable, commandDirectory, entry: executable, entrySha256: artifact.record.binary.sha256, packageVersion: '0.164.0',
      binary: executable, binarySha256: artifact.record.binary.sha256, binaryMode: (await lstat(executable)).mode,
      source: artifact.record, scope: 'Pinned test dependency only; no product version admission policy' };
  })();
  return nativeCodexIdentity.pending;
}
async function trustInstalledFixtureHooks(t, f, codex, agents, workspace) {
  const hooksFile = path.join(f.root, 'Codex/hooks.json'), launcher = path.join(f.root, 'Codex/bin/context-mode-hook.mjs');
  const { reconcileCodexHooks, reconcileCodexToml } = await import(pathToFileURL(path.join(agents, 'plugins/context-mode/scripts/configure-codex.mjs')));
  const expected = JSON.parse(reconcileCodexHooks('', launcher)); assert.deepEqual(JSON.parse(await readFile(hooksFile, 'utf8')), expected);
  const marketplace = JSON.parse(await readFile(path.join(agents, '.agents/plugins/marketplace.json'), 'utf8')).name;
  const installed = path.join(f.root, 'Codex/plugins/cache', marketplace, 'context-mode/2.26.0/hooks/runtime-hook.mjs');
  assert.deepEqual(await readFile(installed), await readFile(path.join(agents, 'plugins/context-mode/hooks/runtime-hook.mjs')));
  assert.equal(await readFile(launcher, 'utf8'), `#!/usr/bin/env node\nimport { pathToFileURL } from "node:url";\n\nprocess.env.GESTALT_HOME ||= ${JSON.stringify(path.join(f.root, 'Gestalt'))};\nprocess.env.CONTEXT_MODE_PLATFORM = "codex";\nawait import(pathToFileURL(${JSON.stringify(installed)}).href);\n`);
  const entries = Object.entries(expected.hooks).flatMap(([event, groups]) => groups.flatMap(group => group.hooks.map(hook =>
    ({ eventName: event[0].toLowerCase() + event.slice(1), sourcePath: hooksFile, command: hook.command }))));
  assert.equal(entries.length, 6);
  const query = async () => {
    const proxy = await appServer(t, f, codex.executable, f.environment, ['--stdio']);
    await proxy.request(1, 'initialize', { clientInfo: { name: 'native-runtime-fixture-trust', version: '1' }, capabilities: {} });
    proxy.child.stdin.write('{"method":"initialized","params":{}}\n');
    const result = await proxy.request(2, 'hooks/list', { cwds: [workspace] }); proxy.child.stdin.end();
    assert.equal((await proxy.closed).status, 0); assert.equal(result.error, undefined, JSON.stringify(result));
    return result.result.data.find(entry => entry.cwd === workspace).hooks;
  };
  const authorized = authorizeFixtureHooks(await query(), entries), configFile = path.join(f.root, 'Codex/config.toml');
  const config = parse(await readFile(configFile, 'utf8')); config.hooks ??= {}; config.hooks.state ??= {};
  for (const hook of authorized) config.hooks.state[hook.key] = { enabled: true, trusted_hash: hook.currentHash };
  // Fixture serialization must preserve the accepted managed MCP canonical
  // form before establishing the strict post-session byte baseline.
  const canonical = reconcileCodexToml(stringify(config), {
    launcherPath: path.join(f.root, 'Codex/bin/context-mode-mcp.mjs'), pluginId: `context-mode@${marketplace}`,
  });
  assert.deepEqual(parse(canonical), config, 'Fixture canonicalization must not change any config value');
  await f.storage.replaceFile(configFile, Buffer.from(canonical));
  const current = await query(); assert.deepEqual(authorizeFixtureHooks(current, entries), authorized);
  assert.ok(current.every(hook => hook.trustStatus === 'trusted'));
  await evidence('runtime-fixture-hook-trust.json', { scope: 'Only six exact verified isolated accepted hooks; no live or global trust change', authorized });
  return readFile(configFile);
}
test('native restart completion refuses missing authenticated authority and preserves the live relay/descriptor', { timeout: 600000 }, async t => {
  const f = await fixture(t), run = path.join(f.root, 'Gestalt/run'); await f.storage.createDirectory(run);
  const relay = spawn(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { stdio: 'ignore' });
  const closed = new Promise(resolve => relay.once('close', resolve)); t.after(async () => { if (relay.exitCode === null) relay.kill(); await closed; });
  const descriptor = path.join(run, `mobile-${relay.pid}.restart`), bytes = Buffer.from([f.root, '--port', 'space ü &', ''].join('\0'));
  await f.storage.replaceFile(descriptor, bytes); const lock = path.join(run, 'update-restart.lock'); await f.storage.createDirectory(lock);
  const identity = await lstat(lock, { bigint: true }), token = randomUUID();
  await f.storage.replaceFile(path.join(lock, 'owner.json'), Buffer.from(JSON.stringify({ schemaVersion: 1, token, dev: String(identity.dev), ino: String(identity.ino) })));
  const streams = { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} };
  const context = new SetupContext({ argv: [], cwd: f.root, environment: f.environment }, streams,
    { userHome: f.environment.HOME, codexHome: f.environment.CODEX_HOME, gestaltHome: f.environment.GESTALT_HOME, marketplace: 'fixture', marketplaceName: 'fixture', manager: f.manager, unixProbe: peer || undefined, ownershipHelper: helper }, '0.16.1');
  let updates = 0;
  const status = await restartWorker(context, [String(relay.pid), descriptor, 'false', path.join(f.root, 'Gestalt/update-restart.log'), lock, 'gestalt', token], async () => { updates++; return false; });
  assert.equal(status, 1); assert.equal(updates, 1); assert.equal(relay.exitCode, null); assert.equal(relay.signalCode, null); assert.deepEqual(await readFile(descriptor), bytes);
  await evidence('restart-authority-refusal.json', { observedAt: new Date().toISOString(), platform: process.platform, status, updates, relayPreserved: true, descriptorSha256: hash(bytes), authority: 'unavailable; no PID signalling' });
});
test('actual runtime interruption fences detached descendants or retains unknown scratch authority', { timeout: 600000 }, async t => {
  const f = await fixture(t), owner = createScratchOwner(peer), primary = owner.scratchAllocate(path.join(f.root, 'workspace'), 'workspace-service', 'runtime-interruption-test');
  // Keep both sides of the durable v2 association together for inspection.
  // Release assertions below concern leases, never deletion of user payload.
  f.retain();
  const token = randomUUID(), ready = path.join(f.root, 'descendant-ready.json'), heartbeat = path.join(f.root, 'descendant-writes'), release = path.join(f.root, 'release-descendant'), done = path.join(f.root, 'descendant-done');
  const descendant = `const fs=require('node:fs');process.on('SIGTERM',()=>{});process.on('SIGINT',()=>{});fs.writeFileSync(${JSON.stringify(ready)},JSON.stringify({pid:process.pid,token:${JSON.stringify(token)}}));const interval=setInterval(()=>{if(fs.existsSync(${JSON.stringify(release)})){clearInterval(interval);fs.writeFileSync(${JSON.stringify(done)},'done');process.exit(0);}fs.appendFileSync(${JSON.stringify(heartbeat)},${JSON.stringify(token)}+'\\n');},40);setTimeout(()=>process.exit(0),600000);`;
  const native = await command(f, 'codex-interrupt', `import {spawn} from 'node:child_process';import {createInterface} from 'node:readline';const worker=spawn(process.execPath,['-e',${JSON.stringify(descendant)}],{detached:true,stdio:'ignore'});worker.unref();createInterface({input:process.stdin}).on('line',()=>{});setInterval(()=>{},1000);`);
  const environment = owner.scratchEnvironment({ ...f.environment, GESTALT_SESSION_TMP_CONTEXT: JSON.stringify(primary.context), GESTALT_RUNTIME_DESCENDANT: token });
  const proxy = await appServer(t, f, native, environment); let observer, held, activated = false;
  try {
    await until(async () => { try { return JSON.parse(await readFile(ready, 'utf8')).token === token && (await readFile(heartbeat, 'utf8')).split('\n').length >= 3; } catch (error) { if (error.code === 'ENOENT') return false; throw error; } }, 'runtime detached descendant did not establish actual writes', process.platform === 'win32' ? 300000 : 60000);
    activated = true;
    if (process.platform === 'win32') { observer = await windowsObserver(JSON.parse(await readFile(ready, 'utf8')).pid); held = observer.identity; }
    proxy.child.kill(process.platform === 'win32' ? 'SIGKILL' : 'SIGTERM'); const result = await proxy.closed;
    if (process.platform !== 'win32') assert.equal(result.status, 143, result.stderr);
    if (observer) await observer.verify();
    const before = await readFile(heartbeat, 'utf8'); await new Promise(resolve => setTimeout(resolve, 400)); const after = await readFile(heartbeat, 'utf8'), fenced = before === after;
    const leases = await Promise.all((await readdir(path.join(primary.context.directory, 'leases'))).map(async file => JSON.parse(await readFile(path.join(primary.context.directory, 'leases', file), 'utf8'))));
    const borrowers = leases.filter(lease => lease.leaseId !== primary.lease.leaseId); assert.ok(borrowers.length > 0);
    f.retain(); assert.ok(borrowers.every(lease => lease.state === 'active')); assert.equal(fenced, true);
    await evidence('runtime-interruption.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version, actualInterruption: process.platform === 'win32' ? 'TerminateProcess parent death' : 'SIGTERM',
      result, heldCreationIdentities: held ?? null, writesFenced: fenced, borrowers, primaryRetained: true, helperSha256: hash(await readFile(helper)), injectedSignal: false });
  } finally {
    await writeFile(release, token); if (proxy.child.exitCode === null && proxy.child.signalCode === null) proxy.child.kill(process.platform === 'win32' ? 'SIGKILL' : 'SIGTERM'); await proxy.closed;
    if (process.platform === 'darwin' && activated) await until(async () => { try { return await readFile(done, 'utf8') === 'done'; } catch (error) { if (error.code === 'ENOENT') return false; throw error; } }, 'controlled detached Mac writer did not acknowledge explicit fixture release');
    if (observer) await observer.verify();
  }
});
test('actual compiled AppServer uses the target native Codex protocol and preserves a custom isolated model/profile', { timeout: 600000 }, async t => {
  const f = await fixture(t), codex = await nativeCodexIdentity();
  await f.storage.replaceFile(path.join(f.root, 'Codex/config.toml'), Buffer.from('model="operator-selected-model"\napproval_policy="on-request"\n'));
  const before = await readFile(path.join(f.root, 'Codex/config.toml'));
  const proxy = await appServer(t, f, codex.executable, f.environment, ['--stdio']), initialized = await proxy.request(1, 'initialize', { clientInfo: { name: 'gestalt-native-runtime-contract', version: '1' }, capabilities: {} });
  assert.ok(initialized.result?.userAgent, JSON.stringify(initialized));
  proxy.child.stdin.write('{"method":"initialized","params":{}}\n');
  const config = await proxy.request(2, 'config/read', { includeLayers: false });
  assert.equal(config.result?.config?.model, 'operator-selected-model', JSON.stringify(config));
  proxy.child.stdin.end(); const result = await proxy.closed; assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(await readFile(path.join(f.root, 'Codex/config.toml')), before);
  await evidence('actual-codex-app-server.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version,
    codex, managerSha256: hash(await readFile(f.manager)), helperSha256: hash(await readFile(helper)), initialized, model: config.result.config.model, configSha256: hash(before), result,
    modelRequestIssued: false, configuredModelAdoptionClaimed: false });
});
test('actual compiled CLI uses accepted Agents, fresh/resume native sessions and a local synthetic Responses fixture', { timeout: 1800000 }, async t => {
  const f = await fixture(t), codex = await nativeCodexIdentity(), agents = process.env.GESTALT_NATIVE_AGENTS_SOURCE;
  assert.ok(agents && path.isAbsolute(agents), 'exact accepted Agents dependency is required');
  const setupEnvironment = { ...f.environment, PATH: [codex.commandDirectory, process.env.PATH].join(path.delimiter), GESTALT_AGENTS_SOURCE: await realpath(agents),
    GESTALT_AGENTS_SETUP_SHA256: hash(await readFile(path.join(agents, 'gestalt-setup.mjs'))),
    GESTALT_AGENTS_SETUP_OWNER_SHA256: hash(await readFile(path.join(agents, 'scripts/setup-agents.mjs'))),
    CLAUDE_CONFIG_DIR: path.join(f.root, 'home/.claude'), CONTEXT_MODE_DIR: path.join(f.root, 'home/context-mode-data'),
    CONTEXT_MODE_PACKAGE_MANAGER: 'npm', npm_config_ignore_scripts: 'true', CONTEXT_MODE_PLATFORM: 'codex', CONTEXT_MODE_WORKSPACE: f.root };
  for (const key of Object.keys(setupEnvironment)) if (key.startsWith('GESTALT_MOBILE_ORG_PLAN_')) delete setupEnvironment[key];
  const setup = await managerCommand(f, ['agents', 'install'], setupEnvironment, 1080000);
  await evidence('actual-cli-agents-setup.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version, codex, setup,
    managerSha256: hash(await readFile(f.manager)), setupEntrySha256: setupEnvironment.GESTALT_AGENTS_SETUP_SHA256, setupOwnerSha256: setupEnvironment.GESTALT_AGENTS_SETUP_OWNER_SHA256 });
  assert.equal(setup.status, 0, setup.stderr);
  f.environment.PATH = [codex.commandDirectory, f.environment.PATH].join(path.delimiter);
  const before = await readFile(path.join(f.root, 'Codex/config.toml'));
  const result = await managerCommand(f, ['cli', '--', 'exec', '--help'], f.environment, 540000);
  await evidence('actual-cli-native.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version, codex, result,
    managerSha256: hash(await readFile(f.manager)), helperSha256: hash(await readFile(helper)), profileSha256Before: hash(before),
    profileSha256After: hash(await readFile(path.join(f.root, 'Codex/config.toml'))), consumerPath: f.environment.PATH, modelRequestIssued: false, modelAdoptionClaimed: false });
  assert.equal(result.status, 0, result.stderr); assert.match(result.stdout, /Usage:.*codex exec/s);
  assert.deepEqual(await readFile(path.join(f.root, 'Codex/config.toml')), before);
  // Real native session/hook/resume behavior, using only a local synthetic
  // Responses fixture. No real model/authentication or model adoption claim.
  const requests = [], metadataRequests = [], model = createHttpServer(async (request, response) => {
    let raw = ''; for await (const chunk of request) { raw += chunk; if (raw.length > 16 * 1024 * 1024) { response.writeHead(413); response.end(); return; } }
    if (!raw && (request.method === 'HEAD' && request.url === '/v1/responses' || request.method === 'GET' && request.url === '/v1/models')) {
      metadataRequests.push({ method: request.method, path: request.url, bodyBytes: 0, observedAt: new Date().toISOString() });
      response.writeHead(200, { 'Content-Type': 'application/json' }); response.end(JSON.stringify({ data: [], models: [] })); return;
    }
    if (request.method !== 'POST' || request.url !== '/v1/responses') { response.writeHead(400); response.end('unexpected fixture request'); return; }
    requests.push(JSON.parse(raw));
    const item = { id: 'msg_' + requests.length, type: 'message', status: 'completed', role: 'assistant', content: [{ type: 'output_text', text: 'Native fixture completed.', annotations: [] }] };
    const result = { id: 'resp_' + requests.length, object: 'response', created_at: 1, status: 'completed', model: 'fixture', output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
    response.writeHead(200, { 'Content-Type': 'text/event-stream' });
    for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } }, { type: 'response.output_item.added', output_index: 0, item },
      { type: 'response.output_item.done', output_index: 0, item }, { type: 'response.completed', response: result }]) response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
    response.end();
  });
  await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
  t.after(() => { model.closeAllConnections(); return new Promise(resolve => model.close(resolve)); });
  const workspace = path.join(f.root, 'workspace'), config = parse(before.toString());
  config.model = 'fixture'; config.model_provider = 'fixture'; config.model_providers = { fixture: { name: 'fixture', base_url: `http://127.0.0.1:${model.address().port}/v1`, wire_api: 'responses', requires_openai_auth: false } };
  config.projects = { [workspace]: { trust_level: 'trusted' } }; config.features = { ...config.features, hooks: true, tool_search: false };
  await f.storage.replaceFile(path.join(f.root, 'Codex/config.toml'), Buffer.from(stringify(config)));
  const profile = await trustInstalledFixtureHooks(t, f, codex, agents, workspace);
  f.retain(); // preserve v2 logical/physical associations even on an assertion failure
  const observedEnvironment = { ...f.environment, NODE_OPTIONS: '--import=' + pathToFileURL(path.join(artifactRoot, 'spawn-capture.mjs')).href,
    GESTALT_SLICE_CODEX_BINARY: codex.executable, GESTALT_SLICE_SPAWN_CAPTURE: path.join(process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE || build, 'fresh-native-spawn.jsonl') };
  const fresh = await managerCommand(f, ['cli', '--', 'exec', '--skip-git-repo-check', '--json', '-C', workspace, 'NATIVE_FRESH'], observedEnvironment, 540000, 'fresh-command-boundary.json');
  await evidence('actual-cli-fresh.json', { observedAt: new Date().toISOString(), platform: process.platform, fresh, syntheticModel: true, realModelAdoptionClaimed: false });
  assert.equal(fresh.status, 0, fresh.stderr);
  const events = fresh.stdout.split('\n').flatMap(line => { try { return [JSON.parse(line)]; } catch { return []; } });
  const threadId = events.find(value => value.type === 'thread.started')?.thread_id; assert.ok(threadId, 'native thread.started identity is required');
  const resumed = await managerCommand(f, ['cli', '--', '-C', workspace, 'exec', 'resume', '--skip-git-repo-check', '--json', threadId, 'NATIVE_RESUME'], f.environment, 540000);
  await evidence('actual-cli-resume.json', { observedAt: new Date().toISOString(), platform: process.platform, threadId, resumed, syntheticModel: true, realModelAdoptionClaimed: false });
  assert.equal(resumed.status, 0, resumed.stderr); assert.ok(requests.length >= 2, 'actual native turns must reach the synthetic fixture');
  const markers = [];
  for (const id of await readdir(path.join(workspace, 'tmp/sessions'))) if (id.startsWith('cli-')) markers.push(JSON.parse(await readFile(path.join(workspace, 'tmp/sessions', id, '.gestalt-owner.json'), 'utf8')));
  const associated = markers.filter(marker => marker.providerThreads.some(value => value.provider === 'codex' && value.threadId === threadId));
  assert.equal(associated.length, 2, 'fresh and resume must associate the actual native thread with distinct invocation owners'); assert.notEqual(associated[0].ownerId, associated[1].ownerId);
  const afterProfile = await readFile(path.join(f.root, 'Codex/config.toml'));
  const retainedBytes = bytes => ({ bytes: bytes.length, sha256: hash(bytes), truncated: bytes.length > 65536, base64: bytes.subarray(0, 65536).toString('base64') });
  await evidence('actual-cli-session-observations.json', { observedAt: new Date().toISOString(), platform: process.platform,
    phase: 'before-strict-profile-byte-assertion', profileByteAssertion: 'not-yet-executed', threadId, associated,
    freshStatus: fresh.status, resumeStatus: resumed.status, syntheticModelRequests: requests.length, metadataRequests,
    syntheticModel: true, realModelAdoptionClaimed: false, profileBefore: retainedBytes(profile), profileAfter: retainedBytes(afterProfile) });
  assert.deepEqual(await readFile(path.join(f.root, 'Codex/config.toml')), profile);
  f.retain(); // keep logical markers and physical runtime associations together
  await evidence('actual-cli-session-associations.json', { observedAt: new Date().toISOString(), platform: process.platform, threadId, associated, syntheticModelRequests: requests.length,
    syntheticModel: true, realModelAdoptionClaimed: false, metadataRequests, profileSha256: hash(profile), helperSha256: hash(await readFile(helper)) });
});
test('actual native Codex OS sandbox succeeds without an unconfined fallback or other-platform sandbox prerequisite', { timeout: 600000 }, async t => {
  const f = await fixture(t), codex = await nativeCodexIdentity();
  f.environment.PATH = [codex.commandDirectory, f.environment.PATH].join(path.delimiter);
  const result = await managerCommand(f, ['doctor', '--probe-sandbox']);
  await evidence('actual-codex-sandbox.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version, codex,
    managerSha256: hash(await readFile(f.manager)), helperSha256: hash(await readFile(helper)), result, fallbackAllowed: false });
  // Missing optional installation components still make doctor nonzero. The
  // native positive claim comes only from the executed Codex sandbox probe.
  assert.match(result.stdout, /^Codex sandbox\s+ready$/m, result.stderr + '\n' + result.stdout);
  if (process.platform !== 'linux') assert.doesNotMatch(result.stdout + result.stderr, /Bubblewrap.*(?:required|MISSING)/);
});
test('actual compiled skills export/import relocates profiles and preserves unrelated user data on a native private namespace', { timeout: 900000 }, async t => {
  const source = await fixture(t), target = await fixture(t);
  for (const directory of ['home/.agents', 'home/.agents/skills', 'home/.agents/skills/native-skill', 'Gestalt/skill-profiles', 'Codex/skills', 'Codex/skills/.system']) await source.storage.createDirectory(path.join(source.root, directory));
  const user = path.join(source.root, 'home/.agents/skills');
  await source.storage.replaceFile(path.join(user, 'native-skill/SKILL.md'), Buffer.from('# native skill\n'));
  await source.storage.replaceFile(path.join(source.root, 'Codex/skills/.system/operator-data'), Buffer.from('never exported'));
  await source.storage.replaceFile(path.join(source.root, 'Gestalt/skill-profiles/default.yaml'), Buffer.from(`skills:\n  - ${user}/native-skill\n`));
  const archive = path.join(source.root, 'skills ü.tar.gz'), exported = await managerCommand(source, ['skills-export', archive]);
  assert.equal(exported.status, 0, exported.stderr);
  assert.ok((await new NativeArchives().list(archive)).every(member => !member.path.includes('/.system')));
  await target.storage.createDirectory(path.join(target.root, 'Codex/skills')); await target.storage.createDirectory(path.join(target.root, 'Codex/skills/unrelated'));
  await target.storage.replaceFile(path.join(target.root, 'Codex/skills/unrelated/user-data'), Buffer.from('preserve exactly'));
  const imported = await managerCommand(target, ['skills-import', archive]); assert.equal(imported.status, 0, imported.stderr);
  assert.equal(await readFile(path.join(target.root, 'home/.agents/skills/native-skill/SKILL.md'), 'utf8'), '# native skill\n');
  const profile = await readFile(path.join(target.root, 'Gestalt/skill-profiles/default.yaml'), 'utf8');
  assert.ok(profile.includes(path.join(target.root, 'home/.agents/skills'))); assert.ok(!profile.includes(user));
  assert.equal(await readFile(path.join(target.root, 'Codex/skills/unrelated/user-data'), 'utf8'), 'preserve exactly');
  await evidence('skills-transfer-native.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version,
    managerSha256: hash(await readFile(source.manager)), archiveSha256: hash(await readFile(archive)), exported, imported, profile });
});
test('actual compiled skills install/update/status preserve receipt integrity and refuse local modifications', { timeout: 600000 }, async t => {
  const f = await fixture(t), collection = path.join(f.root, 'collection'), relative = 'skills/native-lifecycle/SKILL.md';
  for (const directory of ['collection', 'collection/skills', 'collection/skills/native-lifecycle']) await f.storage.createDirectory(path.join(f.root, directory));
  const noHooks = path.join(f.root, 'fixture-no-hooks'); await f.storage.createDirectory(noHooks);
  const git = await new NativeExecutables().resolve('git', process.env); assert.ok(git, 'native fixture Git is required');
  const environment = { ...f.environment, PATH: process.env.PATH, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: 'NUL', GIT_TERMINAL_PROMPT: '0' };
  const runGit = args => execFileSync(git, ['-c', 'user.name=Native fixture', '-c', 'user.email=native@example.invalid', '-c', 'core.hooksPath=' + noHooks, ...args],
    { cwd: collection, env: environment, shell: false, stdio: 'pipe' });
  runGit(['init', '--initial-branch=main']);
  const revision = async (version, text) => {
    await f.storage.replaceFile(path.join(collection, relative), Buffer.from(text));
    await f.storage.replaceFile(path.join(collection, 'LICENSE'), Buffer.from('MIT fixture\n'));
    await f.storage.replaceFile(path.join(collection, 'manifest.json'), Buffer.from(JSON.stringify({ schemaVersion: 1, repository: 'dyne/gestalt-skills',
      source: 'https://github.com/dyne/gestalt-skills', version, skills: [{ name: 'native-lifecycle', entry: relative, files: [{ path: relative, sha256: hash(Buffer.from(text)) }] }] })));
    runGit(['add', '--', 'manifest.json', 'LICENSE', relative]); runGit(['commit', '-m', version]);
  };
  const selected = { ...f.environment, PATH: process.env.PATH, GESTALT_SKILLS_SOURCE: collection, GESTALT_SKILLS_REF: 'main' };
  await revision('0.1.0', '# first\n'); const installed = await managerCommand(f, ['skills', 'install'], selected);
  assert.equal(installed.status, 0, installed.stderr);
  const status = await managerCommand(f, ['skills', 'status', '--json'], selected);
  assert.equal(status.status, 0, status.stderr); assert.equal(JSON.parse(status.stdout).version, '0.1.0');
  await revision('0.2.0', '# second\n'); const updated = await managerCommand(f, ['skills', 'update'], selected);
  assert.equal(updated.status, 0, updated.stderr);
  const receiptFile = path.join(f.root, 'Codex/gestalt-skills/receipt.json'), receipt = await readFile(receiptFile);
  assert.equal(JSON.parse(receipt).version, '0.2.0');
  const discovered = path.join(f.root, 'Codex/skills/native-lifecycle/SKILL.md'); assert.equal(await readFile(discovered, 'utf8'), '# second\n');
  await f.storage.replaceFile(path.join(f.root, 'Codex/gestalt-skills', relative), Buffer.from('operator changes\n'));
  const refused = await managerCommand(f, ['skills', 'update'], selected); assert.equal(refused.status, 1); assert.match(refused.stderr, /local changes/);
  assert.equal(await readFile(discovered, 'utf8'), 'operator changes\n'); assert.deepEqual(await readFile(receiptFile), receipt);
  await evidence('skills-lifecycle-native.json', { observedAt: new Date().toISOString(), platform: process.platform, managerSha256: hash(await readFile(f.manager)),
    installed, status, updated, refused, receiptSha256: hash(receipt), sourceOverride: true, verifiedPublicSource: false, packageContentsExecuted: false });
});
test('actual compiled scratch inspection is read-only and exact offline recovery requires confirmation', { timeout: 600000 }, async t => {
  const f = await fixture(t), workspace = path.join(f.root, 'workspace'), sessionId = randomUUID(), ownerId = 'mobile-' + hash(Buffer.from(sessionId));
  for (const directory of ['workspace/tmp', 'workspace/tmp/sessions', path.join('workspace/tmp/sessions', ownerId)]) await f.storage.createDirectory(path.join(f.root, directory));
  const directory = path.join(workspace, 'tmp/sessions', ownerId), marker = path.join(directory, '.gestalt-owner.json'), fence = path.join(directory, '.containment-lock');
  const markerBytes = Buffer.from(JSON.stringify({ schemaVersion: 1, ownerKind: 'mobile-session', ownerId, workspacePath: workspace, sessionId,
    createdAt: new Date().toISOString(), authority: { kind: 'mobile-roster', sessionId }, providerThreads: [] }));
  await f.storage.replaceFile(marker, markerBytes);
  await f.storage.replaceFile(path.join(directory, 'user-payload'), Buffer.from('retained user content')); await f.storage.createDirectory(fence);
  const before = await captureCopyTree(directory), identity = await lstat(fence, { bigint: true });
  const inspected = await managerCommand(f, ['scratch', 'inspect', '--cwd', workspace, '--owner', ownerId]); assert.equal(inspected.status, 0, inspected.stderr);
  const token = JSON.parse(inspected.stdout).token; assert.match(token, /^[a-f0-9]{64}$/); assert.deepEqual(await captureCopyTree(directory), before);
  const refused = await managerCommand(f, ['scratch', 'recover', '--cwd', workspace, '--owner', ownerId, '--expect', token]); assert.equal(refused.status, 1);
  const current = await lstat(fence, { bigint: true }); assert.equal(current.ino, identity.ino); assert.equal(current.birthtimeNs, identity.birthtimeNs);
  const recovered = await managerCommand(f, ['scratch', 'recover', '--cwd', workspace, '--owner', ownerId, '--expect', token, '--confirm-quiescent']);
  assert.equal(recovered.status, 0, recovered.stderr); assert.equal(JSON.parse(recovered.stdout).status, 'recovered');
  assert.equal(await readFile(path.join(directory, 'user-payload'), 'utf8'), 'retained user content'); assert.deepEqual(await readFile(marker), markerBytes);
  await assert.rejects(lstat(fence), { code: 'ENOENT' });
  await evidence('scratch-recovery-native.json', { observedAt: new Date().toISOString(), platform: process.platform, inspected, refused, recovered, token,
    markerSha256: hash(await readFile(marker)), payloadRetained: true, operatorQuiescenceOverride: true, processAbsenceInferred: false });
});
test('actual compiled diagnostics leave isolated profiles/configuration unchanged and reject unknown native capabilities', { timeout: 600000 }, async t => {
  const f = await fixture(t); await f.storage.replaceFile(path.join(f.root, 'Codex/config.toml'), Buffer.from('model="operator-selected"\n'));
  await f.storage.replaceFile(path.join(f.root, 'Gestalt/operator-data'), Buffer.from('retain user data'));
  const before = { config: await readFile(path.join(f.root, 'Codex/config.toml')), data: await readFile(path.join(f.root, 'Gestalt/operator-data')), inventory: await profiles(f) };
  const results = [];
  for (const argv of [['path', 'codex-home'], ['version'], ['doctor'], ['mobile', '--port', 'literal ü &']]) results.push({ argv, ...await managerCommand(f, argv) });
  assert.equal(results[0].status, 0, results[0].stderr); assert.equal(results[1].status, 0, results[1].stderr);
  assert.equal(results[2].status, 1); assert.equal(results[3].status, 1); assert.match(results[3].stderr, /native.*Mobile host|native win32 gestalt-mobile host/);
  assert.deepEqual(await readFile(path.join(f.root, 'Codex/config.toml')), before.config); assert.deepEqual(await readFile(path.join(f.root, 'Gestalt/operator-data')), before.data);
  assert.deepEqual(await profiles(f), before.inventory);
  await evidence('diagnostics-native.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version, managerSha256: hash(await readFile(f.manager)), results,
    configSha256: hash(before.config), userDataSha256: hash(before.data), beforeInventory: before.inventory, afterInventory: await profiles(f), modelChanged: false, installationAttempted: false });
});
test('actual compiled AppServer forwards early protocol, large stdout and command result without a consumer compiler', { timeout: 360000 }, async t => {
  const f = await fixture(t), leafMarker = path.join(f.root, 'leaf-before-exit7.json'), native = await command(f, 'codex', `import {createInterface} from 'node:readline';import {writeFileSync} from 'node:fs';
createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);if(m.method==='exit'){writeFileSync(${JSON.stringify(leafMarker)},JSON.stringify({pid:process.pid,status:7,observedAt:new Date().toISOString()}),{flag:'wx',mode:0o600});process.exit(7);return;}process.stdout.write(JSON.stringify({id:m.id,result:{argv:process.argv.slice(2),history:m.method==='thread/read'?'x'.repeat(2*1024*1024):'',home:process.env.CODEX_HOME}})+'\\n');});`);
  const proxy = await appServer(t, f, native), first = await proxy.request(1, 'initialize');
  assert.deepEqual(first.result.argv, ['app-server', '--stdio', '', 'literal ü & $HOME']); assert.equal(first.result.home, f.environment.CODEX_HOME);
  assert.equal((await proxy.request(2, 'thread/read')).result.history.length, 2 * 1024 * 1024);
  proxy.child.stdin.write('{"method":"exit"}\n'); const result = await proxy.closed;
  let leaf, leafReadError; try { leaf = JSON.parse(await readFile(leafMarker, 'utf8')); } catch (error) { leafReadError = error.message; }
  await evidence('app-server-failure-boundary.json', { observedAt: new Date().toISOString(), lifecycle: proxy.lifecycle, result, leaf: leaf ?? null, leafReadError: leafReadError ?? null,
    historyBytes: 2 * 1024 * 1024, callerInputEnded: proxy.child.stdin.writableEnded, scope: 'Pre-exit leaf marker is not a native result/release proof; watchdog and null/7 result remain literal' });
  assert.equal(result.status, 7, result.stderr);
  await evidence('app-server-native.json', { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version, abi: process.versions.modules,
    managerSha256: hash(await readFile(f.manager)), helperSha256: hash(await readFile(helper)), argv: first.result.argv, historyBytes: 2 * 1024 * 1024, result,
    consumerPath: f.environment.PATH, sourceCopied: false, runtimeCompilerRequired: false });
});

test('actual inherited owned boundary preserves canonical Agents filesystem guards and authenticated host IPC', { timeout: 600000 }, async t => {
  const f = await fixture(t), agents = process.env.GESTALT_NATIVE_AGENTS_SOURCE;
  assert.ok(agents && path.isAbsolute(agents), 'exact accepted Agents source is required');
  const owner = createScratchOwner(peer), primary = owner.scratchAllocate(path.join(f.root, 'workspace'), 'workspace-service', 'runtime-integration-test');
  f.retain();
  const environment = owner.scratchEnvironment({ ...f.environment, GESTALT_SESSION_TMP_CONTEXT: JSON.stringify(primary.context) });
  let endpoint, expected, server, pipe;
  if (process.platform === 'win32') {
    const name = 'gestalt-' + randomUUID(); endpoint = '\\\\.\\pipe\\' + name;
    const script = `$ErrorActionPreference='Stop';$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User;$acl=New-Object IO.Pipes.PipeSecurity;$acl.SetAccessRuleProtection($true,$false);$acl.SetOwner($sid);$acl.AddAccessRule((New-Object IO.Pipes.PipeAccessRule($sid,'FullControl','Allow')));$p=New-Object IO.Pipes.NamedPipeServerStream('${name}',[IO.Pipes.PipeDirection]::InOut,1,[IO.Pipes.PipeTransmissionMode]::Byte,[IO.Pipes.PipeOptions]::None,4096,4096,$acl);$me=[Diagnostics.Process]::GetCurrentProcess();[Console]::WriteLine((@{pid=$PID;started=$me.StartTime.ToUniversalTime().Ticks.ToString();executable=$me.MainModule.FileName}|ConvertTo-Json -Compress));$p.WaitForConnection();$r=New-Object IO.StreamReader($p);$w=New-Object IO.StreamWriter($p);$w.AutoFlush=$true;$line=$r.ReadLine();$w.Write('pong'+[char]10);$p.Dispose();`;
    pipe = spawn(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'), ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], { stdio: ['ignore', 'pipe', 'pipe'] });
    const lines = createInterface({ input: pipe.stdout }); expected = await new Promise((resolve, reject) => { lines.once('line', line => resolve(JSON.parse(line))); pipe.once('error', reject); pipe.once('exit', code => reject(new Error('pipe owner exited before readiness: ' + code))); });
    t.after(() => { lines.close(); if (pipe.exitCode === null) pipe.kill(); });
  } else {
    endpoint = path.join(primary.context.runtime.directory, 'broker.sock');
    expected = JSON.parse(execFileSync(peer, ['--identity', String(process.pid)], { encoding: 'utf8' }));
    server = createServer(socket => { socket.once('data', () => socket.end('pong\n')); });
    await new Promise((resolve, reject) => { server.once('error', reject); server.listen(endpoint, resolve); }); await chmod(endpoint, 0o600);
    t.after(() => new Promise(resolve => server.close(resolve)));
  }
  const orgStorage = pathToFileURL(path.join(agents, 'plugins/gestalt/skills/org-plan/scripts/org-plan-storage.mjs')).href;
  const native = await command(f, 'codex-probe', `import fs from 'node:fs';import {createInterface} from 'node:readline';
import {ensureManagedRoot} from ${JSON.stringify(orgStorage)};
import {NativePrivateStorage} from ${JSON.stringify(pathToFileURL(path.join(repository, 'src/cli/platform/storage.ts')).href)};
import {NativeLocalIpc} from ${JSON.stringify(pathToFileURL(path.join(repository, 'src/cli/platform/ipc.ts')).href)};
createInterface({input:process.stdin}).on('line',async line=>{const m=JSON.parse(line);const result={uid:process.getuid?.(),gid:process.getgid?.(),ancestors:['/',...(process.platform==='win32'?[]:['/tmp'])].filter(p=>fs.existsSync(p)).map(p=>({path:p,uid:fs.statSync(p).uid,gid:fs.statSync(p).gid,mode:fs.statSync(p).mode})),context:JSON.parse(process.env.GESTALT_SESSION_TMP_CONTEXT),tmp:process.env.TMPDIR};
try{ensureManagedRoot(process.env.CODEX_HOME,{privateMode:false,create:false});result.agentsGuard='ready';}catch(e){result.agentsGuard={error:e.message};}
try{const storage=await NativePrivateStorage.open(${JSON.stringify(process.platform === 'win32' ? f.root : path.dirname(endpoint))},${JSON.stringify(peer)});const ipc=await new NativeLocalIpc(storage,${JSON.stringify(peer)}).connect(${JSON.stringify(endpoint)},${JSON.stringify(expected)});await ipc.stdout(Buffer.from('ping\\n'));for await(const bytes of ipc.stdin){result.ipc=Buffer.from(bytes).toString();break;}}catch(e){result.ipc={error:e.message};}
if(process.platform==='linux'){result.uidMap=fs.readFileSync('/proc/self/uid_map','utf8');result.gidMap=fs.readFileSync('/proc/self/gid_map','utf8');}
process.stdout.write(JSON.stringify({id:m.id,result})+'\\n');});`);
  const proxy = await appServer(t, f, native, environment), response = await proxy.request('integration', 'verify-owned-context');
  proxy.child.stdin.end(); const result = await proxy.closed;
  const leases = owner.scratchContext(environment); assert.equal(leases.ownerId, primary.context.ownerId);
  const record = { observedAt: new Date().toISOString(), platform: process.platform, node: process.version, arch: process.arch, abi: process.versions.modules,
    managerSha256: hash(await readFile(f.manager)), helperSha256: hash(await readFile(helper)), expectedPeer: expected, endpoint, response, result };
  await evidence('inherited-owner-integration.json', record);
  if (result.status === 0 && process.platform !== 'darwin') assert.equal(await owner.scratchReleaseReady(primary), true);
  else f.retain();
  assert.equal(response.result.context.ownerId, primary.context.ownerId); assert.equal(response.result.tmp, primary.context.runtime?.directory ?? primary.context.directory);
  assert.equal(response.result.agentsGuard, 'ready', JSON.stringify(record)); assert.equal(response.result.ipc, 'pong\n', JSON.stringify(record));
  assert.equal(result.status, 0, result.stderr);
});
