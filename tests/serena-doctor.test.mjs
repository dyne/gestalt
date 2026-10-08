import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, rm, readdir, chmod } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
const manager = resolve('public/gestalt');
const tools = ['get_symbols_overview','find_symbol','replace_symbol_body'].map(name=>({name,inputSchema:{type:'object'}}));
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(),'serena-doctor-')); t.after(async()=>{await chmod(join(root,'home'),0o755);await chmod(join(root,'workspace with spaces'),0o755);await rm(root,{recursive:true,force:true});});
  const home=join(root,'home'), managed=join(root,'managed'), workspace=join(root,'workspace with spaces'), stubs=join(root,'stubs');
  for(const p of [join(root,'bin'),home,workspace,join(managed,'serena'),join(root,'python'),join(stubs,'ruamel'),join(stubs,'serena/config'),join(stubs,'mcp/client')])await mkdir(p,{recursive:true});
  await writeFile(join(workspace,'example.py'),'def greet():\n    return 1\n');
  await writeFile(join(root,'bin/codex'),`#!/bin/bash
if [[ $SANDBOX_MODE == blocked ]]; then
 echo 'bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted' >&2
 exit 1
fi
if [[ $SANDBOX_MODE == timeout ]]; then exec /bin/sleep 60; fi
exit 0
`,{mode:0o755});
  await writeFile(join(root,'bin/bwrap'),'#!/bin/sh\necho "bubblewrap fixture"\n',{mode:0o755});
  const python=join(root,'python-wrapper');
  await writeFile(python,`#!/usr/bin/env python3
import sys,os,json
if any('importlib.metadata' in arg for arg in sys.argv):
    if os.environ.get('BAD_METADATA'):sys.exit(1)
    if any('json.dumps' in arg for arg in sys.argv):print(json.dumps(dict(serena='1.7.0',python='3.13.9')))
    else:print('1.7.0')
else:os.execv('/usr/bin/python3',['/usr/bin/python3',*sys.argv[1:]])
`,{mode:0o755});
  const uv=join(root,'uv');await writeFile(uv,'#!/bin/sh\necho "uv 0.12.23"\n',{mode:0o755});
  await writeFile(join(stubs,'ruamel/yaml.py'),'import json\nclass YAML:\n def load(self,s):return json.load(s)\n def dump(self,v,s):json.dump(v,s)\n');
  await writeFile(join(stubs,'serena/config/serena_config.py'),`import pathlib,os,json
class SerenaConfig:
 @classmethod
 def from_config_file(cls):
  p=pathlib.Path(os.environ['SERENA_HOME'])/'serena_config.yml'
  if not p.exists():p.write_text(json.dumps(dict(ignored_paths=[],ls_specific_settings={})))
  return cls()
class ProjectConfigAutoGenerationMode:SYNCHRONOUS='sync'
class ProjectConfig:
 @classmethod
 def load(cls,root,config,autogen):
  p=pathlib.Path(os.environ['SERENA_HOME'])/'project.yml'
  if not p.exists():p.write_text(json.dumps(dict(project_name=root.name,language_servers=['python'])))
  return cls()
`);
  await writeFile(join(stubs,'mcp/__init__.py'),`import json
from types import SimpleNamespace
class StdioServerParameters:
 def __init__(self,**kwargs):self.__dict__.update(kwargs)
class Tool:
 def __init__(self,value):self.value=value;self.name=value['name']
 def model_dump(self,**kwargs):return self.value
class ClientSession:
 def __init__(self,reader,writer):self.reader=reader;self.writer=writer
 async def __aenter__(self):return self
 async def __aexit__(self,*args):pass
 async def request(self,method,params=None):
  self.writer.write((json.dumps(dict(method=method,params=params,id=1))+'\\n').encode());await self.writer.drain()
  result=json.loads(await self.reader.readline())
  if 'error' in result:raise RuntimeError(result['error'])
  return result['result']
 async def initialize(self):return await self.request('initialize')
 async def list_tools(self):return SimpleNamespace(tools=[Tool(t) for t in (await self.request('tools/list'))['tools']])
 async def call_tool(self,name,arguments):
  result=await self.request('tools/call',dict(name=name,arguments=arguments))
  return SimpleNamespace(isError=result.get('isError',False),content=[SimpleNamespace(**c) for c in result['content']])
`);
  await writeFile(join(stubs,'mcp/client/stdio.py'),`import asyncio
from contextlib import asynccontextmanager
@asynccontextmanager
async def stdio_client(parameters):
 process=await asyncio.create_subprocess_exec(parameters.command,*parameters.args,env=parameters.env,
   stdin=asyncio.subprocess.PIPE,stdout=asyncio.subprocess.PIPE)
 try:yield process.stdout,process.stdin
 finally:
  if process.returncode is None:process.terminate()
  await process.wait()
`);
  const executable=join(root,'native');
  await writeFile(executable,`#!/usr/bin/env python3
import os,sys,json,pathlib,time,subprocess
pathlib.Path(os.environ['ARGV_LOG']).write_text(json.dumps(dict(args=sys.argv[1:],env=dict(os.environ))))
if sys.argv[1:3]==['project','index']:
 pathlib.Path(os.environ['SERENA_HOME'],'native-symbol-cache').write_text('warmed')
 print('native index warmed');sys.exit()
if os.environ.get('DOCTOR_MODE')=='timeout':
 child=subprocess.Popen(['/bin/sleep','60']);pathlib.Path(os.environ['PID_LOG']).write_text(str(child.pid));time.sleep(60)
for line in sys.stdin:
 message=json.loads(line)
 if message['method']=='initialize':
  if os.environ.get('DOCTOR_MODE')=='handshake':print(json.dumps(dict(error='handshake failed')),flush=True);continue
  result={}
 elif message['method']=='tools/list':result=dict(tools=${JSON.stringify(tools)})
 else:
  if os.environ.get('DOCTOR_MODE')=='lsp-timeout':time.sleep(60)
  error=os.environ.get('DOCTOR_MODE')=='lsp-error'
  text='Error: language service failed' if error else json.dumps([dict(name='greet',kind=12)])
  result=dict(content=[dict(type='text',text=text)],isError=error)
 print(json.dumps(dict(result=result)),flush=True)
`,{mode:0o755});
  const descriptor=join(managed,'serena/active.json');
  await writeFile(descriptor,JSON.stringify({schemaVersion:1,contractVersion:1,version:'1.7.0',python,executable,uv,pythonInstallDir:join(root,'python'),tools}));
  const env={...process.env,PATH:join(root,'bin')+':'+process.env.PATH,HOME:home,GESTALT_HOME:managed,CODEX_HOME:join(root,'codex'),PYTHONPATH:stubs,
    ARGV_LOG:join(root,'args.json'),PID_LOG:join(root,'pid')};
  async function run(args,extra={}) {
    const child=spawn('bash',[manager,...args],{env:{...env,...extra},stdio:['ignore','pipe','pipe']});
    let stdout='',stderr='';child.stdout.on('data',b=>stdout+=b);child.stderr.on('data',b=>stderr+=b);
    const code=await new Promise((r,j)=>{child.on('error',j);child.on('exit',r);});
    return {code,stdout,stderr};
  }
  const doctor=async(extra={},cwd=workspace)=>{
    const r=await run(['serena','doctor','--json',...(cwd?['--cwd',cwd]:[])],extra);
    const {sandbox,...report}=JSON.parse(r.stdout);
    return {...r,report,sandbox};
  };
  return {root,home,managed,workspace,descriptor,executable,python,uv,env,run,doctor};
}
test('versions/-V use package metadata, leave readonly home and workspace unchanged',async t=>{
  const f=await fixture(t);const before=await readdir(f.workspace);await chmod(f.home,0o555);await chmod(f.workspace,0o555);
  for(const args of [['serena','version'],['serena','-V'],['version']]) {
    const r=await f.run(args);assert.equal(r.code,0,r.stderr);assert.match(r.stdout,/1.7.0/);
    if(args[0]==='version')assert.match(r.stdout,/uv \(Serena\).*0.12.23/);
  }
  assert.deepEqual(await readdir(f.home),[]);assert.deepEqual(await readdir(f.workspace),before);
});
test('doctor distinguishes installed, connected and valid project symbols without source edits',async t=>{
  const f=await fixture(t);const r=await f.doctor();assert.equal(r.code,0,r.stderr);
  assert.deepEqual(r.report,{installed:true,connected:true,projectReady:true,reason:'ready'});
  assert.equal(await readFile(join(f.workspace,'example.py'),'utf8'),'def greet():\n    return 1\n');
  const launch=JSON.parse(await readFile(join(f.root,'args.json'),'utf8'));
  assert.deepEqual(launch.args.slice(0,5),['start-mcp-server','--context','codex','--mode','editing']);
  assert.equal(launch.env.SERENA_HOME,join(f.workspace,'.gestalt/serena'));
  assert.deepEqual(await readdir(f.home),[]);
});
test('healthy MCP with failed language service is not project ready',async t=>{
  const f=await fixture(t);const r=await f.doctor({DOCTOR_MODE:'lsp-error'});
  assert.equal(r.code,1);assert.deepEqual(r.report,{installed:true,connected:true,projectReady:false,reason:'language-service-failed'});
});
test('doctor without project reports connection only, empty project is not ready',async t=>{
  const f=await fixture(t);let r=await f.doctor({},'');assert.equal(r.code,0,r.stderr);
  assert.deepEqual(r.report,{installed:true,connected:true,projectReady:null,reason:'connected'});
  const empty=join(f.root,'empty');await mkdir(empty);r=await f.doctor({},empty);
  assert.equal(r.code,1);assert.equal(r.report.connected,true);assert.equal(r.report.projectReady,false);assert.match(r.report.reason,/no-supported-source/);
  assert.deepEqual(await readdir(empty),[]);
});
test('absent/malformed/incompatible metadata and missing runtime fail before startup',async t=>{
  const f=await fixture(t);
  let r=await f.doctor({BAD_METADATA:'1'});assert.equal(r.code,1);assert.equal(r.report.installed,false);
  await rm(f.python);r=await f.doctor();assert.equal(r.code,1);assert.equal(r.report.installed,false);
  await writeFile(f.descriptor,'{}');r=await f.doctor();assert.equal(r.report.reason,'invalid-installation');
  await writeFile(f.descriptor,'bad json');r=await f.doctor();assert.equal(r.report.reason,'invalid-installation');
  await rm(f.descriptor);r=await f.doctor();assert.equal(r.report.reason,'not-installed');
  await assert.rejects(readFile(join(f.root,'args.json')),{code:'ENOENT'});
});
test('handshake failure remains disconnected; timeout reaps native descendants',async t=>{
  const f=await fixture(t);let r=await f.doctor({DOCTOR_MODE:'handshake'});assert.equal(r.code,1);assert.equal(r.report.connected,false);
  r=await f.doctor({DOCTOR_MODE:'timeout',GESTALT_SERENA_DOCTOR_TIMEOUT_MS:'500'});
  assert.equal(r.code,1);assert.equal(r.report.connected,false);assert.match(r.report.reason,/timeout/);
  const pid=Number(await readFile(join(f.root,'pid'),'utf8'));
  // Linux may retain a dead orphan as a zombie until its init reaps it.
  try {const status=await readFile(`/proc/${pid}/stat`,'utf8');assert.match(status,/^\d+ \(.+\) Z /);} catch(e){if(e.code!=='ENOENT')throw e;}
});
test('bounded language timeout retains healthy connection status',async t=>{
  const f=await fixture(t);const r=await f.doctor({DOCTOR_MODE:'lsp-timeout',GESTALT_SERENA_DOCTOR_TIMEOUT_MS:'800'});
  assert.equal(r.code,1);assert.equal(r.report.installed,true);assert.equal(r.report.connected,true);
  assert.equal(r.report.projectReady,false);assert.equal(r.report.reason,'language-service-timeout');
});
test('native index is explicit and writes project-local cache while retaining source',async t=>{
  const f=await fixture(t);assert.equal((await f.run(['serena','index'])).code,1);
  const r=await f.run(['serena','index','--cwd',f.workspace]);assert.equal(r.code,0,r.stderr);assert.match(r.stdout,/native index/);
  assert.equal(await readFile(join(f.workspace,'.gestalt/serena/native-symbol-cache'),'utf8'),'warmed');
  assert.equal(await readFile(join(f.workspace,'example.py'),'utf8'),'def greet():\n    return 1\n');
  const launch=JSON.parse(await readFile(join(f.root,'args.json'),'utf8'));
  assert.deepEqual(launch.args,['project','index',f.workspace,'--timeout','10']);
  assert.deepEqual(await readdir(f.home),[]);
});


