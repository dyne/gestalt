import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { createInterface } from 'node:readline';
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

const source = await readFile(resolve('public/gestalt'), 'utf8');
const serializedHookCount = input => JSON.stringify(input.input).split('UNRELATED_PROFILE_HOOK').length - 1;
const program = source.split('launch_cli_with_xerj() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];

function execute(command, args, env, input, cwd) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { env, cwd, stdio: ['pipe', 'pipe', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.on('data', data => stdout += data);
    child.stderr.on('data', data => stderr += data);
    child.once('error', reject);
    const timer = setTimeout(() => { child.kill(); reject(new Error('native Codex fixture exceeded deadline')); }, 35000);
    child.once('exit', code => { clearTimeout(timer); resolve({ code, stdout, stderr }); });
    child.stdio[3].end();
    child.stdin.end(input);
  });
}

async function nativeHookHash(command, env, cwd) {
  const child = spawn('codex', ['app-server', '--stdio', '-c',
    `hooks.SessionStart=[{hooks=[{type="command",timeout=2,command=${JSON.stringify(command)}}]}]`],
    { env, cwd, stdio: ['pipe', 'pipe', 'ignore'] });
  const pending = new Map(); let id = 0;
  const lines = createInterface({ input: child.stdout });
  lines.on('line', line => {
    const message = JSON.parse(line), waiter = pending.get(message.id);
    if (waiter) { pending.delete(message.id); message.error ? waiter.reject(new Error('hook metadata unavailable')) : waiter.resolve(message.result); }
  });
  const request = (method, params) => new Promise((resolve, reject) => {
    const next = ++id; pending.set(next, { resolve, reject });
    child.stdin.write(JSON.stringify({ id: next, method, params }) + '\n');
  });
  const timer = setTimeout(() => { for (const waiter of pending.values()) waiter.reject(new Error('hook metadata deadline')); child.kill(); }, 5000);
  try {
    await request('initialize', { clientInfo: { name: 'profile-hook-fixture', version: '1' }, capabilities: {} });
    child.stdin.write(JSON.stringify({ method: 'initialized', params: {} }) + '\n');
    const listing = await request('hooks/list', { cwds: [cwd] });
    return listing.data[0].hooks.find(hook => hook.command === command).currentHash;
  } finally {
    clearTimeout(timer); lines.close(); child.stdin.end(); child.kill();
    if (child.exitCode == null && child.signalCode == null) await new Promise(resolve => child.once('exit', resolve));
  }
}

