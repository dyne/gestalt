import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { mkdtemp, mkdir, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { promisify } from 'node:util';
import { pathToFileURL } from 'node:url';
import test from 'node:test';

const exec = promisify(execFile);
const source = await readFile(resolve('public/gestalt'), 'utf8');
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt retirement '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, 'custom managed state'), codex = join(root, 'custom codex state');
  const manager = join(root, 'manager');
  await mkdir(home); await mkdir(codex);
  // Keep the production explicit-update orchestration and migration. Installers
  // are stubbed so no test can update a real profile, download or start services.
  await writeFile(manager, source.replace(/\nmain "\$@"\s*$/, `
update_manager() { :; }
command_skills() { :; }
run_marketplace_setup() { :; }
install_mobile() { :; }
main "$@"
`), { mode: 0o755 });
  const env = { ...process.env, GESTALT_HOME: home, CODEX_HOME: codex };
  const run = (extra = {}) => exec('bash', [manager, 'update', 'gestalt'], { env: { ...env, ...extra }, timeout: 10000 });
  const config = join(codex, 'config.toml');
  const owned = name => `[mcp_servers.${JSON.stringify(name)}]\ncommand = ${JSON.stringify(manager)}\nargs = ["xerj", "mcp"]\nenabled = true\n`;
  return { root, home, codex, manager, env, config, owned, run };
}

test('fresh custom homes remain free of retired integration state', async t => {
  const f = await fixture(t);
  await f.run(); await f.run();
  assert.deepEqual(await readdir(f.codex), []);
  assert.deepEqual(await readdir(f.home), []);
});

test('obsolete manual entrypoints fail before side effects and public help stays clear', async t => {
  const f = await fixture(t);
  const bin = join(f.root, 'bin'); await mkdir(bin);
  const marker = join(f.root, 'unexpected-execution');
  for (const name of ['curl', 'codex', 'kimi']) await writeFile(join(bin, name), `#!/usr/bin/env bash\nprintf called >> ${JSON.stringify(marker)}\nexit 1\n`, { mode: 0o755 });
  const env = { ...f.env, PATH: bin + ':' + process.env.PATH };
  for (const args of [['xerj'], ['xerj', 'mcp'], ['xerj', 'ensure-ready'], ['xerj', 'install'], ['update', 'xerj'], ['update', '--component', 'xerj'], ['update-restart', 'xerj']]) {
    await assert.rejects(exec('bash', [f.manager, ...args], { env }), error => {
      assert.match(error.stderr, /Xerj has been retired; run gestalt update/);
      return error.code === 1;
    });
  }
  const { stdout } = await exec('bash', [f.manager, 'help'], { env });
  assert.doesNotMatch(stdout, /xerj|watcher/i);
  await assert.rejects(readFile(marker), { code: 'ENOENT' });
  assert.deepEqual(await readdir(f.home), []);
  assert.deepEqual(await readdir(f.codex), []);
});

for (const name of ['gestalt-xerj', 'old retrieval alias']) test(`explicit update retires exact managed ${name} and preserves unrelated bytes`, async t => {
  const f = await fixture(t);
  const before = '# operator comment\r\nmodel = "kept"\r\n\r\n';
  const after = '\r\n# keep this comment\r\n[mcp_servers.user]\r\ncommand = "user-xerj"\r\nargs = ["mcp"]\r\n';
  await writeFile(f.config, before + f.owned(name).replaceAll('\n', '\r\n') + after, { mode: 0o640 });
  await f.run();
  assert.equal(await readFile(f.config, 'utf8'), before + after);
  assert.equal((await stat(f.config)).mode & 0o777, 0o640);
  await f.run();
  assert.equal(await readFile(f.config, 'utf8'), before + after);
});

test('all matching profile fragments retire together while tools, comments and user servers survive', async t => {
  const f = await fixture(t);
  const keep = '# instructions\n[features]\nhooks = true\n';
  const owned = f.owned('alias') + '[mcp_servers.alias.tools.xerj_search]\napproval_mode = "approve"\n';
  for (const name of ['config.toml', 'private.config.toml']) await writeFile(join(f.codex, name), keep + owned);
  await f.run();
  for (const name of ['config.toml', 'private.config.toml']) assert.equal(await readFile(join(f.codex, name), 'utf8'), keep);
});

for (const [label, transport] of [
  ['same name user server', 'command = "xerj"\nargs = ["mcp"]'],
  ['modified arguments', 'command = MANAGER\nargs = ["xerj", "mcp", "--custom"]'],
  ['incomplete ownership', 'enabled = true'],
  ['bare executable', 'command = "gestalt"\nargs = ["xerj", "mcp"]'],
]) test(`preserves ambiguous ${label} with bounded guidance`, async t => {
  const f = await fixture(t);
  const bytes = '# mine\n[mcp_servers."gestalt-xerj"]\n' + transport.replace('MANAGER', JSON.stringify(f.manager)) + '\n';
  await writeFile(f.config, bytes);
  const { stderr } = await f.run();
  assert.equal(await readFile(f.config, 'utf8'), bytes);
  assert.match(stderr, /Xerj retirement:.*preserv.*review/i);
  assert.ok(Buffer.byteLength(stderr) < 2048);
});