test('source probe enforces entry bound inside a single large directory',async t=>{
  const f=await fixture(t), root=join(f.root,'large-project');await mkdir(root);
  for(let i=0;i<1000;i++)await writeFile(join(root,'a'+String(i).padStart(4,'0')+'.txt'),'unsupported');
  await writeFile(join(root,'z.py'),'def outside_probe_limit(): pass');
  const r=await f.doctor({},root);assert.equal(r.code,1);assert.equal(r.report.connected,true);
  assert.equal(r.report.projectReady,false);assert.match(r.report.reason,/no-supported-source-within-probe-limit/);
  assert.deepEqual(await readdir(root),(await readdir(root)).filter(name=>name!=='.gestalt'));
});

test('doctor checks Bubblewrap and a bounded Codex sandbox before MCP startup',async t=>{
  const f=await fixture(t);
  const r=await f.doctor();
  assert.equal(r.sandbox.ready,true);
  assert.equal(r.sandbox.reason,'ready');
  if(process.platform==='linux') {
    assert.equal(r.sandbox.bubblewrap.source,'system');
    assert.equal(r.sandbox.bubblewrap.version,'bubblewrap fixture');
    assert.ok(Object.hasOwn(r.sandbox.userNamespaces,'kernel.apparmor_restrict_unprivileged_userns'));
  }
});
test('sandbox failure exposes Bubblewrap diagnostic and prevents misleading connection readiness',async t=>{
  const f=await fixture(t), r=await f.doctor({SANDBOX_MODE:'blocked'});
  assert.equal(r.code,1);
  assert.equal(r.report.installed,true);
  assert.equal(r.report.connected,false);
  assert.equal(r.report.reason,'sandbox-unavailable');
  assert.equal(r.sandbox.ready,false);
  assert.equal(r.sandbox.reason,'sandbox-startup-failed');
  assert.match(r.sandbox.diagnostic,/Failed RTM_NEWADDR/);
  await assert.rejects(readFile(join(f.root,'args.json')),{code:'ENOENT'});
  assert.deepEqual(await readdir(f.home),[]);
  assert.deepEqual(await readdir(f.workspace),['example.py']);
  const text=await f.run(['serena','doctor'],{SANDBOX_MODE:'blocked'});
  assert.match(text.stdout,/Serena sandbox\s+sandbox-startup-failed/);
  assert.match(text.stdout,/Operation not permitted/);
  const all=await f.run(['doctor'],{SANDBOX_MODE:'blocked'});
  assert.equal(all.code,1);
  assert.match(all.stdout,/Bubblewrap and Codex sandbox/);
  assert.match(all.stdout,/Codex sandbox\s+sandbox-startup-failed/);
  assert.match(all.stdout,/Operation not permitted/);
});
test('sandbox probe timeout is bounded and keeps the backend stopped',async t=>{
  const f=await fixture(t),r=await f.doctor({SANDBOX_MODE:'timeout'});
  assert.equal(r.code,1);
  assert.equal(r.sandbox.reason,'sandbox-timeout');
  await assert.rejects(readFile(join(f.root,'args.json')),{code:'ENOENT'});
});

