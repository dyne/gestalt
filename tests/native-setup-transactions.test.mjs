import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, mkdir, mkdtemp, readFile, readdir, realpath, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { parse } from 'smol-toml';
import { buildCli } from '../scripts/build-cli.mjs';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { CommandFailure, SetupContext } from '../src/cli/setup/context.ts';
import { restartWorker } from '../src/cli/setup/restart.ts';
import { updateManager } from '../src/cli/setup/manager-update.ts';

const repository = path.resolve(import.meta.dirname, '..');
const artifacts = await mkdtemp(path.join(tmpdir(), 'native-setup-build-'));
const manager = await buildCli(repository, path.join(artifacts, 'gestalt.mjs'));
const unixProbe = await buildNativePlatform(repository, path.join(artifacts, 'native'));
test.after(() => rm(artifacts, { recursive: true, force: true }));
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const custom = '# keep custom model\nmodel="fixture-custom"\n[mcp_servers.custom]\ncommand="custom"\ndefault_tools_approval_mode="prompt"\n';

async function command(bin, name, body) {
  const script = path.join(bin, `${name}.mjs`);
  await writeFile(script, '#!/usr/bin/env node\n' + body, { mode: 0o755 });
  if (process.platform !== 'win32') {
    await writeFile(path.join(bin, name), '#!/usr/bin/env node\n' + body, { mode: 0o755 });
  } else {
    const captured = JSON.parse(await readFile(path.join(repository, 'tests/fixtures/native-npm-node-shim.json'), 'utf8'));
    const shim = Buffer.from(captured.base64, 'base64').toString('utf8').replace('.\\native-forward-fixture\\entry.mjs', `${name}.mjs`);
    await writeFile(path.join(bin, `${name}.cmd`), shim);
  }
}

