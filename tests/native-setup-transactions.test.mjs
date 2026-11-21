import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, copyFile, mkdir, mkdtemp, readFile, readdir, realpath, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { parse } from 'smol-toml';
import { buildCli } from '../scripts/build-cli.mjs';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { CommandFailure, SetupContext } from '../src/cli/setup/context.ts';
import { restartWorker } from '../src/cli/setup/restart.ts';
import { updateManager } from '../src/cli/setup/manager-update.ts';
import { capturedNodeCommand } from './fixtures/native-node-command.mjs';
import { captureSetupRetention } from './fixtures/setup-retention.mjs';
import { compositionInputs } from './fixtures/setup-composition-inputs.mjs';
import { captureCopyTree } from '../scripts/native-copy-provenance.mjs';
import { verifyOwnershipBuildInputs } from './fixtures/native-runtime/source-binding.mjs';

const repository = path.resolve(import.meta.dirname, '..');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const prebuilt = process.env.GESTALT_NATIVE_SETUP_ARTIFACTS;
const artifacts = prebuilt ? await realpath(prebuilt) : await mkdtemp(path.join(tmpdir(), 'native-setup-build-'));
let manager = path.join(artifacts, 'gestalt.mjs'), ownershipHelper, unixProbe;
if (prebuilt) {
  assert.equal(process.platform, 'win32');
  const manifest = JSON.parse(await readFile(path.join(artifacts, 'artifacts.json'), 'utf8'));
  assert.equal(manifest.platform, process.platform);
  assert.equal(manifest.arch, process.arch);
  assert.equal(manifest.abi, process.versions.modules);
  assert.equal(manifest.helper, path.join('native', 'gestalt-owned-process.exe'));
  ownershipHelper = path.join(artifacts, manifest.helper);
  for (const file of [manager, ownershipHelper, ownershipHelper + '.provenance.json']) {
    const expected = manifest.artifacts.find(entry => entry.relative === path.relative(artifacts, file));
    const bytes = await readFile(file);
    assert.equal(bytes.length, expected?.bytes);
    assert.equal(hash(bytes), expected?.sha256);
  }
  const provenance = JSON.parse(await readFile(ownershipHelper + '.provenance.json', 'utf8'));
  assert.equal(provenance.artifactSha256, hash(await readFile(ownershipHelper)));
  assert.deepEqual(await verifyOwnershipBuildInputs(provenance, repository), manifest.nativeHelperInputs);
} else {
  await buildCli(repository, manager);
  unixProbe = await buildNativePlatform(repository, path.join(artifacts, 'native'));
  ownershipHelper = await buildOwnedProcess(repository, path.join(artifacts, 'native'));
  test.after(() => rm(artifacts, { recursive: true, force: true }));
}
if (process.env.GESTALT_NATIVE_SETUP_EVIDENCE) {
  await mkdir(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, { recursive: true });
  await copyFile(`${ownershipHelper}.provenance.json`, path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, 'transaction-ownership.provenance.json'));
  await copyFile(ownershipHelper, path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, path.basename(ownershipHelper)));
}
const custom = '# keep custom model\nmodel="fixture-custom"\n[mcp_servers.custom]\ncommand="custom"\ndefault_tools_approval_mode="prompt"\n';
// Native Windows ACL setup measured 117–138s even on an early failed attempt;
// readiness remains an observed phase, never a deadline-based success condition.
const phaseBudgetMs = process.platform === 'win32' ? 300_000 : 60_000;
const managedEntrypoint = path.join(repository, 'tests/fixtures/run-managed-setup.mjs');
const compiledBackend = process.platform === 'win32' ? 'compiled native' : 'managed JavaScript';

async function command(bin, name, body) {
  const script = path.join(bin, `${name}.mjs`);
  await writeFile(script, '#!/usr/bin/env node\n' + body, { mode: 0o755 });
  if (process.platform !== 'win32') {
    await writeFile(path.join(bin, name), '#!/usr/bin/env node\n' + body, { mode: 0o755 });
  } else {
    const captured = JSON.parse(await readFile(path.join(repository, 'tests/fixtures/native-npm-node-shim.json'), 'utf8'));
    const shim = capturedNodeCommand(captured, `${name}.mjs`);
    await writeFile(path.join(bin, `${name}.cmd`), shim);
  }
}

