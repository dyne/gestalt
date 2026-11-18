import assert from 'node:assert/strict';
import { copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { test } from 'node:test';
import { queryFixtureHooks } from './fixtures/native-hook-query.mjs';

const holderProgram = `
const fs = require('node:fs');
const started = Date.now();
const timer = setInterval(() => {
  const released = fs.existsSync('holder-release');
  if (!released && Date.now() - started < 10000) return;
  clearInterval(timer);
  process.exitCode = released ? 0 : 1;
}, 10);
process.once('exit', code => {
  fs.writeFileSync('holder-settlement.tmp', JSON.stringify({ pid: process.pid, code }));
  fs.renameSync('holder-settlement.tmp', 'holder-settlement.json');
});
fs.writeFileSync('holder-ready', String(process.pid));
`;

const appServer = `
const { spawn } = require('node:child_process');
const { createInterface } = require('node:readline');
const { existsSync, writeFileSync } = require('node:fs');
const { setTimeout: delay } = require('node:timers/promises');
const mode = process.env.GESTALT_QUERY_TEST_MODE;

async function serve() {
  const lines = createInterface({ input: process.stdin });
  for await (const line of lines) {
    const { id, method } = JSON.parse(line);
    if (method === 'initialize') {
      if (mode === 'early-exit') process.exit(23);
      console.log(JSON.stringify({ id, result: {} }));
    }
    if (method !== 'hooks/list') continue;
    if (mode.startsWith('held-')) {
      writeFileSync('holder-requested', '');
      const holder = spawn(process.execPath, ['-e', ${JSON.stringify(holderProgram)}],
        { stdio: ['ignore', 'ignore', process.stderr] });
      holder.once('error', error => writeFileSync('holder-spawn-failure', error.message));
      const deadline = Date.now() + 5000;
      while (!existsSync('holder-ready') && !existsSync('holder-spawn-failure') && Date.now() < deadline) await delay(10);
      if (!existsSync('holder-ready')) throw Error('Inherited pipe holder did not start');
      writeFileSync('holder-pid', String(holder.pid));
    }
    const response = mode === 'held-rpc-error'
      ? { id, error: { code: -32603, message: 'controlled hooks/list failure' } }
      : { id, result: { data: [] } };
    console.log(JSON.stringify(response));
  }
}
serve().catch(error => { console.error(error); process.exitCode = 25; });
`;

async function readOptional(file) {
  try { return await readFile(file, 'utf8'); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
}

async function fixture(t, mode) {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt-hook-query-'));
  const evidence = process.env.GESTALT_HOOK_QUERY_TEST_EVIDENCE;
  const receipt = path.join(root, 'query.json');
  let removable = false;
  t.after(async () => {
    if (!mode.startsWith('held-')) {
      const record = await readOptional(receipt);
      removable = record !== undefined && JSON.parse(record).close !== null;
    }
    if (evidence) {
      await mkdir(evidence, { recursive: true });
      for (const file of ['query.json', 'holder-settlement.json']) {
        const source = path.join(root, file);
        if (await readOptional(source) !== undefined) await copyFile(source, path.join(evidence, `${mode}-${file}`));
      }
    }
    if (removable) await rm(root, { recursive: true, force: true });
    else t.diagnostic(`Unsettled pipe fixture retained at ${root}`);
  });
  // The test Node process loads this script as the app-server argument.
  await writeFile(path.join(root, 'app-server'), appServer);
  const inspect = async () => JSON.parse(await readFile(receipt, 'utf8'));
  return {
    invoke: cleanupMs => queryFixtureHooks(process.execPath, { ...process.env, GESTALT_QUERY_TEST_MODE: mode },
      root, [], receipt, { cleanupMs }),
    inspect,
    read: name => readFile(path.join(root, name), 'utf8'),
    async releaseHolder() {
      await writeFile(path.join(root, 'holder-release'), 'release\n');
      const requested = await readOptional(path.join(root, 'holder-requested'));
      if (requested === undefined || await readOptional(path.join(root, 'holder-spawn-failure')) !== undefined) {
        assert.ok((await inspect()).close, 'Query closure is required when no pipe holder started');
        removable = true;
        return;
      }
      const deadline = Date.now() + 5000;
      const settlement = path.join(root, 'holder-settlement.json');
      let result;
      while ((result = await readOptional(settlement)) === undefined && Date.now() < deadline) await delay(10);
      assert.ok(result, 'Pipe holder must acknowledge release before fixture cleanup');
      const readyPid = Number(await readFile(path.join(root, 'holder-ready'), 'utf8'));
      assert.deepEqual(JSON.parse(result), { pid: readyPid, code: 0 });
      assert.ok((await inspect()).directExit, 'Retain the fixture unless the direct query process also exited');
      removable = true;
    },
  };
}

test('successful hook query records both replies and direct process closure', async t => {
  const f = await fixture(t, 'success');
  assert.deepEqual((await f.invoke(2000)).result.result, { data: [] });
  const observed = await f.inspect();
  assert.equal(observed.primaryFailure, null);
  assert.equal(observed.cleanupFailure, null);
  assert.ok(observed.directExit);
  assert.ok(observed.close);
  assert.equal(observed.closedBeforeDeadline, true);
  assert.equal(observed.events.filter(row => row.event === 'rpc-response').length, 2);
});

test('app-server exit before initialize reply is a primary failure with observed closure', async t => {
  const f = await fixture(t, 'early-exit');
  await assert.rejects(f.invoke(2000), /closed before initialize reply/);
  const observed = await f.inspect();
  assert.equal(observed.directExit.code, 23);
  assert.equal(observed.close.code, 23);
  assert.match(observed.primaryFailure.message, /initialize/);
  assert.equal(observed.cleanupFailure, null);
});

async function heldPipe(t, mode, expectedPrimary) {
  const f = await fixture(t, mode);
  try {
    const timeout = /cleanup: deadline reached/;
    await assert.rejects(f.invoke(1000), expectedPrimary ?? timeout);
    const observed = await f.inspect();
    assert.equal(await f.read('holder-ready'), await f.read('holder-pid'));
    assert.ok(observed.directExit);
    assert.equal(observed.close, null);
    assert.equal(observed.closedBeforeDeadline, false);
    assert.equal(observed.events.some(row => row.event === 'pipe-close' && row.pipe === 'stderr'), false);
    assert.match(observed.cleanupFailure.message, timeout);
    if (expectedPrimary) assert.match(observed.primaryFailure.message, expectedPrimary);
    else assert.equal(observed.primaryFailure, null);
  } finally {
    await f.releaseHolder();
  }
}

test('an inherited pipe that holds close cannot mask the primary RPC failure',
  t => heldPipe(t, 'held-rpc-error', /controlled hooks\/list failure/));

test('a successful RPC still fails if an inherited pipe prevents observed close',
  t => heldPipe(t, 'held-rpc-success', null));
