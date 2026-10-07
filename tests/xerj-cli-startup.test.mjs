import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

const manager = resolve('public/gestalt');
const source = await readFile(manager, 'utf8');
const shellFunction = 'launch_cli_with_xerj() {' + source.split('launch_cli_with_xerj() {')[1].split('\ncommand_cli()')[0];
const program = source.split('launch_cli_with_xerj() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];

async function fixture(mode, args = [], ready = { schemaVersion: 1, status: 'ready', version: '1.0.0-rc.87', endpoint: 'http://127.0.0.1:9300' }) {
  const root = await mkdtemp(join(tmpdir(), 'xerj cli startup '));
  const bin = join(root, 'bin');
  const plugin = join(root, 'plugin');
  await mkdir(bin);
  await mkdir(join(plugin, '.codex-plugin'), { recursive: true });
  await mkdir(join(plugin, 'skills/xerj'), { recursive: true });
  const skillPath = join(plugin, 'skills/xerj/SKILL.md');
  await writeFile(skillPath, '---\nname: xerj\ndescription: fixture\n---\n');
  await writeFile(join(plugin, '.codex-plugin/plugin.json'), JSON.stringify({ name: mode === 'incompatible' ? 'other' : 'gestalt', skills: './skills/' }));
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
      result = { config: { features: { hooks: true }, hooks: { state: { 'unrelated.hook': { enabled: false, trusted_hash: 'sha256:kept' } } }, mcp_servers: process.env.FIXTURE_MODE === 'config-conflict' ? { 'gestalt-xerj': { env: { XERJ_AUTH: 'wrong-key' } } } : process.env.FIXTURE_MODE === 'aliases' ? { 'old-xerj': { command: process.env.GESTALT_MANAGER_BIN, args: ['xerj', 'mcp'] }, 'unrelated-secret': { command: 'node', enabled: false, env: { TOKEN: 'do-not-expose-secret' } } } : {}, skills: { config: [{ path: process.env.FIXTURE_SKILL, enabled: false }, { name: 'optional', enabled: false }] } } };
    }
    if (request.method === 'hooks/list') {
      const hooks = args.filter(arg => arg.startsWith('hooks.SessionStart=') || arg.startsWith('hooks.SubagentStart=')).map((arg, index) => ({
        source: 'sessionFlags', command: JSON.parse(arg.match(/"command"=("(?:[^"\\\\]|\\\\.)*")/)[1]), key: 'fixture-' + index, currentHash: 'sha256:fixture' }));
      result = { data: [{ cwd: request.params.cwds[0], hooks: process.env.FIXTURE_MODE === 'hooks-unavailable' ? [] : hooks }] };
    }
    if (request.method === 'config/read') result.layers = [{ name: { type: 'sessionFlags' }, config: { skills: result.config.skills, hooks: result.config.hooks } }, { name: { type: 'user' }, config: { features: { hooks: true } } }];
    if (request.method === 'skills/list') result = { data: [{ cwd: request.params.cwds[0], skills: [{ name: 'gestalt:xerj', enabled: false, path: process.env.FIXTURE_SKILL }] }] };
    console.log(JSON.stringify({ id: request.id, result }));
  });
} else {
  fs.appendFileSync(root + '/launch.jsonl', JSON.stringify(args) + '\\n');
  if (process.env.FIXTURE_MODE === 'signal-passthrough') {
    fs.writeFileSync(root + '/runtime-pid', String(process.pid));
    setInterval(() => {}, 1000);
    process.on('SIGTERM', () => { fs.writeFileSync(root + '/signal', 'SIGTERM'); process.exit(0); });
  }
  if (process.env.FIXTURE_MODE === 'connection-failure' && args.some(arg => arg.includes('required"=true'))) {
    process.stderr.write('Error: required MCP servers failed to initialize: gestalt-xerj: failed connection\\n');
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
require('node:fs').appendFileSync(process.env.FIXTURE_ROOT + '/ensure.jsonl', JSON.stringify(process.argv.slice(2)) + '\\n');
${mode === 'timeout' ? 'setTimeout(() => console.log(process.env.FIXTURE_READY), 5000);' : 'console.log(process.env.FIXTURE_READY);'}
`, { mode: 0o755 });
  await writeFile(join(root, 'private.config.toml'), '[features]\nhooks=true\n');
  const viaShell = mode === 'signal-passthrough';
  const command = viaShell ? 'bash' : process.execPath;
  const launchArgs = viaShell ? ['-c', 'codex_home=$FIXTURE_ROOT\ngestalt_home=$FIXTURE_ROOT\nmanager_target=$GESTALT_MANAGER_BIN\n' + shellFunction + '\nlaunch_cli_with_xerj \"$@\"', '--', ...args] : ['--input-type=module', '-', ...args];
  const started = Date.now();
  const child = spawn(command, launchArgs, { env: {
    ...process.env, PATH: `${bin}:${process.env.PATH}`, GESTALT_MANAGER_BIN: owner,
    CODEX_HOME: root, FIXTURE_ROOT: root, FIXTURE_SKILL: skillPath, FIXTURE_MODE: mode,
    FIXTURE_READY: JSON.stringify(ready), XERJ_READY_TIMEOUT_MS: mode === 'timeout' ? '100' : mode.endsWith('-deadline') ? '600' : mode === 'hook-budget' ? '1500' : '5000',
    XERJ_API_KEY: 'do-not-expose-secret', GESTALT_XERJ_READY: 'stale-hint',
  }, stdio: ['pipe', 'pipe', 'pipe', 'pipe'] });
  let stderr = '';
  child.stderr.on('data', data => stderr += data);
  child.stdout.resume();
  child.stdio[3].end(viaShell ? undefined : 'preserved stdin\n');
  child.stdin.end(viaShell ? 'preserved stdin\n' : program);
  const signaling = viaShell ? setInterval(async () => {
    try { await readFile(join(root, 'runtime-pid')); clearInterval(signaling); child.kill('SIGTERM'); } catch { /* Await native runtime startup. */ }
  }, 30) : undefined;
  const code = await new Promise(resolve => child.once('exit', (code, signal) => resolve(code ?? (signal === 'SIGTERM' ? 143 : 130))));
  if (signaling) clearInterval(signaling);
  const launches = (await readFile(join(root, 'launch.jsonl'), 'utf8')).trim().split('\n').map(JSON.parse);
  return { root, code, launches, stderr, skillPath, elapsed: Date.now() - started };
}

for (const mode of ['healthy', 'absent', 'bad-auth', 'timeout', 'incompatible', 'config-conflict', 'connection-failure', 'hooks-unavailable']) {
  test(`CLI ${mode}: tools and conditional skill use the same state`, async () => {
    const ready = mode === 'absent' ? { schemaVersion: 1, status: 'absent' }
      : mode === 'bad-auth' ? { schemaVersion: 1, status: 'unavailable', reason: 'authentication-failed' } : undefined;
    const result = await fixture(mode, ['resume', '--last', '-c', 'unrelated=true', '--', 'prompt with spaces'], ready);
    try {
      assert.equal(result.code, 0);
      assert.equal(result.launches.length, mode === 'connection-failure' ? 2 : 1);
      const launch = result.launches.at(-1);
      assert.deepEqual(launch.slice(0, 5), ['resume', '--last', '-c', 'unrelated=true', '-c']);
      assert.deepEqual(launch.slice(-2), ['--', 'prompt with spaces']);
      assert.equal(await readFile(join(result.root, 'input'), 'utf8'), 'preserved stdin\n');
      const value = launch.find(arg => arg.startsWith('skills.config='));
      assert.match(value, /"optional".*enabled"=false/);
      assert.match(value, new RegExp(`"name"="gestalt:xerj","enabled"=${mode === 'healthy'}`));
      if (mode === 'healthy') {
        assert.ok(launch.some(arg => arg.startsWith('hooks.state=') && arg.includes('sha256:kept')));
        assert.ok(launch.some(arg => arg.includes('required"=true')));
        assert.ok(launch.some(arg => arg.includes('startup_readiness"="connection"')));
        assert.ok(launch.some(arg => arg.includes('env_vars"=["GESTALT_HOME","XERJ_API_KEY","XERJ_AUTH"]')));
      } else assert.ok(launch.some(arg => arg.startsWith('mcp_servers.gestalt-xerj=') && arg.includes('enabled"=false')));
      assert.ok(!JSON.stringify(launch).includes('do-not-expose-secret'));
      assert.ok(!result.stderr.includes('do-not-expose-secret'));
    } finally { await rm(result.root, { recursive: true, force: true }); }
  });
}

test('CLI protected overrides follow user config; cwd/profile go to native config discovery', async () => {
  const args = ['-C', tmpdir(), '--profile', 'private', '-c', 'mcp_servers.gestalt-xerj.enabled=false', 'question'];
  const result = await fixture('healthy', args);
  try {
    assert.equal(result.code, 0);
    assert.deepEqual(result.launches[0].slice(0, args.length), args);
    const read = JSON.parse(await readFile(join(result.root, 'read.json'), 'utf8'));
    assert.equal(read.params.cwd, tmpdir());
    assert.ok(!read.args.some(arg => arg.startsWith('profile=')), 'v2 profile must not be treated as legacy profile');
    assert.ok(result.launches[0].slice(args.length).some(arg => arg.startsWith('mcp_servers.gestalt-xerj=')));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

test('existing managed proxy alias is disabled without changing unrelated MCP entries', async () => {
  const result = await fixture('aliases');
  try {
    assert.equal(result.code, 0);
    assert.ok(result.launches[0].some(arg => arg.startsWith('mcp_servers=') && arg.includes('\"old-xerj\"=') && arg.includes('\"enabled\"=false')));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});


test('unavailable native snapshot preserves nested CLI skill selectors', async () => {
  const result = await fixture('config-unavailable', ['-c', 'skills={config=[{name="optional",enabled=false}]}']);
  try {
    assert.equal(result.code, 0);
    const rule = result.launches[0].find(arg => arg.startsWith('skills.config='));
    assert.match(rule, /"optional","enabled"=false/);
    assert.match(rule, /"gestalt:xerj","enabled"=false/);
    assert.ok(!result.launches[0].some(arg => arg.includes('required"=true')));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});


test('attached profile, cwd and feature arguments keep native semantics', async () => {
  const args = ['-pprivate', '-C' + tmpdir(), '--enable=hooks', 'question'];
  const result = await fixture('healthy', args);
  try {
    assert.equal(result.code, 0);
    assert.deepEqual(result.launches[0].slice(0, args.length), args);
    const read = JSON.parse(await readFile(join(result.root, 'read.json'), 'utf8'));
    assert.equal(read.params.cwd, tmpdir());
    assert.ok(read.args.includes('features.hooks=true'));
    assert.ok(result.launches[0].some(arg => arg.includes('required"=true')));
  } finally { await rm(result.root, { recursive: true, force: true }); }
});


test('actual Bash launcher forwards SIGTERM and preserves interruption exit code', async () => {
  const result = await fixture('signal-passthrough');
  try {
    assert.equal(result.code, 143);
    assert.equal(await readFile(join(result.root, 'signal'), 'utf8'), 'SIGTERM');
    const pid = Number(await readFile(join(result.root, 'runtime-pid'), 'utf8'));
    assert.throws(() => process.kill(pid, 0), /ESRCH/, 'Codex child retained after interruption');
  } finally { await rm(result.root, { recursive: true, force: true }); }
});

for (const mode of ['discovery-deadline', 'hooks-deadline']) {
  test(`CLI ${mode}: shared deadline bounds discovery and disables capability`, async () => {
    const result = await fixture(mode);
    try {
      assert.equal(result.code, 0);
      assert.ok(result.elapsed < 1400, `600ms budget exceeded: ${result.elapsed}ms`);
      assert.equal(result.launches.length, 1);
      const launch = result.launches[0];
      assert.ok(launch.some(arg => arg.startsWith('mcp_servers.gestalt-xerj=') && arg.includes('enabled"=false')));
      assert.match(launch.find(arg => arg.startsWith('skills.config=')), /"gestalt:xerj","?enabled"?=false/);
      assert.ok(!launch.some(arg => arg.includes('required"=true')));
      assert.ok(!launch.some(arg => arg.includes('<gestalt_xerj_capability>')));
    } finally { await rm(result.root, { recursive: true, force: true }); }
  });
}

test('CLI recomputes MCP startup allowance after hook discovery', async () => {
  const result = await fixture('hook-budget');
  try {
    assert.equal(result.code, 0);
    const config = result.launches[0].find(arg => arg.startsWith('mcp_servers.gestalt-xerj='));
    assert.ok(config.includes('required"=true'));
    const allowance = Number(config.match(/"startup_timeout_ms"=(\d+)/)[1]);
    assert.ok(allowance > 0 && allowance < 1100, `hook time omitted from remaining budget: ${allowance}`);
  } finally { await rm(result.root, { recursive: true, force: true }); }
});
