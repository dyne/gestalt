import assert from 'node:assert/strict';
import {spawn, spawnSync} from 'node:child_process';
import {createRequire} from 'node:module';
import {mkdtemp, mkdir, writeFile, readFile, rm, symlink, chmod, readdir, link, stat} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {createInterface} from 'node:readline';
import test from 'node:test';
import {sessionScratch} from './session-scratch-fixture.mjs';

const manager = resolve('public/gestalt');
const source = await readFile(manager, 'utf8');
const runtime = source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const {scratchEnvironment, scratchDiscoveryEnvironment} = new Function('require', runtime +
  '\nreturn {scratchEnvironment,scratchDiscoveryEnvironment};')(createRequire(import.meta.url));
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt scratch '));
  t.after(() => rm(root, {recursive: true, force: true}));
  const workspace = join(root, 'selected workspace'), nested = join(workspace, 'nested repository');
  await mkdir(nested, {recursive: true});
  return {root, workspace, nested, ...await sessionScratch(workspace)};
}
const cleanEnvironment = () => scratchDiscoveryEnvironment(process.env);

test('validated context repairs hostile temp variables and discovery scrubs only owned values', async t => {
  const f = await fixture(t), input = {...cleanEnvironment(), ...f.environment, TMPDIR: '/forbidden', TMP: '/foreign', TEMP: '/other'};
  const output = scratchEnvironment(input);
  for (const key of ['TMPDIR', 'TMP', 'TEMP']) assert.equal(output[key], f.context.directory);
  assert.equal(output.GESTALT_SESSION_TMP_CONTEXT, f.environment.GESTALT_SESSION_TMP_CONTEXT);
  assert.equal(input.TMPDIR, '/forbidden');
  const discovery = scratchDiscoveryEnvironment(output);
  for (const key of Object.keys(f.environment)) assert.equal(discovery[key], undefined);
  const operator = {TMPDIR: '/operator executable temp', TMP: '/operator alternate', TEMP: ''};
  assert.deepEqual(scratchEnvironment(operator), operator);
  assert.deepEqual(scratchDiscoveryEnvironment(operator), operator);
});

for (const mode of [0o755, 0o775]) test(`native dispatch accepts Mobile context with existing ${mode.toString(8)} tmp and preserves user files and rules`, async t => {
  const root = await mkdtemp(join(tmpdir(), 'gestalt existing tmp '));
  t.after(() => rm(root, {recursive: true, force: true}));
  const workspace = join(root, 'workspace'), parent = join(workspace, 'tmp');
  await mkdir(parent, {recursive: true}); await chmod(parent, mode);
  await writeFile(join(parent, 'user-scratch'), 'user owned');
  await writeFile(join(parent, '.gitignore'), 'keep-this-rule\n');
  const scratch = await sessionScratch(workspace, 'existing-tmp-session');
  const native = join(root, 'native-codex');
  await writeFile(native, `#!${process.execPath}
const {createInterface}=require('node:readline');
createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);console.log(JSON.stringify({id:m.id,result:{environment:Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'].map(key=>[key,process.env[key]]))}}));});`, {mode: 0o700});
  const child = spawn('bash', [manager, '__codex-app-server', native], {cwd: workspace,
    env: {...cleanEnvironment(), ...scratch.environment, TMPDIR: '/hostile', TMP: '/hostile', TEMP: '/hostile'},
    stdio: ['pipe', 'pipe', 'pipe']});
  const exited = new Promise(resolveExit => child.once('exit', resolveExit));
  const lines = createInterface({input: child.stdout}); let stderr = '';
  child.stderr.on('data', part => stderr += part);
  t.after(async () => {child.stdin.end(); await exited; lines.close();});
  const response = new Promise((resolveResponse, reject) => {
    const timer = setTimeout(() => reject(Error('native compatibility deadline: '+stderr)), 5000);
    lines.once('line', line => {clearTimeout(timer); resolveResponse(JSON.parse(line).result);});
    child.once('exit', () => {clearTimeout(timer); reject(Error('native compatibility exited: '+stderr));});
  });
  child.stdin.write(JSON.stringify({id: 1, method: 'initialize'})+'\n');
  assert.deepEqual((await response).environment, scratch.environment);
  child.stdin.end(); assert.equal(await exited, 0, stderr);
  assert.equal((await stat(parent)).mode & 0o777, mode);
  assert.equal(await readFile(join(parent, 'user-scratch'), 'utf8'), 'user owned');
  assert.equal(await readFile(join(parent, '.gitignore'), 'utf8'), 'keep-this-rule\n');
  assert.equal((await stat(join(parent, 'sessions'))).mode & 0o777, 0o700);
  assert.equal((await stat(scratch.context.directory)).mode & 0o777, 0o700);
});

