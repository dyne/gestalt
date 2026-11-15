import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { randomUUID } from 'node:crypto';
import { chmod, copyFile, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import test from 'node:test';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { NativeOwnedProcesses, recoveredOwnershipClaim } from '../src/cli/platform/processes.ts';

const root = path.resolve(import.meta.dirname, '..');
const tree = path.join(root, 'tests/fixtures/native-owned-tree.mjs');
const controller = path.join(root, 'tests/fixtures/native-owned-controller.mjs');
const supported = ['linux', 'win32'].includes(process.platform);
const native = { skip: supported ? false : 'Complete-tree assertions apply to Windows/Linux; macOS has separate best-effort contracts', timeout: 60000 };
const allNative = { timeout: 60000 };
const macNative = { skip: process.platform !== 'darwin', timeout: 60000 };
const artifact = await mkdtemp(path.join(tmpdir(), 'gestalt-ownership-build-'));
let helper;
test.before(async t => {
  if (process.platform === 'linux') {
    const runtime = { kind: 'native-ownership-namespace-prerequisites', uid: process.getuid(), gid: process.getgid(), groups: process.getgroups() };
    for (const [key, file] of Object.entries({ uidMap: '/proc/self/uid_map', gidMap: '/proc/self/gid_map', setgroups: '/proc/self/setgroups', policy: '/proc/self/attr/current', apparmorUserns: '/proc/sys/kernel/apparmor_restrict_unprivileged_userns' })) {
      try { runtime[key] = (await readFile(file, 'utf8')).trim(); } catch (error) { runtime[key] = `unavailable:${error.code}`; }
    }
    runtime.capabilities = (await readFile('/proc/self/status', 'utf8')).split('\n').filter(line => /^(Cap(Eff|Prm|Bnd)|NoNewPrivs|Seccomp):/.test(line));
    t.diagnostic(JSON.stringify(runtime));
  }
  helper = await buildOwnedProcess(root, artifact);
  if (helper && process.env.GESTALT_NATIVE_TEST_EVIDENCE) {
    const evidence = path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE);
    await mkdir(evidence, { recursive: true });
    await copyFile(`${helper}.provenance.json`, path.join(evidence, 'owned-process-tested.provenance.json'));
    await copyFile(helper, path.join(evidence, path.basename(helper)));
  }
});
test.after(() => rm(artifact, { recursive: true, force: true }));
const input = () => (async function* () {})();
const emptyStreams = () => ({ stdin: input(), stdout: async () => {}, stderr: async () => {} });
function bounded(promise, label, milliseconds = 20000) {
  let timer;
  return Promise.race([promise, new Promise((_resolve, reject) => {
    timer = setTimeout(() => reject(new Error(`${label}: fixture did not settle`)), milliseconds);
  })]).finally(() => clearTimeout(timer));
}
function collected() {
  const events = [], out = [], err = []; let partial = '', ready;
  const reached = new Promise(resolve => { ready = resolve; });
  return { events, out, err, reached, streams: { stdin: input(), stdout: async bytes => {
    out.push(Buffer.from(bytes)); partial += Buffer.from(bytes).toString();
    for (;;) {
      const newline = partial.indexOf('\n'); if (newline < 0) break;
      const line = partial.slice(0, newline); partial = partial.slice(newline + 1);
      const event = JSON.parse(line); events.push(event); if (event.kind === 'tree-ready') ready();
    }
  }, stderr: async bytes => { err.push(Buffer.from(bytes)); } } };
}
function readyOrFailure(lease, reached, label) {
  return bounded(Promise.race([reached, lease.completed.then(claim => {
    throw new Error(`${label} precondition not reached: ${JSON.stringify(claim)}`);
  })]), label);
}
async function launchTree(t, behavior = 'resistant', mode = 'root', options = {}, nested = false) {
  const token = randomUUID(), data = collected();
  const lease = await new NativeOwnedProcesses(helper).launch(process.execPath, {
    argv: [tree, mode, behavior, token], cwd: root, environment: { ...process.env, GESTALT_OWNED_FIXTURE: token, ...(nested ? { GESTALT_NESTED_HELPER: helper } : {}) },
  }, data.streams, options);
  t.after(async () => { await bounded(lease.stop(), 'owned cleanup'); });
  await readyOrFailure(lease, data.reached, 'tree readiness');
  return { token, data, lease };
}
async function linuxMembers(token) {
  const ownDepth = (await readFile('/proc/self/status', 'utf8')).match(/^NSpid:\s*(.+)$/m)[1].trim().split(/\s+/).length;
  const result = [];
  for (const pid of (await readdir('/proc')).filter(name => /^\d+$/.test(name))) {
    try {
      const environment = await readFile(`/proc/${pid}/environ`);
      if (!environment.toString().split('\0').includes(`GESTALT_OWNED_FIXTURE=${token}`)) continue;
      const status = await readFile(`/proc/${pid}/status`, 'utf8');
      if (status.match(/^NSpid:\s*(.+)$/m)[1].trim().split(/\s+/).length <= ownDepth) continue;
      const stat = await readFile(`/proc/${pid}/stat`, 'utf8');
      result.push({ pid, started: stat.slice(stat.lastIndexOf(')') + 2).split(' ')[19] });
    } catch (error) { if (!['ENOENT', 'ESRCH', 'EACCES'].includes(error.code)) throw error; }
  }
  return result;
}
async function observer(t, token, events) {
  for (const kind of ['root', 'worker', 'leaf']) {
    const role = events.filter(event => event.kind === kind);
    assert.equal(role.length, 1, `Actual held ${kind} identity required: ${JSON.stringify(events)}`);
    assert.ok(Number.isSafeInteger(role[0].pid) && role[0].pid > 0);
  }
  const pids = [...new Set(events.filter(event => ['root', 'worker', 'leaf'].includes(event.kind)).map(event => event.pid))];
  if (process.platform === 'linux') {
    const members = await linuxMembers(token);
    assert.ok(members.length >= 4, `Expected actual init/root/worker/detached leaf; got ${JSON.stringify(members)}`);
    return async () => {
      const end = Date.now() + 15000;
      for (;;) {
        const live = [];
        for (const expected of members) {
          try {
            const stat = await readFile(`/proc/${expected.pid}/stat`, 'utf8');
            if (stat.slice(stat.lastIndexOf(')') + 2).split(' ')[19] === expected.started) live.push(expected);
          } catch (error) { if (!['ENOENT', 'ESRCH'].includes(error.code)) throw error; }
        }
        if (!live.length) return;
        assert.ok(Date.now() < end, `Fixture process identities remain: ${JSON.stringify(live)}`);
        await delay(20);
      }
    };
  }
  assert.equal(pids.length, 3, `Distinct held Windows process identities required: ${JSON.stringify(events)}`);
  // Keep read-only handles to the actual held fixtures across stop. These
  // observations cannot signal a PID or confuse a later reused PID with it.
  const script = `$ErrorActionPreference='Stop';$ids=ConvertFrom-Json $env:GESTALT_FIXTURE_IDS;$held=@();foreach($id in $ids){$p=[Diagnostics.Process]::GetProcessById($id);[void]$p.Handle;$held+=,$p};[Console]::Out.WriteLine('held');[Console]::In.ReadLine()|Out-Null;foreach($p in $held){if(-not $p.WaitForExit(15000)){throw 'Fixture identity remains alive'}};[Console]::Out.WriteLine('absent')`;
  const child = spawn(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'), ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], { shell: false, stdio: ['pipe', 'pipe', 'pipe'], env: { ...process.env, GESTALT_FIXTURE_IDS: JSON.stringify(pids) } });
  const exit = once(child, 'close'); let text = '', error = ''; let held;
  const ready = new Promise(resolve => { held = resolve; });
  child.stdout.on('data', bytes => { text += bytes; if (text.includes('held')) held(); });
  child.stderr.on('data', bytes => { error += bytes; });
  t.after(async () => { if (child.exitCode === null) child.kill('SIGKILL'); await bounded(exit, 'observer cleanup'); });
  await bounded(Promise.race([ready, exit.then(() => { throw new Error(`Observer exited before capture: ${error}`); })]), 'observer capture');
  return async () => { child.stdin.end('\n'); const [code] = await bounded(exit, 'fixture identity absence'); assert.equal(code, 0, error); assert.match(text, /absent/); };
}
async function sentinel(t) {
  const child = spawn(process.execPath, ['-e', 'console.log("ready");setInterval(()=>{},1000)'], { stdio: ['ignore', 'pipe', 'pipe'] });
  const exit = once(child, 'close'); await bounded(once(child.stdout, 'data'), 'sentinel readiness');
  t.after(async () => { if (child.exitCode === null) child.kill('SIGKILL'); await bounded(exit, 'sentinel cleanup'); });
  return () => { assert.equal(child.exitCode, null); assert.equal(child.signalCode, null); assert.equal(process.kill(child.pid, 0), true); };
}

test('owned native execution preserves argv, binary streams, environment, cwd and nonzero result', allNative, async () => {
  const out = [], err = [], argv = ['', 'space é 東京', '&echo no', '|', '%PATH%', '!PATH!', '"quoted"', 'trail\\', '$()', '\r\n'];
  const lease = await new NativeOwnedProcesses(helper).launch(process.execPath, { argv: [tree, 'echo', ...argv], cwd: root, environment: { ...process.env, RETAINED: 'kept' } }, {
    stdin: (async function* () { yield Buffer.from([0, 255, 13, 10]); })(), stdout: async b => { out.push(Buffer.from(b)); }, stderr: async b => { err.push(Buffer.from(b)); },
  });
  assert.deepEqual(await bounded(lease.result, 'binary command outcome'), { kind: 'exit', code: 23 });
  const result = await bounded(lease.completed, 'binary worker');
  if (process.platform === 'darwin') assert.equal(result.kind, 'unknown', JSON.stringify(result));
  else { assert.equal(result.kind, 'released', JSON.stringify(result)); assert.deepEqual(result.result, { kind: 'exit', code: 23 }); }
  assert.deepEqual(JSON.parse(Buffer.concat(out)), { argv, input: '00ff0d0a', cwd: root, retained: 'kept' }); assert.deepEqual(Buffer.concat(err), Buffer.from([255, 0, 13]));
});
test('owned native execution resolves the captured npm shim without evaluating batch arguments', allNative, async t => {
  const directory = await mkdtemp(path.join(tmpdir(), 'owned shim é ')); t.after(() => rm(directory, { recursive: true, force: true }));
  const captured = JSON.parse(await readFile(path.join(root, 'tests/fixtures/native-npm-node-shim.json'), 'utf8'));
  await mkdir(path.join(directory, 'bin')); await mkdir(path.join(directory, 'native-forward-fixture'));
  const executable = path.join(directory, 'bin/fixture.cmd'); await writeFile(executable, Buffer.from(captured.base64, 'base64'));
  await writeFile(path.join(directory, 'native-forward-fixture/entry.mjs'), '#!/usr/bin/env node\nconsole.log(JSON.stringify(process.argv.slice(2)));process.exitCode=29;\n');
  const argv = ['', 'space é 東京', '&echo unsafe', '%PATH%', '!PATH!', '"', 'tail\\']; const out = [];
  const lease = await new NativeOwnedProcesses(helper).launch(executable, { argv, cwd: directory, environment: process.env }, { ...emptyStreams(), stdout: async bytes => { out.push(Buffer.from(bytes)); } });
  assert.deepEqual(await bounded(lease.result, 'npm shim command outcome'), { kind: 'exit', code: 29 });
  const claim = await bounded(lease.completed, 'owned npm shim');
  if (process.platform === 'darwin') assert.equal(claim.kind, 'unknown', JSON.stringify(claim));
  else { assert.equal(claim.kind, 'released', JSON.stringify(claim)); assert.deepEqual(claim.result, { kind: 'exit', code: 29 }); }
  assert.deepEqual(JSON.parse(Buffer.concat(out)), argv);
});
test('resistant detached grandchild is contained, concurrent stops share one operation, sentinel is untouched', native, async t => {
  const untouched = await sentinel(t); const { token, data, lease } = await launchTree(t);
  assert.equal(lease.claim().kind, 'active'); const absent = await observer(t, token, data.events);
  const a = lease.stop(), b = lease.stop(); assert.equal(a, b); const result = await bounded(a, 'resistant tree stop');
  assert.equal(result.kind, 'released'); assert.equal(result.lease, lease.id); await absent(); untouched();
});
test('graceful tree stop preserves the root exit result before escalation', native, async t => {
  const { token, data, lease } = await launchTree(t, 'graceful', 'root', { graceMs: 3000, deadlineMs: 15000 });
  const absent = await observer(t, token, data.events); const result = await bounded(lease.stop(), 'graceful tree');
  assert.equal(result.kind, 'released'); assert.deepEqual(result.result, { kind: 'exit', code: 17 }); await absent();
});
test('root exit cannot release a live detached descendant; native ownership empties the complete tree', native, async t => {
  let release; const gate = new Promise(resolve => { release = resolve; }); const data = collected(), token = randomUUID();
  data.streams.stdin = (async function* () { await gate; yield Buffer.from('exit-parent'); })();
  const lease = await new NativeOwnedProcesses(helper).launch(process.execPath, { argv: [tree, 'exit-parent', 'resistant', token], cwd: root, environment: { ...process.env, GESTALT_OWNED_FIXTURE: token } }, data.streams);
  t.after(async () => { release(); await bounded(lease.stop(), 'root exit cleanup'); });
  try {
    await readyOrFailure(lease, data.reached, 'held parent/grandchild readiness'); const absent = await observer(t, token, data.events); release();
    const result = await bounded(lease.completed, 'root exit tree cleanup'); assert.equal(result.kind, 'released'); assert.deepEqual(result.result, { kind: 'exit', code: 41 }); await absent();
  } finally { release(); }
});
test('nested kernel ownership remains inside the outer ownership boundary', native, async t => {
  const { lease, data, token } = await launchTree(t, 'resistant', 'root', {}, true);
  const absent = await observer(t, token, data.events); assert.equal((await bounded(lease.stop(), 'nested ownership stop')).kind, 'released'); await absent();
});
test('launch failure before readiness proves the never-running ownership object empty', native, async () => {
  const lease = await new NativeOwnedProcesses(helper).launch(path.join(root, 'definitely-absent-owned-executable'), { argv: [], cwd: root, environment: process.env }, emptyStreams());
  const result = await bounded(lease.completed, 'failed launch'); assert.equal(result.kind, 'released', JSON.stringify(result)); assert.equal(result.result.kind, 'spawn-error');
});
test('stale lease recovery stays unknown and cannot authorize a signal against a replacement owner', native, async t => {
  const first = await launchTree(t); assert.equal((await first.lease.stop()).kind, 'released');
  const replacement = await launchTree(t); assert.notEqual(first.lease.id, replacement.lease.id);
  assert.equal(recoveredOwnershipClaim(first.lease.id).kind, 'unknown'); await first.lease.stop();
  assert.equal(replacement.lease.claim().kind, 'active'); assert.equal((await replacement.lease.stop()).kind, 'released');
});
test('abrupt controller death closes the ownership lifetime channel and contains resistant descendants', native, async t => {
  const untouched = await sentinel(t), token = randomUUID(), data = collected();
  const child = spawn(process.execPath, [controller, helper, tree, token], { stdio: ['ignore', 'pipe', 'pipe', 'ipc'], env: process.env });
  const exit = once(child, 'close'); child.stdout.on('data', bytes => { void data.streams.stdout(bytes); }); child.stderr.on('data', bytes => { data.err.push(bytes); });
  t.after(async () => { if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL'); await bounded(exit, 'controller cleanup'); });
  await bounded(Promise.race([data.reached, exit.then(() => { throw new Error(`Controller exited early: ${Buffer.concat(data.err)}`); })]), 'controller tree readiness');
  const absent = await observer(t, token, data.events); child.kill('SIGKILL'); await bounded(exit, 'abrupt controller exit'); await absent(); untouched();
});
test('controller crash during launch cannot leave a suspended or unattached managed process', native, async t => {
  const token = randomUUID(); const child = spawn(process.execPath, [controller, helper, tree, token, 'immediate-crash'], { stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  const exit = once(child, 'close'); child.stdout.resume(); child.stderr.resume(); await bounded(exit, 'immediate controller crash');
  if (process.platform === 'linux') {
    const end = Date.now() + 15000;
    while ((await linuxMembers(token)).length) { assert.ok(Date.now() < end, 'Namespace member survived launch-time controller crash'); await delay(20); }
  } else {
    const script = `$ErrorActionPreference='Stop';$end=[DateTime]::UtcNow.AddSeconds(20);do{$live=@(Get-CimInstance Win32_Process -Filter "Name = 'node.exe' OR Name = 'gestalt-owned-process.exe'" | Where-Object {$_.CommandLine -and $_.CommandLine.Contains($env:GESTALT_FIXTURE_TOKEN)});if($live.Count -eq 0){exit 0};Start-Sleep -Milliseconds 25}while([DateTime]::UtcNow -lt $end);throw 'Launch-time fixture or native job controller survived'`;
    const observed = spawn(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'), ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], { shell: false, env: { ...process.env, GESTALT_FIXTURE_TOKEN: token }, stdio: ['ignore', 'pipe', 'pipe'] });
    let diagnostic = ''; observed.stdout.resume(); observed.stderr.on('data', bytes => { diagnostic += bytes; });
    const exitObserved = once(observed, 'close'); t.after(async () => { if (observed.exitCode === null) observed.kill('SIGKILL'); await bounded(exitObserved, 'launch observer cleanup'); });
    assert.equal((await bounded(exitObserved, 'launch-time fixture absence', 30000))[0], 0, diagnostic);
  }
  t.diagnostic('Atomic namespace/job attachment precedes target execution; actual controller was SIGKILLed during launch.');
});
test('a held output-consumer boundary makes cleanup deadline unknown and retains the lease after release', native, async t => {
  let release, held; const gate = new Promise(resolve => { release = resolve; }), reached = new Promise(resolve => { held = resolve; });
  const data = collected(), token = randomUUID(); const original = data.streams.stdout;
  data.streams.stdout = async bytes => { await original(bytes); if (data.events.some(event => event.kind === 'tree-ready')) { held(); await gate; } };
  const lease = await new NativeOwnedProcesses(helper).launch(process.execPath, { argv: [tree, 'root', 'resistant'], cwd: root, environment: { ...process.env, GESTALT_OWNED_FIXTURE: token } }, data.streams, { graceMs: 0, deadlineMs: 150 });
  t.after(async () => { release(); await bounded(lease.stop(), 'held fixture cleanup'); });
  try {
    await readyOrFailure(lease, reached, 'held output precondition'); const absent = await observer(t, token, data.events);
    const claim = await bounded(lease.stop(), 'deadline result'); assert.equal(claim.kind, 'unknown'); assert.match(claim.reason, /deadline/);
    release(); await bounded(lease.completed, 'released output/closed native controller'); await absent(); assert.equal(lease.claim().kind, 'unknown');
  } finally { release(); }
});
test('stream-consumer failure loses verification and never turns into released after helper cleanup', native, async t => {
  const lease = await new NativeOwnedProcesses(helper).launch(process.execPath, { argv: [tree, 'echo'], cwd: root, environment: process.env }, { stdin: input(), stdout: async () => { throw new Error('fixture output consumer failure'); }, stderr: async () => {} });
  t.after(() => bounded(lease.stop(), 'failed-consumer cleanup'));
  const result = await bounded(lease.completed, 'failed stream verification'); assert.equal(result.kind, 'unknown'); assert.match(result.reason, /consumer failure/);
});

async function macControl(t) {
  const directory = await mkdtemp(path.join(tmpdir(), 'gm-'));
  const endpoint = path.join(directory, 's'), token = randomUUID(), clients = new Map();
  assert.ok(Buffer.byteLength(endpoint) <= 100, 'Fixture socket path exceeds native Unix limit');
  let reached, fail, closing;
  const ready = new Promise((resolve, reject) => { reached = resolve; fail = reject; });
  const server = createServer(socket => {
    if (closing) { socket.end('release\n'); return; }
    let text = ''; let closed;
    const settled = new Promise(resolve => { closed = resolve; }); socket.once('close', closed);
    socket.on('error', () => {});
    socket.on('data', bytes => {
      text += bytes; if (!text.includes('\n')) return;
      const message = JSON.parse(text.slice(0, text.indexOf('\n'))); text = '';
      if (message.token !== token || !['root', 'leaf'].includes(message.role) || clients.has(message.role)) {
        fail(new Error('Invalid fixture readiness identity')); socket.destroy(); return;
      }
      clients.set(message.role, { socket, closed: settled, pid: message.pid });
      if (clients.size === 2) reached();
    });
  });
  server.listen(endpoint); await bounded(once(server, 'listening'), 'Mac control listener'); await chmod(endpoint, 0o600);
  const scratch = path.join(directory, 'retained-scratch'); await writeFile(scratch, 'keep');
  const close = () => {
    closing ??= (async () => {
      for (const client of clients.values()) if (!client.socket.destroyed) client.socket.end('release\n');
      await bounded(Promise.all([...clients.values()].map(client => client.closed)), 'controlled Mac fixture release');
      await bounded(new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())), 'Mac control server close');
      await rm(directory, { recursive: true, force: true });
    })();
    return closing;
  };
  t.after(close);
  return { ready, clients, token, endpoint, scratch, close,
    environment: { ...process.env, GESTALT_MAC_FIXTURE_SOCKET: endpoint } };
}

test('macOS bounded stop observes its live group cleanup, preserves command result and keeps scratch unknown', macNative, async t => {
  const control = await macControl(t), untouched = await sentinel(t); let lease;
  try {
    lease = await new NativeOwnedProcesses(helper).launch(process.execPath, {
      argv: [path.join(root, 'tests/fixtures/native-macos-group.mjs'), 'root', 'graceful', control.token], cwd: root, environment: control.environment,
    }, emptyStreams());
    await readyOrFailure(lease, control.ready, 'actual Mac root/leaf readiness');
    assert.equal(control.clients.get('leaf').socket.destroyed, false);
    const first = lease.stop(); assert.equal(first, lease.stop());
    assert.equal((await bounded(first, 'bounded Mac group stop')).kind, 'unknown');
    assert.deepEqual(await bounded(lease.result, 'independent Mac root outcome'), { kind: 'exit', code: 17 });
    await bounded(Promise.all([...control.clients.values()].map(client => client.closed)), 'observed Mac group fixture exits');
    assert.equal(await readFile(control.scratch, 'utf8'), 'keep'); untouched();
  } finally { if (lease) await bounded(lease.stop(), 'Mac stop finalizer'); await control.close(); }
});

test('macOS controller EOF attempts bounded live-group cleanup without claiming complete absence', macNative, async t => {
  const control = await macControl(t); let child, closed;
  try {
    child = spawn(process.execPath, [controller, helper, path.join(root, 'tests/fixtures/native-macos-group.mjs'), control.token], {
      env: control.environment, stdio: ['ignore', 'pipe', 'pipe', 'ipc'],
    });
    closed = once(child, 'close'); child.stdout.resume(); let error = ''; child.stderr.on('data', bytes => { error += bytes; });
    let recorded;
    const leaseReady = new Promise(resolve => { child.on('message', message => { if (message.kind === 'lease-ready') { recorded = message.claim; resolve(); } }); });
    await bounded(Promise.race([Promise.all([control.ready, leaseReady]), closed.then(() => { throw new Error(`Mac controller exited before fixture readiness: ${error}`); })]), 'Mac EOF fixture readiness');
    assert.equal(recorded.kind, 'active');
    child.kill('SIGKILL'); await bounded(closed, 'abrupt Mac controller exit');
    await bounded(Promise.all([...control.clients.values()].map(client => client.closed)), 'observed EOF group cleanup');
    assert.equal(recoveredOwnershipClaim(recorded.lease).kind, 'unknown'); assert.equal(await readFile(control.scratch, 'utf8'), 'keep');
  } finally {
    if (child && child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    if (closed) await bounded(closed, 'EOF controller finalizer'); await control.close();
  }
});

test('macOS detached survivor remains unknown with retained scratch and controlled independent teardown', macNative, async t => {
  const control = await macControl(t); let lease;
  try {
    lease = await new NativeOwnedProcesses(helper).launch(process.execPath, {
      argv: [path.join(root, 'tests/fixtures/native-macos-group.mjs'), 'root', 'graceful', control.token], cwd: root,
      environment: { ...control.environment, GESTALT_MAC_DETACH: '1' },
    }, emptyStreams());
    await readyOrFailure(lease, control.ready, 'detached Mac leaf live precondition');
    assert.equal(control.clients.get('leaf').socket.destroyed, false);
    assert.equal((await bounded(lease.stop(), 'detached Mac stop')).kind, 'unknown');
    assert.deepEqual(await bounded(lease.result, 'detached-root command outcome'), { kind: 'exit', code: 17 });
    await bounded(control.clients.get('root').closed, 'owned Mac root exit');
    assert.equal(control.clients.get('leaf').socket.destroyed, false, 'Actual detached fixture must survive group cleanup');
    assert.equal(lease.claim().kind, 'unknown'); assert.equal((await lease.stop()).kind, 'unknown');
    assert.equal(await readFile(control.scratch, 'utf8'), 'keep');
  } finally { if (lease) await bounded(lease.stop(), 'detached Mac finalizer'); await control.close(); }
});
