import assert from 'node:assert/strict';
import {readFile,writeFile,mkdtemp,rm} from 'node:fs/promises';
import {runInNewContext} from 'node:vm';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {join,resolve} from 'node:path';
import {tmpdir} from 'node:os';
import test from 'node:test';

const manager=resolve('public/gestalt'),source=await readFile(manager,'utf8');
const policy=source.split('// BEGIN MANAGED MCP APPROVAL POLICY\n')[1].split('// END MANAGED MCP APPROVAL POLICY')[0];
const {managedMcpApprovals,managedMcpOverrides}=runInNewContext(policy+'\n({managedMcpApprovals,managedMcpOverrides})');
const owner={manager,codexHome:'/fixture/codex'};
const plain=value=>JSON.parse(JSON.stringify(value));
const browser={command:manager,args:['playwright','mcp'],enabled:false,default_tools_approval_mode:'prompt',
  enabled_tools:['browser_navigate'],disabled_tools:['browser_evaluate'],tools:{browser_navigate:{approval_mode:'prompt'},browser_evaluate:{enabled:false,approval_mode:'prompt'}}};
const baseline={approval_policy:'never',sandbox_mode:'workspace-write',mcp_servers:{
  'gestalt-playwright':browser,
  'gestalt-serena':{command:manager,args:['serena','mcp'],default_tools_approval_mode:'writes',tools:{replace_symbol_body:{approval_mode:'prompt',output_token_limit:2000}}},
  'gestalt-xerj':{command:manager,args:['xerj','mcp'],enabled_tools:['xerj_search']},
  'context-mode':{command:'node',args:['/fixture/codex/bin/context-mode-mcp.mjs']},
  third:{command:'third-party',default_tools_approval_mode:'prompt',tools:{edit:{approval_mode:'prompt'}}}},
  plugins:{'context-mode@dyne-gestalt-agents':{enabled:false,mcp_servers:{'context-mode':{tools:{ctx_execute:{approval_mode:'prompt',enabled:false}}}}},
    'unrelated@market':{mcp_servers:{other:{default_tools_approval_mode:'prompt'}}}}};

test('only owned servers/tools are preapproved; allowlists, off states and permissions survive',()=>{
  const before=JSON.stringify(baseline),result=plain(managedMcpApprovals(baseline,owner));
  for(const name of ['gestalt-playwright','gestalt-serena','gestalt-xerj','context-mode'])
    assert.equal(result.mcp_servers[name].default_tools_approval_mode,'approve');
  assert.equal(result.mcp_servers['gestalt-playwright'].tools.browser_navigate.approval_mode,'approve');
  assert.equal(result.mcp_servers['gestalt-playwright'].enabled,false);
  assert.equal(result.mcp_servers['gestalt-playwright'].tools.browser_evaluate.enabled,false);
  assert.deepEqual(result.mcp_servers['gestalt-playwright'].enabled_tools,browser.enabled_tools);
  assert.deepEqual(result.mcp_servers['gestalt-playwright'].disabled_tools,browser.disabled_tools);
  assert.equal(result.mcp_servers['gestalt-serena'].tools.replace_symbol_body.output_token_limit,2000);
  assert.deepEqual(result.mcp_servers.third,baseline.mcp_servers.third);
  assert.deepEqual(result.plugins['unrelated@market'],baseline.plugins['unrelated@market']);
  assert.equal(result.plugins['context-mode@dyne-gestalt-agents'].enabled,false);
  assert.equal(result.plugins['context-mode@dyne-gestalt-agents'].mcp_servers['context-mode'].tools.ctx_execute.approval_mode,'approve');
  assert.equal(result.plugins['context-mode@dyne-gestalt-agents'].mcp_servers['context-mode'].tools.ctx_execute.enabled,false);
  assert.equal(result.approval_policy,'never');assert.equal(result.sandbox_mode,'workspace-write');
  assert.equal(JSON.stringify(baseline),before);
  assert.deepEqual(plain(managedMcpApprovals(result,owner)),result);
});

test('reserved names do not preapprove a third-party transport',()=>{
  for(const replacement of [{command:'/third-party',args:['playwright','mcp']},{url:'https://third.example/mcp'}]){
    const server={...replacement,default_tools_approval_mode:'prompt',tools:{edit:{approval_mode:'prompt'}}};
    assert.deepEqual(plain(managedMcpApprovals({mcp_servers:{'gestalt-playwright':server}},owner)).mcp_servers['gestalt-playwright'],server);
  }
});