for (const invalid of ['empty', 'version', 'traversal', 'foreign-marker', 'unprivate', 'symlink-owner', 'symlink-marker', 'hardlink-marker', 'symlink-workspace'])
test('rejects '+invalid+' context before launcher state creation', async t => {
  const f = await fixture(t), context = {...f.context};
  let value;
  if (invalid === 'version') context.schemaVersion = 2;
  if (invalid === 'traversal') context.directory += '/../escape';
  if (invalid === 'foreign-marker') await writeFile(context.markerPath, JSON.stringify({...f.marker, sessionId: 'foreign'}));
  if (invalid === 'unprivate') await chmod(context.directory, 0o755);
  if (invalid === 'symlink-owner') {
    const moved = join(f.root, 'moved');
    await import('node:fs/promises').then(fs => fs.rename(context.directory, moved));
    await symlink(moved, context.directory);
  }
  if (invalid === 'symlink-marker' || invalid === 'hardlink-marker') {
    const copy = join(f.root, 'marker'); await writeFile(copy, JSON.stringify(f.marker), {mode: 0o600});
    await rm(context.markerPath);
    await (invalid === 'symlink-marker' ? symlink(copy, context.markerPath) : link(copy, context.markerPath));
  }
  if (invalid === 'symlink-workspace') {
    const alias = join(f.root, 'alias'); await symlink(f.workspace, alias);
    context.workspacePath = alias;
    context.directory = join(alias, 'tmp/sessions', context.ownerId);
    context.markerPath = join(context.directory, '.gestalt-owner.json');
  }
  value = invalid === 'empty' ? '' : JSON.stringify(context);
  const home = join(f.root, 'untouched');
  const result = spawnSync('bash', [manager, 'playwright', 'install'], {encoding: 'utf8',
    env: {...cleanEnvironment(), GESTALT_HOME: home, GESTALT_SESSION_TMP_CONTEXT: value}, timeout: 5000});
  assert.equal(result.status, 1, result.stderr);
  assert.match(result.stderr, /temporary context/);
  assert.ok(!(await readdir(f.root)).includes('untouched'));
  assert.ok(!result.stderr.includes(context.directory));
});

test('native proxy descendants use selected-root scratch across nested cwd, restart and simultaneous sessions', async t => {
  const f = await fixture(t), other = await sessionScratch(f.workspace, 'second-session');
  const native = join(f.root, 'native-codex');
  await writeFile(native, `#!${process.execPath}
const {createInterface}=require('node:readline'),{spawnSync}=require('node:child_process');
createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);
const shell=spawnSync('bash',['-c','mktemp -d'],{encoding:'utf8'});
const node=spawnSync(process.execPath,['-e',"console.log(require('node:fs').mkdtempSync(require('node:path').join(require('node:os').tmpdir(),'node-')))"],{encoding:'utf8'});
const python=spawnSync('python3',['-c','import tempfile; print(tempfile.mkdtemp())'],{encoding:'utf8'});
console.log(JSON.stringify({id:m.id,result:{environment:Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'].map(k=>[k,process.env[k]])),shell:shell.stdout.trim(),node:node.stdout.trim(),python:python.stdout.trim(),codes:[shell.status,node.status,python.status]}}));});`, {mode: 0o700});
  async function start(owner) {
    const child = spawn('bash', [manager, '__codex-app-server', native], {cwd: f.nested,
      env: {...cleanEnvironment(), ...owner.environment, TMPDIR: '/hostile', TMP: '/hostile', TEMP: '/hostile'}, stdio: ['pipe', 'pipe', 'pipe']});
    const exited = new Promise(resolveExit => child.once('exit', resolveExit));
    const lines = createInterface({input: child.stdout}); let stderr = '';
    child.stderr.on('data', part => stderr += part);
    t.after(async () => {child.stdin.end(); await exited; lines.close();});
    const response = new Promise((resolveResponse, reject) => {
      const timer = setTimeout(() => reject(Error('native proxy deadline: '+stderr)), 5000);
      lines.once('line', line => {clearTimeout(timer); resolveResponse(JSON.parse(line).result);});
      child.once('exit', () => {clearTimeout(timer); reject(Error('native proxy exited: '+stderr));});
    });
    child.stdin.write(JSON.stringify({id: 1, method: 'initialize'})+'\n');
    return {child, exited, result: await response};
  }
  const [one, two] = await Promise.all([start(f), start(other)]);
  for (const [instance, owner] of [[one, f], [two, other]]) {
    assert.deepEqual(instance.result.environment, owner.environment);
    assert.deepEqual(instance.result.codes, [0, 0, 0]);
    for (const key of ['shell', 'node', 'python']) assert.ok(instance.result[key].startsWith(owner.context.directory+'/'));
  }
  one.child.stdin.end(); await one.exited;
  const reopened = await start(f); assert.deepEqual(reopened.result.environment, f.environment);
  assert.notEqual(one.result.node, two.result.node);
  assert.equal(await readFile(other.context.markerPath, 'utf8'), JSON.stringify(other.marker));
});
