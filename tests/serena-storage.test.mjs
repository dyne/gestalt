import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { mkdtemp, mkdir, writeFile, readFile, rm, symlink, readdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';

const source = await readFile(resolve('public/gestalt'), 'utf8');
const prepare = source.split("serena_workspace_prepare() {\n  cat <<'PYTHON'\n")[1].split('\nPYTHON\n}')[0];
const manager = resolve('public/gestalt');
async function run(command, args, env, input) {
  const child = spawn(command, args, { env, stdio: ['pipe', 'pipe', 'pipe'] });
  let stdout = '', stderr = '';
  child.stdout.on('data', part => stdout += part); child.stderr.on('data', part => stderr += part);
  child.stdin.end(input);
  const code = await new Promise((r, reject) => { child.once('error', reject); child.once('exit', r); });
  return { code, stdout, stderr };
}
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'serena-storage-'));
  const workspace = join(root, 'workspace with spaces'), home = join(root, 'home'), stubs = join(root, 'stubs');
  for (const path of [workspace, home, join(stubs, 'ruamel'), join(stubs, 'serena/config')]) await mkdir(path, { recursive: true });
  // Minimal release API stand-ins keep ordinary tests offline. The real-release
  // smoke separately verifies these same invariants with Serena1.7.0 itself.
  await writeFile(join(stubs, 'ruamel/yaml.py'), `import json\nclass YAML:\n def load(self,s): return json.load(s)\n def load_all(self,s): return (json.loads(part) for part in s.read().split("---") if part.strip())\n def dump(self,v,s): json.dump(v,s)\n`);
  await writeFile(join(stubs, 'serena/config/serena_config.py'), `import pathlib,os,json
class SerenaConfig:
 @classmethod
 def from_config_file(cls):
  p=pathlib.Path(os.environ['SERENA_HOME'])/'serena_config.yml'
  if not p.exists(): p.write_text(json.dumps({'ignored_paths':['kept-global/**'],'ls_specific_settings':{}}))
  return cls()
class ProjectConfigAutoGenerationMode:
 SYNCHRONOUS='sync'
class ProjectConfig:
 @classmethod
 def load(cls,root,config,autogen):
  p=pathlib.Path(os.environ['SERENA_HOME'])/'project.yml'
  if not p.exists(): p.write_text(json.dumps({'project_name':root.name,'language_servers':['python']}))
  return cls()
`);
  const executable = join(root, 'native');
  await writeFile(executable, `#!/usr/bin/env python3\nimport os,json,sys\nprint(json.dumps({'env':dict(os.environ),'args':sys.argv[1:]}))\n`, { mode: 0o755 });
  const settings = { python: '/usr/bin/python3', uv: join(root, 'uv'), pythonInstallDir: join(root, 'python') };
  const env = { ...process.env, HOME: home, PYTHONPATH: stubs };
  const boot = (cwd = workspace) => run('python3', ['-c', prepare, cwd, executable, JSON.stringify(settings),
    'start-mcp-server', '--project', cwd, '--context', 'codex', '--mode', 'editing'], env);
  return { root, workspace, home, boot, env, settings, close: () => rm(root, { recursive: true, force: true }) };
}

