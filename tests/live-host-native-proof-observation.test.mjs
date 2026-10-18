// Cheap observer regressions; these do not substitute for actual Native proof.
import test from 'node:test';
import assert from 'node:assert/strict';
import {createHash,createHmac,randomBytes} from 'node:crypto';
import {mkdtempSync,mkdirSync,writeFileSync,readFileSync,readlinkSync,lstatSync,realpathSync,chmodSync,rmSync,symlinkSync} from 'node:fs';
import {dirname,join} from 'node:path';
import {tmpdir} from 'node:os';
import {spawn} from 'node:child_process';
const source=readFileSync(new URL('../scripts/live-host-policy-native-proof.mjs',import.meta.url),'utf8');
const stable=value=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
const hash=value=>createHash('sha256').update(value).digest('hex');
function extract(start,end){const at=source.indexOf(start);assert.ok(at>=0);const until=source.indexOf(end,at+start.length);assert.ok(until>at);return source.slice(at,until);}
function observers(t,split=true){
 const root=mkdtempSync(join(tmpdir(),'live-proof-observer-'));chmodSync(root,0o700);t.after(()=>rmSync(root,{recursive:true,force:true}));
 const key=join(root,'key');writeFileSync(key,randomBytes(32),{mode:0o600});
 const id='test-capture',pid=999999999,processRecord={pid,birth:'123',uid:process.getuid(),namespaces:{mnt:'test-mnt',user:'test-user',pid:'test-pid'}};
 const entries=[['PATH','/usr/bin:/bin'],['HOME',join(root,split?'normal':'original')],['CODEX_HOME',join(root,'original')],['TMPDIR','/tmp'],['LANG','C.UTF-8']];
 const bytes=Buffer.from(entries.map(([name,value])=>`${name}=${value}`).join('\0')+'\0');
 const environment={schemaVersion:1,kind:'owning-runtime-environment',captureId:id,basisDigest:hash('basis'),process:processRecord,environmentDigest:hash(bytes),entries};
 const sealed={basisDigest:environment.basisDigest,environmentReceiptDigest:hash(stable(environment)),runtime:{process:processRecord,environmentDigest:environment.environmentDigest}};
 const sign=(path,body)=>writeFileSync(path,JSON.stringify({body,mac:createHmac('sha256',readFileSync(key)).update(stable(body)).digest('hex')}),{mode:0o600});
 const runtimePath=join(root,`runtime-${id}.json`),envPath=join(root,`environment-${id}.json`),manifest=join(root,'manifest.json');sign(runtimePath,sealed);sign(envPath,environment);
 let currentBytes=bytes;
 const read=(path,...args)=>path===`/proc/${pid}/environ`?currentBytes:readFileSync(path,...args);
 const dependencies={assert,lstatSync,dirname,realpathSync,process,readFileSync:read,createHmac,key,stable,hash,Buffer,join,identity:actual=>{assert.equal(actual,pid);return processRecord;}};
 const functions=Function(...Object.keys(dependencies),extract('function signedBody(path) {','function runtimeChildren() {')+'\nreturn {signedBody,privateEnvironmentProof};')(...Object.values(dependencies));
 return {...functions,root,id,pid,manifest,environment,sealed,runtimePath,envPath,sign,setBytes:value=>{currentBytes=value;}};
}
test('proof observer authenticates private split-home environment and exports only assertions/digests',t=>{
 const fixture=observers(t);const result=fixture.privateEnvironmentProof(fixture.manifest,fixture.id,fixture.pid);
 assert.equal(result.homesEqual,false);assert.equal(result.valuesExported,false);assert.equal(result.orderedDigestMatched,true);
 assert.deepEqual(Object.keys(result).sort(),['captureId','environmentDigest','homesEqual','orderedDigestMatched','ownerOnly','processBound','signed','valuesExported']);
 assert.equal(JSON.stringify(result).includes(fixture.root),false);
});
test('proof observer preserves same-home assertions',t=>{const f=observers(t,false);assert.equal(f.privateEnvironmentProof(f.manifest,f.id,f.pid).homesEqual,true);});
for(const kind of ['mac','mode','symlink','parent-mode','seal-binding','process-binding','ordered-process-bytes','disallowed-key'])test(`proof observer rejects ${kind}`,t=>{
 const f=observers(t);
 if(kind==='mac'){const saved=JSON.parse(readFileSync(f.envPath));saved.mac='0'.repeat(64);writeFileSync(f.envPath,JSON.stringify(saved));}
 if(kind==='mode')chmodSync(f.envPath,0o644);
 if(kind==='symlink'){const original=f.envPath+'-original';f.sign(original,f.environment);rmSync(f.envPath);symlinkSync(original,f.envPath);}
 if(kind==='parent-mode')chmodSync(f.root,0o755);
 if(kind==='seal-binding'){f.environment.basisDigest=hash('changed');f.sign(f.envPath,f.environment);}
 if(kind==='process-binding'){f.environment.process={...f.environment.process,birth:'124'};f.sealed.environmentReceiptDigest=hash(stable(f.environment));f.sign(f.envPath,f.environment);f.sign(f.runtimePath,f.sealed);}
 if(kind==='ordered-process-bytes')f.setBytes(Buffer.from([...f.environment.entries].reverse().map(([name,value])=>`${name}=${value}`).join('\0')+'\0'));
 if(kind==='disallowed-key'){f.environment.entries.push(['UNRECOGNIZED','fixture']);const bytes=Buffer.from(f.environment.entries.map(([name,value])=>`${name}=${value}`).join('\0')+'\0');f.environment.environmentDigest=hash(bytes);f.sealed.environmentReceiptDigest=hash(stable(f.environment));f.sealed.runtime.environmentDigest=hash(bytes);f.setBytes(bytes);f.sign(f.envPath,f.environment);f.sign(f.runtimePath,f.sealed);}
 assert.throws(()=>f.privateEnvironmentProof(f.manifest,f.id,f.pid));
});
test('proof observer freshly authenticates every receipt observation after mutation',t=>{
 const f=observers(t);f.privateEnvironmentProof(f.manifest,f.id,f.pid);f.privateEnvironmentProof(f.manifest,f.id,f.pid);
 const saved=JSON.parse(readFileSync(f.envPath));saved.mac='f'.repeat(64);writeFileSync(f.envPath,JSON.stringify(saved));
 assert.throws(()=>f.privateEnvironmentProof(f.manifest,f.id,f.pid));
});
const identity=pid=>{const raw=readFileSync(`/proc/${pid}/stat`,'utf8');return {pid,birth:raw.slice(raw.lastIndexOf(')')+2).split(' ')[19],uid:Number(readFileSync(`/proc/${pid}/status`,'utf8').match(/^Uid:\s+(\d+)/m)[1]),namespaces:Object.fromEntries(['mnt','user','pid'].map(name=>[name,readlinkSync(`/proc/${pid}/ns/${name}`)]))};};
const dead=record=>{try{const raw=readFileSync(`/proc/${record.pid}/stat`,'utf8');return raw.slice(raw.lastIndexOf(')')+2).split(' ')[0]==='Z'||identity(record.pid).birth!==record.birth;}catch{return true;}};
const pause=ms=>new Promise(done=>setTimeout(done,ms));
async function waitFor(fn,budget=90000){const until=Date.now()+budget;while(Date.now()<until){if(fn())return fn();await pause(10);}throw Error('PROOF_DEADLINE');}
test('owned closure requires actual birth exit, close event and pipe closure without touching another child',async t=>{
 const ownedChildStates=new WeakMap(),report={};
 const deps={identity,ownedChildStates,dead,waitFor,report};
 const {trackOwnedChild,closeOwned}=Function(...Object.keys(deps),extract("function trackOwnedChild(child,role='owned') {",'function signedBody(path) {')+extract('async function closeOwned(child) {','async function stopped(launch)')+'\nreturn {trackOwnedChild,closeOwned};')(...Object.values(deps));
 const children=[];t.after(async()=>{for(const child of children){if(child.exitCode===null&&child.signalCode===null)child.kill('SIGKILL');}});
 const create=()=>{const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{env:{PATH:'/usr/bin:/bin'},stdio:['pipe','pipe','pipe']});children.push(child);return child;};
 const owned=create(),other=create();const observed=trackOwnedChild(owned,'test-app-server');const otherBirth=identity(other.pid);
 await closeOwned(owned);assert.equal(observed.closed,true);assert.ok(dead(observed.process));assert.ok(owned.stdio.every(stream=>stream.destroyed));assert.equal(dead(otherBirth),false);
 assert.equal(report.ownedClosures.length,1);assert.equal(report.ownedClosures[0].pipesClosed,true);other.kill('SIGTERM');await new Promise(done=>other.once('close',done));
});
for(const kind of ['authenticated-closed','tampered-receipt','open-pipe','live-owner'])test(`signed project cleanup observer ${kind}`,async t=>{
 const f=observers(t),report={},path=join(f.root,'process-fixture.json'),body={kind:'process-launch',status:'exited',process:{pid:111,birth:'1'},owner:{pid:222,birth:'2'},postSpawnValidated:true,termination:'policy-changed'};
 f.sign(path,body);
 if(kind==='tampered-receipt'){const saved=JSON.parse(readFileSync(path));saved.mac='0'.repeat(64);writeFileSync(path,JSON.stringify(saved));}
 const launch={id:'fixture',path,ended:Promise.resolve(),state:()=>({closed:true}),child:{stdio:[{destroyed:kind!=='open-pipe'}]}};
 const deps={assert,report,signedBody:f.signedBody,dead:record=>kind!=='live-owner'||record.pid!==222,
  waitFor:async fn=>{const value=fn();assert.ok(value,'cleanup predicate not satisfied');return value;},pause:()=>new Promise(()=>{})};
 const stopped=Function(...Object.keys(deps),'let active;\n'+extract('async function stopped(launch)','const negativeKinds=')+'\nreturn stopped;')(...Object.values(deps));
 if(kind!=='authenticated-closed'){await assert.rejects(()=>stopped(launch));assert.equal(report.signedCleanups,undefined);}
 else{await stopped(launch);assert.deepEqual(report.signedCleanups,[{launchId:'fixture',signed:true,status:'exited',termination:'policy-changed',nativeBirthExited:true,supervisorBirthExited:true,pipesClosed:true,postSpawnValidated:true}]);}
});
for(const variant of ['completed','missing-completion','failed-turn','active-after-completion'])test(`split-home fixture materializes persistent original thread: ${variant}`,async()=>{
 let notify,unsubscribed=false,persisted=false;const calls=[],report={};
 const threadId='original-thread',turnId='original-turn';
 const rpc=async(method,params)=>{
  calls.push(method);assert.equal(params.threadId,threadId);
  if(method==='thread/resume'){if(!persisted)throw Object.assign(Error('no rollout found for thread id '+threadId),{code:-32600});return {thread:{id:threadId}};}
  if(method==='turn/start'){
   assert.equal(params.input[0].type,'text');
   if(variant!=='missing-completion'){persisted=true;notify({method:'turn/completed',params:{threadId,turn:{id:turnId,status:variant==='failed-turn'?'failed':'completed'}}});}
   return {turn:{id:turnId}};
  }
  assert.equal(method,'thread/read');assert.equal(params.includeTurns,false);
  return {thread:{id:threadId,status:{type:variant==='active-after-completion'?'active':'idle'}}};
 };
 const deps={assert,runtime:{rpc:{onNotification:callback=>{notify=callback;return ()=>{unsubscribed=true;};}}},rpc,
  waitFor:async(fn,budget)=>{assert.equal(budget,60000);if(!fn())throw Error('PROOF_DEADLINE');},providerFailure:undefined,report};
 const materialize=Function(...Object.keys(deps),"let modelPhase='broad';\n"+extract('async function materializeNativeThread(threadId) {','async function proveSplitHomeReplacement() {')+'\nreturn materializeNativeThread;')(...Object.values(deps));
 await assert.rejects(()=>rpc('thread/resume',{threadId}),{code:-32600});
 if(variant==='completed'){await materialize(threadId);assert.deepEqual(await rpc('thread/resume',{threadId}),{thread:{id:threadId}});assert.equal(report.splitThreadPersistence.sameThread,true);assert.equal(report.splitThreadPersistence.completedUserTurn,true);}
 else{await assert.rejects(()=>materialize(threadId));assert.equal(report.splitThreadPersistence,undefined);}
 assert.equal(unsubscribed,true);assert.equal(calls[1],'turn/start');
});
