import assert from 'node:assert/strict';
import {spawn,spawnSync} from 'node:child_process';
import {mkdtemp,mkdir,writeFile,readFile,readdir,rm,stat,open,rmdir} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import test from 'node:test';
import {sessionScratch} from './session-scratch-fixture.mjs';

const manager=resolve('public/gestalt');
const rootThread='01a19bd7-bdb5-76e0-bc41-28d650392275',childThread='01a19bd7-bdb5-76e0-bc41-28d650392276';
async function fixture(t) {
  const root=await mkdtemp(join(tmpdir(),'gestalt manager entry ')),home=join(root,'home'),codexHome=join(root,'codex-home'),managed=join(root,'managed'),bin=join(root,'bin');
  const workspace=join(root,'selected workspace'),other=join(root,'other workspace'),log=join(root,'native.json');
  for(const path of [home,codexHome,bin,workspace,other,join(managed,'runtime/context-mode')])await mkdir(path,{recursive:true,mode:0o700});
  t.after(()=>rm(root,{recursive:true,force:true}));
  const runtime=join(managed,'runtime/context-mode');
  await writeFile(join(runtime,'.context-mode-prepared.json'),JSON.stringify({packageVersion:'0.0.1',nodeModulesAbi:process.versions.modules,platform:process.platform,arch:process.arch}));
  for(const name of ['cli.bundle.mjs','server.bundle.mjs'])await writeFile(join(runtime,name),'export {};\n');
  await writeFile(join(bin,'npm'),'#!/bin/sh\nif [ "${1:-}" = --version ]; then echo 10.0.0; else exit 99; fi\n',{mode:0o700});
  // Only installed-component discovery and provider I/O are fixtures. The
  // actual public manager, allocation, native argv, hooks and guardians run.
  await writeFile(join(bin,'codex'),`#!${process.execPath}
const fs=require('node:fs'),{spawnSync}=require('node:child_process'),{createInterface}=require('node:readline');
const args=process.argv.slice(2),codexHome=${JSON.stringify(codexHome)},log=${JSON.stringify(log)};
if(args[0]==='plugin'&&args[1]==='list'){console.log(JSON.stringify([{name:'gestalt',version:'0.0.1',installed:true,enabled:true},{name:'context-mode',version:'0.0.1',installed:true,enabled:false}]));process.exit();}
if(args[0]==='mcp'&&args[1]==='get'){console.log(JSON.stringify({enabled:true,command:'node',args:[codexHome+'/bin/context-mode-mcp.mjs']}));process.exit();}
if(args.includes('--version')||args.includes('-V')){console.log('codex-cli 0.160.0');process.exit();}
if(args[0]==='doctor'){process.exit();}
if(args[0]==='sandbox'){process.exit(1);}
const hooks=()=>args.filter(v=>v.startsWith('hooks.SessionStart=')||v.startsWith('hooks.SubagentStart=')).map((v,index)=>({event:v.startsWith('hooks.SessionStart=')?'SessionStart':'SubagentStart',command:JSON.parse(v.match(/"command"=("(?:[^"\\\\]|\\\\.)*")/)[1]),key:'fixture-'+index,currentHash:'sha256:fixture',source:'sessionFlags'}));
if(args[0]==='app-server'){
 createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);if(m.id===undefined)return;let result={};
 if(m.method==='initialize')result={userAgent:'codex_cli_rs/0.160.0'};
 if(m.method==='config/read')result={config:{features:{hooks:true},mcp_servers:{},skills:{config:[]}},layers:[{name:{type:'sessionFlags'},config:{}},{name:{type:'user'},config:{features:{hooks:true}}}]};
 if(m.method==='skills/list')result={data:[{cwd:m.params.cwds[0],skills:[]}]};
 if(m.method==='hooks/list')result={data:[{cwd:m.params.cwds[0],hooks:hooks()}]};
 console.log(JSON.stringify({id:m.id,result}));});
}else{
 if(process.env.GESTALT_STARTUP_DIAGNOSTICS_DONE!=='1')process.exit(1);
 let stdin='';process.stdin.on('data',b=>stdin+=b);process.stdin.on('end',()=>{
 const context=process.env.GESTALT_SESSION_TMP_CONTEXT?JSON.parse(process.env.GESTALT_SESSION_TMP_CONTEXT):null;
 const results=[];for(const h of hooks()){
  const payload={hook_event_name:h.event,session_id:${JSON.stringify(rootThread)},cwd:context?.workspacePath,source:process.env.FIXTURE_SOURCE||'startup',agent_id:${JSON.stringify(childThread)}};
  const r=spawnSync('bash',['-c',h.command],{input:JSON.stringify(payload),env:process.env,encoding:'utf8',timeout:5000});results.push(r.status);if(r.status!==0){process.stderr.write(r.stderr);process.exit(91);}
 }
 fs.writeFileSync(log,JSON.stringify({args,stdin,context,hookExits:results,environment:Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'].map(k=>[k,process.env[k]]))}));
 process.stdout.write('NATIVE_STDOUT\\n');});
}
`,{mode:0o700});
  const env={...process.env,HOME:home,CODEX_HOME:codexHome,GESTALT_HOME:managed,GESTALT_STARTUP_DIAGNOSTICS_DONE:'1',PATH:bin+':'+process.env.PATH};
  if(Object.hasOwn(env,'GESTALT_SESSION_TMP_CONTEXT'))for(const key of ['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'])delete env[key];
  const invoke=async(args,source='startup',overrides={})=>{
    const child=spawn('bash',[manager,'cli','--',...args],{cwd:root,env:{...env,FIXTURE_SOURCE:source,...overrides},stdio:['pipe','pipe','pipe']});
    const exited=new Promise((r,reject)=>{child.once('error',reject);child.once('close',r);});
    child.stdin.on('error',()=>{});
    let stdout='',stderr='';child.stdout.on('data',v=>stdout+=v);child.stderr.on('data',v=>stderr+=v);child.stdin.end('original stdin bytes\n');
    let killTimer;
    const timer=setTimeout(()=>{if(child.exitCode===null&&child.signalCode===null)child.kill('SIGTERM');
      killTimer=setTimeout(()=>{if(child.exitCode===null&&child.signalCode===null)child.kill('SIGKILL');},500);},20000);
    try{return{code:await exited,stdout,stderr};}finally{clearTimeout(timer);clearTimeout(killTimer);}
  };
  const run=async(args,source='startup')=>{
    const {code,stdout,stderr}=await invoke(args,source);
    assert.equal(code,0,stderr);assert.match(stdout,/Gestalt manager 0\.12\.0/);
    assert.equal(stdout.match(/NATIVE_STDOUT/g)?.length,1);assert.ok(stdout.endsWith('NATIVE_STDOUT\n'));
    return {value:JSON.parse(await readFile(log,'utf8')),stderr};
  };
  return {root,workspace,other,run,invoke};
}
test('public manager preserves native selectors/images/permissions/stdin and publishes observed root and child IDs per invocation',{timeout:60000},async t=>{
  const f=await fixture(t),contexts=[];
  for(const [workspace,selector] of [[f.workspace,['resume','--last']],[f.workspace,['resume','thread name']],[f.other,['resume']]]) {
    const args=['-C',workspace,'-s','read-only','-a','on-request','-i',join(f.root,'image with spaces.png'),...selector,'MANAGER_FIXTURE_PROMPT'];
    const {value}=await f.run(args,'resume');
    assert.deepEqual(value.args.slice(1,1+args.length),args);assert.equal(value.stdin,'original stdin bytes\n');
    assert.deepEqual(value.hookExits,[0,0]);assert.equal(value.context.workspacePath,workspace);
    for(const key of ['TMPDIR','TMP','TEMP'])assert.equal(value.environment[key],value.context.directory);
    const marker=JSON.parse(await readFile(value.context.markerPath,'utf8'));
    assert.deepEqual(marker.providerThreads,[{provider:'codex',threadId:rootThread},{provider:'codex',threadId:childThread}]);
    assert.match(marker.ownerId,/^cli-/);assert.notEqual(marker.sessionId,rootThread);
    for(const name of await readdir(join(value.context.directory,'leases'))) {
      const lease=JSON.parse(await readFile(join(value.context.directory,'leases',name),'utf8'));assert.equal(lease.state,'released');
    }
    assert.equal((await stat(value.context.directory)).mode&0o777,0o700);contexts.push(value.context);
  }
  assert.notEqual(contexts[0].ownerId,contexts[1].ownerId);assert.notEqual(contexts[0].workspacePath,contexts[2].workspacePath);
  await assert.rejects(stat(join(f.workspace,'.gestalt','cli-session-owners')),{code:'ENOENT'});
});
test('public manager reports unselected workspace without inventing an owner or changing native argv',{timeout:30000},async t=>{
  const f=await fixture(t),args=['resume','--last','MANAGER_FIXTURE_PROMPT'];const {value,stderr}=await f.run(args);
  assert.equal(value.context,null);assert.deepEqual(value.args.slice(1,1+args.length),args);assert.match(stderr,/no selected workspace/);
  await assert.rejects(stat(join(f.root,'tmp')),{code:'ENOENT'});
});