for (const language of ['python', 'typescript', 'cpp']) test(`native storage preserves existing ${language} project settings and legacy data`, async () => {
  const f = await fixture();
  try {
    const legacyRoot = join(f.workspace, '.serena'); await mkdir(legacyRoot);
    const legacy = JSON.stringify({ project_name: 'kept', language_servers: [language], ignored_paths: ['existing/**'], ls_specific_settings: { custom: { enabled: true } } });
    await writeFile(join(legacyRoot, 'project.yml'), legacy);
    await writeFile(join(legacyRoot, 'memory.md'), 'legacy data remains here');
    const result = await f.boot(); assert.equal(result.code, 0, result.stderr);
    const state = join(f.workspace, '.gestalt/serena'), output = JSON.parse(result.stdout);
    for (const key of ['SERENA_HOME', 'HOME', 'TMPDIR', 'UV_TOOL_DIR', 'UV_TOOL_BIN_DIR', 'UV_CACHE_DIR',
      'XDG_CACHE_HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'PYTHONPYCACHEPREFIX', 'PYTHONUSERBASE', 'npm_config_cache'])
      assert.ok(output.env[key].startsWith(state), `${key} escaped project state`);
    assert.equal(output.env.UV_PYTHON_INSTALL_DIR, f.settings.pythonInstallDir);
    assert.equal(output.env.UV_PYTHON_DOWNLOADS, 'never');
    assert.deepEqual(output.args.slice(-4), ['--context', 'codex', '--mode', 'editing']);
    const global = JSON.parse(await readFile(join(state, 'serena_config.yml')));
    assert.equal(global.project_serena_folder_location, '$projectDir/.gestalt/serena');
    assert.equal(global.ls_specific_settings.cpp.compile_commands_dir, '.gestalt/serena/clangd');
    assert.deepEqual(global.ignored_paths, ['kept-global/**', '.gestalt/**']);
    assert.deepEqual(global.trusted_project_path_patterns, []);
    assert.equal(global.web_dashboard, false);
    const project = JSON.parse(await readFile(join(state, 'project.yml')));
    assert.deepEqual(project.language_servers, [language]);
    assert.deepEqual(project.ls_specific_settings, { custom: { enabled: true } });
    assert.deepEqual(project.ignored_paths, ['existing/**', '.gestalt/**']);
    assert.equal(await readFile(join(legacyRoot, 'project.yml'), 'utf8'), legacy);
    assert.equal(await readFile(join(legacyRoot, 'memory.md'), 'utf8'), 'legacy data remains here');
    const before = await readFile(join(state, 'project.yml'), 'utf8');
    assert.equal((await f.boot()).code, 0);
    assert.equal(await readFile(join(state, 'project.yml'), 'utf8'), before);
    assert.deepEqual(await readdir(f.home), []);
  } finally { await f.close(); }
});

for (const path of ['.gestalt', '.gestalt/serena', '.gestalt/serena/uv-cache', '.gestalt/serena/serena_config.yml'])
  test(`redirected ${path} fails before writes`, async () => {
    const f = await fixture();
    try {
      const outside = join(f.root, 'outside'); await mkdir(outside);
      const target = join(f.workspace, path); await mkdir(resolve(target, '..'), { recursive: true });
      await symlink(outside, target);
      const result = await f.boot(); assert.notEqual(result.code, 0); assert.match(result.stderr, /symlink/);
      assert.deepEqual(await readdir(outside), []);
    } finally { await f.close(); }
  });

test('equal-basename workspaces and a worktree git file have independent state', async () => {
  const f = await fixture();
  try {
    const roots = [join(f.root, 'one/project'), join(f.root, 'two/project')];
    for (const root of roots) {
      await mkdir(root, { recursive: true }); await writeFile(join(root, '.git'), 'gitdir: /unused/worktree\n');
      const result = await f.boot(root); assert.equal(result.code, 0, result.stderr);
      assert.equal(JSON.parse(result.stdout).env.SERENA_HOME, join(root, '.gestalt/serena'));
    }
  } finally { await f.close(); }
});

test('concurrent preparation of one workspace retains coherent native configuration', async () => {
  const f = await fixture();
  try {
    const results = await Promise.all([f.boot(), f.boot()]);
    for (const result of results) assert.equal(result.code, 0, result.stderr);
    const config = JSON.parse(await readFile(join(f.workspace, '.gestalt/serena/project.yml')));
    assert.deepEqual(config.ignored_paths, ['.gestalt/**']);
  } finally { await f.close(); }
});

test('clangd databases and caches are isolated for equal-basename source directories', async () => {
  const f = await fixture();
  try {
    const sources = ['one/project', 'two/project'];
    for (const relative of sources) {
      const directory = join(f.workspace, relative);
      await mkdir(directory, { recursive: true });
      await writeFile(join(directory, 'compile_commands.json'), JSON.stringify([
        { directory: '.', file: 'main.cpp', arguments: ['clang++', '-D' + relative.split('/')[0].toUpperCase(), '-c', 'main.cpp'] },
      ]));
    }
    assert.equal((await f.boot()).code, 0);
    const state = join(f.workspace, '.gestalt/serena');
    const configPath = join(state, 'xdg-config/clangd/config.yaml');
    const config = await readFile(configPath, 'utf8');
    const fragments = config.split('\n').filter(line => line.startsWith('{')).map(line => JSON.parse(line));
    assert.equal(fragments.length, 2);
    const databases = fragments.map(fragment => fragment.CompileFlags.CompilationDatabase);
    assert.notEqual(databases[0], databases[1]);
    for (const [index, directory] of databases.entries()) {
      const commands = JSON.parse(await readFile(join(directory, 'compile_commands.json')));
      assert.equal(commands[0].directory, join(f.workspace, sources[index]));
      assert.ok(directory.startsWith(join(state, 'clangd/project-')));
      await writeFile(join(directory, '.cache/probe'), 'managed cache');
      assert.equal(await readFile(join(directory, '../cache/probe'), 'utf8'), 'managed cache');
      assert.deepEqual(await readdir(join(f.workspace, sources[index])), ['compile_commands.json']);
    }
    assert.equal((await f.boot()).code, 0);
    assert.equal(await readFile(configPath, 'utf8'), config);
    await writeFile(join(f.workspace, sources[0], 'compile_commands.json'), '[]');
    assert.equal((await f.boot()).code, 0);
    assert.deepEqual(JSON.parse(await readFile(join(databases[0], 'compile_commands.json'))), []);
    assert.equal(await readFile(join(state, 'home/Library/Preferences/clangd/config.yaml'), 'utf8'), config);
  } finally { await f.close(); }
});

test('absolute compilation databases are relocated without changing their build commands', async () => {
  const f = await fixture();
  try {
    const commands = [{ directory: f.workspace, file: 'main.cpp', arguments: ['clang++', '-std=c++20', '-c', 'main.cpp'] }];
    const original = JSON.stringify(commands);
    await writeFile(join(f.workspace, 'compile_commands.json'), original);
    assert.equal((await f.boot()).code, 0);
    const config = await readFile(join(f.workspace, '.gestalt/serena/xdg-config/clangd/config.yaml'), 'utf8');
    const fragment = JSON.parse(config.split('\n').find(line => line.startsWith('{')));
    assert.deepEqual(JSON.parse(await readFile(join(fragment.CompileFlags.CompilationDatabase, 'compile_commands.json'))), commands);
    assert.equal(await readFile(join(f.workspace, 'compile_commands.json'), 'utf8'), original);
    assert.equal((await readdir(f.workspace)).includes('.cache'), false);
  } finally { await f.close(); }
});

test('real clangd stores shards only in the managed per-source cache', { skip: !process.env.SERENA_TEST_CLANGD, timeout: 30000 }, async () => {
  const f = await fixture();
  let child;
  try {
    for (const relative of ['one/project', 'two/project']) {
      const directory = join(f.workspace, relative);
      await mkdir(directory, { recursive: true });
      await writeFile(join(directory, 'main.cpp'), 'int answer() { return 42; }\n');
      const database = relative.startsWith('two/') ? join(directory, 'out/release') : directory;
      await mkdir(database, { recursive: true });
      if (database !== directory) await writeFile(join(directory, '.clangd'), JSON.stringify({
        If: { PathMatch: '.*\\.cpp' }, CompileFlags: { CompilationDatabase: 'out/release' },
      }));
      await writeFile(join(database, 'compile_commands.json'), JSON.stringify([
        { directory, file: 'main.cpp', arguments: ['clang++', '-std=c++20', '-c', 'main.cpp'] },
      ]));
    }
    const prepared = await f.boot();
    assert.equal(prepared.code, 0, prepared.stderr);
    const { env } = JSON.parse(prepared.stdout);
    child = spawn(process.env.SERENA_TEST_CLANGD, ['--background-index'], { cwd: f.workspace, env, stdio: ['pipe', 'pipe', 'pipe'] });
    let errors = '';
    child.stderr.on('data', part => { errors += part; });
    const send = message => {
      const body = JSON.stringify({ jsonrpc: '2.0', ...message });
      child.stdin.write(`Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`);
    };
    const initialized = new Promise((resolveInit, reject) => {
      let buffer = Buffer.alloc(0);
      const timer = setTimeout(() => reject(Error('clangd initialization timed out')), 10000);
      child.once('error', error => { clearTimeout(timer); reject(error); });
      child.stdout.on('data', part => {
        buffer = Buffer.concat([buffer, part]);
        for (;;) {
          const end = buffer.indexOf('\r\n\r\n');
          if (end < 0) return;
          const length = Number(/Content-Length: (\d+)/i.exec(buffer.subarray(0, end).toString())[1]);
          if (buffer.length < end + 4 + length) return;
          const message = JSON.parse(buffer.subarray(end + 4, end + 4 + length));
          buffer = buffer.subarray(end + 4 + length);
          if (message.id === 1 && !message.method) { clearTimeout(timer); resolveInit(); }
        }
      });
    });
    send({ id: 1, method: 'initialize', params: { processId: process.pid, rootUri: pathToFileURL(f.workspace).href, capabilities: {} } });
    await initialized;
    send({ method: 'initialized', params: {} });
    for (const relative of ['one/project', 'two/project']) {
      send({ method: 'textDocument/didOpen', params: { textDocument: { uri: pathToFileURL(join(f.workspace, relative, 'main.cpp')).href, languageId: 'cpp', version: 1, text: 'int answer() { return 42; }\n' } } });
    }
    const managed = join(f.workspace, '.gestalt/serena/clangd');
    const deadline = Date.now() + 15000;
    let shards = [];
    while (Date.now() < deadline) {
      shards = (await readdir(managed, { recursive: true })).filter(path => path.endsWith('.idx') && path.includes('/cache/'));
      if (shards.length >= 2) break;
      await new Promise(resolveWait => setTimeout(resolveWait, 100));
    }
    assert.equal(shards.length, 2, errors.slice(-3000));
    assert.equal(new Set(shards.map(path => path.split('/')[0])).size, 2);
    for (const relative of ['one/project', 'two/project']) assert.equal((await readdir(join(f.workspace, relative))).includes('.cache'), false);
  } finally {
    if (child && child.exitCode === null && child.signalCode === null) {
      child.kill();
      await new Promise(resolveExit => child.once('exit', resolveExit));
    }
    await f.close();
  }
});

test('public MCP rejects caller context/project/mode and missing/non-directory roots', async () => {
  const f = await fixture();
  try {
    Object.assign(f.env, { CODEX_HOME: join(f.root, 'codex'), GESTALT_HOME: join(f.root, 'managed') });
    for (const args of [[], ['--cwd', f.workspace, '--context', 'ide'], ['--cwd', f.workspace, '--mode', 'planning'],
      ['--cwd', f.workspace, '--project', '/other'], ['--cwd', f.workspace, '--cwd', f.workspace], ['--cwd', '/missing-serena-root']]) {
      const result = await run('bash', [manager, 'serena', 'mcp', ...args], f.env);
      assert.notEqual(result.code, 0); assert.match(result.stderr, /usage:|existing directory/);
      assert.equal(result.stdout, '');
    }
    assert.deepEqual(await readdir(f.home), []);
  } finally { await f.close(); }
});

test('updates retain exact validated interpreter links in uv venvs; other escaping links still fail', async () => {
  const f = await fixture();
  try {
    const state = join(f.workspace, '.gestalt/serena'), bin = join(state, 'uv-cache/archive-v0/retained/bin');
    const previous = join(f.root, 'retained-python');
    await writeFile(previous, 'immutable interpreter');
    await mkdir(bin, { recursive:true }); await symlink(previous,join(bin,'python'));
    let result = await f.boot(); assert.notEqual(result.code,0);
    f.settings.pythonExecutables = [previous];
    result = await f.boot(); assert.equal(result.code,0,result.stderr);
    await symlink(previous,join(state,'memory-link'));
    result = await f.boot(); assert.notEqual(result.code,0);assert.match(result.stderr,/escaping symlink/);
    assert.equal(await readFile(previous,'utf8'),'immutable interpreter');
  } finally { await f.close(); }
});


test('managed MCP carries retained interpreter targets into actual workspace preparation', async () => {
  const f = await fixture();
  let child, lines;
  try {
    const state=join(f.workspace,'.gestalt/serena'), bin=join(state,'uv-cache/archive-v0/prior/bin');
    const previous=join(f.root,'previous-python'), managed=join(f.root,'managed'), commands=join(f.root,'commands');
    await mkdir(bin,{recursive:true});await mkdir(join(managed,'serena'),{recursive:true});await mkdir(commands);
    await writeFile(previous,'retained immutable interpreter');await symlink(previous,join(bin,'python'));
    const catalog=['get_symbols_overview','find_symbol','initial_instructions','replace_symbol_body'].map(name=>({name,inputSchema:{type:'object'}}));
    const native=join(f.root,'mcp-native');
    await writeFile(native,`#!/usr/bin/env python3
import sys,json
for line in sys.stdin:
 message=json.loads(line)
 if 'id' not in message:continue
 if message['method']=='tools/list':result={'tools':${JSON.stringify(catalog)}}
 elif message['method']=='initialize':result={}
 else:result={'content':[{'type':'text','text':'prepared retained cache'}]}
 print(json.dumps({'id':message['id'],'result':result}),flush=True)
`,{mode:0o755});
    // The OS boundary is covered by native tests. This stand-in executes the exact
    // manager preparation arguments to isolate descriptor propagation regression.
    await writeFile(join(commands,'codex'),`#!/usr/bin/env python3
import sys,os
args=sys.argv[sys.argv.index('--')+1:]
os.execv(args[0],args)
`,{mode:0o755});
    const descriptor={schemaVersion:1,contractVersion:1,version:'1.7.0',python:'/usr/bin/python3',
      executable:native,uv:f.settings.uv,pythonInstallDir:f.settings.pythonInstallDir,pythonExecutables:[previous],tools:catalog};
    await writeFile(join(managed,'serena/active.json'),JSON.stringify(descriptor));
    const env={...f.env,PATH:commands+':'+process.env.PATH,CODEX_HOME:join(f.root,'codex'),GESTALT_HOME:managed};
    const policy={permissionProfile:{type:'managed',file_system:{type:'restricted',entries:[
      {path:{type:'special',value:{kind:'root'}},access:'read'},
      {path:{type:'path',path:f.workspace},access:'write'}]},network:'enabled'},
      sandboxCwd:new URL('file://'+f.workspace).href,useLegacyLandlock:false};
    child=spawn('bash',[manager,'serena','mcp','--cwd',f.workspace],{env,stdio:['pipe','pipe','pipe']});
    let stderr='';child.stderr.on('data',part=>stderr+=part);
    lines=createInterface({input:child.stdout});
    const response=new Promise((resolveResponse,reject)=>{
      const timer=setTimeout(()=>reject(Error('MCP preparation deadline: '+stderr)),10000);
      lines.on('line',line=>{const message=JSON.parse(line);if(message.id===1){clearTimeout(timer);resolveResponse(message);}});
    });
    child.stdin.write(JSON.stringify({jsonrpc:'2.0',id:1,method:'tools/call',params:{name:'find_symbol',arguments:{},
      _meta:{'codex/sandbox-state-meta':policy}}})+'\n');
    const message=await response;assert.ok(!message.error,JSON.stringify(message.error)+' '+stderr);
    assert.equal(message.result.content[0].text,'prepared retained cache');
    assert.equal(await readFile(previous,'utf8'),'retained immutable interpreter');
  } finally {
    if(child){child.stdin.end();if(child.exitCode===null)await new Promise(r=>child.once('exit',r));}
    lines?.close();await f.close();
  }
});

test('managed clangd preserves conditional project database choices and user configuration', async () => {
  const f = await fixture();
  try {
    const state = join(f.workspace, '.gestalt/serena');
    const selected = join(f.workspace, 'out/release');
    await mkdir(selected, { recursive: true });
    const commands = [{ directory: f.workspace, file: 'main.cpp', arguments: ['clang++', '-DRELEASE', 'main.cpp'] }];
    await writeFile(join(selected, 'compile_commands.json'), JSON.stringify(commands));
    await writeFile(join(f.workspace, '.clangd'), JSON.stringify({ If: { PathMatch: ['src/.*'], PathExclude: 'src/vendor/.*' }, CompileFlags: { CompilationDatabase: 'out/release' } }) + '\n---\n' + JSON.stringify({ If: { PathMatch: 'disabled/.*' }, CompileFlags: { CompilationDatabase: 'None' } }));
    const configPath = join(state, 'xdg-config/clangd/config.yaml');
    await mkdir(resolve(configPath, '..'), { recursive: true });
    const manual = JSON.stringify({ Diagnostics: { UnusedIncludes: 'Strict' } });
    await writeFile(configPath, manual);
    const result = await f.boot(); assert.equal(result.code, 0, result.stderr);
    const config = await readFile(configPath, 'utf8');
    const fragments = config.split('\n').filter(line => line.startsWith('{')).map(line => JSON.parse(line));
    const release = fragments.find(fragment => Array.isArray(fragment.If?.PathMatch));
    assert.ok(release.If.PathMatch[0].includes('/(src/.*)'));
    assert.ok(release.If.PathExclude[0].includes('/(src/vendor/.*)'));
    assert.deepEqual(JSON.parse(await readFile(join(release.CompileFlags.CompilationDatabase, 'compile_commands.json'))), commands);
    assert.ok(fragments.some(fragment => fragment.CompileFlags?.CompilationDatabase === 'None'));
    assert.deepEqual(fragments.at(-1), JSON.parse(manual));
    assert.equal((await f.boot()).code, 0);
    assert.equal(await readFile(configPath, 'utf8'), config);
  } finally { await f.close(); }
});

test('an incomplete compilation database does not prevent Serena startup', async () => {
  const f = await fixture();
  try {
    await writeFile(join(f.workspace, 'compile_commands.json'), '{unfinished');
    const result = await f.boot();
    assert.equal(result.code, 0, result.stderr);
    assert.match(result.stderr, /skipping clangd database/);
  } finally { await f.close(); }
});
