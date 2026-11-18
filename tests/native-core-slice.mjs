// Explicit target gate; no downloads, profile setup or model activity in npm test.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { access, appendFile, chmod, cp, lstat, mkdir, mkdtemp, readFile, readdir, realpath, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { createServer as httpServer } from 'node:http';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { PassThrough } from 'node:stream';
import { performance } from 'node:perf_hooks';
import { pathToFileURL } from 'node:url';
import { build } from 'esbuild';
import { parse, stringify } from 'smol-toml';
import { buildCli } from '../scripts/build-cli.mjs';
import { buildNpmPackage } from '../scripts/build-npm-package.mjs';
import { managerVersion } from '../scripts/manager-metadata.mjs';
import { NativeExecutables } from '../src/cli/platform/executables.ts';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { NativeOwnedProcesses } from '../src/cli/platform/processes.ts';
import { createScratchOwner } from '../src/cli/runtime/scratch-owner.mjs';
import { nodeHookCommand } from '../src/cli/runtime/hook-command.mjs';
import { observeHostSocket } from './fixtures/unix-compat-peer.mjs';
import { sessionScratch } from './session-scratch-fixture.mjs';
import { authorizeFixtureHooks } from './fixtures/native-cli-hook-trust.mjs';
import { assertMetadataProbesOnly, captureProviderProbes } from './fixtures/native-provider-probes.mjs';
import { terminalControls, terminalSubmission } from './fixtures/native-cli-slice-controls.mjs';
import { queryPhase } from './fixtures/native-cli-spawn-capture.mjs';
import { releasedBorrowerInventory } from './fixtures/native-released-borrowers.mjs';
import { verifySourceRecord, verifyOwnershipBuildInputs } from './fixtures/native-runtime/source-binding.mjs';
import { bounded, digest, isolatedEnvironment, pinnedCodex } from './fixtures/native-runtime/codex.mjs';
import { installPackedCore, npmPrerequisiteTarget } from './fixtures/packed-core.mjs';
import { queryFixtureHooks } from './fixtures/native-hook-query.mjs';

