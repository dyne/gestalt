import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { mkdtemp, mkdir, writeFile, readFile, cp, rm, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

// Opt-in installed-release smoke. The supplied JSON has the same installation
// descriptor as the manager; no installation or downloads happen in this test.
const descriptor = process.env.SERENA_REAL_INSTALL;
const manager = resolve('public/gestalt');
test('real released Serena edits symbols, persists memory and confines project/runtime state',
  { skip: !descriptor, timeout: 120000 }, async () => {
    const root = await mkdtemp(join(tmpdir(), 'serena-real-'));
    const workspace = join(root, 'workspace with spaces'), managed = join(root, 'managed'), home = join(root, 'home');
    const state = join(workspace, '.gestalt/serena');
    for (const path of [workspace, home, join(root, 'codex'), join(managed, 'serena'), join(workspace, '.serena'), join(root, 'sibling')]) await mkdir(path, { recursive: true });
    const legacy = JSON.stringify({ project_name: 'legacy', language_servers: ['python'], ignored_paths: ['kept/**'] });
    await writeFile(join(workspace, '.serena/project.yml'), legacy);
    await writeFile(join(workspace, 'example.py'), 'def greet(name):\n    return "Hello " + name\n');
    await writeFile(join(root, 'sibling/outside.py'), 'def outside():\n    return 1\n');
    await symlink(join(root, 'sibling'), join(workspace, 'escape'));
    await cp(descriptor, join(managed, 'serena/active.json'));
    if (process.env.SERENA_REAL_UV_CACHE) {
      await mkdir(state, { recursive: true });
      // Preserve native venv interpreter links so post-update caches exercise the
      // validated retained-interpreter contract instead of copying executables.
      await cp(process.env.SERENA_REAL_UV_CACHE, join(state, 'uv-cache'), { recursive: true, dereference: false, verbatimSymlinks: true });
    }
    const env = { ...process.env, HOME: home, GESTALT_HOME: managed, CODEX_HOME: join(root, 'codex'), UV_OFFLINE: '1' };
    const policy = { permissionProfile: { type: 'managed', file_system: { type: 'restricted', entries: [
      { path: { type: 'special', value: { kind: 'root' } }, access: 'read' },
      { path: { type: 'path', path: workspace }, access: 'write' },
    ] }, network: 'enabled' }, sandboxCwd: new URL('file://' + workspace).href, useLegacyLandlock: false };
    let instance;
    function start(boundWorkspace = workspace, boundPolicy = policy) {
      const child = spawn('bash', [manager, 'serena', 'mcp', '--cwd', boundWorkspace], { env, stdio: ['pipe', 'pipe', 'pipe'] });
      let stderr = ''; child.stderr.on('data', part => stderr += part);
      const lines = createInterface({ input: child.stdout });
      const pending = new Map(); let id = 0;
      lines.on('line', line => { const m = JSON.parse(line), w = pending.get(m.id); if (w) { pending.delete(m.id); w(m); } });
      const request = (method, params) => new Promise((resolveRequest, reject) => {
        const n = ++id, timer = setTimeout(() => reject(Error('real Serena deadline: ' + stderr.slice(-1500))), 60000);
        pending.set(n, message => { clearTimeout(timer); resolveRequest(message); });
        child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: n, method, params }) + '\n');
      });
      const call = async (name, args) => {
        const message = await request('tools/call', { name, arguments: args, _meta: { 'codex/sandbox-state-meta': boundPolicy } });
        assert.ok(!message.error, JSON.stringify(message.error) + '\n' + stderr.slice(-1500));
        assert.ok(!message.result.isError, JSON.stringify(message.result));
        return message.result;
      };
      const stop = async () => { child.stdin.end(); if (child.exitCode === null) await new Promise(r => child.once('exit', r)); lines.close(); };
      return { child, request, call, stop, stderr: () => stderr };
    }
    try {
      instance = start();
      await instance.request('initialize', { protocolVersion: '2024-11-05' });
      const listed = await instance.request('tools/list', {});
      assert.ok(listed.result.tools.some(t => t.name === 'replace_symbol_body'));
      assert.ok(!listed.result.tools.some(t => t.name === 'activate_project'));
      const overview = await instance.call('get_symbols_overview', { relative_path: 'example.py' });
      assert.match(JSON.stringify(overview), /greet/);
      await instance.call('replace_symbol_body', { name_path: 'greet', relative_path: 'example.py', body: 'def greet(name):\n    return "Managed " + name' });
      assert.match(await readFile(join(workspace, 'example.py'), 'utf8'), /Managed/);
      await instance.call('write_memory', { memory_name: 'persistent', content: 'project-local proof' });
      const symbols = await instance.call('find_symbol', { name_path_pattern: 'greet', relative_path: 'example.py', include_body: true });
      assert.match(JSON.stringify(symbols), /Managed/);
      // Same effective policy retains one backend and one language-service start.
      assert.equal(instance.stderr().split('Starting Serena server').length - 1, 1);
      const escaped = await instance.request('tools/call', { name: 'replace_symbol_body', arguments: {
        name_path: 'outside', relative_path: 'escape/outside.py', body: 'def outside():\n    return 9' }, _meta: { 'codex/sandbox-state-meta': policy } });
      assert.ok(escaped.error || escaped.result.isError || JSON.stringify(escaped.result).includes('Error'));
      assert.equal(await readFile(join(root, 'sibling/outside.py'), 'utf8'), 'def outside():\n    return 1\n');
      await instance.stop();
      instance = start();
      await instance.request('initialize', {});
      const memory = await instance.call('read_memory', { memory_name: 'persistent' });
      assert.match(JSON.stringify(memory), /project-local proof/);
      const yaml = await readFile(join(state, 'serena_config.yml'), 'utf8');
      assert.match(yaml, /project_serena_folder_location:.*\$projectDir\/\.gestalt\/serena/);
      assert.match(yaml, /compile_commands_dir:.*\.gestalt\/serena\/clangd/);
      const project = await readFile(join(state, 'project.yml'), 'utf8');
      assert.match(project, /python/); assert.match(project, /kept\/\*\*/); assert.match(project, /\.gestalt\/\*\*/);
      assert.equal(await readFile(join(workspace, '.serena/project.yml'), 'utf8'), legacy);
      await assert.rejects(readFile(join(home, '.serena/serena_config.yml')), { code: 'ENOENT' });
      // Outer home is read-only to the native process; state directories must be local.
      assert.ok((await readFile(join(state, 'memories/persistent.md'), 'utf8')).includes('project-local proof'));
      const readonly = structuredClone(policy);
      readonly.permissionProfile.file_system.entries[1].access = 'read';
      const denied = await instance.request('tools/call', { name: 'replace_symbol_body', arguments: {
        name_path: 'greet', relative_path: 'example.py', body: 'def greet(name):\n    return "denied"' },
        _meta: { 'codex/sandbox-state-meta': readonly } });
      assert.ok(denied.error, 'read-only runtime unexpectedly accepted an edit');
      assert.match(instance.stderr(), /workspace preparation failed/);
      assert.match(await readFile(join(workspace, 'example.py'), 'utf8'), /Managed/);
      await instance.stop();
      const unavailable = join(root, 'uncached language project');
      await mkdir(join(unavailable, '.serena'), { recursive: true });
      await writeFile(join(unavailable, '.serena/project.yml'), legacy);
      await writeFile(join(unavailable, 'example.py'), 'def greet():\n    return 1\n');
      const languagePolicy = structuredClone(policy);
      languagePolicy.permissionProfile.file_system.entries[1].path.path = unavailable;
      languagePolicy.sandboxCwd = new URL('file://' + unavailable).href;
      instance = start(unavailable, languagePolicy);
      assert.ok((await instance.request('initialize', {})).result);
      const failedLanguage = await instance.request('tools/call', { name: 'get_symbols_overview',
        arguments: { relative_path: 'example.py' }, _meta: { 'codex/sandbox-state-meta': languagePolicy } });
      assert.ok(failedLanguage.error || failedLanguage.result.isError || /Error|failed|unavailable/i.test(JSON.stringify(failedLanguage.result)),
        'an MCP handshake must not imply a working language service');
      assert.equal(await readFile(join(unavailable, 'example.py'), 'utf8'), 'def greet():\n    return 1\n');
    } finally {
      await instance?.stop();
      await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
    }
  });
