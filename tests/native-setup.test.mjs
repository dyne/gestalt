import assert from 'node:assert/strict';
import { cp, mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'smol-toml';
import { approvalConfiguration, permissionConfiguration } from '../src/cli/setup/configuration.mjs';
import { createSetupHandlers, parseSetupOptions, parseUpdateOptions, runInstall, runUpdate } from '../src/cli/setup/commands.ts';
import { SetupContext } from '../src/cli/setup/context.ts';
import { mobileHostSupport } from '../src/cli/setup/mobile.ts';
import { retiredRestartArguments } from '../src/cli/setup/restart.ts';
import { captureCopyTree, compareCopyTrees } from '../scripts/native-copy-provenance.mjs';
import { borrowSetupScratch, releaseSetupScratch } from '../src/cli/setup/scratch-context.mjs';
import { createHash, randomUUID } from 'node:crypto';
import { readFile, readdir, realpath } from 'node:fs/promises';
import { capturedNodeCommand } from './fixtures/native-node-command.mjs';
import { commandTarget } from '../src/cli/platform/executables.ts';
import { captureSetupRetention } from './fixtures/setup-retention.mjs';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';

test('native configuration preserves custom bytes, modes-independent content and repeated reconciliation', () => {
  const custom = '# keep this comment\r\nmodel = "custom" # and this one\r\napproval_policy = "on-request"\r\n[features]\r\nhooks = true\r\n[mcp_servers.custom]\r\ncommand = "unrelated"\r\ndefault_tools_approval_mode = "prompt"\r\n';
  const roots = [path.resolve('Codex ü'), path.resolve('Gestalt ü')];
  const configured = permissionConfiguration(custom, roots, 'C:\\Users\\me\\AppData\\Local\\Temp');
  assert.equal(permissionConfiguration(configured, roots, 'C:\\Users\\me\\AppData\\Local\\Temp'), configured);
  assert.ok(configured.includes('# keep this comment\r\n'));
  assert.ok(configured.includes('[features]\r\nhooks = true\r\n[mcp_servers.custom]\r\ncommand = "unrelated"\r\ndefault_tools_approval_mode = "prompt"\r\n'));
  const config = parse(configured);
  assert.equal(config.model, 'custom');
  assert.equal(config.approval_policy, 'never'); assert.equal(config.default_permissions, 'workspace-git');
  assert.equal(config.permissions['workspace-git'].network.enabled, true);
  assert.equal(config.permissions['workspace-git'].filesystem['C:\\Users\\me\\AppData\\Local\\Temp'], 'write');
  assert.equal(config.permissions['workspace-git'].filesystem[':workspace_roots']['.agents'], 'read');
  assert.equal(config.mcp_servers.custom.default_tools_approval_mode, 'prompt');
  assert.throws(() => permissionConfiguration('model = [broken', roots));
});

test('native managed MCP approvals retain unrelated policy and multiline operator environment', () => {
  const home = path.resolve('Codex ü & profile'), manager = path.resolve('gestalt.mjs');
  const source = `[mcp_servers.context-mode]\ncommand = "node"\nargs = [${JSON.stringify(path.join(home, 'bin/context-mode-mcp.mjs'))}]\nenv_vars = [\n  "OPERATOR", # keep\n]\ndefault_tools_approval_mode = "prompt"\n[mcp_servers.custom]\ncommand = "custom"\ndefault_tools_approval_mode = "prompt"\n`;
  const output = approvalConfiguration(source, manager, home), parsed = parse(output);
  assert.equal(parsed.mcp_servers['context-mode'].default_tools_approval_mode, 'approve');
  assert.deepEqual(parsed.mcp_servers['context-mode'].env_vars, ['OPERATOR', 'GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP']);
  assert.equal(parsed.mcp_servers.custom.default_tools_approval_mode, 'prompt');
  assert.ok(output.includes('"OPERATOR", # keep\n'));
  assert.equal(approvalConfiguration(output, manager, home), output);
});

test('setup and component parsing reject unsupported policy before any operation', () => {
  assert.deepEqual(parseSetupOptions(['--extra-skills']), { extra: true, help: false });
  assert.deepEqual(parseUpdateOptions(['--component', 'gestalt', '--extra-skills']), { component: 'gestalt', extra: true, help: false });
  for (const argv of [['--executor-model', 'x'], ['--unknown']]) assert.throws(() => parseSetupOptions(argv), /unknown setup option/);
  for (const argv of [['xerj'], ['--component', 'xerj'], ['--component'], ['all', 'gestalt'], ['serena', '--extra-skills'], ['--unknown']]) assert.throws(() => parseUpdateOptions(argv));
});

for (const selected of ['all', 'gestalt', 'serena', 'playwright']) test(`Unix update selection policy preserves ${selected} boundaries and ordering`, async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt native selection '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { userHome: root, codexHome: path.join(root, 'codex'), gestaltHome: path.join(root, 'gestalt'), marketplace: 'fixture', marketplaceName: 'fixture', manager: path.join(root, 'manager'), unixProbe: undefined };
  for (const component of ['serena', 'playwright']) { await mkdir(path.join(paths.gestaltHome, component), { recursive: true }); await writeFile(path.join(paths.gestaltHome, component, 'active.json'), '{}'); }
  const streams = { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} };
  const context = new SetupContext({ argv: [], cwd: root, environment: { GESTALT_MANAGER_DISTRIBUTION: 'npm' } }, streams, paths, '0.0.0');
  const order = [];
  // This contract exercises orchestration ordering. Real mode/ACL checks and
  // absent-home creation execute in the native composition contracts.
  context.prepareHomes = async () => { order.push('homes'); };
  const operations = { manager: async () => { order.push('manager'); return false; }, skills: async (_c, mode) => { order.push(`skills:${mode}`); }, agents: async (_c, mode, extra) => { order.push(`agents:${mode}:${extra}`); }, mobile: async () => { order.push('mobile'); }, component: async (_c, component) => { order.push(component); }, diagnostics: async () => { order.push('doctor'); } };
  await runUpdate(context, parseUpdateOptions([selected]), ['update', selected], false, operations, 'linux');
  const core = ['manager', 'homes', 'skills:update', 'agents:upgrade:false', 'mobile'];
  assert.deepEqual(order, selected === 'all' ? [...core, 'serena', 'playwright', 'doctor'] : selected === 'gestalt' ? [...core, 'doctor'] : [selected]);
  for (const failed of order.filter(step => step !== 'doctor')) {
    const seen = [];
    const fail = label => { seen.push(label); if (label === failed) throw new Error('controlled failure'); };
    context.prepareHomes = async () => fail('homes');
    const failing = { manager: async () => { fail('manager'); return false; }, skills: async () => fail('skills:update'), agents: async () => fail('agents:upgrade:false'), mobile: async () => fail('mobile'), component: async (_c, c) => fail(c) };
    await assert.rejects(runUpdate(context, parseUpdateOptions([selected]), ['update', selected], false, failing, 'linux'), /controlled failure/);
    assert.deepEqual(seen, order.slice(0, order.indexOf(failed) + 1));
  }
});

