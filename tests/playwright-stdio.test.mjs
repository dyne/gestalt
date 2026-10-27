import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createInterface} from 'node:readline';
import {mkdtemp,mkdir,readFile,writeFile,rm,stat,symlink} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {tmpdir} from 'node:os';
import test from 'node:test';
const manager=resolve('public/gestalt'),source=await readFile(manager,'utf8');
const program=source.split("cat <<'PLAYWRIGHT_JS'\n")[1].split('\nPLAYWRIGHT_JS\n')[0];
const digest=b=>createHash('sha256').update(b).digest('hex');
const pause=ms=>new Promise(r=>setTimeout(r,ms));
const tools=['browser_navigate','browser_snapshot','browser_fill_form','browser_take_screenshot'].map(name=>({name,inputSchema:{type:'object',properties:{}}}));
async function fixture(t){
 const base=await mkdtemp(join(tmpdir(),'playwright stdio ')),home=join(base,'managed'),workspace=join(base,'workspace with spaces'),bin=join(base,'bin');
 for(const p of [home,workspace,bin])await mkdir(p,{recursive:true});
 t.after(()=>rm(base,{recursive:true,force:true}));
 const release=join(home,'playwright/release-0.0.83-fixture'),pkg=join(release,'node_modules/@playwright/mcp');await mkdir(pkg,{recursive:true});
 const executable=join(pkg,'cli.js');
 const server=`const fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
const config=JSON.parse(fs.readFileSync(process.argv[process.argv.indexOf('--config')+1]));
const profile=config.browser.userDataDir;
fs.appendFileSync(${JSON.stringify(join(base,'browser-events'))},JSON.stringify({pid:process.pid,config,env:process.env})+'\\n');
const worker=spawn(process.execPath,['-e',"process.on('SIGTERM',()=>{});setInterval(()=>{},1000)"],{stdio:'ignore'});
process.on('SIGTERM',()=>process.exit());
require('node:readline').createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);if(m.id===undefined)return;
let result={protocolVersion:'2024-11-05',capabilities:{tools:{}},serverInfo:{name:'fixture',version:'1'}};
if(m.method==='tools/list')result={tools:[...${JSON.stringify(tools)},...['browser_run_code_unsafe','browser_evaluate','browser_install'].map(name=>({name,inputSchema:{type:'object'}}))]};
if(m.method==='tools/call') {if(m.params.arguments.cookie)fs.writeFileSync(path.join(profile,'cookie'),m.params.arguments.cookie);result={content:[{type:'text',text:JSON.stringify({pid:process.pid,worker:worker.pid,profile,cookie:fs.existsSync(path.join(profile,'cookie'))?fs.readFileSync(path.join(profile,'cookie'),'utf8'):null})}]};}
console.log(JSON.stringify({jsonrpc:'2.0',id:m.id,result}));});`;
 await writeFile(executable,server);await writeFile(join(pkg,'package.json'),JSON.stringify({version:'0.0.83'}));
 const sri='sha512-'+'A'.repeat(86)+'==',lock=JSON.stringify({packages:{'node_modules/@playwright/mcp':{version:'0.0.83',integrity:sri}}});
 await writeFile(join(release,'package-lock.json'),lock);
 await writeFile(join(home,'playwright/active.json'),JSON.stringify({schemaVersion:1,contractVersion:1,version:'0.0.83',integrity:sri,node:'>=20',release,lockSha256:digest(lock),executableSha256:digest(server),tools:tools.map(t=>t.name),toolSchemas:tools}));
 const chrome=join(base,'Chrome Fixture');await writeFile(chrome,'#!/bin/sh\necho "Google Chrome 140.0.1.2"\n',{mode:0o755});
 // Any nested Codex launch is a regression, including a shell-sandbox proxy.
 await writeFile(join(bin,'codex'),`#!${process.execPath}
require('node:fs').appendFileSync(${JSON.stringify(join(base,'sandbox-events'))},'unexpected Codex invocation');process.exit(99);`,{mode:0o755});
 const env={...process.env,PATH:bin+':'+process.env.PATH,GESTALT_HOME:home,PLAYWRIGHT_MCP_PORT:'9999',PLAYWRIGHT_MCP_NO_SANDBOX:'1',NODE_OPTIONS:''};
 function start(project=workspace,options={}){
  const p=options.public?spawn('bash',[manager,'playwright','mcp','--cwd',project,'--executable-path',chrome],{env,stdio:['pipe','pipe','pipe']}):spawn(process.execPath,['-e',program,home,'mcp','--cwd',project,'--executable-path',chrome],{env,stdio:['pipe','pipe','pipe']});
  const pending=new Map(),lines=createInterface({input:p.stdout});let id=0,stderr='';p.stderr.on('data',b=>stderr+=b);
  lines.on('line',line=>{const m=JSON.parse(line),w=pending.get(m.id);if(w){pending.delete(m.id);w(m);}});
  const request=(method,params={})=>new Promise((resolve,reject)=>{const n=++id,timer=setTimeout(()=>reject(Error('MCP deadline '+stderr)),8000);pending.set(n,m=>{clearTimeout(timer);resolve(m);});p.stdin.write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\n');});
  const call=(name='browser_snapshot',args={})=>request('tools/call',{name,arguments:args});
  const exited=new Promise(r=>p.once('exit',(code,signal)=>r({code,signal})));
  const close=async()=>{if(p.exitCode===null&&p.signalCode===null)p.stdin.end();await exited;lines.close();};
  t.after(close);return{p,request,call,close,exited,get stderr(){return stderr;}};
 }
 return {base,home,workspace,chrome,start};
}
const payload=m=>{assert.ok(!m.error,m.error?.message);return JSON.parse(m.result.content[0].text);};
async function dead(pid){for(let i=0;i<80;i++){try{const s=await readFile('/proc/'+pid+'/stat','utf8');if(s.split(' ')[2]==='Z')return;}catch(e){if(e.code==='ENOENT')return;}try{process.kill(pid,0);}catch(e){if(e.code==='ESRCH')return;}await pause(25);}assert.fail('owned process survived '+pid);}
async function gone(p){for(let i=0;i<80;i++){try{await stat(p);}catch(e){if(e.code==='ENOENT')return;}await pause(25);}assert.fail('owned lease survived '+p);}
test('offline catalog states host authority and denies unsafe tools and installs',async t=>{
 const f=await fixture(t),c=f.start();const init=await c.request('initialize');assert.equal(init.result.capabilities.experimental,undefined);
 assert.match(init.result.instructions,/Host-owned/);assert.match(init.result.instructions,/Shell sandbox roots do not constrain/);
 const list=await c.request('tools/list');assert.deepEqual(list.result.tools.map(t=>t.name),tools.map(t=>t.name));
 for(const name of ['browser_run_code_unsafe','browser_run_code','browser_evaluate','browser_install','webmcp_untrusted'])assert.match((await c.call(name)).error.message,/disabled/);
 await assert.rejects(stat(join(f.base,'sandbox-events')),{code:'ENOENT'});
});
test('ordinary MCP calls need no Codex metadata; Chrome sandbox, pipes and private state are managed',async t=>{
 const f=await fixture(t),c=f.start();const a=payload(await c.call());
 await assert.rejects(stat(join(f.base,'sandbox-events')),{code:'ENOENT'});
 const e=JSON.parse((await readFile(join(f.base,'browser-events'),'utf8')).trim());assert.equal(e.config.browser.launchOptions.chromiumSandbox,true);assert.equal(e.config.browser.launchOptions.headless,true);assert.equal(e.config.webmcp,false);assert.deepEqual(e.config.capabilities,[]);assert.equal(e.config.server,undefined);
 assert.equal(e.env.PLAYWRIGHT_MCP_PORT,undefined);assert.equal(e.env.PLAYWRIGHT_MCP_NO_SANDBOX,undefined);assert.equal(e.env.NODE_OPTIONS,undefined);
 assert.equal((await stat(a.profile)).mode&0o777,0o700);assert.equal((await stat(join(a.profile,'../lease/owner.json'))).mode&0o777,0o600);
 await c.close();await dead(a.pid);await dead(a.worker);await gone(join(a.profile,'../lease'));
});
test('same-workspace contention cannot disturb owner; independent workspaces run in parallel',async t=>{
 const f=await fixture(t),first=f.start(),a=payload(await first.call('browser_navigate',{cookie:'one'}));
 const second=f.start();assert.ok((await second.call()).error);assert.equal(payload(await first.call()).cookie,'one');
 const other=join(f.base,'other');await mkdir(other);const third=f.start(other);
 const b=payload(await third.call());assert.notEqual(a.profile,b.profile);assert.equal(b.cookie,null);assert.equal(payload(await first.call()).pid,a.pid);
});
for(const method of ['EOF','SIGTERM','SIGKILL'])test('cookies survive restart and all descendants stop after '+method,async t=>{
 const f=await fixture(t),c=f.start(),a=payload(await c.call('browser_navigate',{cookie:'persistent'}));
 if(method==='EOF')await c.close();else{c.p.kill(method);await c.exited;}
 await dead(a.pid);await dead(a.worker);
 // A hard killed backend can leave only a provably dead ownership record.
 const again=f.start();const b=payload(await again.call());assert.equal(b.cookie,'persistent');assert.equal(b.profile,a.profile);assert.equal((await stat(join(b.profile,'cookie'))).mode&0o777,0o600);
});
test('workspace aliases share a lease and managed state symlinks fail closed',async t=>{
 const f=await fixture(t),alias=join(f.base,'alias');await symlink(f.workspace,alias);
 const a=f.start(),p=payload(await a.call()),b=f.start(alias);assert.ok((await b.call()).error);await a.close();
 const state=resolve(p.profile,'..');await rm(join(state,'chrome'),{recursive:true});const outside=join(f.base,'outside');await mkdir(outside);await symlink(outside,join(state,'chrome'));
 const c=f.start();assert.ok((await c.call()).error);assert.deepEqual(await import('node:fs/promises').then(fs=>fs.readdir(outside)),[]);
});
test('public Bash launcher retains MCP stdin and cleans up on EOF',async t=>{
 const f=await fixture(t),c=f.start(f.workspace,{public:true});const a=payload(await c.call());await c.close();await dead(a.pid);await dead(a.worker);
});
test('stale lease recovery requires both owner and process group to be gone',async t=>{
 const f=await fixture(t),state=join(f.home,'playwright/workspaces',digest(f.workspace)),lease=join(state,'lease');
 await mkdir(lease,{recursive:true});
 const owner={schemaVersion:1,workspace:f.workspace,pid:2147483646,identity:'old-owner',group:2147483646,token:'a'.repeat(48)};
 await writeFile(join(lease,'owner.json'),JSON.stringify(owner));
 const c=f.start(),a=payload(await c.call());await c.close();await gone(lease);
 await mkdir(lease);await writeFile(join(lease,'owner.json'),JSON.stringify({...owner,pid:process.pid}));
 const second=f.start();assert.match((await second.call()).error.message,/browser busy/);
 assert.equal(JSON.parse(await readFile(join(lease,'owner.json'),'utf8')).pid,process.pid);
 await dead(a.pid);
});
test('public launcher crash closes the guardian lifetime pipe',async t=>{
 const f=await fixture(t),c=f.start(f.workspace,{public:true}),a=payload(await c.call());
 c.p.kill('SIGKILL');await c.exited;await dead(a.pid);await dead(a.worker);
 const again=f.start();payload(await again.call());
});
test('untrusted request metadata cannot alter host lifecycle or permissions',async t=>{
 const f=await fixture(t),c=f.start(),a=payload(await c.call());
 const b=payload(await c.request('tools/call',{name:'browser_snapshot',arguments:{},_meta:{'codex/sandbox-state-meta':{permissionProfile:{type:'disabled'}}}}));
 assert.equal(a.pid,b.pid);
 assert.match((await c.call('browser_run_code_unsafe')).error.message,/disabled/);
 assert.equal(payload(await c.call()).pid,a.pid);
 await assert.rejects(stat(join(f.base,'sandbox-events')),{code:'ENOENT'});
});
