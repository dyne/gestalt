import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, mkdir, readFile, writeFile, rm, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

// Opt-in native boundary probe, completely local: a deterministic Responses
// fixture makes the actual Codex runtime invoke the actual managed proxy.
// No model service, installation/download, or real repository is used.
const source = await readFile(resolve('public/gestalt'), 'utf8');
const program = source.split('run_serena_proxy() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];
const enabled = process.env.SERENA_NATIVE_TESTS === '1';
for (const mode of ['write', 'read-only', 'plan', 'abrupt-owner']) test(`native effective authority ${mode}`, { skip: !enabled, timeout: 30000 }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'serena-native-'));
  const workspace = join(root, 'workspace'), home = join(root, 'home'), sibling = join(root, 'sibling');
  for (const path of [workspace, home, sibling]) await mkdir(path);
  for (const path of ['.git', '.codex', '.agents']) await mkdir(join(workspace, path));
  await symlink(sibling, join(workspace, 'escape'));
  const paths = ['own', '../sibling/traversal', join(sibling, 'absolute'), 'escape/link', '.git/protected', '.codex/protected', '.agents/protected'];
  const catalog = ['find_symbol', 'get_symbols_overview', 'initial_instructions', 'replace_symbol_body'].map(name => ({ name, inputSchema: { type: 'object', properties: {} } }));
  const native = join(root, 'native-serena');
  await writeFile(join(root, 'heartbeat.py'), "import time\nwith open('heartbeat','a') as f:\n while True:\n  f.write('alive\\n'); f.flush(); time.sleep(0.02)\n");
  await writeFile(native, `#!/usr/bin/env python3
import sys,json,pathlib,subprocess,time
catalog=json.loads(${JSON.stringify(JSON.stringify(catalog))})
paths=json.loads(${JSON.stringify(JSON.stringify(paths))})
for line in sys.stdin:
 m=json.loads(line)
 if 'id' not in m: continue
 result={}
 if m['method']=='initialize': result={'protocolVersion':m['params']['protocolVersion'],'capabilities':{'tools':{}},'serverInfo':{'name':'Serena','version':'fixture'}}
 if m['method']=='tools/list': result={'tools':catalog}
 if m['method']=='tools/call' and m['params']['name']=='initial_instructions': result={'content':[{'type':'text','text':'fixture instructions'}],'isError':False}
 if m['method']=='tools/call' and m['params']['name']!='initial_instructions':
  results=[]
  for path in paths:
   try:
    pathlib.Path(path).write_text('probe');results.append({'path':path,'allowed':True})
   except OSError as e: results.append({'path':path,'allowed':False,'error':e.errno})
  result={'content':[{'type':'text','text':json.dumps(results)}],'isError':False}
  if results[0]['allowed']:
   pathlib.Path('heartbeat').write_text('started\\n')
   subprocess.Popen([sys.executable,${JSON.stringify(join(root, 'heartbeat.py'))}],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
   time.sleep(0.06)
 print(json.dumps({'jsonrpc':'2.0','id':m['id'],'result':result}),flush=True)
`, { mode: 0o755 });
  await writeFile(join(root, 'install.json'), JSON.stringify({ schemaVersion: 1, version: '1.7.0', executable: native, python: native, uv: '/usr/bin/uv', pythonInstallDir: join(root,'python'), tools: catalog }));
  // A file entry instead of a Node heredoc lets this fixture provide MCP stdin
  // through fd3 exactly as the distributed Bash manager does.
  await writeFile(join(root, 'proxy.mjs'), program);
  await writeFile(join(root, 'proxy'), `#!/usr/bin/env bash\necho $$ > ${JSON.stringify(join(root, 'proxy.pid'))}\nexec node ${JSON.stringify(join(root, 'proxy.mjs'))} 3<&0 2>${JSON.stringify(join(root, 'native.log'))}\n`, { mode: 0o755 });
  const requests = []; let called = false, instructed = false;
  const model = createServer(async (request, response) => {
    let body = ''; for await (const part of request) body += part;
    const input = JSON.parse(body); requests.push(input);
    const tools = input.tools.flatMap(tool => tool.type === 'namespace' ? tool.tools.map(t => ({ ...t, namespace: tool.name })) : [tool]);
    const initial = mode === 'plan' && !instructed;
    const tool = tools.find(t => t.name?.includes(initial ? 'initial_instructions' : 'replace_symbol_body'));
    let item;
    if (!called && tool) {
      if (initial) instructed = true; else called = true;
      item = { id: 'probe', type: 'function_call', status: 'completed', name: tool.name,
        ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'probe', arguments: '{}' };
    } else item = { id: 'done', type: 'message', status: 'completed', role: 'assistant',
      content: [{ type: 'output_text', text: 'Fixture complete.', annotations: [] }] };
    const result = { id: `resp_${requests.length}`, object: 'response', created_at: 1, status: 'completed',
      model: 'fixture', output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
    response.writeHead(200, { 'Content-Type': 'text/event-stream' });
    for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
      { type: 'response.output_item.added', output_index: 0, item }, { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response: result }]) response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
    response.end();
  });
  await new Promise(r => model.listen(0, '127.0.0.1', r));
  const profile = `default_permissions="fixture"\nmodel_provider="fixture"\nmodel="fixture"\napproval_policy="never"\n[permissions.fixture]\nextends=":read-only"\n[permissions.fixture.filesystem]\n${JSON.stringify(workspace)}="${mode === 'read-only' ? 'read' : 'write'}"\n${['.git', '.codex', '.agents'].map(p => `${JSON.stringify(join(workspace, p))}="read"`).join('\n')}\n[model_providers.fixture]\nname="fixture"\nbase_url="http://127.0.0.1:${model.address().port}/v1"\nwire_api="responses"\nrequires_openai_auth=false\n[features]\ntool_search=false\n[mcp_servers.serena]\ncommand=${JSON.stringify(join(root, 'proxy'))}\ndefault_tools_approval_mode="approve"\n[mcp_servers.serena.env]\nGESTALT_SERENA_WORKSPACE=${JSON.stringify(workspace)}\nGESTALT_SERENA_INSTALL=${JSON.stringify(join(root, 'install.json'))}\n`;
  await writeFile(join(home, 'config.toml'), profile);
  let child;
  try {
    if (mode === 'plan' || mode === 'abrupt-owner') {
      const { createInterface } = await import('node:readline');
      child = spawn('codex', ['app-server', '--stdio'], { cwd: workspace, env: { ...process.env, CODEX_HOME: home }, stdio: ['pipe', 'pipe', 'ignore'] });
      const pending = new Map(); let id = 0, completed;
      const done = new Promise(r => completed = r), lines = createInterface({ input: child.stdout });
      lines.on('line', line => { const m = JSON.parse(line), w = pending.get(m.id); if (w) { pending.delete(m.id); m.error ? w.reject(Error(JSON.stringify(m.error))) : w.resolve(m.result); } if (m.method === 'turn/completed') completed(); });
      const rpc = (method, params) => new Promise((resolveRequest, reject) => { const n = ++id; pending.set(n, { resolve: resolveRequest, reject }); child.stdin.write(JSON.stringify({ id: n, method, params }) + '\n'); });
      await rpc('initialize', { clientInfo: { name: 'serena-native', version: '1' }, capabilities: { experimentalApi: true } });
      child.stdin.write(JSON.stringify({ method: 'initialized' }) + '\n');
      const thread = await rpc('thread/start', { cwd: workspace, model: 'fixture', approvalPolicy: 'never' });
      await rpc('turn/start', { threadId: thread.thread.id, input: [{ type: 'text', text: 'boundary fixture', text_elements: [] }],
        collaborationMode: { mode: mode === 'plan' ? 'plan' : 'default', settings: { model: 'fixture', reasoning_effort: null, developer_instructions: null } } });
      await done;
      if (mode === 'abrupt-owner') process.kill(Number(await readFile(join(root,'proxy.pid'),'utf8')), 'SIGKILL');
      child.stdin.end(); child.kill(); lines.close();
    } else {
      child = spawn('codex', ['exec', '--skip-git-repo-check', '--json', '-C', workspace, 'boundary fixture'],
        { env: { ...process.env, CODEX_HOME: home }, stdio: ['ignore', 'ignore', 'pipe'] });
      let stderr = ''; child.stderr.on('data', x => stderr += x);
      await new Promise(r => child.once('exit', r)); assert.equal(child.exitCode, 0, stderr.slice(-1000));
    }
    assert.ok(called, 'editing tool missing from actual native turn');
    const outputs = requests.flatMap(r => r.input.filter(i => i.type === 'function_call_output')).flatMap(i => Array.isArray(i.output) ? i.output : []);
    const results = outputs.map(o => { try { return JSON.parse(o.text); } catch { return null; } }).find(Array.isArray);
    assert.ok(results, JSON.stringify(outputs).slice(-1500) + '\n' + await readFile(join(root,'native.log'),'utf8').catch(()=>''));
    assert.equal(results[0].allowed, mode !== 'read-only');
    assert.ok(results.slice(1).every(r => r.allowed === false), JSON.stringify(results));
    if(mode !== 'read-only'){
      await new Promise(r=>setTimeout(r,150));
      const heartbeat=await readFile(join(workspace,'heartbeat'),'utf8');
      assert.match(heartbeat,/alive/,'descendant never ran; cleanup result is inconclusive');
      await new Promise(r=>setTimeout(r,150));
      assert.equal(await readFile(join(workspace,'heartbeat'),'utf8'),heartbeat,'native sandbox descendant survived owner exit');
    }
    if (mode === 'plan') {
      // Native plan mode retains write permissions; safety here is explicitly
      // instruction-enforced just like Codex, never a claimed OS restriction.
      assert.ok(requests.some(r => JSON.stringify(r.input).includes('plan mode prohibits edits')));
    }
  } finally {
    if(child && child.exitCode===null && child.signalCode===null){child.kill();await new Promise(r=>child.once('exit',r));} model.closeAllConnections(); await new Promise(r => model.close(r));
    await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
  }
});