test('private-home preparation rejects an existing refused home before creating an absent peer', async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt home preparation '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const codex = path.join(root, 'absent Codex'), gestalt = path.join(root, 'existing Gestalt');
  await mkdir(gestalt); await writeFile(path.join(gestalt, 'user-data'), 'preserve');
  const context = new SetupContext({ argv: [], cwd: root, environment: {} },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: root, codexHome: codex, gestaltHome: gestalt, manager: process.execPath,
      marketplace: 'fixture', marketplaceName: 'fixture', unixProbe: undefined }, 'fixture');
  const inspected = [];
  // Controlled refusal verifies ordering without pretending this local fixture
  // establishes a native positive privacy/ACL result.
  context.storage = async selected => { inspected.push(selected); throw new Error('controlled private-storage refusal'); };
  await assert.rejects(context.prepareHomes(), /controlled private-storage refusal/);
  assert.deepEqual(inspected, [gestalt]);
  await assert.rejects(readFile(codex), error => error.code === 'ENOENT');
  assert.equal(await readFile(path.join(gestalt, 'user-data'), 'utf8'), 'preserve');
});

test('retired restart options adapt only explicit known values and preserve argument boundaries', () => {
  assert.deepEqual(retiredRestartArguments(['--xerj', 'auto', '--port', '4321', '--', '--xerj', 'keep']), ['--port', '4321', '--', '--xerj', 'keep']);
  for (const argv of [['--xerj'], ['--xerj', 'ambiguous']]) assert.throws(() => retiredRestartArguments(argv), /missing or ambiguous/);
  assert.equal(mobileHostSupport('win32').supported, false);
  assert.match(mobileHostSupport('win32').prerequisite, /native win32.*owned-process.*session-scratch/);
});

