import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createRequire} from 'node:module';
import {mkdtemp, mkdir, readFile, writeFile, rm, readdir, stat} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import test from 'node:test';

const manager = resolve('public/gestalt'), source = await readFile(manager, 'utf8');
const program = source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const runtime = new Function('require', program + '\nreturn {scratchAllocate,scratchContext,scratchAssociate,scratchRelease,scratchBorrow,scratchLeaseRecords,scratchDiscoveryEnvironment};')(createRequire(import.meta.url));
const pause = ms => new Promise(r => setTimeout(r, ms));
async function until(read, predicate) {
  const deadline = Date.now() + 7000;
  while (true) {
    try {const value = await read(); if (predicate(value)) return value;} catch (e) {if (e.code !== 'ENOENT') throw e;}
    assert.ok(Date.now() < deadline, 'owned lifecycle fixture deadline'); await pause(20);
  }
}
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt scratch lifecycle '));
  const workspace = join(root, 'selected workspace'), other = join(root, 'other workspace');
  await mkdir(workspace); await mkdir(other);
  t.after(() => rm(root, {recursive: true, force: true}));
  return {root, workspace, other};
}
const environment = own => ({GESTALT_SESSION_TMP_CONTEXT: JSON.stringify(own.context)});
const leasePath = own => join(own.context.directory, 'leases', own.lease.leaseId+'.json');
const readLease = own => readFile(leasePath(own), 'utf8').then(JSON.parse);

test('CLI invocation UUIDs are isolated and observed resume associations never reuse another invocation owner', async t => {
  const f = await fixture(t), first = runtime.scratchAllocate(f.workspace, 'cli-session', 'cli-controller');
  const simultaneous = runtime.scratchAllocate(f.workspace, 'cli-session', 'cli-controller');
  const separateRoot = runtime.scratchAllocate(f.other, 'cli-session', 'cli-controller');
  assert.match(first.context.ownerId, /^cli-[0-9a-f-]{36}$/);
  assert.notEqual(first.context.ownerId, simultaneous.context.ownerId);
  const initial = JSON.parse(await readFile(first.context.markerPath, 'utf8'));
  assert.deepEqual(initial.providerThreads, []); assert.equal((await readLease(first)).state, 'active');
  assert.equal(runtime.scratchContext(environment(first)).workspacePath, f.workspace);
  const thread = '01a19bd7-bdb5-76e0-bc41-28d650392275';
  runtime.scratchAssociate(environment(first), thread); runtime.scratchAssociate(environment(first), thread);
  runtime.scratchAssociate(environment(separateRoot), thread);
  assert.notEqual(first.context.ownerId,separateRoot.context.ownerId);
  assert.equal(runtime.scratchRelease(first), true);
  assert.throws(() => runtime.scratchContext(environment(first)), /INVALID_CONTEXT/);
  const resumed = runtime.scratchAllocate(f.workspace, 'cli-session', 'cli-controller');
  runtime.scratchAssociate(environment(resumed),thread);
  assert.notEqual(resumed.context.ownerId,first.context.ownerId); assert.notEqual(resumed.lease.leaseId, first.lease.leaseId);
  const retained = JSON.parse(await readFile(resumed.context.markerPath, 'utf8'));
  assert.equal(JSON.parse(await readFile(first.context.markerPath,'utf8')).createdAt,initial.createdAt);
  assert.deepEqual(retained.providerThreads, [{provider: 'codex', threadId: thread}]);
  assert.equal((await stat(resumed.context.directory)).mode & 0o777, 0o700);
  assert.equal((await stat(leasePath(resumed))).mode & 0o777, 0o600);
});

test('active shared consumer leases outlive callers and block release until every owned consumer stops', async t => {
  const f = await fixture(t), cli = runtime.scratchAllocate(f.workspace, 'cli-session', 'cli-controller');
  const service = runtime.scratchAllocate(f.workspace, 'workspace-service', 'shared-tool');
  const consumer = runtime.scratchBorrow(service.context, cli.context.ownerId);
  assert.equal(runtime.scratchRelease(cli), true);
  assert.equal((await readLease(service)).state, 'active');
  assert.equal(runtime.scratchRelease(service), false);
  assert.equal(runtime.scratchRelease(consumer), true);
  assert.equal(runtime.scratchRelease(service), true);
  assert.equal((await readLease(service)).state, 'released');
  assert.deepEqual((await readLease(service)).consumers, []);
});

test('crashed and partially published owners remain protected without PID/age recovery', async t => {
  const f = await fixture(t), owner = runtime.scratchAllocate(f.workspace, 'cli-session', 'cli-controller');
  const record = await readLease(owner);
  await writeFile(leasePath(owner), JSON.stringify({...record, createdAt: '2000-01-01T00:00:00.000Z', updatedAt: '2000-01-01T00:00:00.000Z'}));
  assert.equal(runtime.scratchContext(environment(owner)).ownerId, owner.context.ownerId);
  const next=runtime.scratchAllocate(f.workspace,'cli-session','cli-controller');
  assert.notEqual(next.context.ownerId,owner.context.ownerId);
  assert.equal((await readLease(owner)).state, 'active');
  await writeFile(join(owner.context.directory, 'leases', 'broken.json'), '{}', {mode: 0o600});
  assert.throws(() => runtime.scratchRelease(owner), /INVALID_CONTEXT/);
  assert.equal((await readLease(owner)).state, 'active');
  assert.ok((await readdir(owner.context.directory)).includes('.controller-lock'));
});

