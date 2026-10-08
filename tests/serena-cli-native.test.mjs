import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, mkdir, readFile, writeFile, rm, readdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import test from 'node:test';

const manager = resolve('public/gestalt');
const catalog = ['get_symbols_overview', 'find_symbol', 'initial_instructions', 'replace_symbol_body']
  .map(name => ({ name, description: 'isolated native fixture', inputSchema: { type: 'object', properties:
    name === 'get_symbols_overview' ? { relative_path: { type: 'string' } } : {} } }));

async function execute(args, env, cwd) {
  const child = spawn('bash', [manager, ...args], { cwd, env, stdio: ['ignore', 'pipe', 'pipe'] });
  let stdout = '', stderr = '';
  child.stdout.on('data', data => stdout += data);
  child.stderr.on('data', data => stderr += data);
  const timer = setTimeout(() => child.kill('SIGKILL'), 35000);
  try {
    const code = await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', resolve); });
    return { code, stdout, stderr };
  } finally { clearTimeout(timer); }
}

// Entire public CLI path, actual native Codex and effective sandbox metadata.
// Only the release API/backend and Responses service are deterministic stand-ins;
// no downloaded language service, external model, or global config is involved.
for (const mode of ['default-approval', 'dual-profile-child', 'serena-only-resume', 'excluded', 'backend-failure', 'read-only', 'approval-denied', 'edit-approval-denied']) {
  test(`public CLI native Serena ${mode}`, { timeout: 120000 }, async () => {
    const root = await mkdtemp('/tmp/serena-cli-native-');
    const home = join(root, 'codex'), managed = join(root, 'managed'), repository = join(root, 'marketplace');
    const workspace = join(root, 'one/project'), sibling = join(root, 'two/project');
    const plugin = join(repository, 'plugins/gestalt'), context = join(repository, 'plugins/context-mode');
    const stubs = join(root, 'stubs');
    for (const path of [home, workspace, sibling, join(managed, 'serena'), join(managed, 'xerj'),
      join(home, 'bin'), join(home, 'xerj-data'), join(plugin, '.codex-plugin'), join(context, '.codex-plugin'),
      join(repository, '.agents/plugins'), join(stubs, 'ruamel'), join(stubs, 'serena/config')])
      await mkdir(path, { recursive: true });
    for (const project of [workspace, sibling]) await writeFile(join(project, 'fixture.py'), 'def fixture_symbol():\n    return 1\n');
    const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith('CONTEXT_MODE_')));
    Object.assign(env, { CODEX_HOME: home, GESTALT_HOME: managed, TMPDIR: '/tmp', PYTHONPATH: stubs, XERJ_READY_TIMEOUT_MS: '5000',
      GESTALT_SERENA_DOCTOR_TIMEOUT_MS: '500' });
    await writeFile(join(plugin, '.codex-plugin/plugin.json'), JSON.stringify({ name: 'gestalt', version: '0.0.1',
      description: 'native session composition fixture', skills: './skills/' }));
    await writeFile(join(context, '.codex-plugin/plugin.json'), JSON.stringify({ name: 'context-mode', version: '0.0.1',
      description: 'native manager prerequisite fixture' }));
    for (const name of ['serena', 'xerj', 'kept']) {
      await mkdir(join(plugin, 'skills', name), { recursive: true });
      await writeFile(join(plugin, 'skills', name, 'SKILL.md'), `---\nname: ${name}\ndescription: Native fixture ${name}.\n---\nFixture ${name} guidance.\n`);
    }
    await writeFile(join(repository, '.agents/plugins/marketplace.json'), JSON.stringify({ name: 'dyne-gestalt-agents', plugins:
      ['gestalt', 'context-mode'].map(name => ({ name, source: { source: 'local', path: './plugins/' + name } })) }));
    await writeFile(join(stubs, 'ruamel/yaml.py'), 'import json\nclass YAML:\n def load(self,s): return json.load(s)\n def dump(self,v,s): json.dump(v,s)\n');
    await writeFile(join(stubs, 'serena/config/serena_config.py'), `import pathlib,os,json
class SerenaConfig:
 @classmethod
 def from_config_file(cls):
  p=pathlib.Path(os.environ['SERENA_HOME'])/'serena_config.yml'
  if not p.exists():p.write_text('{}')
  return cls()
class ProjectConfigAutoGenerationMode:
 SYNCHRONOUS='sync'
class ProjectConfig:
 @classmethod
 def load(cls,root,config,autogen):
  p=pathlib.Path(os.environ['SERENA_HOME'])/'project.yml'
  if not p.exists():p.write_text(json.dumps({'project_name':root.name,'language_servers':['python']}))
  return cls()
`);
    const native = join(root, 'native-serena');
    await writeFile(native, `#!/usr/bin/env python3
import json,os,pathlib,sys
catalog=json.loads(${JSON.stringify(JSON.stringify(catalog))})
if ${JSON.stringify(mode)}=='backend-failure':sys.exit(1)
state=pathlib.Path(os.environ['SERENA_HOME'])
with open(state/'native-events.jsonl','a') as f:f.write(json.dumps({'args':sys.argv[1:],'cwd':os.getcwd(),'env':{k:os.environ[k] for k in ['SERENA_HOME','HOME','TMPDIR','UV_CACHE_DIR']}})+'\\n')
for line in sys.stdin:
 m=json.loads(line)
 if 'id' not in m:continue
 result={}
 if m['method']=='initialize':result={'protocolVersion':m['params']['protocolVersion'],'capabilities':{'tools':{}},'serverInfo':{'name':'Serena','version':'fixture'}}
 if m['method']=='tools/list':result={'tools':catalog}
 if m['method']=='tools/call':
  name=m['params']['name']
  value={'symbols':['fixture_symbol'],'workspace':os.getcwd()}
  if name=='get_symbols_overview':
   assert 'fixture_symbol' in pathlib.Path(m['params']['arguments']['relative_path']).read_text()
  if name=='replace_symbol_body':
   value={'writes':[]}
   outside=${JSON.stringify(workspace)} if os.getcwd()==${JSON.stringify(sibling)} else ${JSON.stringify(sibling)}
   for p in [pathlib.Path('edited.py'),pathlib.Path(outside)/'escaped.py']:
    try:p.write_text('fixture edit');value['writes'].append({'path':str(p),'allowed':True})
    except OSError:value['writes'].append({'path':str(p),'allowed':False})
  result={'content':[{'type':'text','text':json.dumps(value)}],'isError':False}
 print(json.dumps({'jsonrpc':'2.0','id':m['id'],'result':result}),flush=True)
`, { mode: 0o755 });
    // A real installed interpreter carries its packages. Keep fixture packages
    // beside its explicit interpreter, rather than widening MCP env forwarding.
    const python = join(root, 'python');
    await writeFile(python, `#!/usr/bin/env bash\nexport PYTHONPATH=${JSON.stringify(stubs)}\nexec /usr/bin/python3 "$@"\n`, { mode: 0o755 });
    const descriptor = { schemaVersion: 1, contractVersion: 1, version: '1.7.0', executable: native,
      python, uv: process.execPath, pythonInstallDir: root, pythonExecutables: [], tools: catalog };
    await writeFile(join(managed, 'serena/active.json'), JSON.stringify(descriptor));
    await writeFile(join(home, 'xerj-data/admin.key'), 'fixture-key');
    await writeFile(join(managed, 'xerj/xerj'), `#!${process.execPath}
if(process.argv[2]==='--version'){console.log('xerj v1.0.0-rc.87');process.exit(0);}
require('node:readline').createInterface({input:process.stdin}).on('line',line=>{
 const m=JSON.parse(line);if(m.id===undefined)return;
 const result=m.method==='initialize'?{protocolVersion:'2025-03-26',capabilities:{tools:{}},serverInfo:{name:'xerj-mcp',version:'1.0.0-rc.87'}}:
  m.method==='tools/list'?{tools:['xerj_search','xerj_map','xerj_code_search'].map(name=>({name,inputSchema:{type:'object',properties:{}}}))}:
  {content:[{type:'text',text:'independent fixture retrieval'}],isError:false};
 console.log(JSON.stringify({jsonrpc:'2.0',id:m.id,result}));
});
`, { mode: 0o755 });
    const backend = createServer((request, response) => {
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify(request.url === '/_cluster/health'
        ? { cluster_name: 'xerj', status: 'green', timed_out: false, number_of_nodes: 1 } : [{ index: 'fixture' }]));
    });
    await new Promise(resolve => backend.listen(0, '127.0.0.1', resolve));
    env.XERJ_URL = `http://127.0.0.1:${backend.address().port}`;
    if (mode === 'serena-only-resume') await rm(join(managed, 'xerj/xerj'));

    const requests = [], errors = [], actors = new Map();
    let spawned = false, waited = false, expectedSerena = mode !== 'excluded', phase = 'initial';
    const model = createServer(async (request, response) => {
      if (request.method !== 'POST') { response.end(JSON.stringify({ data: [] })); return; }
      let body = ''; for await (const part of request) body += part;
      const input = JSON.parse(body); requests.push({ phase, input });
      const tools = input.tools.flatMap(tool => tool.type === 'namespace' ? tool.tools.map(t => ({ ...t, namespace: tool.name })) : [tool]);
      const child = JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('CHILD_FIXTURE');
      const actorKey = phase + (child ? '-child' : '-root');
      const actor = actors.get(actorKey) ?? { index: 0 }; actors.set(actorKey, actor);
      const hasSerena = tools.some(tool => tool.name?.includes('get_symbols_overview'));
      const catalogText = JSON.stringify(input.input.filter(item => item.role === 'developer' && JSON.stringify(item.content).includes('<skills_instructions>')).at(-1));
      const notices = input.input.filter(item => /gestalt_serena_(capability|unavailable)/.test(JSON.stringify(item.content)));
      const guidance = JSON.stringify(notices.at(-1));
      if (hasSerena !== expectedSerena) errors.push(`${actorKey}: Serena catalog ${hasSerena}`);
      if (catalogText.includes('gestalt:serena') !== expectedSerena) errors.push(`${actorKey}: Serena skill selector`);
      if (guidance.includes('<gestalt_serena_capability>') !== expectedSerena) errors.push(`${actorKey}: Serena guidance`);
      if (expectedSerena && !guidance.includes('does not prove language readiness')) errors.push(`${actorKey}: semantic readiness overstated`);
      if (!catalogText.includes('gestalt:kept')) errors.push(`${actorKey}: unrelated profile skill lost`);
      if (!JSON.stringify(input.input).includes('KEPT_INSTRUCTIONS')) errors.push(`${actorKey}: original instructions lost`);
      if (!child && phase === 'initial' && JSON.stringify(input.input).split('KEPT_SESSION_HOOK').length - 1 !== 1)
        errors.push(`${actorKey}: unrelated hook lost or duplicated`);
      const expectedXerj = mode !== 'serena-only-resume';
      if (tools.some(tool => tool.name?.includes('xerj_search')) !== expectedXerj) errors.push(`${actorKey}: independent XERJ catalog`);
      let item;
      const names = [...(expectedSerena ? ['get_symbols_overview', 'replace_symbol_body'] : []), ...(expectedXerj ? ['xerj_search'] : [])];
      if (actor.index < names.length) {
        const name = names[actor.index++], tool = tools.find(tool => tool.name?.includes(name));
        if (tool) item = { id: 'fixture_call', type: 'function_call', status: 'completed', name: tool.name,
          ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'fixture_call_' + actor.index,
          arguments: JSON.stringify(name === 'get_symbols_overview' ? { relative_path: 'fixture.py' } : {}) };
      } else if (mode === 'dual-profile-child' && !child && !spawned) {
        const tool = tools.find(tool => tool.name === 'spawn_agent');
        if (tool) { spawned = true; item = { id: 'spawn', type: 'function_call', status: 'completed', name: tool.name,
          ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'spawn', arguments: JSON.stringify({ message: 'CHILD_FIXTURE: use Serena in this workspace.', fork_context: false }) }; }
      } else if (mode === 'dual-profile-child' && !child && !waited) {
        const tool = tools.find(tool => tool.name === 'wait_agent');
        const agent = input.input.filter(item => item.type === 'function_call_output').map(item => { try { return JSON.parse(item.output).agent_id; } catch { return undefined; } }).find(Boolean);
        if (tool && agent) { waited = true; item = { id: 'wait', type: 'function_call', status: 'completed', name: tool.name,
          ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'wait', arguments: JSON.stringify({ targets: [agent], timeout_ms: 10000 }) }; }
      }
      item ??= { id: 'done', type: 'message', status: 'completed', role: 'assistant', content: [{ type: 'output_text', text: 'Fixture complete.', annotations: [] }] };
      const result = { id: `response_${requests.length}`, object: 'response', created_at: 1, status: 'completed', model: 'fixture', output: [item],
        usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
      response.writeHead(200, { 'Content-Type': 'text/event-stream' });
      for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
        { type: 'response.output_item.added', output_index: 0, item }, { type: 'response.output_item.done', output_index: 0, item },
        { type: 'response.completed', response: result }]) response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
      response.end();
    });
    await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
    try {
      // Install fixture plugins with the real native marketplace, then satisfy
      // the manager's normal context-mode prerequisite without invoking setup.
      for (const args of [['plugin', 'marketplace', 'add', repository], ['plugin', 'add', 'gestalt@dyne-gestalt-agents'],
        ['plugin', 'add', 'context-mode@dyne-gestalt-agents']]) {
        const result = await new Promise((resolve, reject) => {
          const p = spawn('codex', args, { cwd: workspace, env, stdio: ['ignore', 'ignore', 'pipe'] }); let stderr = '';
          p.stderr.on('data', data => stderr += data); p.once('error', reject); p.once('exit', code => resolve({ code, stderr }));
        });
        assert.equal(result.code, 0, result.stderr);
      }
      const runtime = join(managed, 'runtime/context-mode'); await mkdir(runtime, { recursive: true });
      await writeFile(join(runtime, '.context-mode-prepared.json'), JSON.stringify({ packageVersion: '0.0.1',
        nodeModulesAbi: process.versions.modules, platform: process.platform, arch: process.arch }));
      await writeFile(join(runtime, 'cli.bundle.mjs'), 'console.log("fixture context-mode ready")');
      await writeFile(join(runtime, 'server.bundle.mjs'), '');
      const contextLauncher = join(home, 'bin/context-mode-mcp.mjs');
      await writeFile(contextLauncher, `import {createInterface} from 'node:readline';
createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);if(m.id===undefined)return;
console.log(JSON.stringify({jsonrpc:'2.0',id:m.id,result:m.method==='initialize'?{protocolVersion:m.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'fixture-context',version:'1'}}:{tools:[]}}));});`);
      const hookCommand = JSON.stringify(process.execPath) + " -e 'process.stdout.write(\"KEPT_SESSION_HOOK\")'";
      // Obtain this unrelated hook's trust hash with native hooks/list.
      const { createInterface } = await import('node:readline');
      const probe = spawn('codex', ['app-server', '--stdio', '-c', `hooks.SessionStart=[{hooks=[{type="command",command=${JSON.stringify(hookCommand)}}]}]`],
        { cwd: workspace, env, stdio: ['pipe', 'pipe', 'ignore'] });
      const lines = createInterface({ input: probe.stdout }), pending = new Map(); let id = 0;
      lines.on('line', line => { const m = JSON.parse(line), entry = pending.get(m.id); if (entry) { pending.delete(m.id); entry(m.result); } });
      const rpc = (method, params) => new Promise(resolve => { const n = ++id; pending.set(n, resolve); probe.stdin.write(JSON.stringify({ id: n, method, params }) + '\n'); });
      await rpc('initialize', { clientInfo: { name: 'fixture-hook', version: '1' }, capabilities: {} });
      probe.stdin.write(JSON.stringify({ method: 'initialized', params: {} }) + '\n');
      const hash = (await rpc('hooks/list', { cwds: [workspace] })).data[0].hooks.find(h => h.command === hookCommand).currentHash;
      lines.close(); probe.stdin.end(); probe.kill(); await new Promise(resolve => probe.once('exit', resolve));
      const profilePath = join(home, 'fixture.config.toml');
      const profile = `[features]\nhooks=true\n[projects.${JSON.stringify(workspace)}]\ntrust_level="trusted"\n[[skills.config]]\nname="gestalt:kept"\nenabled=true\n${mode === 'excluded' ? '[[skills.config]]\nname="gestalt:serena"\nenabled=false\n' : ''}`;
      await writeFile(profilePath, profile);
      const config = `default_permissions="fixture"\nmodel_provider="fixture"\nmodel="fixture"\napproval_policy="never"\ndeveloper_instructions="KEPT_INSTRUCTIONS"\n[permissions.fixture]\nextends=":read-only"\n[permissions.fixture.filesystem]\n${JSON.stringify(workspace)}="${mode === 'read-only' ? 'read' : 'write'}"\n[model_providers.fixture]\nname="fixture"\nbase_url="http://127.0.0.1:${model.address().port}/v1"\nwire_api="responses"\nrequires_openai_auth=false\nrequest_max_retries=0\n[features]\nhooks=true\ntool_search=false\nmulti_agent=true\n[plugins."gestalt@dyne-gestalt-agents"]\nenabled=true\n[plugins."context-mode@dyne-gestalt-agents"]\nenabled=false\n[mcp_servers.gestalt-serena]\ncommand=${JSON.stringify(manager)}\nargs=["serena","mcp","--cwd",${JSON.stringify(workspace)}]\n${mode === 'default-approval' ? '' : mode !== 'approval-denied' ? 'default_tools_approval_mode="approve"\n' : 'default_tools_approval_mode="prompt"\n'}${mode === 'edit-approval-denied' ? '[mcp_servers.gestalt-serena.tools.replace_symbol_body]\napproval_mode="prompt"\n' : ''}[mcp_servers.context-mode]\ncommand="node"\nargs=[${JSON.stringify(contextLauncher)}]\n[[hooks.SessionStart]]\n[[hooks.SessionStart.hooks]]\ntype="command"\ncommand=${JSON.stringify(hookCommand)}\n[hooks.state.${JSON.stringify(join(home, 'config.toml') + ':session_start:0:0')}]\ntrusted_hash=${JSON.stringify(hash)}\n[marketplaces.dyne-gestalt-agents]\nsource_type="local"\nsource=${JSON.stringify(repository)}\n`;
      await writeFile(join(home, 'config.toml'), config);
      const result = await execute(['cli', '-pfixture', 'exec', '--skip-git-repo-check', '--json', '-C', workspace, 'ROOT_FIXTURE: exercise workspace capabilities.'], env, sibling);
      assert.equal(result.code, 0, result.stderr.slice(-2500));
      assert.ok(requests.length, 'public CLI did not reach native Responses fixture');
      assert.deepEqual(errors, []);
      assert.equal(await readFile(join(home, 'config.toml'), 'utf8'), config, 'persistent config changed');
      assert.equal(await readFile(profilePath, 'utf8'), profile, 'named profile changed');
      const outputs = requests.flatMap(({ input }) => input.input.filter(item => item.type === 'function_call_output'));
      const toolText = outputs.flatMap(item => Array.isArray(item.output) ? item.output.map(content => content.text ?? '') : [item.output]).join('\n');
      if (mode === 'excluded') {
        assert.deepEqual(await readdir(workspace), ['fixture.py'], 'excluded capability prepared project state');
      } else if (mode === 'approval-denied') {
        assert.ok(outputs.some(item => JSON.stringify(item.output).includes('requires approval')), 'default tool approval was bypassed');
        assert.deepEqual(await readdir(workspace), ['fixture.py'], 'unapproved call started backend');
      } else if (mode === 'read-only' || mode === 'backend-failure') {
        assert.ok(outputs.some(item => JSON.stringify(item.output).includes('Serena') && /unavailable|failed|exited/.test(JSON.stringify(item.output))), 'native effective-policy failure missing');
        await assert.rejects(readFile(join(workspace, 'edited.py')), { code: 'ENOENT' });
      } else {
        const evidence = await readFile(join(workspace, '.gestalt/serena/native-events.jsonl'), 'utf8')
          .catch(() => { throw new Error('Native backend evidence missing: ' + JSON.stringify(outputs.slice(0, 3)).slice(-2000) + '\n' + result.stderr.slice(-1500)); });
        const events = evidence.trim().split('\n').map(JSON.parse);
        assert.equal(events.length, mode === 'dual-profile-child' ? 2 : 1, 'wrong per-thread backend count');
        for (const event of events) {
          assert.equal(event.cwd, workspace);
          assert.equal(event.args[event.args.indexOf('--project') + 1], workspace);
          assert.equal(event.args[event.args.indexOf('--context') + 1], 'codex');
          assert.equal(event.args[event.args.indexOf('--mode') + 1], 'editing');
          for (const path of Object.values(event.env)) assert.ok(path.startsWith(join(workspace, '.gestalt/serena')));
        }
        assert.ok(outputs.some(item => JSON.stringify(item.output).includes('fixture_symbol')), 'first policy-bearing semantic call missing');
        if (mode === 'edit-approval-denied') {
          assert.ok(toolText.includes('requires approval'), 'per-tool prompt override was bypassed');
          await assert.rejects(readFile(join(workspace, 'edited.py')), { code: 'ENOENT' });
        } else {
          assert.match(toolText, /"allowed":\s*false/, 'effective sibling write denial missing');
          assert.equal(await readFile(join(workspace, 'edited.py'), 'utf8'), 'fixture edit');
        }
        await assert.rejects(readFile(join(sibling, 'escaped.py')), { code: 'ENOENT' });
        if (mode === 'dual-profile-child') assert.ok(spawned && waited && actors.has('initial-child'), 'native child scope missing');
      }
      if (mode === 'serena-only-resume') {
        phase = 'second-workspace';
        // Explicit fixture permissions select the second project and narrow the
        // first back to read-only. The manager must forward these unchanged.
        const secondProfile = profile + `[permissions.fixture.filesystem]\n${JSON.stringify(workspace)}="read"\n${JSON.stringify(sibling)}="write"\n`;
        await writeFile(join(home, 'second.config.toml'), secondProfile);
        const second = await execute(['cli', '-psecond', '-C', sibling, 'exec', '--skip-git-repo-check', '--json', 'SECOND_FIXTURE: use this project.'], env, workspace);
        assert.equal(second.code, 0, second.stderr.slice(-1500));
        assert.equal(await readFile(join(home, 'second.config.toml'), 'utf8'), secondProfile);
        assert.equal(await readFile(join(home, 'config.toml'), 'utf8'), config);
        assert.deepEqual(errors, []);
        const evidence = await readFile(join(sibling, '.gestalt/serena/native-events.jsonl'), 'utf8').catch(() => {
          const last = requests.filter(entry => entry.phase === 'second-workspace').at(-1)?.input.input.filter(item => item.type === 'function_call_output');
          throw new Error('Second workspace backend missing: ' + JSON.stringify(last).slice(-2500) + second.stderr.slice(-1000));
        });
        const event = JSON.parse(evidence.trim());
        assert.equal(event.cwd, sibling);
        assert.equal(event.args[event.args.indexOf('--project') + 1], sibling);
        assert.equal(event.args[event.args.indexOf('--context') + 1], 'codex');
        assert.equal(event.args[event.args.indexOf('--mode') + 1], 'editing');
        assert.equal(event.env.SERENA_HOME, join(sibling, '.gestalt/serena'));
        assert.equal(await readFile(join(sibling, 'edited.py'), 'utf8'), 'fixture edit');
        await assert.rejects(readFile(join(workspace, 'escaped.py')), { code: 'ENOENT' });
        phase = 'resume'; expectedSerena = false;
        await rm(join(managed, 'serena/active.json'));
        const resumed = await execute(['cli', '-pfixture', '-C', workspace, 'exec', 'resume', '--last', '--json', '--skip-git-repo-check', 'RESUME_FIXTURE: report current capabilities.'], env, sibling);
        assert.equal(resumed.code, 0, resumed.stderr.slice(-1500));
        assert.ok(requests.some(entry => entry.phase === 'resume'), 'native resume not reached');
        assert.deepEqual(errors, []);
        assert.equal(await readFile(join(home, 'config.toml'), 'utf8'), config);
      }
      if (mode !== 'serena-only-resume') assert.ok(outputs.some(item => JSON.stringify(item.output).includes('independent fixture retrieval')), 'Serena failure disabled XERJ');
    } finally {
      model.closeAllConnections(); backend.closeAllConnections();
      await Promise.all([new Promise(resolve => model.close(resolve)), new Promise(resolve => backend.close(resolve))]);
      await rm(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    }
  });
}
