// Explicit target gate; no downloads, profile setup or model activity in npm test.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { appendFile, chmod, cp, lstat, mkdir, mkdtemp, readFile, realpath, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { createServer as httpServer } from 'node:http';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { PassThrough } from 'node:stream';
import { createInterface } from 'node:readline';
import { pathToFileURL } from 'node:url';
import { build } from 'esbuild';
import { parse, stringify } from 'smol-toml';
import { buildCli } from '../scripts/build-cli.mjs';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { NativeOwnedProcesses } from '../src/cli/platform/processes.ts';
import { createScratchOwner } from '../src/cli/runtime/scratch-owner.mjs';
import { nodeHookCommand } from '../src/cli/runtime/hook-command.mjs';
import { observeHostSocket } from './fixtures/unix-compat-peer.mjs';
import { sessionScratch } from './session-scratch-fixture.mjs';
import { authorizeFixtureHooks } from './fixtures/native-cli-hook-trust.mjs';
import { verifySourceRecord } from './fixtures/native-runtime/source-binding.mjs';
import { bounded, digest, isolatedEnvironment, pinnedCodex } from './fixtures/native-runtime/codex.mjs';

const root = path.resolve(import.meta.dirname, '..'), args = process.argv.slice(2);
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const json = async file => JSON.parse(await readFile(file, 'utf8'));
const record = async (file, value) => writeFile(file, JSON.stringify(value, null, 2) + '\n');
function observeTerminalCase(control, report, terminalReport, attempted, readError) {
  if (!report) return { control, status: attempted ? 'unknown-no-valid-terminal-report' : 'unexecuted',
    terminalReport, readError, fixtureTurnSubmitted: null, barrierTextObserved: null };
  const submitted = typeof report.submittedBlockedFixtureTurn === 'boolean' ? report.submittedBlockedFixtureTurn : null;
  const output = typeof report.outputBase64 === 'string' ? Buffer.from(report.outputBase64, 'base64').toString() : null;
  const blocked = output === null ? null : output.includes('Native core slice no-model barrier');
  return { control, status: 'observed-terminal-report', terminalReport,
    fixtureTurnSubmitted: submitted, barrierTextObserved: blocked,
    controlSent: typeof report.controlSent === 'boolean' ? report.controlSent : null,
    timedOut: typeof report.timedOut === 'boolean' ? report.timedOut : null, result: report.result ?? { exit: report.exit ?? null } };
}
async function sourceBindings(agents, copiedBinding) {
  const binding = await json(path.join(root, 'tests/fixtures/native-runtime/agents-source.json'));
  const head = copiedBinding?.head ?? execFileSync('git', ['-C', agents, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
  assert.ok([binding.acceptedCommit, binding.remoteCheckoutCommit].includes(head));
  const files = [];
  for (const expected of binding.files) {
    const source = path.join(agents, expected.path), bytes = await readFile(source), info = await lstat(source);
    const tree = copiedBinding ? copiedBinding.files.find(file => file.path === expected.path)?.gitTree :
      execFileSync('git', ['-C', agents, 'ls-tree', 'HEAD', '--', expected.path], { encoding: 'utf8' }).trim();
    const actual = { bytes: bytes.length, sha256: sha256(bytes), mode: (info.mode & 0o7777).toString(8), regularFile: info.isFile() && !info.isSymbolicLink() };
    if (process.platform === 'win32') {
      assert.equal(actual.bytes, expected.bytes); assert.equal(actual.sha256, expected.sha256); assert.equal(actual.regularFile, true);
      assert.equal(tree, `${expected.gitMode} blob ${expected.blobOid}\t${expected.path}`);
      files.push({ path: expected.path, ...actual, gitTree: tree, canonicalGitMode: expected.gitMode,
        physicalModeScope: 'Windows stat mode is observed; POSIX executable-bit semantics are not representable' });
    } else files.push({ path: expected.path, gitTree: tree, ...verifySourceRecord(expected, actual, tree) });
  }
  const setupApi = [];
  for (const name of ['gestalt-setup.mjs', 'scripts/setup-agents.mjs', 'plugins/context-mode/scripts/configure-codex.mjs',
    'plugins/context-mode/hooks/runtime-hook.mjs', 'plugins/context-mode/scripts/session-scratch.mjs']) {
    const bytes = await readFile(path.join(agents, name)), actual = { path: name, bytes: bytes.length, sha256: sha256(bytes) };
    const expected = copiedBinding ? copiedBinding.setupApi.find(file => file.path === name) :
      (() => { const committed = execFileSync('git', ['-C', agents, 'show', `${head}:${name}`]); return { bytes: committed.length, sha256: sha256(committed) }; })();
    assert.equal(actual.bytes, expected.bytes); assert.equal(actual.sha256, expected.sha256); setupApi.push(actual);
  }
  return { head, binding, files, setupApi, gitTreeOrigin: copiedBinding ? 'Exact builder checkout records; copied bytes reverified before consumption' : 'Current immutable checkout git ls-tree' };
}

async function prepare(output, agents) {
  await mkdir(path.join(output, 'native'), { recursive: true });
  await buildCli(root, path.join(output, 'gestalt.mjs'));
  await cp(path.join(root, 'tests/fixtures/native-cli-spawn-capture.mjs'), path.join(output, 'spawn-capture.mjs'));
  const compiled = await build({ entryPoints: [path.join(root, 'tests/fixtures/native-cli-authority.mjs')],
    outfile: path.join(output, 'authority.mjs'), bundle: true, platform: 'node', target: 'node24', format: 'esm', metafile: true });
  const peer = await buildNativePlatform(root, path.join(output, 'native'));
  const helper = process.platform === 'win32' ? await buildOwnedProcess(root, path.join(output, 'native')) : null;
  const files = [path.join(output, 'gestalt.mjs'), path.join(output, 'legacy/gestalt'), path.join(output, 'authority.mjs'), path.join(output, 'spawn-capture.mjs'),
    ...[peer, helper].filter(Boolean).flatMap(file => [file, file + '.provenance.json'])];
  const artifacts = await Promise.all(files.map(async file => ({ relative: path.relative(output, file), ...await digest(file) })));
  assert.equal(sha256(await readFile(path.join(output, 'legacy/gestalt'))), sha256(await readFile(path.join(root, 'public/gestalt'))));
  const inputs = await Promise.all(Object.keys(compiled.metafile.inputs).map(file => digest(path.resolve(file))));
  await record(path.join(output, 'artifacts.json'), { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch,
    node: process.version, abi: process.versions.modules, peer: peer ? path.relative(output, peer) : null,
    helper: helper ? path.relative(output, helper) : null, artifacts, authorityBuildInputs: inputs, orgSource: await sourceBindings(agents),
    scope: 'Builder only. The copied compiled consumer requires no compiler, TS source or repository modules.' });
}

async function queryHooks(binary, environment, cwd, overrides = []) {
  const child = spawn(binary, ['app-server', '--stdio', ...overrides], { env: environment, cwd, stdio: ['pipe', 'pipe', 'pipe'], shell: false });
  const closed = once(child, 'close'), lines = createInterface({ input: child.stdout }), pending = new Map(); let id = 0, stderr = '';
  child.stderr.on('data', bytes => { stderr += bytes; });
  lines.on('line', line => { const message = JSON.parse(line); pending.get(message.id)?.(message); pending.delete(message.id); });
  const rpc = (method, params) => bounded(new Promise(resolve => { pending.set(++id, resolve); child.stdin.write(JSON.stringify({ id, method, params }) + '\n'); }), method);
  try {
    await rpc('initialize', { clientInfo: { name: 'gestalt-core-slice-hook-fixture', version: '1' }, capabilities: {} });
    child.stdin.write('{"method":"initialized"}\n');
    const result = await rpc('hooks/list', { cwds: [cwd] }); assert.equal(result.error, undefined, JSON.stringify(result));
    return { result, stderr, overrides };
  } finally { lines.close(); child.stdin.end(); child.kill(); await bounded(closed, 'fixture config query cleanup'); }
}

async function trustFixtureHooks(binary, environment, cwd, configFile, expected) {
  const before = await queryHooks(binary, environment, cwd), hooks = before.result.result?.data?.find(entry => entry.cwd === cwd)?.hooks ?? [];
  const authorized = authorizeFixtureHooks(hooks, expected), config = parse(await readFile(configFile, 'utf8'));
  config.hooks.state ??= {};
  for (const hook of authorized) config.hooks.state[hook.key] = { enabled: true, trusted_hash: hook.currentHash };
  await writeFile(configFile, stringify(config));
  const after = await queryHooks(binary, environment, cwd), current = after.result.result?.data?.find(entry => entry.cwd === cwd)?.hooks ?? [];
  const readback = authorizeFixtureHooks(current, expected);
  assert.deepEqual(readback, authorized);
  for (const hook of current) assert.equal(hook.trustStatus, 'trusted', JSON.stringify(hook));
  return { source: 'Only the exact isolated fixture and six verified accepted installed managed hooks; no live trust change', authorized, before, after };
}

async function verifyInstalledHooks(agents, fixture) {
  const home = path.join(fixture, 'Codex'), hooksFile = path.join(home, 'hooks.json'), launcher = path.join(home, 'bin/context-mode-hook.mjs');
  const { reconcileCodexHooks } = await import(pathToFileURL(path.join(agents, 'plugins/context-mode/scripts/configure-codex.mjs')));
  const expected = JSON.parse(reconcileCodexHooks('', launcher)), actual = await json(hooksFile);
  assert.deepEqual(actual, expected, 'Installed hooks differ from the exact accepted canonical fixture configuration');
  const marketplace = (await json(path.join(agents, '.agents/plugins/marketplace.json'))).name;
  const version = (await json(path.join(agents, 'plugins/context-mode/package.json'))).version;
  const plugin = path.join(home, 'plugins/cache', marketplace, 'context-mode', version);
  const installedOwner = path.join(plugin, 'hooks/runtime-hook.mjs'), acceptedOwner = path.join(agents, 'plugins/context-mode/hooks/runtime-hook.mjs');
  assert.deepEqual(await readFile(installedOwner), await readFile(acceptedOwner));
  const expectedBridge = `#!/usr/bin/env node\nimport { pathToFileURL } from "node:url";\n\nprocess.env.GESTALT_HOME ||= ${JSON.stringify(path.join(fixture, 'Gestalt'))};\nprocess.env.CONTEXT_MODE_PLATFORM = "codex";\nawait import(pathToFileURL(${JSON.stringify(installedOwner)}).href);\n`;
  assert.equal(await readFile(launcher, 'utf8'), expectedBridge, 'Installed hook bridge differs from the accepted fixed launcher');
  const entries = Object.entries(expected.hooks).flatMap(([event, groups]) => groups.flatMap(group => group.hooks.map(hook =>
    ({ eventName: event[0].toLowerCase() + event.slice(1), sourcePath: hooksFile, command: hook.command, kind: 'verified-installed-context-mode' }))));
  assert.equal(entries.length, 6);
  return { entries, proof: { hooksFile: await digest(hooksFile), launcher: await digest(launcher), installedOwner: await digest(installedOwner), acceptedOwner: await digest(acceptedOwner) } };
}

async function composedHooks(binary, environment, cwd, capture) {
  const calls = (await readFile(capture, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  const tui = calls.filter(call => call.argv[0] !== 'app-server' && call.argv.includes('--no-alt-screen')).at(-1);
  assert.ok(tui, 'Actual candidate final Codex argv was not observed');
  const overrides = [];
  for (let i = 0; i < tui.argv.length; i++) {
    if (tui.argv[i] === '-c' || tui.argv[i] === '--config') { assert.ok(i + 1 < tui.argv.length); overrides.push('-c', tui.argv[++i]); }
    else if (tui.argv[i].startsWith('--config=')) overrides.push('-c', tui.argv[i].slice(9));
  }
  const effective = await queryHooks(binary, environment, cwd, overrides), hooks = effective.result.result?.data?.find(entry => entry.cwd === cwd)?.hooks ?? [];
  assert.equal(hooks.filter(hook => hook.source === 'user').length, 7);
  assert.ok(hooks.filter(hook => hook.source === 'sessionFlags').length >= 2, 'Manager-composed hook flags were not exercised');
  for (const hook of hooks) assert.equal(hook.trustStatus, 'trusted', JSON.stringify(hook));
  return { calls, exactFinalArgv: tui.argv, effective,
    scope: 'Actual captured final candidate argv, then exact -c flags queried through real Codex hooks/list; no protocol/stream interception or unscoped trust' };
}

async function hostIpc(fixture, peer, evidence, environment) {
  if (process.platform === 'win32') {
    const name = 'gestalt-' + randomUUID(), script = path.join(root, 'tests/fixtures/native-cli-slice-pipe.ps1');
    const child = spawn(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'),
      ['-NoLogo', '-NoProfile', '-NonInteractive', '-File', script],
      { env: { ...environment, GESTALT_PIPE_FIXTURE: Buffer.from(JSON.stringify({ name })).toString('base64') }, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    const closed = once(child, 'close'); let stdout = '', stderr = ''; child.stderr.on('data', bytes => { stderr += bytes; });
    const ready = new Promise(resolve => child.stdout.on('data', bytes => { stdout += bytes; if (stdout.includes('\n')) resolve(JSON.parse(stdout.split(/\r?\n/)[0])); }));
    let host;
    try { host = await bounded(Promise.race([ready, closed.then(() => { throw Error(`Private pipe host ended before readiness: ${stderr}`); })]), 'private host pipe readiness'); }
    catch (error) {
      if (child.exitCode === null) child.kill(); await bounded(closed, 'failed private pipe fixture cleanup');
      await record(path.join(evidence, 'host-start-failure.json'), { stdout, stderr, message: error.message }); throw error;
    }
    return { endpoint: '\\\\.\\pipe\\' + name, host, async stop() {
      if (child.exitCode === null) child.kill(); const [exit, signal] = await bounded(closed, 'test pipe host cleanup');
      await record(path.join(evidence, 'host-ipc.json'), { host, exit, signal, stderr, scope: 'Private owner DACL and whole NativeLocalIpc expected-host authentication' });
    } };
  }
  const endpoint = path.join(fixture, 'host.sock');
  assert.ok(Buffer.byteLength(endpoint) <= 103, 'Use the test-created private fixture path without changing owner TMP');
  const host = JSON.parse(execFileSync(peer, ['--identity', String(process.pid)], { encoding: 'utf8' })), sockets = new Set(), measurements = [], pending = [];
  const server = createServer(socket => {
    sockets.add(socket); socket.on('close', () => sockets.delete(socket)); socket.on('error', () => {});
    const measurement = observeHostSocket(peer, socket).then(value => { measurements.push(value); return value; }); pending.push(measurement);
    socket.on('data', bytes => { void measurement.then(value => { if (value.exit === 0 && !socket.destroyed) socket.end(bytes); }); });
  });
  server.listen(endpoint); await once(server, 'listening'); await chmod(endpoint, 0o600);
  return { endpoint, host, measurements, async stop() {
    for (const socket of sockets) socket.destroy(); await new Promise(resolve => server.close(resolve)); await Promise.all(pending);
    await record(path.join(evidence, 'host-ipc.json'), { host, measurements, scope: 'Whole NativeLocalIpc exchange plus independent host-side same-socket credentials' });
  } };
}

async function terminal(entry, environment, cwd, report, evidence, helper, control) {
  if (process.platform !== 'win32') {
    const request = path.join(evidence, 'terminal-request.json'), result = path.join(evidence, 'terminal.json');
    await record(request, { command: [process.execPath, entry, 'cli', '--no-alt-screen', '--sandbox', 'read-only', '--ask-for-approval', 'never', '-C', cwd], environment, cwd, report, result, control });
    const child = spawn('python3', [path.join(root, 'tests/fixtures/native-cli-slice-pty.py'), request], { shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '', stderr = ''; child.stdout.on('data', b => { stdout += b; }); child.stderr.on('data', b => { stderr += b; });
    const [exit, signal] = await bounded(once(child, 'close'), 'compiled Unix consumer terminal', 190000);
    assert.equal(exit, 0, JSON.stringify({ signal, stdout, stderr })); return json(result);
  }
  const input = new PassThrough(), chunks = []; let tail = '', submitted = false, controlSent = false, lease, failure;
  const adapter = new NativeOwnedProcesses(helper);
  try {
    lease = await adapter.launchTerminal(process.execPath,
      { argv: [entry, 'cli', '--no-alt-screen', '--sandbox', 'read-only', '--ask-for-approval', 'never', '-C', cwd], cwd, environment },
      { input, output: async bytes => {
        chunks.push(Buffer.from(bytes)); const query = tail + Buffer.from(bytes).toString('latin1');
        for (const _ of query.matchAll(/\x1b\[6n/g)) input.write('\x1b[1;1R'); tail = query.slice(-3);
        const output = Buffer.concat(chunks).toString();
        if (!submitted && output.includes('7501;state=idle')) { input.write('Native core slice; trusted hook must block provider I/O\r'); submitted = true; }
        if (!controlSent && output.includes('Native core slice no-model barrier')) { input.write(control === 'eof' ? '\x04' : '\x03\x03'); controlSent = true; }
      } }, { columns: 110, rows: 40 }, { graceMs: 1000, deadlineMs: 10000 });
    const outcome = await bounded(lease.result, 'actual compiled Windows consumer TUI', 300000);
    const claim = await lease.stop(); await lease.completed;
    const result = { result: outcome, claim, submittedBlockedFixtureTurn: submitted, control, controlSent,
      outputBase64: Buffer.concat(chunks).toString('base64'), observedAt: new Date().toISOString() };
    await record(path.join(evidence, 'terminal.json'), result);
    assert.equal(submitted, true); assert.equal(controlSent, true); assert.deepEqual(outcome, { kind: 'exit', code: 0 }); assert.equal(claim.kind, 'released');
    return result;
  } catch (error) { failure = { message: error.message, stack: error.stack }; throw error; }
  finally {
    if (lease) {
      const claim = await lease.stop();
      await record(path.join(evidence, 'terminal-final.json'), { claim, failure: failure ?? null, submittedBlockedFixtureTurn: submitted, control, controlSent,
        outputBase64: Buffer.concat(chunks).toString('base64'), observedAt: new Date().toISOString() });
      await bounded(lease.completed, 'native consumer helper closure', 15000);
    }
    input.destroy();
  }
}

async function consume(artifacts, agents, evidence) {
  await mkdir(evidence, { recursive: true, mode: 0o700 });
  const manifest = await json(path.join(artifacts, 'artifacts.json'));
  assert.equal(manifest.platform, process.platform); assert.equal(manifest.arch, process.arch); assert.equal(manifest.abi, process.versions.modules);
  for (const expected of manifest.artifacts) { const actual = await digest(path.join(artifacts, expected.relative)); assert.equal(actual.sha256, expected.sha256); assert.equal(actual.bytes, expected.bytes); }
  const binding = await sourceBindings(agents, manifest.orgSource); await record(path.join(evidence, 'accepted-agents-source.json'), binding);
  // Preserve the caller's validated TMP/context. Missing privacy is a hard
  // target failure, not an all-skipped gate or a remapped local TMP workaround.
  const outer = await realpath(await mkdtemp(path.join(tmpdir(), 'gc-'))), probe = manifest.peer ? path.join(artifacts, manifest.peer) : undefined;
  const storage = await NativePrivateStorage.open(path.join(outer, 'private'), probe), fixture = storage.root;
  await record(path.join(evidence, 'fixture.json'), { outer, fixture, originalOwnerContext: Object.fromEntries(
    ['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].map(key => [key, process.env[key] ?? null])), observedAt: new Date().toISOString() });
  for (const name of ['home', 'Codex', 'Gestalt', 'bin', 'workspace', 'status']) await storage.createDirectory(path.join(fixture, name));
  const consumer = path.join(fixture, 'consumer'); await cp(artifacts, consumer, { recursive: true });
  const entry = path.join(consumer, 'gestalt.mjs'), peer = manifest.peer ? path.join(consumer, manifest.peer) : undefined, helper = manifest.helper ? path.join(consumer, manifest.helper) : undefined;
  const artifact = await pinnedCodex(path.join(evidence, 'codex-artifact'), { homeDirectory: path.join(fixture, 'Codex') });
  await cp(artifact.binary, path.join(fixture, 'bin', process.platform === 'win32' ? 'codex.exe' : 'codex'));
  if (process.platform !== 'win32') await chmod(path.join(fixture, 'bin/codex'), 0o700);
  const isolated = isolatedEnvironment(path.join(fixture, 'Codex'));
  for (const key of Object.keys(isolated)) if (key.startsWith('GESTALT_') && key !== 'GESTALT_SESSION_TMP_CONTEXT') delete isolated[key];
  const environment = { ...isolated, HOME: path.join(fixture, 'home'), USERPROFILE: path.join(fixture, 'home'), NODE_OPTIONS: '', NODE_PATH: '',
    GESTALT_HOME: path.join(fixture, 'Gestalt'), PATH: [path.join(fixture, 'bin'), path.dirname(process.execPath), process.env.PATH].join(path.delimiter),
    npm_config_ignore_scripts: 'true', CONTEXT_MODE_PACKAGE_MANAGER: 'npm', CLAUDE_CONFIG_DIR: path.join(fixture, 'home/.claude'), CONTEXT_MODE_DIR: path.join(fixture, 'home/context-mode-data') };
  for (const key of Object.keys(environment)) if (key.toUpperCase() === 'PATH' && key !== 'PATH') delete environment[key];
  // Setup is the accepted dependency's explicit standalone fixture mode, with
  // its honest no-complete-tree result. It is not a manager fallback or gate.
  const setupApi = await import(pathToFileURL(path.join(agents, 'gestalt-setup.mjs')));
  assert.equal(setupApi.SETUP_API_VERSION, 1); assert.equal(typeof setupApi.setupAgents, 'function');
  for (const name of ['host-filesystem-v1', 'async-owned-commands-v1', 'shared-context-v1']) assert.ok(setupApi.SETUP_API_CAPABILITIES.includes(name));
  let setup, setupFailure;
  const log = bytes => appendFile(path.join(evidence, 'fixture-setup.log'), bytes);
  try { setup = await setupApi.setupAgents([], { mode: 'standalone', environment, cwd: fixture,
    streams: { stdin: (async function* () {})(), stdout: log, stderr: log } }); }
  catch (error) { setupFailure = { message: error.message, stack: error.stack }; throw error; }
  finally { await record(path.join(evidence, 'fixture-setup.json'), { outcome: setup ?? null, failure: setupFailure ?? null,
    scope: 'Isolated accepted Agents fixture preparation, not a new product managed release proof' }); }
  const dispatcher = await readFile(path.join(fixture, 'Codex/bin/org-plan'), 'utf8'), bundle = /bundles\/([a-f0-9-]{36})\/org-plan-cli\.mjs/.exec(dispatcher)?.[1];
  assert.ok(bundle, 'Actual installed dispatcher must pin one immutable Org bundle');
  const orgBundle = path.join(fixture, 'Codex/lib/gestalt-org-plan/bundles', bundle), configFile = path.join(fixture, 'Codex/config.toml');
  const owner = createScratchOwner(peer);
  // Accepted context-mode2.26 is a v1 consumer. Exercise the existing Unix
  // inherited-owner fixture, without converting or weakening any real v2 owner.
  const primary = process.platform === 'win32' ? owner.scratchAllocate(path.join(fixture, 'workspace'), 'workspace-service', 'core-consumer-slice') :
    await sessionScratch(path.join(fixture, 'workspace'), 'core-consumer-slice-' + randomUUID());
  const selected = owner.scratchEnvironment({ ...environment, GESTALT_SESSION_TMP_CONTEXT: JSON.stringify(primary.context),
    GESTALT_MOBILE_ORG_PLAN_STATUS_DIRECTORY: path.join(fixture, 'status') });
  const inherited = Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].map(key => [key, selected[key] ?? null]));
  const installedHooks = await verifyInstalledHooks(agents, fixture);
  await record(path.join(evidence, 'verified-installed-hooks.json'), installedHooks);
  const plan = path.join(fixture, 'probe.org');
  await writeFile(plan, ['#+TITLE: Private core slice', '#+SUBTITLE: Disposable test only', '#+DATE: 2026-11-16', '#+KEYWORDS: native core slice',
    '* TODO [#A] Probe', ':PROPERTIES:', ':ID: slice-probe', ':SKILLS: $gestalt:coding', ':REVIEW_STATUS: UNREVIEWED', ':END:',
    '- Effort :: Small.', '- Goal :: Actual private publication.', '- Notes :: Not a session plan.',
    ...['first', 'second'].flatMap(id => ['** TODO [#A] ' + id, ':PROPERTIES:', ':ID: slice-' + id, ':END:', '- Why :: Test inherited authority.', '- Change :: Disposable fixture.', '- Tests :: Actual publication and IPC.', '- Done when :: Exchange completes.'])].join('\n') + '\n', { mode: 0o600 });
  let requests = [];
  const attemptedCases = new Set();
  const sentinel = httpServer((request, response) => { requests.push({ method: request.method, path: request.url }); response.writeHead(503); response.end('No model service in this fixture'); });
  sentinel.listen(0, '127.0.0.1'); await once(sentinel, 'listening');
  try {
    for (const control of ['eof', 'interrupt']) {
      attemptedCases.add(control);
      const current = path.join(evidence, control); await mkdir(current);
      const host = await hostIpc(fixture, peer, current, selected), report = path.join(current, 'authority.json'), configuration = path.join(current, 'configuration.json');
      try {
        await record(configuration, { fixture, peerProbe: peer, host: host.host, endpoint: host.endpoint, orgBundle, orgFiles: binding.binding.files, plan, report, inherited });
        const config = parse(await readFile(configFile, 'utf8'));
        config.model_provider = 'native-no-model-fixture'; config.model = 'fixture-unused'; config.check_for_update_on_startup = false;
        config.model_providers = { 'native-no-model-fixture': { name: 'No model request fixture', base_url: `http://127.0.0.1:${sentinel.address().port}`, wire_api: 'responses', requires_openai_auth: false } };
        config.projects ??= {}; config.projects[path.join(fixture, 'workspace')] = { trust_level: 'trusted' }; config.features ??= {}; config.features.hooks = true;
        config.hooks ??= {};
        config.hooks.SessionStart = [{ hooks: [{ type: 'command', timeout: 120,
          command: nodeHookCommand(`await import(${JSON.stringify(pathToFileURL(path.join(consumer, 'authority.mjs')).href)});`) }] }];
        await writeFile(configFile, stringify(config));
        const launchEnvironment = { ...selected, GESTALT_SLICE_CONFIGURATION: configuration };
        const expected = [...installedHooks.entries, { eventName: 'sessionStart', sourcePath: configFile, command: config.hooks.SessionStart[0].hooks[0].command, kind: 'explicit-blocking-fixture' }];
        await record(path.join(current, 'fixture-hook-trust.json'), await trustFixtureHooks(artifact.binary, launchEnvironment, path.join(fixture, 'workspace'), configFile, expected));
        const capture = path.join(current, 'candidate-spawn.jsonl');
        const observedEnvironment = { ...launchEnvironment, NODE_OPTIONS: '--import=' + pathToFileURL(path.join(consumer, 'spawn-capture.mjs')).href,
          GESTALT_SLICE_CODEX_BINARY: path.join(fixture, 'bin', process.platform === 'win32' ? 'codex.exe' : 'codex'), GESTALT_SLICE_SPAWN_CAPTURE: capture };
        let result, terminalFailure;
        try { result = await terminal(entry, observedEnvironment, path.join(fixture, 'workspace'), report, current, helper, control); }
        catch (error) { terminalFailure = error; throw error; }
        finally {
          try { await record(path.join(current, 'effective-manager-hooks.json'), await composedHooks(artifact.binary, launchEnvironment, path.join(fixture, 'workspace'), capture)); }
          catch (error) { await record(path.join(current, 'effective-manager-hooks-failure.json'), { message: error.message, stack: error.stack }); if (!terminalFailure) throw error; }
        }
        const authority = await json(report); assert.equal(authority.pass, true, JSON.stringify(authority.error)); assert.equal(authority.installedBindings.length, 15);
        if (process.platform !== 'win32') assert.ok(host.measurements.some(value => value.exit === 0 &&
          value.peer.pid === authority.identity.pid && value.peer.started === authority.identity.started &&
          value.peer.executable === authority.identity.executable && value.peer.uid === process.getuid()), 'Host did not observe the actual installed-hook client identity');
        else assert.equal(authority.identity.observerSid, host.host.sid, 'Host and installed hook must use the same ordinary principal');
        assert.equal(requests.length, 0, 'A model/provider request escaped the blocking fixture hook');
        await record(path.join(current, 'result.json'), { pass: true, terminal: result, actualInstalledOrgAndWholeIpc: true, zeroProviderRequests: true, requests,
          modelPolicy: 'Explicit isolated test provider only; no production admission/default change', claims: process.platform === 'win32' ? 'Windows matching Job release' : 'Existing Unix compatibility, no new complete-tree claim' });
      } finally { await host.stop(); }
    }
    console.log(JSON.stringify({ kind: 'actual-compiled-cli-codex-installed-org-authenticated-ipc', platform: process.platform, arch: process.arch, tests: 2, pass: 2, fail: 0, skip: 0 }));
  } finally {
    await new Promise(resolve => sentinel.close(resolve));
    const cases = [];
    for (const control of ['eof', 'interrupt']) {
      let report = null, terminalReport = null, readError = null;
      for (const name of ['terminal.json', 'terminal-final.json']) {
        const file = path.join(evidence, control, name);
        try { report = await json(file); terminalReport = file; break; }
        catch (error) { if (error.code !== 'ENOENT') { readError = error.message; terminalReport = file; break; } }
      }
      cases.push(observeTerminalCase(control, report, terminalReport, attemptedCases.has(control), readError));
    }
    await record(path.join(evidence, 'provider-requests.json'), { requests, cases,
      observationScope: 'Requests received by the local sentinel; zero requests alone does not establish that a fixture turn executed.',
      authenticationSelectionPolicy: 'The fixture driver sends no authentication selection; this is policy, not independent observation of Codex UI behavior.' });
    await record(path.join(evidence, 'retained-fixture.json'), { fixture, primary: primary.context, inherited, manifest, consumer: await digest(entry),
      scope: 'Retained isolated fixture; no automatic unknown cleanup and no live installation', observedAt: new Date().toISOString() });
  }
}

if (args[0] === '--prepare' && args[2] === '--agents' && args.length === 4) await prepare(path.resolve(args[1]), await realpath(args[3]));
else if (args[0] === '--artifacts' && args[2] === '--agents' && args[4] === '--evidence' && args.length === 6) {
  const evidence = path.resolve(args[5]); await mkdir(evidence, { recursive: true });
  let failure;
  try { await consume(path.resolve(args[1]), await realpath(args[3]), evidence); }
  catch (error) { failure = { message: error.message, stack: error.stack }; throw error; }
  finally { await record(path.join(evidence, 'execution.json'), { command: [process.execPath, ...process.argv.slice(1)], cwd: process.cwd(), failure: failure ?? null, observedAt: new Date().toISOString() }); }
} else throw Error('usage: native-core-slice.mjs --prepare OUTPUT --agents ACCEPTED_CHECKOUT | --artifacts OUTPUT --agents ACCEPTED_CHECKOUT --evidence OUTPUT');
