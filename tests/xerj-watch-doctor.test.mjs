import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtemp, mkdir, writeFile, readFile, rm, symlink} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import test from 'node:test';

const manager = resolve('public/gestalt');
const source = await readFile(manager, 'utf8');
async function run(command, args, env, input) {
  const child = spawn(command, args, {env, stdio:['pipe','pipe','pipe']});
  let stdout = '', stderr = '';
  child.stdout.on('data', bytes => stdout += bytes);
  child.stderr.on('data', bytes => stderr += bytes);
  child.stdin.end(input);
  const timer = setTimeout(() => child.kill('SIGKILL'), 15000);
  try {
    const code = await new Promise((done, reject) => {child.once('error', reject); child.once('close', done);});
    return {code, stdout, stderr};
  } finally {clearTimeout(timer);}
}
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'xerj watch doctor '));
  t.after(() => rm(root, {recursive:true, force:true}));
  const workspace = join(root, 'home'), managed = join(root, 'managed');
  const codexHome = join(workspace, '.codex');
  for (const path of [codexHome, join(managed, 'xerj'), join(root, 'bin')]) await mkdir(path, {recursive:true});
  await writeFile(join(managed, 'xerj/xerj'), `#!${process.execPath}
console.log(JSON.stringify(process.argv.slice(2)));
`, {mode:0o700});
  const env = {...process.env, HOME:workspace, CODEX_HOME:codexHome, GESTALT_HOME:managed,
    PATH:join(root, 'bin')+':'+process.env.PATH};
  for (const key of ['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP','GESTALT_XERJ_AUTO_INDEX_HOME',
    'XERJ_INSTALL_DIR','XERJ_CODE_HOME','XERJ_CONFIG','XERJ_VERSION']) delete env[key];
  return {root, workspace, managed, codexHome, env};
}

test('home-root watch refuses nested journals with actionable recovery before launching XERJ', async t => {
  const f = await fixture(t);
  const result = await run('bash', [manager, 'xerj', 'autoindex', f.workspace, '--watch', '--no-graph'], f.env);
  assert.equal(result.code, 1);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /state is inside the watched root/);
  assert.match(result.stderr, /--cwd.*narrower source folder/);
  assert.match(result.stderr, /GESTALT_XERJ_AUTO_INDEX_HOME/);
});

test('explicit persistent journal home supports a home-root watch and its status reader', async t => {
  const f = await fixture(t), state = join(f.root, 'persistent journals');
  const env = {...f.env, GESTALT_XERJ_AUTO_INDEX_HOME:state};
  for (const args of [['autoindex', f.workspace, '--watch', '--no-graph'], ['autoindex', 'status', '--cwd', f.workspace]]) {
    const result = await run('bash', [manager, 'xerj', ...args], env);
    assert.equal(result.code, 0, result.stderr);
    const forwarded = JSON.parse(result.stdout);
    assert.equal(forwarded[0], 'autoindex');
    assert.equal(forwarded[1], '--state-dir');
    assert.ok(forwarded[2].startsWith(state+'/ax-'), forwarded[2]);
  }
});

test('narrow source roots keep their existing managed journal paths', async t => {
  const f = await fixture(t), workspace = join(f.workspace, 'source');
  await mkdir(workspace);
  const result = await run('bash', [manager, 'xerj', 'autoindex', workspace, '--watch', '--no-graph'], f.env);
  assert.equal(result.code, 0, result.stderr);
  const args = JSON.parse(result.stdout);
  assert.ok(args[2].startsWith(f.codexHome+'/xerj-data/autoindex/ax-'));
});

test('journal override rejects relative paths and symlink escapes', async t => {
  const f = await fixture(t);
  await symlink(f.workspace, join(f.root, 'alias'));
  for (const state of ['relative', join(f.root, 'alias/journals')]) {
    const result = await run('bash', [manager, 'xerj', 'autoindex', f.workspace, '--watch', '--no-graph'],
      {...f.env, GESTALT_XERJ_AUTO_INDEX_HOME:state});
    assert.notEqual(result.code, 0);
    assert.equal(result.stdout, '');
  }
});

const doctorProgram = source.split('run_codex_doctor() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];
for (const session of [false, true]) test(`operator doctor handles ${session ? 'present' : 'absent'} session-only variables without changing configuration`, async t => {
  const f = await fixture(t);
  await writeFile(join(f.root, 'bin/codex'), `#!${process.execPath}
console.log(JSON.stringify(process.argv.slice(2)));
`, {mode:0o700});
  const config = `[mcp_servers.context-mode]\ncommand = "node"\nargs = [${JSON.stringify(f.codexHome+'/bin/context-mode-mcp.mjs')}]\nenv_vars = ["GESTALT_SESSION_TMP_CONTEXT", "TMPDIR", "TMP", "TEMP", "REQUIRED_OPERATOR_INPUT"]\n
[mcp_servers.unrelated]\ncommand = "other"\nenv_vars = ["TMPDIR", "REQUIRED_SECRET"]\n`;
  await writeFile(join(f.codexHome, 'config.toml'), config);
  const env = {...f.env};
  if (session) Object.assign(env, {GESTALT_SESSION_TMP_CONTEXT:'present', TMPDIR:f.root, TMP:f.root, TEMP:f.root});
  const result = await run('node', ['--input-type=module', '-', manager], env, doctorProgram);
  assert.equal(result.code, 0, result.stderr);
  const args = JSON.parse(result.stdout);
  assert.deepEqual(args.slice(0, 4), ['doctor', '--summary', '--ascii', '--no-color']);
  if (session) assert.equal(args.length, 4);
  else assert.deepEqual(args.slice(4), ['-c', 'mcp_servers.context-mode.env_vars=["REQUIRED_OPERATOR_INPUT"]']);
  assert.equal(await readFile(join(f.codexHome, 'config.toml'), 'utf8'), config);
});
