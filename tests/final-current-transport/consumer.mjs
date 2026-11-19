import assert from 'node:assert/strict';
import { mkdir, mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { installPackedCore } from '../fixtures/packed-core.mjs';
import { NativeExecutables } from '../../src/cli/platform/executables.ts';
import { digest, errorInfo, inputs, json, now, phases, root, save, npm12Inventory, verifyAcceptedPrHelper, verifyInventory } from './common.mjs';

const output = path.join(root, 'native-evidence/final-consumer');
const prebuilt = path.join(root, 'native-evidence/prebuilt');
const agents = path.join(root, '.native-dependencies/agents');
const state = { schemaVersion: 1, lane: 'windows-standard-user', state: 'started', startedAt: now(), platform: process.platform,
  arch: process.arch, node: process.version, groups: [], commands: [], failure: null };
const environment = { ...process.env };
const deadline = Date.parse(environment.FINAL_WINDOWS_DEADLINE_UTC ?? '');
const selection = { id: 'lifecycle-npm12', kind: 'lifecycle', name: 'actual npm 12.2.2 global lifecycle and installed literal commands', timeoutMs: 930_000, state: 'unexecuted' };
assert.deepEqual(inputs.remainingWindowsCaseIds, [selection.id]);
state.cases = [selection];
const remaining = () => deadline - Date.now() - 20_000;
const { group, command, record } = phases(output, state, environment);
let artifacts, helper, windowsInputs, lifecycleRoot, npm12Entry;
async function verifyCopy() {
  assert.equal(await realpath(root), root);
  await verifyInventory(root, await json(path.join(root, 'native-evidence/binding/manager-inventory.json')));
  await verifyInventory(agents, await json(path.join(root, 'native-evidence/binding/agents-inventory.json')));
  const builder = await json(path.join(root, 'native-evidence/binding/result.json'));
  assert.equal(builder.state, 'pass'); assert.equal(builder.lane, 'windows-consumer');
  assert.equal(builder.groups.find(row => row.id === 'source').detail.agentsTree, inputs.agentsTree);
  windowsInputs = builder.groups.find(row => row.id === 'prepare-consumer').detail;
  assert.equal((await digest(path.join(prebuilt, 'artifacts.json'))).sha256, windowsInputs.prebuilt.sha256);
  artifacts = await json(path.join(prebuilt, 'artifacts.json'));
  assert.equal(artifacts.platform, 'win32'); assert.equal(artifacts.arch, 'x64'); assert.equal(artifacts.node, inputs.node);
  assert.equal(artifacts.abi, process.versions.modules);
  const cli = artifacts.artifacts.find(item => item.relative === 'gestalt.mjs');
  assert.equal(cli?.bytes, inputs.normalCliBytes);
  assert.equal(cli.sha256, inputs.normalCliSha256);
  for (const item of artifacts.artifacts) {
    assert.ok(!path.isAbsolute(item.relative) && !item.relative.split(/[\\/]/).includes('..'));
    const actual = await digest(path.join(prebuilt, item.relative));
    assert.equal(actual.sha256, item.sha256); assert.equal(actual.bytes, item.bytes);
  }
  helper = await verifyAcceptedPrHelper(path.join(prebuilt, 'native'));
  const staged = builder.groups.find(row => row.id === 'accepted-pr-helper').detail;
  assert.equal(helper.helper.sha256, staged.helper.sha256);
  assert.equal(helper.provenance.sha256, staged.provenance.sha256);
  assert.deepEqual(helper.source, staged.source);
  assert.equal(artifacts.package.sourceHead, inputs.managerBase);
  assert.equal(builder.groups.find(row => row.id === 'source').detail.sourceTree, inputs.sourceTree);
  assert.equal(builder.groups.find(row => row.id === 'source').detail.sourceHead, inputs.managerBase);
  const packageFile = path.join(prebuilt, artifacts.package.relative);
  assert.equal((await digest(packageFile)).sha256, artifacts.package.sha256);
  state.head = builder.head; state.sourceHead = inputs.managerBase; state.run = builder.run; state.attempt = builder.attempt;
  const bin = path.dirname(process.execPath), npm = path.join(bin, 'node_modules/npm/bin/npm-cli.js');
  for (const [file, expected] of [[process.execPath, windowsInputs.node], [npm, windowsInputs.npm],
    [path.join(bin, 'codex.exe'), windowsInputs.codex], [path.join(bin, 'npm.cmd'), windowsInputs.npmShim],
    [path.join(bin, 'npm-entry.mjs'), windowsInputs.npmEntry]]) assert.equal((await digest(file)).sha256, expected.sha256);
  const npmVersion = await command('consumer-npm-version', process.execPath, [npm, '--version']);
  assert.equal(npmVersion.text.trim(), inputs.npm.version);
  const codexVersion = await command('consumer-codex-version', path.join(bin, 'codex.exe'), ['--version']);
  assert.equal(codexVersion.text.trim(), 'codex-cli 0.164.0');
  npm12Entry = path.join(bin, inputs.npm12.relativeEntry);
  const secondary = builder.groups.find(row => row.id === 'frozen-npm12').detail;
  const inventory = await npm12Inventory(await realpath(path.join(bin, 'npm12')));
  assert.equal(inventory.sha256, secondary.inventory.sha256);
  assert.equal(inventory.leaves, secondary.inventory.leaves);
  const entry = await digest(npm12Entry);
  assert.equal(entry.sha256, secondary.entry.sha256);
  assert.equal(entry.sha256, inputs.npm12.entrySha256);
  const npm12Version = await command('consumer-npm12-version', process.execPath, [npm12Entry, '--version']);
  assert.equal(npm12Version.text.trim(), inputs.npm12.version);
  await save(path.join(output, 'copied-npm12-inventory.json'), inventory);
  return { helper, npm12Version: npm12Version.text.trim(), npm12Entry: entry, npmVersion: npmVersion.text.trim(), codexVersion: codexVersion.text.trim(), managerAndAgentsCopyVerified: true };
}

async function packagedLifecycle() {
  lifecycleRoot = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt final npm lifecycle ')));
  const workspace = path.join(lifecycleRoot, 'profile'), groupOutput = path.join(output, 'lifecycle-npm12');
  await mkdir(workspace, { mode: 0o700 }); await mkdir(groupOutput, { mode: 0o700 });
  const home = path.join(workspace, 'home'); await mkdir(home, { mode: 0o700 });
  const code = path.join(workspace, 'Codex'), gestalt = path.join(workspace, 'Gestalt');
  const env = { ...environment, HOME: home, USERPROFILE: home, CODEX_HOME: code, GESTALT_HOME: gestalt,
    CONTEXT_MODE_PACKAGE_MANAGER: 'npm', CONTEXT_MODE_PLATFORM: 'codex', CONTEXT_MODE_WORKSPACE: workspace };
  assert.ok(await new NativeExecutables().resolve('npm', env));
  await save(path.join(groupOutput, 'lifecycle-start.json'), { startedAt: now(), fixture: lifecycleRoot,
    npm: inputs.npm12.version, sourceHead: state.sourceHead, package: artifacts.package, actualGlobalLifecycleRequested: true });
  const result = await installPackedCore(path.join(prebuilt, artifacts.package.relative), lifecycleRoot, env, agents, groupOutput, { npm12Entry });
  const prerequisite = await json(path.join(groupOutput, 'packed-prerequisite.json'));
  assert.equal(prerequisite.failure, null);
  assert.equal(prerequisite.entry.sha256, inputs.npm12.entrySha256);
  const literal = prerequisite.literalShim;
  assert.equal(literal.inputPathKeys.length, 1);
  assert.equal(literal.inputPathKeys[0].toUpperCase(), 'PATH');
  assert.deepEqual(literal.pathKeys, ['PATH']);
  assert.deepEqual(literal.pathEntries, result.environment.PATH.split(path.win32.delimiter));
  assert.equal(literal.pathEntries[0], result.receipt.prefix);
  assert.equal(literal.path, result.receipt.actualGlobalShim);
  assert.equal(literal.script.file, 'packed-installed-shim.cmd.raw');
  const shim = await digest(path.join(groupOutput, literal.script.file));
  assert.equal(shim.bytes, literal.script.bytes);
  assert.ok(shim.bytes > 0 && shim.bytes <= 64 * 1024);
  assert.equal(shim.sha256, literal.script.sha256);
  assert.equal(literal.node.expectedPath, await realpath(process.execPath));
  assert.equal(literal.node.resolvedPath, literal.node.expectedPath);
  assert.equal(literal.node.expectedSha256, windowsInputs.node.sha256);
  assert.equal(literal.node.resolvedSha256, windowsInputs.node.sha256);
  assert.equal(result.receipt.npmVersion, inputs.npm12.version);
  assert.equal(result.receipt.lifecycleRequested, true); assert.equal(result.receipt.sha256, artifacts.package.sha256);
  assert.equal(result.receipt.manifest.gitHead, state.sourceHead);
  const org = path.join(code, 'bin/org-plan');
  assert.match(await readFile(org, 'utf8'), /bundles\/[a-f0-9-]{36}\/org-plan-cli\.mjs/);
  const installedOrgPlan = await digest(org);
  const provenance = await json(path.join(code, '.gestalt-setup-provenance.json'));
  assert.equal(provenance.delegatedApi.ownership, 'released');
  assert.equal(provenance.delegatedSetupSha256, (await digest(path.join(agents, 'gestalt-setup.mjs'))).sha256);
  assert.equal(provenance.delegatedApi.ownerSha256, (await digest(path.join(agents, 'scripts/setup-agents.mjs'))).sha256);
  const files = JSON.parse((await readFile(path.join(root, 'native-evidence/binding/package-binding.json'), 'utf8')).trim()).members;
  const installed = [];
  for (const [name, item] of Object.entries(files)) {
    const found = await digest(path.join(result.packageRoot, name));
    assert.equal(found.sha256, item.sha256); assert.equal(found.bytes, item.size);
    installed.push(found);
  }
  assert.equal(installed.length, 10);
  const outcome = { install: result.receipt, provenance, installed, installedOrgPlan, literalShim: literal, lifecycleRoot, providerEvidence: 'No provider/relay/session command is scheduled' };
  await save(path.join(groupOutput, 'package-consumer-binding.json'), outcome);
  await rm(lifecycleRoot, { recursive: true }); lifecycleRoot = null;
  return outcome;
}

try {
  await record();
  assert.equal(process.platform, 'win32');
  assert.equal(process.arch, 'x64');
  assert.equal(process.version, inputs.node);
  assert.ok(Number.isFinite(deadline) && remaining() >= selection.timeoutMs);
  assert.ok(!process.env.GESTALT_SESSION_TMP_CONTEXT);
  assert.ok(!Object.keys(process.env).some(key => /^(GH_TOKEN|GITHUB_TOKEN|OPENAI_|AZURE_|ANTHROPIC_|AWS_|GOOGLE_|GEMINI_|CODEX_)/i.test(key)));
  await group('copied-builder-and-source', [], verifyCopy);
  await group(selection.id, ['copied-builder-and-source'], async () => {
    assert.ok(remaining() >= selection.timeoutMs, 'Remaining whole-lane budget below lifecycle admission floor');
    selection.state = 'started'; selection.startedAt = now(); await record();
    try { return await packagedLifecycle(); }
    finally { selection.observedAt = now(); }
  });
  const row = state.groups.find(item => item.id === selection.id);
  selection.state = row.state;
  assert.ok(state.groups.every(item => item.state === 'pass'), 'Required npm 12 lifecycle failed or remained unexecuted');
  state.state = 'pass';
} catch (error) { state.state = 'fail'; state.failure = errorInfo(error); process.exitCode = 1; }
finally {
  state.finishedAt = now();
  state.retainedLifecycleFixture = lifecycleRoot ?? null;
  state.caseCounts = Object.fromEntries(['pass', 'fail', 'started', 'unexecuted'].map(key => [key, state.cases.filter(item => item.state === key).length]));
  await record();
}