test('whole and dotted overrides normalize the final policy without changing third-party settings',()=>{
  const config={'mcp_servers.gestalt-playwright.enabled':true,
    'mcp_servers.gestalt-playwright.tools.browser_navigate.approval_mode':'prompt',
    'mcp_servers.third.tools.edit.approval_mode':'writes',
    approval_policy:'never',sandbox_mode:'read-only',permissions:{filesystem:{'/workspace':'read'}}};
  const result=plain(managedMcpOverrides(config,baseline,owner));
  assert.equal(result.mcp_servers['gestalt-playwright'].enabled,true);
  assert.equal(result.mcp_servers['gestalt-playwright'].tools.browser_navigate.approval_mode,'approve');
  assert.equal(result['mcp_servers.third.tools.edit.approval_mode'],'writes');
  assert.equal(result['mcp_servers.gestalt-playwright.tools.browser_navigate.approval_mode'],undefined);
  assert.deepEqual(result.permissions,config.permissions);assert.equal(result.approval_policy,'never');
  assert.equal(result.sandbox_mode,'read-only');
  const off=plain(managedMcpOverrides({mcp_servers:{'gestalt-playwright':{enabled:false}}},baseline,owner));
  assert.equal(off.mcp_servers['gestalt-playwright'].enabled,false);
});

test('Mobile launch boundary propagates approvals for fresh, resumed and dynamically activated threads',async t=>{
  const root=await mkdtemp(join(tmpdir(),'gestalt-mcp-policy-'));
  t.after(()=>rm(root,{recursive:true,force:true}));
  const native=join(root,'codex');
  await writeFile(native,`#!${process.execPath}
const {createInterface}=require('node:readline');
createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);if(m.id===undefined)return;
const result=m.method==='config/read'?{config:${JSON.stringify(baseline)}}:m.method==='initialize'?{userAgent:'fixture'}:{received:m.params};
console.log(JSON.stringify({id:m.id,result}));});`,{mode:0o755});
  const child=spawn('bash',[manager,'__codex-app-server',native,'--stdio'],{
    env:{...process.env,GESTALT_HOME:root,CODEX_HOME:owner.codexHome,GESTALT_MANAGER_BIN:manager},stdio:['pipe','pipe','pipe']});
  let next=0,stderr='';const pending=new Map();
  child.stderr.on('data',b=>stderr+=b);
  const lines=createInterface({input:child.stdout});
  lines.on('line',line=>{const m=JSON.parse(line),waiter=pending.get(m.id);if(waiter){pending.delete(m.id);m.error?waiter.reject(Error(JSON.stringify(m.error))):waiter.resolve(m.result);}});
  const request=(method,params={})=>new Promise((resolve,reject)=>{
    const id=++next,timer=setTimeout(()=>reject(Error('bridge timeout: '+stderr)),5000);
    pending.set(id,{resolve:x=>{clearTimeout(timer);resolve(x);},reject:e=>{clearTimeout(timer);reject(e);}});
    child.stdin.write(JSON.stringify({id,method,params})+'\n');
  });
  t.after(async()=>{child.stdin.end();await new Promise(r=>child.exitCode!==null?r():child.once('exit',r));lines.close();});
  await request('initialize');
  const read=await request('config/read');
  assert.equal(read.config.mcp_servers['gestalt-playwright'].default_tools_approval_mode,'approve');
  for(const method of ['thread/start','thread/resume','thread/fork'])for(const enabled of [false,true]){
    const params={threadId:'retained-thread',approvalPolicy:'never',sandbox:'read-only',cwd:root,
      config:{mcp_servers:{'gestalt-playwright':{...browser,enabled}},'mcp_servers.gestalt-playwright.tools.browser_navigate.approval_mode':'prompt'}};
    const result=(await request(method,params)).received;
    assert.equal(result.approvalPolicy,'never');assert.equal(result.sandbox,'read-only');
    const server=result.config.mcp_servers['gestalt-playwright'];
    assert.equal(server.default_tools_approval_mode,'approve');assert.equal(server.tools.browser_navigate.approval_mode,'approve');
    assert.equal(server.enabled,enabled);assert.equal(server.tools.browser_evaluate.enabled,false);
    assert.deepEqual(server.disabled_tools,['browser_evaluate']);
  }
});
