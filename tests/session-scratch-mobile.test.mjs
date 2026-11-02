import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createRequire} from 'node:module';
import {createServer, get} from 'node:http';
import {mkdtemp,mkdir,readFile,writeFile,readdir,stat,rm,symlink} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import test from 'node:test';
import {sessionScratch,mobileResourceLease} from './session-scratch-fixture.mjs';

const manager=resolve('public/gestalt'),source=await readFile(manager,'utf8');
const program=source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const runtime=new Function('require',program+'\nreturn {scratchBorrow,scratchRelease,scratchAllocate,scratchLeaseRecords};')(createRequire(import.meta.url));
const pause=ms=>new Promise(r=>setTimeout(r,ms));
async function until(read,predicate) {
  const deadline=Date.now()+8000;
  while(true){try{const value=await read();if(predicate(value))return value;}catch(error){if(error.code!=='ENOENT')throw error;}
    assert.ok(Date.now()<deadline,'Mobile scratch fixture deadline');await pause(20);}
}
async function fixture(t) {
  const root=await mkdtemp(join(tmpdir(),'gestalt mobile borrowers ')),workspace=join(root,'selected workspace');
  await mkdir(workspace);const scratch=await sessionScratch(workspace,'mobile-borrower-fixture');
  t.after(()=>rm(root,{recursive:true,force:true}));return{root,workspace,scratch};
}
const leasePath=own=>join(own.context.directory,'leases',own.lease.leaseId+'.json');
const lease=own=>readFile(leasePath(own),'utf8').then(JSON.parse);

for(const parentResource of [false,true])test('Mobile borrower publishes private independent lease '+(parentResource?'with parent resource':'without prior leases'),async t=>{
  const f=await fixture(t),parent=parentResource?await mobileResourceLease(f.scratch):undefined;
  const before=await readFile(f.scratch.context.markerPath,'utf8');
  const own=runtime.scratchBorrow(f.scratch.context,'managed-tool');
  assert.equal(own.borrowed,true);assert.equal((await lease(own)).state,'active');
  for(const directory of [f.scratch.context.directory,join(f.scratch.context.directory,'leases')])assert.equal((await stat(directory)).mode&0o777,0o700);
  assert.equal((await stat(leasePath(own))).mode&0o777,0o600);
  assert.equal(await readFile(f.scratch.context.markerPath,'utf8'),before);
  await assert.rejects(stat(join(f.scratch.context.directory,'.controller-lock')),{code:'ENOENT'});
  if(parent){await writeFile(parent.path,JSON.stringify({...parent.lease,state:'released',consumers:[]}));assert.equal((await lease(own)).state,'active');}
  assert.equal(runtime.scratchRelease(own),true);assert.equal((await lease(own)).state,'released');
});

test('Mobile borrower cannot steal cleanup/crashed fences or follow a foreign leases directory',async t=>{
  const f=await fixture(t),lock=join(f.scratch.context.directory,'.controller-lock');await mkdir(lock,{mode:0o700});
  assert.throws(()=>runtime.scratchBorrow(f.scratch.context,'blocked'),/INVALID_CONTEXT/);
  assert.deepEqual(await readdir(lock),[]);await rm(lock,{recursive:true});
  const outside=join(f.root,'outside');await mkdir(outside);await symlink(outside,join(f.scratch.context.directory,'leases'));
  assert.throws(()=>runtime.scratchBorrow(f.scratch.context,'blocked'),/INVALID_CONTEXT/);assert.deepEqual(await readdir(outside),[]);
});

test('concurrent Mobile publications are complete and never release another controller lease',async t=>{
  const f=await fixture(t),parent=await mobileResourceLease(f.scratch),before=await readFile(parent.path,'utf8');
  await Promise.all(Array.from({length:8},(_,i)=>new Promise((done,reject)=>{
    const code=program+'\n(async()=>{const own=scratchBorrow(JSON.parse(process.argv[1]),process.argv[2]);if(!await scratchReleaseReady(own))process.exitCode=2;})().catch(()=>process.exitCode=1);';
    const p=spawn(process.execPath,['-e',code,JSON.stringify(f.scratch.context),'parallel-'+i],{stdio:['ignore','ignore','pipe']});
    let stderr='';p.stderr.on('data',v=>stderr+=v);p.once('error',reject);p.once('close',status=>{try{assert.equal(status,0,stderr);done();}catch(error){reject(error);}});
  })));
  const records=runtime.scratchLeaseRecords(f.scratch.context);assert.equal(records.length,9);
  assert.equal(records.filter(record=>record.state==='released').length,8);assert.equal(await readFile(parent.path,'utf8'),before);
});

