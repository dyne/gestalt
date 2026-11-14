import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { createInterface } from 'node:readline';
import { createHash } from 'node:crypto';
import { mkdtemp, mkdir, readFile, writeFile, rm, copyFile, readdir, stat, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { parse } from 'smol-toml';

const source = await readFile(resolve('public/gestalt'), 'utf8');
const program = source.split('launch_cli_with_capabilities() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];
const shellFunction = 'launch_cli_with_capabilities() {' + source.split('launch_cli_with_capabilities() {')[1].split('\ncommand_cli()')[0];

async function execute(command, args, env, cwd, input) {
  const child = spawn(command, args, { env, cwd, stdio: ['pipe', 'pipe', 'pipe', 'pipe'] });
  let stdout = '', stderr = '';
  child.stdout.on('data', data => stdout += data);
  child.stderr.on('data', data => stderr += data);
  const exited = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', resolve); });
  const timer = setTimeout(() => child.kill('SIGKILL'), 30000);
  child.stdio[3].end();
  child.stdin.end(input);
  try { return { code: await exited, stdout, stderr }; } finally { clearTimeout(timer); }
}

async function configRead(env, cwd, overrides = []) {
  const child = spawn('codex', ['app-server', '--stdio', ...overrides.flatMap(value => ['-c', value])],
    { env, cwd, stdio: ['pipe', 'pipe', 'ignore'] });
  let next = 0;
  const pending = new Map();
  const lines = createInterface({ input: child.stdout });
  lines.on('line', line => {
    const value = JSON.parse(line), waiter = pending.get(value.id);
    if (waiter) { pending.delete(value.id); value.error ? waiter.reject(new Error('native config unavailable')) : waiter.resolve(value.result); }
  });
  const request = (method, params) => new Promise((resolve, reject) => {
    const id = ++next; pending.set(id, { resolve, reject });
    child.stdin.write(JSON.stringify({ id, method, params }) + '\n');
  });
  const timer = setTimeout(() => { for (const waiter of pending.values()) waiter.reject(new Error('native config deadline')); child.kill(); }, 5000);
  try {
    const initialized = await request('initialize', { clientInfo: { name: 'startup-native-fixture', version: '1' }, capabilities: {} });
    child.stdin.write(JSON.stringify({ method: 'initialized', params: {} }) + '\n');
    return { initialized, ...await request('config/read', { cwd, includeLayers: true }) };
  } finally {
    clearTimeout(timer); lines.close(); child.stdin.end(); child.kill();
    if (child.exitCode == null && child.signalCode == null) await new Promise(resolve => child.once('exit', resolve));
  }
}

test('native config resolves developer scalar through trusted project and CLI layers', { timeout: 15000 }, async t => {
  const root = await mkdtemp(join(tmpdir(), 'gestalt startup precedence '));
  t.after(() => rm(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }));
  const home = join(root, 'home'), workspace = join(root, 'workspace'), nested = join(workspace, 'nested');
  for (const path of [home, join(workspace, '.codex'), join(nested, '.codex')]) await mkdir(path, { recursive: true });
  const env = { ...process.env, CODEX_HOME: home };
  await execute('git', ['init', '-q', workspace], env, root);
  await writeFile(join(home, 'config.toml'), `developer_instructions='USER_SCALAR'\n[projects.${JSON.stringify(workspace)}]\ntrust_level='trusted'\n`);
  await writeFile(join(workspace, '.codex/config.toml'), 'developer_instructions="PROJECT_SCALAR"\n');
  await writeFile(join(nested, '.codex/config.toml'), 'developer_instructions="""\nCLOSEST_SCALAR\nquoted \"text\"\n"""\n');
  const project = await configRead(env, nested);
  assert.equal(project.config.developer_instructions, 'CLOSEST_SCALAR\nquoted "text"\n');
  assert.ok(project.layers.some(layer => layer.name?.type === 'project' && !layer.disabledReason));
  const cli = await configRead(env, nested, ['developer_instructions="CLI_SCALAR"']);
  assert.equal(cli.config.developer_instructions, 'CLI_SCALAR');
  assert.match(project.initialized.userAgent, /^startup-native-fixture\/\d+\.\d+\.\d+ /);
});

// A small synthetic package keeps this repository's default suite standalone.
// An authorized external package can replace these three contract files to
// exercise the accepted Agents composer without embedding its stable prose.
async function packageFixture(plugin) {
  for (const path of ['.codex-plugin', 'skills/coding', 'startup', 'scripts']) await mkdir(join(plugin, path), { recursive: true });
  await writeFile(join(plugin, '.codex-plugin/plugin.json'), JSON.stringify({ name: 'gestalt', version: '0.0.1', description: 'Synthetic startup fixture', skills: './skills/' }));
  await writeFile(join(plugin, 'skills/coding/SKILL.md'), '---\nname: coding\ndescription: Synthetic coding fixture\n---\nFixture skill.\n');
  if (process.env.GESTALT_STARTUP_FIXTURE_ROOT) {
    for (const path of ['gestalt-startup-contract.json', 'startup/bootstrap.md', 'scripts/startup-contract.mjs'])
      await copyFile(join(process.env.GESTALT_STARTUP_FIXTURE_ROOT, path), join(plugin, path));
  } else {
    await writeFile(join(plugin, 'gestalt-startup-contract.json'), JSON.stringify({ schemaVersion: 1, owner: 'gestalt-agents', blockId: 'gestalt:startup', blockVersion: 1,
      source: 'startup/bootstrap.md', composer: 'scripts/startup-contract.mjs', maxWords: 350 }));
    await writeFile(join(plugin, 'startup/bootstrap.md'), 'SYNTHETIC_STARTUP_CONTENT\n');
    // This test double exercises the consumer's package boundary. The Agents
    // suite owns composer validation; a second run can use that actual library.
    await writeFile(join(plugin, 'scripts/startup-contract.mjs'), `
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
export async function readStartupContract(root) { return {status:'ready',text:await readFile(resolve(root,'startup/bootstrap.md'),'utf8')}; }
export function composeStartup(value,contract) {
  const block='<!-- gestalt:startup:v1 -->\\n'+contract.text+'<!-- /gestalt:startup -->';
  const pattern=/<!-- gestalt:startup:v1 -->[\\s\\S]*?<!-- \\/gestalt:startup -->/g;
  const matches=[...value.matchAll(pattern)];
  if((value.match(/<!-- gestalt:startup/g)??[]).length!==matches.length)throw Error('malformed fixture block');
  if(!matches.length)return value+(value?'\\n\\n':'')+block;
  let first=true;return value.replace(pattern,()=>{if(!first)return '';first=false;return block;});
}
`);
  }
  const text = await readFile(join(plugin, 'startup/bootstrap.md'), 'utf8');
  return `<!-- gestalt:startup:v1 -->\n${text}<!-- /gestalt:startup -->`;
}

const count = (text, needle) => text.split(needle).length - 1;
const digest = value => createHash('sha256').update(JSON.stringify(value)).digest('hex');
const messageText = input => input.input.filter(item => item.role).map(item =>
  typeof item.content === 'string' ? item.content : item.content.map(part => part.text ?? '').join('\n')).join('\n');
const instructionParts = input => input.input.filter(item => ['system', 'developer'].includes(item.role)).flatMap(item =>
  (typeof item.content === 'string' ? [item.content] : item.content.map(part => part.text)).filter(text => typeof text === 'string').map(text => ({ role: item.role, text })));
function assertNativeContent(baseline, input) {
  if (Object.hasOwn(baseline, 'instructions')) assert.equal(digest(input.instructions), digest(baseline.instructions), 'native separate foundation field changed');
  const native = instructionParts(baseline), received = instructionParts(input);
  assert.ok(native.length > 0, 'native foundation not observed');
  for (const part of native) assert.equal(received.filter(value => value.role === part.role)
    .reduce((total, value) => total + count(value.text, part.text), 0), 1, 'complete native instruction component changed or duplicated');
}

async function deliveredFixture(t, mode) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt delivered startup '));
  t.after(() => rm(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }));
  const home = join(root, 'home'), workspace = join(root, 'workspace'), nested = join(workspace, 'nested');
  const marketplace = join(root, 'marketplace'), plugin = join(marketplace, 'plugins/gestalt');
  for (const path of [home, nested, join(marketplace, '.agents/plugins')]) await mkdir(path, { recursive: true });
  const block = await packageFixture(plugin);
  const namedRole = ['named-role', 'role-owned-block', 'root-owned-role-override', 'stale-role-block'].includes(mode);
  const childMode = ['default-child', 'scratch-child'].includes(mode) || namedRole;
  let roleFileContents;
  if (namedRole) {
    await mkdir(join(home, 'agents'));
    roleFileContents = 'name="fixture_worker"\ndescription="Synthetic selected worker role"\ndeveloper_instructions=' +
      JSON.stringify('ROLE_ONLY_DUTY\nquoted "role"\n' + (mode === 'role-owned-block' ? block : mode === 'stale-role-block' ?
        '<!-- gestalt:startup:v1 -->\nOLD_ROLE_BLOCK\n<!-- /gestalt:startup -->' : '')) + '\n';
    await writeFile(join(home, 'agents/fixture-worker.toml'), roleFileContents);
  }
  if (mode === 'explicit-foundation') await writeFile(join(home, 'foundation.md'), 'EXPLICIT_USER_FOUNDATION\n');
  if (mode === 'older-package') await rm(join(plugin, 'gestalt-startup-contract.json'));
  if (mode === 'unsupported-package') {
    const descriptor = JSON.parse(await readFile(join(plugin, 'gestalt-startup-contract.json'), 'utf8'));
    await writeFile(join(plugin, 'gestalt-startup-contract.json'), JSON.stringify({ ...descriptor, schemaVersion: 99 }));
  }
  await writeFile(join(marketplace, '.agents/plugins/marketplace.json'), JSON.stringify({ name: 'startup-fixture', plugins: [{ name: 'gestalt', source: { source: 'local', path: './plugins/gestalt' } }] }));
  const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith('CONTEXT_MODE_') &&
    !['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].includes(name)));
  Object.assign(env, { CODEX_HOME: home, GESTALT_HOME: join(root, 'managed'), GESTALT_MANAGER_BIN: join(root, 'manager') });
  await writeFile(env.GESTALT_MANAGER_BIN, '#!/bin/sh\nprintf \'{"schemaVersion":1,"status":"absent"}\\n\'\n', { mode: 0o755 });
  for (const args of [['plugin', 'marketplace', 'add', marketplace], ['plugin', 'add', 'gestalt@startup-fixture']]) {
    const result = await execute('codex', args, env, workspace);
    assert.equal(result.code, 0, result.stderr.slice(-800));
  }
  await execute('git', ['init', '-q', workspace], env, root);
  await writeFile(join(home, 'AGENTS.md'), 'GLOBAL_STARTUP_AGENTS');
  await writeFile(join(workspace, 'AGENTS.md'), 'REPOSITORY_STARTUP_AGENTS');
  const requests = [];
  const phases = new WeakMap(), childRequests = new WeakMap();
  let activePhase = 'direct';
  const errors = [];
  const ownershipCaptures = [];
  let spawned = false, waited = false;
  let directSpawned = false, directWaited = false;
  const model = createServer(async (request, response) => {
    let raw = ''; for await (const data of request) raw += data;
    const input = JSON.parse(raw); requests.push(input);
    const phase = mode === 'scratch-child' ? await readFile(join(root, 'phase'), 'utf8') : activePhase;
    phases.set(input, phase);
    if (mode === 'scratch-child' && phase === 'wrapped') {
      const { context } = JSON.parse(await readFile(join(root, 'ownership.json'), 'utf8'));
      const marker = JSON.parse(await readFile(context.markerPath, 'utf8'));
      const leases = await Promise.all((await readdir(join(context.directory, 'leases'))).map(async name =>
        JSON.parse(await readFile(join(context.directory, 'leases', name), 'utf8'))));
      ownershipCaptures.push({ context, marker, leases, controllerProtected: (await stat(join(context.directory, '.controller-lock'))).isDirectory() });
    }
    const userMessages = input.input.filter(item => item.role === 'user').map(item => JSON.stringify(item.content));
    const rootRequest = userMessages.some(text => text.includes('STARTUP_ROOT') || text.includes('STARTUP_DIRECT') && !text.includes('STARTUP_DIRECT_CHILD'));
    const isChild = !rootRequest && userMessages.some(text => text.includes('STARTUP_CHILD') || text.includes('STARTUP_DIRECT_CHILD'));
    childRequests.set(input, isChild);
    const tools = input.tools.flatMap(tool => tool.type === 'namespace' ? tool.tools.map(child => ({ ...child, namespace: tool.name })) : [tool]);
    let item;
    const call = (name, args) => {
      const tool = tools.find(tool => tool.name === name);
      if (!tool) { errors.push('missing native ' + name); return; }
      return { id: 'fc_' + name, type: 'function_call', status: 'completed', name: tool.name,
        ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: name, arguments: JSON.stringify(args) };
    };
    const direct = phase === 'direct';
    if (childMode && !isChild && !(direct ? directSpawned : spawned)) {
      if (direct) directSpawned = true; else spawned = true;
      item = call('spawn_agent', { message: direct ? 'STARTUP_DIRECT_CHILD' : 'STARTUP_CHILD', fork_context: false,
        ...(namedRole ? { agent_type: 'fixture_worker' } : {}) });
    } else if (childMode && !isChild && !(direct ? directWaited : waited)) {
      if (direct) directWaited = true; else waited = true;
      const agent = input.input.filter(item => item.type === 'function_call_output').map(item => { try { return JSON.parse(item.output).agent_id; } catch { return undefined; } }).find(Boolean);
      if (!agent) errors.push('missing native child result');
      else item = call('wait_agent', { targets: [agent], timeout_ms: 10000 });
    }
    item ??= { id: `msg_${requests.length}`, type: 'message', status: 'completed', role: 'assistant', content: [{ type: 'output_text', text: 'Synthetic fixture completed.', annotations: [] }] };
    const result = { id: `resp_${requests.length}`, object: 'response', created_at: 1, status: 'completed', model: 'fixture', output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
    response.writeHead(200, { 'Content-Type': 'text/event-stream' });
    for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
      { type: 'response.output_item.added', output_index: 0, item }, { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response: result }]) response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
    response.end();
  });
  await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
  try {
    const scalar = mode === 'empty' ? undefined : mode === 'root-owned-role-override' ? 'USER_STARTUP_SCALAR\nROOT_ONLY_DUTY\n\n' + block :
      namedRole ? 'USER_STARTUP_SCALAR\nROOT_ONLY_DUTY\n' : mode === 'existing-block' ? 'USER_STARTUP_SCALAR\n\n' + block :
      mode === 'stale-block' ? 'USER_STARTUP_SCALAR\n\n<!-- gestalt:startup:v1 -->\nOLD_STARTUP_BLOCK\n<!-- /gestalt:startup -->' :
        'USER_STARTUP_SCALAR\nquoted "value"\nsecond line';
    const profile = mode === 'named-profile' || mode === 'trusted-project' || mode === 'explicit-cli';
    const profileConfig = 'developer_instructions="""\nPROFILE_STARTUP_SCALAR\nquoted \\"value\\"\n"""\n';
    if (profile) await writeFile(join(home, 'fixture.config.toml'), profileConfig);
    const project = mode === 'trusted-project' || mode === 'explicit-cli';
    const cwd = project ? nested : workspace;
    if (project) {
      for (const path of [join(workspace, '.codex'), join(nested, '.codex')]) await mkdir(path);
      await writeFile(join(workspace, '.codex/config.toml'), 'developer_instructions="PROJECT_STARTUP_SCALAR"\n');
      await writeFile(join(nested, '.codex/config.toml'), 'developer_instructions="CLOSEST_STARTUP_SCALAR"\n');
    }
    const config = `model='fixture'\nmodel_provider='fixture'\napproval_policy='never'\n${mode === 'explicit-foundation' ? 'model_instructions_file=' + JSON.stringify(join(home, 'foundation.md')) + '\n' : ''}${scalar === undefined ? '' : 'developer_instructions=' + JSON.stringify(scalar) + '\n'}[projects.${JSON.stringify(workspace)}]\ntrust_level='trusted'\n[model_providers.fixture]\nname='fixture'\nbase_url='http://127.0.0.1:${model.address().port}/v1'\nwire_api='responses'\nrequires_openai_auth=false\n[features]\nhooks=${mode !== 'disabled-hooks'}\nmulti_agent=true\ntool_search=false\n[skills]\ninclude_instructions=${mode !== 'disabled-skills'}\n${mode === 'excluded-skill' ? '[[skills.config]]\nname="gestalt:coding"\nenabled=false\n' : ''}[plugins."gestalt@startup-fixture"]\nenabled=true\n`;
    await writeFile(join(home, 'config.toml'), config);
    const selectors = [...(profile ? ['-pfixture'] : []), ...(mode === 'explicit-cli' ? ['-c', 'developer_instructions="CLI_STARTUP_SCALAR"'] : [])];
    if (mode !== 'scratch-child') {
      const direct = await execute('codex', ['--no-daemon', ...selectors, 'exec', '--skip-git-repo-check', '--json', '-C', cwd, 'STARTUP_DIRECT'], env, cwd);
      assert.equal(direct.code, 0, direct.stderr.slice(-800));
      assert.ok(requests.length > 0, 'direct native request missing');
    }
    if (mode === 'explicit-cli') {
      const nativeArgs = ['exec', '--skip-git-repo-check', '--json', '-C', cwd, 'STARTUP_DIRECT'];
      for (const [phase, args] of [['probe-global', ['--no-daemon', ...selectors, '-c', 'features.tool_search=false', ...nativeArgs]],
        ['probe-split', ['--no-daemon', ...selectors, ...nativeArgs, '-c', 'features.tool_search=false']]]) {
        activePhase = phase;
        const probe = await execute('codex', args, env, cwd);
        assert.equal(probe.code, 0, probe.stderr.slice(-800));
        const input = requests.find(input => phases.get(input) === phase);
        assert.ok(input, 'native override-position probe missing');
        const text = messageText(input);
        t.diagnostic(JSON.stringify({ mode, nativeOverridePosition: phase, cliScalarCount: count(text, 'CLI_STARTUP_SCALAR'),
          profileScalarCount: count(text, 'PROFILE_STARTUP_SCALAR'), userScalarCount: count(text, 'USER_STARTUP_SCALAR'),
          instructionComponents: instructionParts(input).map(part => ({ role: part.role, bytes: Buffer.byteLength(part.text), sha256: digest(part.text) })) }));
      }
    }
    let stderr = '';
    let scratchResult;
    for (let i = 0; i < (mode === 'repeat' ? 2 : 1); i++) {
      activePhase = 'wrapped';
      let result;
      if (mode === 'scratch-child') {
        // Reuse the public allocator, guardian and adapter unchanged. This
        // bypasses unrelated installed-component diagnostics, and still runs
        // native Codex plus the real public __scratch-hook for both identities.
        const manager = resolve('public/gestalt');
        const launchScript = join(root, 'launch.sh'), controller = join(root, 'controller.cjs');
        await writeFile(launchScript, 'codex_home=$CODEX_HOME\ngestalt_home=$GESTALT_HOME\nmanager_target=$GESTALT_MANAGER_BIN\n' + shellFunction +
          '\n# Both native baselines use the same validated owner/environment.\ndirect_args=("$@")\ndirect_args[${#direct_args[@]}-1]=STARTUP_DIRECT\nprintf direct > "$STARTUP_PHASE_FILE"\ncodex --no-daemon "${direct_args[@]}" || exit $?\nprintf wrapped > "$STARTUP_PHASE_FILE"\nlaunch_cli_with_capabilities "$@"\n');
        await writeFile(controller, `const fs=require('node:fs');
const manager=${JSON.stringify(manager)};
const source=fs.readFileSync(manager,'utf8').match(/^\\/\\/ BEGIN SESSION SCRATCH RUNTIME\\n([\\s\\S]*?)^\\/\\/ END SESSION SCRATCH RUNTIME/m)[1];
const api=new Function('require',source+'\\nreturn {scratchAllocate,scratchSupervise};')(require);
const ownership=api.scratchAllocate(${JSON.stringify(cwd)},'cli-session','cli-controller');
fs.writeFileSync(${JSON.stringify(join(root, 'ownership.json'))},JSON.stringify(ownership),{mode:0o600});
api.scratchSupervise(manager,ownership,'bash',[${JSON.stringify(launchScript)},...process.argv.slice(2)]).catch(()=>{process.stderr.write('scratch composition controller failed\\n');process.exitCode=1;});
`);
        result = await execute(process.execPath, [controller, ...selectors, 'exec', '--skip-git-repo-check', '--json', '-C', cwd, 'STARTUP_ROOT'],
          { ...env, GESTALT_MANAGER_BIN: manager, STARTUP_PHASE_FILE: join(root, 'phase') }, cwd);
        scratchResult = result;
      } else result = await execute(process.execPath, ['--input-type=module', '-', ...selectors, 'exec', '--skip-git-repo-check', '--json', '-C', cwd, 'STARTUP_ROOT'], env, cwd, program);
      assert.equal(result.code, 0, result.stderr.slice(-1200)); stderr += result.stderr;
    }
    assert.deepEqual(errors, []);
    const nativeFoundation = requests[0].input[0];
    assert.equal(nativeFoundation.role, 'developer');
    const directRequests = requests.filter(input => phases.get(input) === 'direct');
    const captures = requests.filter(input => phases.get(input) === 'wrapped');
    assert.ok(captures.length);
    const instructionStructure = item => Object.fromEntries(Object.entries(item).map(([key, value]) => [key,
      { bytes: Buffer.byteLength(JSON.stringify(value)), sha256: digest(value) }]));
    t.diagnostic(JSON.stringify({ mode, nativeFoundation: instructionStructure(nativeFoundation),
      wrappedFoundation: instructionStructure(captures[0].input[0]) }));
    t.diagnostic(JSON.stringify({ mode, phases: ['direct', 'wrapped'].map(phase => ({ phase,
      roots: requests.filter(input => phases.get(input) === phase && !childRequests.get(input)).length,
      children: requests.filter(input => phases.get(input) === phase && childRequests.get(input)).length })) }));
    if (childMode) assert.ok(captures.some(input => childRequests.get(input)), 'actual native child delivery missing');
    const omitted = ['disabled-hooks', 'disabled-skills', 'excluded-skill', 'older-package', 'unsupported-package'].includes(mode);
    const expectedScalar = mode === 'explicit-cli' ? 'CLI_STARTUP_SCALAR' : mode === 'trusted-project' ? 'CLOSEST_STARTUP_SCALAR' :
      mode === 'named-profile' ? 'PROFILE_STARTUP_SCALAR' : mode === 'empty' ? undefined : 'USER_STARTUP_SCALAR';
    for (const input of captures) {
      const child = childRequests.get(input);
      const baseline = directRequests.find(value => childRequests.get(value) === child);
      assert.ok(baseline, 'equivalent direct native root/child baseline missing');
      const selectedScalar = namedRole && child ? 'ROLE_ONLY_DUTY' : expectedScalar;
      const directCustom = selectedScalar && baseline.input.find(item => item.role === 'developer' && JSON.stringify(item.content).includes(selectedScalar));
      if (selectedScalar) assert.ok(directCustom, 'direct native effective customization missing');
      const text = messageText(input);
      if (mode === 'explicit-foundation') {
        const separate = value => typeof value.instructions === 'string' ? value.instructions : '';
        const directText = messageText(baseline);
        const nativeFile = await readFile(join(home, 'foundation.md'), 'utf8');
        assert.equal(digest(nativeFile), digest('EXPLICIT_USER_FOUNDATION\n'), 'explicit foundation source changed');
        t.diagnostic(JSON.stringify({ mode, directInputCount: count(directText, 'EXPLICIT_USER_FOUNDATION'),
          directSeparateCount: count(separate(baseline), 'EXPLICIT_USER_FOUNDATION'), wrappedInputCount: count(text, 'EXPLICIT_USER_FOUNDATION'),
          wrappedSeparateCount: count(separate(input), 'EXPLICIT_USER_FOUNDATION'), sourceSha256: digest(nativeFile),
          directSeparateSha256: digest(separate(baseline)), wrappedSeparateSha256: digest(separate(input)) }));
        assert.equal(count(directText + separate(baseline), 'EXPLICIT_USER_FOUNDATION'), 1, 'direct native explicit foundation not observed');
        assert.equal(count(text + separate(input), 'EXPLICIT_USER_FOUNDATION'), 1, 'wrapped explicit foundation not observed');
      }
      if (namedRole) {
        assert.equal(count(text, 'ROOT_ONLY_DUTY'), child ? 0 : 1, 'root-only native customization leaked into selected role');
        assert.equal(count(text, 'ROLE_ONLY_DUTY'), child ? 1 : 0, 'selected role duties changed');
      }
      assert.equal(count(text, '<!-- gestalt:startup:v1 -->'), omitted ? 0 : 1, 'owned startup block multiplicity');
      assert.equal(count(text, 'GLOBAL_STARTUP_AGENTS'), 1);
      assert.equal(count(text, 'REPOSITORY_STARTUP_AGENTS'), 1);
      if (selectedScalar) {
        assert.equal(count(text, selectedScalar), 1);
        const custom = input.input.find(item => item.role === 'developer' && JSON.stringify(item.content).includes(selectedScalar));
        assert.ok(custom, 'wrapped effective customization missing');
        if (mode === 'scratch-child') t.diagnostic(JSON.stringify({ mode, directCustomSha256: digest(directCustom.content), wrappedCustomSha256: digest(custom.content),
          directCustomBytes: Buffer.byteLength(JSON.stringify(directCustom.content)), wrappedCustomBytes: Buffer.byteLength(JSON.stringify(custom.content)) }));
        if (!omitted) assert.ok(text.indexOf(selectedScalar) < text.indexOf('<!-- gestalt:startup:v1 -->'));
      }
      if (mode === 'explicit-cli') t.diagnostic(JSON.stringify({ mode, direct: instructionParts(baseline).map(part => ({ role: part.role, bytes: Buffer.byteLength(part.text), sha256: digest(part.text) })),
        wrapped: instructionParts(input).map(part => ({ role: part.role, bytes: Buffer.byteLength(part.text), sha256: digest(part.text) })) }));
      assertNativeContent(baseline, input);
      if (!omitted && mode !== 'stale-block' && !(mode === 'stale-role-block' && child)) assert.ok(text.includes(block), 'selected installed composer content absent');
      if (mode === 'stale-role-block') assert.equal(count(text, 'OLD_ROLE_BLOCK'), child ? 1 : 0, 'stale role bytes changed');
      if (!omitted && !['existing-block', 'stale-block', 'role-owned-block', 'stale-role-block'].includes(mode)) assert.ok(text.indexOf('<!-- gestalt:startup:v1 -->') < text.indexOf('<gestalt_serena_unavailable>'), 'stable/capability order changed');
      if (mode === 'stale-block') assert.equal(count(text, 'OLD_STARTUP_BLOCK'), 1);
      if (profile && mode !== 'named-profile') assert.equal(count(text, 'PROFILE_STARTUP_SCALAR'), 0, 'lower-priority scalar concatenated');
      if (mode === 'explicit-cli') assert.equal(count(text, 'CLOSEST_STARTUP_SCALAR'), 0);
    }
    if (mode === 'older-package') assert.match(stderr, /startup guidance unavailable \(missing-contract\)/);
    if (mode === 'unsupported-package') assert.match(stderr, /startup guidance unavailable \(unsupported-contract\)/);
    if (mode === 'stale-block') assert.match(stderr, /startup guidance unavailable \(migration-required\)/);
    if (mode === 'scratch-child') {
      const { context } = JSON.parse(await readFile(join(root, 'ownership.json'), 'utf8'));
      const finalMarker = JSON.parse(await readFile(context.markerPath, 'utf8'));
      const rootId = scratchResult.stdout.split('\n').filter(Boolean).map(line => JSON.parse(line)).filter(value => value.type === 'thread.started').at(-1)?.thread_id;
      const childId = captures.flatMap(input => input.input.filter(item => item.type === 'function_call_output')).map(item => {
        try { return JSON.parse(item.output).agent_id; } catch { return undefined; }
      }).find(Boolean);
      assert.ok(rootId && childId && rootId !== childId, 'native provider identities missing');
      assert.equal(finalMarker.ownerKind, 'cli-session');
      assert.deepEqual(finalMarker.providerThreads.map(value => value.threadId).sort(), [rootId, childId].sort());
      assert.equal(ownershipCaptures.length, captures.length);
      for (const capture of ownershipCaptures) {
        assert.equal(capture.context.ownerId, context.ownerId);
        assert.equal(capture.marker.ownerId, context.ownerId);
        assert.equal(capture.controllerProtected, true, 'controller protection released during model request');
        assert.ok(capture.leases.some(lease => lease.state === 'active' && lease.consumers.includes('cli-controller')));
        assert.ok(capture.marker.providerThreads.some(value => value.threadId === rootId));
      }
      assert.ok(ownershipCaptures.some(capture => capture.marker.providerThreads.some(value => value.threadId === childId)), 'child not associated during native turn');
      const container = join(workspace, 'tmp/sessions');
      assert.deepEqual((await readdir(container)).sort(), ['.gitignore', context.ownerId].sort(), 'unexpected entry or additional owner in scratch container');
      assert.ok(await readFile(join(container, '.gitignore'), 'utf8') === '*\n', 'scratch container metadata changed');
      for (const name of await readdir(join(context.directory, 'leases'))) {
        const lease = JSON.parse(await readFile(join(context.directory, 'leases', name), 'utf8'));
        assert.equal(lease.state, 'released', 'lease not released after native shutdown');
      }
      await assert.rejects(stat(join(context.directory, '.controller-lock')), { code: 'ENOENT' });
      await assert.rejects(stat(join(context.directory, '.association-lock')), { code: 'ENOENT' });
      assert.equal((await stat(context.directory)).mode & 0o777, 0o700);
    }
    assert.ok(await readFile(join(home, 'config.toml'), 'utf8') === config, 'persistent user config rewritten');
    if (profile) assert.ok(await readFile(join(home, 'fixture.config.toml'), 'utf8') === profileConfig, 'profile rewritten');
    if (namedRole) assert.ok(await readFile(join(home, 'agents/fixture-worker.toml'), 'utf8') === roleFileContents, 'native role file rewritten');
    t.diagnostic(JSON.stringify({ mode, package: process.env.GESTALT_STARTUP_FIXTURE_ROOT ? 'external-contract-fixture' : 'synthetic-contract-fixture', requests: captures.length,
      blockBytes: Buffer.byteLength(block), blockSha256: createHash('sha256').update(block).digest('hex'), blocksPerRequest: omitted ? 0 : 1 }));
  } finally { model.closeAllConnections(); await new Promise(resolve => model.close(resolve)); }
}

