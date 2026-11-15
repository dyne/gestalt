import assert from 'node:assert/strict';
import { cp, mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'smol-toml';
import { approvalConfiguration, permissionConfiguration } from '../src/cli/setup/configuration.mjs';
import { parseSetupOptions, parseUpdateOptions, runUpdate } from '../src/cli/setup/commands.ts';
import { SetupContext } from '../src/cli/setup/context.ts';
import { mobileHostSupport } from '../src/cli/setup/mobile.ts';
import { retiredRestartArguments } from '../src/cli/setup/restart.ts';
import { captureCopyTree, compareCopyTrees } from '../scripts/native-copy-provenance.mjs';
import { borrowSetupScratch, releaseSetupScratch } from '../src/cli/setup/scratch-context.mjs';
import { createHash, randomUUID } from 'node:crypto';
import { readFile, readdir, realpath } from 'node:fs/promises';
import { capturedNodeCommand } from './fixtures/native-node-command.mjs';
import { commandTarget } from '../src/cli/platform/executables.ts';

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

for (const selected of ['all', 'gestalt', 'serena', 'playwright']) test(`native update selection preserves ${selected} boundaries and ordering`, async t => {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt native selection '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { userHome: root, codexHome: path.join(root, 'codex'), gestaltHome: path.join(root, 'gestalt'), marketplace: 'fixture', marketplaceName: 'fixture', manager: path.join(root, 'manager'), unixProbe: undefined };
  for (const component of ['serena', 'playwright']) { await mkdir(path.join(paths.gestaltHome, component), { recursive: true }); await writeFile(path.join(paths.gestaltHome, component, 'active.json'), '{}'); }
  const streams = { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} };
  const context = new SetupContext({ argv: [], cwd: root, environment: { GESTALT_MANAGER_DISTRIBUTION: 'npm' } }, streams, paths, '0.0.0');
  const order = [];
  const operations = { manager: async () => { order.push('manager'); return false; }, skills: async (_c, mode) => { order.push(`skills:${mode}`); }, agents: async (_c, mode, extra) => { order.push(`agents:${mode}:${extra}`); }, mobile: async () => { order.push('mobile'); }, component: async (_c, component) => { order.push(component); } };
  await runUpdate(context, parseUpdateOptions([selected]), ['update', selected], false, operations);
  const core = ['manager', 'skills:update', 'agents:upgrade:false', 'mobile'];
  assert.deepEqual(order, selected === 'all' ? [...core, 'serena', 'playwright'] : selected === 'gestalt' ? core : [selected]);
  for (const failed of [...order]) {
    const seen = [];
    const fail = label => { seen.push(label); if (label === failed) throw new Error('controlled failure'); };
    const failing = { manager: async () => { fail('manager'); return false; }, skills: async () => fail('skills:update'), agents: async () => fail('agents:upgrade:false'), mobile: async () => fail('mobile'), component: async (_c, c) => fail(c) };
    await assert.rejects(runUpdate(context, parseUpdateOptions([selected]), ['update', selected], false, failing), /controlled failure/);
    assert.deepEqual(seen, order.slice(0, order.indexOf(failed) + 1));
  }
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
  const held = [], result = { kind: 'exit', code: 0 };
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
    } });
  const capability = await context.managedCommands(); assert.equal(capability.apiVersion, 1);
  const unlisten = context.listen();
  try {
    const streams = { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} };
    const first = await capability.launch(process.execPath, { argv: ['literal & ü'], cwd: root, environment: { FIXTURE: 'first' } }, streams);
    const second = await capability.launch(process.execPath, { argv: ['second'], cwd: root, environment: { FIXTURE: 'second' } }, streams);
    assert.equal(context.canRollback(), false);
    assert.equal(context.environment.FIXTURE, 'manager');
    assert.deepEqual(held.map(entry => entry.invocation.environment.FIXTURE), ['first', 'second']);
    process.emit('SIGTERM');
    assert.equal(context.signal.aborted, true); assert.equal(context.signal.reason.status, 143);
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
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt setup borrower ')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const sessionId = randomUUID(), ownerId = 'mobile-' + createHash('sha256').update(sessionId).digest('hex');
  const directory = path.join(root, 'tmp/sessions', ownerId), markerPath = path.join(directory, '.gestalt-owner.json');
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const marker = JSON.stringify({ schemaVersion: 1, ownerKind: 'mobile-session', ownerId, workspacePath: root, sessionId,
    createdAt: new Date().toISOString(), authority: { kind: 'mobile-roster', sessionId }, providerThreads: [] });
  await writeFile(markerPath, marker, { mode: 0o600 });
  const environment = { GESTALT_SESSION_TMP_CONTEXT: JSON.stringify({ schemaVersion: 1, workspacePath: root, ownerId, directory, markerPath }) };
  if (process.platform === 'win32') {
    assert.throws(() => borrowSetupScratch(environment), /portable Windows scratch broker/);
    assert.equal(await readFile(markerPath, 'utf8'), marker);
    assert.deepEqual((await readdir(directory)).sort(), ['.gestalt-owner.json']);
    return;
  }
  const lease = borrowSetupScratch(environment), leases = path.join(directory, 'leases'), names = await readdir(leases);
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