test('old crashed and malformed Mobile lease records remain protected without repair',async t=>{
  const f=await fixture(t),own=runtime.scratchBorrow(f.scratch.context,'crashed-tool');
  const old={...own.lease,createdAt:'2000-01-01T00:00:00.000Z',updatedAt:'2000-01-01T00:00:00.000Z'};
  await writeFile(leasePath(own),JSON.stringify(old));
  assert.throws(()=>runtime.scratchRelease(own),/INVALID_CONTEXT/);assert.equal((await lease(own)).state,'active');
  const broken=join(f.scratch.context.directory,'leases','broken.json');await writeFile(broken,'{',{mode:0o600});
  assert.throws(()=>runtime.scratchBorrow(f.scratch.context,'new-tool'));
  assert.equal(await readFile(broken,'utf8'),'{');assert.deepEqual(await lease(own),old);
});

for(const ending of ['EOF','SIGTERM','SIGKILL','cleanup-fence'])test('Mobile guardian '+ending+' releases only confirmed owned group absence',{timeout:15000,skip:process.platform!=='linux'},async t=>{
  const f=await fixture(t);await mobileResourceLease(f.scratch);
  const own=runtime.scratchBorrow(f.scratch.context,'detached-test'),observation=join(f.root,'worker.json');
  const worker=`const fs=require('node:fs');fs.writeFileSync(${JSON.stringify(observation)},JSON.stringify({pid:process.pid,identity:fs.readFileSync('/proc/self/stat','utf8').split(') ').at(-1).split(' ')[19],env:Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'].map(k=>[k,process.env[k]]))}));setInterval(()=>{},1000);`;
  const code=program+'\nscratchGuard(JSON.parse(process.argv[1]),process.execPath,["-e",process.argv[2]]).catch(()=>process.exitCode=1);';
  const guardian=spawn(process.execPath,['-e',code,JSON.stringify(own),worker],{env:{...process.env,...f.scratch.environment},stdio:['pipe','ignore','pipe','ignore']});
  guardian.stderr.resume();const exited=new Promise(r=>guardian.once('exit',r));let child;
  t.after(async()=>{
    if(guardian.exitCode===null&&guardian.signalCode===null)guardian.kill('SIGTERM');await exited;
    if(child)try{const fields=(await readFile('/proc/'+child.pid+'/stat','utf8')).split(') ').at(-1).split(' ');
      if(fields[19]===child.identity&&fields[0]!=='Z')process.kill(child.pid,'SIGTERM');}catch(error){if(!['ENOENT','ESRCH'].includes(error.code))throw error;}
    if(child)await until(async()=>{try{return (await readFile('/proc/'+child.pid+'/stat','utf8')).split(') ').at(-1).split(' ')[0]==='Z';}
      catch(error){if(error.code==='ENOENT')return true;throw error;}},Boolean);
  });
  child=await until(()=>readFile(observation,'utf8').then(JSON.parse),Boolean);
  assert.deepEqual(child.env,f.scratch.environment);assert.equal((await lease(own)).state,'active');
  if(ending==='cleanup-fence')await mkdir(join(f.scratch.context.directory,'.controller-lock'),{mode:0o700});
  if(['EOF','cleanup-fence'].includes(ending))guardian.stdin.end();else guardian.kill(ending);await exited;
  if(ending==='SIGKILL'){assert.equal((await lease(own)).state,'active');process.kill(child.pid,0);}
  else if(ending==='cleanup-fence'){assert.equal((await lease(own)).state,'active');assert.throws(()=>process.kill(-child.pid,0),/ESRCH/);}
  else{assert.equal((await lease(own)).state,'released');assert.throws(()=>process.kill(-child.pid,0),/ESRCH/);}
});

