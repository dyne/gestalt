import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {createServer} from 'node:http';
import {mkdtemp,mkdir,readFile,writeFile,readdir,readlink,rm,stat} from 'node:fs/promises';
import {join,resolve,dirname} from 'node:path';
import {tmpdir} from 'node:os';
import {createHash} from 'node:crypto';
import test from 'node:test';
const manager=resolve('public/gestalt');
const pause=ms=>new Promise(r=>setTimeout(r,ms));
async function chromeFiles(root){
 const rows=[];
 async function visit(p){for(const entry of await readdir(p,{withFileTypes:true})){const file=join(p,entry.name);if(entry.isDirectory())await visit(file);else{const s=await stat(file);rows.push([file,s.size,s.mtimeMs]);}}}
 await visit(root);return JSON.stringify(rows.sort());
}
// Inspect only this launcher's descendants. The fixture's localhost listener
// belongs to the test, so it is deliberately outside the observed process tree.
async function observe(pid){
 assert.equal(process.platform,'linux','process/listener integration observation currently requires Linux');
 const rows=[];
 for(const name of await readdir('/proc'))if(/^\d+$/.test(name))try{
  const fields=(await readFile('/proc/'+name+'/stat','utf8')).split(') ').at(-1).split(' ');
  rows.push({pid:Number(name),parent:Number(fields[1]),command:(await readFile('/proc/'+name+'/cmdline','utf8')).split('\0').filter(Boolean)});
 }catch{}
 const owned=new Set([pid]);let changed=true;while(changed){changed=false;for(const row of rows)if(owned.has(row.parent)&&!owned.has(row.pid)){owned.add(row.pid);changed=true;}}
 const tree=rows.filter(row=>owned.has(row.pid));assert.ok(tree.every(row=>!row.command.some(a=>a==='codex'||a.endsWith('/codex'))));
 const browser=tree.find(row=>row.command.includes('--remote-debugging-pipe'));
 assert.ok(browser,'Chrome must use a browser pipe');assert.ok(browser.command.some(a=>a.startsWith('--headless')));assert.ok(!browser.command.includes('--no-sandbox'));
 const sockets=new Set();
 for(const row of tree)try{for(const fd of await readdir('/proc/'+row.pid+'/fd'))try{const link=await readlink('/proc/'+row.pid+'/fd/'+fd);const m=link.match(/^socket:\[(\d+)\]$/);if(m)sockets.add(m[1]);}catch{}}catch{}
 let listeners=0;for(const name of ['tcp','tcp6'])for(const line of (await readFile('/proc/net/'+name,'utf8')).trim().split('\n').slice(1)){const fields=line.trim().split(/\s+/);if(fields[3]==='0A'&&sockets.has(fields[9]))listeners++;}
 assert.equal(listeners,0,'MCP/Chrome must not own TCP listeners');
 return tree.map(row=>row.pid);
}
async function settled(pids){for(let i=0;i<120;i++){const live=[];for(const pid of pids)try{const fields=(await readFile('/proc/'+pid+'/stat','utf8')).split(') ').at(-1).split(' ');if(fields[0]!=='Z')live.push(pid);}catch{}if(!live.length)return;await pause(25);}assert.fail('owned browser descendants survived shutdown');}
// Explicit integration gate; no browser or package acquisition is performed.
// Run with the externally prepared install/Chrome prerequisites documented below.
test('managed real Chrome: stdio catalog, form, screenshot, persistence and isolation', {timeout:120000},async t=>{
 const managed=process.env.GESTALT_PLAYWRIGHT_REAL_HOME,chrome=process.env.GESTALT_PLAYWRIGHT_REAL_CHROME;
 assert.ok(managed&&chrome,'Set GESTALT_PLAYWRIGHT_REAL_HOME and GESTALT_PLAYWRIGHT_REAL_CHROME to existing test prerequisites');
 const root=await mkdtemp(join(tmpdir(),'playwright-real-')),workspace=join(root,'project'),bin=join(root,'bin');await mkdir(workspace);await mkdir(bin);
 const blocked=join(root,'blocked-package-commands');
 for(const name of ['codex','npm','npx','curl','wget'])await writeFile(join(bin,name),'#!/bin/sh\nprintf "%s\\n" '+name+' >> "'+blocked+'"\nexit 99\n',{mode:0o755});

 const executableBefore=createHash('sha256').update(await readFile(chrome)).digest('hex');
 const filesBefore=await chromeFiles(dirname(chrome));
 const fixture=createServer((request,response)=>{
  if(new URL(request.url,'http://localhost').pathname==='/saved'){response.setHeader('Set-Cookie','managed-proof=retained; Path=/; Max-Age=3600');response.end('<h1>Saved</h1>');return;}
  response.end('<!doctype html><title>Browser fixture</title><h1>'+((request.headers.cookie||'').includes('managed-proof=retained')?'Returning user':'New user')+'</h1><form action="/saved"><label>Name <input name="name"></label><button>Save</button></form>');
 });
 await new Promise(r=>fixture.listen(0,'127.0.0.1',r));
 const url='http://127.0.0.1:'+fixture.address().port;
 const sessions=[];
 function start(project=workspace){
  const canonical=resolve(project),state=join(managed,'playwright/workspaces',createHash('sha256').update(canonical).digest('hex'));
  const child=spawn('bash',[manager,'playwright','mcp','--cwd',project,'--executable-path',chrome],{env:{...process.env,PATH:bin+':'+process.env.PATH,GESTALT_HOME:managed},stdio:['pipe','pipe','pipe']});
  let id=0,stderr='';const pending=new Map(),lines=createInterface({input:child.stdout});child.stderr.on('data',b=>stderr=(stderr+b).slice(-4096));
  lines.on('line',line=>{const message=JSON.parse(line),waiter=pending.get(message.id);if(waiter){pending.delete(message.id);waiter(message);}});
  const exited=new Promise(r=>child.once('exit',r));
  const request=(method,params={})=>new Promise((resolve,reject)=>{const n=++id,timer=setTimeout(()=>reject(Error('real MCP timeout: '+stderr)),45000);pending.set(n,value=>{clearTimeout(timer);resolve(value);});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\n');});
  const call=async(name,args={})=>{const message=await request('tools/call',{name,arguments:args});assert.ok(!message.error,JSON.stringify(message.error));assert.ok(!message.result.isError,JSON.stringify(message.result));return message.result;};
  const close=async()=>{if(child.exitCode===null&&child.signalCode===null)child.stdin.end();await exited;lines.close();};
  const session={request,call,close,state,child};sessions.push(session);return session;
 }
 t.after(async()=>{for(const s of sessions)await s.close();await new Promise(r=>fixture.close(r));await rm(root,{recursive:true,force:true});});
 let s=start();await s.request('initialize',{protocolVersion:'2024-11-05'});
 const listed=await s.request('tools/list');assert.ok(listed.result.tools.some(t=>t.name==='browser_navigate'));
 assert.ok(!listed.result.tools.some(t=>/run_code|evaluate|install|webmcp/.test(t.name)));
 const forbidden=await s.request('tools/call',{name:'browser_run_code_unsafe',arguments:{code:'()=>1'}});assert.match(forbidden.error.message,/disabled/);
 await s.call('browser_navigate',{url});const page=await s.call('browser_snapshot');const snapshot=page.content.filter(item=>item.type==='text').map(item=>item.text).join('\n');assert.match(snapshot,/New user/);
 const input=snapshot.match(/textbox "Name" \[ref=(e\d+)\]/);assert.ok(input,'fixture textbox reference missing');
 await s.call('browser_fill_form',{fields:[{name:'Name',element:'Name',type:'textbox',target:input[1],value:'Managed browser'}]});
 const button=snapshot.match(/button "Save" \[ref=(e\d+)\]/);assert.ok(button);
 await s.call('browser_click',{element:'Save',target:button[1]});
 const shot=await s.call('browser_take_screenshot',{type:'png',filename:join(workspace,'proof.png')});assert.ok(shot.content.length);assert.ok((await stat(join(workspace,'proof.png'))).size>0);
 const firstTree=await observe(s.child.pid);
 const contender=start();await contender.request('initialize');const busy=await contender.request('tools/call',{name:'browser_snapshot',arguments:{}});assert.match(busy.error.message,/busy/);await contender.close();
 assert.match(JSON.stringify(await s.call('browser_snapshot')),/Saved/);
 await s.close();await settled(firstTree);s=start();await s.request('initialize',{protocolVersion:'2024-11-05'});await s.call('browser_navigate',{url});assert.match(JSON.stringify(await s.call('browser_snapshot')),/Returning user/);
 const other=join(root,'other');await mkdir(other);const independent=start(other);await independent.request('initialize',{protocolVersion:'2024-11-05'});await independent.call('browser_navigate',{url});assert.match(JSON.stringify(await independent.call('browser_snapshot')),/New user/);
 const secondTree=await observe(s.child.pid),otherTree=await observe(independent.child.pid);
 s.child.kill('SIGTERM');await s.close();await settled(secondTree);
 assert.match(JSON.stringify(await independent.call('browser_snapshot')),/New user/);
 s=start();await s.request('initialize');await s.call('browser_navigate',{url});assert.match(JSON.stringify(await s.call('browser_snapshot')),/Returning user/);
 const crashTree=await observe(s.child.pid);s.child.kill('SIGKILL');await s.close();await settled(crashTree);
 s=start();await s.request('initialize');await s.call('browser_navigate',{url});assert.match(JSON.stringify(await s.call('browser_snapshot')),/Returning user/);
 const recoveredTree=await observe(s.child.pid);await s.close();await settled(recoveredTree);
 await independent.close();await settled(otherTree);
 assert.equal(createHash('sha256').update(await readFile(chrome)).digest('hex'),executableBefore);
 assert.equal(await chromeFiles(dirname(chrome)),filesBefore);
 await assert.rejects(stat(blocked),{code:'ENOENT'});
});