async function fixture(t, config = custom) {
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'native setup ü & ')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = path.join(root, 'home ü &'), codex = path.join(home, 'Codex ü &'), gestalt = path.join(home, 'Gestalt ü &');
  await mkdir(home, { mode: 0o700 });
  const storage = await NativePrivateStorage.open(codex, unixProbe);
  await NativePrivateStorage.open(gestalt, unixProbe);
  const bin = path.join(root, 'bin'); await mkdir(bin, { mode: 0o700 });
  const ownerRoot = path.join(root, 'Agents ü &'); await mkdir(ownerRoot, { mode: 0o700 });
  const owner = path.join(ownerRoot, 'gestalt-setup.mjs'), log = path.join(root, 'calls.jsonl');
  if (config !== null) await writeFile(path.join(codex, 'config.toml'), config, { mode: 0o640 });
  await mkdir(path.join(codex, 'agents'), { mode: 0o700 });
  await writeFile(path.join(codex, 'agents/custom.toml'), 'model="preserve"\n');
  await writeFile(path.join(codex, 'AGENTS.md'), 'user instructions\n');
  await command(bin, 'npm', `import fs from 'node:fs';import path from 'node:path';
const args=process.argv.slice(2),index=args.indexOf('--prefix');
if(process.env.FIXTURE_MOBILE_FAIL==='1')process.exit(41);
if(index>=0){const prefix=args[index+1],bin=path.join(prefix,'node_modules/.bin');fs.mkdirSync(bin,{recursive:true,mode:0o700});fs.mkdirSync(path.join(prefix,'node_modules/gestalt-mobile'),{recursive:true,mode:0o700});fs.writeFileSync(path.join(prefix,'node_modules/gestalt-mobile/package.json'),JSON.stringify({name:'gestalt-mobile',version:'0.0.1'}));fs.writeFileSync(path.join(bin,'gestalt-mobile'),'#!/usr/bin/env node\\n',{mode:0o755});}`);
  await command(bin, 'codex', `import fs from 'node:fs';import path from 'node:path';
const args=process.argv.slice(2);fs.appendFileSync(process.env.FIXTURE_LOG,JSON.stringify(['codex',...args])+'\\n');
if(args[0]==='plugin'&&args[1]==='marketplace'){if(process.env.FIXTURE_MARKETPLACE_FAIL==='1')process.exit(7);}
else if(args[0]==='plugin'&&args[1]==='list')console.log(JSON.stringify({installed:[{name:'gestalt',version:'1.2.3',installed:true},{name:'context-mode',version:process.env.FIXTURE_VERIFY_FAIL==='1'?'9.9.9':'1.2.3',installed:true,enabled:false}]}));
else if(args[0]==='mcp')console.log(JSON.stringify({enabled:true,command:'node',args:[path.join(process.env.CODEX_HOME,'bin/context-mode-mcp.mjs')]}));
else throw new Error('unexpected fixture Codex invocation');`);
  await writeFile(owner, `import fs from 'node:fs/promises';import path from 'node:path';
await fs.appendFile(process.env.FIXTURE_LOG,JSON.stringify(['setup',...process.argv.slice(2)])+'\\n');
if(process.env.FIXTURE_WAIT==='1'){await fs.writeFile(process.env.FIXTURE_READY,'ready');while(true){try{await fs.stat(process.env.FIXTURE_RELEASE);break;}catch{}await new Promise(r=>setTimeout(r,20));}}
if(process.env.FIXTURE_FAIL==='1')process.exit(9);
const home=process.env.CODEX_HOME,gestalt=process.env.GESTALT_HOME;
await fs.mkdir(path.join(home,'agents'),{recursive:true,mode:0o700});
await fs.writeFile(path.join(home,'agents/org-plan-executor.toml'),'model="fixture-default"\\n');await fs.writeFile(path.join(home,'agents/org-plan-reviewer.toml'),'model="fixture-default"\\n');
await fs.writeFile(path.join(home,'hooks.json'),'{"fixture":true}\\n',{mode:0o600});
const config=path.join(home,'config.toml');let source=await fs.readFile(config,'utf8');if(!source.includes('[mcp_servers.context-mode]'))source+='\\n[mcp_servers.context-mode]\\ncommand="node"\\nargs=['+JSON.stringify(path.join(home,'bin/context-mode-mcp.mjs'))+']\\ndefault_tools_approval_mode="prompt"\\n';await fs.writeFile(config,source);
const runtime=path.join(gestalt,'runtime/context-mode');await fs.mkdir(runtime,{recursive:true,mode:0o700});
await fs.writeFile(path.join(runtime,'.context-mode-prepared.json'),JSON.stringify({packageVersion:'1.2.3',nodeModulesAbi:process.versions.modules,platform:process.platform,arch:process.arch}));
for(const file of ['cli.bundle.mjs','server.bundle.mjs'])await fs.writeFile(path.join(runtime,file),'// fixture');
`);
  const env = { ...process.env, HOME: home, USERPROFILE: home, CODEX_HOME: codex, GESTALT_HOME: gestalt,
    GESTALT_AGENTS_SOURCE: ownerRoot, GESTALT_AGENTS_SETUP_SHA256: hash(await readFile(owner)), GESTALT_MANAGER_DISTRIBUTION: 'npm',
    GESTALT_NATIVE_UNIX_PROBE: unixProbe || '', FIXTURE_LOG: log, FIXTURE_READY: path.join(root, 'ready'), FIXTURE_RELEASE: path.join(root, 'release'),
    PATH: [bin, path.dirname(process.execPath), process.env.PATH].join(path.delimiter), PATHEXT: '.CMD;.EXE' };
  for (const name of Object.keys(env)) if (name.toUpperCase() === 'PATH' && name !== 'PATH') delete env[name];
  function start(argv, extra = {}) {
    const child = spawn(process.execPath, [manager, ...argv], { env: { ...env, ...extra }, cwd: root, stdio: ['ignore', 'pipe', 'pipe'], shell: false });
    let stdout = '', stderr = '';
    child.stdout.on('data', b => stdout += b); child.stderr.on('data', b => stderr += b);
    const result = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', status => resolve({ status, stdout, stderr })); });
    return { child, result };
  }
  return { root, home, codex, gestalt, owner, ownerRoot, env, log, storage, start,
    run: (argv = ['agents', 'install'], extra) => start(argv, extra).result,
    config: () => readFile(path.join(codex, 'config.toml'), 'utf8'),
    provenance: () => readFile(path.join(codex, '.gestalt-setup-provenance.json'), 'utf8') };
}
async function until(check, message) {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) { if (await check()) return; await new Promise(resolve => setTimeout(resolve, 20)); }
  assert.fail(message);
}
const exists = file => stat(file).then(() => true, error => { if (error.code === 'ENOENT') return false; throw error; });

