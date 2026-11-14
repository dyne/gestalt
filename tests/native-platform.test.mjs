import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { chmod, link, lstat, mkdir, mkdtemp, readFile, readdir, realpath, rename, rm, symlink, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { watch } from 'node:fs';
import test from 'node:test';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { NativePaths } from '../src/cli/platform/paths.ts';
import { NativeExecutables, forwardCommand } from '../src/cli/platform/executables.ts';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { NativeLocalIpc, localEndpoint } from '../src/cli/platform/ipc.ts';
import { windowsSystem } from '../src/cli/platform/windows-system.ts';

const root = path.resolve(import.meta.dirname, '..');
const windows = process.platform === 'win32';
const artifact = await mkdtemp(path.join(tmpdir(), 'gestalt-platform-build-'));
let peerProbe;
test.before(async () => { peerProbe = await buildNativePlatform(root, artifact); });
test.after(() => rm(artifact, { recursive: true, force: true }));

async function fixture(t) {
  const directory = await mkdtemp(path.join(tmpdir(), 'gn-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const storage = await NativePrivateStorage.open(path.join(directory, 'private'), peerProbe);
  return { directory, storage, managed: storage.root };
}
async function forward(executable, argv, cwd, input = Buffer.from([0, 255, 10, 13, 128]), environment = process.env) {
  const out = [], err = [];
  const result = await forwardCommand(executable, { argv, cwd, environment }, {
    stdin: (async function* () { yield input; })(),
    stdout: async chunk => { out.push(chunk); }, stderr: async chunk => { err.push(chunk); },
  });
  return { result, stdout: Buffer.concat(out), stderr: Buffer.concat(err) };
}

test('native binary forwarding preserves empty, Unicode, spaces and shell metacharacters with binary stdin and nonzero exit', async t => {
  const { directory } = await fixture(t);
  const entry = path.join(directory, 'worker space é.mjs');
  await writeFile(entry, `const chunks=[];for await(const b of process.stdin)chunks.push(b);process.stdout.write(JSON.stringify({argv:process.argv.slice(2),cwd:process.cwd(),input:Buffer.concat(chunks).toString('hex'),env:process.env.RETAINED}));process.stderr.write(Buffer.from([255,0]));process.exitCode=23;`);
  const argv = ['', 'a b', '東京 é', '& echo injected', '|', '>', '<', '^', '%PATH%', '!PATH!', '$HOME', '$(touch escaped)', '`id`', '"quoted"', 'trailing\\', '\r\n'];
  const result = await forward(process.execPath, [entry, ...argv], directory, undefined, { ...process.env, RETAINED: 'kept' });
  assert.deepEqual(result.result, { kind: 'exit', code: 23 });
  assert.deepEqual(JSON.parse(result.stdout), { argv, cwd: await realpath(directory), input: '00ff0a0d80', env: 'kept' });
  assert.deepEqual(result.stderr, Buffer.from([255, 0]));
  assert.deepEqual((await readdir(directory)).sort(), ['private', 'worker space é.mjs']);
});

test('PATH resolution uses native precedence, excludes implicit cwd and distinguishes spawn errors', async t => {
  const { directory } = await fixture(t);
  const resolver = new NativeExecutables();
  assert.equal(await resolver.resolve('absent-native-tool', { PATH: `${path.delimiter}.${path.delimiter}${directory}` }), null);
  const resolved = await resolver.resolve(process.execPath, {});
  assert.equal(resolved, await realpath(process.execPath));
  assert.equal(await resolver.resolve(path.basename(process.execPath), { PATH: path.dirname(process.execPath) }), await realpath(process.execPath));
  assert.equal((await forward(path.join(directory, 'absent'), [], directory)).result.kind, 'spawn-error');
  await assert.rejects(resolver.resolve('./relative', process.env), /absolute/);
});

test('real npm-generated native shim forwards argv/stdin/status without shell evaluation', async t => {
  const { directory } = await fixture(t);
  const packageDirectory = path.join(directory, 'package space é');
  const consumer = path.join(directory, 'consumer space é');
  await mkdir(packageDirectory); await mkdir(consumer);
  await writeFile(path.join(packageDirectory, 'package.json'), JSON.stringify({ name: 'native-forward-fixture', version: '1.0.0', bin: { 'native-forward-fixture': 'entry.mjs' } }));
  await writeFile(path.join(packageDirectory, 'entry.mjs'), '#!/usr/bin/env node\nconst chunks=[];for await(const b of process.stdin)chunks.push(b);console.log(JSON.stringify({argv:process.argv.slice(2),input:Buffer.concat(chunks).toString("hex")}));process.exitCode=29;\n');
  await chmod(path.join(packageDirectory, 'entry.mjs'), 0o755);
  const npmCli = path.join(path.dirname(process.execPath), windows ? 'node_modules/npm/bin/npm-cli.js' : '../lib/node_modules/npm/bin/npm-cli.js');
  // npm_execpath points at the real JS entry when run through the contract script.
  const npmEntry = process.env.npm_execpath || npmCli;
  const installed = spawnSync(process.execPath, [npmEntry, 'install', '--prefix', consumer, '--ignore-scripts', '--no-audit', '--no-fund', '--package-lock=false', '--cache', path.join(directory, 'npm-cache'), packageDirectory], { encoding: 'utf8', timeout: 30000, shell: false });
  assert.equal(installed.status, 0, installed.stderr);
  const executable = await new NativeExecutables().resolve('native-forward-fixture', { PATH: path.join(consumer, 'node_modules/.bin'), PATHEXT: '.CMD;.EXE' });
  assert.ok(executable);
  if (windows) assert.match(executable, /\.cmd$/i);
  const argv = ['', 'space é', '&echo should-not-run', '%PATH%', '!PATH!', '|', '"', '^', 'foo\\'];
  const result = await forward(executable, argv, consumer);
  assert.deepEqual(result.result, { kind: 'exit', code: 29 });
  assert.deepEqual(JSON.parse(result.stdout), { argv, input: '00ff0a0d80' });
});

test('unrecognized batch source fails closed instead of interpolating caller arguments', async t => {
  const { directory } = await fixture(t);
  const entry = path.join(directory, 'arbitrary.cmd');
  await writeFile(entry, '@echo unsafe %*\r\n');
  await assert.rejects(forward(entry, ['& echo unsafe'], directory), /Unsupported batch shim/);
});

test('canonical containment refuses lexical escapes, prefix siblings, and external redirects', async t => {
  const { directory, managed } = await fixture(t);
  const paths = new NativePaths();
  await paths.assertContained(managed, path.join(managed, 'new', 'file'));
  await assert.rejects(paths.assertContained(managed, path.join(managed, '..', 'outside')), /outside/);
  await assert.rejects(paths.assertContained(managed, `${managed}-sibling`), /outside/);
  const outside = path.join(directory, 'outside'); await mkdir(outside);
  const redirect = path.join(managed, 'redirect');
  await symlink(outside, redirect, windows ? 'junction' : 'dir');
  await assert.rejects(paths.assertContained(managed, path.join(redirect, 'new')), /outside/);
});

test('private storage refuses redirects, hardlinks, and writes outside its root while retaining unmanaged files', async t => {
  const { directory, storage, managed } = await fixture(t);
  const unmanaged = path.join(directory, 'unmanaged'); await writeFile(unmanaged, 'preserve', { mode: 0o600 });
  await assert.rejects(storage.replaceFile(unmanaged, Buffer.from('changed')), /outside/);
  const outside = path.join(directory, 'outside'); await mkdir(outside);
  await symlink(outside, path.join(managed, 'redirect'), windows ? 'junction' : 'dir');
  await assert.rejects(storage.replaceFile(path.join(managed, 'redirect', 'file'), Buffer.from('changed')), /outside|Redirected/);
  const file = path.join(managed, 'linked'); await writeFile(file, 'preserve', { mode: 0o600 });
  await link(file, path.join(managed, 'alias'));
  await assert.rejects(storage.replaceFile(file, Buffer.from('changed')), /Hardlinked/);
  assert.equal(await readFile(file, 'utf8'), 'preserve');
  assert.equal(await readFile(unmanaged, 'utf8'), 'preserve');
  await assert.rejects(NativePrivateStorage.open(path.join(managed, 'redirect')), /Redirected/);
});

test('replacing the managed root object cannot redirect an existing storage adapter', async t => {
  const { storage, managed } = await fixture(t);
  const moved = `${managed}-original`;
  await rename(managed, moved);
  const replacement = await NativePrivateStorage.open(managed, peerProbe);
  await replacement.replaceFile(path.join(managed, 'unrelated'), Buffer.from('preserve'));
  await assert.rejects(storage.replaceFile(path.join(managed, 'unrelated'), Buffer.from('redirected')), /root identity changed/);
  assert.equal(await readFile(path.join(managed, 'unrelated'), 'utf8'), 'preserve');
});

test('private destination-local replacement retains the prior file on contention and publishes complete bytes', async t => {
  const { storage, managed } = await fixture(t);
  const target = path.join(managed, 'settings é.json');
  await storage.replaceFile(target, Buffer.from('original'));
  const lock = await storage.acquireLock(`${target}.lock`);
  await assert.rejects(storage.replaceFile(target, Buffer.from('blocked')), /EEXIST/);
  assert.equal(await readFile(target, 'utf8'), 'original');
  await lock.release(); await lock.release();
  await storage.replaceFile(target, Buffer.alloc(65536, 23));
  assert.deepEqual(await readFile(target), Buffer.alloc(65536, 23));
  await storage.assertPrivate(target, 'file');
  assert.deepEqual(await readdir(managed), ['settings é.json']);
});

test('exclusive lock contention is enforced across real processes; controller crash never authorizes stale-lock removal', async t => {
  const { storage, managed } = await fixture(t);
  const target = path.join(managed, 'transaction');
  await storage.replaceFile(target, Buffer.from('working'));
  const storageUrl = pathToFileURL(path.join(root, 'src/cli/platform/storage.ts')).href;
  const child = spawn(process.execPath, ['--input-type=module', '-e', `import {NativePrivateStorage} from ${JSON.stringify(storageUrl)};const s=await NativePrivateStorage.open(process.argv[1],process.argv[3]);await s.acquireLock(process.argv[2]);console.log('LOCKED');setInterval(()=>{},1000);`, managed, `${target}.lock`, peerProbe ?? ''], { stdio: ['ignore', 'pipe', 'pipe'] });
  t.after(async () => { if (child.exitCode === null && child.signalCode === null) { child.kill('SIGKILL'); await once(child, 'close'); } });
  const ready = await Promise.race([once(child.stdout, 'data'), once(child, 'close').then(([code]) => { throw new Error(`lock fixture exited ${code}`); })]);
  assert.match(String(ready[0]), /LOCKED/);
  await assert.rejects(storage.acquireLock(`${target}.lock`), /EEXIST/);
  child.kill('SIGKILL'); await once(child, 'close');
  await assert.rejects(storage.replaceFile(target, Buffer.from('lost')), /EEXIST/);
  assert.equal(await readFile(target, 'utf8'), 'working');
  assert.ok((await lstat(`${target}.lock`)).isFile());
});

test('an abandoned stage and unrelated entries are retained during the next atomic replacement', async t => {
  const { storage, managed } = await fixture(t);
  await writeFile(path.join(managed, '.settings.interrupted.stage'), 'incomplete', { mode: 0o600 });
  await writeFile(path.join(managed, 'unmanaged'), 'keep', { mode: 0o600 });
  await storage.replaceFile(path.join(managed, 'settings'), Buffer.from('complete'));
  assert.equal(await readFile(path.join(managed, '.settings.interrupted.stage'), 'utf8'), 'incomplete');
  assert.equal(await readFile(path.join(managed, 'unmanaged'), 'utf8'), 'keep');
  assert.equal(await readFile(path.join(managed, 'settings'), 'utf8'), 'complete');
});

test('an actual interrupted staged write retains the working destination and refuses automatic stale-lock recovery', { timeout: 120000 }, async t => {
  const { storage, managed } = await fixture(t);
  const target = path.join(managed, 'state');
  await storage.replaceFile(target, Buffer.from('working'));
  const storageUrl = pathToFileURL(path.join(root, 'src/cli/platform/storage.ts')).href;
  let child;
  const watcher = watch(managed);
  const staged = new Promise((resolve, reject) => {
    watcher.on('error', reject);
    watcher.on('change', (_event, name) => {
      if (String(name).endsWith('.stage')) { child.kill('SIGKILL'); resolve(String(name)); }
    });
  });
  t.after(async () => { watcher.close(); if (child && child.exitCode === null && child.signalCode === null) { child.kill('SIGKILL'); await once(child, 'close'); } });
  child = spawn(process.execPath, ['--input-type=module', '-e', `import {NativePrivateStorage} from ${JSON.stringify(storageUrl)};const s=await NativePrivateStorage.open(process.argv[1],process.argv[3]);await s.replaceFile(process.argv[2],Buffer.alloc(64*1024*1024,0x5a));`, managed, target, peerProbe ?? ''], { stdio: ['ignore', 'ignore', 'pipe'] });
  const exited = once(child, 'close');
  const stage = await Promise.race([staged, exited.then(([code]) => { throw new Error(`stage fixture exited before interruption: ${code}`); })]);
  await exited; watcher.close();
  assert.equal(await readFile(target, 'utf8'), 'working');
  assert.ok((await lstat(path.join(managed, stage))).isFile());
  await assert.rejects(storage.replaceFile(target, Buffer.from('unsafe recovery')), /EEXIST/);
});

test('native long paths and Unicode names retain private storage contracts', async t => {
  const { storage, managed } = await fixture(t);
  let nested = managed;
  for (let index = 0; index < 5; index++) { nested = path.join(nested, `${index}-${'a'.repeat(48)}`); await storage.createDirectory(nested); }
  const target = path.join(nested, '東京 é.txt');
  assert.ok(target.length > 260);
  await storage.replaceFile(target, Buffer.from('long path'));
  assert.equal(await readFile(target, 'utf8'), 'long path');
  await storage.assertPrivate(target, 'file');
});

test('Unix storage rejects permissive existing directories without changing permissions or data', { skip: windows }, async t => {
  const { storage, managed } = await fixture(t);
  const exposed = path.join(managed, 'exposed'); await mkdir(exposed, { mode: 0o755 }); await chmod(exposed, 0o755);
  await assert.rejects(storage.assertPrivate(exposed, 'directory'), /not private/);
  assert.equal((await lstat(exposed)).mode & 0o777, 0o755);
});

test('macOS rejects an extended allow ACL even when POSIX mode bits remain private', { skip: process.platform !== 'darwin' }, async t => {
  const { storage, managed } = await fixture(t);
  const exposed = path.join(managed, 'extended-acl'); await storage.createDirectory(exposed);
  const changed = spawnSync('/bin/chmod', ['+a', 'everyone allow read', exposed], { encoding: 'utf8' });
  assert.equal(changed.status, 0, changed.stderr);
  assert.equal((await lstat(exposed)).mode & 0o077, 0);
  await assert.rejects(storage.assertPrivate(exposed, 'directory'), /macOS ACL validation failed/);
});

test('Unix IPC validates credentials on the connected socket, carries binary protocol, and rejects mismatched creation identity', { skip: windows }, async t => {
  const { managed } = await fixture(t);
  // Relative AF_UNIX names use a private child cwd without changing inherited
  // TMPDIR or allocating scratch outside the caller's session-owned directory.
  const result = spawnSync(process.execPath, [path.join(root, 'tests/fixtures/native-platform-ipc.mjs'), managed, peerProbe], { cwd: managed, encoding: 'utf8', timeout: 10000 });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /contracts passed/);
});

test('Windows rejects drive-relative/device/ADS paths and accepts canonical case aliases and actual junction containment', { skip: !windows }, async t => {
  const { managed, storage } = await fixture(t);
  const paths = new NativePaths();
  for (const value of ['C:relative', '\\\\.\\PhysicalDrive0', `${managed}\\file:stream`, `${managed}\\NUL`, `${managed}\\trailing. `])
    await assert.rejects(paths.canonical(value), /Windows|native/);
  await paths.assertContained(managed, path.join(managed.toUpperCase(), 'new'));
  await storage.createDirectory(path.join(managed, 'real'));
  await symlink(path.join(managed, 'real'), path.join(managed, 'alias'), 'junction');
  await paths.assertContained(managed, path.join(managed, 'alias', 'new'));
  await assert.rejects(storage.replaceFile(path.join(managed, 'alias', 'file'), Buffer.from('bad')), /Redirected|Reparse/);
  await assert.rejects(paths.assertContained(managed, '\\\\different-host\\share\\file'), /outside/);
});

test('Windows ACL validation rejects another principal while preserving an existing home', { skip: !windows }, async t => {
  const { directory, storage, managed } = await fixture(t);
  await storage.assertPrivate(managed, 'directory');
  const exposed = path.join(directory, 'exposed'); await mkdir(exposed);
  await writeFile(path.join(exposed, 'unmanaged'), 'retain');
  await assert.rejects(NativePrivateStorage.open(exposed), /ACL|owner|control/);
  assert.equal(await readFile(path.join(exposed, 'unmanaged'), 'utf8'), 'retain');
  await assert.rejects(windowsSystem({ operation: 'assert-private', path: exposed }), /ACL|owner|control/);
});

test('Windows named-pipe client checks the same handle ACL and complete server identity before binary protocol', { skip: !windows, timeout: 120000 }, async t => {
  const { storage, managed } = await fixture(t);
  const endpoint = localEndpoint(managed);
  const name = endpoint.slice('\\\\.\\pipe\\'.length);
  const script = await readFile(path.join(root, 'tests/fixtures/native-platform-pipe.ps1'), 'utf8');
  const executable = path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe');
  const child = spawn(executable, ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], {
    stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, GESTALT_PIPE_FIXTURE: Buffer.from(JSON.stringify({ name })).toString('base64') },
  });
  const childResult = once(child, 'close');
  let diagnostic = ''; child.stderr.on('data', bytes => { diagnostic += bytes; });
  t.after(async () => { if (child.exitCode === null && child.signalCode === null) { child.kill(); await once(child, 'close'); } });
  const metadata = await Promise.race([once(child.stdout, 'data'), once(child, 'close').then(([code]) => { throw new Error(`pipe fixture ${code}: ${diagnostic}`); })]);
  const owner = JSON.parse(String(metadata[0]));
  const ipc = new NativeLocalIpc(storage);
  await assert.rejects(ipc.connect(endpoint, { ...owner, started: 'stale' }), /creation identity/);
  const channel = await ipc.connect(endpoint, owner);
  await channel.stdout(Buffer.from([0, 255, 13, 10]));
  const bytes = []; for await (const chunk of channel.stdin) bytes.push(chunk);
  assert.deepEqual(Buffer.concat(bytes), Buffer.from([0, 255, 13, 10]));
  const [code] = await childResult;
  assert.equal(code, 0, diagnostic);
  await assert.rejects(ipc.connect('\\\\remote\\pipe\\not-local', owner), /local managed/);
});

test('Windows runner records a standard-user token for native acceptance', { skip: !windows }, async () => {
  const script = "$p=New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent());if($p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){[Console]::Error.WriteLine('Native acceptance requires a standard-user token');exit 1};[Console]::Out.WriteLine('standard-user')";
  const result = spawnSync(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'), ['-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], { encoding: 'utf8', timeout: 15000 });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /standard-user/);
});
