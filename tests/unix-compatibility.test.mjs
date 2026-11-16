import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { access, mkdir, mkdtemp, readFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { interactiveBackendFor, selectInteractiveRuntime } from '../src/cli/platform/interactive.ts';
const root=path.resolve(import.meta.dirname,'..'), unix=['linux','darwin'].includes(process.platform);
const options={skip:unix?false:'Unix replacement contracts; Windows owned producer has separate native evidence',timeout:15000};
const directory=await mkdtemp(path.join(tmpdir(),'unix-compat-'));let peer;
test.before(async()=>{if(unix)peer=await buildNativePlatform(root,path.join(directory,'native'));});
test.after(()=>rm(directory,{recursive:true,force:true}));
function bounded(promise) {let timer;return Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('compatibility fixture deadline')),10000)})]).finally(()=>clearTimeout(timer));}
test('interactive platform selection excludes the namespace producer from both Unix targets',()=>{
  assert.equal(interactiveBackendFor('linux'),'unix-compatibility');assert.equal(interactiveBackendFor('darwin'),'unix-compatibility');assert.equal(interactiveBackendFor('win32'),'windows-owned');
  assert.throws(()=>interactiveBackendFor('freebsd'),/unavailable/);
  if(unix){const value=selectInteractiveRuntime({legacyManager:process.execPath,ownedHelper:'relative-and-invalid'});assert.equal(value.kind,'unix-compatibility');assert.equal(value.legacyEntry,process.execPath);assert.equal('owned'in value,false);assert.equal('stop'in value,false);}
});
async function execute(t,manager,args,input=Buffer.alloc(0),environment=process.env){
  const before=path.join(directory,`before-${Math.random()}.json`), argv=[path.join(root,'tests/fixtures/unix-compat-entry.mjs'),manager,peer,before,root,...args];
  const child=spawn(process.execPath,argv,{env:environment,shell:false,stdio:['pipe','pipe','pipe']});
  const out=[],err=[],exit=once(child,'close');child.stdout.on('data',b=>out.push(Buffer.from(b)));child.stderr.on('data',b=>err.push(Buffer.from(b)));child.stdin.on('error',()=>{});
  t.after(async()=>{if(child.exitCode===null&&child.signalCode===null)child.kill();await bounded(exit);});child.stdin.end(input);
  const [code,signal]=await bounded(exit);return{child,code,signal,before:JSON.parse(await readFile(before,'utf8')),stdout:Buffer.concat(out),stderr:Buffer.concat(err),marker:`${before}.js-exit-cleanup`};
}
test('Unix replacement preserves actual host process identity, literal argv, environment and byte streams',options,async t=>{
  const argv=['','space é 東京','&echo no','|','%PATH%','$(not-evaluated)','"quoted"','tail\\','\r\n'],input=Buffer.from([0,255,13,10]);
  const result=await execute(t,process.execPath,[path.join(root,'tests/fixtures/unix-compat-target.mjs'),peer,'echo',...argv],input);
  assert.equal(result.code,23);assert.equal(result.signal,null);assert.deepEqual(result.stderr,Buffer.from([255,0,13]));
  const target=JSON.parse(result.stdout);assert.deepEqual(target.argv,argv);assert.equal(target.inputHex,input.toString('hex'));
  assert.deepEqual(target.identity,result.before.identity);assert.equal(target.pid,result.child.pid);assert.equal(target.ppid,result.before.ppid);
  assert.equal(target.uid,result.before.uid);assert.equal(target.gid,result.before.gid);assert.deepEqual(target.environment,result.before.environment);
  assert.equal(target.rootOwner,(await stat('/')).uid);assert.equal(target.tmpOwner,(await stat('/tmp')).uid);
  if(process.platform==='linux')assert.deepEqual(target.namespaces,result.before.namespaces);
  await assert.rejects(access(result.marker),{code:'ENOENT'});
});
test('Unix replacement keeps the actual three terminal descriptors and dimensions',options,async t=>{
  const before=path.join(directory,'tty-before.json');
  const args=[path.join(root,'tests/fixtures/unix-compat-pty.py'),process.execPath,path.join(root,'tests/fixtures/unix-compat-entry.mjs'),process.execPath,peer,before,root,path.join(root,'tests/fixtures/unix-compat-target.mjs'),peer,'tty'];
  const child=spawn('python3',args,{shell:false,stdio:['ignore','pipe','pipe']});const out=[],err=[],exit=once(child,'close');
  child.stdout.on('data',b=>out.push(b));child.stderr.on('data',b=>err.push(b));t.after(async()=>{if(child.exitCode===null&&child.signalCode===null)child.kill();await bounded(exit);});
  assert.equal((await bounded(exit))[0],0,Buffer.concat(err).toString());const record=JSON.parse(Buffer.concat(out));assert.equal(record.exit,23);
  const match=Buffer.from(record.outputBase64,'base64').toString().match(/ACTUAL_TTY:([^\r\n]+)/);assert.ok(match);const actual=JSON.parse(match[1]);
  assert.deepEqual(actual.tty,[true,true,true]);assert.deepEqual(actual.ttySize,{columns:81,rows:25});assert.deepEqual(actual.identity,JSON.parse(await readFile(before,'utf8')).identity);
});
test('Unix compatibility executes the unchanged public manager help and retirement guard',options,async t=>{
  const home=path.join(directory,'home');await mkdir(home,{mode:0o700});
  const env={...process.env,HOME:home,CODEX_HOME:path.join(home,'codex'),GESTALT_HOME:path.join(home,'gestalt')};
  const help=await execute(t,path.join(root,'public/gestalt'),['help'],Buffer.alloc(0),env);assert.equal(help.code,0,help.stderr.toString());assert.match(help.stdout.toString(),/Usage: gestalt/);
  const retired=await execute(t,path.join(root,'public/gestalt'),['xerj'],Buffer.alloc(0),env);assert.equal(retired.code,1);assert.match(retired.stderr.toString(),/retired/);
});
test('the original Unix caller process receives cancellation after replacement without a new supervisor',options,async t=>{
  const before=path.join(directory,'signal-before.json'), argv=[path.join(root,'tests/fixtures/unix-compat-entry.mjs'),process.execPath,peer,before,root,path.join(root,'tests/fixtures/unix-compat-target.mjs'),peer,'hold'];
  const child=spawn(process.execPath,argv,{shell:false,stdio:['ignore','pipe','pipe']});let text='',wake;const ready=new Promise(resolve=>{wake=resolve});const exit=once(child,'close');
  child.stdout.on('data',b=>{text+=b;if(text.includes('\n'))wake();});child.stderr.resume();
  t.after(async()=>{if(child.exitCode===null&&child.signalCode===null)child.kill('SIGKILL');await bounded(exit);});
  await bounded(Promise.race([ready,exit.then(()=>{throw Error('replacement exited before signal readiness')})]));
  const target=JSON.parse(text.split('\n')[0]);assert.equal(target.pid,child.pid);assert.deepEqual(target.identity,JSON.parse(await readFile(before,'utf8')).identity);
  child.kill('SIGTERM');assert.equal((await bounded(exit))[0],19);assert.match(text,/signal-observed/);
});