test('an exited leader with a surviving descendant retains its lease and receives no stale group signal', {timeout:15000,skip:process.platform!=='linux'}, async t => {
  const f=await fixture(t),owner=runtime.scratchAllocate(f.workspace,'cli-session','cli-controller');
  const observation=join(f.root,'descendant.json'),signalFile=join(f.root,'unexpected-signal');
  const worker=join(f.root,'leader.cjs');
  const descendant=`const fs=require('node:fs');const identity=fs.readFileSync('/proc/self/stat','utf8').split(') ').at(-1).split(' ')[19];fs.writeFileSync(${JSON.stringify(observation)},JSON.stringify({pid:process.pid,identity}));process.on('SIGTERM',()=>{fs.writeFileSync(${JSON.stringify(signalFile)},'TERM');process.exit();});setInterval(()=>{},1000);`;
  await writeFile(worker,`const{spawn}=require('node:child_process');const fs=require('node:fs');spawn(process.execPath,['-e',${JSON.stringify(descendant)}],{stdio:'ignore'});const poll=setInterval(()=>{if(fs.existsSync(${JSON.stringify(observation)})){clearInterval(poll);process.exit(7);}},10);`);
  const controller=spawn(process.execPath,['-e',program+'\nscratchSupervise('+JSON.stringify(manager)+','+JSON.stringify(owner)+',process.execPath,'+JSON.stringify([worker])+').catch(()=>process.exitCode=1);'],
    {env:runtime.scratchDiscoveryEnvironment(process.env),stdio:['pipe','ignore','pipe']});
  controller.stderr.resume();const exited=new Promise(r=>controller.once('exit',r));
  let child;
  t.after(async()=>{
    if(controller.exitCode===null&&controller.signalCode===null)controller.kill('SIGTERM');await exited;
    if(child){try{const info=await readFile('/proc/'+child.pid+'/stat','utf8');if(info.split(') ').at(-1).split(' ')[19]===child.identity)process.kill(child.pid,'SIGTERM');}catch(e){if(e.code!=='ENOENT'&&e.code!=='ESRCH')throw e;}
      await until(async()=>{try{return (await readFile('/proc/'+child.pid+'/stat','utf8')).split(') ')[1].startsWith('Z');}catch(e){if(e.code==='ENOENT')return true;throw e;}},Boolean);}
  });
  child=await until(()=>readFile(observation,'utf8').then(JSON.parse),Boolean);
  assert.equal(await exited,7);assert.equal((await readLease(owner)).state,'active');
  process.kill(child.pid,0);await assert.rejects(stat(signalFile),{code:'ENOENT'});
  assert.ok((await readdir(owner.context.directory)).includes('.controller-lock'));
});

for (const event of ['SIGINT', 'SIGKILL']) test('lifetime pipe stops actual child before releasing lease after controller '+event, {timeout: 15000}, async t => {
  const f = await fixture(t), owner = runtime.scratchAllocate(f.workspace, 'cli-session', 'cli-controller');
  const worker = join(f.root, 'worker.cjs'), observation = join(f.root, 'child.json');
  await writeFile(worker, `const fs=require('node:fs'),{spawnSync}=require('node:child_process');
const nested=spawnSync(process.execPath,['-e','console.log(JSON.stringify(process.env))'],{encoding:'utf8'});
fs.writeFileSync(${JSON.stringify(observation)},JSON.stringify({pid:process.pid,environment:process.env,nested:JSON.parse(nested.stdout)}));
setInterval(()=>{},1000);`);
  const controller = spawn(process.execPath, ['-e', program+'\nscratchSupervise('+JSON.stringify(manager)+','+JSON.stringify(owner)+',process.execPath,'+JSON.stringify([worker])+').catch(()=>process.exitCode=1);'],
    {env: runtime.scratchDiscoveryEnvironment(process.env), stdio: ['pipe', 'ignore', 'pipe']});
  controller.stderr.resume(); const exited = new Promise(r => controller.once('exit', r));
  t.after(async () => {if (controller.exitCode === null && controller.signalCode === null) controller.kill('SIGTERM'); await exited;});
  const child = await until(() => readFile(observation, 'utf8').then(JSON.parse), Boolean);
  for (const env of [child.environment, child.nested]) {
    assert.equal(env.TMPDIR, owner.context.directory); assert.equal(env.TMP, env.TMPDIR); assert.equal(env.TEMP, env.TMPDIR);
    assert.equal(env.GESTALT_SESSION_TMP_CONTEXT, environment(owner).GESTALT_SESSION_TMP_CONTEXT);
  }
  assert.equal((await readLease(owner)).state, 'active');
  controller.kill(event); await exited;
  await until(() => readLease(owner), lease => lease.state === 'released');
  assert.throws(() => process.kill(child.pid, 0), /ESRCH/);
  assert.ok(!(await readdir(owner.context.directory)).includes('.controller-lock'));
});
