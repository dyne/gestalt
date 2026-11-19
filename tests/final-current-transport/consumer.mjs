import assert from 'node:assert/strict';
import { mkdir, mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { installPackedCore } from '../fixtures/packed-core.mjs';
import { NativeExecutables } from '../../src/cli/platform/executables.ts';
import { digest, errorInfo, exactPattern, inputs, json, now, phases, root, save, selectedTap, verifyHelper, verifyInventory } from './common.mjs';

const output = path.join(root, 'native-evidence/final-consumer');
const prebuilt = path.join(root, 'native-evidence/prebuilt');
const agents = path.join(root, '.native-dependencies/agents');
const state = { schemaVersion: 1, lane: 'windows-standard-user', state: 'started', startedAt: now(), platform: process.platform,
  arch: process.arch, node: process.version, groups: [], commands: [], failure: null };
const environment = { ...process.env };
const phaseObserver = './tests/final-current-transport/phase-observer.mjs';
const deadline = Date.parse(environment.FINAL_WINDOWS_DEADLINE_UTC ?? '');
const windowsCases = [
  ...inputs.windowsCases.installed.map(name => ({ id: 'installed', kind: 'installed', name,
    file: 'tests/native-setup-installed.test.mjs', timeoutMs: 1_260_000 })),
  ...inputs.windowsCases.transactions.map((name, i) => ({ id: 'transaction-' + (i + 1), kind: 'transaction', name,
    file: 'tests/native-setup-transactions.test.mjs', timeoutMs: 660_000 })),
  { id: 'lifecycle', kind: 'lifecycle', name: 'actual global npm lifecycle and installed literal commands', timeoutMs: 930_000 },
  ...inputs.windowsCases.runtime.map((name, i) => ({ id: 'runtime-' + (i + 1), kind: 'runtime', name,
    file: 'tests/native-runtime-installed.test.mjs', timeoutMs: 660_000 })),
];
state.cases = windowsCases.map(item => ({ ...item, state: 'unexecuted', reason: 'awaiting prerequisites' }));
const remaining = () => deadline - Date.now() - 20_000; // Reserve receipt copy/closure time.
const { group, command, record } = phases(output, state, environment);
let artifacts, helper, windowsInputs, lifecycleRoot;
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
  helper = await verifyHelper(path.join(prebuilt, 'native'));
  const staged = builder.groups.find(row => row.id === 'same-run-helper').detail;
  assert.equal(helper.helper.sha256, staged.helper.sha256);
  assert.equal(helper.provenance.sha256, staged.provenance.sha256);
  assert.deepEqual(helper.source, staged.source);
  assert.equal(artifacts.package.sourceHead, builder.head);
  const packageFile = path.join(prebuilt, artifacts.package.relative);
  assert.equal((await digest(packageFile)).sha256, artifacts.package.sha256);
  state.head = builder.head; state.run = builder.run; state.attempt = builder.attempt;
  const bin = path.dirname(process.execPath), npm = path.join(bin, 'node_modules/npm/bin/npm-cli.js');
  for (const [file, expected] of [[process.execPath, windowsInputs.node], [npm, windowsInputs.npm],
    [path.join(bin, 'codex.exe'), windowsInputs.codex], [path.join(bin, 'npm.cmd'), windowsInputs.npmShim],
    [path.join(bin, 'npm-entry.mjs'), windowsInputs.npmEntry]]) assert.equal((await digest(file)).sha256, expected.sha256);
  const npmVersion = await command('consumer-npm-version', process.execPath, [npm, '--version']);
  assert.equal(npmVersion.text.trim(), inputs.npm.version);
  const codexVersion = await command('consumer-codex-version', path.join(bin, 'codex.exe'), ['--version']);
  assert.equal(codexVersion.text.trim(), 'codex-cli 0.164.0');
  return { helper, npmVersion: npmVersion.text.trim(), codexVersion: codexVersion.text.trim(), managerAndAgentsCopyVerified: true };
}

async function selected(id, file, names, timeout, extra = {}, loader = false) {
  const groupOutput = path.join(output, id); await mkdir(groupOutput, { mode: 0o700 });
  const args = ['--import', phaseObserver, ...(loader ? ['--import', './tests/final-current-transport/prebuilt-loader.mjs'] : []),
    '--test', '--test-reporter=tap', '--test-concurrency=1', '--test-name-pattern=' + exactPattern(names), file];
  const launched = await command(id, process.execPath, args, { timeout,
    env: { ...environment, FINAL_WINDOWS_PHASE_LOG: path.join(groupOutput, 'phases.ndjson'), GESTALT_NATIVE_AGENTS_SOURCE: agents, GESTALT_NATIVE_SETUP_EVIDENCE: groupOutput, ...extra, ...(loader ? { FINAL_WIN_PREBUILT: prebuilt } : {}) } });
  const counts = selectedTap(launched.text, names);
  if (loader) {
    const events = (await readFile(path.join(groupOutput, 'fixture-loader.ndjson'), 'utf8')).trim().split(/\r?\n/).map(JSON.parse);
    assert.deepEqual(events.map(item => item.replacedImport).sort(), ['../scripts/build-cli.mjs','../scripts/build-owned-process.mjs']);
    assert.ok(events.every(item => item.fixture === path.join(root, file)));
    const installed = await json(path.join(groupOutput, 'installed-identity.json'));
    assert.equal(installed.ownershipHelperSha256, helper.helper.sha256);
    assert.equal(installed.dependencySource, await realpath(agents));
    assert.equal(installed.delegatedSetupSha256, (await digest(path.join(agents, 'gestalt-setup.mjs'))).sha256);
    assert.equal(installed.delegatedApiOwnerSha256, (await digest(path.join(agents, 'scripts/setup-agents.mjs'))).sha256);
    assert.equal(installed.managedOwnership, 'released'); assert.equal(installed.repeat, 'offline-success');
    assert.equal(installed.codexStatus, 0); assert.equal(installed.codexVersion, 'codex-cli 0.164.0');
    for (const name of ['fixture-prebuilt-cli.json','fixture-prebuilt-helper.json','installed-identity.json']) await digest(path.join(groupOutput, name));
  }
  return { counts, exactCases: names, stdout: launched.row.stdout };
}