test('native copy preflight inventories nested dist and reports all missing or changed dependency bytes', async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt native copy '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = path.join(root, 'source'), copied = path.join(root, 'copied');
  await mkdir(path.join(source, 'smol-toml/dist'), { recursive: true });
  await mkdir(path.join(source, 'native/bin'), { recursive: true });
  await writeFile(path.join(source, 'smol-toml/package.json'), '{"exports":"./dist/index.js"}');
  await writeFile(path.join(source, 'smol-toml/dist/index.js'), 'export const parser = "fixture";\n');
  await writeFile(path.join(source, 'native/bin/compiler.exe'), Buffer.from([0, 255, 10, 128]));
  const expected = await captureCopyTree(source);
  await cp(source, copied, { recursive: true });
  assert.deepEqual(compareCopyTrees(expected, await captureCopyTree(copied)), []);
  await rm(path.join(copied, 'smol-toml/dist'), { recursive: true });
  await writeFile(path.join(copied, 'native/bin/compiler.exe'), Buffer.from([0, 0, 10, 128]));
  const differences = compareCopyTrees(expected, await captureCopyTree(copied));
  assert.deepEqual(differences.map(entry => entry.path), ['native/bin/compiler.exe', 'smol-toml/dist', 'smol-toml/dist/index.js']);
  assert.equal(differences[0].expected.bytes, differences[0].actual.bytes);
  assert.notEqual(differences[0].expected.sha256, differences[0].actual.sha256);
  assert.equal(differences[2].actual, null);
});

test('setup waits for the ownership release proof before propagating cancellation', async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt setup owned wait '));
  t.after(() => rm(root, { recursive: true, force: true }));
  let finishStop, stopped = false;
  const result = { kind: 'exit', code: 143 }, claim = { kind: 'released', lease: 'controlled', backend: 'controlled-test', result };
  const completion = new Promise(resolve => { finishStop = resolve; });
  const context = new SetupContext({ argv: [], cwd: root, environment: {} },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: root, codexHome: path.join(root, 'codex'), gestaltHome: path.join(root, 'gestalt'), manager: process.execPath,
      marketplace: 'fixture', marketplaceName: 'fixture', unixProbe: undefined, ownershipHelper: process.execPath }, 'fixture',
    { launch: async () => ({ id: 'controlled', result: Promise.resolve(result), completed: completion, claim: () => ({ kind: 'active', lease: 'controlled', backend: 'controlled-test' }),
      stop: async () => { stopped = true; return completion; } }) });
  const unlisten = context.listen();
  let settled = false;
  const operation = context.run(process.execPath, []).finally(() => { settled = true; });
  void operation.catch(() => {});
  try {
    const deadline = Date.now() + 5000;
    while (!stopped && Date.now() < deadline) await new Promise(resolve => setImmediate(resolve));
    assert.equal(stopped, true, 'controlled ownership cleanup was not reached');
    process.emit('SIGTERM');
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(settled, false, 'cancellation returned before descendant cleanup completed');
    finishStop(claim);
    await assert.rejects(operation, error => error.status === 143 && !/previous configuration restored/.test(error.message));
    assert.equal(context.canRollback(), true);
  } finally { finishStop(claim); await operation.catch(() => {}); unlisten(); }
});