async function fixture(t, config = custom, { freshHomes = false, publicUnix = false } = {}) {
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'native setup ü & ')));
  let retained = false;
  t.after(() => retained ? undefined : rm(root, { recursive: true, force: true }));
  const home = path.join(root, 'home ü &'), codex = path.join(home, 'Codex ü &'), gestalt = path.join(home, 'Gestalt ü &');
  await mkdir(home, { mode: 0o700 });
  let storage = null;
  if (!freshHomes) {
    if (publicUnix) {
      await mkdir(codex, { mode: 0o700 }); await mkdir(gestalt, { mode: 0o700 });
    } else {
      const windowsReadHelper = process.platform === 'win32' ? async () => ownershipHelper : undefined;
      storage = await NativePrivateStorage.open(codex, unixProbe, windowsReadHelper);
      await NativePrivateStorage.open(gestalt, unixProbe, windowsReadHelper);
    }
  }
  const bin = path.join(root, 'bin'); await mkdir(bin, { mode: 0o700 });
  const ownerRoot = path.join(root, 'Agents ü &'); await mkdir(ownerRoot, { mode: 0o700 });
  const owner = path.join(ownerRoot, 'gestalt-setup.mjs'), log = path.join(root, 'calls.jsonl');
  const apiOwner = path.join(ownerRoot, 'scripts/setup-agents.mjs'), worker = path.join(ownerRoot, 'setup-worker.mjs');
  await mkdir(path.dirname(apiOwner), { mode: 0o700 });
  await writeFile(owner, "export * from './scripts/setup-agents.mjs';\n");
  if (publicUnix) await writeFile(path.join(ownerRoot, 'gestalt-setup.sh'),
    '#!/usr/bin/env bash\nset -e\nexec node "$(dirname -- "$0")/setup-worker.mjs" "$@"\n', { mode: 0o700 });
  await writeFile(apiOwner, `import {fileURLToPath} from 'node:url';
export const SETUP_API_VERSION=1,SETUP_API_CAPABILITIES=['async-owned-commands-v1','host-filesystem-v1','shared-context-v1'];
export async function setupAgents(args,{mode,commands,environment,cwd,streams,signal}){
if(mode!=='managed'||commands?.apiVersion!==1)throw new Error('explicit managed capability required');
const lease=await commands.launch(process.execPath,{argv:[fileURLToPath(new URL('../setup-worker.mjs',import.meta.url)),...args],cwd,environment},streams,{graceMs:1000,deadlineMs:10000});
const abort=()=>{void lease.stop();};signal.addEventListener('abort',abort,{once:true});if(signal.aborted)abort();
try{const result=await lease.result,claim=await lease.stop(),status=signal.aborted?signal.reason.status:result.kind==='exit'?result.code:1;
const execution={apiVersion:1,mode,ownership:claim.kind==='released'?{kind:'released',completeTree:true}:{kind:'unknown',claims:[claim]},retained:[]};
if(status!==0||(claim.kind!=='released'&&process.platform!=='darwin'))throw Object.assign(new Error('fixture setup failed ('+status+')'),{status:status||1,execution});
return execution;}finally{signal.removeEventListener('abort',abort);}}
`);
  if (!freshHomes) {
    if (config !== null) await writeFile(path.join(codex, 'config.toml'), config, { mode: 0o640 });
    await mkdir(path.join(codex, 'agents'), { mode: 0o700 });
    await writeFile(path.join(codex, 'agents/custom.toml'), 'model="preserve"\n');
    await writeFile(path.join(codex, 'AGENTS.md'), 'user instructions\n');
  }
  await command(bin, 'npm', `import fs from 'node:fs';import path from 'node:path';
const args=process.argv.slice(2),index=args.indexOf('--prefix');
if(process.env.FIXTURE_PUBLIC_UNIX==='1')fs.appendFileSync(process.env.FIXTURE_LOG,JSON.stringify(['npm',...args])+'\\n');
if(process.env.FIXTURE_MOBILE_FAIL==='1')process.exit(41);
if(index>=0){const prefix=args[index+1],bin=path.join(prefix,'node_modules/.bin');fs.mkdirSync(bin,{recursive:true,mode:0o700});fs.mkdirSync(path.join(prefix,'node_modules/gestalt-mobile'),{recursive:true,mode:0o700});fs.writeFileSync(path.join(prefix,'node_modules/gestalt-mobile/package.json'),JSON.stringify({name:'gestalt-mobile',version:'0.0.1'}));fs.writeFileSync(path.join(bin,'gestalt-mobile'),'#!/usr/bin/env node\\n',{mode:0o755});}`);
  await command(bin, 'codex', `import fs from 'node:fs';import path from 'node:path';
const args=process.argv.slice(2);fs.appendFileSync(process.env.FIXTURE_LOG,JSON.stringify(['codex',...args])+'\\n');
if(args[0]==='plugin'&&args[1]==='marketplace'){
  if(process.env.FIXTURE_FRESH_MARKETPLACE==='1'){
    const cache=path.join(process.env.CODEX_HOME,'.tmp/marketplaces/dyne-gestalt-agents');
    if(args[2]==='upgrade'){console.error('marketplace is not configured as a Git marketplace');process.exit(7);}
    if(fs.existsSync(cache)){console.error('marketplace is already added from a different source');process.exit(7);}
    fs.cpSync(process.env.FIXTURE_AGENTS_SOURCE,cache,{recursive:true});
    fs.appendFileSync(path.join(process.env.CODEX_HOME,'config.toml'),'\\n[marketplaces.dyne-gestalt-agents]\\nsource_type="git"\\n');
  }
  if(process.env.FIXTURE_MARKETPLACE_FAIL==='1'){fs.appendFileSync(process.env.FIXTURE_LOG,JSON.stringify(['phase','marketplace-failure',7])+'\\n');process.exit(7);}
  if(process.env.FIXTURE_PUBLIC_UNIX==='1'){
    const cache=path.join(process.env.CODEX_HOME,'.tmp/marketplaces/dyne-gestalt-agents');
    fs.mkdirSync(path.dirname(cache),{recursive:true,mode:0o700});
    if(!fs.existsSync(cache))fs.symlinkSync(process.env.GESTALT_AGENTS_SOURCE,cache);
  }
}
else if(args[0]==='plugin'&&args[1]==='list'){
  if(process.env.FIXTURE_VERIFY_FAIL==='1')fs.appendFileSync(process.env.FIXTURE_LOG,JSON.stringify(['phase','plugin-verification',0])+'\\n');
  console.log(JSON.stringify({installed:[{name:'gestalt',version:'1.2.3',installed:true},{name:'context-mode',version:process.env.FIXTURE_VERIFY_FAIL==='1'?'9.9.9':'1.2.3',installed:true,enabled:false}]}));
}
else if(args[0]==='mcp')console.log(JSON.stringify({enabled:true,command:'node',args:[path.join(process.env.CODEX_HOME,'bin/context-mode-mcp.mjs')]}));
else throw new Error('unexpected fixture Codex invocation');`);
  await writeFile(worker, `import fs from 'node:fs/promises';import path from 'node:path';
await fs.appendFile(process.env.FIXTURE_LOG,JSON.stringify(['setup',...process.argv.slice(2)])+'\\n');
if(process.env.FIXTURE_WAIT==='1'){await fs.writeFile(process.env.FIXTURE_READY,'ready');while(true){try{await fs.stat(process.env.FIXTURE_RELEASE);break;}catch{}await new Promise(r=>setTimeout(r,20));}}
if(process.env.FIXTURE_FAIL==='1'){await fs.appendFile(process.env.FIXTURE_LOG,JSON.stringify(['phase','owner-failure',9])+'\\n');process.exit(9);}
const home=process.env.CODEX_HOME,gestalt=process.env.GESTALT_HOME;
await fs.mkdir(path.join(home,'agents'),{recursive:true,mode:0o700});
await fs.writeFile(path.join(home,'agents/org-plan-executor.toml'),'model="fixture-default"\\n');await fs.writeFile(path.join(home,'agents/org-plan-reviewer.toml'),'model="fixture-default"\\n');
await fs.writeFile(path.join(home,'hooks.json'),'{"fixture":true}\\n',{mode:0o600});
const config=path.join(home,'config.toml');let source=await fs.readFile(config,'utf8');if(!source.includes('[mcp_servers.context-mode]'))source+='\\n[mcp_servers.context-mode]\\ncommand="node"\\nargs=['+JSON.stringify(path.join(home,'bin/context-mode-mcp.mjs'))+']\\ndefault_tools_approval_mode="prompt"\\n';await fs.writeFile(config,source);
const runtime=path.join(gestalt,'runtime/context-mode');await fs.mkdir(runtime,{recursive:true,mode:0o700});
await fs.writeFile(path.join(runtime,'.context-mode-prepared.json'),JSON.stringify({packageVersion:'1.2.3',nodeModulesAbi:process.versions.modules,platform:process.platform,arch:process.arch}));
for(const file of ['cli.bundle.mjs','server.bundle.mjs'])await fs.writeFile(path.join(runtime,file),'// fixture');
if(process.env.FIXTURE_VERIFY_FAIL==='1'&&process.env.FIXTURE_PUBLIC_UNIX!=='1'){
  await fs.appendFile(process.env.FIXTURE_LOG,JSON.stringify(['phase','owner-verification-failure',9])+'\\n');process.exit(9);
}
await fs.appendFile(process.env.FIXTURE_LOG,JSON.stringify(['phase','owner-completed',0])+'\\n');
`);
  const env = { ...process.env, HOME: home, USERPROFILE: home, CODEX_HOME: codex, GESTALT_HOME: gestalt,
    GESTALT_AGENTS_SOURCE: ownerRoot, GESTALT_AGENTS_SETUP_SHA256: hash(await readFile(owner)), GESTALT_MANAGER_DISTRIBUTION: 'npm',
    FIXTURE_LOG: log, FIXTURE_READY: path.join(root, 'ready'), FIXTURE_RELEASE: path.join(root, 'release'),
    FIXTURE_PUBLIC_UNIX: publicUnix ? '1' : '',
    PATH: [bin, path.dirname(process.execPath), process.env.PATH].join(path.delimiter), PATHEXT: '.CMD;.EXE' };
  if (publicUnix) {
    env.GESTALT_MARKETPLACE = 'dyne/gestalt-agents';
    env.GESTALT_MARKETPLACE_NAME = 'dyne-gestalt-agents';
  }
  for (const name of Object.keys(env)) if (name.toUpperCase() === 'PATH' && name !== 'PATH') delete env[name];
  function start(argv, extra = {}) {
    const args = process.platform === 'win32' || publicUnix ? [manager, ...argv]
      : [managedEntrypoint, manager, ownershipHelper, ...(argv[0] === '__updated-manager' ? argv : ['__updated-manager', ...argv])];
    const child = spawn(process.execPath, args, { env: { ...env, ...extra }, cwd: root, stdio: ['ignore', 'pipe', 'pipe'], shell: false });
    let stdout = '', stderr = '';
    child.stdout.on('data', b => stdout += b); child.stderr.on('data', b => stderr += b);
    const result = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', status => resolve({ status, stdout, stderr })); });
    return { child, result };
  }
  return { root, home, codex, gestalt, owner, apiOwner, worker, ownerRoot, env, log, storage, start, retain: () => { retained = true; },
    run: (argv = ['agents', 'install'], extra) => start(argv, extra).result,
    config: () => readFile(path.join(codex, 'config.toml'), 'utf8'),
    provenance: () => readFile(path.join(codex, '.gestalt-setup-provenance.json'), 'utf8'),
    events: async () => (await readFile(log, 'utf8')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) };
}
for (const orphaned of [false, true]) test(`first update registers an unconfigured marketplace ${orphaned ? 'with a stale cache' : 'on a clean profile'}`, async t => {
  const f = await fixture(t);
  const cache = path.join(f.codex, '.tmp/marketplaces/dyne-gestalt-agents');
  const stale = path.join(cache, 'stale');
  const environment = { GESTALT_AGENTS_SOURCE: '', GESTALT_MARKETPLACE: '',
    FIXTURE_AGENTS_SOURCE: f.ownerRoot, FIXTURE_FRESH_MARKETPLACE: '1' };
  if (orphaned) {
    await mkdir(cache, { recursive: true });
    await writeFile(stale, 'previous marketplace');
    const failure = await f.run(['agents', 'update'], { ...environment, FIXTURE_FAIL: '1' });
    assert.equal(failure.status, 9, failure.stderr);
    if (process.platform === 'darwin') return;
    assert.equal(await f.config(), custom);
    assert.equal(await readFile(stale, 'utf8'), 'previous marketplace');
    await writeFile(f.log, '');
  }
  const result = await f.run(['agents', 'update'], environment);
  assert.equal(result.status, 0, result.stderr);
  const marketplaceCalls = (await f.events()).filter(event => event[0] === 'codex' && event[2] === 'marketplace');
  const { acceptedCommit } = JSON.parse(await readFile(new URL('./fixtures/native-runtime/agents-source.json', import.meta.url)));
  const source = process.platform === 'win32' ? `dyne/gestalt-agents@${acceptedCommit}` : 'dyne/gestalt-agents';
  assert.deepEqual(marketplaceCalls, [['codex', 'plugin', 'marketplace', 'add', source]]);
  assert.ok((await f.events()).some(event => event[0] === 'setup'));
  assert.equal(await exists(stale), false);
});