async function packagedLifecycle() {
  lifecycleRoot = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt final npm lifecycle ')));
  const workspace = path.join(lifecycleRoot, 'profile'), groupOutput = path.join(output, 'lifecycle');
  await mkdir(workspace, { mode: 0o700 }); await mkdir(groupOutput, { mode: 0o700 });
  const home = path.join(workspace, 'home'); await mkdir(home, { mode: 0o700 });
  const code = path.join(workspace, 'Codex'), gestalt = path.join(workspace, 'Gestalt');
  const env = { ...environment, HOME: home, USERPROFILE: home, CODEX_HOME: code, GESTALT_HOME: gestalt,
    CONTEXT_MODE_PACKAGE_MANAGER: 'npm', CONTEXT_MODE_PLATFORM: 'codex', CONTEXT_MODE_WORKSPACE: workspace };
  assert.ok(await new NativeExecutables().resolve('npm', env));
  await save(path.join(groupOutput, 'lifecycle-start.json'), { startedAt: now(), fixture: lifecycleRoot,
    npm: inputs.npm.version, sourceHead: state.head, package: artifacts.package, actualGlobalLifecycleRequested: true });
  const result = await installPackedCore(path.join(prebuilt, artifacts.package.relative), lifecycleRoot, env, agents, groupOutput);
  assert.equal(result.receipt.npmVersion, inputs.npm.version);
  assert.equal(result.receipt.lifecycleRequested, true); assert.equal(result.receipt.sha256, artifacts.package.sha256);
  assert.equal(result.receipt.manifest.gitHead, state.head);
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
  const outcome = { install: result.receipt, provenance, installed, lifecycleRoot, providerEvidence: 'No provider/relay/session command is scheduled' };
  await save(path.join(groupOutput, 'package-consumer-binding.json'), outcome);
  await rm(lifecycleRoot, { recursive: true }); lifecycleRoot = null;
  return outcome;
}

async function runCase(item) {
  const minimum = item.timeoutMs;
  const available = remaining();
  if (!Number.isFinite(available) || available < minimum) {
    item.reason = 'remaining whole-lane budget below case admission floor'; await record();
    throw new Error(item.reason);
  }
  item.state = 'started'; item.startedAt = now(); item.reason = null;
  item.commandTimeoutMs = item.kind === 'lifecycle' ? null : item.timeoutMs;
  await record();
  try {
    if (item.kind === 'lifecycle') return await packagedLifecycle();
    const extra = item.kind === 'transaction' ? { GESTALT_NATIVE_SETUP_ARTIFACTS: prebuilt }
      : item.kind === 'runtime' ? { GESTALT_NATIVE_RUNTIME_ARTIFACTS: prebuilt, GESTALT_NATIVE_RUNTIME_EVIDENCE: path.join(output, item.id) }
      : {};
    return await selected(item.id, item.file, [item.name], item.commandTimeoutMs, extra, item.kind === 'installed');
  } finally { item.observedAt = now(); }
}

try {
  await record();
  assert.equal(process.platform, 'win32'); assert.equal(process.arch, 'x64'); assert.equal(process.version, inputs.node);
  assert.ok(Number.isFinite(deadline) && remaining() >= windowsCases[0].timeoutMs);
  assert.ok(!process.env.GESTALT_SESSION_TMP_CONTEXT);
  assert.ok(!Object.keys(process.env).some(key => /^(GH_TOKEN|GITHUB_TOKEN|OPENAI_|AZURE_|ANTHROPIC_|AWS_|GOOGLE_|GEMINI_|CODEX_)/i.test(key)));
  await group('copied-builder-and-source', [], verifyCopy);
  let previous = 'copied-builder-and-source';
  for (const item of state.cases) {
    await group(item.id, [previous], () => runCase(item));
    const outcome = state.groups.find(row => row.id === item.id);
    if (item.state === 'started') item.state = outcome.state;
    if (outcome.state === 'unexecuted') item.reason = 'blocked by ' + outcome.blockedBy.join(', ');
    await record();
    previous = item.id;
  }
  assert.equal(state.cases.length, 9);
  assert.ok(state.groups.every(row => row.state === 'pass') && state.cases.every(item => item.state === 'pass'),
    'Required Windows ordinary case failed or remained unexecuted');
  state.state = 'pass';
} catch (error) { state.state = 'fail'; state.failure = errorInfo(error); process.exitCode = 1; }
finally {
  state.finishedAt = now(); state.retainedLifecycleFixture = lifecycleRoot ?? null;
  state.caseCounts = Object.fromEntries(['pass','fail','started','unexecuted'].map(key =>
    [key, state.cases.filter(item => item.state === key).length]));
  await record();
}
