import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, rm, readdir, chmod } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import test from 'node:test';

const source = await readFile(resolve('public/gestalt'), 'utf8');
const digest = value => createHash('sha256').update(value).digest('hex');
const quote = value => "'" + value.replaceAll("'", "'\\''") + "'";

async function run(fixture, args, changes = {}) {
  const child = spawn('bash', [fixture.manager, 'impeccable', ...args], {
    cwd: fixture.project, env: { ...fixture.env, ...changes }, stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stdout = '', stderr = '';
  child.stdout.on('data', part => stdout += part);
  child.stderr.on('data', part => stderr += part);
  const timer = setTimeout(() => child.kill('SIGKILL'), 15000);
  try {
    const code = await new Promise((resolve, reject) => { child.once('error', reject); child.once('close', resolve); });
    return { code, stdout, stderr };
  } finally { clearTimeout(timer); }
}

async function fixture(t, { probeFailure = false } = {}) {
  const root = await mkdtemp('/tmp/impeccable-runtime-');
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, 'home with spaces'), managed = join(home, 'private runtime');
  const project = join(root, 'project with spaces'), packageRoot = join(root, 'package');
  const bin = join(root, 'bin');
  for (const directory of [home, project, packageRoot, bin]) await mkdir(directory, { recursive: true });
  await mkdir(join(project, '.impeccable/live'), { recursive: true });
  await writeFile(join(project, '.impeccable/live/journal.json'), 'preserved project journal');
  const executable = `#!/usr/bin/env node
if(process.argv[2]==='engine-probe'){${probeFailure ? 'process.exit(1);' : "console.log('impeccable-engine 0.1.11');"}}
else if(process.argv[2]==='--version')console.log('4.0.0');
else if(process.argv[2]==='live-server' && process.argv[3]==='--help')console.log('IMPECCABLE_LIVE_PUBLIC_BASE_URL');
else if(process.argv[2]==='live-poll' && process.argv[3]==='--help')console.log('--then-poll');
else process.exit(99);
`;
  const files = { impeccable: executable, LICENSE: 'Apache License Version 2.0 fixture\n',
    'NOTICE.md': 'Original upstream notices\n', 'SOURCE.md': 'Source provenance fixture\n',
    'source.patch': 'Pinned adaptation patch fixture\n', 'source.tar.gz': 'Pinned source snapshot fixture\n' };
  for (const [name, content] of Object.entries(files)) await writeFile(join(packageRoot, name), content);
  await chmod(join(packageRoot, 'impeccable'), 0o755);
  const adaptation = JSON.stringify({ schemaVersion: 1, contractVersion: 1, release: 'gestalt-live-1',
    platform: 'x86_64-unknown-linux-musl', engineVersion: '0.1.11', cliVersion: '4.0.0',
    sourceCommit: 'b593626bffd40e404c9e8b7972628660e210dbf4', baseCommit: '778c8a7b71ccd5bfe3ca6ac68c15d9d872d0f87d',
    publicBaseUrl: true, copyAgent: 'chat', protocol: 'impeccable-live-poll-v1', license: 'Apache-2.0',
    files: Object.fromEntries(Object.entries(files).map(([name, content]) => [name, digest(content)])) }) + '\n';
  await writeFile(join(packageRoot, 'adaptation.json'), adaptation);
  const archive = join(root, 'release with spaces.tar.gz');
  assert.equal(spawnSync('tar', ['-czf', archive, '-C', packageRoot, ...Object.keys(files), 'adaptation.json']).status, 0);
  // Substitute only the three trust anchors in a private manager copy. Tests do
  // not need a published artifact or bypass any runtime verification function.
  const manager = join(root, 'manager');
  const pins = { ARCHIVE: digest(await readFile(archive)), BINARY: digest(executable), ADAPTATION: digest(adaptation) };
  let managerSource = source;
  for (const [name, hash] of Object.entries(pins)) {
    const pattern = new RegExp(`(readonly MANAGED_IMPECCABLE_${name}_SHA256=)'(?:[a-f0-9]{64})?'`);
    assert.match(managerSource, pattern);
    managerSource = managerSource.replace(pattern, `$1'${hash}'`);
  }
  await writeFile(manager, managerSource);
  // A random PATH binary and sudo must never participate in installation.
  for (const name of ['impeccable', 'sudo']) await writeFile(join(bin, name),
    `#!/bin/sh\nprintf forbidden > ${quote(join(root, name + '-called'))}\nexit 99\n`, { mode: 0o755 });
  return { root, home, managed, project, packageRoot, bin, manager, archive,
    env: { ...process.env, HOME: home, GESTALT_HOME: managed, CODEX_HOME: join(home, 'codex'),
      GESTALT_IMPECCABLE_ENABLED: '1', GESTALT_IMPECCABLE_ARTIFACT: '', PATH: bin + ':' + process.env.PATH } };
}

test('pinned local install is private, idempotent, handles spaces and never uses PATH binary or sudo', async t => {
  const f = await fixture(t);
  const installed = await run(f, ['install', '--artifact', f.archive]);
  assert.equal(installed.code, 0, installed.stderr);
  const active = join(f.managed, 'impeccable/active.json');
  const before = await readFile(active, 'utf8');
  const descriptor = JSON.parse(before);
  assert.ok(descriptor.executable.startsWith(join(f.managed, 'impeccable/release-gestalt-live-1.')));
  assert.equal(await readFile(join(descriptor.releaseRoot, 'LICENSE'), 'utf8'), 'Apache License Version 2.0 fixture\n');
  const status = await run(f, ['status']);
  assert.equal(status.code, 0);
  assert.equal(JSON.parse(status.stdout).ready, true);
  assert.equal(JSON.parse(status.stdout).distributionPublished, false);
  assert.equal((await run(f, ['install', '--artifact', '/missing/archive'])).code, 0);
  assert.equal(await readFile(active, 'utf8'), before);
  assert.equal((await readdir(join(f.managed, 'impeccable'))).filter(n => n.startsWith('release-')).length, 1);
  assert.equal((await run(f, ['path'])).stdout.trim(), descriptor.executable);
  for (const name of ['impeccable', 'sudo']) await assert.rejects(readFile(join(f.root, name + '-called')), { code: 'ENOENT' });
  assert.equal(await readFile(join(f.project, '.impeccable/live/journal.json'), 'utf8'), 'preserved project journal');
});

test('checksum failure and offline update preserve the active healthy release', async t => {
  const f = await fixture(t);
  assert.equal((await run(f, ['install', '--artifact', f.archive])).code, 0);
  const active = join(f.managed, 'impeccable/active.json'), before = await readFile(active, 'utf8');
  const corrupt = join(f.root, 'corrupt.tar.gz'); await writeFile(corrupt, 'corrupt');
  assert.notEqual((await run(f, ['update', '--artifact', corrupt])).code, 0);
  await writeFile(join(f.bin, 'curl'), '#!/bin/sh\nexit 22\n', { mode: 0o755 });
  assert.notEqual((await run(f, ['update', '--artifact', 'https://fixture.invalid/artifact.tar.gz'])).code, 0);
  assert.equal(await readFile(active, 'utf8'), before);
  assert.equal(JSON.parse((await run(f, ['doctor', '--json'])).stdout).ready, true);
  assert.deepEqual((await readdir(join(f.managed, 'impeccable'))).sort(), ['active.json', JSON.parse(before).releaseRoot.split('/').at(-1)].sort());
});

test('interrupted download cleans candidate and lock while retaining the previous executable', async t => {
  const f = await fixture(t);
  assert.equal((await run(f, ['install', '--artifact', f.archive])).code, 0);
  const active = join(f.managed, 'impeccable/active.json'), before = await readFile(active, 'utf8');
  const entered = join(f.root, 'download-entered');
  await writeFile(join(f.bin, 'curl'), `#!/bin/sh\nprintf '%s' "$$" > ${quote(entered)}\nexec sleep 30\n`, { mode: 0o755 });
  const child = spawn('bash', [f.manager, 'impeccable', 'update', '--artifact', 'https://fixture.invalid/release'],
    { cwd: f.project, env: f.env, detached: true, stdio: 'ignore' });
  t.after(() => { try { process.kill(-child.pid, 'SIGKILL'); } catch {} });
  const closed = new Promise(resolve => child.once('close', resolve));
  for (let n = 0; ; n++) {
    try { await readFile(entered); break; } catch { assert.ok(n < 200, 'download never entered'); }
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  const awaitedPid = await readFile(entered, 'utf8');
  // Only stop the manager. Its cleanup must terminate the owned download too.
  child.kill('SIGTERM');
  assert.notEqual(await closed, 0);
  assert.throws(() => process.kill(Number(awaitedPid), 0), { code: 'ESRCH' });
  assert.equal(await readFile(active, 'utf8'), before);
  assert.equal(JSON.parse((await run(f, ['status'])).stdout).ready, true);
  assert.equal((await readdir(join(f.managed, 'impeccable'))).length, 2);
});

test('failed candidate probe cannot activate and does not leave install state', async t => {
  const f = await fixture(t, { probeFailure: true });
  assert.notEqual((await run(f, ['install', '--artifact', f.archive])).code, 0);
  await assert.rejects(readFile(join(f.managed, 'impeccable/active.json')), { code: 'ENOENT' });
  assert.deepEqual(await readdir(join(f.managed, 'impeccable')), []);
});

test('candidate probe failure during upgrade retains the prior validated release', async t => {
  const healthy = await fixture(t), failing = await fixture(t, { probeFailure: true });
  assert.equal((await run(healthy, ['install', '--artifact', healthy.archive])).code, 0);
  const active = join(healthy.managed, 'impeccable/active.json'), before = await readFile(active, 'utf8');
  assert.notEqual((await run(failing, ['update', '--artifact', failing.archive], { GESTALT_HOME: healthy.managed })).code, 0);
  assert.equal(await readFile(active, 'utf8'), before);
  assert.equal(JSON.parse((await run(healthy, ['doctor'])).stdout).ready, true);
});

test('HTTPS source uses the same pinned bytes and update retains immutable prior releases', async t => {
  const f = await fixture(t);
  assert.equal((await run(f, ['install', '--artifact', f.archive])).code, 0);
  const active = join(f.managed, 'impeccable/active.json'), before = JSON.parse(await readFile(active, 'utf8'));
  await writeFile(join(f.bin, 'curl'), `#!/bin/sh\nfor destination do :; done\ncp ${quote(f.archive)} "$destination"\n`, { mode: 0o755 });
  assert.equal((await run(f, ['update', '--artifact', 'https://fixture.invalid/pinned.tar.gz'])).code, 0);
  const after = JSON.parse(await readFile(active, 'utf8'));
  assert.notEqual(after.executable, before.executable);
  assert.equal(await readFile(before.executable, 'utf8'), await readFile(after.executable, 'utf8'));
  assert.equal((await readdir(join(f.managed, 'impeccable'))).filter(n => n.startsWith('release-')).length, 2);
  const denied = await run(f, ['update', '--artifact', 'https://user:private-secret@fixture.invalid/pinned.tar.gz']);
  assert.notEqual(denied.code, 0);
  assert.ok(!denied.stderr.includes('private-secret'));
  assert.equal(JSON.parse(await readFile(active, 'utf8')).executable, after.executable);
});

test('signal after atomic promotion preserves the newly active release', async t => {
  const f = await fixture(t);
  assert.equal((await run(f, ['install', '--artifact', f.archive])).code, 0);
  const active = join(f.managed, 'impeccable/active.json'), before = JSON.parse(await readFile(active, 'utf8'));
  await writeFile(join(f.bin, 'mv'), '#!/bin/sh\n/bin/mv "$@" || exit $?\nkill -TERM "$PPID"\n', { mode: 0o755 });
  assert.notEqual((await run(f, ['update', '--artifact', f.archive])).code, 0);
  const after = JSON.parse(await readFile(active, 'utf8'));
  assert.notEqual(after.releaseRoot, before.releaseRoot);
  assert.equal(JSON.parse((await run(f, ['status'])).stdout).ready, true);
  assert.equal((await readdir(join(f.managed, 'impeccable'))).length, 3);
});

test('unsupported platform and unavailable default fail clearly without allocating state', async t => {
  const f = await fixture(t);
  const missing = await run(f, ['install']);
  assert.notEqual(missing.code, 0); assert.match(missing.stderr, /distribution unpublished/);
  await writeFile(join(f.bin, 'uname'), '#!/bin/sh\nif [ "$1" = -s ]; then echo Linux; else echo aarch64; fi\n', { mode: 0o755 });
  const unsupported = await run(f, ['install', '--artifact', f.archive]);
  assert.notEqual(unsupported.code, 0); assert.match(unsupported.stderr, /Linux x86_64 only/);
  await assert.rejects(readdir(f.managed), { code: 'ENOENT' });
});

test('production installer cannot substitute historical local bytes for unverified CI pins', async t => {
  const f = await fixture(t), manager = join(f.root, 'unbootstrapped-manager');
  await writeFile(manager, source);
  const denied = await run({ ...f, manager }, ['install', '--artifact', f.archive]);
  assert.notEqual(denied.code, 0);
  assert.match(denied.stderr, /CI-produced Impeccable distribution has not been verified/);
  await assert.rejects(readdir(f.managed), { code: 'ENOENT' });
});

test('uninstall touches private component state only and missing status never uses PATH', async t => {
  const f = await fixture(t);
  const missing = JSON.parse((await run(f, ['status'])).stdout);
  assert.equal(missing.ready, false); assert.equal(missing.reason, 'not-installed');
  assert.equal((await run(f, ['install', '--artifact', f.archive])).code, 0);
  assert.equal((await run(f, ['uninstall'])).code, 0);
  await assert.rejects(readdir(join(f.managed, 'impeccable')), { code: 'ENOENT' });
  assert.equal((await run(f, ['uninstall'])).code, 0);
  assert.equal(await readFile(join(f.project, '.impeccable/live/journal.json'), 'utf8'), 'preserved project journal');
  await assert.rejects(readFile(join(f.root, 'impeccable-called')), { code: 'ENOENT' });
});