const source = await readFile(manager, 'utf8');
const sandboxProgram = source.split("codex_sandbox_probe() {\n  node --input-type=module - <<'JS'\n")[1].split('\nJS\n}')[0];
async function probe(f) {
  const child=spawn(process.execPath,['--input-type=module','-'],{env:{...f.env,PATH:join(f.root,'bin')},stdio:['pipe','pipe','pipe']});
  child.stdin.end(sandboxProgram);
  let stdout='',stderr='';child.stdout.on('data',b=>stdout+=b);child.stderr.on('data',b=>stderr+=b);
  const code=await new Promise((r,j)=>{child.on('error',j);child.on('exit',r);});
  assert.equal(code,0,stderr);
  return JSON.parse(stdout);
}
test('sandbox doctor detects Codex-bundled Bubblewrap without requiring a system binary',async t=>{
  if(process.platform!=='linux'||!['x64','arm64'].includes(process.arch)){t.skip();return;}
  const f=await fixture(t);
  await rm(join(f.root,'bin/bwrap'));
  const triple=(process.arch==='x64'?'x86_64':'aarch64')+'-unknown-linux-musl';
  const resources=join(f.root,'vendor',triple,'codex-resources');
  await mkdir(resources,{recursive:true});
  await writeFile(join(resources,'bwrap'),'#!/bin/sh\necho "bubblewrap bundled fixture"\n',{mode:0o755});
  const r=await probe(f);
  assert.equal(r.ready,true);
  assert.equal(r.bubblewrap.available,true);
  assert.equal(r.bubblewrap.source,'bundled');
  assert.equal(r.bubblewrap.path,join(resources,'bwrap'));
});
test('sandbox doctor reports missing Codex independently of Bubblewrap availability',async t=>{
  const f=await fixture(t);await rm(join(f.root,'bin/codex'));
  const r=await probe(f);
  assert.equal(r.ready,false);
  assert.equal(r.reason,'codex-not-found');
  if(process.platform==='linux')assert.equal(r.bubblewrap.available,true);
});