for (const mode of ['healthy-child', 'healthy-resume', 'failed-connection', 'absent', 'healthy-profile', 'profile-excluded', 'profile-project-excluded', 'profile-auth-conflict']) test(`native first-turn ${mode}`, { timeout: 60000 }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'xerj-native-cli-'));
  const home = join(root, 'codex home');
  const repository = join(root, 'marketplace');
  const plugin = join(repository, 'plugins/gestalt');
  const workspace = join(root, 'workspace');
  const owner = join(root, 'manager');
  const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith('CONTEXT_MODE_')));
  Object.assign(env, { CODEX_HOME: home, GESTALT_HOME: join(root, 'managed'), GESTALT_MANAGER_BIN: owner,
    XERJ_READY_TIMEOUT_MS: '5000', NATIVE_MODE: mode, NATIVE_ROOT: root });
  for (const path of [home, workspace, join(plugin, '.codex-plugin'), join(plugin, 'skills/xerj'), join(repository, '.agents/plugins')])
    await mkdir(path, { recursive: true });
  await writeFile(join(plugin, '.codex-plugin/plugin.json'), JSON.stringify({ name: 'gestalt', version: '0.0.1',
    description: 'isolated native fixture', skills: './skills/' }));
  for (const name of ['profile-kept', 'profile-hidden', 'session-kept']) {
    await mkdir(join(plugin, 'skills', name), { recursive: true });
    await writeFile(join(plugin, 'skills', name, 'SKILL.md'), `---\nname: ${name}\ndescription: Preserve fixture selector.\n---\nFixture instructions.\n`);
  }
  await writeFile(join(plugin, 'skills/xerj/SKILL.md'), '---\nname: xerj\ndescription: Retrieve fixture source with verified xerj tools.\n---\nUse verified retrieval and confirm source\'s policy.\n');
  await writeFile(join(repository, '.agents/plugins/marketplace.json'), JSON.stringify({ name: 'dyne-gestalt-agents', plugins: [
    { name: 'gestalt', source: { source: 'local', path: './plugins/gestalt' } },
  ] }));
  // Native MCP process, controlled through the same manager argv as production.
  // No replacement provider transport or real model service is used.
  await writeFile(owner, `#!/usr/bin/env node
const fs = require('node:fs');
const root = ${JSON.stringify(root)};
const mode = ${JSON.stringify(mode)};
const args = process.argv.slice(2);
if (args[1] === 'ensure-ready') {
  console.log(JSON.stringify(mode === 'absent' ? { schemaVersion: 1, status: 'absent' }
    : { schemaVersion: 1, status: 'ready', version: '1.0.0-rc.87', endpoint: 'http://127.0.0.1:9300' }));
} else {
  fs.appendFileSync(root + '/proxies', 'start\\n');
  fs.appendFileSync(root + '/proxy-pids', String(process.pid) + '\\n');
  if (mode === 'failed-connection') process.exit(1);
  require('node:readline').createInterface({ input: process.stdin }).on('line', line => {
    const message = JSON.parse(line);
    if (message.id === undefined) return;
    let result = {};
    if (message.method === 'initialize') result = { protocolVersion: message.params.protocolVersion,
      capabilities: { tools: {} }, serverInfo: { name: 'xerj-mcp', version: '1.0.0-rc.87' } };
    if (message.method === 'tools/list') result = { tools: [{ name: 'xerj_search', description: 'fixture retrieval',
      inputSchema: { type: 'object', properties: { query: { type: 'string' } } } }] };
    if (message.method === 'tools/call') result = { content: [{ type: 'text', text: 'verified fixture source' }], isError: false };
    console.log(JSON.stringify({ jsonrpc: '2.0', id: message.id, result }));
  }).on('close', () => fs.appendFileSync(root + '/proxies', 'eof\\n'));
}
`, { mode: 0o755 });
  const requests = [];
  const healthy = mode.startsWith('healthy');
  let expectedReady = healthy;
  const fixtureErrors = [];
  let queried = false;
  let childQueried = false;
  let spawned = false;
  let waited = false;
  const model = createServer(async (request, response) => {
    let body = '';
    for await (const data of request) body += data;
    const input = JSON.parse(body);
    requests.push(input);
    const flattened = input.tools.flatMap(tool => tool.type === 'namespace'
      ? tool.tools.map(child => ({ ...child, namespace: tool.name })) : [tool]);
    const isChild = JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('CHILD_FIXTURE');
    let item;
    const spawnTool = flattened.find(tool => tool.name === 'spawn_agent');
    if (healthy && (isChild ? !childQueried : !queried) && flattened.some(tool => tool.name?.includes('xerj_search'))) {
      const tool = flattened.find(tool => tool.name?.includes('xerj_search'));
      assert.ok(tool, 'configured live MCP retrieval tool missing');
      if (isChild) childQueried = true; else queried = true;
      item = { id: 'fc_search', type: 'function_call', status: 'completed', name: tool.name,
        ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'call_search',
        arguments: JSON.stringify({ query: 'fixture policy' }) };
    } else if (['healthy-child', 'healthy-profile'].includes(mode) && !isChild && !spawned && spawnTool) {
      spawned = true;
      item = { id: 'fc_1', type: 'function_call', status: 'completed', name: spawnTool.name,
        ...(spawnTool.namespace ? { namespace: spawnTool.namespace } : {}), call_id: 'call_1',
        arguments: JSON.stringify({ message: 'CHILD_FIXTURE: report retrieval capability.', fork_context: false }) };
    } else if (['healthy-child', 'healthy-profile'].includes(mode) && !isChild && !waited) {
      const waitTool = flattened.find(tool => tool.name === 'wait_agent');
      const agent = input.input.filter(item => item.type === 'function_call_output').map(item => { try { return JSON.parse(item.output).agent_id; } catch { return undefined; } }).find(Boolean);
      assert.ok(waitTool && agent, 'native child spawn result missing');
      waited = true;
      item = { id: 'fc_wait', type: 'function_call', status: 'completed', name: waitTool.name,
        ...(waitTool.namespace ? { namespace: waitTool.namespace } : {}), call_id: 'call_wait',
        arguments: JSON.stringify({ targets: [agent], timeout_ms: 10000 }) };
    } else {
      item = { id: `msg_${requests.length}`, type: 'message', status: 'completed', role: 'assistant',
        content: [{ type: 'output_text', text: 'Fixture finished.', annotations: [] }] };
    }
    const catalogs = input.input.filter(item => item.role === 'developer' && JSON.stringify(item.content).includes('<skills_instructions>'));
    const catalog = JSON.stringify(catalogs.at(-1));
    const hasSkill = catalog.includes('gestalt:xerj');
    if (mode === 'healthy-profile' && !isChild && serializedHookCount(input) !== 1) fixtureErrors.push('profile hook changed or duplicated');
    if (mode.includes('profile')) {
      if (!catalog.includes('gestalt:profile-kept')) fixtureErrors.push('profile selector overridden');
      if (!catalog.includes('gestalt:session-kept')) fixtureErrors.push('session selector overridden');
      if (catalog.includes('gestalt:profile-hidden')) fixtureErrors.push('disabled profile skill exposed');
    }
    const capabilityMessages = input.input.filter(item => /gestalt_xerj_(capability|unavailable)/.test(JSON.stringify(item.content)));
    const hasGuidance = JSON.stringify(capabilityMessages.at(-1) ?? null).includes('<gestalt_xerj_capability>');
    const hasTool = flattened.some(tool => tool.name?.includes('xerj_search'));
    if (hasSkill !== expectedReady) fixtureErrors.push(`skill ${hasSkill}`);
    if (hasTool !== expectedReady) fixtureErrors.push(`tool ${hasTool}`);
    if (hasGuidance !== expectedReady) fixtureErrors.push(`guidance ${hasGuidance}`);
    const result = { id: `resp_${requests.length}`, object: 'response', created_at: 1, status: 'completed',
      model: 'fixture', output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
    response.writeHead(200, { 'Content-Type': 'text/event-stream' });
    for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
      { type: 'response.output_item.added', output_index: 0, item },
      { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response: result }])
      response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
    response.end();
  });
  await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
  try {
    for (const args of [['plugin', 'marketplace', 'add', repository], ['plugin', 'add', 'gestalt@dyne-gestalt-agents']]) {
      const result = await execute('codex', args, env, undefined, workspace);
      assert.equal(result.code, 0, result.stderr.slice(-800));
    }
    const skillPath = join(home, 'plugins/cache/dyne-gestalt-agents/gestalt/0.0.1/skills/xerj/SKILL.md');
    const alias = mode === 'healthy-profile' ? `\n[mcp_servers.\"old.xerj proxy\"]\ncommand=${JSON.stringify(owner)}\nargs=[\"xerj\",\"mcp\"]\n` : '';
    const profileAuth = mode === 'profile-auth-conflict' ? `\n[mcp_servers.gestalt-xerj]\ncommand=${JSON.stringify(owner)}\nargs=[\"xerj\",\"mcp\"]\n[mcp_servers.gestalt-xerj.env]\nXERJ_AUTH=\"do-not-expose-secret\"\n` : '';
    let profile = mode.includes('profile') ? `[features]\nhooks=${mode !== 'profile-excluded'}\n[projects.${JSON.stringify(workspace)}]\ntrust_level=\"trusted\"\n[[skills.config]]\nname=\"gestalt:profile-kept\"\nenabled=true\n[[skills.config]]\nname=\"gestalt:profile-hidden\"\nenabled=false\n${profileAuth}` : '';
    const config = `model_provider="fixture"\nmodel="fixture"\napproval_policy="never"\n[model_providers.fixture]\nname="fixture"\nbase_url="http://127.0.0.1:${model.address().port}/v1"\nwire_api="responses"\nrequires_openai_auth=false\nrequest_max_retries=0\n[features]\ntool_search=false\nhooks=true\nmulti_agent=true\n[[skills.config]]\npath=${JSON.stringify(skillPath)}\nenabled=${!healthy}\n[[skills.config]]\nname=\"gestalt:profile-kept\"\nenabled=false\n[[skills.config]]\nname=\"gestalt:session-kept\"\nenabled=false\n[plugins."gestalt@dyne-gestalt-agents"]\nenabled=true\n${alias}`;
    await writeFile(join(home, 'config.toml'), config);
    if (mode === 'healthy-profile') {
      const command = JSON.stringify(process.execPath) + ' -e ' + "'process.stdout.write(\"UNRELATED_PROFILE_HOOK\")'";
      const hash = await nativeHookHash(command, env, workspace);
      const key = join(home, 'fixture-profile.config.toml') + ':session_start:0:0';
      profile += `\n[[hooks.SessionStart]]\n[[hooks.SessionStart.hooks]]\ntype="command"\ntimeout=2\ncommand=${JSON.stringify(command)}\n[hooks.state.${JSON.stringify(key)}]\ntrusted_hash=${JSON.stringify(hash)}\n`;
    }
    if (profile) await writeFile(join(home, 'fixture-profile.config.toml'), profile);
    if (mode === 'profile-project-excluded') {
      await mkdir(join(workspace, '.codex'));
      await writeFile(join(workspace, '.codex/config.toml'), '[features]\nhooks=false\n');
    }
    const args = [...(profile ? ['-pfixture-profile', '-c', 'skills.config=[{name="gestalt:session-kept",enabled=true}]'] : []), 'exec', '--skip-git-repo-check', '--json', '-C', workspace, 'ROOT_FIXTURE: report retrieval capability.'];
    const result = await execute(process.execPath, ['--input-type=module', '-', ...args], env, program, workspace);
    await writeFile(join('/tmp', `xerj-cli-native-${mode}.json`), JSON.stringify({ result, requests, fixtureErrors }, null, 2));
    assert.equal(result.code, 0, result.stderr.slice(-1200));
    assert.ok(!result.stderr.includes('do-not-expose-secret'));
    assert.ok(!result.stdout.includes('do-not-expose-secret'));
    assert.ok(requests.length > 0, 'no model request observed');
    assert.deepEqual(fixtureErrors, []);
    assert.equal(await readFile(join(home, 'config.toml'), 'utf8'), config, 'persistent config changed');
    if (profile) assert.equal(await readFile(join(home, 'fixture-profile.config.toml'), 'utf8'), profile);
    if (healthy) {
      if (['healthy-child', 'healthy-profile'].includes(mode)) assert.ok(spawned, 'native spawn_agent was not callable');
      assert.ok(requests.some(input => input.input.some(item => item.type === 'function_call_output' &&
        JSON.stringify(item.output).includes('verified fixture source'))), 'actual MCP tool call not observed');
      if (['healthy-child', 'healthy-profile'].includes(mode)) assert.ok(requests.some(input => JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('CHILD_FIXTURE')), 'child model turn not observed');
      if (['healthy-child', 'healthy-profile'].includes(mode)) assert.ok(requests.some(input =>
        JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('CHILD_FIXTURE') &&
        input.input.some(item => item.type === 'function_call_output' && JSON.stringify(item.output).includes('verified fixture source'))), 'child retrieval result not observed');
      const proxies = await readFile(join(root, 'proxies'), 'utf8');
      const clients = ['healthy-child', 'healthy-profile'].includes(mode) ? 2 : 1;
      assert.equal(proxies.split('start').length - 1, clients, 'duplicate client within a native thread');
      const pids = (await readFile(join(root, 'proxy-pids'), 'utf8')).trim().split('\n').map(Number);
      assert.equal(pids.length, clients);
      await new Promise(resolve => setTimeout(resolve, 100));
      for (const pid of pids) assert.throws(() => process.kill(pid, 0), /ESRCH/, 'native runtime client was not reaped');
      if (mode === 'healthy-resume') {
        const priorCount = requests.length;
        expectedReady = false;
        const ownerCode = await readFile(owner, 'utf8');
        await writeFile(owner, ownerCode.replace('const mode = \"healthy-resume\";', 'const mode = \"absent\";'), { mode: 0o755 });
        const resumed = await execute(process.execPath, ['--input-type=module', '-', '-C', workspace, 'exec', 'resume', '--last', '--json', '--skip-git-repo-check', 'RESUME_FIXTURE: report retrieval capability.'], env, program, workspace);
        assert.equal(resumed.code, 0, resumed.stderr.slice(-1000));
        assert.ok(requests.length > priorCount, 'resume model request not observed');
        await writeFile(join('/tmp', 'xerj-cli-native-resume.json'), JSON.stringify({ resumed, requests, fixtureErrors }, null, 2));
        assert.deepEqual(fixtureErrors, []);
        assert.equal(await readFile(join(home, 'config.toml'), 'utf8'), config);
      }
    } else if (mode === 'failed-connection') {
      assert.match(result.stderr, /runtime-connection-failed/);
      assert.equal((await readFile(join(root, 'proxies'), 'utf8')).trim(), 'start');
    }
  } finally {
    model.closeAllConnections();
    await new Promise(resolve => model.close(resolve));
    // Native plugin refresh may finish a private Git clone just after exit.
    await rm(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  }
});
