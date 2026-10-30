import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdtemp, open, readFile, rm, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {createInterface} from 'node:readline';
import test from 'node:test';

const manager = resolve(process.env.GESTALT_TEST_MANAGER_BIN ?? 'public/gestalt');

function deadline(promise, ms = 5000) {
  let timer;
  return Promise.race([promise, new Promise((_, reject) => {
    timer = setTimeout(() => reject(Error('process lifecycle deadline exceeded')), ms);
  })]).finally(() => clearTimeout(timer));
}

function killGroup(pid) {
  try { process.kill(-pid, 'SIGKILL'); } catch (error) { if (error.code !== 'ESRCH') throw error; }
}

async function assertStopped(pid) {
  const until = Date.now() + 2000;
  while (true) {
    try { process.kill(pid, 0); }
    catch (error) { if (error.code === 'ESRCH') return; throw error; }
    // A killed descendant can briefly await reaping by init after its parent
    // exits; it no longer runs or retains a resident Node runtime.
    try {
      if (/^\d+ \(.+\) Z /.test(await readFile(`/proc/${pid}/stat`, 'utf8'))) return;
    } catch (error) { if (error.code !== 'ENOENT') throw error; }
    assert.ok(Date.now() < until, `owned native process ${pid} survived wrapper shutdown`);
    await new Promise(resolve => setTimeout(resolve, 20));
  }
}

async function fixture(t, mode = 'normal', inputFd) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt-codex-lifecycle-'));
  const native = join(root, 'codex');
  await writeFile(native, `#!${process.execPath}
const {createInterface} = require('node:readline');
const {writeFileSync} = require('node:fs');
const {spawn} = require('node:child_process');
writeFileSync(${JSON.stringify(join(root, 'pid'))}, String(process.pid));
const mode = ${JSON.stringify(mode)};
if (mode === 'stubborn') { process.on('SIGTERM', () => {}); setInterval(() => {}, 1000); }
createInterface({input: process.stdin}).on('line', line => {
  const request = JSON.parse(line);
  const result = {pid: process.pid};
  if (mode === 'early-exit') process.exitCode = 7, setImmediate(() => process.exit(7));
  if (mode === 'inherited-pipe') {
    result.descendantPid = spawn(process.execPath, ['-e', "process.on('SIGTERM',()=>{});setInterval(()=>{},1000)"], {stdio: ['ignore', 'inherit', 'inherit']}).pid;
    setTimeout(() => process.exit(7), 100);
  }
  console.log(JSON.stringify({id: request.id, result}));
});
`, {mode: 0o755});
  const child = spawn('bash', [manager, '__codex-app-server', native, '--stdio'], {
    env: {...process.env, GESTALT_HOME: root}, stdio: [inputFd ?? 'pipe', 'pipe', 'pipe'],
  });
  child.stderr.resume();
  child.stdin?.on('error', () => {});
  const exit = once(child, 'exit');
  const lines = createInterface({input: child.stdout});
  t.after(async () => {
    child.stdin?.end();
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    await deadline(exit);
    try { killGroup(Number(await readFile(join(root, 'pid'), 'utf8'))); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    lines.close();
    await rm(root, {recursive: true, force: true});
  });
  if (inputFd !== undefined) return {child, exit};
  const response = once(lines, 'line');
  child.stdin.write(JSON.stringify({id: 1, method: 'initialize', params: {}}) + '\n');
  const [line] = await deadline(response);
  const message = JSON.parse(line);
  assert.equal(message.id, 1);
  return {child, exit, nativePid: message.result.pid, descendantPid: message.result.descendantPid};
}

for (const [signal, code] of [['SIGTERM', 143], ['SIGINT', 130]]) {
  test(`${signal} exits with caller input still open`, async t => {
    const {child, exit, nativePid} = await fixture(t);
    child.kill(signal);
    assert.deepEqual(await deadline(exit), [code, null]);
    assert.equal(child.stdin.writableEnded, false);
    await assertStopped(nativePid);
  });
}

test('EOF shuts down the managed runtime', async t => {
  const {child, exit, nativePid} = await fixture(t);
  child.stdin.end();
  assert.deepEqual(await deadline(exit), [0, null]);
  await assertStopped(nativePid);
});

test('redirected file and null-device input retain EOF shutdown', async t => {
  const root = await mkdtemp(join(tmpdir(), 'gestalt-codex-input-'));
  t.after(() => rm(root, {recursive: true, force: true}));
  const path = join(root, 'empty');
  await writeFile(path, '');
  for (const name of [path, '/dev/null']) {
    const input = await open(name, 'r');
    try {
      const {exit} = await fixture(t, 'normal', input.fd);
      assert.deepEqual(await deadline(exit), [0, null]);
    } finally {
      await input.close();
    }
  }
});

test('native exit releases the wrapper while caller input stays open', async t => {
  const {exit, nativePid} = await fixture(t, 'early-exit');
  assert.deepEqual(await deadline(exit), [7, null]);
  await assertStopped(nativePid);
});

test('shutdown deadline kills a native process ignoring SIGTERM', async t => {
  const {child, exit, nativePid} = await fixture(t, 'stubborn');
  child.kill('SIGTERM');
  assert.deepEqual(await deadline(exit), [143, null]);
  await assertStopped(nativePid);
});

test('native exit cannot leave a wrapper waiting on descendant output pipes', async t => {
  const {exit, nativePid, descendantPid} = await fixture(t, 'inherited-pipe');
  assert.deepEqual(await deadline(exit), [7, null]);
  await assertStopped(nativePid);
  await assertStopped(descendantPid);
});

test('repeated temporary readers exit without caller EOF', async t => {
  for (let i = 0; i < 20; i++) {
    const {child, exit, nativePid} = await fixture(t);
    child.kill('SIGTERM');
    assert.deepEqual(await deadline(exit), [143, null]);
    await assertStopped(nativePid);
  }
});