for (const mode of ['empty', 'custom', 'default-child', 'scratch-child', 'named-role', 'role-owned-block', 'root-owned-role-override', 'stale-role-block', 'explicit-foundation', 'repeat', 'named-profile', 'trusted-project', 'explicit-cli',
  'disabled-hooks', 'disabled-skills', 'excluded-skill', 'older-package', 'unsupported-package', 'existing-block', 'stale-block'])
  test(`selected installed startup contract: ${mode}`, { timeout: 45000 }, t => deliveredFixture(t, mode));

test('startup hook preserves stale or changed role metadata with bounded fallback', async t => {
  const root = await mkdtemp(join(tmpdir(), 'gestalt role hook policy '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, 'home'), plugin = join(root, 'plugin');
  await mkdir(join(home, 'agents'), { recursive: true });
  await packageFixture(plugin);
  const rolePath = join(home, 'agents/worker.toml');
  const stale = 'name="fixture_worker"\ndescription="Synthetic role"\ndeveloper_instructions="""\nROLE_ONLY_DUTY\n<!-- gestalt:startup:v1 -->\nOLD_ROLE_BLOCK\n<!-- /gestalt:startup -->\n"""\n';
  const body = 'async function selectedStartupHook() {' + source.split('async function selectedStartupHook() {')[1].split('\nstartupHook = await selectedStartupHook();')[0];
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const select = new AsyncFunction('common', 'state', 'rules', 'startupNotice', 'resolve', 'realpath', 'readFile', 'shQuote', 'environment', 'gestaltProfileToml', body + '\nreturn selectedStartupHook();');
  const state = { version: 'fixture/0.164.0', config: { skills: { config: [] }, developer_instructions: 'ROOT_ONLY_DUTY' }, layers: [],
    skills: [{ name: 'gestalt:coding', enabled: true, path: join(plugin, 'skills/coding/SKILL.md') }] };
  for (const changed of [false, true]) {
    await writeFile(rolePath, changed ? 'name="fixture_worker"\ndescription="Synthetic role"\ndeveloper_instructions="ROLE_ONLY_DUTY"\n' : stale);
    const command = await select(true, state, [], () => {}, resolve, realpath, readFile,
      value => "'" + value.replaceAll("'", "'\\'" + "'") + "'", { CODEX_HOME: home }, { parse });
    assert.equal(typeof command, 'string');
    if (changed) await writeFile(rolePath, (await readFile(rolePath, 'utf8')) + '# changed after discovery\n');
    const before = digest(await readFile(rolePath, 'utf8'));
    const result = await execute('bash', ['-c', command], process.env, root,
      JSON.stringify({ hook_event_name: 'SubagentStart', agent_type: 'fixture_worker' }));
    assert.equal(result.code, 0);
    assert.equal(result.stdout, '', 'fallback added a second instruction block');
    assert.ok(result.stderr.includes(changed ? '(role-config-changed)' : '(migration-required)'), 'bounded role diagnostic missing');
    assert.equal(digest(await readFile(rolePath, 'utf8')), before, 'role content mutated by hook');
    assert.ok(!result.stderr.includes('ROLE_ONLY_DUTY') && !result.stderr.includes('OLD_ROLE_BLOCK'), 'private role bytes in diagnostic');
  }
});

async function openProtocol(env, cwd) {
  const child = spawn(process.execPath, ['--input-type=module', '-', 'app-server', '--stdio'], { env, cwd, stdio: ['pipe', 'pipe', 'pipe', 'pipe'] });
  const exited = new Promise(resolve => child.once('close', resolve));
  let stderr = '', next = 0;
  child.stderr.on('data', data => stderr = (stderr + data).slice(-2000));
  const pending = new Map(), completed = new Set(), turns = new Map();
  const lines = createInterface({ input: child.stdout });
  const fail = () => { for (const waiter of [...pending.values(), ...turns.values()]) waiter.reject(Error('native lifecycle transport stopped')); };
  child.once('error', fail); child.once('close', fail);
  lines.on('line', line => {
    let message; try { message = JSON.parse(line); } catch { return; }
    const waiter = pending.get(message.id);
    if (waiter) { pending.delete(message.id); message.error ? waiter.reject(Error('native lifecycle request failed: ' + waiter.method + ' code ' + message.error.code)) : waiter.resolve(message.result); }
    if (message.method === 'turn/completed') {
      const id = message.params.turn.id; completed.add(id);
      const waiting = turns.get(id); if (waiting) { turns.delete(id); waiting.resolve(); }
    }
  });
  const bounded = (promise, label) => {
    let timer;
    return Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(Error('native lifecycle deadline: ' + label)), 15000); })]).finally(() => clearTimeout(timer));
  };
  const request = (method, params) => bounded(new Promise((resolve, reject) => {
    const id = ++next; pending.set(id, { resolve, reject, method });
    child.stdio[3].write(JSON.stringify({ id, method, params }) + '\n');
  }), method);
  child.stdin.end(program);
  const close = async () => {
    child.stdio[3].end();
    const timer = setTimeout(() => child.kill('SIGTERM'), 1000);
    const kill = setTimeout(() => child.kill('SIGKILL'), 2500);
    try { await exited; } finally { clearTimeout(timer); clearTimeout(kill); lines.close(); }
  };
  try {
    await request('initialize', { clientInfo: { name: 'startup-lifecycle-fixture', version: '1' }, capabilities: { experimentalApi: true } });
    child.stdio[3].write(JSON.stringify({ method: 'initialized', params: {} }) + '\n');
  } catch (error) { await close(); throw error; }
  return { request, close, stderr: () => stderr, turn: async (threadId, text) => {
    const { turn } = await request('turn/start', { threadId, input: [{ type: 'text', text }] });
    if (!completed.has(turn.id)) await bounded(new Promise((resolve, reject) => turns.set(turn.id, { resolve, reject })), 'turn/completed');
  } };
}

