import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, mkdir, writeFile, readFile, symlink, rm, realpath, stat, readdir, readlink, copyFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve, dirname } from 'node:path';
import { createInterface } from 'node:readline';
import { createHash } from 'node:crypto';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { parse } from 'smol-toml';

const source = await readFile(resolve('public/gestalt'), 'utf8');
const program = source.split('launch_cli_with_xerj() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];
const controller = 'async function managedPlaywright(launchArgs, reportDiagnostic = () => {}, checkConsent = async () => false) {' + program.split('async function managedPlaywright(launchArgs, reportDiagnostic = () => {}, checkConsent = async () => false) {')[1].split('\nconst interactivePlaywright')[0];
const quote = value => "'" + value.replaceAll("'", "'\\''") + "'";
const waitFor = async (predicate, description, timeout = 20000) => {
  const deadline = Date.now() + timeout;
  while (!(await predicate()) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 25));
  assert.ok(await predicate(), description);
};
const execute = (command, args, env, cwd) => new Promise((resolve, reject) => {
  const child = spawn(command, args, { env, cwd, stdio: ['ignore', 'pipe', 'pipe'] });
  let output = ''; child.stdout.on('data', data => output += data); child.stderr.on('data', data => output += data);
  const timer = setTimeout(() => child.kill('SIGKILL'), 10000);
  child.once('error', reject); child.once('exit', code => { clearTimeout(timer); code === 0 ? resolve(output) : reject(new Error(output.slice(-1800))); });
});
async function seedPersistentThread(env, workspace) {
  const child = spawn(resolve('node_modules/.bin/codex'), ['app-server', '--stdio'], {
    env, cwd: workspace, detached: true, stdio: ['pipe', 'pipe', 'pipe'],
  });
  const pending = new Map(), events = []; let next = 0, stderr = '';
  child.stderr.on('data', data => stderr = (stderr + data).slice(-1600));
  const lines = createInterface({ input: child.stdout });
  lines.on('line', line => {
    const message = JSON.parse(line), waiter = pending.get(message.id);
    if (waiter) { pending.delete(message.id); message.error ? waiter.reject(new Error(JSON.stringify(message.error))) : waiter.resolve(message.result); }
    else events.push(message);
  });
  const request = (method, params) => new Promise((resolve, reject) => {
    const id = ++next, timer = setTimeout(() => { pending.delete(id); reject(new Error('seed thread deadline: ' + stderr)); }, 10000);
    pending.set(id, { resolve: value => { clearTimeout(timer); resolve(value); }, reject: error => { clearTimeout(timer); reject(error); } });
    child.stdin.write(JSON.stringify({ id, method, params }) + '\n');
  });
  try {
    await request('initialize', { clientInfo: { name: 'codex_cli_rs', version: 'fixture' }, capabilities: { experimentalApi: true } });
    child.stdin.write(JSON.stringify({ method: 'initialized', params: {} }) + '\n');
    // Ordinary native sessions have no Gestalt lifecycle tool. Never preinstall
    // one here: that would mask unsupported resume/fork attachment behavior.
    const started = await request('thread/start', { cwd: workspace, ephemeral: false, threadSource: 'user' });
    const turn = await request('turn/start', { threadId: started.thread.id, input: [{ type: 'text', text: 'SEED_NATIVE_THREAD' }] });
    await waitFor(() => events.some(event => event.method === 'turn/completed' && event.params.turn.id === turn.turn.id), 'seed persistent turn did not finish');
    return started.thread.id;
  } finally {
    for (const waiter of pending.values()) waiter.reject(new Error('seed cleanup'));
    lines.close(); child.stdin.end();
    const signal = name => { try { process.kill(-child.pid, name); } catch (error) { if (error.code !== 'ESRCH') throw error; } };
    signal('SIGTERM'); const timer = setTimeout(() => signal('SIGKILL'), 2000);
    await new Promise(resolve => child.exitCode !== null || child.signalCode !== null ? resolve() : child.once('exit', resolve));
    clearTimeout(timer); signal('SIGKILL');
  }
}
async function seedManagedTui(adapter, env, workspace, nativeArguments, events) {
  const args = ['-pbrowser', ...nativeArguments.slice(0, -1), 'SEED_NATIVE_THREAD'];
  const terminal = spawn('script', ['-qefc', `stty rows 30 cols 100; exec ${quote(process.execPath)} ${quote(adapter)} ${args.map(quote).join(' ')} 3<&0`, '/dev/null'],
    { env, cwd: workspace, stdio: ['pipe', 'pipe', 'pipe'] });
  let output = ''; terminal.stderr.resume();
  terminal.stdout.on('data', data => {
    const value = data.toString(); output += value;
    if (value.includes('\x1b[6n')) terminal.stdin.write('\x1b[1;1R');
    if (value.includes('\x1b[c') || value.includes('\x1b[>c')) terminal.stdin.write('\x1b[?1;2c');
  });
  try {
    await waitFor(() => output.includes('SEEDED_FIXTURE') || terminal.exitCode !== null, 'managed seed TUI did not finish');
    assert.equal(terminal.exitCode, null, output.slice(-1400));
    const root = (await events()).find(e => e.stage === 'runtime:thread/start' && e.message === 'persistent')?.cause.threadId;
    assert.ok(root, 'actual managed seed root absent');
    await waitFor(async () => (await events()).some(e => e.stage === 'runtime:turn/completed' && e.cause.threadId === root), 'managed seed turn did not complete');
    const before = output.length;
    terminal.stdin.write('\x1b[200~/exit\x1b[201~');
    await waitFor(() => output.slice(before).includes('/exit'), 'managed seed composer did not render exit');
    terminal.stdin.write('\r');
    await waitFor(() => terminal.exitCode !== null, 'managed seed TUI did not exit');
    assert.equal(terminal.exitCode, 0, output.slice(-1400));
    return root;
  } finally {
    if (terminal.exitCode === null && terminal.signalCode === null) {
      terminal.kill('SIGTERM'); const timer = setTimeout(() => terminal.kill('SIGKILL'), 3000);
      await new Promise(resolve => terminal.once('exit', resolve)); clearTimeout(timer);
    }
  }
}
function assertNativeOnly(actualText, beforeText, inheritedCounter) {
  const actual = parse(actualText), expected = parse(beforeText);
  const before = expected.tui?.model_availability_nux?.['gpt-6.1-sol'];
  const after = actual.tui?.model_availability_nux?.['gpt-6.1-sol'];
  if (before !== undefined) assert.ok(after === before || after === before + 1, 'unexpected native onboarding counter delta');
  else if (after !== undefined) assert.equal(after, inheritedCounter + 1, 'unexpected inherited native onboarding counter');
  if (after !== undefined) {
    const nux = Object.assign(Object.create(null), expected.tui?.model_availability_nux ?? {}, { 'gpt-6.1-sol': after });
    expected.tui = Object.assign(Object.create(null), expected.tui ?? {}, { model_availability_nux: nux });
  }
  assert.deepEqual(actual, expected, 'configuration changed beyond the bounded native onboarding side effect');
}
const toolText = value => typeof value === 'string' ? (() => {
  try { return toolText(JSON.parse(value)); } catch { return value; }
})() : Array.isArray(value) ? value.map(toolText).join('\n') : value && typeof value === 'object'
  ? Object.values(value).map(toolText).join('\n') : '';