test('setup retains failed unknown ownership instead of authorizing rollback', async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt setup unknown '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const result = { kind: 'exit', code: 7 }, claim = { kind: 'unknown', lease: 'controlled-unknown', reason: 'controlled ownership loss' };
  const context = new SetupContext({ argv: [], cwd: root, environment: {} },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: root, codexHome: path.join(root, 'codex'), gestaltHome: path.join(root, 'gestalt'), manager: process.execPath,
      marketplace: 'fixture', marketplaceName: 'fixture', unixProbe: undefined, ownershipHelper: process.execPath }, 'fixture',
    { launch: async () => ({ id: claim.lease, result: Promise.resolve(result), completed: Promise.resolve(claim), claim: () => claim, stop: async () => claim }) });
  await assert.rejects(context.run(process.execPath, []), error => error.status === 7 && /absence is unconfirmed/.test(error.message));
  assert.equal(context.canRollback(), false);
  assert.equal(context.unknownOwnership.length, 1);
});

test('managed Agents commands share cancellation and await each owned release before manager compensation', async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt managed capability '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const held = [], result = { kind: 'exit', code: 0 }, upstream = new AbortController();
  const context = new SetupContext({ argv: [], cwd: root, environment: { FIXTURE: 'manager' } },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: root, codexHome: path.join(root, 'codex'), gestaltHome: path.join(root, 'gestalt'), manager: process.execPath,
      marketplace: 'fixture', marketplaceName: 'fixture', unixProbe: undefined }, 'fixture',
    { launch: async (_executable, invocation) => {
      const id = `nested-${held.length}`;
      let release; const completed = new Promise(resolve => { release = resolve; });
      const entry = { id, invocation, release, stopped: false }; held.push(entry);
      return { id, result: Promise.resolve(result), completed, claim: () => ({ kind: 'active', lease: id, backend: 'controlled-test' }),
        stop: async () => { entry.stopped = true; return completed; } };
    } }, upstream.signal);
  const capability = await context.managedCommands(); assert.equal(capability.apiVersion, 1);
  const unlisten = context.listen();
  try {
    const streams = { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} };
    const first = await capability.launch(process.execPath, { argv: ['literal & ü'], cwd: root, environment: { FIXTURE: 'first' } }, streams);
    const redirected = await context.withStreams(streams), unlistenRedirected = redirected.listen();
    t.after(async () => { await redirected.stopActive(); unlistenRedirected(); });
    const second = await (await redirected.managedCommands()).launch(process.execPath,
      { argv: ['second'], cwd: root, environment: { FIXTURE: 'second' } }, streams);
    assert.equal(context.canRollback(), false);
    assert.equal(context.environment.FIXTURE, 'manager');
    assert.deepEqual(held.map(entry => entry.invocation.environment.FIXTURE), ['first', 'second']);
    upstream.abort({ status: 143 });
    assert.equal(context.signal.aborted, true); assert.equal(context.signal.reason.status, 143);
    assert.equal(redirected.signal.aborted, true); assert.equal(redirected.signal.reason.status, 143);
    assert.ok(held.every(entry => entry.stopped));
    held[0].release({ kind: 'released', lease: first.id, backend: 'controlled-test', result });
    await first.completed; assert.equal(context.canRollback(), false);
    held[1].release({ kind: 'released', lease: second.id, backend: 'controlled-test', result });
    await second.completed; await context.stopActive(); assert.equal(context.canRollback(), true);
    assert.deepEqual(context.unknownOwnership, []);
  } finally {
    for (const entry of held) entry.release({ kind: 'released', lease: entry.id, backend: 'controlled-test', result });
    await context.stopActive(); unlisten();
  }
});