async function until(check, message, timeoutMs = 15_000, operation) {
  const deadline = Date.now() + timeoutMs;
  let completed;
  if (operation) void operation.then(result => { completed = result; }, error => { completed = { status: 'spawn-error', stderr: error.message }; });
  while (Date.now() < deadline) {
    if (await check()) return;
    if (completed) assert.fail(`${message}; command exited before the phase (status ${completed.status}): ${(completed.stderr || '').slice(-2048)}`);
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  assert.fail(message);
}
async function settleFixture(pending, fixture) {
  let timer;
  try {
    return await Promise.race([pending.result, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error('released fixture did not settle within its phase budget')), phaseBudgetMs + 60_000);
    })]);
  } catch (error) {
    fixture.retain();
    if (pending.child.exitCode === null && pending.child.signalCode === null) pending.child.kill('SIGKILL');
    await pending.result.catch(() => {});
    throw error;
  } finally { clearTimeout(timer); }
}
const exists = file => stat(file).then(() => true, error => { if (error.code === 'ENOENT') return false; throw error; });
async function linuxSetupIdentities(token) {
  const members = [];
  for (const pid of (await readdir('/proc')).filter(name => /^\d+$/.test(name))) {
    try {
      if (!(await readFile(`/proc/${pid}/environ`, 'utf8')).split('\0').includes(`GESTALT_SETUP_DESCENDANT=${token}`)) continue;
      const info = await readFile(`/proc/${pid}/stat`, 'utf8');
      members.push({ pid: Number(pid), started: info.slice(info.lastIndexOf(')') + 2).split(' ')[19] });
    } catch (error) { if (!['ENOENT', 'ESRCH', 'EACCES', 'EPERM'].includes(error.code)) throw error; }
  }
  return members;
}
async function windowsSetupObserver(pid) {
  const script = "$ErrorActionPreference='Stop';$p=[Diagnostics.Process]::GetProcessById([int]$env:GESTALT_SETUP_OBSERVE_PID);[void]$p.Handle;[Console]::Out.WriteLine('held:'+ $p.Id + ':' + $p.StartTime.ToUniversalTime().Ticks);[Console]::In.ReadLine()|Out-Null;if(-not $p.WaitForExit(15000)){throw 'Captured setup descendant remains alive'};[Console]::Out.WriteLine('absent')";
  const child = spawn(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'), ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')],
    { shell: false, env: { ...process.env, GESTALT_SETUP_OBSERVE_PID: String(pid) }, stdio: ['pipe', 'pipe', 'pipe'] });
  let stdout = '', stderr = '', exit;
  const completion = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', status => { exit = status; resolve(status); }); });
  void completion.catch(() => {});
  child.stdin.on('error', () => {});
  child.stdout.on('data', data => stdout += data); child.stderr.on('data', data => stderr += data);
  try {
    await until(async () => { if (exit !== undefined) throw new Error(`native setup observer failed before identity capture: ${stderr}`); return /held:\d+:\d+/.test(stdout); }, 'native setup observer did not capture a process handle');
  } catch (error) {
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    await completion.catch(() => {}); throw error;
  }
  const identity = stdout.match(/held:(\d+):(\d+)/).slice(1);
  let verified = false;
  return { identity, verify: async () => {
    if (verified) return;
    child.stdin.end('\n');
    assert.equal(await completion, 0, stderr); assert.match(stdout, /absent/); verified = true;
  } };
}

test(`${compiledBackend} interruption fences setup descendants or retains unknown transaction ownership`, { timeout: phaseBudgetMs + 120_000 }, async t => {
  const f = await fixture(t), ready = path.join(f.root, 'descendant.json'), heartbeat = path.join(f.root, 'descendant-writes'), release = path.join(f.root, 'stop-descendant'), done = path.join(f.root, 'descendant-done');
  const token = crypto.randomUUID(), config = path.join(f.codex, 'config.toml');
  const descendant = `const fs=require('node:fs');process.on('SIGTERM',()=>{});process.on('SIGINT',()=>{});const token=${JSON.stringify(token)};
fs.writeFileSync(${JSON.stringify(ready)},JSON.stringify({pid:process.pid,token}));
const interval=setInterval(()=>{if(fs.existsSync(${JSON.stringify(release)})){clearInterval(interval);fs.writeFileSync(${JSON.stringify(done)},token);process.exit(0);}fs.appendFileSync(${JSON.stringify(heartbeat)},token+'\\n');fs.writeFileSync(${JSON.stringify(config)},'# resistant descendant '+token+'\\n');},40);setTimeout(()=>process.exit(0),20000);`;
  await writeFile(f.worker, `import {spawn} from 'node:child_process';const child=spawn(process.execPath,['-e',${JSON.stringify(descendant)}],{detached:true,stdio:'ignore'});child.unref();setInterval(()=>{},1000);`);
  const extra = { GESTALT_AGENTS_SETUP_SHA256: hash(await readFile(f.owner)), GESTALT_SETUP_DESCENDANT: token };
  let leases;
  // A synthetic owner is isolated CI data. An existing owning context is always
  // inherited unchanged rather than remapping the active session's TMP roots.
  if (process.platform !== 'win32' && !Object.hasOwn(process.env, 'GESTALT_SESSION_TMP_CONTEXT')) {
    const sessionId = `setup-${token}`, ownerId = 'mobile-' + hash(Buffer.from(sessionId)), directory = path.join(f.root, 'tmp/sessions', ownerId), markerPath = path.join(directory, '.gestalt-owner.json');
    await mkdir(directory, { recursive: true, mode: 0o700 });
    await writeFile(markerPath, JSON.stringify({ schemaVersion: 1, ownerKind: 'mobile-session', ownerId, workspacePath: f.root, sessionId,
      createdAt: new Date().toISOString(), authority: { kind: 'mobile-roster', sessionId }, providerThreads: [] }), { mode: 0o600 });
    extra.GESTALT_SESSION_TMP_CONTEXT = JSON.stringify({ schemaVersion: 1, workspacePath: f.root, ownerId, directory, markerPath });
    leases = path.join(directory, 'leases');
  }
  const pending = f.start(['agents', 'install'], extra);
  let started = false, observer, heldIdentities = null;
  try {
    await until(async () => {
      if (!(await exists(ready)) || !(await exists(heartbeat))) return false;
      const identity = JSON.parse(await readFile(ready, 'utf8'));
      return identity.token === token && (await readFile(heartbeat, 'utf8')).split('\n').length >= 3;
    }, 'resistant detached descendant did not establish its writes', phaseBudgetMs, pending.result);
    started = true;
    if (process.platform === 'linux') {
      heldIdentities = await linuxSetupIdentities(token);
      assert.ok(heldIdentities.length >= 4, 'real controller/helper/installer/detached identities must be captured before cancellation');
    } else if (process.platform === 'win32') {
      observer = await windowsSetupObserver(JSON.parse(await readFile(ready, 'utf8')).pid);
      heldIdentities = observer.identity;
    }
    // Windows TerminateProcess is an actual abrupt parent death; POSIX uses a
    // real catchable SIGTERM. Neither path injects a fake platform or release.
    pending.child.kill(process.platform === 'win32' ? 'SIGKILL' : 'SIGTERM');
    const result = await pending.result;
    if (process.platform !== 'win32') assert.equal(result.status, 143, result.stderr);
    if (process.platform === 'linux') {
      assert.equal(await f.config(), custom);
      assert.match(result.stderr, /previous configuration restored after owned setup writers stopped/);
      assert.equal(await exists(path.join(f.codex, '.gestalt-manager-setup.lock')), false);
    } else {
      assert.equal(await exists(path.join(f.codex, '.gestalt-manager-setup.lock')), true);
      const stages = (await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.'));
      assert.equal(stages.length, 1);
      assert.equal(await readFile(path.join(f.codex, stages[0], 'config.toml'), 'utf8'), custom);
      assert.doesNotMatch(result.stderr, /previous configuration restored/);
    }
    if (process.platform === 'win32') await observer.verify();
    else if (process.platform === 'linux') await until(async () => {
      for (const expected of heldIdentities) {
        try { const info = await readFile(`/proc/${expected.pid}/stat`, 'utf8'); if (info.slice(info.lastIndexOf(')') + 2).split(' ')[19] === expected.started) return false; }
        catch (error) { if (!['ENOENT', 'ESRCH'].includes(error.code)) throw error; }
      }
      return true;
    }, 'captured setup process creation identities remain after release');
    const before = await readFile(heartbeat, 'utf8'), profile = await f.config();
    await new Promise(resolve => setTimeout(resolve, 400));
    const after = await readFile(heartbeat, 'utf8');
    const fenced = before === after;
    if (process.platform === 'darwin') {
      assert.equal(fenced, false, 'detached macOS fixture must exercise the unknown-retention case');
      assert.match(result.stderr, /rollback is unconfirmed/);
      const second = await f.run(); assert.notEqual(second.status, 0); assert.match(second.stderr, /EEXIST|already running/);
    } else { assert.equal(fenced, true, 'an owned descendant wrote after cancellation completed'); assert.equal(await f.config(), profile); }
    const leaseStates = leases ? await Promise.all((await readdir(leases)).map(async name => JSON.parse(await readFile(path.join(leases, name), 'utf8')).state)) : null;
    if (leaseStates) {
      assert.ok(leaseStates.length > 0);
      assert.ok(leaseStates.every(state => state === (process.platform === 'darwin' ? 'active' : 'released')));
    }
    if (process.env.GESTALT_NATIVE_SETUP_EVIDENCE) await writeFile(path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, 'setup-interruption.json'), JSON.stringify({
      observedAt: new Date().toISOString(), platform: process.platform, architecture: process.arch, actualSignal: process.platform === 'win32' ? 'TerminateProcess parent death' : 'SIGTERM',
      managerStatus: result.status, descendantReadyToken: token, descendantWritesFenced: fenced, rollback: process.platform === 'linux' ? 'restored after release proof' : 'snapshot and lock retained',
      borrowerStates: leaseStates, heldCreationIdentities: heldIdentities, capturedIdentitiesAbsent: process.platform !== 'darwin', ownershipHelperSha256: hash(await readFile(ownershipHelper)),
    }, null, 2));
  } finally {
    await writeFile(release, token);
    if (pending.child.exitCode === null && pending.child.signalCode === null) pending.child.kill(process.platform === 'win32' ? 'SIGKILL' : 'SIGTERM');
    await settleFixture(pending, f);
    if (process.platform === 'darwin' && started) {
      try {
        await until(() => exists(done), 'controlled detached macOS fixture did not stop after explicit release');
        const pid = JSON.parse(await readFile(ready, 'utf8')).pid;
        await until(async () => { try { process.kill(pid, 0); return false; } catch (error) { if (error.code === 'ESRCH') return true; throw error; } }, 'explicitly released macOS fixture did not exit');
      }
      catch (error) { f.retain(); throw error; }
    }
    if (observer) { try { await observer.verify(); } catch (error) { f.retain(); throw error; } }
  }
});

