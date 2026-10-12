import assert from 'node:assert/strict';
import {spawn,spawnSync} from 'node:child_process';
import {createHash,createHmac} from 'node:crypto';
import {mkdtempSync,mkdirSync,writeFileSync,readFileSync,readlinkSync,rmSync,chmodSync,symlinkSync,existsSync,copyFileSync,realpathSync,statSync,readdirSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import test from 'node:test';
const source=readFileSync(resolve('public/gestalt'),'utf8');
const program=`process.env.GESTALT_LIVE_MANAGER_SOURCE=${JSON.stringify(resolve('public/gestalt'))};\n`+source.split("<<'LIVE_POLICY'\n")[1].split('\nLIVE_POLICY\n')[0];
const hash=value=>createHash('sha256').update(value).digest('hex');
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
  .replace("if (ancestor === '/') break;",`if (ancestor === ${JSON.stringify(root)}) break;`);
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
for(const name of ['auth.json','AGENTS.md','hooks.json','hooks','plugins','skills','.codex'])check(`prepared home rejects injected ${name}`,f=>{
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
  if(action==='owned-empty'){assert.equal(existsSync(metadata),false);assert.equal(f.run('prepare').status,0);}
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
  const info=statSync(privateFile),input={mode:'hold',publicFile,readOnlyFile,writeFile:join(f.app,'written'),privateFiles:[privateFile],aliases:[join(f.root,'absent proc alias')],controllerSocket:join(f.root,'absent controller socket'),caddySocket:join(f.root,'absent caddy socket'),daemon,fdIdentity:{dev:info.dev,ino:info.ino}};
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