test('managed Agents capability records unknown once across stop and completion', async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt managed unknown '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const claim = { kind: 'unknown', lease: 'api-unknown', reason: 'controlled loss' }, result = { kind: 'exit', code: 0 };
  const context = new SetupContext({ argv: [], cwd: root, environment: {} },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: root, codexHome: path.join(root, 'codex'), gestaltHome: path.join(root, 'gestalt'), manager: process.execPath,
      marketplace: 'fixture', marketplaceName: 'fixture', unixProbe: undefined }, 'fixture',
    { launch: async () => ({ id: claim.lease, result: Promise.resolve(result), completed: Promise.resolve(claim), claim: () => claim, stop: async () => claim }) });
  const capability = await context.managedCommands();
  const lease = await capability.launch(process.execPath, context.invocation, context.streams);
  const [completed, stopped] = await Promise.all([lease.completed, lease.stop()]);
  assert.deepEqual(completed, claim); assert.deepEqual(stopped, claim);
  assert.equal(context.unknownOwnership.length, 1); assert.equal(context.canRollback(), false);
});

test('setup borrower publication preserves the existing owner and releases only the exact stopped claim', async t => {
  const outer = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt setup borrower ')));
  t.after(() => rm(outer, { recursive: true, force: true }));
  const probe = process.platform === 'darwin' ? await buildNativePlatform(path.resolve(import.meta.dirname, '..'), path.join(outer, 'native')) : undefined;
  const storage = await NativePrivateStorage.open(path.join(outer, 'private'), probe), root = storage.root;
  const sessionId = randomUUID(), ownerId = 'mobile-' + createHash('sha256').update(sessionId).digest('hex');
  const directory = path.join(root, 'tmp/sessions', ownerId), markerPath = path.join(directory, '.gestalt-owner.json');
  for (const entry of ['tmp', 'tmp/sessions', path.join('tmp/sessions', ownerId)]) await storage.createDirectory(path.join(root, entry));
  const marker = JSON.stringify({ schemaVersion: 1, ownerKind: 'mobile-session', ownerId, workspacePath: root, sessionId,
    createdAt: new Date().toISOString(), authority: { kind: 'mobile-roster', sessionId }, providerThreads: [] });
  await storage.replaceFile(markerPath, Buffer.from(marker));
  const environment = { GESTALT_SESSION_TMP_CONTEXT: JSON.stringify({ schemaVersion: 1, workspacePath: root, ownerId, directory, markerPath }) };
  const lease = borrowSetupScratch(environment, probe), leases = path.join(directory, 'leases'), names = await readdir(leases);
  assert.equal(names.length, 1);
  const file = path.join(leases, names[0]), active = JSON.parse(await readFile(file, 'utf8'));
  assert.equal(active.state, 'active'); assert.deepEqual(active.consumers, ['manager-setup']);
  assert.equal(await readFile(markerPath, 'utf8'), marker);
  assert.equal(await releaseSetupScratch(lease), true);
  const released = JSON.parse(await readFile(file, 'utf8'));
  assert.equal(released.state, 'released'); assert.deepEqual(released.consumers, []);
  assert.equal(released.leaseId, active.leaseId); assert.equal(released.processIdentity, active.processIdentity);
  assert.equal(await readFile(markerPath, 'utf8'), marker);
});

