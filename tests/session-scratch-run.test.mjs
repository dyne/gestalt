import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdtemp, mkdir, readFile, readdir, rm, stat, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import test from 'node:test';
import {sessionScratch, mobileResourceLease} from './session-scratch-fixture.mjs';

const manager = resolve('public/gestalt');
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'socket runner '));
  const workspace = join(root, 'workspace with spaces ' + 'x'.repeat(80));
  await mkdir(workspace);
  const scratch = await sessionScratch(workspace);
  t.after(() => rm(root, {recursive:true, force:true}));
  return {root, workspace, scratch};
}
function launch(f, args, overrides = {}) {
  const child = spawn('bash', [manager, 'scratch', 'run', ...args], {
    cwd:f.workspace, env:{...process.env, ...f.scratch.environment, ...overrides},
    stdio:['pipe', 'pipe', 'pipe'],
  });
  let stdout = '', stderr = '';
  child.stdout.on('data', bytes => stdout += bytes);
  child.stderr.on('data', bytes => stderr += bytes);
  const done = once(child, 'close').then(([code, signal]) => ({code, signal, stdout, stderr}));
  return {child, done};
}
async function leases(f) {
  const path = join(f.scratch.context.directory, 'leases');
  return Promise.all((await readdir(path)).map(async name => JSON.parse(await readFile(join(path, name), 'utf8'))));
}

test('public runner binds a Chrome-shaped Unix socket under a long managed owner and preserves native I/O',
  {skip:process.platform !== 'linux', timeout:15000}, async t => {
    const f = await fixture(t);
    const script = join(f.root, 'socket fixture.cjs');
    await writeFile(script, `
const assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path'),net=require('node:net');
const context=JSON.parse(process.env.GESTALT_SESSION_TMP_CONTEXT),root=os.tmpdir();
assert.match(root,/^\\/proc\\/\\d+\\/fd\\/\\d+$/);
assert.equal(fs.realpathSync(root),context.directory);
assert.equal(process.env.TMP,root);assert.equal(process.env.TEMP,root);
const directory=fs.mkdtempSync(path.join(root,'com.google.Chrome.'));
const socket=path.join(directory,'SingletonSocket');assert.ok(Buffer.byteLength(socket)<108);
const actual=path.join(context.directory,path.basename(directory),'SingletonSocket');
const server=net.createServer(client=>client.end('connected'));
server.listen(socket,()=>{
 assert.ok(fs.lstatSync(actual).isSocket());
 const client=net.connect(socket);let reply='';client.on('data',b=>reply+=b);
 client.on('end',()=>{assert.equal(reply,'connected');server.close(()=>{
  fs.rmSync(directory,{recursive:true,force:true});
  let input='';process.stdin.on('data',b=>input+=b);process.stdin.on('end',()=>{
   console.log(JSON.stringify({root,input,args:process.argv.slice(2),cwd:process.cwd(),context:process.env.GESTALT_SESSION_TMP_CONTEXT}));
  });
 });});
});
`);
    const args = ['a path with spaces', 'literal $(no) `command`\nnext line'];
    const {child, done} = launch(f, ['--', process.execPath, script, ...args], {TMPDIR:'/hostile', TMP:'/hostile', TEMP:'/hostile'});
    child.stdin.end('original stdin\n');
    const result = await done;
    assert.equal(result.code, 0, result.stderr);
    const value = JSON.parse(result.stdout);
    assert.equal(value.input, 'original stdin\n');
    assert.deepEqual(value.args, args);
    assert.equal(value.cwd, f.workspace);
    assert.equal(value.context, f.scratch.environment.GESTALT_SESSION_TMP_CONTEXT);
    await assert.rejects(stat(value.root), {code:'ENOENT'});
    const records = await leases(f);
    assert.equal(records.length, 1);
    assert.equal(records[0].state, 'released');
    assert.deepEqual(records[0].consumers, []);
    assert.ok((await stat(f.scratch.context.directory)).isDirectory());
  });

test('runner preserves a command failure and releases only its borrower', {timeout:15000}, async t => {
  const f = await fixture(t);
  const resource = await mobileResourceLease(f.scratch);
  const {child, done} = launch(f, ['--', process.execPath, '-e', 'process.stderr.write("fixture failure\\n");process.exit(42)']);
  child.stdin.end();
  const result = await done;
  assert.equal(result.code, 42);
  assert.equal(result.stdout, '');
  assert.equal(result.stderr, 'fixture failure\n');
  const records = await leases(f);
  assert.equal(records.find(lease => lease.leaseId !== resource.lease.leaseId).state, 'released');
  assert.deepEqual(JSON.parse(await readFile(resource.path, 'utf8')), resource.lease);
});

test('TERM keeps the short path alive through command shutdown and releases the borrower',
  {skip:process.platform !== 'linux', timeout:15000}, async t => {
    const f = await fixture(t), witness = join(f.root, 'shutdown.json');
    const script = `const fs=require('node:fs');
process.on('SIGTERM',()=>{fs.writeFileSync(process.argv[1],JSON.stringify({root:process.env.TMPDIR,real:fs.realpathSync(process.env.TMPDIR)}));process.exit();});
console.log('ready');setInterval(()=>{},1000);`;
    const {child, done} = launch(f, ['--', process.execPath, '-e', script, witness]);
    t.after(async () => {if(child.exitCode === null && child.signalCode === null)child.kill('SIGTERM');await done;});
    await once(child.stdout, 'data');
    const active = await leases(f);
    assert.equal(active.length, 1);
    assert.equal(active[0].state, 'active');
    assert.deepEqual(active[0].consumers, ['socket-command']);
    child.kill('SIGTERM');
    const result = await done;
    assert.equal(result.code, 143, result.stderr);
    const value = JSON.parse(await readFile(witness, 'utf8'));
    assert.equal(value.real, f.scratch.context.directory);
    await assert.rejects(stat(value.root), {code:'ENOENT'});
    assert.equal((await leases(f))[0].state, 'released');
  });

test('runner rejects missing commands and invalid or absent managed context before execution', {timeout:15000}, async t => {
  const f = await fixture(t), witness = join(f.root, 'unexpected');
  for (const [args, env] of [
    [[], {}],
    [['--'], {}],
    [[process.execPath, '-e', `require('node:fs').writeFileSync(${JSON.stringify(witness)},'bad')`], {}],
    [['--', process.execPath, '-e', `require('node:fs').writeFileSync(${JSON.stringify(witness)},'bad')`], {GESTALT_SESSION_TMP_CONTEXT:''}],
  ]) {
    const {child, done} = launch(f, args, env);child.stdin.end();
    assert.notEqual((await done).code, 0);
  }
  const env = {...process.env};
  delete env.GESTALT_SESSION_TMP_CONTEXT;
  const child = spawn('bash', [manager, 'scratch', 'run', '--', process.execPath, '-e', `require('node:fs').writeFileSync(${JSON.stringify(witness)},'bad')`], {env, stdio:'ignore'});
  assert.notEqual((await once(child, 'close'))[0], 0);
  await assert.rejects(stat(witness), {code:'ENOENT'});
  await assert.rejects(stat(join(f.scratch.context.directory, 'leases')), {code:'ENOENT'});
});
