import assert from 'node:assert/strict';
import { copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { PassThrough } from 'node:stream';
import test from 'node:test';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { NativeOwnedProcesses } from '../src/cli/platform/processes.ts';

const root = path.resolve(import.meta.dirname, '..');
const artifact = await mkdtemp(path.join(tmpdir(), 'gestalt-terminal-build-'));
let helper;
test.before(async () => {
  helper = await buildOwnedProcess(root, artifact);
  if (process.env.GESTALT_NATIVE_TEST_EVIDENCE) {
    const evidence = path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE);
    await mkdir(evidence, { recursive: true });
    const prefix = path.join(evidence, 'terminal-owned-process');
    await copyFile(`${helper}.provenance.json`, `${prefix}.provenance.json`);
    await copyFile(helper, process.platform === 'win32' ? `${prefix}.exe` : prefix);
  }
});
test.after(() => rm(artifact, { recursive: true, force: true }));
function bounded(promise, label) {
  let timer;
  return Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(`${label} did not settle`)), 15000); })]).finally(() => clearTimeout(timer));
}
async function terminal(t, source, options = {}) {
  const input = new PassThrough(), output = []; let text = '';
  const waiters = new Set();
  const lease = await new NativeOwnedProcesses(options.helper ?? helper).launchTerminal(process.execPath, {
    argv: ['-e', source], cwd: root, environment: { ...process.env, TERM: 'xterm-256color' },
  }, { input, output: async bytes => {
    output.push(Buffer.from(bytes)); text = Buffer.concat(output).toString().replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, '');
    for (const wake of waiters) wake();
    await options.outputGate?.(text);
  } }, { columns: 91, rows: 37 }, options.limits ?? { graceMs: 50, deadlineMs: 10000 });
  t.after(async () => {
    try { input.destroy(); await bounded(lease.stop(), 'terminal teardown'); await bounded(lease.completed, 'terminal helper close'); }
    finally {
      if (options.evidenceName && process.env.GESTALT_NATIVE_TEST_EVIDENCE) {
        const prefix = path.join(path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE), options.evidenceName);
        await writeFile(`${prefix}.stdout.bin`, Buffer.concat(output));
        await writeFile(`${prefix}.runtime.json`, JSON.stringify({ observedAt: new Date().toISOString(), source, helper: options.helper ?? helper,
          claim: lease.claim(), outputBytes: Buffer.concat(output).length, text }, null, 2) + '\n');
      }
    }
  });
  const until = (pattern) => bounded(new Promise((resolve, reject) => {
    const wake = () => { const match = text.match(pattern); if (match) { waiters.delete(wake); resolve(match); } };
    waiters.add(wake); wake();
    void lease.completed.then(claim => { if (waiters.delete(wake)) reject(new Error(`Terminal ended before ${pattern}: ${JSON.stringify(claim)} ${text}`)); });
  }), `terminal output ${pattern}`);
  return { lease, input, output, until, text: () => text };
}
function completedClaim(claim) {
  assert.equal(claim.kind, process.platform === 'darwin' ? 'unknown' : 'released', JSON.stringify(claim));
}
test('owned terminal has actual stdin/stdout/stderr terminals, initial size, resize and Unicode/control bytes', { timeout: 30000 }, async t => {
  const child = await terminal(t, `
    if(!process.stdin.isTTY||!process.stdout.isTTY||!process.stderr.isTTY)throw Error('actual terminals required');
    process.stdin.setRawMode(true);process.stdin.resume();
    console.log('SIZE:'+process.stdout.columns+':'+process.stdout.rows);
    process.stdout.on('resize',()=>console.log('RESIZED:'+process.stdout.columns+':'+process.stdout.rows));
    process.stderr.write('MERGED:é東京\\n');
    let bytes=Buffer.alloc(0);process.stdin.on('data',chunk=>{
      bytes=Buffer.concat([bytes,chunk]);if(bytes.includes(4)){console.log('INPUT:'+bytes.toString('base64'));process.exit(23);}
    });
  `);
  await child.until(/SIZE:91:37/); await child.until(/MERGED:é東京/);
  await assert.rejects(child.lease.resize({ columns: 0, rows: 41 }), /Invalid terminal columns/);
  assert.equal(child.lease.claim().kind, 'active');
  await child.lease.resize({ columns: 103, rows: 41 }); await child.until(/RESIZED:103:41/);
  const bytes = Buffer.from('é東京\x1b[A\x04'); child.input.write(bytes);
  const match = await child.until(/INPUT:([A-Za-z0-9+/=]+)/);
  assert.deepEqual(Buffer.from(match[1], 'base64'), bytes);
  assert.deepEqual(await bounded(child.lease.result, 'terminal result'), { kind: 'exit', code: 23 });
  completedClaim(await bounded(child.lease.completed, 'terminal completion'));
  await assert.rejects(child.lease.resize({ columns: 80, rows: 24 }), /no longer/);
});
test('owned terminal delivers a real interrupt control byte', { timeout: 30000 }, async t => {
  const child = await terminal(t, `
    process.on('SIGINT',()=>{console.log('INTERRUPTED');process.exit(31)});
    process.stdin.resume();console.log('INTERRUPT-READY');setInterval(()=>{},1000);
  `);
  await child.until(/INTERRUPT-READY/); child.input.write(Buffer.from([3]));
  await child.until(/INTERRUPTED/);
  assert.deepEqual(await bounded(child.lease.result, 'interrupt result'), { kind: 'exit', code: 31 });
  completedClaim(await bounded(child.lease.completed, 'interrupt cleanup'));
});
test('owned terminal transport EOF requests bounded stop and never fabricates macOS release', { timeout: 30000 }, async t => {
  const child = await terminal(t, `process.stdin.resume();console.log('EOF-READY');setInterval(()=>{},1000)`);
  await child.until(/EOF-READY/); child.input.end();
  completedClaim(await bounded(child.lease.completed, 'terminal EOF cleanup'));
});
test('owned terminal startup failure reports spawn error with conservative ownership', { timeout: 30000 }, async () => {
  const input = new PassThrough();
  const lease = await new NativeOwnedProcesses(helper).launchTerminal(path.join(artifact, 'missing-target'), {
    argv: [], cwd: root, environment: process.env,
  }, { input, output: async () => {} }, { columns: 80, rows: 24 });
  try {
    assert.equal((await bounded(lease.result, 'failed terminal launch result')).kind, 'spawn-error');
    completedClaim(await bounded(lease.completed, 'failed terminal launch settlement'));
  } finally { input.destroy(); await bounded(lease.stop(), 'failed terminal cleanup'); }
});
test('an applied resize without acknowledgement is bounded and remains unknown through concurrent stop', { timeout: 60000 }, async t => {
  const source = await readFile(path.join(root, 'src/cli/platform/native/owned-process.c'), 'utf8');
  const acknowledgement = `message('Z', "")`;
  assert.equal(source.split(acknowledgement).length, 3, 'Inject exactly Unix and Windows resize acknowledgement sites');
  const injected = source.replaceAll(acknowledgement, '0 /* test-only withheld resize acknowledgement */');
  const fixtureRoot = path.join(artifact, 'no-resize-ack'), file = path.join(fixtureRoot, 'src/cli/platform/native/owned-process.c');
  await mkdir(path.dirname(file), { recursive: true }); await writeFile(file, injected);
  const fixtureHelper = await buildOwnedProcess(fixtureRoot, path.join(fixtureRoot, 'bin'));
  if (process.env.GESTALT_NATIVE_TEST_EVIDENCE) {
    const prefix = path.join(path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE), 'terminal-resize-no-ack');
    await copyFile(file, `${prefix}.c`); await copyFile(fixtureHelper, process.platform === 'win32' ? `${prefix}.exe` : prefix);
    await copyFile(`${fixtureHelper}.provenance.json`, `${prefix}.provenance.json`);
  }
  for (const concurrentStop of [false, true]) {
    const runtimeFile = path.join(artifact, `resize-runtime-${concurrentStop}.json`);
    const evidenceName = `terminal-resize-no-ack-${concurrentStop ? 'concurrent' : 'serial'}`;
    const child = await terminal(t, `
      const save=(phase)=>require('node:fs').writeFileSync(${JSON.stringify(runtimeFile)},JSON.stringify({phase,observedAt:new Date().toISOString(),node:process.version,versions:process.versions,rawMode:process.stdin.isRaw,tty:[!!process.stdin.isTTY,!!process.stdout.isTTY],columns:process.stdout.columns,rows:process.stdout.rows}));
      if(!process.stdin.isTTY||!process.stdout.isTTY)throw Error('actual resize fixture terminal required');
      process.stdin.setRawMode(true);process.stdin.resume();
      process.stdout.on('resize',()=>{save('applied');console.log('APPLIED:'+process.stdout.columns+':'+process.stdout.rows)});
      save('ready');
      console.log('NO-ACK-READY');setInterval(()=>{},1000);
    `, { helper: fixtureHelper, limits: { graceMs: 0, deadlineMs: 500 }, evidenceName });
    t.after(async () => {
      if (process.env.GESTALT_NATIVE_TEST_EVIDENCE) {
        const target = path.join(path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE), `${evidenceName}.target.json`);
        try { await copyFile(runtimeFile, target); } catch (error) { if (error.code !== 'ENOENT') throw error; }
      }
    });
    await child.until(/NO-ACK-READY/);
    const rejected = assert.rejects(child.lease.resize({ columns: 102, rows: 40 }), /resize acknowledgement/);
    await child.until(/APPLIED:102:40/);
    assert.equal(child.lease.claim().kind, 'active', 'Actual resize application must precede the unacknowledged deadline');
    const applied = JSON.parse(await readFile(runtimeFile, 'utf8'));
    assert.equal(applied.phase, 'applied'); assert.equal(applied.rawMode, true);
    assert.deepEqual([applied.columns, applied.rows], [102, 40]);
    if (concurrentStop) await bounded(child.lease.stop(), 'stop with pending resize');
    await bounded(rejected, 'missing resize acknowledgement rejection');
    const claim = await bounded(child.lease.completed, 'no-ack helper close');
    assert.equal(claim.kind, 'unknown', JSON.stringify(claim)); assert.match(claim.reason, /resize acknowledgement/);
  }
});
test('terminal backpressure leaves resize and stop bounded while stalled streams retain unknown', { timeout: 30000 }, async t => {
  let release, reached, held = false;
  const gate = new Promise(resolve => { release = resolve; }), boundary = new Promise(resolve => { reached = resolve; });
  const child = await terminal(t, `
    process.stdin.setRawMode(true);process.stdin.pause();console.log('STALL-READY');
    setTimeout(()=>process.stdout.write('FLOOD'+ 'x'.repeat(1024*1024)),0);setInterval(()=>{},1000);
  `, { limits: { graceMs: 0, deadlineMs: 300 }, outputGate: async text => {
    if (!held && text.includes('FLOOD')) { held = true; reached(); await gate; }
  } });
  t.after(() => release());
  try {
    await child.until(/STALL-READY/); child.input.write(Buffer.alloc(1024 * 1024, 120));
    await bounded(boundary, 'actual held terminal output');
    const rejected = assert.rejects(child.lease.resize({ columns: 80, rows: 24 }), /resize acknowledgement/);
    await bounded(rejected, 'backpressured resize deadline');
    assert.equal((await bounded(child.lease.stop(), 'backpressured stop')).kind, 'unknown');
  } finally { release(); }
  assert.equal((await bounded(child.lease.completed, 'released terminal output boundary')).kind, 'unknown');
});
