import assert from 'node:assert/strict';
import {spawn,spawnSync} from 'node:child_process';
import {createHash,createHmac,randomUUID} from 'node:crypto';
import {constants,openSync,closeSync,renameSync,mkdtempSync,mkdirSync,writeFileSync,readFileSync,readlinkSync,rmSync,chmodSync,symlinkSync,existsSync,copyFileSync,realpathSync,statSync,readdirSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {dirname,join,resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import test from 'node:test';
import {controlledObservation,quiesceNative} from '../scripts/live-host-renewal-observation.mjs';
const source=readFileSync(resolve('public/gestalt'),'utf8');
const program=`process.env.GESTALT_LIVE_MANAGER_SOURCE=${JSON.stringify(resolve('public/gestalt'))};\n`+source.split("<<'LIVE_POLICY'\n")[1].split('\nLIVE_POLICY\n')[0];
const hash=value=>createHash('sha256').update(value).digest('hex');
test('renewal Native observation derives account absence only from explicit supported RPC fields',()=>{
 const config={config:{model_providers:{controlled:{requires_openai_auth:false}}}};
 const reply={model:'gpt-6-sol',modelProvider:'controlled',cwd:'/app',approvalPolicy:'never',activePermissionProfile:{id:'proof'},sandbox:{},reasoningEffort:'medium'};
 const result=controlledObservation(config,reply,{account:null,requiresOpenaiAuth:false,workspaceRouting:null});
 assert.equal(result.accountDigest,hash(renewalStable({providerId:'controlled',accountId:null})));assert.deepEqual(result.sessionFlags,reply);
 for(const account of [{},{account:null},{account:null,requiresOpenaiAuth:true},{account:{type:'apiKey'},requiresOpenaiAuth:false},{account:null,requiresOpenaiAuth:false,unknown:null},{account:null,requiresOpenaiAuth:false,workspaceRouting:{account:'unknown'}}])assert.throws(()=>controlledObservation(config,reply,account));
 assert.throws(()=>controlledObservation({config:{model_providers:{controlled:{requires_openai_auth:true}}}},reply,{account:null,requiresOpenaiAuth:false}));
});
test('renewal Native quiescence checks turns, terminals and unload rather than alive PID',async()=>{
 const calls=[];const request=async(method,params)=>{calls.push(method);if(method==='thread/loaded/list')return {data:calls.filter(v=>v==='thread/loaded/list').length===1?['root','child']:[],nextCursor:null};if(method==='thread/read')return {thread:{id:params.threadId,status:{type:'idle'},turns:[]}};if(method==='thread/backgroundTerminals/list')return {data:[],nextCursor:null};if(method==='thread/unsubscribe')return {status:'unsubscribed'};throw Error('unknown');};
 const result=await quiesceNative(request,['root']);assert.equal(result.observed.length,2);assert.equal(result.unsubscribed,true);assert.equal(calls.filter(v=>v==='thread/read').length,2);assert.equal(calls.filter(v=>v==='thread/backgroundTerminals/list').length,2);
});
for(const failure of ['active-status','active-turn','terminal','pagination','unknown-status','subscription-unknown'])test('renewal Native quiescence refuses '+failure,async()=>{
 const request=async(method,params)=>{if(method==='thread/loaded/list')return {data:['root'],nextCursor:failure==='pagination'?'unobserved':null};if(method==='thread/read')return {thread:{id:params.threadId,status:{type:failure==='active-status'?'active':failure==='unknown-status'?'unknown':'idle'},turns:failure==='active-turn'?[{status:'inProgress'}]:[]}};if(method==='thread/backgroundTerminals/list')return {data:failure==='terminal'?[{}]:[],nextCursor:null};if(method==='thread/unsubscribe')return {status:failure==='subscription-unknown'?'unknown':'unsubscribed'};};
 await assert.rejects(quiesceNative(request,['root']));
});
function identity(pid) {
 const raw=readFileSync(`/proc/${pid}/stat`,'utf8');
 return {pid,birth:raw.slice(raw.lastIndexOf(')')+2).split(' ')[19],uid:process.getuid(),
  namespaces:Object.fromEntries(['mnt','user','pid'].map(key=>[key,readlinkSync(`/proc/${pid}/ns/${key}`)]))};
}
function fixture() {
 const root=mkdtempSync(join(tmpdir(),'live policy ')),app=join(root,'app with spaces'),privateRoot=join(root,'controller'),home=join(privateRoot,'home');
 for(const path of [app,privateRoot,home,join(root,'auth'),join(root,'caddy'),join(root,`daemon-${process.getuid()}`)]) mkdirSync(path,{mode:0o700});
 const managerPath=join(root,'gestalt manager');writeFileSync(managerPath,source,{mode:0o700});
 const bin=join(root,'codex');writeFileSync(bin,'#!/bin/sh\nprintf "codex-cli 0.161.0\\n"\n',{mode:0o700});
 const config=join(home,'config.toml');writeFileSync(config,'[shell_environment_policy]\ninherit = "none"\n',{mode:0o600});
 const key=join(privateRoot,'key');writeFileSync(key,Buffer.alloc(32,1),{mode:0o600});
 const manifest=join(privateRoot,'manifest.json');
 const m={schemaVersion:1,profileId:'trusted-live',appRoot:app,controller:identity(process.pid),binary:{path:bin,version:'0.161.0',sha256:hash(readFileSync(bin))},
  sandboxState:{permissionProfile:{type:'managed',file_system:{type:'restricted',entries:[{path:{type:'special',value:{kind:'root'}},access:'read'},{path:{type:'path',path:app},access:'write'}]},network:'restricted'},sandboxCwd:pathToFileURL(app).href,useLegacyLandlock:false},
  configFiles:[{path:config,sha256:hash(readFileSync(config))}],components:{manager:'0.7.1',mobile:'test-pin',agents:'test-pin'},
  privatePaths:{auth:[join(root,'auth')],controller:[privateRoot],caddy:[join(root,'caddy')],daemon:[join(root,`daemon-${process.getuid()}`)]},
  runtimeHome:home,keyFile:key,environment:{PATH:'/usr/bin:/bin',HOME:home,TMPDIR:'/tmp'}};
 const save=()=>writeFileSync(manifest,JSON.stringify(m),{mode:0o600});save();
 // Only the host-context directory observation is replaced in these offline
 // contract fixtures. This does not claim native enforcement or host proof.
 const fixtureProgram=program.replace(JSON.stringify(resolve('public/gestalt')),JSON.stringify(managerPath)).replaceAll('/tmp/codex-daemon-',join(root,'daemon-'))
  .replaceAll("if (ancestor === '/') break;",`if (ancestor === ${JSON.stringify(root)}) break;`);
 const run=(action='probe',extra=[])=>{save();const result=spawnSync(process.execPath,['--input-type=module','-',action,'--manifest',manifest,...extra],
  {input:fixtureProgram,encoding:'utf8',env:{...process.env,OPENAI_API_KEY:'fixture-secret',NODE_OPTIONS:''}});
  assert.equal(result.stderr,'');assert.ok(!result.stdout.includes('fixture-secret'));return {status:result.status,...JSON.parse(result.stdout)};};
 return {root,app,privateRoot,home,manifest,m,managerPath,save,run,program:fixtureProgram,cleanup:()=>{chmodSync(join(root,`daemon-${process.getuid()}`),0o700);rmSync(root,{recursive:true,force:true});}};
}
function check(name,fn) {test(name,()=>{const f=fixture();try {fn(f);}finally {f.cleanup();}});}
check('spaces: bounded probe is read-only; private signed preparation and verification are explicit',f=>{
 assert.equal(f.run().available,true);assert.equal(f.run().liveReady,false);assert.equal(existsSync(join(f.privateRoot,'policy-receipt.json')),false);
 assert.equal(f.run('prepare').status,0);assert.equal(f.run('verify').status,0);
 const saved=JSON.parse(readFileSync(join(f.privateRoot,'policy-receipt.json')));
 assert.equal(saved.body.kind,'policy-contract');assert.deepEqual(saved.body.launch.args.slice(0,2),['sandbox','--sandbox-state-json']);
 assert.equal(saved.body.launch.extraFileDescriptors,false);assert.equal(saved.body.launch.lifetime,'per-call');
 assert.deepEqual(Object.keys(saved.body.launch.env).sort(),['CODEX_HOME','HOME','LANG','PATH','TMPDIR']);
 assert.equal(saved.body.app.path,f.app);assert.equal(saved.body.controller.birth,identity(process.pid).birth);
});
check('normalized private denial covers nested paths and removes more-specific grants without broadening unrelated policy',f=>{
 const child=join(f.m.privatePaths.auth[0],'child');mkdirSync(child,{mode:0o700});
 const secret=join(f.privateRoot,'nested secret');writeFileSync(secret,'private fixture');
 f.m.privatePaths.auth.push(secret);f.m.privatePaths.caddy.push(secret);
 f.m.sandboxState.permissionProfile.file_system.entries.push({path:{type:'path',path:child},access:'write'},{path:{type:'path',path:secret},access:'read'},{path:{type:'path',path:secret},access:'deny'});
 const unrelated=structuredClone(f.m.sandboxState.permissionProfile.file_system.entries.slice(0,2));assert.equal(f.run('prepare').status,0);
 const entries=JSON.parse(readFileSync(join(f.privateRoot,'policy-receipt.json'))).body.sandboxState.permissionProfile.file_system.entries;
 assert.deepEqual(entries.slice(0,2),unrelated);
 const denies=entries.filter(entry=>entry.access==='deny').map(entry=>entry.path.path);
 const within=(parent,path)=>path===parent||path.startsWith(parent+'/');
 for(const path of [...Object.values(f.m.privatePaths).flat(),child,secret,'/proc']){
  assert.ok(denies.some(parent=>within(parent,path)));
  const matches=entries.filter(entry=>entry.path.path&&within(entry.path.path,path)).sort((a,b)=>b.path.path.length-a.path.path.length);
  assert.equal(matches[0].access,'deny');
 }
 assert.ok(denies.every(path=>!denies.some(parent=>parent!==path&&within(parent,path))));
 assert.ok(!entries.some(entry=>entry.path.path===child||entry.path.path===secret));
});
for(const [name,change,reason] of [
 ['missing profile',f=>delete f.m.profileId,'incompatible-manifest'],
 ['disabled profile',f=>f.m.sandboxState.permissionProfile.type='disabled','incompatible-profile'],
 ['external profile',f=>f.m.sandboxState.permissionProfile.type='external','incompatible-profile'],
 ['unrestricted files',f=>f.m.sandboxState.permissionProfile.file_system.type='unrestricted','incompatible-profile'],
 ['network broadening',f=>f.m.sandboxState.permissionProfile.network='enabled','incompatible-profile'],
 ['arbitrary sandbox executable',f=>f.m.sandboxState.codexLinuxSandboxExe=f.m.binary.path,'incompatible-profile'],
 ['legacy sandbox',f=>f.m.sandboxState.useLegacyLandlock=true,'incompatible-profile'],
 ['unknown state',f=>f.m.sandboxState.extra=true,'incompatible-profile'],
 ['config precedence override',f=>f.m.sandboxState.permissionProfile.default_permissions='unrestricted','incompatible-profile'],
 ['writable controller ancestor',f=>f.m.sandboxState.permissionProfile.file_system.entries.push({path:{type:'path',path:f.root},access:'write'}),'project-writable-policy'],
 ['writable tmp authority',f=>f.m.sandboxState.permissionProfile.file_system.entries.push({path:{type:'special',value:{kind:'slash_tmp'}},access:'write'}),'project-writable-policy'],
 ['stale birth',f=>f.m.controller.birth='0','stale-controller'],
 ['namespace mismatch',f=>f.m.controller.namespaces.mnt='mnt:[0]','stale-controller'],
 ['missing daemon exclusion',f=>f.m.privatePaths.daemon=[],'missing-private-paths'],
 ['FD injection',f=>f.m.extraFileDescriptors=[3],'incompatible-manifest'],
 ['secret forwarding',f=>f.m.environment.OPENAI_API_KEY='fixture-secret','unsafe-environment'],
 ['unsafe search path',f=>f.m.environment.PATH=f.app,'unsafe-environment'],
 ['wrong native version',f=>f.m.binary.version='0.160.0','incompatible-binary'],
 ['tampered binary',f=>writeFileSync(f.m.binary.path,'changed',{mode:0o700}),'binary-changed'],
 ['missing config',f=>f.m.configFiles=[],'missing-config'],
 ['tampered config',f=>writeFileSync(join(f.home,'config.toml'),'hooks = "unsafe"\n',{mode:0o600}),'unsafe-runtime-config'],
 ['project hook/config',f=>mkdirSync(join(f.app,'.codex')),'project-config-present'],
 ['public manifest',f=>chmodSync(f.manifest,0o644),'untrusted-storage'],
 ['missing binary',f=>f.m.binary.path=join(f.root,'missing'),'missing-prerequisite'],
])check(`${name} fails closed with bounded reason`,f=>{change(f);const result=f.run();assert.equal(result.available,false);assert.equal(result.reason,reason);assert.equal(result.liveReady,false);assert.equal(result.status,1);});
check('symlinked app and binary are rejected',f=>{
 const alias=join(f.root,'alias');symlinkSync(f.app,alias);f.m.appRoot=alias;assert.equal(f.run().reason,'symlink-path');f.m.appRoot=f.app;
 const binaryAlias=join(f.root,'codex alias');symlinkSync(f.m.binary.path,binaryAlias);f.m.binary.path=binaryAlias;assert.equal(f.run().reason,'symlink-path');
});
check('tampered receipt MAC and policy cannot be reused',f=>{
 f.run('prepare');const path=join(f.privateRoot,'policy-receipt.json'),saved=JSON.parse(readFileSync(path));
 saved.body.sandboxState.permissionProfile.file_system.entries=[];writeFileSync(path,JSON.stringify(saved));assert.equal(f.run('verify').reason,'tampered-receipt');
 f.run('prepare');const valid=JSON.parse(readFileSync(path));valid.mac='0'.repeat(64);writeFileSync(path,JSON.stringify(valid));assert.equal(f.run('verify').reason,'tampered-receipt');
});
for(const [name,change] of [
 ['permissions',f=>f.m.sandboxState.permissionProfile.file_system.entries[1].access='read'],
 ['profile identity',f=>f.m.profileId='narrower'],
 ['component',f=>f.m.components.mobile='next-pin'],
 ['config set',f=>{const path=join(f.privateRoot,'effective-config');writeFileSync(path,'new',{mode:0o600});f.m.configFiles.push({path,sha256:hash('new')});}],
])check(`${name} change invalidates prior receipt`,f=>{assert.equal(f.run('prepare').status,0);change(f);assert.equal(f.run('verify').reason,'stale-receipt');});
check('masked native daemon directory is never repaired and cannot prepare receipts',f=>{
 chmodSync(f.m.privatePaths.daemon[0],0);assert.equal(f.run().reason,'host-launch-unavailable');assert.equal(existsSync(join(f.privateRoot,'policy-receipt.json')),false);
});
test('real CLI missing private manifest returns bounded unavailable JSON without launching Live',()=>{
 const result=spawnSync('bash',['public/gestalt','live-policy','probe','--manifest','/nonexistent-live-policy'],{encoding:'utf8'});
 assert.equal(result.status,1);const report=JSON.parse(result.stdout);assert.equal(report.reason,'missing-prerequisite');assert.equal(report.liveReady,false);
});
check('relative-context special grants require trusted materialization',f=>{
 for(const kind of ['project_roots','tmpdir']) {
  const entry={path:{type:'special',value:{kind}},access:'write'};
  f.m.sandboxState.permissionProfile.file_system.entries.push(entry);
  assert.equal(f.run().reason,'incompatible-profile');f.m.sandboxState.permissionProfile.file_system.entries.pop();
 }
});
check('private child names beginning with two dots cannot evade ancestry checks',f=>{
 const path=join(f.app,'..private');mkdirSync(path,{mode:0o700});f.m.privatePaths.auth=[path];
 // This root is a private descendant of writable app. Composition retains the
 // write entry and adds the specific denial; it never grants the child access.
 assert.equal(f.run('prepare').status,0);
 const saved=JSON.parse(readFileSync(join(f.privateRoot,'policy-receipt.json')));
 assert.ok(saved.body.sandboxState.permissionProfile.file_system.entries.some(e=>e.path.path===path&&e.access==='deny'));
});
check('canonical state cwd cannot hide a symlink alias',f=>{
 const path=join(f.root,'alias');symlinkSync(f.app,path);f.m.sandboxState.sandboxCwd=pathToFileURL(path).href;
 assert.equal(f.run().reason,'workspace-mismatch');
});
check('app inode replacement invalidates policy receipt',f=>{
 assert.equal(f.run('prepare').status,0);
 const original=f.app+' original';
 // Keep old inode allocated while replacing its canonical path.
 const result=spawnSync('/bin/mv',[f.app,original]);assert.equal(result.status,0);mkdirSync(f.app,{mode:0o700});
 assert.equal(f.run('verify').reason,'stale-receipt');
});
check('true namespace mismatch is explicitly unsupported',f=>{
 const changedProgram=program.replaceAll('/tmp/codex-daemon-',join(f.root,'daemon-'))
  .replace('const current = identity(process.pid);',"const current = identity(process.pid); current.namespaces.mnt = 'mnt:[unsupported]';");
 const result=spawnSync(process.execPath,['--input-type=module','-','probe','--manifest',f.manifest],{input:changedProgram,encoding:'utf8'});
 assert.equal(JSON.parse(result.stdout).reason,'unsupported-host-namespace');
});
check('nonexistent PID is diagnosed as stale rather than trusted by its receipt',f=>{
 f.m.controller.pid=2147483647;assert.equal(f.run().reason,'stale-process');
});
check('binary diagnosis receives isolated environment, never controller credentials',f=>{
 const log=join(f.root,'version environment');
 writeFileSync(f.m.binary.path,`#!/bin/sh\n/usr/bin/env > '${log}'\nprintf "codex-cli 0.161.0\\n"\n`,{mode:0o700});
 f.m.binary.sha256=hash(readFileSync(f.m.binary.path));assert.equal(f.run().available,true);
 const env=readFileSync(log,'utf8');assert.ok(!env.includes('fixture-secret'));assert.ok(!env.includes('OPENAI_API_KEY'));
 assert.match(env,/CODEX_HOME=/);assert.match(env,/PATH=\/usr\/bin:\/bin/);
});
for(const field of ['manager','mobile','agents'])check(`missing required component ${field} fails closed`,f=>{
 delete f.m.components[field];assert.equal(f.run().reason,'missing-components');
});
check('empty components record fails closed',f=>{f.m.components={};assert.equal(f.run().reason,'missing-components');});
for(const field of ['auth','controller','caddy','daemon'])check(`missing private path category ${field} fails closed`,f=>{
 delete f.m.privatePaths[field];assert.equal(f.run().reason,'missing-private-paths');
});
for(const field of ['permissionProfile','sandboxCwd','useLegacyLandlock'])check(`missing required native state ${field} fails closed`,f=>{
 delete f.m.sandboxState[field];assert.equal(f.run().reason,'incompatible-profile');
});
for(const field of ['type','file_system','network'])check(`missing permission profile ${field} fails closed`,f=>{
 delete f.m.sandboxState.permissionProfile[field];assert.equal(f.run().reason,'incompatible-profile');
});
for(const field of ['type','entries'])check(`missing filesystem record ${field} fails closed`,f=>{
 delete f.m.sandboxState.permissionProfile.file_system[field];assert.equal(f.run().reason,'incompatible-profile');
});
for(const field of ['path','access'])check(`missing filesystem entry ${field} fails closed`,f=>{
 delete f.m.sandboxState.permissionProfile.file_system.entries[1][field];assert.equal(f.run().reason,'incompatible-profile');
});
for(const field of ['path','version','sha256'])check(`missing binary record ${field} fails closed`,f=>{
 delete f.m.binary[field];assert.equal(f.run().reason,'incompatible-binary');
});
for(const field of ['path','sha256'])check(`missing config record ${field} fails closed`,f=>{
 delete f.m.configFiles[0][field];assert.equal(f.run().reason,'invalid-config');
});
for(const field of ['PATH','HOME','TMPDIR'])check(`missing environment record ${field} fails closed`,f=>{
 delete f.m.environment[field];assert.equal(f.run().reason,'unsafe-environment');
});
for(const field of ['pid','birth','uid','namespaces'])check(`missing controller identity ${field} fails closed`,f=>{
 delete f.m.controller[field];assert.equal(f.run().available,false);
});
check('only documented native state fields are optional',f=>{
 assert.equal(f.run().available,true);
 f.m.sandboxState.useMxc=false;f.m.sandboxState.codexLinuxSandboxExe=null;assert.equal(f.run().available,true);
});
for(const field of ['body','mac'])check(`missing receipt record ${field} fails closed`,f=>{
 assert.equal(f.run('prepare').status,0);const path=join(f.privateRoot,'policy-receipt.json');
 const saved=JSON.parse(readFileSync(path));delete saved[field];writeFileSync(path,JSON.stringify(saved));assert.equal(f.run('verify').available,false);
});
for(const field of ['schemaVersion','profileId','appRoot','controller','binary','sandboxState','configFiles','components','privatePaths','keyFile','runtimeHome','environment'])check(`missing top-level required manifest ${field} fails closed`,f=>{
 delete f.m[field];assert.equal(f.run().reason,'incompatible-manifest');
});
for(const field of ['mnt','user','pid'])check(`missing controller namespace ${field} fails closed`,f=>{
 delete f.m.controller.namespaces[field];assert.equal(f.run().reason,'invalid-controller');
});
for(const variant of ['writable','symlink'])check(`admitted command ${variant} is rejected without loosening executable trust`,f=>{
 const executable=join(f.root,'command');copyFileSync(realpathSync(process.execPath),executable);chmodSync(executable,variant==='writable'?0o770:0o700);
 const alias=join(f.root,'command alias');if(variant==='symlink')symlinkSync(executable,alias);
 f.m.commands={probe:{executable:variant==='symlink'?alias:executable,sha256:hash(readFileSync(executable)),args:[],timeoutMs:1000}};
 const result=f.run('prepare');assert.equal(result.available,false);assert.equal(result.reason,variant==='writable'?'command-changed':'symlink-path');
});
const pause=ms=>new Promise(resolvePause=>setTimeout(resolvePause,ms));
async function waitFor(fn) {for(let n=0;n<100;n++){try {const value=fn();if(value)return value;}catch{}await pause(50);}assert.fail('fixture deadline');}
function launchFixture(f,code,fullCLI=false,inputChunks=[],keepInputOpen=false) {
 if(fullCLI){const parts=source.split("<<'LIVE_POLICY'\n");const tail=parts[1].split("\nLIVE_POLICY\n")[1];writeFileSync(f.managerPath,parts[0]+"<<'LIVE_POLICY'\n"+f.program+"\nLIVE_POLICY\n"+tail,{mode:0o700});}
 const interpreter=join(f.root,'admitted node');copyFileSync(realpathSync(process.execPath),interpreter);chmodSync(interpreter,0o700);
 const wrapper=join(f.root,'native stand-in.cjs');
 writeFileSync(wrapper,`const {spawn}=require('node:child_process');const a=process.argv.slice(2);if(a[0]==='--version'){console.log('codex-cli 0.161.0');process.exit(0);}const i=a.indexOf('--');const c=spawn(a[i+1],a.slice(i+2),{stdio:'inherit',env:process.env});c.on('exit',x=>process.exit(x??1));\n`,{mode:0o600});
 writeFileSync(f.m.binary.path,`#!/bin/sh\nexec "${interpreter}" "${wrapper}" "$@"\n`,{mode:0o700});
 f.m.binary.sha256=hash(readFileSync(f.m.binary.path));f.m.commands={probe:{executable:interpreter,sha256:hash(readFileSync(interpreter)),args:['-e',code],timeoutMs:30000}};
 const prepared=f.run('prepare');assert.equal(prepared.status,0,JSON.stringify(prepared));
 const id='00000000-0000-4000-8000-000000000001';
 const argumentsTail=['run','--manifest',f.manifest,'--command','probe','--launch-id',id];
 const child=spawn(fullCLI?'/usr/bin/bash':process.execPath,fullCLI?[f.managerPath,'live-policy',...argumentsTail]:['--input-type=module','-',...argumentsTail],{stdio:['pipe','pipe','pipe','pipe'],env:fullCLI?{PATH:`${resolve(process.execPath,'..')}:/usr/bin:/bin`,HOME:f.privateRoot,CODEX_HOME:f.home,TMPDIR:'/tmp',LANG:'C.UTF-8'}:{...process.env,OPENAI_API_KEY:'fixture-secret'}});
 let out='',err='';child.stdout.on('data',data=>out+=data);child.stderr.on('data',data=>err+=data);
 child.once('exit',()=>child.stdio[3].destroy());
 const ended=new Promise(resolveExit=>child.on('close',code=>resolveExit(code)));
 if(fullCLI&&(inputChunks.length||keepInputOpen)){void(async()=>{if(inputChunks.length)await waitFor(()=>out.includes('filtered-input-ready'));for(const chunk of inputChunks){child.stdin.write(chunk);await pause(50);}if(!keepInputOpen)child.stdin.end();})();}else child.stdin.end(fullCLI?undefined:f.program);child.stdio[3].end();
 const receiptPath=join(f.privateRoot,`process-${id}.json`);
 const control=op=>{const result=spawnSync(process.execPath,['--input-type=module','-',op,'--manifest',f.manifest,'--launch-id',id],{input:f.program,encoding:'utf8'});return {status:result.status,...JSON.parse(result.stdout)};};
 return {child,id,receiptPath,control,ended,output:()=>out,error:()=>err};
}
const running=()=>"process.stdout.write('ready\\n');setInterval(()=>{},1000)";
for(const reason of ['stop','policy-change','owner-kill'])test(`offline supervisor ${reason} revokes and records the owned native launch`,async()=>{
 const f=fixture();let launch;try {
  launch=launchFixture(f,running());await waitFor(()=>launch.output().includes('ready'));
  const saved=await waitFor(()=>{const r=JSON.parse(readFileSync(launch.receiptPath));return r.body.status==='running'&&r;});
  assert.equal(saved.body.kind,'process-launch');assert.equal(saved.body.process.pid,saved.body.processGroup);
  assert.equal(launch.control('verify-process').reason,'process-verified');
  assert.equal(f.run('prepare').reason,'active-launch');
  if(reason==='stop')assert.equal(launch.control('stop').reason,'stop-requested');
  if(reason==='policy-change'){f.m.sandboxState.permissionProfile.file_system.entries[1].access='read';f.save();}
  if(reason==='owner-kill')launch.child.kill('SIGKILL');
  await launch.ended;
  await waitFor(()=>JSON.parse(readFileSync(launch.receiptPath)).body.status==='exited');
  assert.equal(launch.control('verify-process').available,false);assert.equal(existsSync(join(f.privateRoot,'dispatch.lock')),false);
  assert.equal(f.run('prepare').status,0);
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
test('offline leader exit reaps a resistant descendant and preserves an unrelated process',async()=>{
 const f=fixture();let launch;const unowned=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore'});
 try {
  launch=launchFixture(f,"const {spawn}=require('node:child_process');const c=spawn(process.execPath,['-e',\"process.on('SIGTERM',()=>{});console.log('descendant-ready '+process.pid);setInterval(()=>{},1000)\"],{stdio:'inherit'});setTimeout(()=>process.exit(0),300);");
  await waitFor(()=>launch.output().includes('descendant-ready'));const pid=Number(launch.output().match(/descendant-ready (\d+)/)[1]);
  assert.equal(await launch.ended,0);
  await waitFor(()=>{try{return readFileSync(`/proc/${pid}/stat`,'utf8').split(' ')[2]==='Z';}catch(e){return e.code==='ENOENT';}});
  process.kill(unowned.pid,0);assert.equal(JSON.parse(readFileSync(launch.receiptPath)).body.termination,'leader-exit');
 }finally{unowned.kill('SIGTERM');if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
check('unadmitted command is refused without a launch receipt or active lock',f=>{
 assert.equal(f.run('prepare').status,0);
 const result=spawnSync(process.execPath,['--input-type=module','-','run','--manifest',f.manifest,'--command','unapproved','--launch-id','00000000-0000-4000-8000-000000000001'],{input:f.program,encoding:'utf8'});
 assert.equal(JSON.parse(result.stdout).reason,'unadmitted-command');assert.equal(existsSync(join(f.privateRoot,'dispatch.lock')),false);
});
test('offline failed native launch rolls back owned lock and permits explicit retry',async()=>{
 const f=fixture();let launch;try {
  launch=launchFixture(f,"setTimeout(()=>process.exit(23),150)");assert.equal(await launch.ended,23);
  assert.equal(existsSync(join(f.privateRoot,'dispatch.lock')),false);assert.equal(f.run('prepare').status,0);
  assert.equal(launch.control('verify-process').available,false);
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
test('tampered process receipt cannot signal an unrelated process',async()=>{
 const f=fixture();let launch;const unowned=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore'});
 try {
  launch=launchFixture(f,running());await waitFor(()=>launch.output().includes('ready'));
  const original=readFileSync(launch.receiptPath),saved=JSON.parse(original);saved.body.owner.pid=unowned.pid;writeFileSync(launch.receiptPath,JSON.stringify(saved));
  assert.equal(launch.control('stop').reason,'tampered-receipt');process.kill(unowned.pid,0);
  writeFileSync(launch.receiptPath,original);assert.equal(launch.control('stop').reason,'stop-requested');await launch.ended;process.kill(unowned.pid,0);
 }finally{unowned.kill('SIGTERM');if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
check('project-writable measured manager is unavailable',f=>{
 f.m.sandboxState.permissionProfile.file_system.entries.push({path:{type:'path',path:f.managerPath},access:'write'});
 assert.equal(f.run().reason,'untrusted-manager');
});
test('installed manager replacement revokes a running launch even without a manifest edit',async()=>{
 const f=fixture();let launch;try {
  launch=launchFixture(f,running());await waitFor(()=>launch.output().includes('ready'));
  writeFileSync(f.managerPath,source+'\n# fixture manager update\n',{mode:0o700});
  await launch.ended;assert.equal(JSON.parse(readFileSync(launch.receiptPath)).body.termination,'policy-changed');
  assert.equal(f.run('verify').reason,'stale-receipt');
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
test('a valid signed receipt from another boot cannot authorize Stop',async()=>{
 const f=fixture();let launch;try {
  launch=launchFixture(f,running());await waitFor(()=>launch.output().includes('ready'));
  const original=readFileSync(launch.receiptPath),saved=JSON.parse(original);saved.body.policy.bootId='previous-boot';
  const canonical=value=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
  saved.mac=createHmac('sha256',readFileSync(f.m.keyFile)).update(canonical(saved.body)).digest('hex');writeFileSync(launch.receiptPath,JSON.stringify(saved));
  assert.equal(launch.control('stop').reason,'stale-process');assert.equal(launch.child.exitCode,null);
  writeFileSync(launch.receiptPath,original);assert.equal(launch.control('stop').reason,'stop-requested');await launch.ended;
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
test('owner reconciles an abruptly killed supervisor before releasing admission',async()=>{
 const f=fixture();let launch;try {
  launch=launchFixture(f,running());await waitFor(()=>launch.output().includes('ready'));
  const saved=JSON.parse(readFileSync(launch.receiptPath));process.kill(saved.body.owner.pid,'SIGKILL');
  await launch.ended;const final=JSON.parse(readFileSync(launch.receiptPath)).body;
  assert.equal(final.status,'exited');assert.equal(final.termination,'supervisor-exit');assert.equal(existsSync(join(f.privateRoot,'dispatch.lock')),false);
  assert.equal(f.run('prepare').status,0);
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
for(const name of ['auth.json','AGENTS.md','AGENTS.override.md','hooks.json','hooks','plugins','skills','.codex'])check(`prepared home rejects injected ${name}`,f=>{
 writeFileSync(join(f.home,name),'fixture injection',{mode:0o600});assert.equal(f.run().reason,'unsafe-runtime-home');
});

test('hosted native panic diagnostics classify package failure without exposing arbitrary stderr',()=>{
 const proof=readFileSync(resolve('scripts/live-host-policy-native-proof.mjs'),'utf8');
 const expression=proof.split('const nativeDiagnostic=')[1].split('\nconst managerResultDiagnostic=')[0];
 const diagnose=Function(`return ${expression}`)();
 const stderr="thread 'fixture' panicked at secret/controller/launcher.rs:57:13: bubblewrap is unavailable; fixture-secret";
 const result=diagnose(stderr);assert.equal(result.rustPanic,true);assert.equal(result.bubblewrapUnavailable,true);
 const node=diagnose('TypeError [ERR_INVALID_FD_TYPE]: fixture-secret');assert.deepEqual(node.nodeCodes,['ERR_INVALID_FD_TYPE']);assert.deepEqual(node.nodeErrors,['TypeError']);assert.ok(!JSON.stringify(node).includes('fixture-secret'));
 const role=diagnose("bwrap: Can't create file at /private/host policy abc/controller/auth: Read-only file system",{policyDirectory:'/private/host policy abc/controller',authentication:'/private/host policy abc/controller/auth'});assert.equal(role.createFileFailureRole,'authentication');assert.ok(!JSON.stringify(role).includes('host policy abc'));
 const template=diagnose('error building bubblewrap command: open /private/fixture-secret: \"fixture-secret\"');assert.ok(!JSON.stringify(template).includes('fixture-secret'));assert.ok(!JSON.stringify(template).includes('/private'));
 assert.deepEqual(result.sourceLocations,['launcher.rs:57:13']);assert.ok(!JSON.stringify(result).includes('fixture-secret'));assert.ok(!JSON.stringify(result).includes('secret/controller'));
});

check('full CLI with cleared environment rejects equal HOME and CODEX_HOME before dispatch; separated homes return bounded policy JSON',f=>{
 const env={PATH:`${resolve(process.execPath,'..')}:/usr/bin:/bin`,HOME:f.home,CODEX_HOME:f.home,TMPDIR:'/tmp',LANG:'C.UTF-8'};
 const args=[f.managerPath,'live-policy','prepare','--manifest',f.manifest];
 const refused=spawnSync('/usr/bin/bash',args,{cwd:f.app,env,encoding:'utf8'});
 assert.equal(refused.status,1);assert.equal(refused.stdout,'');assert.match(refused.stderr,/refusing to use HOME itself as CODEX_HOME/);
 const dispatched=spawnSync('/usr/bin/bash',args,{cwd:f.app,env:{...env,HOME:f.privateRoot},encoding:'utf8'});
 const body=JSON.parse(dispatched.stdout);assert.equal(body.schemaVersion,1);assert.equal(body.liveReady,false);assert.equal(typeof body.reason,'string');
 assert.ok(!dispatched.stderr.includes('refusing to use HOME itself'));assert.ok(!dispatched.stdout.includes('fixture-secret'));
});
test('manager pre-parse boundary reports empty/malformed output and fixed error categories without stderr text',()=>{
 const proof=readFileSync(resolve('scripts/live-host-policy-native-proof.mjs'),'utf8');
 const expression=proof.split('const managerResultDiagnostic=result=>')[1].split('\nconst tree=')[0];
 const diagnose=Function(`return result=>${expression}`)();
 for(const stdout of ['', 'fixture-secret', '[]']){
  const result=diagnose({code:1,signal:null,stdout,stderr:'gestalt: error: refusing to use HOME itself as CODEX_HOME; fixture-secret'});
  assert.equal(result.sameHomeRefused,true);assert.equal(result.stdoutBytes,Buffer.byteLength(stdout));
  assert.equal(result.json,stdout==='[]'?'invalid-shape':'invalid-json');assert.ok(!JSON.stringify(result).includes('fixture-secret'));
 }
 const valid=diagnose({code:1,stdout:'{"schemaVersion":1,"available":false,"reason":"unsupported-host-context"}',stderr:''});
 assert.equal(valid.json,'valid-object');assert.equal(valid.reason,'unsupported-host-context');
 const missing=diagnose({code:null,signal:null,errorCode:'ENOENT',stdout:'',stderr:''});assert.equal(missing.spawnError,'ENOENT');assert.equal(missing.stdoutBytes,0);assert.equal(missing.json,'invalid-json');
});

test('full Bash CLI dispatch preserves pipe stdin and privately supervises an admitted offline child',async()=>{
 const f=fixture();let launch;try{
  launch=launchFixture(f,running(),true,[],true);await waitFor(()=>launch.output().includes('ready'));
  assert.equal(launch.control('verify-process').reason,'process-verified');assert.equal(launch.control('stop').reason,'stop-requested');
  await launch.ended;assert.equal(JSON.parse(readFileSync(launch.receiptPath)).body.status,'exited');assert.equal(launch.error(),'');
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});


test('post-spawn private-home injection fails closed with a signed bounded reason and owned cleanup',async()=>{
 const f=fixture();let launch;try{
  f.program=f.program.replace("phase='post-spawn-contract';", "phase='post-spawn-contract';writeFileSync(join(root,'home','hooks.json'),'fixture-secret');");
  launch=launchFixture(f,running());await launch.ended;
  const body=JSON.parse(readFileSync(launch.receiptPath)).body;
  assert.equal(body.status,'exited');assert.equal(body.termination,'launch-failed');
  assert.equal(body.failure.phase,'post-spawn-contract');assert.equal(body.failure.reason,'unsafe-runtime-home');
  assert.ok(!JSON.stringify(body.failure).includes('fixture-secret'));assert.equal(existsSync(join(f.privateRoot,'dispatch.lock')),false);
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});

for(const action of ['owned-empty','real-config','symlink','replacement'])test(`native synthetic project metadata ${action} preserves project-config rejection`,async()=>{
 const f=fixture();let launch;try{
  const metadata=join(f.app,'.codex');
  f.program=f.program.replace("phase='post-spawn-contract';", "phase='post-spawn-contract';mkdirSync(join(body.app.path,'.codex'),{mode:0o755});");
  launch=launchFixture(f,running());await waitFor(()=>launch.output().includes('ready'));
  await waitFor(()=>JSON.parse(readFileSync(launch.receiptPath)).body.nativeMetadata?.placeholders[metadata]);
  assert.equal(launch.control('verify-process').reason,'process-verified');assert.equal(f.run('prepare').reason,'active-launch');
  if(action==='owned-empty')assert.equal(launch.control('stop').reason,'stop-requested');
  if(action==='real-config')writeFileSync(join(metadata,'config.toml'),'fixture-secret');
  if(action==='symlink'){rmSync(metadata,{recursive:true});symlinkSync(f.privateRoot,metadata);}
  if(action==='replacement'){await pause(20);rmSync(metadata,{recursive:true});mkdirSync(metadata,{mode:0o755});}
  await launch.ended;const body=JSON.parse(readFileSync(launch.receiptPath)).body;
  assert.equal(body.status,'exited');assert.ok(!JSON.stringify(body.failure??{}).includes('fixture-secret'));
  if(action==='owned-empty'){assert.equal(existsSync(metadata),true);assert.equal(f.run('prepare').reason,'project-config-present');}
  else{assert.equal(body.termination,'policy-changed');assert.equal(body.failure.reason,'project-config-present');assert.equal(body.failure.metadataRole,'app-root');assert.equal(f.run().reason,'project-config-present');assert.equal(existsSync(metadata),true);}
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});

test('actual Node probe flow emits bounded stages and completes filesystem/socket/ordinary-command checks through three generations',async()=>{
 const f=fixture();let child;const pids=[];try{
  const proof=readFileSync(resolve('scripts/live-host-policy-native-proof.mjs'),'utf8');
  const body='async function childProgram() {'+proof.split('async function childProgram() {')[1].split('\nconst childFile=')[0];
  const script=join(f.app,'flow helper.mjs');writeFileSync(script,`await (${body})();\n`);
  const publicFile=join(f.app,'public.txt'),readOnlyFile=join(f.app,'read-only.txt'),privateFile=join(f.privateRoot,'auth');
  writeFileSync(publicFile,'public-fixture');writeFileSync(readOnlyFile,'read-only fixture');chmodSync(readOnlyFile,0o400);writeFileSync(privateFile,'fixture-secret');chmodSync(privateFile,0);
  const daemon=join(f.root,'masked daemon');mkdirSync(daemon,{mode:0});
  const info=statSync(privateFile),input={mode:'hold',expectedWrite:true,publicFile,readOnlyFile,writeFile:join(f.app,'written'),privateFiles:[privateFile],aliases:[join(f.root,'absent proc alias')],controllerSocket:join(f.root,'absent controller socket'),caddySocket:join(f.root,'absent caddy socket'),daemon,fdIdentity:{dev:info.dev,ino:info.ino}};
  child=spawn(process.execPath,[script,JSON.stringify(input)],{stdio:['ignore','pipe','pipe'],env:{PATH:'/usr/bin:/bin',HOME:f.privateRoot,TMPDIR:'/tmp'}});
  let output='',stderr='';child.stdout.on('data',data=>{output+=data;try{for(const frame of output.split('\n').slice(0,-1).filter(Boolean).map(line=>JSON.parse(line))){if(!pids.some(record=>record.pid===frame.pid))pids.push(identity(frame.pid));}}catch{}});child.stderr.on('data',data=>stderr+=data);
  const frames=()=>output.split('\n').slice(0,-1).filter(Boolean).map(line=>JSON.parse(line));
  await waitFor(()=>frames().filter(frame=>frame.kind==='probe-result').length===3);
  const results=frames().filter(frame=>frame.kind==='probe-result');
  assert.deepEqual(results.map(value=>value.depth).sort(),[0,1,2]);assert.ok(results.every(value=>Object.values(value.checks).every(Boolean)));
  assert.ok(frames().some(frame=>frame.kind==='probe-stage'&&frame.stage==='startup'));assert.ok(frames().some(frame=>frame.stage==='ordinary-command'));
  assert.equal(stderr,'');assert.ok(!output.includes('fixture-secret'));chmodSync(daemon,0o700);
 }finally{
  for(const record of pids.reverse()){try{if(identity(record.pid).birth===record.birth)process.kill(record.pid,'SIGKILL');}catch{}}
  if(child?.exitCode===null&&child?.signalCode===null)child.kill('SIGKILL');
  if(child)await new Promise(done=>{if(child.exitCode!==null||child.signalCode!==null)done();else child.once('close',done);});
  f.cleanup();
 }
});

test('native process snapshot finds worker-thread descendants and classifies the executable without exposing argv',async()=>{
 const f=fixture();let parent,owned;try{
  const code=`import {Worker} from 'node:worker_threads';const worker=new Worker(\`const {parentPort}=require('node:worker_threads');const {spawn}=require('node:child_process');const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore'});parentPort.postMessage({pid:process.pid,child:child.pid});setInterval(()=>{},1000);\`,{eval:true,execArgv:[]});worker.on('message',value=>console.log(JSON.stringify(value)));setInterval(()=>{},1000);`;
  parent=spawn(process.execPath,['--input-type=module','-e',code,join(f.app,'fixture-secret-helper')],{stdio:['ignore','pipe','pipe']});
  let output='';parent.stdout.on('data',data=>output+=data);const info=await waitFor(()=>output.includes('\n')&&JSON.parse(output.trim()));owned=identity(info.child);
  assert.ok(!readFileSync(`/proc/${info.pid}/task/${info.pid}/children`,'utf8').trim().split(/\s+/).includes(String(info.child)));
  const proof=readFileSync(resolve('scripts/live-host-policy-native-proof.mjs'),'utf8');
  const fn='function ownedProcessSnapshot(launch){'+proof.split('function ownedProcessSnapshot(launch){')[1].split('\nasync function closeOwned')[0];
  const snapshot=Function('identity','readFileSync','readlinkSync','readdirSync','binary','admittedNode',`return ${fn}`)(identity,readFileSync,readlinkSync,readdirSync,realpathSync(process.execPath),join(f.root,'other-interpreter'));
  const receipt=join(f.privateRoot,'observed.json');writeFileSync(receipt,JSON.stringify({body:{process:{pid:info.pid}}}));
  const rows=snapshot({path:receipt});assert.ok(rows.some(row=>row.pid===info.child));assert.equal(rows.find(row=>row.pid===info.pid).role,'native-cli');
  assert.ok(!JSON.stringify(rows).includes('fixture-secret-helper'));assert.ok(rows.every(row=>!Object.hasOwn(row,'argv')));
 }finally{if(owned){try{if(identity(owned.pid).birth===owned.birth)process.kill(owned.pid,'SIGKILL');}catch{}}if(parent?.exitCode===null&&parent?.signalCode===null)parent.kill('SIGKILL');if(parent)await new Promise(done=>{if(parent.exitCode!==null||parent.signalCode!==null)done();else parent.once('close',done);});f.cleanup();}
});

test('live stdout and stderr bursts stream through full CLI before exit under backpressure',async()=>{
 const f=fixture();let launch;try{
  launch=launchFixture(f,"const fs=require('fs');process.stdout.write('fifo:'+fs.fstatSync(1).isFIFO()+':'+fs.fstatSync(2).isFIFO()+'\\n');process.stdout.write('O'.repeat(512*1024)+'\\n');process.stderr.write('E'.repeat(512*1024)+'\\n');process.stdout.write('stream-ready\\n');setInterval(()=>{},1000)",true);
  await waitFor(()=>launch.output().includes('stream-ready')&&launch.error().length>=512*1024);
  assert.equal(launch.child.exitCode,null);assert.ok(launch.output().includes('fifo:true:true'));assert.equal(launch.control('verify-process').reason,'process-verified');
  const body=JSON.parse(readFileSync(launch.receiptPath)).body;assert.deepEqual(body.policy.launch.stdio,['posix-pipe','posix-pipe','posix-pipe']);
  assert.equal(body.policy.launch.extraFileDescriptors,false);assert.equal(launch.control('stop').reason,'stop-requested');await launch.ended;
  assert.equal(JSON.parse(readFileSync(launch.receiptPath)).body.status,'exited');assert.ok(!launch.output().includes('fixture-secret'));
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});

test('restricted getsockname drops Node socket input/output; full CLI FIFO bridge forwards chunks and EOF on all stdio',async()=>{
 const f=fixture();let launch;try{
  const compiler=spawnSync('/usr/bin/cc',['--version'],{encoding:'utf8'});assert.equal(compiler.status,0,'supported Linux fixture compiler unavailable');
  const sourceFile=join(f.root,'stdio seccomp.c'),filter=join(f.root,'stdio seccomp');
  writeFileSync(sourceFile,`#include <stddef.h>
#include <errno.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <unistd.h>
int main(int argc,char **argv){if(argc<2)return 64;struct sock_filter f[]={BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,nr)),BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,SYS_getsockname,0,1),BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ERRNO|EPERM),BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ALLOW)};struct sock_fprog p={sizeof(f)/sizeof(f[0]),f};if(prctl(PR_SET_NO_NEW_PRIVS,1,0,0,0)||prctl(PR_SET_SECCOMP,SECCOMP_MODE_FILTER,&p))return 65;execv(argv[1],argv+1);return 66;}`);
  assert.equal(spawnSync('/usr/bin/cc',['-Wall','-Wextra','-Werror',sourceFile,'-o',filter],{encoding:'utf8'}).status,0);
  const inputChunks=['caller input with spaces\n','second ','line\n'],expected=inputChunks.join('');
  const nodeCode="const fs=require('fs');process.stdout.write('filtered-stdout-marker\\n');process.stderr.write('filtered-stderr-marker\\n');fs.writeSync(1,JSON.stringify({type:process.stdout._type??'black-hole'})+'\\n');let received='',chunks=0;const stdin=process.stdin;process.stdout.write('filtered-input-ready\\n');stdin.on('data',chunk=>{received+=chunk;chunks++;});stdin.on('end',()=>{fs.writeSync(1,JSON.stringify({kind:'input-result',received,bytes:Buffer.byteLength(received),chunks,eof:true,constructor:stdin.constructor.name,fifo:fs.fstatSync(0).isFIFO()})+'\\n');});stdin.on('error',()=>process.exit(2));setTimeout(()=>process.exit(0),500);";
  const socket=spawnSync(filter,[process.execPath,'-e',nodeCode],{input:expected,encoding:'utf8',env:{PATH:'/usr/bin:/bin'}});
  assert.equal(socket.status,0);assert.equal(socket.stdout.includes('filtered-stdout-marker'),false);assert.equal(socket.stderr,'');
  const socketRecords=socket.stdout.trim().split('\n').map(line=>JSON.parse(line));assert.equal(socketRecords[0].type,'black-hole');
  assert.equal(socketRecords[1].received,'');assert.equal(socketRecords[1].eof,true);assert.equal(socketRecords[1].constructor,'Readable');
  const code=`const {spawnSync}=require('child_process');const result=spawnSync(${JSON.stringify(filter)},[process.execPath,'-e',${JSON.stringify(nodeCode)}],{stdio:'inherit'});if(result.status!==0)process.exit(1);process.stdout.write('filtered-ready\\n');setInterval(()=>{},1000);`;
  launch=launchFixture(f,code,true,inputChunks);await waitFor(()=>launch.output().includes('filtered-ready'));
  assert.ok(launch.output().includes('filtered-stdout-marker'));assert.ok(launch.error().includes('filtered-stderr-marker'));assert.ok(launch.output().includes('"type":"pipe"'));
  const inputResult=launch.output().split('\n').filter(line=>line.startsWith('{')).map(line=>JSON.parse(line)).find(value=>value.kind==='input-result');
  assert.equal(inputResult.received,expected);assert.equal(inputResult.bytes,Buffer.byteLength(expected));assert.ok(inputResult.chunks>=2);assert.equal(inputResult.eof,true);assert.equal(inputResult.fifo,true);assert.notEqual(inputResult.constructor,'Readable');
  const receipt=JSON.parse(readFileSync(launch.receiptPath)).body;assert.equal(receipt.policy.launch.stdioBridge.shell.path,'/usr/bin/bash');assert.match(receipt.policy.launch.stdioBridge.consumer.sha256,/^[a-f0-9]{64}$/);
  assert.equal(launch.control('stop').reason,'stop-requested');await launch.ended;assert.equal(JSON.parse(readFileSync(launch.receiptPath)).body.status,'exited');
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});
check('project-writable stdio bridge helper is refused without launch',f=>{
 const unsafe=join(f.app,'cat');writeFileSync(unsafe,'#!/bin/sh\nexit 0\n',{mode:0o700});
 f.program=f.program.replace("['consumer','/usr/bin/cat']",`['consumer',${JSON.stringify(unsafe)}]`);
 const result=spawnSync(process.execPath,['--input-type=module','-','probe','--manifest',f.manifest],{input:f.program,encoding:'utf8'});assert.equal(JSON.parse(result.stdout).reason,'untrusted-stdio-bridge');
});

test('full CLI owner SIGKILL closes the private lifetime writer and reaps native and stdio consumers',async()=>{
 const f=fixture();let launch;try{
  launch=launchFixture(f,running(),true,[],true);await waitFor(()=>launch.output().includes('ready'));
  const saved=JSON.parse(readFileSync(launch.receiptPath)).body;assert.equal(saved.parent.pid,launch.child.pid);
  launch.child.kill('SIGKILL');await Promise.race([launch.ended,pause(5000).then(()=>{throw Error('owned full CLI EOF deadline');})]);
  const body=JSON.parse(readFileSync(launch.receiptPath)).body;assert.equal(body.status,'exited');assert.equal(body.termination,'owner-eof');
  assert.equal(existsSync(join(f.privateRoot,'dispatch.lock')),false);assert.equal(f.run('prepare').status,0);
 }finally{if(launch?.child.exitCode===null&&launch?.child.signalCode===null)launch.child.kill('SIGKILL');f.cleanup();}
});

// Actual kernel election and process identities; the copied Node runtime is an
// explicit fixture, NOT evidence of pinned Native execution or source discovery.
async function provenanceFixture(options={}) {
 const f=fixture();let holder,owner;
 const bin=join(f.root,'owning runtime fixture');copyFileSync(realpathSync(process.execPath),bin);chmodSync(bin,0o700);
 f.m.binary={path:bin,version:'0.161.0',sha256:hash(readFileSync(bin))};
 renameSync(f.m.keyFile,join(f.privateRoot,'live-policy.key'));f.m.keyFile=join(f.privateRoot,'live-policy.key');
 f.m.schemaVersion=2;f.m.generation={sessionId:randomUUID(),generation:1};f.m.owningRuntime={args:['app-server']};
 const lockFile=join(f.privateRoot,'controller.lock'),fd=openSync(lockFile,constants.O_CREAT|constants.O_RDWR,0o600);
 holder=spawn('/usr/bin/flock',['--nonblock','--no-fork','/proc/self/fd/3',process.execPath,'-e',"console.log('elected');process.stdin.resume()"],{stdio:['pipe','pipe','pipe',fd],env:{}});closeSync(fd);
 let ready='';holder.stdout.on('data',chunk=>ready+=chunk);await waitFor(()=>ready==='elected\n');
 f.m.election={holder:identity(holder.pid),lockFile};
 // Substitute only pinned-binary VERSION discovery for this non-Native fixture.
 f.program=f.program.replace('version.stdout.trim() !== `codex-cli ${binary.version}`','version.stdout.trim() !== process.version');
 f.run=(action='probe',extra=[])=>{f.save();const result=spawnSync(process.execPath,['--input-type=module','-',action,'--manifest',f.manifest,...extra],{input:f.program,encoding:'utf8',env:{...process.env,OPENAI_API_KEY:'fixture-secret',NODE_OPTIONS:''}});
  assert.equal(result.stderr,'');assert.ok(!result.stdout.includes('fixture-secret'));return {status:result.status,...JSON.parse(result.stdout)};};
 const id=randomUUID(),sources=join(f.privateRoot,'owning-sources.json');
 const capture=()=>f.run('capture-runtime',['--capture-id',id]);
 const start=async()=>{
  // Strictly later than the signed USER_HZ observation.
  await pause(40);
  writeFileSync(join(f.privateRoot,'app-server'),`setTimeout(()=>{if(!require('node:fs').existsSync(${JSON.stringify(join(f.app,'.codex'))}))require('node:fs').mkdirSync(${JSON.stringify(join(f.app,'.codex'))},{mode:0o755});console.log('owner-ready');setInterval(()=>{},1000);},40);`,{mode:0o600});
  owner=spawn(bin,['app-server'],{cwd:f.privateRoot,stdio:['ignore','pipe','pipe'],env:options.managedEnvironment?{PATH:'/usr/bin:/bin',HOME:f.home,CODEX_HOME:f.home,TMPDIR:'/tmp',LANG:'C.UTF-8'}:{HOME:f.home,CODEX_HOME:f.home,PATH:'/usr/bin:/bin'}});
  let output='';owner.stdout.on('data',chunk=>output+=chunk);await waitFor(()=>output==='owner-ready\n');
  return owner;
 };
 const inventory=(runtimeOwner=owner)=>{
  const captured=JSON.parse(readFileSync(join(f.privateRoot,`birth-${id}.json`))).body;
  const paths=captured.directories.map(item=>item.path);
  // policyDigest is returned by capture and is identical in the signing basis.
  return {schemaVersion:2,runtime:identity(runtimeOwner.pid),policyDigest:captureResult.policyDigest,
   effectiveConfigDigest:hash('actual owner RPC fixture'),sessionFlagsDigest:hash('actual owner flags fixture'),sources:[
    ...f.m.configFiles.map(file=>({kind:'file',role:'user',...file})),
    ...paths.map(path=>{
     if(!existsSync(path))return {kind:'absent',role:'project',path};
     const info=statSync(path);return {kind:'directory',role:'project',path,identity:{dev:info.dev,ino:info.ino,uid:info.uid,mode:info.mode,ctime:info.ctimeMs,mtime:info.mtimeMs,birthtime:info.birthtimeMs}};
    }),
    ...['/etc/codex/config.toml','/etc/codex/managed_config.toml','/etc/codex/requirements.toml'].map(path=>({kind:'absent',role:'system',path})),
   ]};
 };
 let captureResult;
 const captureAndStart=async(beforeStart)=>{captureResult=capture();assert.equal(captureResult.status,0,JSON.stringify(captureResult));if(beforeStart)beforeStart();await start();};
 const seal=(input=inventory())=>{writeFileSync(sources,JSON.stringify(input),{mode:0o600});return f.run('seal-runtime',['--capture-id',id,'--runtime-pid',String(owner.pid),'--sources',sources]);};
 const prepare=()=>{f.m.runtimeProvenance={captureId:id,effectiveConfigDigest:hash('actual owner RPC fixture'),sessionFlagsDigest:hash('actual owner flags fixture')};return f.run('prepare');};
 const cleanup=async()=>{for(const child of [owner,holder])if(child?.exitCode===null&&child?.signalCode===null){const closed=new Promise(done=>child.once('close',done));child.kill();await closed;}f.cleanup();};
 return {...f,id,capture,start,captureAndStart,inventory,seal,prepare,cleanup,get owner(){return owner;},get holder(){return holder;}};
}
function provenanceCheck(name,fn){test(`owning provenance v2: ${name}`,async()=>{const f=await provenanceFixture();try{await fn(f);}finally{await f.cleanup();}});}
provenanceCheck('elected pre-birth capture and empty Native-shaped directory admit with explicit directory sources',async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);assert.equal(f.run('verify').status,0);
 const saved=JSON.parse(readFileSync(join(f.privateRoot,'policy-receipt.json'))).body;
 assert.equal(saved.schemaVersion,2);assert.equal(saved.provenance.runtime.process.pid,f.owner.pid);
 assert.equal(saved.provenance.inventory.sources.filter(source=>source.kind==='directory').length,1);
 assert.equal(saved.provenance.inventory.sources.some(source=>source.kind==='file'&&source.path.endsWith('/.codex')),false);
 assert.ok(saved.sandboxState.permissionProfile.file_system.entries.some(entry=>entry.access==='deny'&&entry.path.path===join(f.app,'.codex')));
 assert.equal(f.capture().reason,'reused-capture-id');assert.equal(f.seal().reason,'reused-capture-id');
});
provenanceCheck('original enabled network is preserved only with sealed birth provenance and all private denies',async f=>{
 f.m.sandboxState.permissionProfile.network='enabled';await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 const saved=JSON.parse(readFileSync(join(f.privateRoot,'policy-receipt.json'))).body;
 assert.equal(saved.sandboxState.permissionProfile.network,'enabled');
 const denies=saved.sandboxState.permissionProfile.file_system.entries.filter(entry=>entry.access==='deny').map(entry=>entry.path.path);
 for(const path of [f.privateRoot,'/proc',join(f.app,'.codex')])assert.ok(denies.includes(path));
 assert.equal(f.run('verify').status,0);f.m.sandboxState.permissionProfile.network='restricted';assert.equal(f.run('verify').status,1);
});
provenanceCheck('empty preexisting metadata cannot acquire absence evidence',async f=>{mkdirSync(join(f.app,'.codex'));assert.equal(f.capture().reason,'project-config-present');});
provenanceCheck('election is mandatory before capture, even when the directory is absent',async f=>{delete f.m.election;assert.equal(f.capture().reason,'missing-election');});
provenanceCheck('unlocked holder FD is not elected authority',async f=>{f.m.election.holder=identity(process.pid);assert.equal(f.capture().reason,'stale-election');});
provenanceCheck('capture cannot authorize first prepare until sealed',async f=>{await f.captureAndStart();assert.equal(f.prepare().reason,'revoked-runtime-provenance');});
for(const name of ['config.toml','hooks.json','rules','agents'])provenanceCheck(`injected ${name} blocks sealing`,async f=>{
 await f.captureAndStart();writeFileSync(join(f.app,'.codex',name),'fixture-secret');assert.equal(f.seal().reason,'metadata-content-present');
});
for(const variant of ['missing-directory','missing-system','missing-user','duplicate','unknown-kind','wrong-policy','wrong-runtime','directory-as-file','extra-source'])provenanceCheck(`incomplete source inventory ${variant} fails closed`,async f=>{
 await f.captureAndStart();const input=f.inventory();
 if(variant==='missing-directory')input.sources=input.sources.filter(source=>source.role!=='project');
 if(variant==='missing-system')input.sources=input.sources.filter(source=>source.role!=='system');
 if(variant==='missing-user')input.sources=input.sources.filter(source=>source.role!=='user');
 if(variant==='duplicate')input.sources.push(input.sources[0]);
 if(variant==='unknown-kind')input.sources[0].kind='copied-directory';
 if(variant==='wrong-policy')input.policyDigest='0'.repeat(64);
 if(variant==='wrong-runtime')input.runtime.birth='0';
 if(variant==='directory-as-file'){const source=input.sources.find(source=>source.kind==='directory');source.kind='file';}
 if(variant==='extra-source')input.sources.push({kind:'absent',role:'user',path:'/private-uninventoried-user'});
 assert.equal(f.seal(input).reason,'incomplete-source-inventory');
});
for(const variant of ['config','hook','rules','agents','symlink','replacement','generation','session','profile','permissions','network','component','owner-birth','election-birth','parent-replacement','tampered-mac','unsigned-marker'])provenanceCheck(`sealed ${variant} change revokes before next admission`,async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 const metadata=join(f.app,'.codex');
 if(['config','hook','rules','agents'].includes(variant))writeFileSync(join(metadata,variant),'fixture-secret');
 if(variant==='symlink'){renameSync(metadata,metadata+' prior');symlinkSync(f.privateRoot,metadata);}
 if(variant==='replacement'){renameSync(metadata,metadata+' prior');mkdirSync(metadata,{mode:0o755});}
 if(variant==='parent-replacement'){renameSync(f.app,f.app+' prior');mkdirSync(f.app,{mode:0o700});mkdirSync(metadata,{mode:0o755});}
 if(variant==='generation')f.m.generation.generation++;
 if(variant==='session')f.m.generation.sessionId=randomUUID();
 if(variant==='profile')f.m.profileId='changed';
 if(variant==='permissions')f.m.sandboxState.permissionProfile.file_system.entries[1].access='read';
 if(variant==='network')f.m.sandboxState.permissionProfile.network='enabled';
 if(variant==='component')f.m.components.agents='changed';
 if(variant==='election-birth')f.m.election.holder.birth='0';
 if(['owner-birth','tampered-mac','unsigned-marker'].includes(variant)){
  const path=join(f.privateRoot,`runtime-${f.id}.json`),saved=JSON.parse(readFileSync(path));
  if(variant==='owner-birth')saved.body.runtime.process.birth='0';
  if(variant==='tampered-mac')saved.mac='0'.repeat(64);
  writeFileSync(path,JSON.stringify(variant==='unsigned-marker'?saved.body:saved),{mode:0o600});
 }
 const result=f.run('verify');assert.equal(result.available,false,variant);assert.equal(result.status,1);assert.ok(!JSON.stringify(result).includes('fixture-secret'));
 assert.equal(existsSync(metadata),true);
});
provenanceCheck('actual owner exit invalidates admission and preserves metadata',async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 const closed=new Promise(done=>f.owner.once('close',done));f.owner.kill();await closed;
 assert.equal(f.run('verify').available,false);assert.equal(existsSync(join(f.app,'.codex')),true);
});
provenanceCheck('actual election holder loss invalidates admission',async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 const closed=new Promise(done=>f.holder.once('close',done));f.holder.kill();await closed;assert.equal(f.run('verify').available,false);
});
provenanceCheck('runtime born before signed absence capture is rejected, even with the correct executable and parent',async f=>{
 const owner=await f.start();rmSync(join(f.app,'.codex'),{recursive:true}); // fixture-only reset to exercise stale birth; production never deletes metadata.
 const captured=f.capture();assert.equal(captured.status,0);
 writeFileSync(join(f.privateRoot,'old-sources.json'),'{}',{mode:0o600});
 assert.equal(f.run('seal-runtime',['--capture-id',f.id,'--runtime-pid',String(owner.pid),'--sources',join(f.privateRoot,'old-sources.json')]).reason,'stale-runtime-provenance');
});
provenanceCheck('competing capture/session cannot replace the active admission fence',async f=>{
 await f.captureAndStart();assert.equal(f.run('capture-runtime',['--capture-id',randomUUID()]).reason,'active-runtime-admission');
 const before=JSON.parse(readFileSync(join(f.privateRoot,`runtime-admission-${hash(f.app)}.json`))).body;
 f.m.generation.sessionId=randomUUID();
 assert.equal(f.seal().reason,'revoked-runtime-provenance');
 assert.deepEqual(JSON.parse(readFileSync(join(f.privateRoot,`runtime-admission-${hash(f.app)}.json`))).body,before);
});
provenanceCheck('exclusive fence rejects a second capture before either owner starts',async f=>{
 assert.equal(f.capture().status,0);
 assert.equal(f.run('capture-runtime',['--capture-id',randomUUID()]).reason,'active-runtime-admission');
});
provenanceCheck('explicit revoke is authenticated, idempotent and preserves runtime metadata',async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 const revoke=()=>f.run('revoke-runtime',['--capture-id',f.id]);
 assert.equal(f.run('revoke-runtime',['--capture-id',randomUUID()]).reason,'unowned-runtime-admission');
 // Revocation remains usable after profile/config changes; it cannot grant work.
 f.m.profileId='changed';assert.equal(revoke().reason,'runtime-provenance-revoked');assert.equal(revoke().status,0);
 assert.equal(f.run('verify').reason,'revoked-runtime-provenance');
 assert.equal(existsSync(join(f.app,'.codex')),true);assert.equal(existsSync(join(f.privateRoot,`runtime-admission-${hash(f.app)}.lock`)),false);
 assert.equal(f.run('capture-runtime',['--capture-id',randomUUID()]).reason,'project-config-present');
});
provenanceCheck('revoked capture cannot seal a live child later',async f=>{
 await f.captureAndStart();assert.equal(f.run('revoke-runtime',['--capture-id',f.id]).status,0);
 assert.equal(f.seal().reason,'revoked-runtime-provenance');
});
provenanceCheck('an intermediary process cannot claim elected-controller signing authority',async f=>{
 f.save();const code=`const{spawnSync}=require('node:child_process');const r=spawnSync(process.execPath,['--input-type=module','-','capture-runtime','--manifest',${JSON.stringify(f.manifest)},'--capture-id',${JSON.stringify(f.id)}],{input:${JSON.stringify(f.program)},encoding:'utf8'});process.stdout.write(r.stdout);`;
 const result=spawnSync(process.execPath,['-e',code],{encoding:'utf8'});assert.equal(JSON.parse(result.stdout).reason,'unauthorized-controller');
});
for(const field of ['effectiveConfigDigest','sessionFlagsDigest'])provenanceCheck(`current observed ${field} change invalidates sealed source identity`,async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 f.m.runtimeProvenance[field]='0'.repeat(64);assert.equal(f.run('verify').reason,'source-changed');
});
provenanceCheck('changed original regular source bytes revoke sealed owner',async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 const config=f.m.configFiles[0];writeFileSync(config.path,'unsafe original config');assert.equal(f.run('verify').available,false);
});
provenanceCheck('later generated regular config invalidates policy receipt without rewriting original source provenance',async f=>{
 await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
 const file=join(f.privateRoot,'live-mcp-config.json');writeFileSync(file,'{}',{mode:0o600});f.m.configFiles.push({path:file,sha256:hash('{}')});
 assert.equal(f.run('verify').reason,'stale-receipt');assert.equal(f.prepare().status,0);assert.equal(f.run('verify').status,0);
 const receipt=JSON.parse(readFileSync(join(f.privateRoot,'policy-receipt.json'))).body;
 assert.equal(receipt.provenance.inventory.sources.some(source=>source.path===file),false);
});
for(const reason of ['revocation','owner-exit','metadata-injection','generation-change'])provenanceCheck(`persistent admitted helper is reaped on ${reason} with versioned per-call checks`,async f=>{
 let helper;
 try {
  // Explicit non-Native dispatcher fixture. It exercises the real manager
  // supervisor, receipts, fences and owned cleanup; it does not enforce a sandbox.
  writeFileSync(join(f.privateRoot,'sandbox'),`const{spawn}=require('node:child_process');const a=process.argv.slice(2),i=a.indexOf('--');const c=spawn(a[i+1],a.slice(i+2),{stdio:'inherit'});c.on('exit',code=>process.exit(code??1));`,{mode:0o600});
  f.program=f.program.replace("timer=setTimeout(()=>stop('timeout'),command.timeoutMs);","process.stdout.write('lifetime-ready\\n');timer=setTimeout(()=>stop('timeout'),command.timeoutMs);");
  f.m.commands={ordinary:{executable:f.m.binary.path,sha256:f.m.binary.sha256,args:['-e',running()],timeoutMs:30000}};
  await f.captureAndStart();assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);
  const id=randomUUID(),path=join(f.privateRoot,`process-${id}.json`);
  helper=spawn(process.execPath,['--input-type=module','-','run','--manifest',f.manifest,'--command','ordinary','--launch-id',id],{stdio:['pipe','pipe','pipe','pipe']});
  let output='',errors='';helper.stdout.on('data',chunk=>output+=chunk);helper.stderr.on('data',chunk=>errors+=chunk);
  helper.stdin.end(f.program);helper.stdio[3].end();helper.once('exit',()=>helper.stdio[3].destroy());
  const ended=new Promise(done=>helper.once('close',done));await waitFor(()=>output.includes('lifetime-ready'));
  assert.equal(JSON.parse(readFileSync(path)).body.schemaVersion,2);
  if(reason==='revocation')assert.equal(f.run('revoke-runtime',['--capture-id',f.id]).status,0);
  if(reason==='owner-exit'){const closed=new Promise(done=>f.owner.once('close',done));f.owner.kill();await closed;}
  if(reason==='metadata-injection')writeFileSync(join(f.app,'.codex','config.toml'),'fixture-secret');
  if(reason==='generation-change'){f.m.generation.generation++;f.save();}
  await Promise.race([ended,pause(6000).then(()=>assert.fail('v2 lifetime deadline'))]);
  const receipt=JSON.parse(readFileSync(path)).body;assert.equal(receipt.status,'exited');assert.equal(receipt.termination,'policy-changed');
  assert.equal(existsSync(join(f.app,'.codex')),true);assert.equal(existsSync(join(f.privateRoot,'dispatch.lock')),false);assert.equal(errors,'');
 }finally{if(helper?.exitCode===null&&helper?.signalCode===null){const closed=new Promise(done=>helper.once('close',done));helper.kill('SIGKILL');await closed;}}
});
provenanceCheck('generation-specific manifest reuses the existing ancestor election and app fence',async f=>{
 const generation=join(f.privateRoot,'generation with spaces'),home=join(generation,'home');mkdirSync(generation,{mode:0o700});mkdirSync(home,{mode:0o700});
 const config=join(home,'config.toml');writeFileSync(config,'[shell_environment_policy]\ninherit = "none"\n',{mode:0o600});
 const manifest=join(generation,'manifest.json'),m=structuredClone(f.m),id=randomUUID();m.runtimeHome=home;m.environment.HOME=home;m.configFiles=[{path:config,sha256:hash(readFileSync(config))}];
 writeFileSync(manifest,JSON.stringify(m),{mode:0o600});
 const captured=spawnSync(process.execPath,['--input-type=module','-','capture-runtime','--manifest',manifest,'--capture-id',id],{input:f.program,encoding:'utf8'});
 assert.equal(captured.status,0,captured.stdout);assert.equal(existsSync(join(generation,`birth-${id}.json`)),true);
 const fence=JSON.parse(readFileSync(join(f.privateRoot,`runtime-admission-${hash(f.app)}.json`))).body;assert.equal(fence.manifestRoot,generation);assert.equal(fence.captureId,id);
 // The different manifest directory cannot compete against the same app/election.
 assert.equal(f.capture().reason,'active-runtime-admission');
});
provenanceCheck('a private generation cannot hide an original write grant to the shared election key',async f=>{
 f.m.sandboxState.permissionProfile.file_system.entries.push({path:{type:'path',path:f.m.keyFile},access:'write'});
 assert.equal(f.capture().reason,'project-writable-policy');
});
provenanceCheck('metadata creation raced between capture and original owning birth is rejected',async f=>{
 await f.captureAndStart(()=>mkdirSync(join(f.app,'.codex'),{mode:0o755}));
 assert.equal(f.seal().reason,'metadata-before-runtime-birth');
});
provenanceCheck('already materialized Native denial of absent metadata stays denied during capture',async f=>{
 const path=join(f.app,'.codex');f.m.sandboxState.permissionProfile.file_system.entries.push({path:{type:'path',path},access:'deny'});
 assert.equal(f.capture().status,0);
});
provenanceCheck('missing-denial normalization cannot hide an existing symlink ancestor',async f=>{
 const alias=join(f.root,'private alias');symlinkSync(f.privateRoot,alias);
 f.m.sandboxState.permissionProfile.file_system.entries.push({path:{type:'path',path:join(alias,'missing')},access:'deny'});
 assert.equal(f.capture().reason,'symlink-path');
});
provenanceCheck('managed private CLI exec preserves original birth, direct parent and caller stdio',async f=>{
 let owner,extra;
 try {
  // Install only the fixture Manager script, not plugins or user configuration.
  const parts=source.split("<<'LIVE_POLICY'\n"),tail=parts[1].split('\nLIVE_POLICY\n')[1];
  writeFileSync(f.managerPath,parts[0]+"<<'LIVE_POLICY'\n"+f.program+'\nLIVE_POLICY\n'+tail,{mode:0o700});
  // Capture this exact candidate once; a revoked capture is never reset.
  const id=randomUUID();assert.equal(f.run('capture-runtime',['--capture-id',id]).status,0);await pause(40);
  writeFileSync(join(f.privateRoot,'app-server'),`const keyPin=${JSON.stringify({dev:statSync(f.m.keyFile).dev,ino:statSync(f.m.keyFile).ino})};for(let fd=3;fd<128;fd++){try{const p=require('node:fs').fstatSync(fd);if(p.dev===keyPin.dev&&p.ino===keyPin.ino)process.exit(7)}catch{}};if(Object.keys(process.env).some(key=>/SECRET|TOKEN|API_KEY/.test(key)))process.exit(8);process.stdout.write('managed-ready\\n');process.stdin.on('data',chunk=>process.stdout.write(chunk));process.stdin.on('end',()=>process.exit(0));`,{mode:0o600});
  extra=openSync(f.m.keyFile,'r');
  owner=spawn('/usr/bin/bash',[f.managerPath,'cli','--','--live-policy-owning-runtime','--manifest',f.manifest,'--capture-id',id],
    {cwd:f.app,env:{PATH:`${dirname(process.execPath)}:/usr/bin:/bin`,HOME:f.privateRoot,CODEX_HOME:f.home,GESTALT_HOME:join(f.privateRoot,'manager-state'),OPENAI_API_KEY:'fixture-secret'},stdio:['pipe','pipe','pipe','ignore','ignore','ignore','ignore',extra]});
  let output='',errors='';owner.stdout.on('data',chunk=>output+=chunk);owner.stderr.on('data',chunk=>errors+=chunk);
  const ended=new Promise(done=>owner.once('close',done));await Promise.race([waitFor(()=>output==='managed-ready\n'),ended.then(code=>assert.fail(JSON.stringify({code,stdout:output,stderr:errors})))]);
  assert.equal(readlinkSync(`/proc/${owner.pid}/exe`),f.m.binary.path);
  assert.equal(readFileSync(`/proc/${owner.pid}/stat`,'utf8').split(')').at(-1).trim().split(' ')[1],String(process.pid));
  owner.stdin.end('original-private-stdio\n');assert.equal(await ended,0);assert.equal(output,'managed-ready\noriginal-private-stdio\n');
  assert.ok(!errors.includes('Gestalt manager'));assert.ok(!errors.includes('fixture-secret'));
 }finally{if(extra!==undefined)closeSync(extra);if(owner?.exitCode===null&&owner?.signalCode===null){const closed=new Promise(done=>owner.once('close',done));owner.kill('SIGKILL');await closed;}}
});
provenanceCheck('actual startup instruction source must match its immutable pre-birth pin',async f=>{
 const path=join(f.app,'AGENTS.md');writeFileSync(path,'original project instructions',{mode:0o644});await f.captureAndStart();
 const info=statSync(path),input=f.inventory();input.sources.push({kind:'instruction',path,sha256:hash(readFileSync(path)),identity:{dev:info.dev,ino:info.ino,uid:info.uid,mode:info.mode,ctime:info.ctimeMs,mtime:info.mtimeMs}});
 assert.equal(f.seal(input).status,0);assert.equal(f.prepare().status,0);
 writeFileSync(path,'changed instruction source');assert.equal(f.run('verify').reason,'source-changed');
});
provenanceCheck('an instruction injected after original runtime birth cannot be described as a startup source',async f=>{
 await f.captureAndStart();const path=join(f.app,'AGENTS.md');writeFileSync(path,'injected instructions',{mode:0o644});
 const info=statSync(path),input=f.inventory();input.sources.push({kind:'instruction',path,sha256:hash(readFileSync(path)),identity:{dev:info.dev,ino:info.ino,uid:info.uid,mode:info.mode,ctime:info.ctimeMs,mtime:info.mtimeMs}});
 assert.equal(f.seal(input).reason,'source-changed');
});

provenanceCheck('omitted original AGENTS instruction cannot seal an incomplete source inventory',async f=>{
 writeFileSync(join(f.app,'AGENTS.md'),'original instructions',{mode:0o644});await f.captureAndStart();
 assert.equal(f.seal().reason,'incomplete-source-inventory');
});
for(const phase of ['seal','verify'])for(const kind of ['file','symlink'])provenanceCheck(`originally absent instruction ${kind} appearance blocks ${phase}`,async f=>{
 await f.captureAndStart();if(phase==='verify'){assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);}
 const path=join(f.app,'AGENTS.override.md');if(kind==='file')writeFileSync(path,'injected override',{mode:0o644});else symlinkSync(join(f.home,'config.toml'),path);
 assert.equal((phase==='seal'?f.seal():f.run('verify')).reason,'source-changed');
});
provenanceCheck('shadowed instruction candidates remain pinned without claiming Native loaded them',async f=>{
 const original=join(f.app,'AGENTS.md'),override=join(f.app,'AGENTS.override.md');writeFileSync(original,'shadowed original',{mode:0o644});writeFileSync(override,'selected override',{mode:0o644});await f.captureAndStart();
 const source=(path,kind)=>{const info=statSync(path);return {kind,path,sha256:hash(readFileSync(path)),identity:{dev:info.dev,ino:info.ino,uid:info.uid,mode:info.mode,ctime:info.ctimeMs,mtime:info.mtimeMs}};};
 const input=f.inventory();input.sources.push(source(override,'instruction'));assert.equal(f.seal(input).reason,'incomplete-source-inventory');
 input.sources.push(source(original,'instruction-candidate'));assert.equal(f.seal(input).status,0);assert.equal(f.prepare().status,0);
 writeFileSync(original,'changed shadowed original');assert.equal(f.run('verify').reason,'source-changed');
});
test('controlled Native command rejects failed and unfinished results even when they echo the probe marker',async()=>{
 const proof=readFileSync(resolve('scripts/live-host-policy-native-proof.mjs'),'utf8');
 const implementation=proof.slice(proof.indexOf('function modelCall('),proof.indexOf('\nconst modelProbe='));
 const call=Function('assert','report',`${implementation};return modelCall;`)(assert,{});
 const event=call([{name:'exec',description:'exec_command(args)'}],'probe','exec_command',{cmd:'controlled probe'});
 const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
 const invoke=new AsyncFunction('tools','text',event.item.input),outputs=[];
 for(const exit_code of [1,undefined])await assert.rejects(invoke({exec_command:async()=>({exit_code,output:'native-private-and-metadata-denied'})},value=>outputs.push(value)),/NATIVE_PROBE_EXIT/);
 assert.equal(outputs.length,0);
 await invoke({exec_command:async()=>({exit_code:0,output:'native-private-and-metadata-denied'})},value=>outputs.push(value));
 assert.equal(outputs.length,1);assert.equal(outputs[0].exit_code,0);
});

test('startup RPC diagnostic identifies method and fixed error facts without config, instructions or error text',()=>{
 const proof=readFileSync(resolve('scripts/live-host-policy-native-proof.mjs'),'utf8');
 const expression=proof.split('const rpcFailureDiagnostic=')[1].split('\nlet runtime,')[0];
 const diagnose=Function('hash',`return ${expression}`)(hash);
 const message='error creating thread: Permission denied (os error 13) reading AGENTS.md at /private/fixture-secret; config token=fixture-secret prompt=fixture-secret';
 const result=diagnose('thread/start',{code:-32603,message},{originalHome:'/private/fixture-secret'});
 assert.equal(result.method,'thread/start');assert.equal(result.code,-32603);assert.equal(result.threadCreation,true);assert.equal(result.permissionDenied,true);assert.equal(result.instructionDiscovery,true);
 assert.deepEqual(result.matchedPathRoles,['originalHome']);assert.equal(result.messageSha256,hash(message));
 assert.ok(!JSON.stringify(result).includes('fixture-secret'));assert.ok(!JSON.stringify(result).includes('/private'));assert.ok(!JSON.stringify(result).includes('token='));assert.ok(!JSON.stringify(result).includes('prompt='));
 const full='error creating thread: failed to load AGENTS.md instructions: fs sandbox helper failed: bwrap: Can\'t create file at /private/'+('x'.repeat(300))+': Read-only file system; fixture-secret';
 const truncated=diagnose('thread/start',{code:-32603,message:full.slice(0,256)});
 const wire=diagnose('thread/start',{code:-32603,message:full});
 assert.equal(truncated.readOnly,false);assert.equal(wire.readOnly,true);assert.equal(wire.bubblewrapCreateFile,true);assert.ok(wire.messageBytes>256);assert.ok(!JSON.stringify(wire).includes('fixture-secret'));assert.ok(!JSON.stringify(wire).includes('/private'));
 for(const [phrase,code] of [['Is a directory','EISDIR'],['Not a directory','ENOTDIR'],['File exists','EEXIST'],['Bad file descriptor','EBADF'],['Read-only file system','EROFS']]){
  const classified=diagnose('thread/start',{code:-32603,message:'error creating thread: bwrap: Can\'t create file at /private/fixture-secret: '+phrase},{metadata:'/private/fixture-secret'});
  assert.deepEqual(classified.operation,{kind:'create-file',errnoClass:code,targetRole:'metadata'});assert.ok(!JSON.stringify(classified).includes('fixture-secret'));
 }
 const unknown=diagnose('thread/start',{code:-32603,message:'bwrap: Can\'t create file at /private/fixture-secret: arbitrary fixture-secret'},{metadata:'/private/fixture-secret'});
 assert.equal(unknown.operation.errnoClass,'unclassified');assert.ok(!JSON.stringify(unknown).includes('fixture-secret'));
});

test('Native build failure reports locked dependency classes without raw logs or credentials',()=>{
 const root=mkdtempSync(join(tmpdir(),'native build diagnostic '));
 try{
  const script=readFileSync(resolve('scripts/live-host-native-mask-build.sh'),'utf8');
  const diagnostic=script.split("<<'PY'\n")[1].split('\nPY\n')[0];
  const log=join(root,'log'),lock=join(root,'lock'),version=join(root,'version');
  const revision='a'.repeat(40);
  writeFileSync(lock,`name = "mxc"\nsource = "git+https://github.com/microsoft/mxc#${revision}"\n`);
  writeFileSync(version,'rustc 1.95.0\nrelease: 1.95.0\nhost: x86_64-unknown-linux-gnu\n');
  writeFileSync(log,`failed to load source for dependency \`mxc\`\nfailed to update https://fixture-secret@github.com/microsoft/mxc#${revision}\nrevision ${revision} not found\nHTTP 403\nprivate path /private/fixture-secret prompt=fixture-secret token=fixture-secret\ndependency \`fixture-secret\`\n---- suite::landlock::sandbox_enforces_denied_metadata_directory_and_file_types stdout ----\nNATIVE_METADATA_EXIT kind=missing exitCode=12 stderrClass=unclassified stdoutBytes=12 stderrBytes=0\ntest result: FAILED. 80 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out\n`);
  const run=spawnSync('python3',['-','build-bwrap','101',log,lock,version],{input:diagnostic,encoding:'utf8'});
  assert.equal(run.status,0,run.stderr);
  const result=JSON.parse(run.stdout.split('title=Native mask candidate failure::')[1]);
  assert.deepEqual(result.cargoErrorClasses,['dependencyLoad','missingRevision','sourceUpdate']);
  assert.deepEqual(result.dependencies,['mxc']);assert.deepEqual(result.publicRepositories,['microsoft/mxc']);
  assert.deepEqual(result.lockedRevisions,[revision]);assert.deepEqual(result.httpStatuses,['403']);
  assert.equal(result.rustRelease,'1.95.0');assert.equal(result.host,'x86_64-unknown-linux-gnu');
  assert.deepEqual(result.failedTests,['suite::landlock::sandbox_enforces_denied_metadata_directory_and_file_types']);
  assert.deepEqual(result.metadataFailure,{fixtureKind:'missing',exitCode:12,stderrClass:'unclassified',stdoutBytes:12,stderrBytes:0,commandRole:'denied-directory-list-succeeded'});
  assert.deepEqual(result.testResults,['test result: FAILED. 80 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out']);
  for(const secret of ['fixture-secret','/private/','prompt=','token=','https://'])assert.ok(!run.stdout.includes(secret));
 }finally{rmSync(root,{recursive:true,force:true});}
});

const renewalStable=value=>JSON.stringify((function order(input){if(Array.isArray(input))return input.map(order);if(input&&typeof input==='object')return Object.fromEntries(Object.keys(input).sort().map(key=>[key,order(input[key])]));return input;})(value));
async function renewalFixture({observed=true,explicitHome=true}={}) {
 const f=await provenanceFixture({managedEnvironment:true});if(explicitHome)f.m.owningRuntime.home=f.home;
 const parts=source.split("<<'LIVE_POLICY'\n"),tail=parts[1].split('\nLIVE_POLICY\n')[1];writeFileSync(f.managerPath,parts[0]+"<<'LIVE_POLICY'\n"+f.program+'\nLIVE_POLICY\n'+tail,{mode:0o700});
 const flags=value=>({model:'fixture-model',modelProvider:'fixture-provider',cwd:value.appRoot,approvalPolicy:'never',activePermissionProfile:{id:value.profileId,extends:':read-only'},sandbox:{type:'fixture'},reasoningEffort:'medium',instructionSources:[]});
 const observedInventory=(value,inventory)=>{if(!observed)return inventory;const observation={sessionFlags:flags(value),accountDigest:hash('account-fixture')};return {...inventory,observation,sessionFlagsDigest:hash(renewalStable(observation.sessionFlags))};};
 await f.captureAndStart();const original=observedInventory(f.m,f.inventory());assert.equal(f.seal(original).status,0);f.m.runtimeProvenance={captureId:f.id,effectiveConfigDigest:original.effectiveConfigDigest,sessionFlagsDigest:original.sessionFlagsDigest};assert.equal(f.run('prepare').status,0);
 const invoke=(path,action,args=[])=>{const result=spawnSync(process.execPath,['--input-type=module','-',action,'--manifest',path,...args],{input:f.program,encoding:'utf8',env:{...process.env,NODE_OPTIONS:''}});assert.equal(result.stderr,'');return {status:result.status,...JSON.parse(result.stdout)};};
 let sequence=1;const generations=[];
 const next=(previous={manifest:f.manifest,id:f.id,value:f.m})=>{
  const directory=join(f.privateRoot,'renewal-'+sequence++);mkdirSync(directory,{mode:0o700});const home=join(directory,'worker');mkdirSync(home,{mode:0o700});copyFileSync(join(f.home,'config.toml'),join(home,'config.toml'));chmodSync(join(home,'config.toml'),0o600);
  const value=structuredClone(previous.value);value.generation.generation++;value.runtimeHome=home;value.environment.HOME=home;delete value.runtimeProvenance;
  value.configFiles=value.configFiles.filter(file=>file.path!==join(previous.value.runtimeHome,'config.toml')||previous.value.runtimeHome===(previous.value.owningRuntime.home??previous.value.runtimeHome));value.configFiles.push({path:join(home,'config.toml'),sha256:hash(readFileSync(join(home,'config.toml')))});
  const manifest=join(directory,'manifest.json'),id=randomUUID(),save=()=>writeFileSync(manifest,JSON.stringify(value),{mode:0o600});save();
  const run=(action,args=[])=>{save();return invoke(manifest,action,args);};
  const capture=(mode='survivor')=>run('renew-runtime',['--capture-id',id,'--previous-manifest',previous.manifest,'--previous-capture-id',previous.id,'--mode',mode]);
  const inventory=(pid=f.owner.pid)=>{const result=f.inventory({pid});result.runtime=identity(pid);result.policyDigest=captured.policyDigest;result.sources=result.sources.filter(source=>source.path!==join(f.home,'config.toml')||value.owningRuntime.home===f.home);result.sources=result.sources.filter(source=>!source.path.includes('/renewal-'));result.sources=result.sources.filter(source=>source.kind!=='file'||value.configFiles.some(file=>file.path===source.path&&file.sha256===source.sha256));result.sources.push({kind:'file',role:'sandbox-worker',path:join(home,'config.toml'),sha256:hash(readFileSync(join(home,'config.toml')))});return observedInventory(value,result);};
  let captured;
  const renew=(mode='survivor')=>{captured=capture(mode);return captured;};
  const seal=(input=inventory())=>{const path=join(directory,'observed-sources.json');writeFileSync(path,JSON.stringify(input),{mode:0o600});const result=run('seal-runtime',['--capture-id',id,'--runtime-pid',String(input.runtime.pid),'--sources',path]);if(result.status===0)value.runtimeProvenance={captureId:id,effectiveConfigDigest:input.effectiveConfigDigest,sessionFlagsDigest:input.sessionFlagsDigest};return result;};
  const result={manifest,id,value,run,renew,inventory,seal,save,previous};generations.push(result);return result;
 };
 return {f,next,flags,invoke,original,generations,revoke:()=>f.run('revoke-runtime',['--capture-id',f.id]),cleanup:()=>f.cleanup()};
}
function renewalCheck(name,fn,options){test('runtime renewal: '+name,async()=>{const r=await renewalFixture(options);try{await fn(r);}finally{await r.cleanup();}});}
renewalCheck('survivor gets a fresh generation while old authority remains irrevocably revoked',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();const captured=n.renew();assert.equal(captured.status,0,JSON.stringify(captured));const sealed=n.seal();assert.equal(sealed.status,0,JSON.stringify(sealed));const prepared=n.run('prepare');assert.equal(prepared.status,0,JSON.stringify(prepared));assert.equal(n.run('verify').status,0);
 assert.equal(r.f.run('verify').reason,'revoked-runtime-provenance');assert.equal(r.f.run('revoke-runtime',['--capture-id',r.f.id]).reason,'unowned-runtime-admission');
 assert.ok(existsSync(join(r.f.app,'.codex')));assert.ok(existsSync(join(r.f.privateRoot,`revocation-${r.f.id}.json`)));
 assert.equal(n.run('renew-runtime',['--capture-id',n.id,'--previous-manifest',r.f.manifest,'--previous-capture-id',r.f.id,'--mode','survivor']).reason,'reused-capture-id');
});
renewalCheck('active old authority cannot be renewed',async r=>{assert.equal(r.next().renew().reason,'renewal-not-revoked');});
renewalCheck('unobserved legacy capture cannot acquire renewal intent',async r=>{assert.equal(r.revoke().status,0);assert.equal(r.next().renew().reason,'renewal-observation-required');},{observed:false});
for(const variant of ['generation','session','components','binary','source','account','provider','model','thinking','broadening','entry','metadata','instruction-file','instruction-symlink','dispatch','pending-process','forged-parent'])renewalCheck(`rejects ${variant} mismatch`,async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();
 if(variant==='generation')n.value.generation.generation--;
 if(variant==='session')n.value.generation.sessionId=randomUUID();
 if(variant==='components')n.value.components.agents='changed';
 if(variant==='binary')n.value.binary.version='0.160.0';
 if(variant==='source')writeFileSync(join(r.f.home,'config.toml'),'changed');
 if(variant==='broadening')n.value.sandboxState.permissionProfile.network='enabled';
 if(variant==='entry')n.value.sandboxState.permissionProfile.file_system.entries.push({path:{type:'path',path:'/dev/null'},access:'read'});
 if(variant==='metadata'){renameSync(join(r.f.app,'.codex'),join(r.f.app,'.codex-original'));mkdirSync(join(r.f.app,'.codex'),{mode:0o755});}
 if(variant==='instruction-file')writeFileSync(join(r.f.app,'AGENTS.override.md'),'injected',{mode:0o644});
 if(variant==='instruction-symlink')symlinkSync(join(r.f.home,'config.toml'),join(r.f.app,'AGENTS.override.md'));
 if(variant==='dispatch')mkdirSync(join(r.f.privateRoot,'dispatch.lock'),{mode:0o700});
 if(variant==='pending-process')writeFileSync(join(r.f.privateRoot,`process-${randomUUID()}.json`),'{}',{mode:0o600});
 if(variant==='forged-parent'){const path=join(r.f.privateRoot,`runtime-${r.f.id}.json`),saved=JSON.parse(readFileSync(path));saved.body.runtime.process.birth='0';writeFileSync(path,JSON.stringify(saved),{mode:0o600});}
 const captured=n.renew();
 if(['account','provider','model','thinking'].includes(variant)){
  assert.equal(captured.status,0);const input=n.inventory();if(variant==='account')input.observation.accountDigest=hash('other-account');else input.observation.sessionFlags[{provider:'modelProvider',model:'model',thinking:'reasoningEffort'}[variant]]='changed';input.sessionFlagsDigest=hash(renewalStable(input.observation.sessionFlags));assert.equal(n.seal(input).reason,'renewal-intent-changed');
 }else assert.equal(captured.status,1,JSON.stringify({variant,captured}));
 assert.equal(r.f.run('verify').status,1);
});
renewalCheck('read-only narrowing is allowed but does not revive the old dispatch',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();n.value.profileId='trusted-live-narrow';n.value.sandboxState.permissionProfile.file_system.entries[1].access='read';assert.equal(n.renew().status,0);assert.equal(n.seal().status,0);assert.equal(n.run('prepare').status,0);assert.equal(r.f.run('verify').reason,'revoked-runtime-provenance');
});
renewalCheck('replacement refuses a live original and survivor refuses a departed original',async r=>{
 assert.equal(r.revoke().status,0);assert.equal(r.next().renew('replacement').reason,'renewal-owner-alive');const ended=new Promise(done=>r.f.owner.once('close',done));r.f.owner.kill();await ended;assert.equal(r.next().renew('survivor').status,1);
});
renewalCheck('renewed lineage remains checked on every verification',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();assert.equal(n.renew().status,0);assert.equal(n.seal().status,0);assert.equal(n.run('prepare').status,0);writeFileSync(join(r.f.app,'AGENTS.override.md'),'later injection',{mode:0o644});assert.equal(n.run('verify').reason,'source-changed');
});
renewalCheck('an aborted renewal reservation can be revoked and retried at a higher generation',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();assert.equal(n.renew().status,0);assert.equal(n.run('revoke-runtime',['--capture-id',n.id]).status,0);const retry=r.next();retry.value.generation.generation=n.value.generation.generation+1;assert.equal(retry.renew().status,0);assert.equal(retry.seal().status,0);assert.equal(retry.run('prepare').status,0);assert.equal(n.run('seal-runtime',['--capture-id',n.id,'--runtime-pid',String(r.f.owner.pid),'--sources',join(r.f.privateRoot,'owning-sources.json')]).reason,'revoked-runtime-provenance');
});

renewalCheck('managed replacement after actual exit preserves lineage and rejects old authority',async r=>{
 const ended=new Promise(done=>r.f.owner.once('close',done));r.f.owner.kill();await ended;assert.equal(r.revoke().status,0);const n=r.next();assert.equal(n.renew('replacement').status,0);await pause(40);
 const child=spawn('/usr/bin/bash',[r.f.managerPath,'cli','--','--live-policy-owning-runtime','--manifest',n.manifest,'--capture-id',n.id],{cwd:r.f.privateRoot,env:{PATH:dirname(process.execPath)+':/usr/bin:/bin',HOME:r.f.privateRoot,CODEX_HOME:r.f.home,GESTALT_HOME:join(r.f.privateRoot,'manager-state')},stdio:['pipe','pipe','pipe']});
 try{let output='',errors='';child.stdout.on('data',part=>output+=part);child.stderr.on('data',part=>errors+=part);await waitFor(()=>output==='owner-ready\n'||child.exitCode!==null);assert.equal(child.exitCode,null,JSON.stringify({output,errors}));const input=n.inventory(child.pid);const sealed=n.seal(input);assert.equal(sealed.status,0,JSON.stringify(sealed));assert.equal(n.run('prepare').status,0);assert.equal(n.run('verify').status,0);assert.equal(r.f.run('verify').reason,'revoked-runtime-provenance');}
 finally{if(child.exitCode===null&&child.signalCode===null){const closed=new Promise(done=>child.once('close',done));child.kill();await closed;}}
});
renewalCheck('unmanaged replacement cannot seal despite matching executable argv and environment',async r=>{
 const ended=new Promise(done=>r.f.owner.once('close',done));r.f.owner.kill();await ended;assert.equal(r.revoke().status,0);const n=r.next();assert.equal(n.renew('replacement').status,0);await r.f.start();assert.equal(n.seal(n.inventory(r.f.owner.pid)).reason,'unverified-owning-runtime');
});

renewalCheck('a signed exited label cannot conceal an actually live old helper',async r=>{
 const helper=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{detached:true,stdio:'ignore'});
 try{const processRecord=identity(helper.pid),body={kind:'process-launch',status:'exited',process:processRecord,owner:processRecord,processGroup:helper.pid};writeFileSync(join(r.f.privateRoot,`process-${randomUUID()}.json`),JSON.stringify({body,mac:createHmac('sha256',readFileSync(r.f.m.keyFile)).update(renewalStable(body)).digest('hex')}),{mode:0o600});assert.equal(r.revoke().status,0);assert.equal(r.next().renew().reason,'renewal-undrained');}
 finally{if(helper.exitCode===null&&helper.signalCode===null){const closed=new Promise(done=>helper.once('close',done));process.kill(-helper.pid,'SIGKILL');await closed;}}
});
renewalCheck('old election or generation cannot be relabeled for a successor controller',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();n.value.election.holder.birth='0';assert.equal(n.renew().reason,'stale-election');
});
renewalCheck('freshly observed config mismatch fails closed at sealing',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();assert.equal(n.renew().status,0);const input=n.inventory();input.effectiveConfigDigest=hash('different Native config');assert.equal(n.seal(input).reason,'renewal-intent-changed');
});
renewalCheck('successive renewals preserve all revoked ancestors',async r=>{
 assert.equal(r.revoke().status,0);const first=r.next();assert.equal(first.renew().status,0);assert.equal(first.seal().status,0);assert.equal(first.run('prepare').status,0);assert.equal(first.run('revoke-runtime',['--capture-id',first.id]).status,0);
 const second=r.next(first);assert.equal(second.renew().status,0);assert.equal(second.seal().status,0);assert.equal(second.run('prepare').status,0);assert.equal(first.run('verify').reason,'revoked-runtime-provenance');assert.equal(r.f.run('verify').reason,'revoked-runtime-provenance');
});
renewalCheck('concurrent renewals publish only one fresh capture and old revoke cannot release it',async r=>{
 assert.equal(r.revoke().status,0);const candidates=[r.next(),r.next()];
 const results=await Promise.all(candidates.map(n=>new Promise(done=>{n.save();const child=spawn(process.execPath,['--input-type=module','-','renew-runtime','--manifest',n.manifest,'--capture-id',n.id,'--previous-manifest',r.f.manifest,'--previous-capture-id',r.f.id,'--mode','survivor'],{env:{...process.env,NODE_OPTIONS:''},stdio:['pipe','pipe','pipe']});let output='',error='';child.stdout.on('data',part=>output+=part);child.stderr.on('data',part=>error+=part);child.once('close',status=>{assert.equal(error,'');done({n,status,...JSON.parse(output)});});child.stdin.end(r.f.program);})));assert.equal(results.filter(v=>v.status===0).length,1);assert.equal(results.filter(v=>v.status===1).length,1);
 const winner=results.find(v=>v.status===0).n,fence=JSON.parse(readFileSync(join(r.f.privateRoot,`runtime-admission-${hash(r.f.app)}.json`))).body;assert.equal(fence.captureId,winner.id);assert.equal(fence.status,'captured');assert.equal(r.revoke().reason,'unowned-runtime-admission');assert.ok(existsSync(join(r.f.privateRoot,`runtime-admission-${hash(r.f.app)}.lock`)));
});
renewalCheck('crash after fence publication cannot admit work and can be revoked for a newer retry',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();n.save();
 // Test-only process death at the publication boundary; no production fault flag.
 const code=r.f.program.replace('writeReceipt(paths.receipt,next,keyFile);\n    const clock','writeReceipt(paths.receipt,next,keyFile);process.exit(77);\n    const clock');assert.notEqual(code,r.f.program);
 const crashed=spawnSync(process.execPath,['--input-type=module','-','renew-runtime','--manifest',n.manifest,'--capture-id',n.id,'--previous-manifest',r.f.manifest,'--previous-capture-id',r.f.id,'--mode','survivor'],{input:code,encoding:'utf8',env:{...process.env,NODE_OPTIONS:''}});assert.equal(crashed.status,77);assert.equal(crashed.stdout,'');assert.equal(n.run('verify').status,1);assert.equal(r.f.run('verify').reason,'revoked-runtime-provenance');assert.equal(n.run('revoke-runtime',['--capture-id',n.id]).status,0);
 const retry=r.next();retry.value.generation.generation=3;assert.equal(retry.renew().status,0);assert.equal(retry.seal().status,0);assert.equal(retry.run('prepare').status,0);
});

renewalCheck('generated admission configs stay separate from original Native config lineage',async r=>{
 const oldGenerated=join(r.f.privateRoot,'old-generated.toml');writeFileSync(oldGenerated,'generated admission fixture',{mode:0o600});r.f.m.configFiles.push({path:oldGenerated,sha256:hash(readFileSync(oldGenerated))});assert.equal(r.f.run('prepare').status,0);assert.equal(r.revoke().status,0);
 const n=r.next();n.value.configFiles=n.value.configFiles.filter(file=>file.path!==oldGenerated);assert.equal(n.renew().status,0);assert.equal(n.seal().status,0);assert.equal(n.run('prepare').status,0);
 const freshGenerated=join(dirname(n.manifest),'fresh-generated.toml');writeFileSync(freshGenerated,'new generated admission fixture',{mode:0o600});n.value.configFiles.push({path:freshGenerated,sha256:hash(readFileSync(freshGenerated))});assert.equal(n.run('prepare').status,0);assert.equal(n.run('verify').status,0);
 writeFileSync(oldGenerated,'changed archived config',{mode:0o600});assert.equal(n.run('verify').reason,'config-changed');
});

provenanceCheck('original owning-home override absence is captured and rechecked after sealing',async f=>{
 const home=join(f.privateRoot,'original-home');mkdirSync(home,{mode:0o700});copyFileSync(join(f.home,'config.toml'),join(home,'config.toml'));chmodSync(join(home,'config.toml'),0o600);f.m.owningRuntime.home=home;f.m.configFiles.push({path:join(home,'config.toml'),sha256:hash(readFileSync(join(home,'config.toml')))});
 await f.captureAndStart();const path=join(home,'AGENTS.override.md');assert.equal(JSON.parse(readFileSync(join(f.privateRoot,`birth-${f.id}.json`))).body.instructions[path],null);assert.equal(f.seal().status,0);assert.equal(f.prepare().status,0);writeFileSync(path,'new original-home override',{mode:0o644});assert.equal(f.run('verify').reason,'source-changed');
});

renewalCheck('implicit original home can become explicit without relocating owning authority',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();n.value.owningRuntime.home=r.f.home;assert.equal(n.renew().status,0);assert.equal(n.seal().status,0);assert.equal(n.run('prepare').status,0);
},{explicitHome:false});
renewalCheck('failed seal publication revokes and releases only its own reservation',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();assert.equal(n.renew().status,0);const prior=r.f.program;
 r.f.program=prior.replace('try{m.runtimeProvenance={captureId:id','try{if(before.renewal)fail("renewal-test-publication-failure");m.runtimeProvenance={captureId:id');assert.notEqual(r.f.program,prior);
 try{assert.equal(n.seal().reason,'renewal-test-publication-failure');}finally{r.f.program=prior;}
 assert.equal(existsSync(join(r.f.privateRoot,`runtime-admission-${hash(r.f.app)}.lock`)),false);const fence=JSON.parse(readFileSync(join(r.f.privateRoot,`runtime-admission-${hash(r.f.app)}.json`))).body;assert.equal(fence.status,'revoked');assert.equal(fence.captureId,n.id);
 const retry=r.next();retry.value.generation.generation=3;assert.equal(retry.renew().status,0);assert.equal(retry.seal().status,0);assert.equal(retry.run('prepare').status,0);
});
renewalCheck('kernel transition guard releases after verifier death without deleting lock state',async r=>{
 assert.equal(r.revoke().status,0);const n=r.next();n.save();const code=r.f.program.replace('async function renewRuntime(manifestPath,id,previousPath,previousId,mode) {','async function renewRuntime(manifestPath,id,previousPath,previousId,mode) {process.exit(88);');assert.notEqual(code,r.f.program);
 const crashed=spawnSync(process.execPath,['--input-type=module','-','renew-runtime','--manifest',n.manifest,'--capture-id',n.id,'--previous-manifest',r.f.manifest,'--previous-capture-id',r.f.id,'--mode','survivor'],{input:code,encoding:'utf8',env:{...process.env,NODE_OPTIONS:''}});assert.equal(crashed.status,88);assert.equal(crashed.stdout,'');
 let result;for(let attempt=0;attempt<10;attempt++){result=n.renew();if(result.reason!=='admission-transition-busy')break;await pause(25);}assert.equal(result.status,0,JSON.stringify(result));assert.equal(n.seal().status,0);assert.equal(n.run('prepare').status,0);
});
for(const mode of ['original','survivor'])renewalCheck('actual serialized supervisor accepts observed '+mode+' and revokes its child',async r=>{
 // Non-Native command dispatcher; the actual process/closure/lineage checks run.
 r.f.m.commands={ordinary:{executable:r.f.m.binary.path,sha256:r.f.m.binary.sha256,args:['-e',running()],timeoutMs:30000}};
 assert.equal(r.f.run('prepare').status,0);let manifest=r.f.manifest;
 if(mode==='survivor'){assert.equal(r.revoke().status,0);const next=r.next();assert.equal(next.renew().status,0);assert.equal(next.seal().status,0);assert.equal(next.run('prepare').status,0);manifest=next.manifest;}
 writeFileSync(join(dirname(manifest),'sandbox'),`const{spawn}=require('node:child_process');const a=process.argv.slice(2),i=a.indexOf('--');const c=spawn(a[i+1],a.slice(i+2),{stdio:'inherit'});c.on('exit',code=>process.exit(code??1));`,{mode:0o600});
 const id=randomUUID(),helper=spawn(process.execPath,['--input-type=module','-','run','--manifest',manifest,'--command','ordinary','--launch-id',id],{stdio:['pipe','pipe','pipe','pipe']});
 let output='',errors='';helper.stdout.on('data',chunk=>output+=chunk);helper.stderr.on('data',chunk=>errors+=chunk);const ended=new Promise(done=>helper.once('close',done));helper.stdin.end(r.f.program);helper.stdio[3].end();helper.once('exit',()=>helper.stdio[3].destroy());
 try{
  await Promise.race([waitFor(()=>output.includes('ready')),ended.then(()=>assert.fail('supervisor exited before admitted command: '+errors))]);
  const receipt=JSON.parse(readFileSync(join(dirname(manifest),`process-${id}.json`))).body;assert.equal(receipt.kind,'process-launch');assert.ok(receipt.policy.provenance.inventory.observation);
  if(mode==='survivor')assert.ok(receipt.policy.provenance.renewalDigest);
  assert.equal(r.invoke(manifest,'stop',['--launch-id',id]).status,0);await ended;
  assert.equal(errors,'');assert.equal(JSON.parse(readFileSync(join(dirname(manifest),`process-${id}.json`))).body.status,'exited');
 }finally{if(helper.exitCode===null&&helper.signalCode===null){helper.kill('SIGKILL');await ended;}}
});
test('renewal Native departed child requires notLoaded plus absence in actual loaded registry',async()=>{
 const calls=[];const request=async(method,params)=>{calls.push(method);if(method==='thread/loaded/list')return {data:[],nextCursor:null};if(method==='thread/read')return {thread:{id:params.threadId,status:{type:'notLoaded'},turns:[]}};if(method==='thread/unsubscribe')return {status:'notLoaded'};throw Error('terminal observation is unsupported for unloaded Native threads');};
 const result=await quiesceNative(request,['departed-child']);assert.equal(result.observed[0].terminals,'unloaded');assert.equal(calls.includes('thread/backgroundTerminals/list'),false);
 await assert.rejects(quiesceNative(async(method,params)=>method==='thread/loaded/list'?{data:['departed-child'],nextCursor:null}:request(method,params),['departed-child']));
});
renewalCheck('failed intent reservations require explicit revoke before fresh survivor retry',async r=>{
 assert.equal(r.revoke().status,0);let generation=2;
 for(const field of ['account','modelProvider','model','reasoningEffort']){
  const next=r.next();next.value.generation.generation=generation;assert.equal(next.renew().status,0);
  const input=next.inventory();if(field==='account')input.observation.accountDigest=hash('changed-account');else input.observation.sessionFlags[field]='changed';input.sessionFlagsDigest=hash(renewalStable(input.observation.sessionFlags));
  assert.equal(next.seal(input).reason,'renewal-intent-changed');assert.equal(next.run('verify').status,1);
  const premature=r.next();premature.value.generation.generation=generation+1;assert.equal(premature.renew().reason,'renewal-not-revoked');
  assert.equal(next.run('revoke-runtime',['--capture-id',next.id]).status,0);assert.equal(r.f.run('verify').reason,'revoked-runtime-provenance');generation++;
 }
 const valid=r.next();valid.value.generation.generation=generation;assert.equal(valid.renew().status,0);assert.equal(valid.seal().status,0);assert.equal(valid.run('prepare').status,0);assert.equal(valid.run('verify').status,0);
});
test('renewal Native unsubscribe preserves a measured idle cache without claiming worker shutdown',async()=>{
 const request=async(method,params)=>{if(method==='thread/loaded/list')return {data:['root'],nextCursor:null};if(method==='thread/read')return {thread:{id:params.threadId,status:{type:'idle'},turns:[]}};if(method==='thread/backgroundTerminals/list')return {data:[],nextCursor:null};if(method==='thread/unsubscribe')return {status:'unsubscribed'};};
 const result=await quiesceNative(request,['root']);assert.equal(result.unsubscribed,true);assert.equal(result.cachedThreads,1);assert.equal(Object.hasOwn(result,'unloaded'),false);
});
test('renewal Native internal loaded thread still requires actual empty terminal registry',async()=>{
 let terminals=0;const request=async(method,params)=>{if(method==='thread/loaded/list')return {data:['internal-child'],nextCursor:null};if(method==='thread/read'){assert.equal(params.includeTurns,false);return {thread:{id:params.threadId,status:{type:'notLoaded'},turns:[]}};}if(method==='thread/backgroundTerminals/list'){terminals++;return {data:[],nextCursor:null};}if(method==='thread/unsubscribe')return {status:'notSubscribed'};};
 const result=await quiesceNative(request,['internal-child']);assert.equal(result.cachedThreads,1);assert.equal(terminals,1);assert.equal(result.observed[0].terminals,0);assert.equal(result.observed[0].historyObserved,false);
 await assert.rejects(quiesceNative(async(method,params)=>method==='thread/backgroundTerminals/list'?{data:[{}],nextCursor:null}:request(method,params),['internal-child']));
});
