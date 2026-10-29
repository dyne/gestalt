import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, readdir, rm, stat, symlink } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import test from 'node:test';
const manager = resolve('public/gestalt');
const sri = 'sha512-' + 'A'.repeat(86) + '==';
const names = ['browser_navigate','browser_fill_form','browser_take_screenshot','browser_snapshot','browser_run_code_unsafe','browser_evaluate','browser_install'];
const server = `if(process.argv.includes('--help')) {console.log('--headless --user-data-dir --executable-path --no-webmcp');process.exit();}
let b='';process.stdin.on('data',chunk=>{b+=chunk;while(b.includes('\\n')){const i=b.indexOf('\\n'),v=JSON.parse(b.slice(0,i));b=b.slice(i+1);if(v.id)console.log(JSON.stringify({jsonrpc:'2.0',id:v.id,result:v.method==='tools/list'?{tools:${JSON.stringify(names)}.map(name=>({name,inputSchema:{type:'object'}}))}:{protocolVersion:'2024-11-05',capabilities:{},serverInfo:{name:'fixture',version:'1'}}}));}});`;
async function fixture(t) {
  const base = await mkdtemp(join(tmpdir(),'playwright lifecycle '));
  t.after(()=>rm(base,{recursive:true,force:true}));
  const home=join(base,'managed'),bin=join(base,'bin'),log=join(base,'npm.log');await mkdir(bin);
  // Explicit install can now provision system Chrome; fixtures always supply one.
  await writeFile(join(bin,'google-chrome'),'#!/bin/sh\necho "Google Chrome 140.0.1.2"\n',{mode:0o755});
  // Only package acquisition is simulated. Manager staging, metadata validation,
  // integrity matching, catalog handshake and atomic activation stay real.
  await writeFile(join(bin,'npm'),`#!${process.execPath}
const fs=require('node:fs'),path=require('node:path');
const base=${JSON.stringify(base)},log=${JSON.stringify(log)},sri=${JSON.stringify(sri)};
const a=process.argv.slice(2),root=process.cwd();
fs.appendFileSync(log,JSON.stringify({args:a,env:process.env,cwd:root})+'\\n');
if(fs.existsSync(path.join(base,'fail')))process.exit(22);
if(fs.existsSync(path.join(base,'delay')))Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,1500);
if(a[0]==='view'){console.log(JSON.stringify({version:'0.0.83',dist:{integrity:sri}}));process.exit();}
const d=JSON.parse(fs.readFileSync('package.json')),version=d.dependencies['@playwright/mcp'];
if(a.includes('--package-lock-only')){fs.writeFileSync('package-lock.json',JSON.stringify({packages:{'node_modules/@playwright/mcp':{version,integrity:fs.existsSync(path.join(base,'bad-integrity'))?'bad':sri,resolved:'https://registry.npmjs.org/@playwright/mcp/-/mcp-'+version+'.tgz'}}}));process.exit();}
const target=path.join(root,'node_modules/@playwright/mcp');fs.mkdirSync(target,{recursive:true});
fs.writeFileSync(path.join(target,'package.json'),JSON.stringify({version}));
fs.writeFileSync(path.join(target,'cli.js'),fs.existsSync(path.join(base,'bad-catalog'))?'process.exit(1)':${JSON.stringify(server)});
`,{mode:0o755});
  const env={...process.env,PATH:bin+':'+process.env.PATH,GESTALT_HOME:home};
  const run=(args,extra={})=>spawnSync('bash',[manager,'playwright',...args],{env:{...env,...extra},encoding:'utf8',timeout:20000});
  const active=()=>readFile(join(home,'playwright/active.json'),'utf8').then(JSON.parse);
  return {base,home,env,run,active,log};
}
test('help, missing version and doctor leave managed state absent',async t=>{
  const f=await fixture(t);
  assert.equal(f.run(['help']).status,0);
  assert.equal(f.run(['version']).status,1);
  const d=f.run(['doctor','--json']);assert.equal(d.status,1);
  assert.equal(JSON.parse(d.stdout).package.ready,false);
  assert.deepEqual((await readdir(f.base)).sort(),['bin']);
});
test('fresh install pins integrity and standard catalog, reinstall is offline and immutable',async t=>{
  const f=await fixture(t);let r=f.run(['install']);assert.equal(r.status,0,r.stderr);
  const d=await f.active();assert.equal(d.version,'0.0.83');assert.equal(d.integrity,sri);assert.equal(d.contractVersion,1);
  assert.ok(d.tools.includes('browser_navigate'));assert.ok(!d.tools.includes('browser_run_code_unsafe'));assert.ok(!d.tools.includes('browser_install'));
  const before=await readFile(f.log,'utf8');await writeFile(join(f.base,'fail'),'');
  r=f.run(['install']);assert.equal(r.status,0,r.stderr);assert.equal(await readFile(f.log,'utf8'),before);
  assert.deepEqual(await f.active(),d);
  assert.equal((await stat(join(f.home,'playwright'))).mode&0o777,0o700);
  assert.equal((await stat(join(f.home,'playwright/active.json'))).mode&0o777,0o600);
});
test('explicit version is validated, updates retain old releases and never share npm config/hooks',async t=>{
  const f=await fixture(t);assert.equal(f.run(['install','0.0.83']).status,0);const before=await f.active();
  const cookies=join(f.home,'playwright/workspaces/test/chrome/cookies');await mkdir(join(cookies,'..'),{recursive:true});await writeFile(cookies,'retain');
  assert.equal(f.run(['update','0.0.83']).status,0);const after=await f.active();assert.notEqual(before.release,after.release);await stat(before.release);assert.equal(await readFile(cookies,'utf8'),'retain');
  const entries=(await readFile(f.log,'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(entries.length,6);
  for(const e of entries){assert.equal(e.env.PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD,'1');assert.equal(e.env.npm_config_ignore_scripts,'true');assert.equal(e.env.npm_config_engine_strict,'true');assert.equal(e.env.npm_config_registry,'https://registry.npmjs.org/');assert.equal(e.env.PLAYWRIGHT_MCP_PORT,undefined);assert.equal(e.env.NODE_OPTIONS,undefined);}
  for(const version of ['latest','../1.0.0','1.0.0-beta','--help'])assert.equal(f.run(['update',version]).status,1);
});
for(const failure of ['fail','bad-integrity','bad-catalog'])test('failed staging preserves active: '+failure,async t=>{
  const f=await fixture(t);assert.equal(f.run(['install']).status,0);const before=await f.active();
  await writeFile(join(f.base,failure),'');assert.equal(f.run(['update']).status,1);assert.deepEqual(await f.active(),before);
  assert.ok(!(await readdir(join(f.home,'playwright'))).includes('.install-lock'));
  assert.equal((await readdir(join(f.home,'playwright'))).filter(n=>n.startsWith('release-')).length,1);
});
test('tampered metadata, executable, lock and symlink paths fail closed without network',async t=>{
  const f=await fixture(t);assert.equal(f.run(['install']).status,0);const d=await f.active();
  const lock=join(d.release,'package-lock.json'),original=await readFile(lock);
  await writeFile(lock,'{}');assert.equal(f.run(['version']).status,1);await writeFile(lock,original);
  await writeFile(join(d.release,'node_modules/@playwright/mcp/cli.js'),'tamper');assert.equal(f.run(['version']).status,1);
  await rm(join(f.home,'playwright/active.json'));await symlink(join(f.base,'npm.log'),join(f.home,'playwright/active.json'));
  assert.equal(f.run(['version']).status,1);
});
test('Chrome readiness is independent and read-only, supports spaces and rejects missing/incompatible executables',async t=>{
  const f=await fixture(t);assert.equal(f.run(['install']).status,0);const chrome=join(f.base,'Chrome With Spaces');
  await writeFile(chrome,'#!/bin/sh\nprintf "Google Chrome 140.0.1.2\\n"\n',{mode:0o755});
  const before=await readFile(f.log,'utf8'),meta=await readFile(join(f.home,'playwright/active.json'),'utf8');
  const r=f.run(['doctor','--json','--executable-path',chrome]);assert.equal(r.status,0,r.stderr);assert.equal(JSON.parse(r.stdout).chrome.ready,true);
  assert.equal(await readFile(f.log,'utf8'),before);assert.equal(await readFile(join(f.home,'playwright/active.json'),'utf8'),meta);
  for(const p of ['/missing/chrome','relative/chrome'])assert.equal(f.run(['doctor','--executable-path',p]).status,1);
  await writeFile(chrome,'#!/bin/sh\necho "Google Chrome 99.0.1.2"\n');
  assert.equal(JSON.parse(f.run(['doctor','--json','--executable-path',chrome]).stdout).chrome.reason,'incompatible-chrome');
});
test('a concurrent updater cannot stage or activate another release',async t=>{
  const f=await fixture(t);await writeFile(join(f.base,'delay'),'');
  const p=spawn('bash',[manager,'playwright','install'],{env:f.env,stdio:'ignore'});
  await new Promise((resolve,reject)=>{let n=0;const timer=setInterval(async()=>{if(++n>100){clearInterval(timer);reject(Error('lock not acquired'));}try{await stat(join(f.home,'playwright/.install-lock'));clearInterval(timer);resolve();}catch{}},20);});
  assert.match(f.run(['update']).stderr,/already running/);
  assert.equal(await new Promise(resolve=>p.on('exit',resolve)),0);
});
test('interrupted staging terminates package subprocess and preserves the active release',async t=>{
  const f=await fixture(t);assert.equal(f.run(['install']).status,0);const before=await f.active();
  await writeFile(join(f.base,'delay'),'');
  const p=spawn('bash',[manager,'playwright','update'],{env:f.env,stdio:'ignore'});
  const exited=new Promise(resolve=>p.on('exit',(code,signal)=>resolve({code,signal})));
  for(let i=0;i<100;i++){try{await stat(join(f.home,'playwright/.install-lock'));break;}catch{await new Promise(r=>setTimeout(r,20));}}
  p.kill('SIGTERM');await exited;
  assert.deepEqual(await f.active(),before);
  assert.ok(!(await readdir(join(f.home,'playwright'))).includes('.install-lock'));
  assert.equal((await readdir(join(f.home,'playwright'))).filter(n=>n.startsWith('release-')).length,1);
});
test('failed atomic activation never advertises a staged candidate',async t=>{
  const f=await fixture(t);
  await mkdir(join(f.home,'playwright/active.json'),{recursive:true});
  const r=f.run(['install']);assert.equal(r.status,1);
  assert.ok((await stat(join(f.home,'playwright/active.json'))).isDirectory());
  assert.deepEqual(await readdir(join(f.home,'playwright')),['active.json']);
});
test('managed directory symlinks cannot redirect package installation',async t=>{
  const f=await fixture(t);const outside=join(f.base,'outside');await mkdir(outside);await mkdir(f.home);
  await symlink(outside,join(f.home,'playwright'));
  assert.equal(f.run(['install']).status,1);assert.deepEqual(await readdir(outside),[]);
  assert.equal(f.run(['doctor','--json']).status,1);
});