for (const initial of [null, custom]) test(`${compiledBackend} ${initial === null ? 'fresh' : 'custom'} Agents setup ${process.platform === 'darwin' ? 'retains unknown authority and refuses repeat' : 'repeats with stable config and provenance'}`, async t => {
  const f = await fixture(t, initial);
  const first = await f.run(); assert.equal(first.status, 0, first.stderr);
  if (process.platform === 'darwin') {
    assert.match(first.stderr, /command succeeded.*snapshot and setup locks retained/);
    const config = await f.config();
    const retained = (await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.'));
    assert.equal(retained.length, 1);
    assert.equal(JSON.parse(await readFile(path.join(f.codex, retained[0], 'ownership-unknown.json'), 'utf8')).status, 'command-succeeded-cleanup-unknown');
    assert.equal(await exists(path.join(f.codex, '.gestalt-manager-setup.lock')), true);
    assert.equal(await exists(path.join(f.codex, '.gestalt-setup-provenance.json')), false);
    const repeat = await f.run(['agents', 'update']);
    assert.notEqual(repeat.status, 0); assert.match(repeat.stderr, /EEXIST|already running/);
    assert.equal(await f.config(), config);
    assert.equal(await readFile(path.join(f.codex, 'AGENTS.md'), 'utf8'), 'user instructions\n');
    return;
  }
  const config = await f.config(), provenance = await f.provenance(), parsed = parse(config), metadata = JSON.parse(provenance);
  assert.equal(parsed.approval_policy, 'never'); assert.equal(parsed.default_permissions, 'workspace-git');
  assert.equal(parsed.mcp_servers['context-mode'].default_tools_approval_mode, 'approve');
  if (initial) { assert.ok(config.includes('# keep custom model')); assert.equal(parsed.model, 'fixture-custom'); }
  assert.equal(await readFile(path.join(f.codex, 'agents/custom.toml'), 'utf8'), 'model="preserve"\n');
  assert.equal(await readFile(path.join(f.codex, 'AGENTS.md'), 'utf8'), 'user instructions\n');
  assert.equal(metadata.scope, 'setup-artifacts-only'); assert.equal(metadata.runtimeEnforcement, false);
  assert.equal(metadata.manager.sha256, hash(await readFile(manager))); assert.equal(metadata.delegatedSetupSha256, hash(await readFile(f.owner)));
  assert.equal(metadata.outputs['config.toml'], hash(Buffer.from(config)));
  const repeat = await f.run(['agents', 'update']); assert.equal(repeat.status, 0, repeat.stderr);
  assert.equal(await f.config(), config); assert.equal(await f.provenance(), provenance);
  const retained = (await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.'));
  assert.deepEqual(retained, []);
  assert.equal(await exists(path.join(f.codex, '.gestalt-manager-setup.lock')), false);
  assert.equal(await exists(path.join(f.codex, '.gestalt-setup.lock')), false);
  if (process.platform !== 'win32' && initial) assert.equal((await stat(path.join(f.codex, 'config.toml'))).mode & 0o777, 0o640);
});

for (const failure of ['FIXTURE_FAIL', 'FIXTURE_VERIFY_FAIL', 'FIXTURE_MARKETPLACE_FAIL'])
  for (const initial of [null, custom]) test(`${failure} ${process.platform === 'darwin' ? 'retains unknown setup and' : 'restores'} ${initial === null ? 'absent' : 'custom'} native profile without successful provenance`, async t => {
    const f = await fixture(t, initial), result = await f.run(undefined, { [failure]: '1' });
    const [phase, status] = {
      FIXTURE_FAIL: ['owner-failure', 9],
      FIXTURE_VERIFY_FAIL: ['owner-verification-failure', 9],
      FIXTURE_MARKETPLACE_FAIL: ['marketplace-failure', 7],
    }[failure];
    assert.equal(result.status, status, result.stderr);
    assert.deepEqual((await f.events()).filter(event => event[0] === 'phase'), [['phase', phase, status]]);
    if (process.platform === 'darwin') {
      assert.match(result.stderr, /rollback is unconfirmed/);
      assert.doesNotMatch(result.stderr, /previous configuration restored/);
      assert.equal(await exists(path.join(f.codex, '.gestalt-manager-setup.lock')), true);
      const stages = (await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.'));
      assert.equal(stages.length, 1);
      const stage = path.join(f.codex, stages[0]), snapshot = JSON.parse(await readFile(path.join(stage, 'snapshot.json'), 'utf8'));
      assert.equal(snapshot.present, initial !== null);
      if (initial !== null) assert.equal(await readFile(path.join(stage, 'config.toml'), 'utf8'), initial);
      assert.equal(JSON.parse(await readFile(path.join(stage, 'ownership-unknown.json'), 'utf8')).status, 'failed-cleanup-unknown');
    } else {
      if (initial === null) assert.equal(await exists(path.join(f.codex, 'config.toml')), false); else assert.equal(await f.config(), initial);
      assert.deepEqual((await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup') || name === '.gestalt-manager-setup.lock'), []);
    }
    assert.equal(await exists(path.join(f.codex, '.gestalt-setup-provenance.json')), false);
  });

test('native setup preserves last successful provenance on a failed repeat', async t => {
  const f = await fixture(t); assert.equal((await f.run()).status, 0);
  if (process.platform === 'darwin') {
    const before = await f.config();
    const repeated = await f.run(['agents', 'update'], { FIXTURE_FAIL: '1' });
    assert.notEqual(repeated.status, 0); assert.match(repeated.stderr, /EEXIST|already running/);
    assert.equal(await f.config(), before); assert.equal(await exists(path.join(f.codex, '.gestalt-setup-provenance.json')), false);
    return;
  }
  const before = await f.config(), provenance = await f.provenance();
  const failed = await f.run(['agents', 'update'], { FIXTURE_FAIL: '1' });
  assert.equal(failed.status, 9, failed.stderr);
  assert.deepEqual((await f.events()).at(-1), ['phase', 'owner-failure', 9]);
  assert.equal(await f.config(), before); assert.equal(await f.provenance(), provenance);
});

test('native setup serializes concurrent managers and releases only its own lock', { timeout: process.platform === 'win32' ? 900_000 : 180_000 }, async t => {
  const f = await fixture(t), first = f.start(['agents', 'install'], { FIXTURE_WAIT: '1' });
  let firstResult;
  try {
    await until(() => exists(f.env.FIXTURE_READY), 'first owner did not reach controlled wait', phaseBudgetMs, first.result);
    const second = await f.run(); assert.notEqual(second.status, 0); assert.match(second.stderr, /EEXIST|already running/);
  } finally { await writeFile(f.env.FIXTURE_RELEASE, 'release'); firstResult = await settleFixture(first, f); }
  assert.equal(firstResult.status, 0, firstResult.stderr);
  const repeated = await f.run();
  if (process.platform === 'darwin') { assert.notEqual(repeated.status, 0); assert.match(repeated.stderr, /EEXIST|already running/); }
  else assert.equal(repeated.status, 0, repeated.stderr);
});

test('native setup refuses an existing delegated-owner lock before modifying config', async t => {
  const f = await fixture(t); await mkdir(path.join(f.codex, '.gestalt-setup.lock'), { mode: 0o700 });
  const result = await f.run(); assert.notEqual(result.status, 0); assert.match(result.stderr, /already running|unknown/);
  assert.equal(await f.config(), custom); assert.equal(await exists(f.log), false);
  assert.equal(await exists(path.join(f.codex, '.gestalt-setup.lock')), true);
});

test('native setup rejects invalid session context and unsupported options before mutation', async t => {
  const f = await fixture(t);
  const invalid = await f.run(undefined, { GESTALT_SESSION_TMP_CONTEXT: '' }); assert.notEqual(invalid.status, 0);
  assert.equal(await f.config(), custom); assert.equal(await exists(f.log), false);
  for (const args of [['install', '--executor-model', 'bad'], ['agents', 'install', '--extra-skills'], ['update', 'serena', '--extra-skills']]) {
    const result = await f.run(args); assert.notEqual(result.status, 0, result.stderr); assert.equal(await exists(f.log), false);
  }
});

async function fixturePermissions(file, makePermissive = false) {
  let acl = null;
  if (process.platform === 'win32') {
    if (makePermissive) {
      // icacls /grant changes this fixture's DACL only. Set-Acl can request a
      // SACL write and SeSecurityPrivilege even for a directory we own.
      const result = spawnSync(path.join(process.env.SystemRoot, 'System32/icacls.exe'),
        [file, '/grant', '*S-1-1-0:(OI)(CI)RX', '/Q'], { encoding: 'utf8', shell: false, timeout: 30_000 });
      assert.equal(result.status, 0, result.stderr || result.stdout);
    }
    const script = `$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue';$p=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:GESTALT_FIXTURE_ACL_PATH));$acl=(Get-Item -LiteralPath $p).GetAccessControl();
$everyone=@($acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])|Where-Object {$_.AccessControlType -eq 'Allow' -and $_.IdentityReference.Value -eq 'S-1-1-0'});
@{owner=$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value;group=$acl.GetGroup([Security.Principal.SecurityIdentifier]).Value;dacl=$acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Access);everyoneAllow=($everyone.Count -gt 0)}|ConvertTo-Json -Compress`;
    const result = spawnSync(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'),
      ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')],
      { env: { ...process.env, GESTALT_FIXTURE_ACL_PATH: Buffer.from(file).toString('base64') }, encoding: 'utf8', shell: false, timeout: 30_000 });
    assert.equal(result.status, 0, result.stderr); acl = JSON.parse(result.stdout);
    assert.match(acl.owner, /^S-1-/); assert.match(acl.group, /^S-1-/);
    assert.ok(acl.dacl.length > 0, 'fixture ACL inspection must return a DACL');
    if (makePermissive) assert.equal(acl.everyoneAllow, true, 'permissive fixture allow must actually be established');
  } else if (makePermissive) await chmod(file, 0o755);
  const info = await stat(file);
  return { mode: info.mode & 0o7777, uid: info.uid, gid: info.gid, dev: info.dev, ino: info.ino, mtimeMs: info.mtimeMs, ctimeMs: info.ctimeMs, acl };
}

for (const selected of ['codex', 'gestalt']) test(`install/update reject existing permissive ${selected} root without repair or subordinate mutation`,
  { timeout: process.platform === 'win32' ? 600_000 : 120_000 }, async t => {
    const f = await fixture(t), root = f[selected];
    const initialPermissions = await fixturePermissions(root);
    const permissions = await fixturePermissions(root, true);
    if (process.platform === 'win32') {
      assert.equal(permissions.acl.owner, initialPermissions.acl.owner, 'DACL fixture operation must preserve owner');
      assert.equal(permissions.acl.group, initialPermissions.acl.group, 'DACL fixture operation must preserve group');
      assert.equal(initialPermissions.acl.everyoneAllow, false); assert.equal(permissions.acl.everyoneAllow, true);
      assert.notEqual(permissions.acl.dacl, initialPermissions.acl.dacl);
    }
    const before = { codex: await captureSetupRetention(f.codex), gestalt: await captureSetupRetention(f.gestalt) };
    for (const argv of [['install'], ['update', 'gestalt']]) {
      const result = await f.run(argv);
      assert.notEqual(result.status, 0); assert.match(result.stderr, /Storage is not private|ACL permits another principal/);
      assert.equal(await exists(f.log), false, 'no selected/dependency command may run before private-home validation');
      assert.deepEqual(await fixturePermissions(root), permissions, 'existing permissive mode/ACL must remain unchanged');
      assert.deepEqual({ codex: await captureSetupRetention(f.codex), gestalt: await captureSetupRetention(f.gestalt) }, before);
    }
    if (process.env.GESTALT_NATIVE_SETUP_EVIDENCE) await writeFile(path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, `permissive-${selected}-root.json`),
      JSON.stringify({ observedAt: new Date().toISOString(), platform: process.platform, selected, initialPermissions, permissions, preserved: true, subordinateCommandStarted: false }, null, 2));
  });

test('npm-owned native manager update preserves package executable and retains component failure status', async t => {
  const f = await fixture(t), before = await readFile(manager);
  const result = await f.run(['update', 'gestalt'], { GESTALT_SKILLS_SOURCE: path.join(f.root, 'missing-fixture-collection') });
  assert.notEqual(result.status, 0); assert.match(result.stderr, /installed with npm/);
  assert.deepEqual(await readFile(manager), before); assert.equal(await exists(f.log), false);
});

test(`${compiledBackend} updated-manager continuation rejects invalid reentry without mutation`, async t => {
  const f = await fixture(t, custom, { freshHomes: process.platform !== 'win32' });
  const before = await captureCopyTree(f.home);
  for (const args of [[], ['__updated-manager'], ['unknown']]) {
    const result = await f.run(['__updated-manager', ...args]);
    assert.equal(result.status, 1, result.stderr);
    assert.match(result.stderr, /invalid updated-manager continuation/);
    assert.equal(await exists(f.log), false);
    assert.deepEqual(await captureCopyTree(f.home), before);
  }
});

// This fixture controls the marketplace/owner edges. The installed suite
// separately binds the real owner and real Codex to the public Unix entry.
if (process.platform !== 'win32') {
  const bash = path.join(path.dirname(manager), 'legacy/gestalt');
  const setupFiles = async f => (await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.') || name === '.gestalt-manager-setup.lock');
  const phases = async f => (await f.events()).filter(event => event[0] === 'phase');
  for (const initial of [null, custom]) {
    const profile = initial === null ? 'fresh profile' : 'custom profile';
    test(`public Unix Bash installs, repeats and retains last successful provenance (${profile})`, { timeout: 30_000 }, async t => {
      const f = await fixture(t, initial, { publicUnix: true });
      const first = await f.run();
      assert.equal(first.status, 0, first.stderr);
      assert.deepEqual(await phases(f), [['phase', 'owner-completed', 0]]);
      const [config, provenance] = await Promise.all([f.config(), f.provenance()]);
      const record = JSON.parse(provenance);
      assert.equal(record.manager.sha256, hash(await readFile(bash)));
      assert.equal(record.delegatedSetupSha256, hash(await readFile(path.join(f.ownerRoot, 'gestalt-setup.sh'))));
      assert.equal(record.outputs['config.toml'], hash(Buffer.from(config)));
      assert.equal(Object.hasOwn(record, 'delegatedApi'), false);
      assert.equal(parse(config).default_permissions, 'workspace-git');
      if (initial !== null) {
        assert.equal(parse(config).model, 'fixture-custom');
        assert.match(config, /# keep custom model/);
        assert.equal((await stat(path.join(f.codex, 'config.toml'))).mode & 0o777, 0o640);
      }
      const repeated = await f.run(['agents', 'update']);
      assert.equal(repeated.status, 0, repeated.stderr);
      assert.deepEqual(await phases(f), [['phase', 'owner-completed', 0], ['phase', 'owner-completed', 0]]);
      assert.equal(await f.config(), config);
      assert.equal(await f.provenance(), provenance);
      const failed = await f.run(['agents', 'update'], { FIXTURE_FAIL: '1' });
      assert.equal(failed.status, 9, failed.stderr);
      assert.deepEqual((await phases(f)).at(-1), ['phase', 'owner-failure', 9]);
      assert.equal(await f.config(), config);
      assert.equal(await f.provenance(), provenance);
      assert.equal(await readFile(path.join(f.codex, 'AGENTS.md'), 'utf8'), 'user instructions\n');
      assert.deepEqual(await setupFiles(f), []);
      for (const home of [f.codex, f.gestalt]) assert.equal(await exists(path.join(home, '.gestalt-setup.lock')), false);
    });

    for (const failure of [
      { key: 'FIXTURE_FAIL', status: 9, phases: [['phase', 'owner-failure', 9]], retained: false },
      { key: 'FIXTURE_VERIFY_FAIL', status: 1, phases: [['phase', 'owner-completed', 0], ['phase', 'plugin-verification', 0], ['phase', 'plugin-verification', 0]], retained: true },
      { key: 'FIXTURE_MARKETPLACE_FAIL', status: 7, phases: [['phase', 'marketplace-failure', 7], ['phase', 'marketplace-failure', 7]], retained: false },
    ]) test(`public Unix Bash ${failure.key} reaches its configured phase (${profile})`, { timeout: 30_000 }, async t => {
      const f = await fixture(t, initial, { publicUnix: true });
      const result = await f.run(undefined, { [failure.key]: '1' });
      const observed = await phases(f);
      t.diagnostic(JSON.stringify({ backend: 'unix-bash', controlledFixture: true, exit: result.status, phases: observed }));
      assert.equal(result.status, failure.status, result.stderr);
      assert.deepEqual(observed, failure.phases);
      assert.equal(await exists(path.join(f.codex, '.gestalt-setup-provenance.json')), false);
      if (failure.retained) {
        assert.match(result.stderr, /plugin version skew after marketplace setup/);
        assert.match(result.stderr, /ownership or rollback is unconfirmed/);
        const entries = await setupFiles(f);
        assert.ok(entries.includes('.gestalt-manager-setup.lock'));
        const stages = entries.filter(name => name.startsWith('.gestalt-setup.'));
        assert.equal(stages.length, 1);
        const stage = path.join(f.codex, stages[0]);
        if (initial === null) assert.equal(await exists(path.join(stage, 'config-absent')), true);
        else {
          const snapshot = path.join(stage, 'config.toml');
          assert.equal(await readFile(snapshot, 'utf8'), initial);
          assert.equal((await stat(snapshot)).mode & 0o777, 0o640);
        }
        assert.match(await f.config(), /mcp_servers\.context-mode/);
      } else {
        if (initial === null) assert.equal(await exists(path.join(f.codex, 'config.toml')), false);
        else {
          assert.equal(await f.config(), initial);
          assert.equal((await stat(path.join(f.codex, 'config.toml'))).mode & 0o777, 0o640);
        }
        assert.deepEqual(await setupFiles(f), []);
      }
      for (const home of [f.codex, f.gestalt]) assert.equal(await exists(path.join(home, '.gestalt-setup.lock')), false);
      assert.equal(await readFile(path.join(f.codex, 'AGENTS.md'), 'utf8'), 'user instructions\n');
    });
  }

  for (const selected of ['codex', 'gestalt']) test(`public Unix Bash refuses unsafe existing ${selected} home with a selected native owner`, { timeout: 30_000 }, async t => {
    const f = await fixture(t, custom, { publicUnix: true });
    const market = path.join(f.codex, '.tmp/marketplaces/dyne-gestalt-agents');
    await mkdir(path.dirname(market), { recursive: true, mode: 0o700 });
    await symlink(f.ownerRoot, market, 'dir');
    await chmod(f[selected], 0o755);
    const before = await captureCopyTree(f.home);
    const result = await f.run(['agents', 'update']);
    assert.equal(result.status, 1, result.stderr);
    assert.match(result.stderr, /native Agents setup requires a directory private to its owner/);
    assert.ok(result.stderr.includes(f[selected]));
    assert.equal(await exists(f.log), false);
    assert.equal((await stat(f[selected])).mode & 0o777, 0o755);
    assert.deepEqual(await captureCopyTree(f.home), before);
  });

  test('public Unix Bash validates, replaces and reenters the packaged update asset', { timeout: 60_000 }, async t => {
    const f = await fixture(t, null, { freshHomes: true, publicUnix: true });
    const { collection, release, skill } = await compositionInputs(f.root);
    const compiled = await buildCli(repository, path.join(f.root, 'public bin ü', 'gestalt.mjs'));
    const backend = path.join(path.dirname(compiled), 'legacy/gestalt');
    const asset = path.join(release, 'gestalt');
    const previous = await readFile(backend), compiledBefore = await readFile(compiled);
    const env = { ...f.env, GESTALT_MANAGER_DISTRIBUTION: '', GESTALT_SKILLS_SOURCE: collection,
      GESTALT_INSTALL_BASE_URL: pathToFileURL(release).href };
    const run = (extra = {}) => spawnSync(process.execPath, [compiled, 'update', 'gestalt', '--extra-skills'],
      { env: { ...env, ...extra }, cwd: f.root, encoding: 'utf8', shell: false, timeout: 30_000 });
    await writeFile(asset, '#!/usr/bin/env bash\nif then\n');
    const invalid = run();
    assert.equal(invalid.status, 1, invalid.stderr);
    assert.match(invalid.stderr, /downloaded manager has invalid Bash syntax/);
    assert.deepEqual(await readFile(backend), previous);
    assert.equal(await exists(f.codex), false);
    assert.equal(await exists(f.gestalt), false);
    assert.equal(await exists(f.log), false);

    const replacement = Buffer.concat([previous, Buffer.from('\n# controlled standalone Bash replacement\n')]);
    await writeFile(asset, replacement);
    const fetched = spawnSync('curl', ['--fail', '--silent', '--show-error', pathToFileURL(asset).href],
      { env, cwd: f.root, shell: false, timeout: 30_000 });
    assert.equal(fetched.status, 0, fetched.stderr?.toString());
    assert.deepEqual(fetched.stdout, replacement);
    const updated = run();
    assert.equal(updated.status, 0, updated.stderr);
    assert.equal((updated.stderr.match(/checking for a manager update/g) ?? []).length, 1);
    assert.match(updated.stderr, /continuing update with the newly installed manager/);
    assert.deepEqual(await readFile(backend), replacement);
    assert.deepEqual(await readFile(compiled), compiledBefore);
    assert.equal(await readFile(path.join(f.codex, 'skills/native-update/SKILL.md'), 'utf8'), skill);
    const setup = JSON.parse(await f.provenance());
    assert.equal(setup.manager.sha256, hash(replacement));
    assert.equal(setup.delegatedSetupSha256, hash(await readFile(path.join(f.ownerRoot, 'gestalt-setup.sh'))));
    const calls = await f.events();
    assert.deepEqual(calls.find(row => row[0] === 'setup'), ['setup', '--extra-skills']);
    assert.ok(calls.some(row => row[0] === 'npm' && row[1] === 'install' && /^gestalt-mobile@/.test(row.at(-1))));
    const userData = path.join(f.gestalt, 'mobile/operator.txt');
    await writeFile(userData, 'preserved');
    const failedMobile = run({ GESTALT_MANAGER_DISTRIBUTION: 'npm', FIXTURE_MOBILE_FAIL: '1' });
    assert.equal(failedMobile.status, 41, failedMobile.stderr);
    const failingCall = (await f.events()).at(-1);
    assert.equal(failingCall[0], 'npm');
    assert.match(failingCall.at(-1), /^gestalt-mobile@/);
    assert.match(failedMobile.stderr, /manager is installed with npm/);
    assert.doesNotMatch(failedMobile.stderr, /checking for a manager update|update complete/);
    assert.equal(await readFile(userData, 'utf8'), 'preserved');
    assert.deepEqual(await readFile(backend), replacement);
  });
}

test(process.platform === 'win32'
  ? 'compiled install/update preserves native skills, extra selection and optional Mobile policy'
  : 'managed JavaScript install/update preserves native skills, extra selection and optional Mobile policy', async t => {
  const f = await fixture(t, null, { freshHomes: true }), collection = path.join(f.root, 'collection ü');
  assert.equal(await exists(f.codex), false); assert.equal(await exists(f.gestalt), false);
  await mkdir(path.join(collection, 'skills/native-fixture'), { recursive: true });
  const skill = '---\nname: native-fixture\ndescription: Native setup fixture.\n---\n# Fixture\n';
  await writeFile(path.join(collection, 'skills/native-fixture/SKILL.md'), skill);
  await writeFile(path.join(collection, 'LICENSE'), 'MIT fixture\n');
  await writeFile(path.join(collection, 'manifest.json'), JSON.stringify({ schemaVersion: 1, repository: 'dyne/gestalt-skills', source: 'https://github.com/dyne/gestalt-skills', version: '0.1.0', skills: [{ name: 'native-fixture', entry: 'skills/native-fixture/SKILL.md', files: [{ path: 'skills/native-fixture/SKILL.md', sha256: hash(Buffer.from(skill)) }] }] }));
  for (const args of [['init', '--initial-branch=main', '--template='], ['add', '--all'], ['commit', '-m', 'native collection fixture']]) {
    const git = spawnSync('git', ['-c', 'core.hooksPath=' + path.join(collection, '.empty-hooks'), '-c', 'commit.gpgSign=false', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test', ...args], { cwd: collection, encoding: 'utf8', shell: false });
    assert.equal(git.status, 0, git.stderr);
  }
  const env = { GESTALT_SKILLS_SOURCE: collection };
  const policy = { platform: process.platform, observedAt: new Date().toISOString(), observations: [], complete: false };
  const observePolicy = async (phase, argv, result) => {
    policy.observations.push({ phase, argv, result });
    if (process.env.GESTALT_NATIVE_SETUP_EVIDENCE) await writeFile(path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, 'compiled-setup-selection.json'), JSON.stringify(policy, null, 2));
  };
  const refuseDeferred = async phase => {
    const before = await captureCopyTree(f.home), priorLog = await readFile(f.log).catch(error => { if (error.code === 'ENOENT') return null; throw error; });
    for (const command of ['install', 'update', 'update-restart']) for (const component of ['serena', 'playwright', 'mobile']) {
      const argv = [command, component], result = await f.run(argv, { ...env, GESTALT_SESSION_TMP_CONTEXT: '{}' });
      await observePolicy(phase, argv, result);
      assert.equal(result.status, 1, result.stderr); assert.match(result.stderr, new RegExp(`native ${component} support is deferred`, 'i'));
      assert.doesNotMatch(result.stderr, /SESSION_SCRATCH_INVALID_CONTEXT/);
      assert.deepEqual(await captureCopyTree(f.home), before);
      assert.deepEqual(await readFile(f.log).catch(error => { if (error.code === 'ENOENT') return null; throw error; }), priorLog);
    }
  };
  if (process.platform === 'win32') await refuseDeferred('absent-profiles');
  const installed = await f.run(['install', '--extra-skills'], env);
  await observePolicy('install', ['install', '--extra-skills'], installed); assert.equal(installed.status, 0, installed.stderr);
  const receipt = JSON.parse(await readFile(path.join(f.codex, 'gestalt-skills/receipt.json'), 'utf8'));
  assert.equal(receipt.verifiedSource, false);
  assert.equal(await readFile(path.join(f.codex, 'skills/native-fixture/SKILL.md'), 'utf8'), skill);
  assert.ok((await readFile(f.log, 'utf8')).split('\n').includes('["setup","--extra-skills"]'));
  if (process.platform === 'win32') {
    assert.match(installed.stderr, /Mobile omitted:.*native win32.*owned-process.*session-scratch/);
    assert.equal(await exists(path.join(f.gestalt, 'mobile')), false);
    assert.doesNotMatch(installed.stderr, /then 'gestalt cli' or 'gestalt mobile'/);
    assert.match(installed.stderr, /Serena omitted:.*deferred/); assert.match(installed.stderr, /Playwright omitted:.*deferred/);
    const retained = await NativePrivateStorage.open(f.gestalt, unixProbe, async () => ownershipHelper);
    for (const component of ['serena', 'playwright', 'mobile']) {
      await retained.createDirectory(path.join(f.gestalt, component));
      await retained.replaceFile(path.join(f.gestalt, component, 'active.json'), Buffer.from('retained operator metadata\n'));
    }
    await refuseDeferred('existing-profiles');
    const all = await f.run(['update', 'all'], { ...env, FIXTURE_MOBILE_FAIL: '1' });
    await observePolicy('update-all', ['update', 'all'], all); assert.equal(all.status, 0, all.stderr);
    assert.match(all.stderr, /Serena omitted:.*deferred/); assert.match(all.stderr, /Playwright omitted:.*deferred/);
    assert.doesNotMatch(all.stderr, /use gestalt update serena to install|use gestalt playwright install/);
    for (const component of ['serena', 'playwright', 'mobile']) assert.equal(await readFile(path.join(f.gestalt, component, 'active.json'), 'utf8'), 'retained operator metadata\n');
  } else await writeFile(path.join(f.gestalt, 'mobile/user-data.txt'), 'preserve this data\n');
  const captureRetainedSetup = async () => ({
    codex: await captureSetupRetention(f.codex),
    gestalt: await captureSetupRetention(f.gestalt),
    subordinateCalls: await readFile(f.log, 'utf8'),
  });
  const retained = process.platform === 'darwin' ? await captureRetainedSetup() : null;
  if (retained) {
    assert.match(installed.stderr, /command succeeded.*snapshot and setup locks retained/);
    assert.ok(retained.codex.managerLock, 'the Manager setup operation lock must be retained');
    assert.deepEqual(retained.codex.managerSnapshots.map(snapshot => snapshot.ownership.status),
      ['command-succeeded-cleanup-unknown']);
    const owner = JSON.parse(await readFile(path.join(f.codex, '.gestalt-skills.lock/owner.json'), 'utf8'));
    assert.equal(owner.schemaVersion, 1);
    assert.equal(owner.home, f.codex);
    assert.equal(owner.committed, true);
    assert.ok(owner.journal.links.some(link => link.name === 'native-fixture'));
    assert.equal(path.dirname(owner.stage), f.codex);
    assert.match(path.basename(owner.stage), /^\.gestalt-skills-stage-[A-Za-z0-9]+$/);
    assert.ok(retained.codex.entries.some(entry => entry.path === path.basename(owner.stage) && entry.kind === 'directory'));
  }
  const updated = await f.run(['update', 'gestalt'], env);
  await observePolicy('update-gestalt', ['update', 'gestalt'], updated);
  if (retained) {
    assert.equal(updated.status, 1, updated.stderr);
    assert.match(updated.stderr, /skills transaction is retained; use gestalt skills inspect-recovery.*explicit confirmed recovery/);
    assert.doesNotMatch(updated.stderr, /update complete/);
    assert.deepEqual(await captureRetainedSetup(), retained, 'refused update must preserve retained state and start no subordinate fixture commands');
    assert.equal(await readFile(path.join(f.gestalt, 'mobile/user-data.txt'), 'utf8'), 'preserve this data\n');
    policy.complete = true;
    await observePolicy('retained-state-verified', [], { status: 0 });
    return;
  }
  assert.equal(updated.status, 0, updated.stderr);
  assert.equal(await readFile(path.join(f.codex, 'skills/native-fixture/SKILL.md'), 'utf8'), skill);
  assert.equal(await readFile(path.join(f.codex, 'gestalt-skills/receipt.json'), 'utf8'), JSON.stringify(receipt, null, 2) + '\n');
  if (process.platform === 'win32') assert.doesNotMatch(updated.stderr, /Mobile, and context-mode are up to date/);
  else {
    assert.equal(await readFile(path.join(f.gestalt, 'mobile/user-data.txt'), 'utf8'), 'preserve this data\n');
    const failed = await f.run(['update', 'gestalt'], { ...env, FIXTURE_MOBILE_FAIL: '1' });
    assert.equal(failed.status, 41, failed.stderr);
    assert.equal(await readFile(path.join(f.gestalt, 'mobile/user-data.txt'), 'utf8'), 'preserve this data\n');
    assert.doesNotMatch(failed.stderr, /update complete/);
  }
  policy.complete = true;
  await observePolicy('complete', [], { status: 0 });
});

test('native restart update failure retains the live relay, descriptor and non-owned data', async t => {
  const f = await fixture(t), run = path.join(f.gestalt, 'run');
  await mkdir(run, { mode: 0o700 });
  const relay = spawn(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { stdio: 'ignore' });
  const stopped = new Promise(resolve => relay.once('close', resolve));
  t.after(async () => { if (relay.exitCode === null && relay.signalCode === null) relay.kill(); await stopped; });
  const descriptor = path.join(run, `mobile-${relay.pid}.restart`), bytes = Buffer.from([f.root, '--port', '4321', ''].join('\0'));
  await writeFile(descriptor, bytes, { mode: 0o600 });
  const lock = path.join(run, 'update-restart.lock'); await mkdir(lock, { mode: 0o700 });
  const identity = await stat(lock, { bigint: true }), token = 'fixture-controlled-token';
  await writeFile(path.join(lock, 'owner.json'), JSON.stringify({ schemaVersion: 1, token, dev: String(identity.dev), ino: String(identity.ino) }), { mode: 0o600 });
  const log = path.join(f.gestalt, 'update-restart.log');
  const context = new SetupContext({ argv: [], cwd: f.root, environment: f.env },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: f.home, codexHome: f.codex, gestaltHome: f.gestalt, marketplace: 'fixture', marketplaceName: 'fixture', manager, unixProbe: unixProbe || undefined, ownershipHelper }, '0.16.1');
  const status = await restartWorker(context, [String(relay.pid), descriptor, 'false', log, lock, 'gestalt', token],
    async () => { throw new CommandFailure('fixture update failed (exit 42)', 42); });
  assert.equal(status, 42); assert.equal(relay.exitCode, null); assert.equal(relay.signalCode, null);
  assert.deepEqual(await readFile(descriptor), bytes); assert.equal(await exists(lock), false);
  assert.match(await readFile(log, 'utf8'), /fixture update failed \(exit 42\)/);
});

for (const invalid of [false, true]) test(`native standalone ${invalid ? 'invalid replacement preserves installed executable' : 'replacement resumes exact argv and propagates failure'}`, async t => {
  const f = await fixture(t), directory = path.join(f.root, 'public bin ü'), release = path.join(f.root, 'release');
  await mkdir(directory, { mode: 0o755 }); await mkdir(release, { mode: 0o700 });
  const target = path.join(directory, 'gestalt.mjs'), marker = '#!/usr/bin/env node\n// GESTALT_CLI_VERSION=\'0.16.1\'\n// GESTALT_UPDATED_MANAGER_API=\'1\'\n';
  const previous = marker + '// previous release\n';
  await writeFile(target, previous, { mode: 0o755 });
  const resultFile = path.join(f.root, 'replacement-argv.json');
  const replacement = invalid ? marker + 'invalid javascript !' : marker + `import fs from 'node:fs';
export const UPDATED_MANAGER_API_VERSION=1,UPDATED_MANAGER_API_CAPABILITIES=['host-filesystem-v1','async-owned-commands-v1','cancellation-v1'];
export async function continueUpdatedManager(invocation,streams,{commands,signal}){if(commands.apiVersion!==1||signal.aborted)throw new Error('bad managed handoff');fs.writeFileSync(${JSON.stringify(resultFile)},JSON.stringify(invocation.argv));return {apiVersion:1,status:42,ownership:{kind:'released'}};}\n`;
  await writeFile(path.join(release, 'gestalt.mjs'), replacement);
  const env = { ...f.env, GESTALT_MANAGER_DISTRIBUTION: '', GESTALT_INSTALL_BASE_URL: pathToFileURL(release).href };
  const context = new SetupContext({ argv: [], cwd: f.root, environment: env },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: f.home, codexHome: f.codex, gestaltHome: f.gestalt, marketplace: 'fixture', marketplaceName: 'fixture', manager: target, unixProbe: unixProbe || undefined, ownershipHelper }, '0.16.1');
  const argv = ['update', 'gestalt', '--extra-skills', 'space ü & literal'];
  await assert.rejects(updateManager(context, argv, false), error => invalid || error.status === 42);
  assert.equal(await readFile(target, 'utf8'), invalid ? previous : replacement);
  if (!invalid) assert.deepEqual(JSON.parse(await readFile(resultFile, 'utf8')), ['__updated-manager', ...argv]);
  const lock = path.join(directory, '.gestalt-manager-update.lock');
  assert.equal(await exists(lock), process.platform === 'darwin');
  if (process.platform === 'darwin') {
    f.retain();
    const receipt = JSON.parse(await readFile(path.join(lock, 'ownership-unknown.json'), 'utf8'));
    assert.equal(receipt.promoted, !invalid); assert.ok(receipt.claims.length > 0);
    assert.ok(receipt.claims.some(entry => entry.claim.kind === 'unknown'));
    assert.equal(await exists(path.join(receipt.scratch, 'gestalt.mjs')), true);
    assert.equal(await exists(path.join(receipt.scratch, 'ownership-unknown.json')), true);
  }
  if (process.platform !== 'win32') assert.equal((await stat(target)).mode & 0o777, 0o755);
});

for (const scenario of ['interrupt-before-handoff', 'failure-before-handoff', 'interrupt-active-handoff'])
  test(`standalone update ${scenario} retains only unknown authority and settles its owned commands`, { timeout: phaseBudgetMs + 180_000 }, async t => {
    const f = await fixture(t), directory = path.join(f.root, 'manager bin'), release = path.join(f.root, 'release');
    await mkdir(directory, { mode: 0o755 }); await mkdir(release, { mode: 0o700 });
    const target = path.join(directory, 'gestalt.mjs'), entered = path.join(f.root, 'handoff-entered'), ready = path.join(f.root, 'handoff-worker-ready');
    const marker = '#!/usr/bin/env node\n// GESTALT_CLI_VERSION=\'0.16.1\'\n// GESTALT_UPDATED_MANAGER_API=\'1\'\n';
    await writeFile(target, marker + '// old complete executable\n', { mode: 0o755 });
    const worker = `const fs=require('node:fs');process.on('SIGTERM',()=>{});fs.writeFileSync(${JSON.stringify(ready)},'ready');setInterval(()=>{},1000);`;
    const replacement = marker + `import fs from 'node:fs';
export const UPDATED_MANAGER_API_VERSION=1,UPDATED_MANAGER_API_CAPABILITIES=['host-filesystem-v1','async-owned-commands-v1','cancellation-v1'];
export async function continueUpdatedManager(invocation,streams,{commands,signal}){
fs.writeFileSync(${JSON.stringify(entered)},JSON.stringify(invocation));
const lease=await commands.launch(process.execPath,{argv:['-e',${JSON.stringify(worker)}],cwd:invocation.cwd,environment:invocation.environment},streams,{graceMs:1000,deadlineMs:10000});
const abort=()=>{void lease.stop();};signal.addEventListener('abort',abort,{once:true});if(signal.aborted)abort();
try{await lease.result;const claim=await lease.stop();return {apiVersion:1,status:signal.aborted?signal.reason.status:0,ownership:{kind:claim.kind==='released'?'released':'unknown'}};}finally{signal.removeEventListener('abort',abort);}}
`;
    await writeFile(path.join(release, 'gestalt.mjs'), replacement);
    const env = { ...f.env, GESTALT_MANAGER_DISTRIBUTION: '', GESTALT_INSTALL_BASE_URL: pathToFileURL(release).href };
    let reached, releaseGate;
    const gate = new Promise(resolve => { releaseGate = resolve; }), reachedGate = new Promise(resolve => { reached = resolve; });
    const log = [];
    const context = new SetupContext({ argv: [], cwd: f.root, environment: env },
      { stdin: (async function* () {})(), stdout: async () => {}, stderr: async bytes => {
        const text = Buffer.from(bytes).toString('utf8'); log.push(text);
        if (text.includes('updated manager at') && scenario !== 'interrupt-active-handoff') {
          reached(); await gate;
          if (scenario === 'failure-before-handoff') throw new Error('controlled failure after replacement');
        }
      } },
      { userHome: f.home, codexHome: f.codex, gestaltHome: f.gestalt, marketplace: 'fixture', marketplaceName: 'fixture', manager: target,
        unixProbe: unixProbe || undefined, ownershipHelper }, '0.16.1');
    const argv = ['update', 'gestalt', '--extra-skills', 'literal ü & token'];
    const operation = updateManager(context, argv, false); void operation.catch(() => {});
    const lock = path.join(directory, '.gestalt-manager-update.lock');
    try {
      if (scenario === 'interrupt-active-handoff') {
        await until(() => exists(ready), 'replacement external command did not reach controlled readiness', phaseBudgetMs,
          operation.then(() => ({ status: 0 }), error => ({ status: error.status || 1, stderr: error.message })));
        assert.equal(await exists(lock), true, 'serialization lock must span the active continuation');
        const forwarded = JSON.parse(await readFile(entered, 'utf8'));
        assert.deepEqual(forwarded.argv, ['__updated-manager', ...argv]);
        assert.equal(forwarded.cwd, f.root); assert.equal(forwarded.environment.CODEX_HOME, f.codex);
        process.emit('SIGTERM');
      } else {
        let timer;
        try { await Promise.race([reachedGate,
          operation.then(() => { throw new Error('manager ended before the controlled handoff phase'); }, error => { throw error; }),
          new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('replacement phase was not reached')), phaseBudgetMs); })]); }
        finally { clearTimeout(timer); }
        assert.equal(await exists(lock), true);
        if (scenario === 'interrupt-before-handoff') process.emit('SIGTERM');
        releaseGate();
      }
      await assert.rejects(operation, error => scenario === 'failure-before-handoff'
        ? /controlled failure after replacement/.test(error.message) : error.status === 143);
      assert.equal(await readFile(target, 'utf8'), replacement, 'a complete installed replacement is not rolled back');
      assert.equal(await exists(entered), scenario === 'interrupt-active-handoff');
      assert.equal(await exists(lock), process.platform === 'darwin');
      if (process.platform === 'darwin') {
        const receipt = JSON.parse(await readFile(path.join(lock, 'ownership-unknown.json'), 'utf8'));
        assert.equal(receipt.promoted, true); assert.equal(await exists(path.join(receipt.scratch, 'gestalt.mjs')), true);
      } else {
        assert.equal(context.canRollback(), true); assert.deepEqual(context.unknownOwnership, []);
      }
      if (process.env.GESTALT_NATIVE_SETUP_EVIDENCE) await writeFile(path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, `manager-update-${scenario}.json`),
        JSON.stringify({ observedAt: new Date().toISOString(), platform: process.platform, scenario, injectedSignal: scenario !== 'failure-before-handoff',
          replacementSha256: hash(Buffer.from(replacement)), installedSha256: hash(await readFile(target)), continuationEntered: await exists(entered),
          lockRetained: await exists(lock), cleanupAuthorized: context.canRollback(), unknownClaims: context.unknownOwnership, log }, null, 2));
    } finally {
      releaseGate(); await context.stopActive(); await operation.catch(() => {});
      if (!context.canRollback()) f.retain();
    }
  });
