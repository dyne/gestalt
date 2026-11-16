// Explicit hosted Linux diagnostic, never imported by the production CLI.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { lstat, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { NativeOwnedProcesses } from '../src/cli/platform/processes.ts';
import { bounded, command, digest, isolatedEnvironment, pinnedCodex, exerciseCodexTerminal } from './fixtures/native-ptrace/codex.mjs';

assert.equal(process.platform, 'linux', 'Host-namespace ptrace diagnostic requires actual Linux');
const root = path.resolve(import.meta.dirname, '..');
const argument = name => { const index = process.argv.indexOf(name); assert.ok(index > 0 && process.argv[index + 1], `Missing ${name}`); return path.resolve(process.argv[index + 1]); };
const agents = argument('--agents'), evidence = argument('--evidence');
const pin = 'e98a4cd15f945a7749d02e071126a854376b93c0';
await mkdir(evidence, { recursive: true, mode: 0o700 });
const report = { productionBackendSelected: false, testOnly: true, source: root, agents, pin,
  command: process.argv.slice(1), startedAt: new Date().toISOString(), phases: [], failures: [],
  scratchContext: Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].map(k => [k, process.env[k] ?? null])) };
const record = async (name, operation) => {
  const phase = { name, startedAt: new Date().toISOString() }; report.phases.push(phase);
  try { phase.result = await operation(); phase.status = 'passed'; return phase.result; }
  catch (error) { phase.status = 'failed'; phase.failure = { message: error.message, stack: error.stack }; report.failures.push(name); return null; }
  finally { phase.observedAt = new Date().toISOString(); await writeFile(path.join(evidence, 'diagnostic.json'), JSON.stringify(report, null, 2) + '\n'); }
};
let server, serverExit, fixture, lease, allAuthorityPassed = false;
try {
  const verifiedAgents = await record('accepted-agents-source', async () => {
    const binding = JSON.parse(await readFile(path.join(root, 'tests/fixtures/native-ptrace/agents-source.json'), 'utf8'));
    assert.equal(binding.acceptedCommit, pin);
    const identity = await command('git', ['-C', agents, 'rev-parse', 'HEAD']); assert.equal(identity.exit, 0);
    assert.ok([pin, binding.remoteCheckoutCommit].includes(identity.stdout.trim()), 'Unexpected Agents checkout identity');
    const status = await command('git', ['-C', agents, 'status', '--porcelain=v1']); assert.equal(status.exit, 0); assert.equal(status.stdout, '');
    const files = [];
    for (const expected of binding.files) {
      const file = path.join(agents, expected.path), actual = await digest(file), info = await lstat(file);
      assert.ok(info.isFile() && !info.isSymbolicLink()); assert.equal(actual.sha256, expected.sha256); assert.equal(actual.bytes, expected.bytes); assert.equal(actual.mode, expected.physicalMode);
      const tree = await command('git', ['-C', agents, 'ls-tree', 'HEAD', '--', expected.path]); assert.equal(tree.exit, 0);
      assert.equal(tree.stdout.trim(), `${expected.gitMode} blob ${expected.blobOid}\t${expected.path}`); files.push(actual);
    }
    assert.equal(files.length, 15);
    return { identity, status, acceptedSourceCommit: pin, binding: await digest(path.join(root, 'tests/fixtures/native-ptrace/agents-source.json')), equivalenceScope: binding.equivalenceScope, files };
  });
  assert.ok(verifiedAgents, 'Exact accepted read-only Agents checkout required');
  const lifecycle = await record('actual-kernel-lifecycle-escape-prototype', async () => {
    const result = await command('python3', [path.join(root, 'tests/fixtures/native-ptrace/run.py'), path.join(evidence, 'prototype')]);
    const actual = JSON.parse(await readFile(path.join(evidence, 'prototype/prototype-results.json'), 'utf8'));
    assert.equal(result.exit, 0, result.stderr); assert.deepEqual(actual.counts, { passed: 12, failed: 0, total: 12 }); assert.equal(actual.nonOwnedSentinelUntouched, true);
    return { command: result, resultFile: await digest(path.join(evidence, 'prototype/prototype-results.json')) };
  });
  assert.ok(lifecycle, 'Prototype prerequisite/lifecycle experiment failed');
  const helper = await buildOwnedProcess(root, path.join(evidence, 'native')), peer = await buildNativePlatform(root, path.join(evidence, 'native'));
  report.native = { helper: await digest(helper), helperProvenance: JSON.parse(await readFile(`${helper}.provenance.json`, 'utf8')), peer: await digest(peer), peerProvenance: JSON.parse(await readFile(`${peer}.provenance.json`, 'utf8')) };
  fixture = await mkdtemp(path.join(tmpdir(), 'gpi-')); await mkdir(path.join(fixture, 'status'), { mode: 0o700 });
  // A private test fixture, never a session's canonical plan or status directory.
  await writeFile(path.join(fixture, 'fixture.org'), `#+TITLE: Private inherited runtime diagnostic
#+SUBTITLE: Test fixture only
#+DATE: 2026-11-16
#+KEYWORDS: diagnostic
* TODO [#A] Fixture
:PROPERTIES:
:ID: fixture
:SKILLS: $gestalt:coding
:REVIEW_STATUS: UNREVIEWED
:END:
- Effort :: Small.
- Goal :: Exercise actual publication in an isolated fixture.
- Notes :: Never a running session plan.
** TODO [#A] Child
:PROPERTIES:
:ID: child
:END:
- Why :: Test inherited authority.
- Change :: Private fixture only.
- Tests :: Actual unchanged publication and IPC.
- Done when :: Diagnostic completes.
`, { mode: 0o600 });
  report.fixture = { path: fixture, canonical: await realpath(fixture), isolated: true, ancestry: [] };
  for (let current = await realpath(fixture);; current = path.dirname(current)) {
    const s = await lstat(current); report.fixture.ancestry.push({ path: current, uid: s.uid, gid: s.gid, mode: (s.mode & 0o7777).toString(8) });
    if (current === path.dirname(current)) break;
  }
  server = spawn(process.execPath, ['-e', `
    const fs=require('node:fs'),net=require('node:net');let applicationBytes=0,connections=0;
    const server=net.createServer(socket=>{connections++;socket.on('data',b=>{applicationBytes+=b.length;socket.write(b)});socket.on('error',()=>{});});
    server.listen('s',()=>{fs.chmodSync('s',0o600);console.log('READY')});
    process.stdin.resume();process.stdin.on('end',()=>server.close(()=>console.log(JSON.stringify({applicationBytes,connections}))));
  `], { cwd: fixture, stdio: ['pipe', 'pipe', 'pipe'] });
  serverExit = once(server, 'close'); let serverOutput = '', serverError = '';
  server.stdout.on('data', b => { serverOutput += b; }); server.stderr.on('data', b => { serverError += b; });
  await bounded(once(server.stdout, 'data'), 'private server readiness');
  const owner = await command(peer, ['--identity', String(server.pid)]); assert.equal(owner.exit, 0, owner.stderr); const expected = JSON.parse(owner.stdout);
  report.hostServer = { expected, identityCommand: owner };
  const args = [path.join(root, 'tests/fixtures/native-ptrace/inherited.mjs'), root, agents, fixture, peer, JSON.stringify(expected)];
  const environment = { ...process.env, GESTALT_MOBILE_ORG_PLAN_STATUS_DIRECTORY: path.join(fixture, 'status'), GESTALT_MOBILE_ORG_PLAN_STATUS_FILE: '' };
  const phases = {};
  for (const mode of ['host', 'legacy-owned', 'prototype-owned']) await record(`whole-authority-${mode}`, async () => {
    let execution;
    if (mode === 'legacy-owned') {
      let stdout = '', stderr = '';
      lease = await new NativeOwnedProcesses(helper).launch(process.execPath, { argv: args, cwd: fixture, environment }, {
        stdin: (async function* () {})(), stdout: async b => { stdout += Buffer.from(b); }, stderr: async b => { stderr += Buffer.from(b); },
      });
      const claim = await bounded(lease.completed, 'legacy actual inherited diagnostic'); execution = { result: await lease.result, claim, stdout, stderr };
      assert.equal(claim.kind, 'released', JSON.stringify(claim)); assert.deepEqual(execution.result, { kind: 'exit', code: 0 });
    } else {
      const executable = mode === 'host' ? process.execPath : path.join(evidence, 'prototype/prototype');
      execution = await command(executable, mode === 'host' ? args : [process.execPath, ...args], { cwd: fixture, env: environment });
      assert.equal(execution.exit, 0, execution.stderr);
      if (mode === 'prototype-owned') assert.match(execution.stderr, /TRACE_DRAINED .*live=0 .*root_status=0/);
    }
    const observed = JSON.parse(execution.stdout); phases[mode] = observed;
    await writeFile(path.join(evidence, `authority-${mode}.json`), JSON.stringify({ execution, observed }, null, 2) + '\n');
    if (mode === 'legacy-owned') {
      assert.equal(observed.guard.ok, false); assert.equal(observed.publication.published, false); assert.equal(observed.ipc.ok, false);
      assert.notEqual(observed.sameSocketPeer.exit, 0); assert.equal(observed.rawCredentials.exit, 0);
      assert.equal(JSON.parse(observed.rawCredentials.stdout).peerPid, 0);
    } else {
      assert.equal(observed.guard.ok, true, JSON.stringify(observed.guard)); assert.equal(observed.publication.published, true, JSON.stringify(observed.publication));
      assert.equal(observed.publishedFile.canonicalPlanMatches, true); assert.equal(observed.ipc.ok, true, JSON.stringify(observed.ipc));
      assert.equal(observed.identityMismatch.refused, true); assert.equal(observed.sameSocketPeer.exit, 0);
      assert.deepEqual(JSON.parse(observed.sameSocketPeer.stdout), expected); assert.equal(observed.expectedProc.executable, expected.executable);
      assert.equal(observed.rawCredentials.exit, 0); const raw = JSON.parse(observed.rawCredentials.stdout);
      assert.equal(raw.peerPid, expected.pid); assert.equal(raw.peerUid, observed.uid);
      assert.equal(observed.uid, process.getuid());
      if (mode === 'prototype-owned') { assert.equal(observed.uid_map, phases.host.uid_map); assert.equal(observed.gid_map, phases.host.gid_map); }
    }
    return { mode, actualUID: observed.uid, actualMap: observed.uid_map, actualGuard: observed.guard, publication: observed.publication, wholeIpc: observed.ipc, identityMismatch: observed.identityMismatch };
  });
  allAuthorityPassed = ['whole-authority-host', 'whole-authority-legacy-owned', 'whole-authority-prototype-owned'].every(name => report.phases.find(p => p.name === name)?.status === 'passed');
  server.stdin.end(); const [serverCode] = await bounded(serverExit, 'private host server settlement');
  const measurements = JSON.parse(serverOutput.trim().split('\n').at(-1)); report.hostServer.final = { serverCode, stderr: serverError, measurements };
  if (allAuthorityPassed) assert.equal(measurements.applicationBytes, 2 * Buffer.byteLength('authenticated private echo é東京'), 'Only successfully authenticated host/prototype clients may send application data');
  assert.equal(serverCode, 0, serverError);
  let artifact;
  await record('pinned-official-codex-artifact', async () => { artifact = await pinnedCodex(path.join(evidence, 'codex-artifact')); return artifact.record; });
  if (artifact) {
    await record('pinned-codex-production-terminal', () => exerciseCodexTerminal({ helper, artifact, evidence: path.join(evidence, 'codex-candidate'), directory: path.join(evidence, 'codex-candidate-work') }));
    await record('pinned-codex-prototype-terminal', async () => {
      const work = path.join(evidence, 'codex-prototype-work'); await mkdir(work, { mode: 0o700 });
      const result = await command('python3', [path.join(root, 'tests/fixtures/native-ptrace/codex-terminal.py'), path.join(evidence, 'prototype/prototype'), artifact.binary, artifact.home, work, path.join(evidence, 'codex-prototype')], { env: isolatedEnvironment(artifact.home) });
      assert.equal(result.exit, 0, result.stderr); const actual = JSON.parse(await readFile(path.join(evidence, 'codex-prototype/result.json'), 'utf8'));
      assert.equal(actual.interactiveSignInChoices, true); return actual;
    });
    await record('pinned-codex-actual-sandbox-helper', async () => {
      const daemon = path.join(await realpath('/tmp'), `codex-daemon-${process.getuid()}`); let before;
      try { const s = await lstat(daemon); before = { uid: s.uid, gid: s.gid, mode: (s.mode & 0o7777).toString(8) }; } catch (error) { before = { error: error.code }; }
      const argv = [artifact.binary, 'sandbox', '--', process.execPath, '-e', `const fs=require('node:fs');console.log(JSON.stringify({kind:'ACTUAL_CODEX_SANDBOX_CHILD',uid:process.getuid(),pid:process.pid,uidMap:fs.readFileSync('/proc/self/uid_map','utf8')}))`];
      const result = await command(path.join(evidence, 'prototype/prototype'), argv, { cwd: fixture, env: isolatedEnvironment(artifact.home) });
      await writeFile(path.join(evidence, 'codex-sandbox.json'), JSON.stringify({ daemon, before, command: result, hostPolicyChanged: false }, null, 2) + '\n');
      assert.equal(result.exit, 0, result.stderr); assert.match(result.stderr, /TRACE_DRAINED .*live=0 .*root_status=0/);
      const observed = JSON.parse(result.stdout); assert.equal(observed.kind, 'ACTUAL_CODEX_SANDBOX_CHILD');
      return { observed, command: result, daemon, before };
    });
  }
} catch (error) { report.failures.push('setup-or-control'); report.setupFailure = { message: error.message, stack: error.stack }; }
finally {
  if (lease) { await lease.stop(); await bounded(lease.completed, 'legacy final cleanup'); }
  if (server && server.exitCode === null && server.signalCode === null) { server.stdin.end(); try { await bounded(serverExit, 'host server final cleanup'); } catch { server.kill('SIGKILL'); await bounded(serverExit, 'host server kill settlement'); } }
  report.fixtureRetained = !(allAuthorityPassed && report.failures.length === 0);
  if (fixture && !report.fixtureRetained) await rm(fixture, { recursive: true, force: true });
  report.observedAt = new Date().toISOString(); report.counts = { passed: report.phases.filter(p => p.status === 'passed').length, failed: report.phases.filter(p => p.status === 'failed').length, total: report.phases.length };
  await writeFile(path.join(evidence, 'diagnostic.json'), JSON.stringify(report, null, 2) + '\n');
}
console.log(JSON.stringify({ counts: report.counts, failures: report.failures, fixtureRetained: report.fixtureRetained }));
assert.equal(report.failures.length, 0, JSON.stringify(report.failures)); assert.equal(report.phases.length, 9, 'All hosted diagnostic gates must actually execute');