test('captured setup command shim resolves the complete sibling Node entry without a leading dot', async t => {
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt captured setup shim ')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const captured = JSON.parse(await readFile(new URL('./fixtures/native-npm-node-shim.json', import.meta.url), 'utf8'));
  for (const name of ['codex', 'npm']) {
    const entry = path.join(root, `${name}.mjs`), shim = path.join(root, `${name}.cmd`);
    await writeFile(entry, '#!/usr/bin/env node\n');
    const generated = capturedNodeCommand(captured, `${name}.mjs`);
    assert.ok(generated.includes(`"%dp0%\\${name}.mjs"`));
    assert.ok(!generated.includes(`"%dp0%\\.${name}.mjs"`));
    await writeFile(shim, generated);
    assert.deepEqual(await commandTarget(shim), { executable: process.execPath, prefix: [await realpath(entry)] });
  }
});

test('retained setup inventory distinguishes manager snapshots, Agents snapshots and the colliding lock prefix', async t => {
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt retained owners ')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const manager = `.gestalt-setup.${randomUUID()}`, agents = `.gestalt-setup-state-${randomUUID()}`;
  for (const name of [manager, agents, '.gestalt-setup.lock']) await mkdir(path.join(root, name));
  const snapshot = { present: false, mode: null }, ownership = { schemaVersion: 1, status: 'command-succeeded-cleanup-unknown' };
  const owner = { schemaVersion: 1, root, pid: process.pid, records: [{ relative: 'config.toml', present: true, index: 0 }] };
  await writeFile(path.join(root, manager, 'snapshot.json'), JSON.stringify(snapshot));
  await writeFile(path.join(root, manager, 'ownership-unknown.json'), JSON.stringify(ownership));
  await writeFile(path.join(root, agents, 'owner.json'), JSON.stringify(owner));
  await writeFile(path.join(root, agents, '0'), 'preserved config');
  await writeFile(path.join(root, '.gestalt-manager-setup.lock'), JSON.stringify({ version: 1, token: randomUUID() }));
  await writeFile(path.join(root, '.gestalt-setup.lock', `owner-${process.pid}-${randomUUID()}.json`), JSON.stringify({ schemaVersion: 1, pid: process.pid }));
  const before = await captureSetupRetention(root);
  assert.deepEqual(before.managerSnapshots, [{ name: manager, snapshot, ownership }]);
  assert.deepEqual(before.agentsSnapshots, [{ name: agents, owner }]);
  assert.equal(before.agentsLock.length, 1); assert.equal(before.managerLock.version, 1);
  assert.deepEqual(await captureSetupRetention(root), before, 'inspection must not modify retained state');
  await writeFile(path.join(root, agents, '0'), 'late changed config');
  assert.notDeepEqual((await captureSetupRetention(root)).entries, before.entries, 'snapshot byte mutations must be observable');
});