test('public CLI nested publication fails under an independent Mobile containment fence and succeeds after release',{timeout:30000},async t=>{
  const f=await fixture(t),outer=await sessionScratch(f.workspace,'outer-mobile');
  const selected=join(outer.context.directory,'selected workspace'),lock=join(outer.context.directory,'.containment-lock');
  await mkdir(selected,{mode:0o700});await mkdir(lock,{mode:0o700});
  const held=await open(lock,'r'),identity=await held.stat();
  try{
    const result=await f.invoke(['-C',selected,'resume','--last','MANAGER_FIXTURE_PROMPT']);
    assert.notEqual(result.code,0);assert.match(result.stderr,/temporary ownership unavailable/);
    await assert.rejects(stat(join(selected,'tmp')),{code:'ENOENT'});
    const current=await stat(lock);assert.equal(current.dev,identity.dev);assert.equal(current.ino,identity.ino);
  }finally{
    try{const current=await stat(lock);assert.equal(current.dev,identity.dev);assert.equal(current.ino,identity.ino);await rmdir(lock);}
    finally{await held.close();}
  }
  const {value}=await f.run(['-C',selected,'resume','--last','MANAGER_FIXTURE_PROMPT']);
  assert.equal(value.context.workspacePath,selected);assert.equal(value.stdin,'original stdin bytes\n');
  await assert.rejects(stat(lock),{code:'ENOENT'});
});