for (const legacy of [false, true]) test(`native retained startup lifecycle: ${legacy ? 'legacy thread' : 'current thread'}`, { timeout: 60000 }, async t => {
  const root = await mkdtemp(join(tmpdir(), 'gestalt retained startup '));
  t.after(() => rm(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }));
  const home = join(root, 'home'), cwd = join(root, 'workspace'), marketplace = join(root, 'marketplace'), plugin = join(marketplace, 'plugins/gestalt');
  for (const path of [home, cwd, join(marketplace, '.agents/plugins')]) await mkdir(path, { recursive: true });
  const block = await packageFixture(plugin);
  await writeFile(join(marketplace, '.agents/plugins/marketplace.json'), JSON.stringify({ name: 'startup-fixture', plugins: [{ name: 'gestalt', source: { source: 'local', path: './plugins/gestalt' } }] }));
  const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith('CONTEXT_MODE_') && !['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].includes(name)));
  Object.assign(env, { CODEX_HOME: home, GESTALT_HOME: join(root, 'managed'), GESTALT_MANAGER_BIN: join(root, 'manager') });
  await writeFile(env.GESTALT_MANAGER_BIN, '#!/bin/sh\nprintf \'{"schemaVersion":1,"status":"absent"}\\n\'\n', { mode: 0o755 });
  for (const args of [['plugin', 'marketplace', 'add', marketplace], ['plugin', 'add', 'gestalt@startup-fixture']]) {
    const result = await execute('codex', args, env, cwd); assert.equal(result.code, 0, 'isolated plugin installation failed');
  }
  const installed = join(home, 'plugins/cache/startup-fixture/gestalt/0.0.1');
  const descriptorPath = join(installed, 'gestalt-startup-contract.json');
  const descriptor = await readFile(descriptorPath, 'utf8');
  if (legacy) await rm(descriptorPath);
  const requests = [];
  const model = createServer(async (request, response) => {
    let raw = ''; for await (const data of request) raw += data;
    requests.push(JSON.parse(raw));
    const item = { id: 'msg_' + requests.length, type: 'message', status: 'completed', role: 'assistant', content: [{ type: 'output_text', text: 'Synthetic lifecycle complete.', annotations: [] }] };
    const result = { id: 'resp_' + requests.length, object: 'response', created_at: 1, status: 'completed', model: 'fixture', output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
    response.writeHead(200, { 'Content-Type': 'text/event-stream' });
    for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
      { type: 'response.output_item.added', output_index: 0, item }, { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response: result }]) response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
    response.end();
  });
  await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
  let session;
  try {
    const config = `model='fixture'\nmodel_provider='fixture'\napproval_policy='never'\ndeveloper_instructions='RETAINED_USER_SCALAR'\n[model_providers.fixture]\nname='fixture'\nbase_url='http://127.0.0.1:${model.address().port}/v1'\nwire_api='responses'\nrequires_openai_auth=false\n[features]\nhooks=true\ntool_search=false\n[plugins."gestalt@startup-fixture"]\nenabled=true\n`;
    await writeFile(join(home, 'config.toml'), config);
    const captures = [];
    const turn = async (id, label, expected) => {
      const before = requests.length; await session.turn(id, label);
      assert.ok(requests.length > before, 'native lifecycle backend turn missing');
      for (const input of requests.slice(before)) {
        const text = messageText(input);
        assert.equal(count(text, '<!-- gestalt:startup:v1 -->'), expected, 'retained startup block multiplicity');
        if (expected) assert.equal(count(text, block), 1, 'retained canonical bytes changed');
        assert.equal(count(text, 'RETAINED_USER_SCALAR'), 1);
        assert.equal(count(text, '<gestalt_xerj_capability>'), 0, 'missing provider falsely advertised');
        captures.push({ label, blocks: expected, instructionSha256: digest(instructionParts(input)) });
      }
    };
    session = await openProtocol(env, cwd);
    const started = await session.request('thread/start', { cwd });
    const id = started.thread.id;
    await turn(id, 'LIFECYCLE_FRESH', legacy ? 0 : 1);
    const forked = await session.request('thread/fork', { threadId: id });
    assert.notEqual(forked.thread.id, id);
    await turn(forked.thread.id, 'LIFECYCLE_FORK', legacy ? 0 : 1);
    await session.close(); session = undefined;
    // Reload selected package data only inside this disposable installation.
    if (legacy) await writeFile(descriptorPath, descriptor);
    else await writeFile(descriptorPath, JSON.stringify({ ...JSON.parse(descriptor), blockVersion: 2 }));
    session = await openProtocol(env, cwd);
    await session.request('thread/resume', { threadId: id });
    await turn(id, 'LIFECYCLE_RESTART_RESUME', legacy ? 0 : 1);
    await session.request('thread/resume', { threadId: id });
    await turn(id, 'LIFECYCLE_REPEAT_RESUME', legacy ? 0 : 1);
    const fresh = await session.request('thread/start', { cwd });
    await turn(fresh.thread.id, 'LIFECYCLE_RELOADED_PACKAGE', legacy ? 1 : 0);
    assert.ok(await readFile(join(home, 'config.toml'), 'utf8') === config, 'live config rewritten');
    t.diagnostic(JSON.stringify({ legacy, captures, blockSha256: digest(block), refresh: 'retained threads keep durable bytes; fresh assembly uses selected compatible package' }));
  } finally {
    if (session) await session.close();
    model.closeAllConnections(); await new Promise(resolve => model.close(resolve));
  }
});

