import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, mkdir, writeFile, readFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { Duplex } from 'node:stream';
import { createRequire } from 'node:module';
import test from 'node:test';

// Reuse the repository's existing Playwright-bundled ws implementation. This
// imports transport code only; it launches no browser and installs nothing.
const { ws: WebSocket } = createRequire(import.meta.url)('playwright-core/lib/utilsBundle');

// Evidence-only protocol investigation. Run only through the configured host
// test runner. Every runtime, socket, config and MCP process is fixture-owned.
const codex = resolve('node_modules/.bin/codex');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

test('isolated native Unix controller: shared identity, resume overlays and dynamic request routing',
  { timeout: 60000 }, async t => {
  const root = await mkdtemp(join(tmpdir(), 'playwright-control-probe-'));
  const home = join(root, 'home'), workspace = join(root, 'workspace');
  const socket = join(root, 'control.sock');
  const clients = [], children = [], observations = { dynamicRoutes: [] };
  const transport = { phase: 'setup', serverStderr: '', serverExit: null, proxies: {} };
  let serverGroup;
  let modelCalls = 0;
  const model = createServer(async (req, res) => {
    try {
      let raw = ''; for await (const part of req) raw += part;
      const input = JSON.parse(raw);
      const tools = input.tools.flatMap(tool => tool.type === 'namespace'
        ? tool.tools.map(child => ({ ...child, namespace: tool.name })) : [tool]);
      const tool = tools.find(tool => tool.name === 'managed_playwright_status');
      const item = modelCalls++ === 0 && tool
        ? { id: 'fc_probe', type: 'function_call', status: 'completed', name: tool.name,
          ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'call_probe', arguments: '{}' }
        : { id: 'msg_probe', type: 'message', status: 'completed', role: 'assistant',
          content: [{ type: 'output_text', text: 'Probe finished.', annotations: [] }] };
      const response = { id: `resp_${modelCalls}`, object: 'response', created_at: 1,
        status: 'completed', model: 'fixture', output: [item],
        usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      for (const event of [
        { type: 'response.created', response: { ...response, status: 'in_progress', output: [] } },
        { type: 'response.output_item.added', output_index: 0, item },
        { type: 'response.output_item.done', output_index: 0, item },
        { type: 'response.completed', response },
      ]) res.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
      res.end();
    } catch (error) { res.writeHead(500); res.end(String(error)); }
  });
  const own = child => { children.push(child); return child; };
  const connect = async name => {
    const child = own(spawn(codex, ['app-server', 'proxy', '--sock', socket],
      { cwd: workspace, env, stdio: ['pipe', 'pipe', 'pipe'] }));
    const wire = transport.proxies[name] = { pid: child.pid, sentBytes: 0,
      receivedBytes: 0, prefixHex: '', lines: 0, stderr: '', exit: null };
    let stderr = ''; child.stderr.on('data', chunk => stderr = (stderr + chunk).slice(-4000));
    child.stderr.on('data', chunk => wire.stderr = (wire.stderr + chunk).slice(-1200));
    child.stdout.on('data', chunk => {
      wire.receivedBytes += chunk.length;
      wire.prefixHex = (wire.prefixHex + chunk.toString('hex')).slice(0, 128);
    });
    child.on('exit', (code, signal) => wire.exit = { code, signal });
    let next = 0; const pending = new Map(), events = [];
    // `proxy` forwards bytes, while native Unix app-server endpoints require
    // standard HTTP Upgrade followed by WebSocket text frames (not JSONL).
    // Keep the proxy process and run its byte stream through the existing ws
    // client so this exercises the documented host control transport exactly.
    transport.phase = `${name}:websocket-upgrade`;
    const connection = new WebSocket('ws://localhost/', {
      createConnection: () => Duplex.from({ readable: child.stdout, writable: child.stdin }),
      handshakeTimeout: 10000, perMessageDeflate: false,
    });
    const fail = error => { for (const p of pending.values()) p.reject(error); pending.clear(); };
    child.on('error', fail); child.on('exit', () => fail(new Error(`${name} proxy exit: ${stderr}`)));
    child.stdin.on('error', fail);
    connection.on('error', fail);
    connection.on('close', () => fail(new Error(`${name}: websocket closed`)));
    connection.on('message', data => {
      wire.lines++;
      let message; try { message = JSON.parse(data.toString()); } catch { return; }
      if (message.method === 'item/tool/call') {
        observations.dynamicRoutes.push({ client: name, threadId: message.params.threadId,
          turnId: message.params.turnId, tool: message.params.tool });
        connection.send(JSON.stringify({ id: message.id, result: {
          contentItems: [{ type: 'inputText', text: 'inactive fixture capability' }], success: true,
        } }));
      } else if (message.id !== undefined && pending.has(message.id)) {
        const p = pending.get(message.id); pending.delete(message.id);
        message.error ? p.reject(new Error(JSON.stringify(message.error))) : p.resolve(message.result);
      } else events.push(message);
    });
    const request = (method, params) => {
      const id = ++next;
      transport.phase = `${name}:${method}`;
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          pending.delete(id);
          reject(new Error(`${name}: ${method} deadline; transport=${JSON.stringify(transport)}`));
        }, 10000);
        pending.set(id, { resolve: value => { clearTimeout(timer); resolve(value); },
          reject: error => { clearTimeout(timer); reject(error); } });
        const data = JSON.stringify({ id, method, params });
        wire.sentBytes += Buffer.byteLength(data);
        connection.send(data);
      });
    };
    const client = { request, events, close: () => { fail(new Error('probe cleanup')); connection.terminate(); child.stdin.end(); } };
    clients.push(client);
    await new Promise((resolve, reject) => {
      connection.once('open', resolve);
      connection.once('error', reject);
    });
    await request('initialize', { clientInfo: { name, version: '1' }, capabilities: { experimentalApi: true } });
    connection.send(JSON.stringify({ method: 'initialized', params: {} }));
    return client;
  };
  const env = { ...process.env, CODEX_HOME: home };
  // Do not leak ambient context-mode runtime wiring into this isolated client.
  for (const key of Object.keys(env)) if (key.startsWith('CONTEXT_MODE_')) delete env[key];
  const stopRuntime = async () => {
    for (const client of clients.splice(0)) client.close();
    const signalGroup = signal => {
      if (serverGroup) try { process.kill(-serverGroup, signal); } catch (error) {
        if (error.code !== 'ESRCH') throw error;
      }
    };
    signalGroup('SIGTERM');
    for (const child of children) if (child.exitCode === null && child.signalCode === null)
      try { child.kill('SIGTERM'); } catch {}
    const escalator = setTimeout(() => {
      signalGroup('SIGKILL');
      for (const child of children) if (child.exitCode === null && child.signalCode === null)
        try { child.kill('SIGKILL'); } catch {}
    }, 2000);
    await Promise.all(children.map(child => new Promise(resolve => {
      if (child.exitCode !== null || child.signalCode !== null) return resolve();
      child.once('exit', resolve);
    })));
    clearTimeout(escalator);
    signalGroup('SIGKILL');
    serverGroup = undefined;
    children.length = 0;
  };
  try {
    await mkdir(home, { mode: 0o700 }); await mkdir(workspace);
    await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
    const config = `model_provider="fixture"\nmodel="fixture"\napproval_policy="on-request"\nsandbox_mode="read-only"\n[model_providers.fixture]\nname="fixture"\nbase_url="http://127.0.0.1:${model.address().port}/v1"\nwire_api="responses"\nrequires_openai_auth=false\nrequest_max_retries=0\n[features]\ntool_search=false\n`;
    await writeFile(join(home, 'config.toml'), config, { mode: 0o600 });
    const fixture = join(root, 'mcp.cjs');
    await writeFile(fixture, `require('node:readline').createInterface({input:process.stdin}).on('line',line=>{
      const m=JSON.parse(line);if(m.id===undefined)return;
      let result={};if(m.method==='initialize')result={protocolVersion:m.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'probe',version:'1'}};
      if(m.method==='tools/list')result={tools:[{name:'probe_read',description:'Read fixture',inputSchema:{type:'object',properties:{}}}]};
      console.log(JSON.stringify({jsonrpc:'2.0',id:m.id,result}));
    });`);
    const startRuntime = async () => {
    const server = own(spawn(codex, ['app-server', '--listen', `unix://${socket}`],
      { cwd: workspace, env, detached: true, stdio: ['ignore', 'ignore', 'pipe'] }));
    serverGroup = server.pid;
    let stderr = ''; server.stderr.on('data', chunk => stderr = (stderr + chunk).slice(-4000));
    server.stderr.on('data', chunk => transport.serverStderr = (transport.serverStderr + chunk).slice(-1800));
    server.on('exit', (code, signal) => transport.serverExit = { code, signal });
    transport.phase = 'waiting-for-private-socket';
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      try { if ((await stat(socket)).isSocket()) break; } catch {}
      if (server.exitCode !== null) throw new Error(`server startup: ${stderr}`);
      await pause(40);
    }
    assert.ok((await stat(socket)).isSocket(), 'private socket not created');
    };
    await startRuntime();
    let a = await connect('probe-owner');
    const mcp = { command: process.execPath, args: [fixture], enabled: false, required: true };
    const started = await a.request('thread/start', { cwd: workspace, ephemeral: false,
      config: { mcp_servers: { probe: mcp } }, dynamicTools: [{ type: 'function', name: 'managed_playwright_status',
        description: 'Read isolated capability status', inputSchema: { type: 'object', properties: {}, additionalProperties: false } }] });
    const id = started.thread.id; observations.rootThreadId = id;
    let b = await connect('probe-observer');
    const seen = await b.request('thread/read', { threadId: id, includeTurns: false });
    assert.equal(seen.thread.id, id); observations.source = seen.thread.source;
    const initial = await b.request('mcpServerStatus/list', { threadId: id, serverName: 'probe' });
    observations.initialTools = initial.data?.flatMap(s => Object.keys(s.tools ?? {})) ?? [];
    assert.equal(observations.initialTools.length, 0, 'disabled fixture unexpectedly connected');
    const runTurn = async () => {
      const result = await a.request('turn/start', { threadId: id,
        input: [{ type: 'text', text: 'Report fixture capability status.' }] });
      const completed = () => [...a.events, ...b.events].find(e =>
        e.method === 'turn/completed' && e.params.turn.id === result.turn.id);
      const deadline = Date.now() + 20000;
      while (Date.now() < deadline && !completed()) await pause(40);
      assert.ok(completed(), 'fixture turn did not complete');
      assert.equal(completed().params.turn.status, 'completed', 'fixture turn failed');
      return result.turn.id;
    };
    // Fresh persistent threads are saved lazily. Complete a harmless local
    // fixture turn before asking another connection to resume its rollout.
    observations.initialTurnId = await runTurn();
    assert.ok(modelCalls > 0, 'local fixture model was not called');
    observations.initialDynamicRoutes = [...observations.dynamicRoutes];
    const resumed = await b.request('thread/resume', { threadId: id,
      config: { mcp_servers: { probe: { ...mcp, enabled: true } } } });
    assert.equal(resumed.thread.id, id);
    const after = await b.request('mcpServerStatus/list', { threadId: id, serverName: 'probe' });
    observations.afterLoadedResumeTools = after.data?.flatMap(s => Object.keys(s.tools ?? {})) ?? [];
    observations.loadedResumeConfigApplied = observations.afterLoadedResumeTools.includes('probe_read');
    observations.afterResumeTurnId = await runTurn();
    observations.modelCalls = modelCalls;
    observations.dynamicToolObserved = observations.dynamicRoutes.length > 0;
    const afterTurn = await b.request('mcpServerStatus/list', { threadId: id, serverName: 'probe' });
    observations.afterTurnTools = afterTurn.data?.flatMap(s => Object.keys(s.tools ?? {})) ?? [];
    await b.request('thread/resume', { threadId: id, config: { mcp_servers: { probe: mcp } } });
    const released = await b.request('mcpServerStatus/list', { threadId: id, serverName: 'probe' });
    observations.afterDisabledResumeTools = released.data?.flatMap(s => Object.keys(s.tools ?? {})) ?? [];
    // Unsubscribe has a documented 30-minute inactivity grace; use a bounded
    // restart of ONLY this owned runtime, preserving its persisted thread ID.
    // This models pending activation at a supported runtime/resume boundary.
    await stopRuntime();
    await rm(socket, { force: true });
    await startRuntime();
    a = await connect('probe-owner-enabled'); b = a;
    const activated = await a.request('thread/resume', { threadId: id,
      config: { mcp_servers: { probe: { ...mcp, enabled: true } } } });
    assert.equal(activated.thread.id, id, 'activation changed session identity');
    const connected = await a.request('mcpServerStatus/list', { threadId: id, serverName: 'probe' });
    observations.afterRestartEnabledTools = connected.data?.flatMap(s => Object.keys(s.tools ?? {})) ?? [];
    assert.ok(observations.afterRestartEnabledTools.includes('probe_read'), 'resume boundary did not connect fixture MCP');
    observations.enabledResumeTurnId = await runTurn();
    await stopRuntime();
    await rm(socket, { force: true });
    await startRuntime();
    a = await connect('probe-owner-disabled'); b = a;
    const disabled = await a.request('thread/resume', { threadId: id,
      config: { mcp_servers: { probe: mcp } } });
    assert.equal(disabled.thread.id, id, 'release changed session identity');
    const disconnected = await a.request('mcpServerStatus/list', { threadId: id, serverName: 'probe' });
    observations.afterRestartDisabledTools = disconnected.data?.flatMap(s => Object.keys(s.tools ?? {})) ?? [];
    assert.equal(observations.afterRestartDisabledTools.length, 0, 'release boundary left MCP connected');
    assert.equal(await readFile(join(home, 'config.toml'), 'utf8'), config, 'persistent fixture config changed');
    t.diagnostic(JSON.stringify(observations));
  } catch (error) {
    t.diagnostic(`isolated-transport ${JSON.stringify(transport)}`);
    throw error;
  } finally {
    await stopRuntime();
    await new Promise(resolve => model.close(resolve));
    await rm(root, { recursive: true, force: true });
  }
});