test('public CLI holds ancestor containment through full metadata publication against independent cleanup claim',{timeout:30000},async t=>{
  const f=await fixture(t),outer=await sessionScratch(f.workspace,'outer-mobile-publisher');
  const selected=join(outer.context.directory,'selected workspace'),lock=join(outer.context.directory,'.containment-lock');
  await mkdir(selected,{mode:0o700});
  const paused=join(f.root,'publisher-paused'),release=join(f.root,'release-publisher'),preload=join(f.root,'publication preload.cjs');
  await writeFile(preload,`const fs=require('node:fs'),original=fs.openSync;
fs.openSync=function(path,...args){
 if(typeof path==='string'&&path.startsWith(${JSON.stringify(join(selected,'tmp/sessions/cli-'))})&&path.endsWith('/.gestalt-owner.json')&&args[0]==='wx'){
  fs.writeFileSync(${JSON.stringify(paused)},'held');const deadline=Date.now()+10000;
  while(!fs.existsSync(${JSON.stringify(release)})){if(Date.now()>=deadline)throw Error('bounded publication fixture deadline');Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,10);}
 }
 return original.call(fs,path,...args);
};
`);
  const pending=f.invoke(['-C',selected,'exec','MANAGER_FIXTURE_PROMPT'],'startup',{NODE_OPTIONS:'--require '+JSON.stringify(preload)})
    .then(value=>({value}),error=>({error}));
  let result;
  try{
    const deadline=Date.now()+6000;
    while(true){try{await stat(paused);break;}catch(error){if(error.code!=='ENOENT')throw error;}
      assert.ok(Date.now()<deadline,'publisher did not reach controlled metadata boundary');await new Promise(r=>setTimeout(r,20));}
    assert.equal((await stat(lock)).mode&0o777,0o700);
    const cleanup=spawnSync(process.execPath,['-e',`const fs=require('node:fs');try{fs.mkdirSync(process.argv[1],{mode:0o700});fs.rmdirSync(process.argv[1]);console.log('claimed');}catch(e){if(e.code!=='EEXIST')throw e;console.log('busy');}`,lock],
      {encoding:'utf8',timeout:2000});
    assert.equal(cleanup.status,0,cleanup.stderr);assert.equal(cleanup.stdout,'busy\n');
    const owners=(await readdir(join(selected,'tmp/sessions'))).filter(name=>name.startsWith('cli-'));assert.equal(owners.length,1);
    const directory=join(selected,'tmp/sessions',owners[0]),leases=await readdir(join(directory,'leases'));
    assert.equal(leases.length,1);const lease=JSON.parse(await readFile(join(directory,'leases',leases[0]),'utf8'));
    assert.equal(lease.schemaVersion,1);assert.equal(lease.ownerId,owners[0]);assert.equal(lease.state,'active');
    await assert.rejects(stat(join(directory,'.gestalt-owner.json')),{code:'ENOENT'});
  }finally{await writeFile(release,'continue');const settled=await pending;if(settled.error)throw settled.error;result=settled.value;}
  assert.equal(result.code,0,result.stderr);assert.ok(result.stdout.endsWith('NATIVE_STDOUT\n'));
  await assert.rejects(stat(lock),{code:'ENOENT'});
});
