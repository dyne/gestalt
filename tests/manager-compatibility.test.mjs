import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const exec = promisify(execFile);
const source = await readFile(resolve('public/gestalt'), 'utf8');
async function fixture(t, mode = 'current', installed = true) {
  const root = await mkdtemp(join(tmpdir(), 'manager compatibility '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, 'managed'), codex = join(root, 'codex'), manager = join(root, 'gestalt'), bin = join(root, 'bin');
  const userHome = join(root, 'user home');
  await mkdir(userHome);
  const mobile = join(home, 'mobile/node_modules/.bin/gestalt-mobile');
  await mkdir(join(mobile, '..'), { recursive: true }); await mkdir(codex);
  await mkdir(bin);
  await writeFile(join(bin, 'codex'), '#!/usr/bin/env bash\nprintf \'[]\'\n', { mode: 0o755 });
  // Exercise the candidate's actual Mobile argv/environment/restart boundary.
  // External package setup is excluded from this isolated fixture.
  await writeFile(manager, source.replace(/\nmain "\$@"\s*$/, `
install_mobile() { printf called > "$GESTALT_HOME/unexpected-install"; return 99; }
main "$@"
`), { mode: 0o755 });
  if (installed) await writeFile(mobile, `#!${process.execPath}
const fs=require('node:fs'),{spawnSync}=require('node:child_process');
const args=process.argv.slice(2);
if(args.some(arg=>arg==='--xerj'||arg.startsWith('--xerj=')))process.exit(2);
let fallback=false;
if(${JSON.stringify(mode)}==='legacy'){
 const old=spawnSync('bash',[process.env.GESTALT_MANAGER_BIN,'xerj','ensure-ready'],{env:process.env,encoding:'utf8'});
 if(old.status!==1||!old.stderr.includes('has been retired'))process.exit(3);
 fallback=true;
}
// Simulate the post-retirement Mobile boundary: unused old version metadata
// does not gate core startup. Actual assembled Mobile coverage belongs to L5.
fs.writeFileSync(${JSON.stringify(join(root, 'launch.json'))},JSON.stringify({args,fallback,version:process.env.GESTALT_XERJ_VERSION??null}));
console.log('core-started');
`, { mode: 0o755 });
  const env = { ...process.env, HOME: userHome, PATH: bin + ':' + process.env.PATH, CODEX_HOME: codex, GESTALT_HOME: home, GESTALT_XERJ_VERSION: undefined };
  const run = (args, extra = {}) => exec('bash', [manager, 'mobile', '--', ...args], { cwd: root, env: { ...env, ...extra }, timeout: 10000 });
  return { root, home, codex, manager, workspace: join(userHome, 'gestalt'), run };
}

for (const mode of ['current', 'legacy']) test(`candidate manager with ${mode} Mobile fixture retains core startup`, async t => {
  const f = await fixture(t, mode);
  const args = ['--cwd', f.root, '--port', '4321'];
  const result = await f.run(args);
  assert.match(result.stdout, /core-started/);
  assert.deepEqual(JSON.parse(await readFile(join(f.root, 'launch.json'), 'utf8')), { args, fallback: mode === 'legacy', version: null });
  await assert.rejects(stat(f.workspace), { code: 'ENOENT' });
  await assert.rejects(readFile(join(f.home, 'xerj/xerj')), { code: 'ENOENT' });
});

test('obsolete inherited manager metadata does not become a launch option', async t => {
  const f = await fixture(t);
  const args = ['--port', '4321'];
  await f.run(args, { GESTALT_XERJ_VERSION: 'legacy-fixture-version' });
  const launch = JSON.parse(await readFile(join(f.root, 'launch.json'), 'utf8'));
  assert.deepEqual(launch.args, [...args, '--cwd', f.workspace]);
  assert.equal(launch.version, 'legacy-fixture-version');
});

test('default Mobile launch creates the home workspace and records it for restart', async t => {
  const f = await fixture(t);
  await f.run([]);
  assert.ok((await stat(f.workspace)).isDirectory());
  const launch = JSON.parse(await readFile(join(f.root, 'launch.json'), 'utf8'));
  assert.deepEqual(launch.args, ['--cwd', f.workspace]);
  const run = join(f.home, 'run');
  const [descriptor] = (await readdir(run)).filter(name => name.endsWith('.restart'));
  const fields = (await readFile(join(run, descriptor), 'utf8')).split('\0');
  assert.deepEqual(fields.slice(1), ['--cwd', f.workspace, '']);
});

for (const args of [['--help'], ['--version'], ['--skills', 'list'], ['trace', 'session']])
  test(`Mobile ${args.join(' ')} keeps its arguments and does not create the default workspace`, async t => {
    const f = await fixture(t);
    await f.run(args);
    const launch = JSON.parse(await readFile(join(f.root, 'launch.json'), 'utf8'));
    assert.deepEqual(launch.args, args);
    await assert.rejects(stat(f.workspace), { code: 'ENOENT' });
  });

test('a file at the default Mobile workspace path stops launch', async t => {
  const f = await fixture(t);
  await writeFile(f.workspace, 'keep');
  await assert.rejects(f.run([]), { code: 1, stderr: /could not create the default Mobile workspace/ });
  assert.equal(await readFile(f.workspace, 'utf8'), 'keep');
  await assert.rejects(stat(join(f.root, 'launch.json')), { code: 'ENOENT' });
});

for (const args of [['--xerj', 'auto'], ['--xerj', 'manual'], ['--xerj', 'off'], ['--xerj=auto']])
  test(`obsolete explicit Mobile option fails before installation: ${args.join(' ')}`, async t => {
    const f = await fixture(t, 'current', false);
    await assert.rejects(f.run(args), error => {
      assert.match(error.stderr, /omit --xerj.*update/);
      return error.code === 1;
    });
    await assert.rejects(readFile(join(f.home, 'unexpected-install')), { code: 'ENOENT' });
    await assert.rejects(readFile(join(f.root, 'launch.json')), { code: 'ENOENT' });
  });
