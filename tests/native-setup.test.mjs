import assert from 'node:assert/strict';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'smol-toml';
import { approvalConfiguration, permissionConfiguration } from '../src/cli/setup/configuration.mjs';
import { parseSetupOptions, parseUpdateOptions, runUpdate } from '../src/cli/setup/commands.ts';
import { SetupContext } from '../src/cli/setup/context.ts';
import { mobileHostSupport } from '../src/cli/setup/mobile.ts';
import { retiredRestartArguments } from '../src/cli/setup/restart.ts';

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
