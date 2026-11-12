import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdtemp, open, readFile, rm, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {createInterface} from 'node:readline';
import test from 'node:test';
import {workerProgram,stopFixtureWorkers} from './fixture-workers.mjs';

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
const {writeFileSync,writeSync} = require('node:fs');
const {spawn} = require('node:child_process');
writeFileSync(${JSON.stringify(join(root, 'pid'))}, String(process.pid));
const mode = ${JSON.stringify(mode)};
if (mode === 'stubborn') { process.on('SIGTERM', () => {}); setInterval(() => {}, 1000); }
createInterface({input: process.stdin}).on('line', line => {
  const request = JSON.parse(line);
  const result = {pid: process.pid};
  if (mode === 'large-history' && request.method === 'thread/read') {
    const reply = Buffer.from(JSON.stringify({id:request.id,result:{history:'x'.repeat(2*1024*1024)}})+'\\n');
    let offset = 0;
    while (offset < reply.length) offset += writeSync(1,reply,offset,reply.length-offset);
    return;
  }
  if (mode === 'early-exit') process.exitCode = 7, setImmediate(() => process.exit(7));
  if (mode === 'inherited-pipe' || mode === 'closed-pipes') {
    const worker = spawn(process.execPath, [${JSON.stringify(join(root, 'worker.cjs'))}], {stdio: ['ignore', mode === 'inherited-pipe' ? 'inherit' : 'ignore', mode === 'inherited-pipe' ? 'inherit' : 'ignore', 'ipc']});
    worker.once('message', () => process.exit(7));
    result.descendantPid = worker.pid;
  }
  // Match a native stdout writer; console.log would itself set O_NONBLOCK.
  writeSync(1, JSON.stringify({id: request.id, result})+'\\n');
});
`, {mode: 0o755});
  await writeFile(join(root, 'worker.cjs'), workerProgram(join(root,'workers.jsonl'), "process.on('SIGTERM',()=>{});process.send('ready');setInterval(()=>{},1000);"));
  const child = spawn('bash', [manager, '__codex-app-server', native, '--stdio'], {
    env: {...process.env, GESTALT_HOME: root}, stdio: [inputFd ?? 'pipe', 'pipe', 'pipe'],
  });
  let diagnostics = '';
  child.stderr.on('data', bytes => { diagnostics = (diagnostics + bytes).slice(-2048); });
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
    await stopFixtureWorkers(join(root,'workers.jsonl'));
    await rm(root, {recursive: true, force: true});
  });
  if (inputFd !== undefined) return {child, exit};
  const response = once(lines, 'line');
  child.stdin.write(JSON.stringify({id: 1, method: 'initialize', params: {}}) + '\n');
  const [line] = await deadline(response).catch(error => {
    throw new Error(`${error.message}: ${diagnostics}`, {cause:error});
  });
  const message = JSON.parse(line);
  assert.equal(message.id, 1);
  return {child, exit, lines, nativePid: message.result.pid, descendantPid: message.result.descendantPid};
}

test('large native history replies arrive intact and leave the protocol usable', async t => {
  const {child, lines} = await fixture(t, 'large-history');
  const history = once(lines, 'line');
  child.stdin.write(JSON.stringify({id:2,method:'thread/read',params:{includeTurns:true}})+'\n');
  const [line] = await deadline(history);
  const response = JSON.parse(line);
  assert.equal(response.id, 2);
  assert.equal(response.result.history, 'x'.repeat(2*1024*1024));
  const next = once(lines, 'line');
  child.stdin.write(JSON.stringify({id:3,method:'config/read',params:{}})+'\n');
  assert.equal(JSON.parse((await deadline(next))[0]).id, 3);
});

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

test('native exit still reaps a resistant descendant with all output pipes closed', async t => {
  const {exit, nativePid, descendantPid} = await fixture(t, 'closed-pipes');
  assert.deepEqual(await deadline(exit), [7, null]);
  await assertStopped(nativePid);
  await assertStopped(descendantPid);
});

test('an uncatchable wrapper exit closes the native group lifetime pipe', async t => {
  const {child, exit, nativePid} = await fixture(t, 'stubborn');
  child.kill('SIGKILL');
  await deadline(exit);
  await assertStopped(nativePid);
});