test('manager executable symlink aliases are recognized, configuration symlinks are preserved', async t => {
  const f = await fixture(t);
  const alias = join(f.root, 'manager alias'); await symlink(f.manager, alias);
  await writeFile(f.config, f.owned('managed alias').replace(JSON.stringify(f.manager), JSON.stringify(alias)));
  await f.run(); assert.equal(await readFile(f.config, 'utf8'), '');
  const outside = join(f.root, 'external config'); await writeFile(outside, f.owned('gestalt-xerj'));
  await rm(f.config); await symlink(outside, f.config);
  const { stderr } = await f.run();
  assert.equal(await readFile(outside, 'utf8'), f.owned('gestalt-xerj'));
  assert.match(stderr, /preserv.*review/i);
});

test('unrepresentable inline entries and malformed config stay byte identical', async t => {
  const f = await fixture(t);
  for (const bytes of [`mcp_servers = { alias = { command = ${JSON.stringify(f.manager)}, args = ["xerj", "mcp"] } }\n`, '[mcp_servers.bad\n']) {
    await writeFile(f.config, bytes);
    const { stderr } = await f.run();
    assert.equal(await readFile(f.config, 'utf8'), bytes);
    assert.match(stderr, /preserv.*review/i);
  }
});

test('old interrupted stages do not replace config and repeated migration preserves all durable data', async t => {
  const f = await fixture(t);
  await writeFile(f.config, f.owned('gestalt-xerj'));
  const stage = join(f.codex, 'config.toml.tmp-interrupted'); await writeFile(stage, 'partial');
  const data = ['xerj-data/managed.toml', 'xerj-data/auth/key', 'xerj-data/indexes/fixture', 'auth.json', 'sessions/fixture', 'profiles/fixture'];
  for (const name of data) { await mkdir(join(f.codex, name, '..'), { recursive: true }); await writeFile(join(f.codex, name), 'durable bytes'); }
  await mkdir(join(f.home, 'xerj')); await writeFile(join(f.home, 'xerj/xerj'), 'user binary');
  await f.run(); await f.run();
  assert.equal(await readFile(stage, 'utf8'), 'partial');
  for (const name of data) assert.equal(await readFile(join(f.codex, name), 'utf8'), 'durable bytes');
  assert.equal(await readFile(join(f.home, 'xerj/xerj'), 'utf8'), 'user binary');
});

test('interruption before atomic promotion leaves original bytes and a retry converges', async t => {
  const f = await fixture(t);
  const bytes = '# retained\n' + f.owned('gestalt-xerj');
  await writeFile(f.config, bytes);
  const hook = join(f.root, 'interrupt promotion.mjs');
  await writeFile(hook, `import fs from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
const rename = fs.renameSync;
fs.renameSync = (from, to) => { if (String(from).includes('.gestalt-retirement-')) process.exit(75); return rename(from, to); };
syncBuiltinESMExports();\n`);
  await assert.rejects(f.run({ NODE_OPTIONS: '--import=' + pathToFileURL(hook).href }), { code: 75 });
  assert.equal(await readFile(f.config, 'utf8'), bytes);
  assert.equal((await readdir(f.codex)).filter(name => name.startsWith('.gestalt-retirement-')).length, 1);
  await f.run(); await f.run();
  assert.equal(await readFile(f.config, 'utf8'), '# retained\n');
});

test('restart consumption removes only known obsolete options and preserves user arguments', async () => {
  const helper = 'retire_mobile_restart_arguments() {' + source.split('retire_mobile_restart_arguments() {')[1].split('\ncommand_update_restart_worker()')[0];
  for (const mode of ['auto', 'manual', 'off']) {
    const { stdout } = await exec('bash', ['-c', helper + '\nretire_mobile_restart_arguments "$@"\nprintf "%s\\0" "${migrated_restart_args[@]}"', '--', '--port', '4321', '--xerj', mode, '--cwd', '/workspace with spaces', '--', '--xerj', 'user argument']);
    assert.equal(stdout, ['--port', '4321', '--cwd', '/workspace with spaces', '--', '--xerj', 'user argument', ''].join('\0'));
  }
  for (const args of [['--xerj'], ['--xerj', 'custom']])
    await assert.rejects(exec('bash', ['-c', 'die() { exit 1; }\n' + helper + '\nretire_mobile_restart_arguments "$@"', '--', ...args]), { code: 1 });
});

test('stale/reused PIDs, incomplete process records and unproven restart descriptors cause no signals or changes', async t => {
  const f = await fixture(t);
  const child = spawn('sleep', ['30']); const stopped = new Promise(resolve => child.once('exit', resolve));
  t.after(async () => { child.kill(); await stopped; });
  const lifecycle = join(f.codex, 'xerj-data/.lifecycle'); await mkdir(lifecycle, { recursive: true });
  await mkdir(join(f.home, 'run'));
  const records = [join(lifecycle, 'stale.json'), join(lifecycle, 'incomplete.json'), join(f.home, `run/mobile-${child.pid}.restart`)];
  const bytes = [JSON.stringify({ pid: child.pid, identity: 'stale reused identity', root: f.home }), '{}', [f.root, '--xerj', 'auto', '--port', '4321', ''].join('\0')];
  for (let i = 0; i < records.length; i++) await writeFile(records[i], bytes[i]);
  const { stderr } = await f.run(); await f.run();
  assert.equal(child.exitCode, null); assert.equal(child.signalCode, null);
  for (let i = 0; i < records.length; i++) assert.equal(await readFile(records[i], 'utf8'), bytes[i]);
  assert.match(stderr, /process.*preserv.*review/i);
  assert.match(stderr, /restart.*preserv.*review/i);
});