for (const initial of [null, custom]) test(`compiled native ${initial === null ? 'fresh' : 'custom'} Agents setup repeats with stable config and provenance`, async t => {
  const f = await fixture(t, initial);
  const first = await f.run(); assert.equal(first.status, 0, first.stderr);
  const config = await f.config(), provenance = await f.provenance(), parsed = parse(config), metadata = JSON.parse(provenance);
  assert.equal(parsed.approval_policy, 'never'); assert.equal(parsed.default_permissions, 'workspace-git');
  assert.equal(parsed.mcp_servers['context-mode'].default_tools_approval_mode, 'approve');
  if (initial) { assert.ok(config.includes('# keep custom model')); assert.equal(parsed.model, 'fixture-custom'); }
  assert.equal(await readFile(path.join(f.codex, 'agents/custom.toml'), 'utf8'), 'model="preserve"\n');
  assert.equal(await readFile(path.join(f.codex, 'AGENTS.md'), 'utf8'), 'user instructions\n');
  assert.equal(metadata.scope, 'setup-artifacts-only'); assert.equal(metadata.runtimeEnforcement, false);
  assert.equal(metadata.manager.sha256, hash(await readFile(manager))); assert.equal(metadata.delegatedSetupSha256, hash(await readFile(f.owner)));
  assert.equal(metadata.outputs['config.toml'], hash(Buffer.from(config)));
  const repeat = await f.run(['agents', 'update']); assert.equal(repeat.status, 0, repeat.stderr);
  assert.equal(await f.config(), config); assert.equal(await f.provenance(), provenance);
  assert.deepEqual((await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup.') || name === '.gestalt-manager-setup.lock' || name === '.gestalt-setup.lock'), []);
  if (process.platform !== 'win32' && initial) assert.equal((await stat(path.join(f.codex, 'config.toml'))).mode & 0o777, 0o640);
});

for (const failure of ['FIXTURE_FAIL', 'FIXTURE_VERIFY_FAIL', 'FIXTURE_MARKETPLACE_FAIL'])
  for (const initial of [null, custom]) test(`${failure} restores ${initial === null ? 'absent' : 'custom'} native profile without successful provenance`, async t => {
    const f = await fixture(t, initial), result = await f.run(undefined, { [failure]: '1' });
    assert.notEqual(result.status, 0);
    if (initial === null) assert.equal(await exists(path.join(f.codex, 'config.toml')), false); else assert.equal(await f.config(), initial);
    assert.equal(await exists(path.join(f.codex, '.gestalt-setup-provenance.json')), false);
    assert.deepEqual((await readdir(f.codex)).filter(name => name.startsWith('.gestalt-setup') || name === '.gestalt-manager-setup.lock'), []);
  });

test('native setup preserves last successful provenance on a failed repeat', async t => {
  const f = await fixture(t); assert.equal((await f.run()).status, 0);
  const before = await f.config(), provenance = await f.provenance();
  assert.notEqual((await f.run(['agents', 'update'], { FIXTURE_FAIL: '1' })).status, 0);
  assert.equal(await f.config(), before); assert.equal(await f.provenance(), provenance);
});

test('native setup serializes concurrent managers and releases only its own lock', async t => {
  const f = await fixture(t), first = f.start(['agents', 'install'], { FIXTURE_WAIT: '1' });
  try {
    await until(() => exists(f.env.FIXTURE_READY), 'first owner did not reach controlled wait');
    const second = await f.run(); assert.notEqual(second.status, 0); assert.match(second.stderr, /EEXIST|already running/);
  } finally { await writeFile(f.env.FIXTURE_RELEASE, 'release'); }
  assert.equal((await first.result).status, 0); assert.equal((await f.run()).status, 0);
});

test('native setup refuses an existing delegated-owner lock before modifying config', async t => {
  const f = await fixture(t); await mkdir(path.join(f.codex, '.gestalt-setup.lock'), { mode: 0o700 });
  const result = await f.run(); assert.notEqual(result.status, 0); assert.match(result.stderr, /already running|unknown/);
  assert.equal(await f.config(), custom); assert.equal(await exists(f.log), false);
  assert.equal(await exists(path.join(f.codex, '.gestalt-setup.lock')), true);
});

test('native setup rejects invalid session context and unsupported options before mutation', async t => {
  const f = await fixture(t);
  const invalid = await f.run(undefined, { GESTALT_SESSION_TMP_CONTEXT: '' }); assert.notEqual(invalid.status, 0);
  assert.equal(await f.config(), custom); assert.equal(await exists(f.log), false);
  for (const args of [['install', '--executor-model', 'bad'], ['agents', 'install', '--extra-skills'], ['update', 'serena', '--extra-skills']]) {
    const result = await f.run(args); assert.notEqual(result.status, 0, result.stderr); assert.equal(await exists(f.log), false);
  }
});

test('npm-owned native manager update preserves package executable and retains component failure status', async t => {
  const f = await fixture(t), before = await readFile(manager);
  const result = await f.run(['update', 'gestalt'], { GESTALT_SKILLS_SOURCE: path.join(f.root, 'missing-fixture-collection') });
  assert.notEqual(result.status, 0); assert.match(result.stderr, /installed with npm/);
  assert.deepEqual(await readFile(manager), before); assert.equal(await exists(f.log), false);
});

test('native updated-manager continuation rejects invalid reentry without mutation', async t => {
  const f = await fixture(t);
  for (const args of [[], ['__updated-manager'], ['unknown']]) {
    const result = await f.run(['__updated-manager', ...args]); assert.notEqual(result.status, 0); assert.equal(await exists(f.log), false);
  }
});

test('compiled install/update preserves native skills, extra selection and optional Mobile policy', async t => {
  const f = await fixture(t), collection = path.join(f.root, 'collection ü');
  await mkdir(path.join(collection, 'skills/native-fixture'), { recursive: true });
  const skill = '---\nname: native-fixture\ndescription: Native setup fixture.\n---\n# Fixture\n';
  await writeFile(path.join(collection, 'skills/native-fixture/SKILL.md'), skill);
  await writeFile(path.join(collection, 'LICENSE'), 'MIT fixture\n');
  await writeFile(path.join(collection, 'manifest.json'), JSON.stringify({ schemaVersion: 1, repository: 'dyne/gestalt-skills', source: 'https://github.com/dyne/gestalt-skills', version: '0.1.0', skills: [{ name: 'native-fixture', entry: 'skills/native-fixture/SKILL.md', files: [{ path: 'skills/native-fixture/SKILL.md', sha256: hash(Buffer.from(skill)) }] }] }));
  for (const args of [['init', '--initial-branch=main', '--template='], ['add', '--all'], ['commit', '-m', 'native collection fixture']]) {
    const git = spawnSync('git', ['-c', 'core.hooksPath=' + path.join(collection, '.empty-hooks'), '-c', 'commit.gpgSign=false', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test', ...args], { cwd: collection, encoding: 'utf8', shell: false });
    assert.equal(git.status, 0, git.stderr);
  }
  const env = { GESTALT_SKILLS_SOURCE: collection };
  const installed = await f.run(['install', '--extra-skills'], env); assert.equal(installed.status, 0, installed.stderr);
  const receipt = JSON.parse(await readFile(path.join(f.codex, 'gestalt-skills/receipt.json'), 'utf8'));
  assert.equal(receipt.verifiedSource, false);
  assert.equal(await readFile(path.join(f.codex, 'skills/native-fixture/SKILL.md'), 'utf8'), skill);
  assert.ok((await readFile(f.log, 'utf8')).split('\n').includes('["setup","--extra-skills"]'));
  if (process.platform === 'win32') {
    assert.match(installed.stderr, /Mobile omitted:.*native win32.*owned-process.*session-scratch/);
    assert.equal(await exists(path.join(f.gestalt, 'mobile')), false);
    assert.doesNotMatch(installed.stderr, /then 'gestalt cli' or 'gestalt mobile'/);
  } else await writeFile(path.join(f.gestalt, 'mobile/user-data.txt'), 'preserve this data\n');
  const updated = await f.run(['update', 'gestalt'], env); assert.equal(updated.status, 0, updated.stderr);
  assert.equal(await readFile(path.join(f.codex, 'skills/native-fixture/SKILL.md'), 'utf8'), skill);
  assert.equal(await readFile(path.join(f.codex, 'gestalt-skills/receipt.json'), 'utf8'), JSON.stringify(receipt, null, 2) + '\n');
  if (process.platform === 'win32') assert.doesNotMatch(updated.stderr, /Mobile, and context-mode are up to date/);
  else {
    assert.equal(await readFile(path.join(f.gestalt, 'mobile/user-data.txt'), 'utf8'), 'preserve this data\n');
    const failed = await f.run(['update', 'gestalt'], { ...env, FIXTURE_MOBILE_FAIL: '1' });
    assert.equal(failed.status, 41, failed.stderr);
    assert.equal(await readFile(path.join(f.gestalt, 'mobile/user-data.txt'), 'utf8'), 'preserve this data\n');
    assert.doesNotMatch(failed.stderr, /update complete/);
  }
});

test('native restart update failure retains the live relay, descriptor and non-owned data', async t => {
  const f = await fixture(t), run = path.join(f.gestalt, 'run');
  await mkdir(run, { mode: 0o700 });
  const relay = spawn(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { stdio: 'ignore' });
  const stopped = new Promise(resolve => relay.once('close', resolve));
  t.after(async () => { if (relay.exitCode === null && relay.signalCode === null) relay.kill(); await stopped; });
  const descriptor = path.join(run, `mobile-${relay.pid}.restart`), bytes = Buffer.from([f.root, '--port', '4321', ''].join('\0'));
  await writeFile(descriptor, bytes, { mode: 0o600 });
  const lock = path.join(run, 'update-restart.lock'); await mkdir(lock, { mode: 0o700 });
  const identity = await stat(lock, { bigint: true }), token = 'fixture-controlled-token';
  await writeFile(path.join(lock, 'owner.json'), JSON.stringify({ schemaVersion: 1, token, dev: String(identity.dev), ino: String(identity.ino) }), { mode: 0o600 });
  const log = path.join(f.gestalt, 'update-restart.log');
  const context = new SetupContext({ argv: [], cwd: f.root, environment: f.env },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: f.home, codexHome: f.codex, gestaltHome: f.gestalt, marketplace: 'fixture', marketplaceName: 'fixture', manager, unixProbe: unixProbe || undefined }, '0.16.1');
  const status = await restartWorker(context, [String(relay.pid), descriptor, 'false', log, lock, 'gestalt', token],
    async () => { throw new CommandFailure('fixture update failed (exit 42)', 42); });
  assert.equal(status, 42); assert.equal(relay.exitCode, null); assert.equal(relay.signalCode, null);
  assert.deepEqual(await readFile(descriptor), bytes); assert.equal(await exists(lock), false);
  assert.match(await readFile(log, 'utf8'), /fixture update failed \(exit 42\)/);
});

for (const invalid of [false, true]) test(`native standalone ${invalid ? 'invalid replacement preserves installed executable' : 'replacement resumes exact argv and propagates failure'}`, async t => {
  const f = await fixture(t), directory = path.join(f.root, 'public bin ü'), release = path.join(f.root, 'release');
  await mkdir(directory, { mode: 0o755 }); await mkdir(release, { mode: 0o700 });
  const target = path.join(directory, 'gestalt.mjs'), marker = '#!/usr/bin/env node\n// GESTALT_CLI_VERSION=\'0.16.1\'\n';
  const previous = marker + '// previous release\n';
  await writeFile(target, previous, { mode: 0o755 });
  const resultFile = path.join(f.root, 'replacement-argv.json');
  const replacement = invalid ? marker + 'invalid javascript !' : marker + `import fs from 'node:fs';fs.writeFileSync(${JSON.stringify(resultFile)},JSON.stringify(process.argv.slice(2)));process.exitCode=42;\n`;
  await writeFile(path.join(release, 'gestalt.mjs'), replacement);
  const env = { ...f.env, GESTALT_MANAGER_DISTRIBUTION: '', GESTALT_INSTALL_BASE_URL: pathToFileURL(release).href };
  const context = new SetupContext({ argv: [], cwd: f.root, environment: env },
    { stdin: (async function* () {})(), stdout: async () => {}, stderr: async () => {} },
    { userHome: f.home, codexHome: f.codex, gestaltHome: f.gestalt, marketplace: 'fixture', marketplaceName: 'fixture', manager: target, unixProbe: unixProbe || undefined }, '0.16.1');
  const argv = ['update', 'gestalt', '--extra-skills', 'space ü & literal'];
  await assert.rejects(updateManager(context, argv, false), error => invalid || error.status === 42);
  assert.equal(await readFile(target, 'utf8'), invalid ? previous : replacement);
  if (!invalid) assert.deepEqual(JSON.parse(await readFile(resultFile, 'utf8')), ['__updated-manager', ...argv]);
  assert.equal(await exists(path.join(directory, '.gestalt-manager-update.lock')), false);
  if (process.platform !== 'win32') assert.equal((await stat(target)).mode & 0o777, 0o755);
});