async function browserTree(workspace, managed) {
  assert.equal(process.platform, 'linux', 'real process/listener gate requires Linux');
  const rows = [];
  for (const name of await readdir('/proc')) if (/^\d+$/.test(name)) try {
    const fields = (await readFile('/proc/' + name + '/stat', 'utf8')).split(') ').at(-1).split(' ');
    rows.push({ pid: Number(name), parent: Number(fields[1]), args: (await readFile('/proc/' + name + '/cmdline', 'utf8')).split('\0').filter(Boolean) });
  } catch {}
  const launchers = rows.filter(row => row.args.includes('mcp') && row.args.includes(managed) &&
    row.args[row.args.indexOf('--cwd') + 1] === workspace);
  assert.equal(launchers.length, 1, 'expected exactly one managed workspace MCP owner');
  const owned = new Set([launchers[0].pid]); let changed = true;
  while (changed) { changed = false; for (const row of rows) if (owned.has(row.parent) && !owned.has(row.pid)) { owned.add(row.pid); changed = true; } }
  const tree = rows.filter(row => owned.has(row.pid));
  assert.ok(tree.every(row => !row.args.some(arg => arg === 'codex' || arg.endsWith('/codex'))), 'nested Codex in Playwright path');
  const chrome = tree.find(row => row.args.includes('--remote-debugging-pipe'));
  assert.ok(chrome, 'real Chrome pipe missing');
  assert.ok(chrome.args.some(arg => arg.startsWith('--headless'))); assert.ok(!chrome.args.includes('--no-sandbox'));
  const sockets = new Set();
  for (const row of tree) try { for (const fd of await readdir('/proc/' + row.pid + '/fd')) try {
    const match = (await readlink('/proc/' + row.pid + '/fd/' + fd)).match(/^socket:\[(\d+)\]$/); if (match) sockets.add(match[1]);
  } catch {} } catch {}
  for (const kind of ['tcp', 'tcp6']) for (const line of (await readFile('/proc/net/' + kind, 'utf8')).trim().split('\n').slice(1)) {
    const fields = line.trim().split(/\s+/); assert.ok(!(fields[3] === '0A' && sockets.has(fields[9])), 'browser/MCP owns a TCP listener');
  }
  return tree.map(row => row.pid);
}
async function proveContention(manager, env, workspace) {
  const child = spawn(manager, ['playwright', 'mcp', '--cwd', workspace, '--executable-path', env.GESTALT_PLAYWRIGHT_REAL_CHROME],
    { env, cwd: workspace, stdio: ['pipe', 'pipe', 'pipe'] });
  let next = 0; const pending = new Map(), exited = new Promise(resolve => child.once('exit', resolve));
  child.stderr.resume(); const lines = createInterface({ input: child.stdout });
  lines.on('line', line => { const message = JSON.parse(line); pending.get(message.id)?.(message); pending.delete(message.id); });
  const request = (method, params) => new Promise((resolve, reject) => {
    const id = ++next, timer = setTimeout(() => reject(new Error('contention fixture deadline')), 10000);
    pending.set(id, message => { clearTimeout(timer); resolve(message); }); child.stdin.write(JSON.stringify({ id, method, params }) + '\n');
  });
  try {
    await request('initialize', { protocolVersion: '2024-11-05' });
    const listed = await request('tools/list', {}); assert.ok(listed.result.tools.some(tool => tool.name === 'browser_snapshot'));
    const busy = await request('tools/call', { name: 'browser_snapshot', arguments: {} });
    assert.match(busy.error?.message ?? '', /busy/);
  } finally { child.stdin.end(); await exited; lines.close(); }
}

const modes = process.env.GESTALT_PLAYWRIGHT_CLI_REAL === '1' ? ['profile-real-browser', 'profile-missing-chrome'] :
  process.env.GESTALT_PLAYWRIGHT_CLI_INTEGRATION === '1' ? ['profile-plan', 'profile-plan-resume'] :
  ['controller', 'profile-bootstrap', 'profile-repeat', 'profile-consent-loss', 'profile-empty-tools', 'profile-server-exit', 'profile-timeout', 'profile-resume', 'profile-fork', 'profile-managed-resume', 'profile-managed-fork', 'profile-managed-stale', 'profile-child'];
