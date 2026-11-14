import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { managerStubScratchRuntime } from './manager-stub-fixture.mjs';

const manager = resolve('public/gestalt');
const source = await readFile(manager, 'utf8');
const shellFunction = 'launch_cli_with_capabilities() {' + source.split('launch_cli_with_capabilities() {')[1].split('\ncommand_cli()')[0];
const program = source.split('launch_cli_with_capabilities() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];

async function fixture(mode, args = []) {
  const root = await mkdtemp(join(tmpdir(), 'capability cli startup '));
  const bin = join(root, 'bin');
  const plugin = join(root, 'plugin');
  await mkdir(bin);
  await mkdir(join(plugin, '.codex-plugin'), { recursive: true });
  await mkdir(join(plugin, 'skills/kept'), { recursive: true });
  const skillPath = join(plugin, 'skills/kept/SKILL.md');
  await writeFile(skillPath, '---\nname: kept\ndescription: fixture\n---\n');
  await writeFile(join(plugin, '.codex-plugin/plugin.json'), JSON.stringify({ name: mode === 'incompatible' ? 'other' : 'gestalt', skills: './skills/' }));
  const serena = mode.startsWith('serena-');
  const serenaPath = join(plugin, 'skills/serena/SKILL.md');
  if (serena) {
    await mkdir(join(plugin, 'skills/serena'), { recursive: true });
    await writeFile(serenaPath, '---\nname: serena\ndescription: fixture\n---\nSemantic project work.\n');
    if (mode !== 'serena-absent') {
      await mkdir(join(root, 'serena'));
      await writeFile(join(root, 'serena/active.json'), JSON.stringify({ schemaVersion: 1, contractVersion: 1, version: '1.7.0',
        executable: process.execPath, python: process.execPath, uv: process.execPath,
        tools: ['get_symbols_overview','find_symbol','initial_instructions','replace_symbol_body'].map(name =>
          ({ name, inputSchema: { type: 'object' } })) }));
    }
  }
  const codex = join(bin, 'codex');
  await writeFile(codex, `#!/usr/bin/env node
const fs = require('node:fs');
const { createInterface } = require('node:readline');
const args = process.argv.slice(2);
const root = process.env.FIXTURE_ROOT;
if (args[0] === 'app-server') {
  createInterface({ input: process.stdin }).on('line', async line => {
    const request = JSON.parse(line);
    if (!request.id) return;
    if ((process.env.FIXTURE_MODE === 'discovery-deadline' && request.method === 'config/read') ||
        (process.env.FIXTURE_MODE === 'hooks-deadline' && request.method === 'hooks/list')) {
      await new Promise(resolve => setTimeout(resolve, 2000));
    }
    if (process.env.FIXTURE_MODE === 'hook-budget' && request.method === 'hooks/list') {
      await new Promise(resolve => setTimeout(resolve, 400));
    }
    let result = {};
    if (request.method === 'initialize') result = { userAgent: 'codex_cli_rs/0.160.0' };
    if (request.method === 'config/read' && process.env.FIXTURE_MODE === 'config-unavailable') {
      console.log(JSON.stringify({ id: request.id, error: { code: -1, message: 'optional snapshot unavailable' } })); return;
    }
    if (request.method === 'config/read') {
      fs.writeFileSync(root + '/read.json', JSON.stringify({ args, params: request.params }));
      result = { config: { approval_policy: args.includes('approval_policy="never"') ? 'never' : 'on-request', features: { hooks: true }, hooks: { state: { 'unrelated.hook': { enabled: false, trusted_hash: 'sha256:kept' } } }, mcp_servers: process.env.FIXTURE_MODE === 'aliases' ? { 'old-serena': { command: process.env.GESTALT_MANAGER_BIN, args: ['serena', 'mcp'] }, 'unrelated-secret': { command: 'node', enabled: false, env: { TOKEN: 'do-not-expose-secret' } } } : {}, skills: { config: [{ path: process.env.FIXTURE_SKILL, enabled: false }, { name: 'optional', enabled: false }] } } };
    }
    if (request.method === 'hooks/list') {
      const hooks = args.filter(arg => arg.startsWith('hooks.SessionStart=') || arg.startsWith('hooks.SubagentStart=')).map((arg, index) => ({
        source: 'sessionFlags', command: JSON.parse(arg.match(/"command"=("(?:[^"\\\\]|\\\\.)*")/)[1]), key: 'fixture-' + index, currentHash: 'sha256:fixture' }));
      result = { data: [{ cwd: request.params.cwds[0], hooks: process.env.FIXTURE_MODE === 'hooks-unavailable' ? [] : hooks }] };
    }
    if (request.method === 'config/read') result.layers = [{ name: { type: 'sessionFlags' }, config: { skills: result.config.skills, hooks: result.config.hooks } }, { name: { type: 'user' }, config: { features: { hooks: true } } }];
    if (request.method === 'config/read' && process.env.FIXTURE_MODE === 'serena-conflict') result.config.mcp_servers['gestalt-serena'] = { env: { TOKEN: 'do-not-expose-secret' } };
    if (request.method === 'skills/list') result = { data: [{ cwd: request.params.cwds[0], skills: [{ name: 'gestalt:kept', enabled: false, path: process.env.FIXTURE_SKILL }, ...(process.env.FIXTURE_SERENA ? [{ name: 'gestalt:serena', enabled: process.env.FIXTURE_MODE !== 'serena-excluded', path: process.env.FIXTURE_SERENA }] : [])] }] };
    console.log(JSON.stringify({ id: request.id, result }));
  });
} else {
  fs.appendFileSync(root + '/launch.jsonl', JSON.stringify(args) + '\\n');
  if (process.env.FIXTURE_MODE === 'signal-passthrough') {
    fs.writeFileSync(root + '/runtime-pid', String(process.pid));
    setInterval(() => {}, 1000);
    process.on('SIGTERM', () => { fs.writeFileSync(root + '/signal', 'SIGTERM'); process.exit(0); });
  }
  if (process.env.FIXTURE_MODE === 'serena-connection-failure' && args.some(arg => arg.startsWith('mcp_servers.gestalt-serena=') && arg.includes('required\"=true'))) {
    process.stderr.write('unrelated-mcp: earlier diagnostic\\n');
    process.stderr.write('Error: required MCP servers failed to initialize: gestalt-serena: failed connection\\n');
    process.exitCode = 1;
  } else {
    let input = '';
    process.stdin.on('data', data => input += data);
    process.stdin.on('end', () => fs.writeFileSync(root + '/input', input));
  }
}
`, { mode: 0o755 });
  const owner = join(bin, 'manager');
  await writeFile(owner, `#!/usr/bin/env node
${managerStubScratchRuntime(source)}
require('node:fs').appendFileSync(process.env.FIXTURE_ROOT + '/ensure.jsonl', JSON.stringify(process.argv.slice(2)) + '\\n');
process.exit(1);
`, { mode: 0o755 });
  await writeFile(join(root, 'private.config.toml'), '[features]\nhooks=true\n');
  const viaShell = mode === 'signal-passthrough';
  const command = viaShell ? 'bash' : process.execPath;
  const launchArgs = viaShell ? ['-c', 'codex_home=$FIXTURE_ROOT\ngestalt_home=$FIXTURE_ROOT\nmanager_target=$GESTALT_MANAGER_BIN\n' + shellFunction + '\nlaunch_cli_with_capabilities \"$@\"', '--', ...args] : ['--input-type=module', '-', ...args];
  const started = Date.now();
  const child = spawn(command, launchArgs, { env: {
    ...process.env, PATH: `${bin}:${process.env.PATH}`, GESTALT_MANAGER_BIN: owner,
    GESTALT_HOME: root, FIXTURE_SERENA: serena ? serenaPath : '', CODEX_HOME: root, FIXTURE_ROOT: root, FIXTURE_SKILL: skillPath, FIXTURE_MODE: mode,
  }, stdio: ['pipe', 'pipe', 'pipe', 'pipe'] });
  let stderr = '';
  child.stderr.on('data', data => stderr += data);
  child.stdout.resume();
  child.stdio[3].end(viaShell ? undefined : 'preserved stdin\n');
  child.stdin.end(viaShell ? 'preserved stdin\n' : program.replace('const budget = 5000;', `const budget = ${mode.endsWith('-deadline') ? 600 : 5000};`));
  const signaling = viaShell ? setInterval(async () => {
    try { await readFile(join(root, 'runtime-pid')); clearInterval(signaling); child.kill('SIGTERM'); } catch { /* Await native runtime startup. */ }
  }, 30) : undefined;
  const code = await new Promise(resolve => child.once('exit', (code, signal) => resolve(code ?? (signal === 'SIGTERM' ? 143 : 130))));
  if (signaling) clearInterval(signaling);
  const launches = (await readFile(join(root, 'launch.jsonl'), 'utf8')).trim().split('\n').map(JSON.parse);
  return { root, code, launches, stderr, skillPath, elapsed: Date.now() - started };
}

for (const mode of ['healthy', 'config-unavailable', 'hooks-unavailable', 'incompatible']) {
  test(`CLI ${mode}: native input, arguments, selectors and approvals survive without retrieval`, async () => {
    const args = ['resume', '--last', '-c', 'approval_policy="never"', '--', 'prompt with spaces'];
    const result = await fixture(mode, args);
    try {
      assert.equal(result.code, 0, result.stderr);
      assert.equal(result.launches.length, 1);
      const launch = result.launches[0];
      assert.deepEqual(launch.slice(0, 5), ['--no-daemon', ...args.slice(0, 4)]);
      assert.deepEqual(launch.slice(-2), ['--', 'prompt with spaces']);
      assert.equal(await readFile(join(result.root, 'input'), 'utf8'), 'preserved stdin\n');
      if (mode !== 'config-unavailable') assert.match(launch.find(arg => arg.startsWith('skills.config=')), /"optional".*enabled"=false/);
      assert.ok(!JSON.stringify(launch).includes('do-not-expose-secret'));
      assert.ok(!JSON.stringify(launch).toLowerCase().includes('xerj'));
      assert.ok(!result.stderr.toLowerCase().includes('xerj'));
      await assert.rejects(readFile(join(result.root, 'ensure.jsonl')), { code: 'ENOENT' });
    } finally { await rm(result.root, { recursive: true, force: true }); }
  });
}

for (const args of [['--no-daemon'], ['--remote', 'ws://127.0.0.1:4321']]) test(`native launch mode ${args[0]} stays intact`, async () => {
  const result = await fixture('healthy', args);
  try {
    assert.equal(result.code, 0);
    assert.equal(result.launches[0].filter(arg => arg === '--no-daemon').length, args[0] === '--remote' ? 0 : 1);
    for (const arg of args) assert.ok(result.launches[0].includes(arg));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

test('cwd/profile and explicit scalar keep native parse scope without generated developer overrides', async () => {
  const args = ['-C', tmpdir(), '--profile', 'private', '-c', 'developer_instructions="USER"', 'question'];
  const result = await fixture('healthy', args);
  try {
    assert.equal(result.code, 0, result.stderr);
    const launch = result.launches[0];
    assert.deepEqual(launch.slice(1, args.length), args.slice(0, -1));
    assert.equal(launch.at(-1), 'question');
    assert.equal(launch.filter(arg => arg.startsWith('developer_instructions=')).length, 1);
    const read = JSON.parse(await readFile(join(result.root, 'read.json'), 'utf8'));
    assert.equal(read.params.cwd, tmpdir());
    assert.ok(!read.args.some(arg => arg.startsWith('profile=')));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

test('managed Serena alias is disabled while unrelated credentials remain private', async () => {
  const result = await fixture('aliases');
  try {
    assert.equal(result.code, 0);
    assert.ok(result.launches[0].some(arg => arg.startsWith('mcp_servers=') && arg.includes('old-serena') && arg.includes('"enabled"=false')));
    assert.ok(!JSON.stringify(result.launches).includes('do-not-expose-secret'));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

test('unavailable native snapshot preserves nested CLI skill selectors', async () => {
  const result = await fixture('config-unavailable', ['-c', 'skills={config=[{name="optional",enabled=false}]}']);
  try {
    assert.equal(result.code, 0);
    assert.match(result.launches[0].find(arg => arg.startsWith('skills.config=')), /"optional","enabled"=false/);
    assert.ok(!result.launches[0].some(arg => arg.includes('required"=true')));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

test('actual Bash launcher forwards SIGTERM and reaps its native child', async () => {
  const result = await fixture('signal-passthrough');
  try {
    assert.equal(result.code, 143);
    assert.equal(await readFile(join(result.root, 'signal'), 'utf8'), 'SIGTERM');
    const pid = Number(await readFile(join(result.root, 'runtime-pid'), 'utf8'));
    assert.throws(() => process.kill(pid, 0), /ESRCH/);
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

for (const mode of ['discovery-deadline', 'hooks-deadline']) test(`${mode} bounds optional native discovery`, async () => {
  const result = await fixture(mode);
  try {
    assert.equal(result.code, 0);
    assert.ok(result.elapsed < 1400, `600ms budget exceeded: ${result.elapsed}ms`);
    assert.equal(result.launches.length, 1);
    assert.ok(!result.launches[0].some(arg => arg.includes('required"=true')));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

for (const mode of ['serena-healthy', 'serena-absent', 'serena-excluded', 'serena-conflict', 'serena-connection-failure']) {
  test(`CLI ${mode}: retained workspace capability and approval policy`, async () => {
    const result = await fixture(mode, ['exec', '-C', '/tmp', '-c', 'approval_policy="never"', '--', 'prompt']);
    try {
      assert.equal(result.code, 0, result.stderr);
      assert.equal(result.launches.length, mode === 'serena-connection-failure' ? 2 : 1);
      const launch = result.launches.at(-1), ready = mode === 'serena-healthy';
      const config = launch.find(arg => arg.startsWith('mcp_servers.gestalt-serena='));
      assert.ok(config.includes(`"enabled"=${ready}`), config);
      if (ready) {
        assert.match(config, /"args"=\["serena","mcp","--cwd","\/tmp"\]/);
        assert.ok(config.includes('"default_tools_approval_mode"="approve"'));
      }
      assert.ok(launch.includes('approval_policy="never"'));
      assert.ok(!JSON.stringify(launch).includes('do-not-expose-secret'));
      const hooks = launch.filter(arg => arg.startsWith('hooks.SessionStart=') || arg.startsWith('hooks.SubagentStart='));
      assert.equal(hooks.length, 2);
      assert.equal(hooks.some(arg => arg.includes('<gestalt_serena_capability>')), ready);
    } finally { await rm(result.root, { recursive: true, force: true }); }
  });
}