test('synthetic older-runtime guard omits startup hooks with a bounded legacy fallback', async () => {
  const body = 'async function selectedStartupHook() {' + source.split('async function selectedStartupHook() {')[1].split('\nstartupHook = await selectedStartupHook();')[0];
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const select = new AsyncFunction('common', 'state', 'startupNotice', body + '\nreturn selectedStartupHook();');
  const notices = [];
  const result = await select(true, { version: 'fixture/0.159.0' }, reason => notices.push(reason));
  assert.equal(result, undefined);
  assert.deepEqual(notices, ['unsupported-runtime']);
});

test('native SessionStart/SubagentStart hooks add one synthetic block alongside developer and AGENTS instructions', { timeout: 45000 }, async t => {
  const root = await mkdtemp(join(tmpdir(), 'gestalt startup hook probe '));
  t.after(() => rm(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }));
  const home = join(root, 'home'), workspace = join(root, 'workspace');
  for (const path of [home, workspace]) await mkdir(path);
  const requests = [];
  let spawned = false, waited = false;
  let directSpawned = false, directWaited = false;
  const model = createServer(async (request, response) => {
    let raw = ''; for await (const data of request) raw += data;
    const input = JSON.parse(raw); requests.push(input);
    const tools = input.tools.flatMap(tool => tool.type === 'namespace' ? tool.tools.map(child => ({ ...child, namespace: tool.name })) : [tool]);
    const isChild = JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('CHILD_PROBE');
    const isDirect = JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('DIRECT_PROBE') ||
      JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('DIRECT_CHILD_PROBE');
    let item;
    if (!isChild && !(isDirect ? directSpawned : spawned)) {
      const tool = tools.find(tool => tool.name === 'spawn_agent');
      assert.ok(tool); if (isDirect) directSpawned = true; else spawned = true;
      item = { id: 'fc_spawn', type: 'function_call', status: 'completed', name: tool.name,
        ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'spawn',
        arguments: JSON.stringify({ message: isDirect ? 'DIRECT_CHILD_PROBE' : 'CHILD_PROBE', fork_context: false }) };
    } else if (!isChild && !(isDirect ? directWaited : waited)) {
      const tool = tools.find(tool => tool.name === 'wait_agent');
      const agent = input.input.filter(item => item.type === 'function_call_output').map(item => { try { return JSON.parse(item.output).agent_id; } catch { return undefined; } }).find(Boolean);
      assert.ok(tool && agent); if (isDirect) directWaited = true; else waited = true;
      item = { id: 'fc_wait', type: 'function_call', status: 'completed', name: tool.name,
        ...(tool.namespace ? { namespace: tool.namespace } : {}), call_id: 'wait', arguments: JSON.stringify({ targets: [agent], timeout_ms: 10000 }) };
    } else item = { id: `msg_${requests.length}`, type: 'message', status: 'completed', role: 'assistant', content: [{ type: 'output_text', text: 'Probe finished.', annotations: [] }] };
    const result = { id: `resp_${requests.length}`, object: 'response', created_at: 1, status: 'completed', model: 'fixture', output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
    response.writeHead(200, { 'Content-Type': 'text/event-stream' });
    for (const event of [{ type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
      { type: 'response.output_item.added', output_index: 0, item }, { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response: result }]) response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
    response.end();
  });
  await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
  try {
    await writeFile(join(home, 'AGENTS.md'), 'GLOBAL_AGENTS_PROBE');
    await writeFile(join(workspace, 'AGENTS.md'), 'PROJECT_AGENTS_PROBE');
    const config = `model='fixture'\nmodel_provider='fixture'\napproval_policy='never'\ndeveloper_instructions='DEVELOPER_PROBE'\n[model_providers.fixture]\nname='fixture'\nbase_url='http://127.0.0.1:${model.address().port}/v1'\nwire_api='responses'\nrequires_openai_auth=false\n[features]\nhooks=true\nmulti_agent=true\ntool_search=false\n`;
    await writeFile(join(home, 'config.toml'), config);
    const env = { ...process.env, CODEX_HOME: home };
    const direct = await execute('codex', ['--no-daemon', 'exec', '--skip-git-repo-check', '--json', '-C', workspace, 'DIRECT_PROBE'], env, workspace);
    assert.equal(direct.code, 0, direct.stderr.slice(-1600));
    assert.ok(requests.length > 0, 'direct native baseline request missing');
    const directCount = requests.length;
    const baseline = requests[0];
    const nativeInstructions = baseline.input.filter(item => ['system', 'developer'].includes(item.role));
    // Retain only structural evidence. The baseline bytes stay in this private
    // fixture and are compared directly with every root and child request.
    const structure = input => ({ keys: Object.keys(input).sort(), messages: input.input.filter(item => item.role).map(item => ({
      role: item.role, bytes: Buffer.byteLength(JSON.stringify(item.content)),
      sha256: createHash('sha256').update(JSON.stringify(item.content)).digest('hex'),
      customDeveloper: JSON.stringify(item.content).includes('DEVELOPER_PROBE'),
      hook: JSON.stringify(item.content).includes('HOOK_BLOCK_PROBE'),
    })) });
    t.diagnostic(JSON.stringify({ directNative: structure(baseline) }));
    assert.ok(nativeInstructions.length, 'native baseline instruction placement unknown; inspect structural evidence');
    // 0.160 can combine the native foundation and custom scalar in one
    // developer message. Preserve the entire content under that native layout.
    // The public adapter already owns and trusts its two session hooks. Probe
    // that exact path with no selected package or optional MCP provider.
    const owner = join(root, 'manager');
    // The adapter reads its manager's runtime when scratch context is inherited.
    // Keep the exact bundle inert in this readiness stub and use the real hook.
    const scratchRuntime = source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n[\s\S]*?^\/\/ END SESSION SCRATCH RUNTIME/m)?.[0];
    assert.ok(scratchRuntime, 'public manager scratch runtime missing');
    const delimiter = 'GESTALT_HOOK_PROBE_SCRATCH_RUNTIME';
    assert.ok(!scratchRuntime.split('\n').includes(delimiter));
    const managerPath = "'" + resolve('public/gestalt').replaceAll("'", "'\\''") + "'";
    await writeFile(owner, `#!/bin/sh
: <<'${delimiter}'
${scratchRuntime}
${delimiter}
if [ "\${1-}" = __scratch-hook ]; then exec ${managerPath} "$@"; fi
printf '{"schemaVersion":1,"status":"absent"}\\n'
`, { mode: 0o755 });
    // Existing hook output is generated by the public adapter. Replace only
    // its capability strings for this pre-implementation transport probe.
    const probeProgram = program.replace("capabilities.map(cap => cap.valid ? cap.context : cap.inactiveContext).join('\\n')", "'HOOK_BLOCK_PROBE'");
    const result = await execute(process.execPath, ['--input-type=module', '-', 'exec', '--skip-git-repo-check', '--json', '-C', workspace, 'ROOT_PROBE'],
      { ...env, GESTALT_HOME: join(root, 'managed'), GESTALT_MANAGER_BIN: owner }, workspace, probeProgram);
    assert.equal(result.code, 0, result.stderr.slice(-1600));
    assert.ok(requests.some(input => JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('CHILD_PROBE')), 'child turn missing');
    t.diagnostic(JSON.stringify({ wrappedNative: requests.slice(directCount).map(structure) }));
    for (const input of requests.slice(directCount)) {
      const text = JSON.stringify(input.input);
      assert.equal(text.split('HOOK_BLOCK_PROBE').length - 1, 1, 'hook output duplicated or absent');
      assert.equal(text.split('DEVELOPER_PROBE').length - 1, 1);
      assert.equal(text.split('GLOBAL_AGENTS_PROBE').length - 1, 1);
      assert.equal(text.split('PROJECT_AGENTS_PROBE').length - 1, 1);
      const child = JSON.stringify(input.input.filter(item => item.role === 'user').at(-1)).includes('CHILD_PROBE');
      const baseline = requests.slice(0, directCount).find(value => JSON.stringify(value.input.filter(item => item.role === 'user').at(-1)).includes('DIRECT_CHILD_PROBE') === child);
      assert.ok(baseline, 'equivalent native root/child baseline missing');
      for (const native of baseline.input.filter(item => ['system', 'developer'].includes(item.role))) assert.ok(input.input.some(item =>
        item.role === native.role && JSON.stringify(item.content) === JSON.stringify(native.content)),
      'direct native instruction bytes changed or disappeared');
    }
    assert.equal(await readFile(join(home, 'config.toml'), 'utf8'), config);
  } finally {
    model.closeAllConnections(); await new Promise(resolve => model.close(resolve));
  }
});