for (const mode of modes) test(`native TUI ${mode} retains session across pending browser activation and release`, { timeout: 90000 }, async t => {
  const bootstrap = mode !== 'controller';
  const realBrowser = ['profile-real-browser', 'profile-missing-chrome'].includes(mode);
  const ordinary = ['profile-resume', 'profile-fork'].includes(mode);
  const incompatible = mode === 'profile-managed-stale';
  const managedSeed = ['profile-managed-resume', 'profile-managed-fork', 'profile-managed-stale', 'profile-plan-resume'].includes(mode);
  const planMode = mode.startsWith('profile-plan');
  const failsReadiness = ['profile-empty-tools', 'profile-server-exit', 'profile-timeout', 'profile-missing-chrome'].includes(mode);
  let steps = [{ action: 'start', browser: false }, { text: 'FIRST_TURN_DONE', browser: false },
    { text: 'SECOND_TURN_DONE', browser: !failsReadiness }, { action: 'stop', browser: !failsReadiness },
    { text: 'THIRD_TURN_DONE', browser: !failsReadiness }, { text: 'FOURTH_TURN_DONE', browser: false }];
  if (mode === 'profile-repeat') steps = [{ action: 'start', browser: false }, { action: 'status', browser: false },
    { action: 'start', browser: false }, steps[1], steps[2], { action: 'status', browser: true },
    steps[3], { action: 'stop', browser: true }, steps[4], steps[5]];
  if (mode === 'profile-consent-loss') steps.splice(3, 2, { action: 'start', browser: false }, { text: 'THIRD_TURN_DONE', browser: false });
  if (ordinary) steps = ['FIRST_TURN_DONE', 'SECOND_TURN_DONE', 'THIRD_TURN_DONE', 'FOURTH_TURN_DONE'].map(text => ({ text, browser: false }));
  if (incompatible) for (const step of steps) step.browser = false;
  if (planMode) {
    steps.splice(0, 1, { action: 'bind_plan', plan: '../../forged', browser: false },
      { action: 'bind_plan', plan: 'browser-fixture', browser: false }, { action: 'status', browser: false });
    steps.splice(5, 2, { action: 'status', browser: false }, { text: 'THIRD_TURN_DONE', browser: false });
  }
  if (mode === 'profile-child') steps.splice(2, 0, { nativeTool: 'spawn_agent', browser: true }, { nativeTool: 'wait_agent', browser: true });
  if (mode === 'profile-real-browser') steps.splice(2, 0, ...['browser_navigate', 'browser_snapshot', 'browser_fill_form', 'browser_snapshot', 'browser_click', 'browser_take_screenshot']
    .map(nativeTool => ({ nativeTool, browser: true })));
  if (mode === 'profile-real-browser') steps.splice(9, 0, { nativeTool: 'browser_snapshot', browser: true });
  const root = await mkdtemp(join(tmpdir(), 'playwright-native-tui-'));
  const home = join(root, 'codex home'), workspace = join(root, 'workspace');
  if (realBrowser) assert.ok(process.env.GESTALT_PLAYWRIGHT_REAL_HOME && process.env.GESTALT_PLAYWRIGHT_REAL_CHROME,
    'Real CLI requires existing managed package and Chrome prerequisites');
  const managed = realBrowser ? process.env.GESTALT_PLAYWRIGHT_REAL_HOME : join(root, 'managed');
  const release = join(managed, 'playwright/release-0.0.1-fixture');
  const manager = join(root, 'manager.cjs'), adapter = join(root, 'adapter.mjs');
  const browserManager = realBrowser ? resolve('public/gestalt') : manager;
  const userOverride = `mcp_servers.fixture-user={command=${JSON.stringify(manager)},args=["unrelated"],enabled=true}`;
  const nativeArguments = ['--no-alt-screen', '-c', userOverride, 'ROOT_FIXTURE'];
  const eventsPath = join(root, 'private-events.jsonl');
  const env = { ...process.env, CODEX_HOME: home, GESTALT_HOME: managed, GESTALT_MANAGER_BIN: browserManager,
    TERM: 'xterm-256color', PATH: resolve('node_modules/.bin') + ':' + process.env.PATH };
  for (const key of Object.keys(env)) if (key.startsWith('CONTEXT_MODE_') || key.startsWith('GESTALT_MOBILE_') || key.startsWith('GESTALT_ORG_PLAN_')) delete env[key];
  if (mode === 'profile-timeout') env.GESTALT_PLAYWRIGHT_READY_TIMEOUT_MS = '1000';
  const events = async () => { try { return (await readFile(eventsPath, 'utf8')).trim().split('\n').filter(Boolean).map(JSON.parse); } catch { return []; } };
  let terminal, output = '', stderr = '', phase = 0, auxiliaryRequests = 0, childSeen = false, childCalled = false, modelRequests = 0;
  let seededId, planFile, planText, corePath, configBaseline, pageFixture, pageUrl, profileContents, packageBefore, chromeBefore, realVerified = false, observedPids = [];
  const catalogs = [], fixtureErrors = [];
  const model = createServer(async (request, response) => {
    try {
      let body = ''; for await (const chunk of request) body += chunk;
      const input = JSON.parse(body);
      const tools = input.tools.flatMap(tool => tool.type === 'namespace'
        ? tool.tools.map(child => ({ ...child, namespace: tool.name })) : [tool]);
      const browser = tools.some(tool => tool.name?.includes('browser_snapshot'));
      const lifecycle = tools.find(tool => tool.name === 'gestalt_managed_playwright');
      const latestUser = JSON.stringify(input.input.filter(item => item.role === 'user').at(-1));
      const seed = latestUser.includes('SEED_NATIVE_THREAD');
      const child = latestUser.includes('CHILD_AUTHORITY_FIXTURE');
      const auxiliary = (!lifecycle && !(ordinary && tools.some(tool => tool.name?.includes('fixture_user_read')))) || seed;
      if (planMode && !auxiliary && phase === 0) {
        const planSchema = (lifecycle.parameters ?? lifecycle.inputSchema)?.properties?.plan;
        assert.equal(planSchema?.type, 'string', 'persisted plan control schema unavailable');
        assert.equal(planSchema.enum, undefined, 'persisted plan catalog must not be an enum');
      }
      if (!auxiliary && !child && !tools.some(tool => tool.name?.includes('fixture_user_read')))
        fixtureErrors.push('explicit unrelated transport override lost');
      if (auxiliary) auxiliaryRequests++;
      else if (!child) catalogs.push({ phase, browser });
      let item;
      if (child) {
        childSeen = true;
        if (lifecycle && !childCalled) {
          childCalled = true;
          item = { id: 'child_fc', type: 'function_call', status: 'completed', name: lifecycle.name,
            ...(lifecycle.namespace ? { namespace: lifecycle.namespace } : {}), call_id: 'child_call', arguments: '{"action":"start"}' };
        } else {
          if (childCalled && !JSON.stringify(input.input).includes('root-supervisor-required-or-invalid-request')) fixtureErrors.push('child lifecycle authority was not rejected');
          item = { id: 'child_done', type: 'message', status: 'completed', role: 'assistant', content: [{ type: 'output_text', text: 'CHILD_AUTHORITY_DONE', annotations: [] }] };
        }
      } else if (auxiliary) {
        if (browser) fixtureErrors.push('auxiliary thread exposed managed browser tools');
        item = { id: `aux_${auxiliaryRequests}`, type: 'message', status: 'completed', role: 'assistant',
          content: [{ type: 'output_text', text: seed ? 'SEEDED_FIXTURE' : 'Fixture session', annotations: [] }] };
      } else if (steps[phase]?.nativeTool) {
        const operation = steps[phase].nativeTool;
        const selected = tools.find(tool => tool.name === operation || tool.name?.endsWith('__' + operation));
        assert.ok(selected, 'native child tool missing: ' + steps[phase].nativeTool);
        const agent = input.input.filter(item => item.type === 'function_call_output').map(item => {
          try { return JSON.parse(item.output).agent_id; } catch { return undefined; }
        }).find(Boolean);
        let args;
        if (operation === 'spawn_agent') args = { message: 'CHILD_AUTHORITY_FIXTURE: request browser lifecycle status without changing files.', fork_context: false };
        else if (operation === 'wait_agent') { assert.ok(agent, 'native child identity absent'); args = { targets: [agent], timeout_ms: 10000 }; }
        else {
          const text = toolText(input.input.filter(item => item.type === 'function_call_output').at(-1)?.output);
          if (operation === 'browser_navigate') args = { url: pageUrl };
          if (operation === 'browser_snapshot') args = {};
          if (operation === 'browser_fill_form') {
            const target = text.match(/textbox "Name" \[ref=(e\d+)\]/)?.[1]; assert.ok(target, 'fresh real textbox reference missing');
            args = { fields: [{ name: 'Name', element: 'Name', type: 'textbox', target, value: 'Native CLI browser' }] };
          }
          if (operation === 'browser_click') {
            const target = text.match(/button "Save" \[ref=(e\d+)\]/)?.[1]; assert.ok(target, 'fresh real button reference missing');
            args = { element: 'Save', target };
          }
          if (operation === 'browser_take_screenshot') args = { type: 'png', filename: join(workspace, 'cli-proof.png') };
        }
        item = { id: `native_${phase}`, type: 'function_call', status: 'completed', name: selected.name,
          ...(selected.namespace ? { namespace: selected.namespace } : {}), call_id: `native_call_${phase}`, arguments: JSON.stringify(args) };
      } else if (steps[phase]?.action) {
        if (!lifecycle) fixtureErrors.push('root lifecycle dynamic tool absent');
        item = { id: `fc_${phase}`, type: 'function_call', status: 'completed', name: lifecycle?.name ?? 'missing',
          ...(lifecycle?.namespace ? { namespace: lifecycle.namespace } : {}), call_id: `call_${phase}`,
          arguments: JSON.stringify({ action: steps[phase].action, ...(steps[phase].plan ? { plan: steps[phase].plan } : {}) }) };
      } else {
        const text = steps[phase]?.text ?? 'EXTRA_TURN_DONE';
        item = { id: `msg_${phase}`, type: 'message', status: 'completed', role: 'assistant',
          content: [{ type: 'output_text', text, annotations: [] }] };
      }
      if (!auxiliary && !child) {
        if (browser !== steps[phase]?.browser) fixtureErrors.push(`browser availability mismatch phase ${phase}`);
        if (steps[phase]?.browser && !JSON.stringify(input.input).includes('VERIFIED_BROWSER_GUIDANCE')) fixtureErrors.push('verified guidance absent');
        if (incompatible && phase === 1 && !JSON.stringify(input.input).includes('existing-thread-lifecycle-unavailable'))
          fixtureErrors.push('stale managed controls were not refused');
        if (mode === 'profile-real-browser' && steps[phase]?.action === 'stop' &&
            !toolText(input.input.filter(item => item.type === 'function_call_output').at(-1)?.output).includes('Saved'))
          fixtureErrors.push('contending owner disrupted the original browser');
        if (realBrowser && tools.some(tool => tool.name?.includes('browser_') && /run_code|evaluate|install/.test(tool.name)))
          fixtureErrors.push('unsafe browser capability exposed by default');
        phase++;
      }
      const result = { id: `resp_${++modelRequests}`, object: 'response', created_at: 1, status: 'completed', model: 'fixture', output: [item],
        usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
      response.writeHead(200, { 'Content-Type': 'text/event-stream' });
      for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
        { type: 'response.output_item.added', output_index: 0, item },
        { type: 'response.output_item.done', output_index: 0, item }, { type: 'response.completed', response: result }])
        response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
      response.end();
    } catch (error) { fixtureErrors.push(String(error)); response.writeHead(500); response.end(); }
  });
  try {
    for (const path of [home, workspace]) await mkdir(path, { recursive: true });
    if (!realBrowser) {
      await mkdir(join(release, 'node_modules'), { recursive: true });
      await symlink(resolve('node_modules/playwright-core'), join(release, 'node_modules/playwright-core'), 'dir');
      await writeFile(join(release, 'package.json'), '{"private":true}');
      await writeFile(join(managed, 'playwright/active.json'), JSON.stringify({ schemaVersion: 1, contractVersion: 1, version: '0.0.1', release }));
    } else {
      packageBefore = await readFile(join(managed, 'playwright/active.json'), 'utf8');
      chromeBefore = await stat(env.GESTALT_PLAYWRIGHT_REAL_CHROME);
      const blocked = join(root, 'blocked-commands'); await mkdir(blocked);
      for (const command of ['npm', 'npx', 'curl', 'wget']) await writeFile(join(blocked, command),
        '#!/bin/sh\nprintf "%s\\n" ' + command + ' >> ' + quote(join(root, 'unexpected-installer')) + '\nexit 99\n', { mode: 0o755 });
      env.PATH = blocked + ':' + env.PATH;
      pageFixture = createServer((request, response) => {
        const url = new URL(request.url, 'http://localhost');
        if (url.pathname === '/saved') {
          if (url.searchParams.get('name') !== 'Native CLI browser') fixtureErrors.push('real browser form value incorrect');
          response.setHeader('Set-Cookie', 'native-cli-proof=retained; Path=/; Max-Age=3600'); response.end('<h1>Saved</h1>'); return;
        }
        response.end('<!doctype html><title>CLI browser fixture</title><h1>Native CLI</h1><form action="/saved"><label>Name <input name="name"></label><button>Save</button></form>');
      });
      await new Promise(resolve => pageFixture.listen(0, '127.0.0.1', resolve));
      pageUrl = 'http://127.0.0.1:' + pageFixture.address().port;
    }
    await writeFile(manager, `#!/usr/bin/env node
const fs=require('node:fs');
fs.appendFileSync(${JSON.stringify(join(root, 'manager-calls'))},JSON.stringify(process.argv.slice(2))+'\\n');
if(process.argv[2]==='playwright'&&process.argv[3]==='doctor'){console.log(JSON.stringify({schemaVersion:1,contractVersion:1,ready:true,package:{ready:true},chrome:{ready:true},node:{ready:true}}));process.exit(0);}
const browser=!process.argv.includes('unrelated');
if(browser&&${JSON.stringify(mode)}==='profile-server-exit')process.exit(1);
require('node:readline').createInterface({input:process.stdin}).on('line',line=>{
 const m=JSON.parse(line);if(m.id===undefined)return;let result={};
 if(m.method==='initialize')result={protocolVersion:m.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'fixture',version:'1'}};
 if(m.method==='tools/list'&&browser&&${JSON.stringify(mode)}==='profile-timeout')return;
 if(m.method==='tools/list')result={tools:(!browser?['fixture_user_read']:${JSON.stringify(mode)}==='profile-empty-tools'?[]:['browser_navigate','browser_snapshot','browser_take_screenshot']).map(name=>({name,description:'Fixture tool',inputSchema:{type:'object',properties:{}},annotations:{readOnlyHint:true}}))};
 if(m.method==='tools/call')result={content:[{type:'text',text:'fixture browser'}]};
 console.log(JSON.stringify({jsonrpc:'2.0',id:m.id,result}));
});`, { mode: 0o755 });
    await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
    if (bootstrap) {
      const marketplace = join(root, 'marketplace'), plugin = join(marketplace, 'plugins/gestalt');
      await mkdir(join(marketplace, '.agents/plugins'), { recursive: true });
      await mkdir(join(plugin, '.codex-plugin'), { recursive: true });
      await mkdir(join(plugin, 'skills/playwright'), { recursive: true });
      await writeFile(join(plugin, '.codex-plugin/plugin.json'), JSON.stringify({ name: 'gestalt', version: '0.0.1',
        description: 'Isolated native Playwright fixture', skills: './skills/' }));
      await writeFile(join(plugin, 'skills/playwright/SKILL.md'), '---\nname: playwright\ndescription: Managed browser fixture guidance.\n---\nVERIFIED_BROWSER_GUIDANCE\n');
      await writeFile(join(marketplace, '.agents/plugins/marketplace.json'), JSON.stringify({ name: 'playwright-fixture', plugins: [
        { name: 'gestalt', source: { source: 'local', path: './plugins/gestalt' } },
      ] }));
      const codex = resolve('node_modules/.bin/codex');
      await execute(codex, ['plugin', 'marketplace', 'add', marketplace], env, workspace);
      await execute(codex, ['plugin', 'add', 'gestalt@playwright-fixture'], env, workspace);
      profileContents = '[mcp_servers.gestalt-playwright]\nenabled=true\n' + (realBrowser ?
        `default_tools_approval_mode="approve"\nargs=["playwright","mcp","--executable-path",${JSON.stringify(mode === 'profile-missing-chrome' ? join(root, 'absent-chrome') : env.GESTALT_PLAYWRIGHT_REAL_CHROME)}]\n` : '');
      await writeFile(join(home, 'browser.config.toml'), profileContents);
    }
    // Preseed native onboarding. The native availability marker is a launch
    // counter; assert its exact single increment and compare all other fields.
    // This capability does not depend on command hooks. Explicit native off
    // prevents unrelated XERJ/Serena guidance-hook review from driving this
    // browser fixture; deep config equality asserts it stays off throughout.
    const config = `model_provider="fixture"\nmodel="fixture"\napproval_policy="on-request"\nsandbox_mode="read-only"\n[projects.${JSON.stringify(workspace)}]\ntrust_level="trusted"\n[model_providers.fixture]\nname="fixture"\nbase_url="http://127.0.0.1:${model.address().port}/v1"\nwire_api="responses"\nrequires_openai_auth=false\nrequest_max_retries=0\n[features]\ntool_search=false\nhooks=false\n${mode === 'profile-child' ? 'multi_agent=true\n' : ''}[tui]\nscreen_reader_detection_done=true\n[tui.model_availability_nux]\n"gpt-6.1-sol"=1\n` +
      '[mcp_servers.fixture-user]\ncommand="/fixture-disabled-original"\nargs=[]\nenabled=false\n' +
      (bootstrap ? 'tool_timeout_sec=7.5\n' : '') +
      (bootstrap ? '[plugins."gestalt@playwright-fixture"]\nenabled=true\n' : '');
    await writeFile(join(home, 'config.toml'), config);
    configBaseline = config;
    if (planMode) {
      assert.ok(process.env.GESTALT_PLAYWRIGHT_AGENTS_CORE, 'Set fixed GESTALT_PLAYWRIGHT_AGENTS_CORE to accepted real Org core source');
      corePath = await realpath(process.env.GESTALT_PLAYWRIGHT_AGENTS_CORE);
      await mkdir(join(home, 'lib/gestalt-org-plan'), { recursive: true });
      await symlink(corePath, join(home, 'lib/gestalt-org-plan/org-plan-core.mjs'));
      await mkdir(join(workspace, '.gestalt'));
      planFile = join(workspace, '.gestalt/browser-fixture.org');
      planText = '#+TITLE: Browser fixture\n#+SUBTITLE: Validated host binding\n#+DATE: 2026-10-28\n#+KEYWORDS: fixture browser\n* WIP [#A] Browser requirement\n:PROPERTIES:\n:ID: browser-root\n:SKILLS: $bash-defensive-patterns\n:REVIEW_STATUS: UNREVIEWED\n:END:\n- Effort :: Medium\n- Goal :: Exercise host browser binding.\n- Notes :: Isolated native fixture.\n- Managed MCP :: playwright\n** WIP [#A] Browser child\n:PROPERTIES:\n:ID: browser-child\n:END:\n- Why :: Verify declaration.\n- Change :: Isolated fixture.\n- Tests :: Native host suite.\n- Done when :: Fixture completes.\n';
      if (!managedSeed) await writeFile(planFile, planText);
      const core = await import(pathToFileURL(corePath).href);
      if (!managedSeed) assert.equal(core.readPlan(planFile).items[0].fields.get('Managed MCP'), 'playwright');
      const dependency = (await execute('git', ['-C', dirname(corePath), 'rev-parse', 'HEAD'], env, workspace)).trim();
      assert.equal((await execute('git', ['-C', dirname(corePath), 'status', '--porcelain'], env, workspace)).trim(), '', 'Org dependency contains unreviewed source changes');
      t.diagnostic(JSON.stringify({ agentsCommit: dependency, coreSha256: createHash('sha256').update(await readFile(corePath)).digest('hex') }));
    }
    if (mode === 'profile-resume' || mode === 'profile-fork') {
      seededId = await seedPersistentThread(env, workspace);
      nativeArguments.splice(nativeArguments.length - 1, 0, mode === 'profile-resume' ? 'resume' : 'fork', seededId);
    }
    await writeFile(adapter, `import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {realpath,readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {appendFileSync} from 'node:fs';
const environment={...process.env},cwd=${JSON.stringify(workspace)},manager=${JSON.stringify(manager)},playwrightBudget=5000,flags=[];
const playwrightName='gestalt-playwright',playwrightConfig={enabled:true},playwrightSkill={},state={version:'codex 0.160.0'};
const usableGuidance=async()=> 'VERIFIED_BROWSER_GUIDANCE';
let currentChild;
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{currentChild?.kill(signal);setTimeout(()=>currentChild?.kill('SIGKILL'),2000).unref();});
${controller}
const result=await managedPlaywright(${JSON.stringify(nativeArguments)},(stage,error)=>appendFileSync(${JSON.stringify(eventsPath)},JSON.stringify({stage,message:error.message,cause:error.cause})+'\\n'),async()=>true);process.exitCode=result.code;
`);
    if (bootstrap) await writeFile(adapter,
      `import {appendFileSync as writeFixtureDiagnostic} from 'node:fs';\nconst fixtureDiagnostic=(stage,error)=>writeFixtureDiagnostic(${JSON.stringify(eventsPath)},JSON.stringify({stage,message:error.message,cause:error.cause})+'\\n');\n` +
      program.replace('managedPlaywright(launchArgs, undefined, refreshPlaywrightConsent)', 'managedPlaywright(launchArgs, fixtureDiagnostic, refreshPlaywrightConsent)'));
    if (managedSeed) {
      seededId = await seedManagedTui(adapter, env, workspace, nativeArguments, events);
      const recordDirectory = join(managed, 'playwright/cli-sessions');
      const recordFile = join(recordDirectory, seededId + '.json');
      assert.equal((await stat(recordDirectory)).mode & 0o777, 0o700);
      assert.equal((await stat(recordFile)).mode & 0o777, 0o600);
      const recorded = JSON.parse(await readFile(recordFile, 'utf8'));
      assert.deepEqual(Object.keys(recorded).sort(), ['nativeVersion', 'schemaHash', 'threadId', 'version', 'workspaceHash']);
      assert.equal(recorded.threadId, seededId); assert.equal(recorded.nativeVersion, '0.160.0');
      assert.equal(recorded.workspaceHash, createHash('sha256').update(await realpath(workspace)).digest('hex'));
      const seededConfig = await readFile(join(home, 'config.toml'), 'utf8');
      const seededProfile = await readFile(join(home, 'browser.config.toml'), 'utf8');
      assertNativeOnly(seededConfig, configBaseline);
      assertNativeOnly(seededProfile, profileContents, parse(configBaseline).tui.model_availability_nux['gpt-6.1-sol']);
      configBaseline = seededConfig; profileContents = seededProfile;
      await rm(eventsPath);
      if (incompatible) {
        const path = join(managed, 'playwright/cli-sessions', seededId + '.json');
        const record = JSON.parse(await readFile(path, 'utf8')); record.schemaHash = '0'.repeat(64);
        await writeFile(path, JSON.stringify(record), { mode: 0o600 });
      }
      nativeArguments.splice(nativeArguments.length - 1, 0, mode.endsWith('fork') ? 'fork' : 'resume', seededId);
      if (planMode) {
        // The persisted managed schema was created with an empty catalog. The
        // new plan must be discovered without stale serialized enum choices.
        await writeFile(planFile, planText);
        const core = await import(pathToFileURL(corePath).href);
        assert.equal(core.readPlan(planFile).items[0].fields.get('Managed MCP'), 'playwright');
      }
    }
    const nativeArgs = bootstrap ? ' ' + ['-pbrowser', ...nativeArguments].map(quote).join(' ') : '';
    terminal = spawn('script', ['-qefc', `stty rows 30 cols 100; exec ${quote(process.execPath)} ${quote(adapter)}${nativeArgs} 3<&0`, '/dev/null'],
      { cwd: workspace, env, stdio: ['pipe', 'pipe', 'pipe'] });
    terminal.stdout.on('data', chunk => {
      const value = chunk.toString(); output = (output + value).slice(-100000);
      // Answer standard terminal cursor/device queries from the real TUI.
      if (value.includes('\x1b[6n')) terminal.stdin.write('\x1b[1;1R');
      if (value.includes('\x1b[c') || value.includes('\x1b[>c')) terminal.stdin.write('\x1b[?1;2c');
    });
    terminal.stderr.on('data', chunk => stderr = (stderr + chunk).slice(-4000));
    const submit = async prompt => {
      const before = output.length;
      // TUI burst detection can treat text plus Enter in one write as a pasted
      // newline. Use the terminal's explicit bracketed-paste event, observe the
      // composer render, then send Enter as its own key event.
      terminal.stdin.write('\x1b[200~' + prompt + '\x1b[201~');
      await waitFor(() => output.slice(before).includes(prompt) || terminal.exitCode !== null,
        'native composer did not render submitted text');
      assert.equal(terminal.exitCode, null, output.slice(-1800));
      terminal.stdin.write('\r');
    };
    let completedTurns = 0;
    for (const [marker, prompt] of [['FIRST_TURN_DONE', 'FOLLOWUP'], ['SECOND_TURN_DONE', 'RELEASE'], ['THIRD_TURN_DONE', 'VERIFY_RELEASE']]) {
      await waitFor(() => output.includes(marker) || terminal.exitCode !== null, `native TUI missing ${marker}`);
      assert.equal(terminal.exitCode, null, output.slice(-3000) + stderr);
      completedTurns++;
      await waitFor(async () => {
        const current = await events();
        const rootId = current.find(e => ['runtime:thread/start', 'runtime:thread/resume', 'runtime:thread/fork'].includes(e.stage) && e.message === 'persistent')?.cause.threadId;
        return current.filter(e => e.stage === 'runtime:turn/completed' && e.cause.threadId === rootId).length >= completedTurns;
      },
        'native turn did not reach observable completion before next submission');
      if (mode === 'profile-consent-loss' && marker === 'SECOND_TURN_DONE')
        await writeFile(join(home, 'browser.config.toml'), '[mcp_servers.gestalt-playwright]\nenabled=false\n');
      if (planMode && marker === 'SECOND_TURN_DONE') {
        const cli = join(dirname(corePath), 'org-plan-cli.mjs');
        await execute(process.execPath, [cli, 'l2', planFile, 'browser-child', 'DONE'], env, workspace);
        await execute(process.execPath, [cli, 'set', planFile, 'browser-root', 'DONE'], env, workspace);
      }
      if (mode === 'profile-real-browser' && marker === 'SECOND_TURN_DONE') {
        assert.ok((await stat(join(workspace, 'cli-proof.png'))).size > 0, 'real CLI screenshot absent');
        observedPids = await browserTree(workspace, managed);
        await proveContention(browserManager, env, workspace);
        if (env.GESTALT_PLAYWRIGHT_CLI_ARTIFACTS) {
          await mkdir(env.GESTALT_PLAYWRIGHT_CLI_ARTIFACTS, { recursive: true });
          await copyFile(join(workspace, 'cli-proof.png'), join(env.GESTALT_PLAYWRIGHT_CLI_ARTIFACTS, 'cli-browser-proof.png'));
        }
      }
      await submit(prompt);
    }
    await waitFor(() => output.includes('FOURTH_TURN_DONE') || terminal.exitCode !== null, 'native TUI missing release verification');
    await submit('/exit');
    await waitFor(() => terminal.exitCode !== null, 'native TUI did not exit');
    assert.equal(terminal.exitCode, 0, output.slice(-3000) + stderr);
    assert.deepEqual(fixtureErrors, []);
    const rootId = (await events()).find(e => ['runtime:thread/start', 'runtime:thread/resume', 'runtime:thread/fork'].includes(e.stage) && e.message === 'persistent')?.cause.threadId;
    if (mode.endsWith('resume') || incompatible) assert.equal(rootId, seededId, 'native resume changed persistent session identity');
    if (mode.endsWith('fork')) {
      assert.notEqual(rootId, seededId, 'native fork reused source thread');
      assert.ok((await events()).some(e => e.stage === 'frontend:thread/fork' && e.cause.threadId === seededId));
    }
    if (mode === 'profile-child') assert.ok(childSeen, 'actual native child did not run');
    const failures = (await events()).filter(event => ['thread/start', 'thread/resume', 'thread/fork', 'turn/start'].includes(event.stage));
    assert.deepEqual(failures, [], 'native auxiliary/root configuration request failed');
    const timeouts = (await events()).filter(event => event.stage === 'runtime:overlay-timeouts').flatMap(event => event.cause);
    assert.ok(timeouts.length, 'no actual native config roundtrip captured');
    assert.ok(timeouts.every(server => server.nullFields.length === 0), 'native absent optional values leaked into TOML overrides');
    for (const server of timeouts.filter(server => server.name === 'fixture-user'))
      assert.equal(server.value, bootstrap ? 7.5 : undefined, 'configured numeric timeout changed');
    assertNativeOnly(await readFile(join(home, 'config.toml'), 'utf8'), configBaseline);
    if (bootstrap) {
      assertNativeOnly(await readFile(join(home, 'browser.config.toml'), 'utf8'),
        mode === 'profile-consent-loss' ? '[mcp_servers.gestalt-playwright]\nenabled=false\n' : profileContents,
        parse(configBaseline).tui.model_availability_nux['gpt-6.1-sol']);
    }
    const managerCalls = (await readFile(join(root, 'manager-calls'), 'utf8')).trim().split('\n').map(JSON.parse);
    assert.ok(managerCalls.every(args => args[0] === 'unrelated' || (args[0] === 'playwright' && ['mcp', 'doctor'].includes(args[1]))),
      'CLI startup invoked package installation/update or an unexpected manager operation');
    if (!realBrowser) assert.equal(managerCalls.filter(args => args[0] === 'playwright' && args[1] === 'mcp').length, ordinary || incompatible ? 0 : 1, 'unexpected or duplicate MCP owner');
    if (ordinary || incompatible) {
      assert.ok(output.includes('existing thread has no compatible managed controls'), 'unsupported old thread did not receive truthful diagnostic');
      assert.ok(catalogs.every(row => row.browser === false));
      assert.ok(managerCalls.every(args => args[0] !== 'playwright'), 'unavailable persisted controls invoked a browser readiness/activation command');
      if (ordinary) assert.equal((await readdir(join(managed, 'playwright/cli-sessions'))).filter(name => name.endsWith('.json')).length, 0, 'ordinary seed incorrectly recorded as managed');
    }
    if (realBrowser) {
      assert.equal(await readFile(join(managed, 'playwright/active.json'), 'utf8'), packageBefore, 'CLI changed installed package metadata');
      const chromeAfter = await stat(env.GESTALT_PLAYWRIGHT_REAL_CHROME);
      assert.deepEqual([chromeAfter.size, chromeAfter.mtimeMs], [chromeBefore.size, chromeBefore.mtimeMs], 'CLI changed external Chrome');
      await assert.rejects(stat(join(root, 'unexpected-installer')), { code: 'ENOENT' });
      await waitFor(async () => {
        for (const pid of observedPids) try {
          if ((await readFile('/proc/' + pid + '/stat', 'utf8')).split(') ').at(-1).split(' ')[0] !== 'Z') return false;
        } catch {}
        return true;
      }, 'owned browser descendants survived CLI shutdown', 5000);
      const state = join(managed, 'playwright/workspaces', createHash('sha256').update(workspace).digest('hex'));
      await assert.rejects(stat(join(state, 'lease')), { code: 'ENOENT' }, 'CLI left workspace lease owned');
      if (mode === 'profile-real-browser') {
        assert.equal((await stat(join(state, 'chrome'))).mode & 0o777, 0o700);
        t.diagnostic(JSON.stringify({ realBrowserProcesses: observedPids.length,
          package: JSON.parse(packageBefore).version, chrome: (await execute(env.GESTALT_PLAYWRIGHT_REAL_CHROME, ['--version'], env, workspace)).trim(),
          screenshotSha256: createHash('sha256').update(await readFile(join(workspace, 'cli-proof.png'))).digest('hex') }));
      } else await assert.rejects(stat(join(state, 'chrome')), { code: 'ENOENT' }, 'missing Chrome created a browser profile');
      realVerified = true;
    }
    t.diagnostic(JSON.stringify({ catalogs, managerCalls: managerCalls.length }));
  } catch (error) {
    t.diagnostic(JSON.stringify({ phase, catalogs, events: (await events()).slice(-12) }));
    t.diagnostic('terminal tail: ' + output.slice(-1800).replaceAll('\x1b', '<ESC>') + stderr);
    throw error;
  } finally {
    if (terminal && terminal.exitCode === null) {
      terminal.kill('SIGTERM');
      await new Promise(resolve => {
        const timer = setTimeout(() => { terminal.kill('SIGKILL'); }, 5000);
        terminal.once('exit', () => { clearTimeout(timer); resolve(); });
      });
    }
    // Recover only this fixture's captured detached backends if terminal
    // teardown interrupted its normal finally block. Verify the private home
    // marker before signalling any recorded process group.
    for (const event of await events()) if (event.stage === 'runtime:spawn' && event.cause?.pid) try {
      const pid = event.cause.pid;
      if ((await readFile('/proc/' + pid + '/environ', 'utf8')).split('\0').includes('CODEX_HOME=' + home)) process.kill(-pid, 'SIGKILL');
    } catch (error) { if (!['ENOENT', 'ESRCH'].includes(error.code)) throw error; }
    await new Promise(resolve => model.close(resolve));
    if (pageFixture) await new Promise(resolve => pageFixture.close(resolve));
    if (realBrowser && realVerified) {
      const state = join(managed, 'playwright/workspaces', createHash('sha256').update(workspace).digest('hex'));
      try { await stat(join(state, 'lease')); } catch (error) { if (error.code === 'ENOENT') await rm(state, { recursive: true, force: true }); else throw error; }
      for (const event of await events()) if (['runtime:thread/start', 'runtime:thread/resume', 'runtime:thread/fork'].includes(event.stage) && event.message === 'persistent')
        await rm(join(managed, 'playwright/cli-sessions', event.cause.threadId + '.json'), { force: true });
    }
    await rm(root, { recursive: true, force: true });
  }
});