async function selectionPolicyFixture(t, present) {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt setup policy ü '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { userHome: root, codexHome: path.join(root, 'Codex'), gestaltHome: path.join(root, 'Gestalt'),
    marketplace: 'fixture', marketplaceName: 'fixture', manager: path.join(root, 'manager') };
  if (present) for (const component of ['serena', 'playwright', 'mobile']) {
    await mkdir(path.join(paths.gestaltHome, component), { recursive: true });
    await writeFile(path.join(paths.gestaltHome, component, 'active.json'), 'retained operator metadata\n');
  }
  const logs = [], calls = [], streams = { stdin: (async function* () { throw Error('Policy command consumed stdin'); })(),
    stdout: async () => {}, stderr: async bytes => logs.push(Buffer.from(bytes)) };
  const context = new SetupContext({ argv: [], cwd: root, environment: { GESTALT_MANAGER_DISTRIBUTION: 'npm' } }, streams, paths, 'fixture');
  // Policy/orchestration fixture only: no native privacy or process proof.
  context.prepareHomes = async () => { calls.push('homes'); };
  const operations = { manager: async () => { calls.push('manager'); return false; },
    skills: async (_context, mode) => { calls.push('skills:' + mode); },
    agents: async (_context, mode, extra) => { calls.push(`agents:${mode}:${extra}`); },
    mobile: async () => { calls.push('mobile'); throw Error('Unsupported Mobile operation must never run'); },
    component: async (_context, component) => { calls.push(component); throw Error('Deferred component operation must never run'); } };
  return { root, paths, context, operations, streams, calls, output: () => Buffer.concat(logs).toString() };
}
for (const present of [false, true]) test(`native setup policy Windows all omits deferred operations with retained components=${present}`, async t => {
  assert.equal(mobileHostSupport('win32').supported, false);
  const f = await selectionPolicyFixture(t, present);
  await runUpdate(f.context, parseUpdateOptions([]), ['update'], false, f.operations, 'win32');
  assert.deepEqual(f.calls, ['manager', 'homes', 'skills:update', 'agents:upgrade:false']);
  assert.match(f.output(), /Serena omitted:.*deferred/); assert.match(f.output(), /Playwright omitted:.*deferred/);
  assert.match(f.output(), /Mobile omitted:.*native win32.*owned-process.*session-scratch/);
  assert.doesNotMatch(f.output(), /use gestalt update serena to install|use gestalt playwright install/);
  for (const component of present ? ['serena', 'playwright', 'mobile'] : [])
    assert.equal(await readFile(path.join(f.paths.gestaltHome, component, 'active.json'), 'utf8'), 'retained operator metadata\n');
  t.diagnostic('Controlled Windows policy input on actual ' + process.platform + '; native Windows execution is a separate gate');
});
for (const command of ['install', 'update', 'update-restart']) for (const component of ['serena', 'playwright', 'mobile'])
  test(`native setup policy Windows explicit ${command} ${component} refuses before context and profile work`, async t => {
    const f = await selectionPolicyFixture(t, false);
    const environment = { HOME: f.root, USERPROFILE: f.root, CODEX_HOME: f.paths.codexHome, GESTALT_HOME: f.paths.gestaltHome,
      GESTALT_SESSION_TMP_CONTEXT: '{}' };
    const before = await readdir(f.root);
    const handlers = createSetupHandlers({ version: 'fixture', help: 'help\n' }, f.operations, false, undefined, 'win32');
    assert.equal(await handlers[command]({ argv: [component], cwd: f.root, environment }, f.streams), 1);
    assert.match(f.output(), new RegExp(`native ${component} support is deferred`, 'i'));
    if (component === 'mobile') assert.match(f.output(), /native win32.*owned-process.*session-scratch/);
    assert.doesNotMatch(f.output(), /SESSION_SCRATCH_INVALID_CONTEXT/);
    assert.deepEqual(f.calls, []); assert.deepEqual(await readdir(f.root), before);
  });