const root = path.resolve(import.meta.dirname, '..'), args = process.argv.slice(2);
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const json = async file => JSON.parse(await readFile(file, 'utf8'));
const record = async (file, value) => writeFile(file, JSON.stringify(value, null, 2) + '\n');
export function selectedSliceMode(environment, platform = process.platform) {
  const flag = environment.GESTALT_CORE_SLICE_READINESS_ONLY;
  if (flag === undefined || flag === '' || flag === '0') return 'terminal';
  if (flag !== '1' || platform !== 'win32') throw Error('Readiness requires explicit flag1 inside the Windows ordinary account');
  return 'readiness';
}
export const readinessArgv = entry => [entry, 'cli', '--', 'exec', '--help'];
export function selectedReadinessComparison(environment, mode, platform = process.platform) {
  const flag = environment.GESTALT_CORE_SLICE_READINESS_COMPARISON;
  if (flag === undefined || flag === '' || flag === '0') return false;
  if (flag !== '1' || mode !== 'readiness' || platform !== 'win32') throw Error('Comparison requires explicit flag1 with Windows readiness selected');
  return true;
}
export function observerFreeEnvironment(observed, observerImport) {
  assert.equal(observed.NODE_OPTIONS, '--import=' + observerImport, 'Remove only this fixture observer import');
  const environment = { ...observed, NODE_OPTIONS: '' };
  for (const key of ['GESTALT_SLICE_CODEX_BINARY', 'GESTALT_SLICE_SPAWN_CAPTURE', 'GESTALT_SLICE_READINESS_CAPTURE']) delete environment[key];
  const changes = [...new Set([...Object.keys(observed), ...Object.keys(environment)])].sort().filter(key => observed[key] !== environment[key])
    .map(key => ({ key, before: observed[key] ?? null, after: environment[key] ?? null, afterPresent: Object.hasOwn(environment, key) }));
  assert.deepEqual(changes.map(change => change.key), ['GESTALT_SLICE_CODEX_BINARY', 'GESTALT_SLICE_READINESS_CAPTURE', 'GESTALT_SLICE_SPAWN_CAPTURE', 'NODE_OPTIONS']);
  return { environment, changes };
}
export async function readinessComparison(run, save) {
  const cases = [];
  for (const control of ['comparator', 'readiness']) {
    let failure = null;
    try { await run(control); } catch (error) { failure = { message: error.message, stack: error.stack }; }
    const result = { control, completed: failure === null, failure };
    cases.push(result);
    try { await save(result); }
    catch (error) { result.completed = false; result.captureFailure = { message: error.message, stack: error.stack }; }
  }
  return { cases, pass: cases.every(result => result.completed), plannedQueryInvocations: 2, submittedTurns: 0, terminalCasesExecuted: 0 };
}
export function finalCapturedInvocation(calls, mode = 'terminal') {
  if (mode === 'terminal') return calls.filter(call => call.argv[0] !== 'app-server' && call.argv.includes('--no-alt-screen')).at(-1);
  const selected = calls.filter(call => queryPhase(call.argv) === 'final-help');
  assert.equal(selected.length, 1, 'Readiness requires one actual final exec --help invocation');
  return selected[0];
}
export function readinessObservations(observation, inherited, target, helper) {
  const failures = [], required = ['required-inventory', 'required-mcp', 'capability-config', 'capability-hooks', 'final-help'];
  const fail = id => failures.push(id);
  if (!observation || observation.overflow || observation.errors?.length || observation.processExit !== 0) fail('observation-missing-overflow-error-or-exit');
  const channels = Array.isArray(observation?.channels) ? observation.channels : [], events = Array.isArray(observation?.events) ? observation.events : [];
  if (channels.length !== required.length) fail('expected-five-owned-product-phases');
  for (const phase of required) {
    const matches = channels.filter(channel => channel.phase === phase);
    if (matches.length !== 1) { fail(phase + ':missing-or-duplicate'); continue; }
    const channel = matches[0], current = events.filter(event => event.channel === channel.id);
    if (!channel.call || typeof channel.call.target !== 'string' || !channel.environment ||
        !Array.isArray(channel.incompleteFrameBytes) || !Array.isArray(channel.incompleteRpcBytes)) { fail(phase + ':malformed-observation'); continue; }
    if (channel.call.transport !== 'accepted-owned-helper' || channel.call.target.toLowerCase() !== target.toLowerCase()) fail(phase + ':wrong-owned-target');
    if (helper && (typeof channel.call.executable !== 'string' || channel.call.executable.toLowerCase() !== helper.toLowerCase())) fail(phase + ':wrong-current-helper');
    if (Object.keys(inherited).some(key => channel.environment[key] !== inherited[key])) fail(phase + ':context-or-TMP-changed');
    if ([...channel.incompleteFrameBytes, ...channel.incompleteRpcBytes].some(Boolean)) fail(phase + ':incomplete-frame-or-RPC');
    const frames = current.filter(event => event.type === 'native-frame'), payload = kind => frames.filter(event => event.kind === kind);
    if (['R', 'F', 'D'].some(kind => payload(kind).length !== 1) || frames.some(event => ['U', 'X'].includes(event.kind))) fail(phase + ':missing-native-RFD-or-unknown');
    if (payload('R').length === 1 && payload('F').length === 1 && payload('D').length === 1 &&
        !(frames.indexOf(payload('R')[0]) < frames.indexOf(payload('F')[0]) && frames.indexOf(payload('F')[0]) < frames.indexOf(payload('D')[0]))) fail(phase + ':native-frame-order');
    const text = kind => Buffer.from(payload(kind)[0]?.payloadBase64 ?? '', 'base64').toString();
    if (text('R') !== 'windows-job' || text('D') !== 'windows-job') fail(phase + ':wrong-native-backend');
    const closes = current.filter(event => event.type === 'helper-close');
    if (closes.length !== 1 || closes[0].code !== 0 || closes[0].signal !== null) fail(phase + ':helper-not-naturally-closed0');
    if (closes.length === 1 && payload('D').length === 1 && current.indexOf(closes[0]) <= current.indexOf(payload('D')[0])) fail(phase + ':close-before-empty-frame');
    if (phase.startsWith('capability-')) {
      const methods = phase === 'capability-config' ? ['initialize', 'config/read', 'skills/list'] : ['initialize', 'hooks/list'];
      for (const method of methods) {
        const requests = current.filter(event => event.type === 'rpc' && event.direction === 'input' && event.method === method);
        if (requests.length !== 1) { fail(phase + ':RPC-' + method + '-missing-or-duplicate'); continue; }
        const replies = current.filter(event => event.type === 'rpc' && event.direction === 'output' && event.id === requests[0].id);
        if (replies.length !== 1 || replies[0].error || replies[0].response !== true || replies[0].hasResult !== true) fail(phase + ':RPC-' + method + '-incomplete-or-error');
      }
      // Existing snapshot closes its server after replies. Keep the original
      // F result; an intentional stop is not normalized into natural command0.
      if (!/^exit:\d+$|^signal:\d+$/.test(text('F'))) fail(phase + ':invalid-command-result');
      if (text('F') !== 'exit:0' && !current.some(event => event.type === 'native-control' && event.kind === 'S' && current.indexOf(event) < current.indexOf(payload('F')[0]))) fail(phase + ':nonzero-without-prior-observed-owned-stop');
    } else if (text('F') !== 'exit:0') fail(phase + ':command-not-natural0');
  }
  const created = events.filter(event => event.type === 'timer-created');
  if (created.filter(event => /\bat [^\n]*\.capture\s+\(/.test(event.callsite) && event.delay === 10000).length < 2) fail('required-capture10s-timers-unobserved');
  if (created.filter(event => /\bat snapshot\s+\(/.test(event.callsite) && event.delay > 0 && event.delay <= 5000).length < 2) fail('per-query5s-snapshot-cap-timers-unobserved');
  return { pass: failures.length === 0, failures, requiredPhases: required, rawInnerRecordsAreLeaseClaims: false, timerClearProvesNonexpiry: false, timerObservationScope: 'Per-query5s cap only; shared global budget is source policy, not an observed callback timestamp' };
}
function observeTerminalCase(control, report, terminalReport, attempted, readError) {
  if (!report) return { control, status: attempted ? 'unknown-no-valid-terminal-report' : 'unexecuted',
    terminalReport, readError, fixtureTurnSubmitted: null, barrierTextObserved: null };
  const submitted = typeof report.submittedBlockedFixtureTurn === 'boolean' ? report.submittedBlockedFixtureTurn : null;
  const output = typeof report.outputBase64 === 'string' ? Buffer.from(report.outputBase64, 'base64').toString() : null;
  const blocked = output === null ? null : output.includes('Native core slice no-model barrier');
  return { control, status: 'observed-terminal-report', terminalReport,
    fixtureTurnSubmitted: submitted, barrierTextObserved: blocked,
    ...(report.submission ? { inputWriteFacts: report.submission,
      acceptedWorkObserved: report.submission.acceptedWorkObserved,
      submissionScope: 'Legacy submitted flag records control delivery; accepted work requires fresh Working and actual authority/barrier' } : {}),
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
  let helper = null;
  if (process.platform === 'win32') {
    helper = path.join(output, 'native/gestalt-owned-process.exe');
    if (process.env.GESTALT_CORE_SLICE_NATIVE_ARTIFACTS) {
      const supplied = await realpath(process.env.GESTALT_CORE_SLICE_NATIVE_ARTIFACTS);
      for (const suffix of ['', '.provenance.json']) await cp(path.join(supplied, 'gestalt-owned-process.exe' + suffix), helper + suffix);
      const provenance = await json(helper + '.provenance.json');
      assert.equal(provenance.platform, 'win32'); assert.equal(provenance.architecture, process.arch);
      assert.equal(provenance.artifactSha256, (await digest(helper)).sha256);
      await verifyOwnershipBuildInputs(provenance, root);
    } else helper = await buildOwnedProcess(root, path.join(output, 'native'));
  }
  const nativeHelperInputs = helper ? await verifyOwnershipBuildInputs(await json(helper + '.provenance.json'), root) : null;
  const files = [path.join(output, 'gestalt.mjs'), path.join(output, 'legacy/gestalt'), path.join(output, 'authority.mjs'), path.join(output, 'spawn-capture.mjs'),
    ...[peer, helper].filter(Boolean).flatMap(file => [file, file + '.provenance.json'])];
  const artifacts = await Promise.all(files.map(async file => ({ relative: path.relative(output, file), ...await digest(file) })));
  assert.equal(sha256(await readFile(path.join(output, 'legacy/gestalt'))), sha256(await readFile(path.join(root, 'public/gestalt'))));
  const inputs = await Promise.all(Object.keys(compiled.metafile.inputs).map(file => digest(path.resolve(file))));
  let packed = null;
  const sourceManifest = path.join(root, 'npm/package.json');
  if (process.env.GESTALT_CORE_SLICE_NPM_TARBALL) {
    assert.equal(process.env.GESTALT_CORE_SLICE_PACKAGE, '1');
    const sourceHead = process.env.GESTALT_CORE_SLICE_NPM_SOURCE_HEAD;
    assert.match(sourceHead ?? '', /^[a-f0-9]{40}$/, 'Frozen tarball requires its exact source commit');
    const supplied = await realpath(process.env.GESTALT_CORE_SLICE_NPM_TARBALL), actual = await digest(supplied);
    assert.equal(actual.sha256, process.env.GESTALT_CORE_SLICE_NPM_SHA256, 'Frozen package input must match its explicitly reviewed digest');
    const filename = 'packed-core-candidate.tgz'; await cp(supplied, path.join(output, filename));
    packed = { ...await digest(path.join(output, filename)), relative: filename,
      supplied: actual, sourceHead, sourceManifest: await digest(sourceManifest) };
  } else if (process.env.GESTALT_CORE_SLICE_PACKAGE === '1') {
    const destination = await buildNpmPackage(root, { nativeDirectory: path.join(output, 'native') });
    const packageManifest = await json(path.join(destination, 'package.json'));
    assert.ok(packageManifest.os.includes(process.platform), 'Source package must admit the actual consumer platform');
    const npm = await new NativeExecutables().resolve('npm', process.env);
    assert.ok(npm); const target = await npmPrerequisiteTarget(npm, {});
    const result = JSON.parse(execFileSync(target.executable, [...target.prefix, 'pack', destination, '--json', '--ignore-scripts',
      '--pack-destination', path.resolve(output), '--cache', path.join(root, 'node_modules/.cache/gestalt-package')], { cwd: root, encoding: 'utf8' }));
    const item = Array.isArray(result) ? result[0] : result[packageManifest.name];
    assert.ok(item?.filename);
    packed = { ...await digest(path.join(output, item.filename)), relative: item.filename, sourceHead: packageManifest.gitHead,
      sourceManifest: await digest(sourceManifest), packedManifest: await digest(path.join(destination, 'package.json')) };
  }
  await record(path.join(output, 'artifacts.json'), { observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch,
    node: process.version, abi: process.versions.modules, peer: peer ? path.relative(output, peer) : null,
    helper: helper ? path.relative(output, helper) : null, nativeHelperInputs, artifacts, package: packed, authorityBuildInputs: inputs, orgSource: await sourceBindings(agents),
    scope: 'Builder only. The copied compiled consumer requires no compiler, TS source or repository modules.' });
}

async function trustFixtureHooks(binary, environment, cwd, configFile, expected, evidence) {
  const before = await queryFixtureHooks(binary, environment, cwd, [], path.join(evidence, 'hook-query-before-trust.json'));
  const hooks = before.result.result?.data?.find(entry => entry.cwd === cwd)?.hooks ?? [];
  const authorized = authorizeFixtureHooks(hooks, expected), config = parse(await readFile(configFile, 'utf8'));
  config.hooks.state ??= {};
  for (const hook of authorized) config.hooks.state[hook.key] = { enabled: true, trusted_hash: hook.currentHash };
  await writeFile(configFile, stringify(config));
  const after = await queryFixtureHooks(binary, environment, cwd, [], path.join(evidence, 'hook-query-after-trust.json'));
  const current = after.result.result?.data?.find(entry => entry.cwd === cwd)?.hooks ?? [];
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

async function composedHooks(binary, environment, cwd, capture, mode, evidence) {
  const calls = (await readFile(capture, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  const tui = finalCapturedInvocation(calls, mode);
  assert.ok(tui, 'Actual candidate final Codex argv was not observed');
  const overrides = [];
  for (let i = 0; i < tui.argv.length; i++) {
    if (tui.argv[i] === '-c' || tui.argv[i] === '--config') { assert.ok(i + 1 < tui.argv.length); overrides.push('-c', tui.argv[++i]); }
    else if (tui.argv[i].startsWith('--config=')) overrides.push('-c', tui.argv[i].slice(9));
  }
  const effective = await queryFixtureHooks(binary, environment, cwd, overrides, path.join(evidence, 'hook-query-composed.json'));
  const hooks = effective.result.result?.data?.find(entry => entry.cwd === cwd)?.hooks ?? [];
  assert.equal(hooks.filter(hook => hook.source === 'user').length, 7);
  assert.ok(hooks.filter(hook => hook.source === 'sessionFlags').length >= 2, 'Manager-composed hook flags were not exercised');
  for (const hook of hooks) assert.equal(hook.trustStatus, 'trusted', JSON.stringify(hook));
  return { calls, exactFinalArgv: tui.argv, effective,
    scope: 'Actual captured final candidate argv, then exact -c flags queried through real Codex hooks/list; no protocol/stream interception or unscoped trust' };
}

async function borrowerInventory(primary) {
  const directory = path.join(primary.context.directory, 'leases'), records = [];
  for (const name of (await readdir(directory)).sort()) {
    const file = path.join(directory, name), info = await lstat(file);
    assert.ok(info.isFile() && !info.isSymbolicLink() && info.nlink === 1 && info.size <= 16384);
    const bytes = await readFile(file); records.push({ file, sha256: sha256(bytes), lease: JSON.parse(bytes) });
  }
  return records;
}
async function queryComparator(entry, observedEnvironment, cwd, directory, helper, primary, configFile, hooksFile, inherited, observerImport) {
  const { environment, changes } = observerFreeEnvironment(observedEnvironment, observerImport);
  const policy = async label => Promise.all([configFile, hooksFile].map(async file => {
    const bytes = await readFile(file), copy = path.join(directory, label + '-' + path.basename(file)); await writeFile(copy, bytes, { flag: 'wx', mode: 0o600 });
    return { file, sha256: sha256(bytes), bytes: bytes.length, copy };
  }));
  const beforePolicy = await policy('before'), beforeBorrowers = await borrowerInventory(primary);
  const adapter = new NativeOwnedProcesses(helper), stdout = [], stderr = [];
  let lease, outcome, claim, failure, afterBorrowers, afterPolicy, resultMonotonicMs;
  await record(path.join(directory, 'invocation.json'), { executable: process.execPath, argv: readinessArgv(entry), cwd, inherited,
    environmentDeltaFromThisFixturesObservedInvocation: changes, observerImport, observerEnabled: false,
    scope: 'Exactly the test observer import and its three capture inputs removed. Other environment values, including owner/TMP, are unchanged within this fresh fixture; separate fixtures have different homes/owners/ports.' });
  const startedMonotonicMs = performance.now(), startedAt = new Date().toISOString();
  try {
    lease = await adapter.launch(process.execPath, { argv: readinessArgv(entry), cwd, environment }, {
      stdin: (async function* () {})(), stdout: async bytes => stdout.push(Buffer.from(bytes)), stderr: async bytes => stderr.push(Buffer.from(bytes)),
    }, { graceMs: 1000, deadlineMs: 10000 });
    outcome = await bounded(lease.result, 'actual compiled Windows query comparator', 300000);
    resultMonotonicMs = performance.now();
  } catch (error) { failure = { message: error.message, stack: error.stack }; }
  finally {
    if (lease) {
      try { claim = await lease.stop(); await bounded(lease.completed, 'query comparator helper closure', 15000); }
      catch (error) { failure ??= { message: error.message, stack: error.stack }; }
    }
    try { afterBorrowers = await borrowerInventory(primary); afterPolicy = await policy('after'); }
    catch (error) { failure ??= { message: error.message, stack: error.stack }; }
  }
  const completedMonotonicMs = performance.now(), output = Buffer.concat(stdout).toString(), diagnostics = Buffer.concat(stderr).toString();
  const errors = [];
  if (failure) errors.push('comparator-execution-or-capture-failure');
  if (outcome?.kind !== 'exit' || outcome.code !== 0 || claim?.kind !== 'released' || claim.lease !== lease?.id ||
      claim.backend !== 'windows-job' || claim.result?.kind !== 'exit' || claim.result.code !== 0) errors.push('outer-natural0-or-matching-lease-unconfirmed');
  if (!/Usage:.*codex exec/s.test(output)) errors.push('native-exec-help-unobserved');
  if (!afterPolicy || beforePolicy.some((value, index) => value.file !== afterPolicy[index]?.file || value.sha256 !== afterPolicy[index]?.sha256)) errors.push('configuration-policy-bytes-changed');
  const borrowerGate = releasedBorrowerInventory(primary, beforeBorrowers, afterBorrowers), added = borrowerGate.added;
  if (!borrowerGate.pass) errors.push('new-runtime-borrowers-not-confirmed-released', ...borrowerGate.failures);
  const beforePrimary = beforeBorrowers.find(value => value.lease.leaseId === primary.lease.leaseId), afterPrimary = afterBorrowers?.find(value => value.lease.leaseId === primary.lease.leaseId);
  if (beforePrimary?.lease.state !== 'active' || afterPrimary?.lease.state !== 'active' || beforePrimary?.sha256 !== afterPrimary?.sha256) errors.push('primary-fixture-lease-not-retained-active-and-unchanged');
  const result = { kind: 'actual-compiled-windows-observer-free-query-comparator', measurementComplete: errors.length === 0, failures: errors,
    result: outcome ?? null, outerClaim: claim ?? null, outerLease: lease?.id ?? null, failure: failure ?? null,
    startedAt, startedMonotonicMs, resultMonotonicMs: resultMonotonicMs ?? null, completedMonotonicMs,
    elapsedToResultMs: resultMonotonicMs === undefined ? null : resultMonotonicMs - startedMonotonicMs, elapsedThroughClosureMs: completedMonotonicMs - startedMonotonicMs,
    fallbackObserved: /capability discovery unavailable|startup guidance unavailable|complete descendant release is unconfirmed/.test(diagnostics),
    beforePolicy, afterPolicy, beforeBorrowers, afterBorrowers, addedBorrowers: added, borrowerGate, environmentChanges: changes, inherited,
    readinessPass: null, rawInnerRecordsClaimed: false, innerPhasesClaimed: false, submittedTurns: 0, terminalCasesExecuted: 0,
    scope: 'Observer-free symptom comparator only. Help0, no fallback and outer Job release do not prove complete inner readiness. Separate fresh fixtures/order/OS caches limit causal comparison.' };
  await writeFile(path.join(directory, 'manager.stdout'), Buffer.concat(stdout)); await writeFile(path.join(directory, 'manager.stderr'), Buffer.concat(stderr));
  await record(path.join(directory, 'query-result.json'), result);
  assert.equal(result.measurementComplete, true, JSON.stringify(errors)); return result;
}
async function queryReadiness(entry, environment, cwd, directory, helper, primary, configFile, hooksFile, inherited) {
  const adapter = new NativeOwnedProcesses(helper), stdout = [], stderr = [];
  const policy = async label => Promise.all([configFile, hooksFile].map(async file => {
    const bytes = await readFile(file), copy = path.join(directory, label + '-' + path.basename(file)); await writeFile(copy, bytes, { flag: 'wx', mode: 0o600 });
    return { file, sha256: sha256(bytes), bytes: bytes.length, copy };
  }));
  const beforePolicy = await policy('before'), beforeBorrowers = await borrowerInventory(primary);
  let lease, outcome, claim, failure, observation, afterBorrowers, afterPolicy;
  try {
    lease = await adapter.launch(process.execPath, { argv: readinessArgv(entry), cwd, environment }, {
      stdin: (async function* () {})(), stdout: async bytes => stdout.push(Buffer.from(bytes)), stderr: async bytes => stderr.push(Buffer.from(bytes)),
    }, { graceMs: 1000, deadlineMs: 10000 });
    outcome = await bounded(lease.result, 'actual compiled Windows query readiness', 300000);
  } catch (error) { failure = { message: error.message, stack: error.stack }; }
  finally {
    if (lease) {
      try { claim = await lease.stop(); await bounded(lease.completed, 'query readiness helper closure', 15000); }
      catch (error) { failure ??= { message: error.message, stack: error.stack }; }
    }
    try { observation = await json(environment.GESTALT_SLICE_READINESS_CAPTURE); } catch (error) { failure ??= { message: 'Readiness observation unavailable: ' + error.message }; }
    try { afterBorrowers = await borrowerInventory(primary); afterPolicy = await policy('after'); } catch (error) { failure ??= { message: error.message }; }
  }
  const observed = readinessObservations(observation, inherited, environment.GESTALT_SLICE_CODEX_BINARY, helper);
  const errors = [...observed.failures];
  if (failure) errors.push('readiness-execution-or-capture-failure');
  if (JSON.stringify(observation?.managerArgv?.slice(1)) !== JSON.stringify(readinessArgv(entry))) errors.push('observation-not-from-exact-manager-invocation');
  if (outcome?.kind !== 'exit' || outcome.code !== 0 || claim?.kind !== 'released' || claim.lease !== lease?.id ||
      claim.backend !== 'windows-job' || claim.result?.kind !== 'exit' || claim.result.code !== 0) errors.push('outer-natural0-or-matching-lease-unconfirmed');
  const output = Buffer.concat(stdout).toString(), diagnostics = Buffer.concat(stderr).toString();
  if (!/Usage:.*codex exec/s.test(output)) errors.push('native-exec-help-unobserved');
  if (/capability discovery unavailable|startup guidance unavailable|complete descendant release is unconfirmed/.test(diagnostics)) errors.push('required-query-or-guidance-fallback');
  if (!afterPolicy || beforePolicy.some((record, index) => record.file !== afterPolicy[index]?.file || record.sha256 !== afterPolicy[index]?.sha256)) errors.push('configuration-policy-bytes-changed');
  const borrowerGate = releasedBorrowerInventory(primary, beforeBorrowers, afterBorrowers, 5), added = borrowerGate.added;
  if (!borrowerGate.pass) errors.push('new-runtime-borrowers-not-five-released', ...borrowerGate.failures);
  const beforePrimary = beforeBorrowers.find(record => record.lease.leaseId === primary.lease.leaseId), afterPrimary = afterBorrowers?.find(record => record.lease.leaseId === primary.lease.leaseId);
  if (beforePrimary?.lease.state !== 'active' || afterPrimary?.lease.state !== 'active' || beforePrimary?.sha256 !== afterPrimary?.sha256) errors.push('primary-fixture-lease-not-retained-active-and-unchanged');
  const result = { kind: 'actual-compiled-windows-owned-query-readiness', pass: errors.length === 0, failures: errors,
    result: outcome ?? null, outerClaim: claim ?? null, outerLease: lease?.id ?? null, failure: failure ?? null,
    observationGate: observed, beforePolicy, afterPolicy, beforeBorrowers, afterBorrowers, addedBorrowers: added, borrowerGate,
    submittedTurns: 0, terminalCasesExecuted: 0, modelRequestPolicy: 'No generation endpoint; fixture submits only exec --help',
    scope: 'Instrumented readiness only; inner raw records are not adapter lease UUID claims. Timer clear alone proves no deadline outcome.' };
  await writeFile(path.join(directory, 'manager.stdout'), Buffer.concat(stdout)); await writeFile(path.join(directory, 'manager.stderr'), Buffer.concat(stderr));
  await record(path.join(directory, 'query-result.json'), result);
  assert.equal(result.pass, true, JSON.stringify(errors)); return result;
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
  const controls = terminalControls(control), submission = terminalSubmission();
  const adapter = new NativeOwnedProcesses(helper);
  try {
    lease = await adapter.launchTerminal(process.execPath,
      { argv: [entry, 'cli', '--no-alt-screen', '--sandbox', 'read-only', '--ask-for-approval', 'never', '-C', cwd], cwd, environment },
      { input, output: async bytes => {
        chunks.push(Buffer.from(bytes)); const query = tail + Buffer.from(bytes).toString('latin1');
        for (const _ of query.matchAll(/\x1b\[6n/g)) input.write('\x1b[1;1R'); tail = query.slice(-3);
        const output = Buffer.concat(chunks).toString();
        const authorityReady = await access(report).then(() => true, () => false);
        const submitBytes = submission.observe(output, authorityReady);
        if (submitBytes) input.write(submitBytes);
        submitted = submission.facts().submitControlWritten;
        const controlBytes = controls.observe(output, authorityReady);
        if (controlBytes) { input.write(controlBytes); controlSent = true; }
      } }, { columns: 110, rows: 40 }, { graceMs: 1000, deadlineMs: 10000 });
    const outcome = await bounded(lease.result, 'actual compiled Windows consumer TUI', 300000);
    const claim = await lease.stop(); await lease.completed;
    const result = { result: outcome, claim, submittedBlockedFixtureTurn: submitted, submission: submission.facts(), control, controlSent, controlEvents: controls.events,
      outputBase64: Buffer.concat(chunks).toString('base64'), observedAt: new Date().toISOString() };
    await record(path.join(evidence, 'terminal.json'), result);
    assert.equal(submitted, true); assert.equal(submission.facts().acceptedWorkObserved, true); assert.equal(controlSent, true); assert.deepEqual(outcome, { kind: 'exit', code: 0 }); assert.equal(claim.kind, 'released');
    return result;
  } catch (error) { failure = { message: error.message, stack: error.stack }; throw error; }
  finally {
    if (lease) {
      const claim = await lease.stop();
      await record(path.join(evidence, 'terminal-final.json'), { claim, failure: failure ?? null, submittedBlockedFixtureTurn: submitted, submission: submission.facts(), control, controlSent, controlEvents: controls.events,
        outputBase64: Buffer.concat(chunks).toString('base64'), observedAt: new Date().toISOString() });
      await bounded(lease.completed, 'native consumer helper closure', 15000);
    }
    input.destroy();
  }
}

async function consume(artifacts, agents, evidence, mode) {
  const queryMode = mode === 'readiness' || mode === 'comparator';
  await mkdir(evidence, { recursive: true, mode: 0o700 });
  const manifest = await json(path.join(artifacts, 'artifacts.json'));
  assert.equal(manifest.platform, process.platform); assert.equal(manifest.arch, process.arch); assert.equal(manifest.abi, process.versions.modules);
  for (const expected of manifest.artifacts) { const actual = await digest(path.join(artifacts, expected.relative)); assert.equal(actual.sha256, expected.sha256); assert.equal(actual.bytes, expected.bytes); }
  if (manifest.package) {
    const packed = await digest(path.join(artifacts, manifest.package.relative));
    assert.equal(packed.sha256, manifest.package.sha256); assert.equal(packed.bytes, manifest.package.bytes);
  }
  if (manifest.helper) {
    const helper = path.join(artifacts, manifest.helper), provenance = await json(helper + '.provenance.json');
    assert.equal(provenance.artifactSha256, (await digest(helper)).sha256);
    const inputs = await verifyOwnershipBuildInputs(provenance, root); assert.deepEqual(inputs, manifest.nativeHelperInputs);
    await record(path.join(evidence, 'native-helper-inputs.json'), { inputs, provenance, scope: 'All current native C/header inputs and exact copied helper bytes; not execution proof' });
  }
  const binding = await sourceBindings(agents, manifest.orgSource); await record(path.join(evidence, 'accepted-agents-source.json'), binding);
  // Preserve the caller's validated TMP/context. Missing privacy is a hard
  // target failure, not an all-skipped gate or a remapped local TMP workaround.
  const outer = await realpath(await mkdtemp(path.join(tmpdir(), 'gc-'))), probe = manifest.peer ? path.join(artifacts, manifest.peer) : undefined;
  const storage = await NativePrivateStorage.open(path.join(outer, 'private'), probe), fixture = storage.root;
  await record(path.join(evidence, 'fixture.json'), { outer, fixture, originalOwnerContext: Object.fromEntries(
    ['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].map(key => [key, process.env[key] ?? null])), observedAt: new Date().toISOString() });
  for (const name of ['home', 'Codex', 'Gestalt', 'bin', 'workspace', 'status']) await storage.createDirectory(path.join(fixture, name));
  const consumer = path.join(fixture, 'consumer'); await cp(artifacts, consumer, { recursive: true });
  let entry = path.join(consumer, 'gestalt.mjs');
  const peer = manifest.peer ? path.join(consumer, manifest.peer) : undefined, helper = manifest.helper ? path.join(consumer, manifest.helper) : undefined;
  if (queryMode) assert.equal((await digest(entry)).sha256, '651db98194c2a24aa2da9f9e9ff9f8df2090c895df1a85ee4527f55c5fcae463', 'Readiness must consume the sealed current product binary');
  const artifact = await pinnedCodex(path.join(evidence, 'codex-artifact'), { homeDirectory: path.join(fixture, 'Codex') });
  await cp(artifact.binary, path.join(fixture, 'bin', process.platform === 'win32' ? 'codex.exe' : 'codex'));
  if (process.platform !== 'win32') await chmod(path.join(fixture, 'bin/codex'), 0o700);
  const isolated = isolatedEnvironment(path.join(fixture, 'Codex'));
  for (const key of Object.keys(isolated)) if (key.startsWith('GESTALT_') && key !== 'GESTALT_SESSION_TMP_CONTEXT') delete isolated[key];
  let environment = { ...isolated, HOME: path.join(fixture, 'home'), USERPROFILE: path.join(fixture, 'home'), NODE_OPTIONS: '', NODE_PATH: '',
    GESTALT_HOME: path.join(fixture, 'Gestalt'), PATH: [path.join(fixture, 'bin'), path.dirname(process.execPath), process.env.PATH].join(path.delimiter),
    npm_config_ignore_scripts: 'true', CONTEXT_MODE_PACKAGE_MANAGER: 'npm', CLAUDE_CONFIG_DIR: path.join(fixture, 'home/.claude'), CONTEXT_MODE_DIR: path.join(fixture, 'home/context-mode-data') };
  for (const key of Object.keys(environment)) if (key.toUpperCase() === 'PATH' && key !== 'PATH') delete environment[key];
  const baseEnvironment = environment;
  const npm12Entry = process.env.GESTALT_CORE_SLICE_NPM12_ENTRY;
  if (npm12Entry) assert.ok(process.platform === 'win32' && manifest.package && !queryMode);
  let packed, expectedPackage;
  if (manifest.package) {
    assert.equal(queryMode, false, 'Packed installation must exercise the actual terminal/Org/IPC slice');
    const sourceManifest = path.join(root, 'npm/package.json');
    assert.equal((await digest(sourceManifest)).sha256, manifest.package.sourceManifest.sha256);
    assert.match(manifest.package.sourceHead, /^[a-f0-9]{40}$/);
    expectedPackage = { ...await json(sourceManifest),
      version: managerVersion(await readFile(path.join(root, 'public/gestalt'), 'utf8')), gitHead: manifest.package.sourceHead };
    assert.ok(expectedPackage.os.includes(process.platform));
    packed = await installPackedCore(await realpath(path.join(artifacts, manifest.package.relative)), fixture, environment, agents, evidence);
    await record(path.join(evidence, 'packed-install.json'), packed.receipt);
    assert.deepEqual(packed.receipt.manifest, expectedPackage);
    if (npm12Entry) assert.match(packed.receipt.npmVersion, /^11\./);
    entry = packed.entry; environment = packed.environment;
    assert.equal((await digest(path.join(packed.packageRoot, 'lib/gestalt.mjs'))).sha256,
      (await digest(path.join(consumer, 'gestalt.mjs'))).sha256, 'Installed payload must equal the source-bound compiled candidate');
    if (helper) assert.equal((await digest(path.join(packed.packageRoot, 'lib/native/gestalt-owned-process.exe'))).sha256,
      (await digest(helper)).sha256, 'Installed helper must equal the source-bound target-built artifact');
  }
  // Setup is the accepted dependency's explicit standalone fixture mode, with
  // its honest no-complete-tree result. It is not a manager fallback or gate.
  const setupApi = await import(pathToFileURL(path.join(agents, 'gestalt-setup.mjs')));
  assert.equal(setupApi.SETUP_API_VERSION, 1); assert.equal(typeof setupApi.setupAgents, 'function');
  for (const name of ['host-filesystem-v1', 'async-owned-commands-v1', 'shared-context-v1']) assert.ok(setupApi.SETUP_API_CAPABILITIES.includes(name));
  let setup, setupFailure;
  const log = bytes => appendFile(path.join(evidence, 'fixture-setup.log'), bytes);
  try { setup = packed && process.platform === 'win32' ? { scope: 'Actual npm global lifecycle installed accepted Agents' } :
    await setupApi.setupAgents([], { mode: 'standalone', environment, cwd: fixture,
      streams: { stdin: (async function* () {})(), stdout: log, stderr: log } }); }
  catch (error) { setupFailure = { message: error.message, stack: error.stack }; throw error; }
  finally { await record(path.join(evidence, 'fixture-setup.json'), { outcome: setup ?? null, failure: setupFailure ?? null,
    scope: 'Isolated accepted Agents fixture preparation, not a new product managed release proof' }); }
  const dispatcher = await readFile(path.join(fixture, 'Codex/bin/org-plan'), 'utf8'), bundle = /bundles\/([a-f0-9-]{36})\/org-plan-cli\.mjs/.exec(dispatcher)?.[1];
  assert.ok(bundle, 'Actual installed dispatcher must pin one immutable Org bundle');
  const orgBundle = path.join(fixture, 'Codex/lib/gestalt-org-plan/bundles', bundle), configFile = path.join(fixture, 'Codex/config.toml');
  let readinessInstalledBindings;
  if (queryMode) {
    readinessInstalledBindings = [];
    for (const expected of binding.binding.files) {
      const file = path.join(orgBundle, path.basename(expected.path)), bytes = await readFile(file), info = await lstat(file);
      assert.ok(info.isFile() && !info.isSymbolicLink()); assert.equal(bytes.length, expected.bytes); assert.equal(sha256(bytes), expected.sha256);
      readinessInstalledBindings.push({ file, bytes: bytes.length, sha256: sha256(bytes), observedMode: (info.mode & 0o7777).toString(8), expectedGitMode: expected.gitMode });
    }
    assert.equal(readinessInstalledBindings.length, 15);
    await record(path.join(evidence, 'readiness-installed-bindings.json'), { files: readinessInstalledBindings,
      scope: 'Actual installed bytes; Windows stat mode is observed, not POSIX execute-bit semantics or a publication/IPC claim' });
  }
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
  let activeCase = null;
  let runFailure = null;
  const attemptedCases = new Set();
  const selectedCases = queryMode ? [mode] : ['eof', 'interrupt'];
  const sentinel = httpServer(captureProviderProbes(requests, () => activeCase));
  sentinel.listen(0, '127.0.0.1'); await once(sentinel, 'listening');
  try {
    for (const control of selectedCases) {
      activeCase = control;
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
        await record(path.join(current, 'fixture-hook-trust.json'), await trustFixtureHooks(artifact.binary, launchEnvironment, path.join(fixture, 'workspace'), configFile, expected, current));
        const capture = path.join(current, 'candidate-spawn.jsonl');
        const observedEnvironment = { ...launchEnvironment, NODE_OPTIONS: '--import=' + pathToFileURL(path.join(consumer, 'spawn-capture.mjs')).href,
          GESTALT_SLICE_CODEX_BINARY: path.join(fixture, 'bin', process.platform === 'win32' ? 'codex.exe' : 'codex'), GESTALT_SLICE_SPAWN_CAPTURE: capture };
        if (queryMode) observedEnvironment.GESTALT_SLICE_READINESS_CAPTURE = path.join(current, 'observation.json');
        let result, terminalFailure;
        try { result = mode === 'comparator' ? await queryComparator(entry, observedEnvironment, path.join(fixture, 'workspace'), current, helper, primary, configFile, path.join(fixture, 'Codex/hooks.json'), inherited, pathToFileURL(path.join(consumer, 'spawn-capture.mjs')).href) :
          mode === 'readiness' ? await queryReadiness(entry, observedEnvironment, path.join(fixture, 'workspace'), current, helper, primary, configFile, path.join(fixture, 'Codex/hooks.json'), inherited) :
          await terminal(entry, observedEnvironment, path.join(fixture, 'workspace'), report, current, helper, control); }
        catch (error) { terminalFailure = error; throw error; }
        finally {
          try { if (mode !== 'comparator') await record(path.join(current, 'effective-manager-hooks.json'), await composedHooks(artifact.binary, launchEnvironment, path.join(fixture, 'workspace'), capture, mode, current)); }
          catch (error) { await record(path.join(current, 'effective-manager-hooks-failure.json'), { message: error.message, stack: error.stack }); if (!terminalFailure) throw error; }
        }
        if (mode === 'comparator') {
          const provider = assertMetadataProbesOnly(requests, control); assert.equal(requests.length, 0, 'Observer-free comparator requires zero provider requests');
          await record(path.join(current, 'result.json'), { ...result, provider, tests: 1, pass: true, failed: 0, skip: 0,
            zeroProviderRequests: true, actualInstalledBindingsVerified: readinessInstalledBindings.length, readinessPass: null });
          continue;
        }
        if (mode === 'readiness') {
          const provider = assertMetadataProbesOnly(requests, control);
          await record(path.join(current, 'result.json'), { ...result, tests: 1, pass: true, failed: 0, skip: 0, provider,
            zeroGenerationOrOtherRequests: true, actualInstalledOrgAndWholeIpcClaimed: false, actualInstalledBindingsVerified: readinessInstalledBindings.length, submittedTurns: 0, terminalCasesExecuted: 0 });
          continue;
        }
        const authority = await json(report); assert.equal(authority.pass, true, JSON.stringify(authority.error)); assert.equal(authority.installedBindings.length, 15);
        if (process.platform !== 'win32') assert.ok(host.measurements.some(value => value.exit === 0 &&
          value.peer.pid === authority.identity.pid && value.peer.started === authority.identity.started &&
          value.peer.executable === authority.identity.executable && value.peer.uid === process.getuid()), 'Host did not observe the actual installed-hook client identity');
        else assert.equal(authority.identity.observerSid, host.host.sid, 'Host and installed hook must use the same ordinary principal');
        const provider = assertMetadataProbesOnly(requests, control);
        await record(path.join(current, 'result.json'), { pass: true, terminal: result, actualInstalledOrgAndWholeIpc: true, zeroGenerationOrOtherRequests: true, provider, requests,
          modelPolicy: 'Explicit isolated test provider only; no production admission/default change', claims: process.platform === 'win32' ? 'Windows matching Job release' : 'Existing Unix compatibility, no new complete-tree claim' });
      } finally { await host.stop(); }
    }
  } catch (error) { runFailure = error; throw error; }
  finally {
    sentinel.closeAllConnections();
    await new Promise(resolve => sentinel.close(resolve));
    const cases = [];
    for (const control of selectedCases) {
      if (queryMode) {
        let query = null; try { query = await json(path.join(evidence, control, 'query-result.json')); } catch { /* unknown if unexecuted/abrupt */ }
        cases.push({ control, mode, attempted: attemptedCases.has(control), submittedTurns: 0, terminalCasesExecuted: 0, query }); continue;
      }
      let report = null, terminalReport = null, readError = null;
      for (const name of ['terminal.json', 'terminal-final.json']) {
        const file = path.join(evidence, control, name);
        try { report = await json(file); terminalReport = file; break; }
        catch (error) { if (error.code !== 'ENOENT') { readError = error.message; terminalReport = file; break; } }
      }
      cases.push(observeTerminalCase(control, report, terminalReport, attemptedCases.has(control), readError));
    }
    let finalPolicyFailure = null;
    try { for (const control of selectedCases) assertMetadataProbesOnly(requests, control); }
    catch (error) { finalPolicyFailure = error; }
    await record(path.join(evidence, 'provider-requests.json'), { requests, cases, policyError: finalPolicyFailure?.message ?? null,
      observationScope: 'Exact local requests, timestamps and complete body byte counts. Only pinned doctor HEAD /responses then GET /models with empty bodies are permitted, at most once each per case. No generation service exists; no POST, query, body, authentication, upgrade or other request is permitted. Metadata alone does not establish a fixture turn.',
      authenticationSelectionPolicy: 'The fixture driver sends no authentication selection; this is policy, not independent observation of Codex UI behavior.' });
    await record(path.join(evidence, 'retained-fixture.json'), { fixture, primary: primary.context, inherited, manifest, consumer: await digest(entry),
      scope: 'Retained isolated fixture; no automatic unknown cleanup and no live installation', observedAt: new Date().toISOString() });
    if (!runFailure && finalPolicyFailure) throw finalPolicyFailure;
  }
  if (npm12Entry) {
    const smoke = path.join(fixture, 'npm 12 smoke ü');
    await storage.createDirectory(smoke);
    const smokeEvidence = path.join(evidence, 'npm12');
    await mkdir(smokeEvidence);
    for (const name of ['home', 'Codex', 'Gestalt']) await storage.createDirectory(path.join(smoke, name));
    const smokeEnvironment = { ...baseEnvironment, HOME: path.join(smoke, 'home'), USERPROFILE: path.join(smoke, 'home'),
      CODEX_HOME: path.join(smoke, 'Codex'), GESTALT_HOME: path.join(smoke, 'Gestalt'),
      CLAUDE_CONFIG_DIR: path.join(smoke, 'home/.claude'), CONTEXT_MODE_DIR: path.join(smoke, 'home/context-mode-data') };
    const installed = await installPackedCore(packed.receipt.tarball, smoke, smokeEnvironment, agents, smokeEvidence, { npm12Entry });
    await record(path.join(smokeEvidence, 'packed-install.json'), installed.receipt);
    assert.deepEqual(installed.receipt.manifest, expectedPackage);
    const orgPlan = await readFile(path.join(smoke, 'Codex/bin/org-plan'), 'utf8');
    assert.match(orgPlan, /bundles\/[a-f0-9-]{36}\/org-plan-cli\.mjs/);
    await record(path.join(smokeEvidence, 'lifecycle.json'), { pass: true, npmVersion: installed.receipt.npmVersion,
      tarballSha256: installed.receipt.sha256, installedOrgDispatcherSha256: sha256(orgPlan), observedAt: new Date().toISOString() });
  }
  if (queryMode) console.log(JSON.stringify({ kind: mode === 'readiness' ? 'actual-compiled-windows-owned-query-readiness' : 'actual-compiled-windows-observer-free-query-comparator', platform: process.platform, arch: process.arch, tests: 1, pass: 1, fail: 0, skip: 0, terminalCasesExecuted: 0, submittedTurns: 0 }));
  else console.log(JSON.stringify({ kind: 'actual-compiled-cli-codex-installed-org-authenticated-ipc', platform: process.platform, arch: process.arch, tests: 2, pass: 2, fail: 0, skip: 0 }));
}

if (process.argv[1] && import.meta.url === pathToFileURL(await realpath(process.argv[1])).href) {
if (args[0] === '--prepare' && args[2] === '--agents' && args.length === 4) await prepare(path.resolve(args[1]), await realpath(args[3]));
else if (args[0] === '--artifacts' && args[2] === '--agents' && args[4] === '--evidence' && args.length === 6) {
  const evidence = path.resolve(args[5]); await mkdir(evidence, { recursive: true });
  const mode = selectedSliceMode(process.env);
  const comparison = selectedReadinessComparison(process.env, mode);
  await record(path.join(evidence, 'mode-entry.json'), { mode, flag: process.env.GESTALT_CORE_SLICE_READINESS_ONLY ?? null,
    comparison, comparisonFlag: process.env.GESTALT_CORE_SLICE_READINESS_COMPARISON ?? null,
    platform: process.platform, pid: process.pid, node: process.version, observedAt: new Date().toISOString(),
    scope: 'Read inside the actual ordinary-account Node entry; CI step environment inheritance is not assumed' });
  let failure;
  try {
    if (!comparison) await consume(path.resolve(args[1]), await realpath(args[3]), evidence, mode);
    else {
      const results = await readinessComparison(control => consume(path.resolve(args[1]), path.resolve(args[3]), path.join(evidence, control), control),
        result => record(path.join(evidence, result.control + '-execution.json'), result));
      let actualQueryInvocations = 0;
      for (const control of ['comparator', 'readiness']) {
        try { const query = await json(path.join(evidence, control, control, 'query-result.json')); if (query.outerLease) actualQueryInvocations++; }
        catch { /* Missing/abrupt capture is not an observed query. */ }
      }
      await record(path.join(evidence, 'comparison.json'), { ...results, actualQueryInvocations,
        readinessPass: results.cases.find(result => result.control === 'readiness')?.completed === true,
        scope: 'Fixed observer-free first / instrumented second ordering, separate fresh fixtures and contexts. OS/cache/order differ; no inner phase claim from comparator.' });
      assert.equal(actualQueryInvocations, 2, 'Both actual query invocations must be captured');
      assert.equal(results.pass, true, 'Comparator measurement and unchanged observed readiness must independently succeed');
    }
  }
  catch (error) { failure = { message: error.message, stack: error.stack }; throw error; }
  finally { await record(path.join(evidence, 'execution.json'), { command: [process.execPath, ...process.argv.slice(1)], cwd: process.cwd(), failure: failure ?? null, observedAt: new Date().toISOString() }); }
} else throw Error('usage: native-core-slice.mjs --prepare OUTPUT --agents ACCEPTED_CHECKOUT | --artifacts OUTPUT --agents ACCEPTED_CHECKOUT --evidence OUTPUT');
}
