import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {mkdtemp,mkdir,readFile,writeFile,rm,symlink} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import test from 'node:test';
const source=await readFile(resolve('public/gestalt'),'utf8');
const program=source.split('run_serena_proxy() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];
const catalog=['find_symbol','get_symbols_overview','initial_instructions','replace_symbol_body','activate_project','switch_modes','execute_shell_command'].map(name=>({name,description:name,inputSchema:{type:'object',properties:{}}}));
const pause=ms=>new Promise(r=>setTimeout(r,ms));
async function fixture({spawnError=false,rootKind='canonical'}={}){
 const root=await mkdtemp(join(tmpdir(),'serena-policy-')),workspace=join(root,'workspace with spaces');
 await mkdir(workspace);await mkdir(join(root,'bin'));
 const workspaceAlias=join(root,'workspace alias');
 if(rootKind==='symlink')await symlink(workspace,workspaceAlias);
 await writeFile(join(root,'install.json'),JSON.stringify({schemaVersion:1,version:'1.7.0',executable:join(root,'serena'),python:'/usr/bin/python3',uv:'/usr/bin/uv',pythonInstallDir:join(root,'python'),tools:catalog}));
 // Offline stand-in checks argv and exact policy propagation. Real confinement
 // is checked separately with the native Codex sandbox and disposable roots.
 await writeFile(join(root,'bin/codex'),`#!/usr/bin/env node
const fs=require('node:fs'),{spawn}=require('node:child_process');const args=process.argv.slice(2);
fs.appendFileSync(${JSON.stringify(join(root,'starts'))},JSON.stringify(args)+'\\n');
const descendant=spawn(process.execPath,['-e',"process.on('SIGTERM',()=>{});process.send('ready');setInterval(()=>{},1000)"],{stdio:['ignore','ignore','ignore','ipc']});
// Wait for the descendant's actual handler before returning any tool result.
const ready=new Promise(resolve=>descendant.once('message',resolve));
process.on('SIGTERM',()=>process.exit(0));
require('node:readline').createInterface({input:process.stdin}).on('line',async line=>{await ready;const m=JSON.parse(line);if(m.id===undefined)return;let result={};
if(m.method==='initialize')result={protocolVersion:m.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'Serena',version:'SDK'}};
if(m.method==='tools/list')result={tools:${JSON.stringify(catalog)}};
if(m.method==='tools/call')result={content:[{type:'text',text:JSON.stringify({pid:process.pid,descendant:descendant.pid,state:JSON.parse(args[2]),args})}],isError:false};
process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:m.id,result})+'\\n',()=>{if(m.params?.arguments?.exitLeader)process.exit(0);});});
`,{mode:0o755});
 const child=spawn(process.execPath,['--input-type=module','-'],{cwd:workspace,env:{...process.env,PATH:join(root,'bin')+':'+process.env.PATH,GESTALT_SERENA_WORKSPACE:rootKind==='relative'?'.':rootKind==='symlink'?workspaceAlias:workspace,GESTALT_SERENA_INSTALL:join(root,'install.json')},stdio:['pipe','pipe','pipe','pipe']});
 child.stdin.end(spawnError?program.replace('const child = spawn(process.execPath,',"const child = spawn('/nonexistent-serena-supervisor',"):program);let stderr='';child.stderr.on('data',x=>stderr+=x);
 const lines=createInterface({input:child.stdout}),pending=new Map();let id=0;
 lines.on('line',line=>{const m=JSON.parse(line),w=pending.get(m.id);if(w){pending.delete(m.id);w(m);}});
 const request=(method,params)=>new Promise((resolveRequest,reject)=>{const n=++id,timer=setTimeout(()=>reject(Error('proxy deadline: '+stderr)),5000);pending.set(n,m=>{clearTimeout(timer);resolveRequest(m);});child.stdio[3].write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\n');});
 const state={permissionProfile:{type:'managed',file_system:{type:'restricted',entries:[{path:{type:'path',path:workspace},access:'write'},{path:{type:'path',path:join(workspace,'.git')},access:'read'}]},network:'restricted'},sandboxCwd:new URL('file://'+workspace).href,useLegacyLandlock:false};
 const call=(name,policy=state)=>request('tools/call',{name,arguments:{},...(policy?{_meta:{'codex/sandbox-state-meta':policy}}:{})});
 const close=async()=>{child.stdio[3].end();if(child.exitCode===null&&child.signalCode===null)await new Promise(r=>child.once('exit',r));lines.close();await rm(root,{recursive:true,force:true});};
 return {root,workspace,child,request,state,call,close};
}
const payload=m=>{assert.ok(!m.error,m.error?.message);return JSON.parse(m.result.content[0].text);};
async function dead(pid){for(let i=0;i<40;i++){try{const status=await readFile(`/proc/${pid}/stat`,'utf8');if(status.split(' ')[2]==='Z')return;}catch(e){if(e.code==='ENOENT')return;}try{process.kill(pid,0);}catch(e){if(e.code==='ESRCH')return;}await pause(50);}assert.fail(`owned process ${pid} survived cleanup`);}
test('catalog exposes edits, excludes project/mode/shell tools, includes plan guidance and starts no process',async()=>{const f=await fixture();try{
 const init=await f.request('initialize',{protocolVersion:'2024-11-05'});assert.ok(init.result.capabilities.experimental['codex/sandbox-state-meta']);assert.match(init.result.instructions,/plan mode prohibits edits/);
 const list=await f.request('tools/list',{});assert.ok(list.result.tools.some(t=>t.name==='replace_symbol_body'));assert.ok(!list.result.tools.some(t=>/activate_project|switch_modes|execute_shell/.test(t.name)));
 assert.match((await f.call('replace_symbol_body',null)).error.message,/metadata required/);assert.match((await f.call('activate_project')).error.message,/tool unavailable/);
 await assert.rejects(readFile(join(f.root,'starts')),{code:'ENOENT'});
}finally{await f.close();}});
test('exact native state is retained, matching calls reuse process, narrowed/read-only policies restart it',async()=>{const f=await fixture();try{
 const first=payload(await f.call('replace_symbol_body'));assert.deepEqual(first.state,f.state);assert.equal(first.args[0],'sandbox');assert.equal(first.args[1],'--sandbox-state-json');
 assert.deepEqual(first.args.slice(first.args.indexOf('--context'),first.args.indexOf('--context')+4),['--context','codex','--mode','editing']);assert.deepEqual(first.args.slice(first.args.indexOf('--project'),first.args.indexOf('--project')+2),['--project',f.workspace]);
 assert.equal(payload(await f.call('find_symbol')).pid,first.pid);
 const narrowed=structuredClone(f.state);narrowed.permissionProfile.file_system.entries[0].path.path+='/child';const second=payload(await f.call('find_symbol',narrowed));assert.notEqual(second.pid,first.pid);assert.deepEqual(second.state,narrowed);await dead(first.pid);await dead(first.descendant);
 const readonly=structuredClone(f.state);readonly.permissionProfile.file_system.entries[0].access='read';const third=payload(await f.call('find_symbol',readonly));assert.notEqual(third.pid,second.pid);assert.deepEqual(third.state,readonly);await dead(second.pid);await dead(second.descendant);
}finally{await f.close();}});
for(const mode of ['EOF','SIGKILL'])test(`owned descendants stop on ${mode}`,async()=>{const f=await fixture();try{const running=payload(await f.call('find_symbol'));if(mode==='SIGKILL')f.child.kill('SIGKILL');else f.child.stdio[3].end();await dead(running.pid);await dead(running.descendant);}finally{await f.close();}});
test('unexpected leader exit still escalates against a SIGTERM-resistant descendant',async()=>{const f=await fixture();try{
 const running=payload(await f.request('tools/call',{name:'find_symbol',arguments:{exitLeader:true},_meta:{'codex/sandbox-state-meta':f.state}}));
 await dead(running.pid);await dead(running.descendant);
}finally{await f.close();}});
test('supervisor spawn error settles pending requests and cleanup',async()=>{const f=await fixture({spawnError:true});try{
 assert.match((await f.call('find_symbol')).error.message,/sandbox exited/);
}finally{await f.close();}});
for(const rootKind of ['relative','symlink'])test(`${rootKind} workspace roots bind to the canonical project`,async()=>{const f=await fixture({rootKind});try{
 const running=payload(await f.call('find_symbol'));
 assert.deepEqual(running.args.slice(running.args.indexOf('--project'),running.args.indexOf('--project')+2),['--project',f.workspace]);
}finally{await f.close();}});
test('missing, disabled, external, mismatched and unrestricted authority fail closed',async()=>{const f=await fixture();try{
 for(const profile of [{type:'disabled'},{type:'external',network:'enabled'},{type:'managed',file_system:{type:'unrestricted'},network:'enabled'}])assert.match((await f.call('find_symbol',{...f.state,permissionProfile:profile})).error.message,/metadata required/);
 assert.match((await f.call('find_symbol',{...f.state,sandboxCwd:'file:///sibling'})).error.message,/differs/);await assert.rejects(readFile(join(f.root,'starts')),{code:'ENOENT'});
}finally{await f.close();}});