for (const extra of [false, true]) test(`native setup policy Windows install keeps supported operations and extra selection=${extra}`, async t => {
  const f = await selectionPolicyFixture(t, true);
  await runInstall(f.context, extra, f.operations, 'win32');
  assert.deepEqual(f.calls, ['homes', 'skills:install', `agents:install:${extra}`]);
  assert.match(f.output(), /Mobile omitted:/); assert.match(f.output(), /Serena omitted:.*deferred/); assert.match(f.output(), /Playwright omitted:.*deferred/);
  assert.match(f.output(), /then 'gestalt cli'/); assert.doesNotMatch(f.output(), /or 'gestalt mobile'/);
  for (const component of ['serena', 'playwright', 'mobile']) assert.equal(await readFile(path.join(f.paths.gestaltHome, component, 'active.json'), 'utf8'), 'retained operator metadata\n');
});
test('native setup policy rejects a direct Windows named update before any core or optional operation', async t => {
  const f = await selectionPolicyFixture(t, true);
  for (const component of ['serena', 'playwright']) await assert.rejects(runUpdate(f.context, parseUpdateOptions([component]), ['update', component], false, f.operations, 'win32'), /support is deferred/);
  assert.deepEqual(f.calls, []);
});
test('native setup policy uses actual host support by default and keeps ordinary Unix Mobile failures visible', async t => {
  const f = await selectionPolicyFixture(t, false), supported = mobileHostSupport().supported;
  if (supported) {
    await assert.rejects(runInstall(f.context, false, f.operations), /Unsupported Mobile operation/);
    assert.deepEqual(f.calls, ['homes', 'skills:install', 'agents:install:false', 'mobile']);
  } else {
    await runInstall(f.context, false, f.operations);
    assert.deepEqual(f.calls, ['homes', 'skills:install', 'agents:install:false']);
  }
});
test('native setup policy Windows core-only update preserves extra skills and npm manager ownership', async t => {
  const f = await selectionPolicyFixture(t, true);
  await runUpdate(f.context, parseUpdateOptions(['gestalt', '--extra-skills']), ['update', 'gestalt', '--extra-skills'], false, f.operations, 'win32');
  assert.deepEqual(f.calls, ['manager', 'homes', 'skills:update', 'agents:upgrade:true']);
  assert.match(f.output(), /update the manager with npm install -g/); assert.match(f.output(), /update complete \(gestalt\)/);
  assert.doesNotMatch(f.output(), /Mobile, and context-mode are up to date/);
});
test('native setup policy Windows does not hide required core failure or continue after manager handoff', async t => {
  for (const failed of ['manager', 'homes', 'skills', 'agents']) {
    const f = await selectionPolicyFixture(t, false), order = [];
    const step = name => { order.push(name); if (name === failed) throw Error('required core failed'); };
    f.context.prepareHomes = async () => step('homes');
    const operations = { ...f.operations, manager: async () => { step('manager'); return false; },
      skills: async () => step('skills'), agents: async () => step('agents') };
    await assert.rejects(runUpdate(f.context, parseUpdateOptions(['all']), ['update', 'all'], false, operations, 'win32'), /required core failed/);
    assert.deepEqual(order, ['manager', 'homes', 'skills', 'agents'].slice(0, ['manager', 'homes', 'skills', 'agents'].indexOf(failed) + 1));
    assert.doesNotMatch(f.output(), /update complete/); assert.deepEqual(f.calls, []);
  }
  const f = await selectionPolicyFixture(t, true);
  assert.equal(await runUpdate(f.context, parseUpdateOptions(['all']), ['update', 'all'], false,
    { ...f.operations, manager: async () => { f.calls.push('handoff'); return true; } }, 'win32'), true);
  assert.deepEqual(f.calls, ['handoff']);
});
test('native setup policy Windows named component flag forms preserve existing profile bytes before refusal', async t => {
  const f = await selectionPolicyFixture(t, true); await mkdir(f.paths.codexHome);
  await writeFile(path.join(f.paths.codexHome, 'config.toml'), '# operator\nmodel="unchanged"\n');
  const before = await captureCopyTree(f.root);
  const environment = { HOME: f.root, USERPROFILE: f.root, CODEX_HOME: f.paths.codexHome, GESTALT_HOME: f.paths.gestaltHome, GESTALT_SESSION_TMP_CONTEXT: '{}' };
  const handlers = createSetupHandlers({ version: 'fixture', help: 'help\n' }, f.operations, false, undefined, 'win32');
  for (const component of ['serena', 'playwright', 'mobile']) for (const argv of [['--component', component], ['--component=' + component]]) {
    assert.equal(await handlers.update({ argv, cwd: f.root, environment }, f.streams), 1);
    assert.deepEqual(await captureCopyTree(f.root), before); assert.deepEqual(f.calls, []);
  }
});
test('native setup policy Windows explicit all flag retains the same supported-only update', async t => {
  const f = await selectionPolicyFixture(t, true);
  await runUpdate(f.context, parseUpdateOptions(['--component', 'all']), ['update', '--component', 'all'], false, f.operations, 'win32');
  assert.deepEqual(f.calls, ['manager', 'homes', 'skills:update', 'agents:upgrade:false']);
  assert.match(f.output(), /update complete \(all\)/);
});