test('Mobile cleanup sees an active detached borrower after parent release and retains a simultaneous CLI owner',
  {timeout:15000,skip:!process.env.GESTALT_MOBILE_SCRATCH_CLEANUP||process.platform!=='linux'},async t=>{
  const {FilesystemScratchCleanup}=await import(pathToFileURL(process.env.GESTALT_MOBILE_SCRATCH_CLEANUP).href);
  const f=await fixture(t),parent=await mobileResourceLease(f.scratch),own=runtime.scratchBorrow(f.scratch.context,'detached-tool');
  const cli=runtime.scratchAllocate(f.workspace,'cli-session','cli-controller'),cleanup=new FilesystemScratchCleanup();
  const observed=join(f.root,'cleanup-worker.json');
  const worker=`require('node:fs').writeFileSync(${JSON.stringify(observed)},JSON.stringify({pid:process.pid}));setInterval(()=>{},1000);`;
  const guardian=spawn(process.execPath,['-e',program+'\nscratchGuard(JSON.parse(process.argv[1]),process.execPath,["-e",process.argv[2]]).catch(()=>process.exitCode=1);',JSON.stringify(own),worker],
    {env:{...process.env,...f.scratch.environment},stdio:['pipe','ignore','ignore','ignore']});
  const exited=new Promise(r=>guardian.once('exit',r));
  t.after(async()=>{if(guardian.exitCode===null&&guardian.signalCode===null)guardian.stdin.end();await exited;});
  const child=await until(()=>readFile(observed,'utf8').then(JSON.parse),Boolean);
  await writeFile(parent.path,JSON.stringify({...parent.lease,state:'released',consumers:[]}));
  const active=await cleanup.cleanWorkspace(f.workspace,()=>false);assert.equal(active.removed,0);assert.ok(active.skipped>=2);
  assert.equal((await lease(own)).state,'active');process.kill(child.pid,0);
  guardian.stdin.end();await exited;
  assert.equal((await lease(own)).state,'released');assert.throws(()=>process.kill(-child.pid,0),/ESRCH/);
  const after=await cleanup.cleanWorkspace(f.workspace,()=>false);assert.equal(after.removed,1);
  await assert.rejects(stat(f.scratch.context.directory),{code:'ENOENT'});assert.equal((await lease(cli)).state,'active');
});

test('production Unix broker address binds and connects inside a long Mobile owner without relocating storage',
  {skip:process.platform!=='linux'},async t=>{
  const f=await fixture(t);await mobileResourceLease(f.scratch);
  const directory=await mkdtemp(join(f.scratch.context.directory,'gestalt-cli-playwright-'));
  const region=source.match(/\/\/ BEGIN MANAGED BROWSER SOCKET ADDRESS\n([\s\S]*?)\/\/ END MANAGED BROWSER SOCKET ADDRESS/)[1];
  const openSocket=new Function(region+'\nreturn managedBrowserSocket;')(),socket=await openSocket(directory);
  assert.ok(Buffer.byteLength(socket.endpoint)>108);assert.ok(Buffer.byteLength(socket.address)<100);
  const server=createServer((_req,res)=>res.end('owned socket'));
  try{
    await new Promise((done,reject)=>{server.once('error',reject);server.listen(socket.address,done);});
    const fs=await import('node:fs/promises');await fs.chmod(socket.endpoint,0o600);
    assert.ok((await stat(socket.endpoint)).isSocket());assert.equal((await stat(socket.endpoint)).mode&0o777,0o600);
    assert.equal(await new Promise((done,reject)=>{get({socketPath:socket.address,path:'/'},res=>{let data='';res.on('data',v=>data+=v);res.on('end',()=>done(data));}).on('error',reject);}), 'owned socket');
  }finally{await new Promise(done=>server.close(done));await socket.close();await rm(directory,{recursive:true,force:true});}
  await stat(f.scratch.context.markerPath);await assert.rejects(stat(directory),{code:'ENOENT'});
});
