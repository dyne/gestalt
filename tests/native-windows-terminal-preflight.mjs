// Explicit Windows correction lane. Variants are diagnostics, never product.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, fsyncSync, openSync, writeSync } from 'node:fs';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { encodeFrame, decodeFrames, terminalVariants } from './fixtures/native-windows-terminal-wire.mjs';
import { stageOwnedProcessSource } from './fixtures/stage-owned-process-source.mjs';

assert.equal(process.platform, 'win32', 'Preflight requires actual Windows, not mocked platform');
const root = path.resolve(import.meta.dirname, '..');
const evidence = path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE ?? 'native-evidence', 'windows-terminal-preflight');
await mkdir(evidence, { recursive: true, mode: 0o700 });
function progress(event, details = {}) {
  const fd = openSync(path.join(evidence, 'progress.jsonl'), 'a');
  try { writeSync(fd, JSON.stringify({ observedAt: new Date().toISOString(), harnessPid: process.pid, event, ...details }) + '\n'); fsyncSync(fd); }
  finally { closeSync(fd); }
}
progress('harness-start');
process.on('exit', code => progress('harness-process-exit', { code }));
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const sourcePath = path.join(root, 'src/cli/platform/native/owned-process.c');
const original = await readFile(sourcePath, 'utf8');
const variants = terminalVariants(original);
const manifest = { kind: 'standard-user-windows-terminal-preflight', observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version,
  sourcePath, sourceSha256: digest(Buffer.from(original)), cases: [], builds: [], failures: [], fullNativeSuite: 'not executed by this script', productionSelectionChanged: false };
async function build(name, source, role, origin) {
  progress('build-before', { name, role, origin });
  const location = path.join(evidence, 'builds', name);
  const fixtureSource = await stageOwnedProcessSource(root, location, source);
  const executable = await buildOwnedProcess(location, path.join(location, 'bin'));
  manifest.builds.push({ name, role, origin, fixtureSource, sourceSha256: digest(Buffer.from(source)), executable,
    provenance: JSON.parse(await readFile(`${executable}.provenance.json`, 'utf8')) });
  await writeFile(path.join(evidence, 'manifest.partial.json'), JSON.stringify(manifest, null, 2) + '\n');
  progress('build-after', { name, executable });
  return executable;
}
async function scenario(helper, variant, target, transport, stop = false) {
  const name = `${variant}-${target}-${transport}${stop ? '-stop' : ''}`, directory = path.join(evidence, name);
  await mkdir(directory, { recursive: true }); const reportPath = path.join(directory, 'target.json');
  progress('case-before', { name, helper, target, transport, stop });
  const record = { name, observedAt: new Date().toISOString(), helper, target, transport, stop, events: [], pass: false };
  const args = ['--controller', String(process.pid), '--grace', '50', ...(transport === 'terminal' ? ['--terminal', '91', '37'] : []), '--',
    ...(target === 'c' ? [targetExecutable, reportPath, transport] : [process.execPath, path.join(root, 'tests/fixtures/native-windows-terminal-node.mjs'), reportPath, stop ? 'hold' : 'input'])];
  const child = spawn(helper, args, { shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
  record.command = [helper, ...args]; let raw = Buffer.alloc(0), errors = Buffer.alloc(0), closed = false, childError;
  child.on('spawn', () => progress('helper-spawn', { name, pid: child.pid }));
  child.on('exit', (code, signal) => progress('helper-process-exit', { name, code, signal, streamCloseObserved: closed }));
  let resolveClose; const close = new Promise(resolve => { resolveClose = resolve; });
  child.stdin.on('error', error => { record.inputError = error.message; });
  child.on('error', error => { childError = error.message; });
  child.stdout.on('data', bytes => {
    if (raw.length + bytes.length > 1024 * 1024) { record.outputOverflow = true; child.kill(); } else raw = Buffer.concat([raw, bytes]);
  });
  child.stderr.on('data', bytes => { if (errors.length + bytes.length <= 1024 * 1024) errors = Buffer.concat([errors, bytes]); else record.stderrOverflow = true; });
  child.on('close', (code, signal) => { closed = true; record.helperExit = { code, signal }; progress('helper-stream-close', { name, code, signal }); resolveClose(); });
  const start = Date.now();
  const until = async (predicate, label, limit = 10000) => {
    const end = Date.now() + limit;
    while (Date.now() < end) {
      if (predicate()) { record.events.push({ label, elapsedMs: Date.now() - start }); return; }
      if (closed) throw Error(`Helper closed before ${label}: ${JSON.stringify(record.helperExit)}`);
      await delay(10);
    }
    throw Error(`Bounded preflight missing ${label}`);
  };
  const frame = kind => decodeFrames(raw).frames.find(item => item.kind === kind);
  let targetReport;
  const loadReport = async () => { try { targetReport = JSON.parse(await readFile(reportPath, 'utf8')); return true; } catch { return false; } };
  try {
    child.stdin.write(encodeFrame('L'));
    // Independent report survives stdout/stderr accidentally entering control.
    const reportEnd = Date.now() + 10000;
    while (!(await loadReport()) && Date.now() < reportEnd && !closed) await delay(10);
    await loadReport(); assert.ok(targetReport, 'Actual target must write independent Win32/Node report');
    record.initialTarget = targetReport;
    progress('target-report-observed', { name, phase: targetReport.phase });
    const tty = target === 'c' ? targetReport.descriptors.map(d => d.console && d.crtIsatty !== 0) : targetReport.tty;
    if (target === 'c') {
      assert.equal(targetReport.tokenError, 0); assert.equal(targetReport.elevated, 0);
      assert.equal(targetReport.admin, false); assert.equal(targetReport.elevationType, 1, 'Actual standard-user token');
    }
    assert.deepEqual(tty, transport === 'terminal' ? [true, true, true] : [false, false, false], 'Actual three descriptors');
    if (transport === 'terminal') assert.deepEqual({ columns: targetReport.initialSize.columns, rows: targetReport.initialSize.rows }, { columns: 91, rows: 37 });
    if (transport === 'terminal') {
      const end = Date.now() + 10000;
      while (targetReport.phase !== 'ready' && Date.now() < end && !closed) { await delay(10); await loadReport(); }
      assert.equal(targetReport.phase, 'ready', 'Input starts only after actual raw-mode readiness');
      if (target === 'c') assert.equal(targetReport.modeSet, true);
    }
    await until(() => Boolean(frame('R')) || Boolean(decodeFrames(raw).error), 'ready or corrupt frame');
    assert.equal(decodeFrames(raw).error, undefined, 'Private ownership protocol must remain framed');
    assert.equal(frame('R').bytes.toString(), 'windows-job');
    if (stop) child.stdin.write(encodeFrame('S'));
    else {
      if (transport === 'terminal') {
        const size = Buffer.alloc(4); size.writeUInt16LE(103, 0); size.writeUInt16LE(41, 2); child.stdin.write(encodeFrame('W', size));
        await until(() => Boolean(frame('Z')), 'resize acknowledgement');
      }
      child.stdin.write(encodeFrame('I', Buffer.from('é東京\x04')));
    }
    await until(() => closed, 'helper closed');
    const parsed = decodeFrames(raw);
    assert.equal(parsed.error, undefined); assert.equal(parsed.pendingBytes, 0); assert.ok(!record.outputOverflow && !record.stderrOverflow);
    assert.equal(childError, undefined); assert.deepEqual(record.helperExit, { code: 0, signal: null });
    assert.equal(frame('D')?.bytes.toString(), 'windows-job', 'Release requires helper protocol proof');
    assert.ok(frame('F')); assert.ok(!frame('X') && !frame('U'));
    if (!stop) {
      assert.equal(frame('F').bytes.toString(), 'exit:23'); await loadReport();
      assert.equal(targetReport.phase, 'input-complete');
      if (target === 'c') assert.deepEqual(targetReport.inputCodeUnits, [233, 26481, 20140, 4]);
      else assert.equal(targetReport.inputHex, Buffer.from('é東京\x04').toString('hex'));
      if (transport === 'terminal') assert.deepEqual({ columns: targetReport.resized.columns, rows: targetReport.resized.rows }, { columns: 103, rows: 41 });
      const output = Buffer.concat(parsed.frames.filter(f => 'OB'.includes(f.kind)).map(f => f.bytes)).toString();
      assert.match(output, /(?:TARGET|NODE)_STDOUT_READY/); assert.match(output, /(?:TARGET|NODE)_STDERR_READY/);
      if (transport === 'terminal') assert.equal(parsed.frames.filter(f => f.kind === 'B').length, 0, 'ConPTY merges both output descriptors');
    }
    record.pass = true;
  } catch (error) { record.error = { message: error.message, stack: error.stack }; }
  finally {
    progress('case-teardown-before', { name, closed, error: record.error?.message });
    if (!closed) {
      child.stdin.destroy(); await Promise.race([close, delay(1000)]);
      if (!closed) { record.forcedHelperTeardown = true; child.kill(); await Promise.race([close, delay(5000)]); }
    }
    if (!closed) {
      record.cleanupUnknown = true; child.stdout.destroy(); child.stderr.destroy(); child.unref();
    }
    record.teardownClosed = closed; record.elapsedMs = Date.now() - start;
    await loadReport(); record.finalTarget = targetReport ?? null;
    const parsed = decodeFrames(raw); record.protocol = { ...parsed, frames: parsed.frames.map(f => ({ kind: f.kind, bytes: f.bytes.length, text: 'OB'.includes(f.kind) ? undefined : f.bytes.toString() })) };
    for (const [file, bytes] of [['stdout.bin', raw], ['stderr.bin', errors]]) { await writeFile(path.join(directory, file), bytes); record[file] = { bytes: bytes.length, sha256: digest(bytes) }; }
    await writeFile(path.join(directory, 'result.json'), JSON.stringify(record, null, 2) + '\n');
    manifest.cases.push(record); console.log(JSON.stringify({ name, pass: record.pass, error: record.error?.message, helperExit: record.helperExit, protocolError: record.protocol.error, actualTarget: record.finalTarget }));
    await writeFile(path.join(evidence, 'manifest.partial.json'), JSON.stringify(manifest, null, 2) + '\n');
    progress('case-after', { name, pass: record.pass, teardownClosed: closed, cleanupUnknown: record.cleanupUnknown ?? false });
  }
  return record;
}
async function refusedBeforePayload(helper) {
  const directory = path.join(evidence, 'setstd-failure'); await mkdir(directory, { recursive: true });
  const report = path.join(directory, 'target.json'), args = ['--controller', String(process.pid), '--grace', '50', '--terminal', '91', '37', '--', targetExecutable, report, 'terminal'];
  progress('failure-case-before', { name: 'setstd-failure' });
  const child = spawn(helper, args, { shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
  const chunks = [], errors = []; let closed = false, exit, launchError, captured = 0, overflow = false;
  child.stdin.on('error', () => {}); child.on('error', error => { launchError = error.message; });
  const capture = (list, b) => { captured += b.length; if (captured > 1024 * 1024) { overflow = true; child.kill(); } else list.push(b); };
  child.stdout.on('data', b => capture(chunks, b)); child.stderr.on('data', b => capture(errors, b));
  child.on('exit', (code, signal) => progress('failure-helper-process-exit', { code, signal }));
  child.on('close', (code, signal) => { closed = true; exit = { code, signal }; });
  child.stdin.end(encodeFrame('L'));
  const deadline = Date.now() + 10000;
  while (!closed && Date.now() < deadline) await delay(10);
  if (!closed) { child.kill(); const end = Date.now() + 5000; while (!closed && Date.now() < end) await delay(10); }
  if (!closed) { child.stdout.destroy(); child.stderr.destroy(); child.unref(); }
  const raw = Buffer.concat(chunks), stderr = Buffer.concat(errors), parsed = decodeFrames(raw);
  let reportMissing = false;
  try { await readFile(report); } catch (error) { if (error.code === 'ENOENT') reportMissing = true; else throw error; }
  const record = { command: [helper, ...args], observedAt: new Date().toISOString(), closed, exit, launchError, overflow, reportMissing, frames: parsed.frames.map(f => ({ kind: f.kind, text: f.bytes.toString() })), protocolError: parsed.error, pendingBytes: parsed.pendingBytes };
  await writeFile(path.join(directory, 'stdout.bin'), raw); await writeFile(path.join(directory, 'stderr.bin'), stderr);
  record.raw = { bytes: raw.length, sha256: digest(raw), stderrBytes: stderr.length, stderrSha256: digest(stderr) };
  await writeFile(path.join(directory, 'result.json'), JSON.stringify(record, null, 2) + '\n');
  manifest.setStdFailure = record;
  assert.equal(launchError, undefined); assert.equal(overflow, false); assert.equal(closed, true); assert.deepEqual(exit, { code: 2, signal: null });
  assert.equal(reportMissing, true, 'Failed standard-handle isolation must not start payload');
  assert.equal(parsed.error, undefined); assert.equal(parsed.pendingBytes, 0);
  assert.deepEqual(parsed.frames.map(f => f.kind), ['X']); assert.match(parsed.frames[0].bytes.toString(), /standard-handle isolation failed before payload/);
  progress('failure-case-after', { payloadNeverStarted: true, released: false });
}
let targetExecutable;
try {
  const targetSource = path.join(root, 'tests/fixtures/native-windows-terminal-probe.c');
  targetExecutable = await build('target-probe', await readFile(targetSource, 'utf8'), 'original independent Win32 diagnostic target, not ownership implementation', targetSource);
  progress('variant-build-before', { name: 'primary' });
  variants[0].helper = await buildOwnedProcess(root, path.join(evidence, 'builds/primary/bin'));
  progress('variant-build-after', { name: 'primary', executable: variants[0].helper });
  manifest.builds.push({ name: 'primary', role: variants[0].role, executable: variants[0].helper, provenance: JSON.parse(await readFile(`${variants[0].helper}.provenance.json`, 'utf8')) });
  await scenario(variants[0].helper, 'primary', 'c', 'byte');
  for (const target of ['c', 'node']) await scenario(variants[0].helper, 'primary', target, 'terminal');
  await scenario(variants[0].helper, 'primary', 'node', 'terminal', true);
  assert.equal(manifest.cases.length, 4); assert.ok(manifest.cases.every(c => c.pass && c.teardownClosed), 'Primary preflight must pass before regression variants');
  const legacy = variants[1]; legacy.helper = await build(legacy.name, legacy.source, legacy.role, sourcePath);
  manifest.legacyNegativeChecks = [];
  for (const target of ['c', 'node']) {
    const record = await scenario(legacy.helper, legacy.name, target, 'terminal');
    const tty = target === 'c' ? record.finalTarget?.descriptors?.map(d => d.console && d.crtIsatty !== 0) : record.finalTarget?.tty;
    assert.equal(record.pass, false); assert.equal(record.teardownClosed, true); assert.deepEqual(tty, [false, false, false]);
    if (target === 'c') {
      assert.ok(record.protocol.error, 'Legacy C writes corrupt the private protocol');
      const raw = await readFile(path.join(evidence, record.name, 'stdout.bin'));
      assert.match(raw.subarray(record.protocol.error.offset).toString(), /^TARGET_STDOUT_READY/);
    }
    manifest.legacyNegativeChecks.push({ name: record.name, reproduced: true });
  }
  const refused = variants[2]; refused.helper = await build(refused.name, refused.source, refused.role, sourcePath);
  await refusedBeforePayload(refused.helper);
} catch (error) { manifest.failures.push({ message: error.message, stack: error.stack }); }
finally {
  manifest.finishedAt = new Date().toISOString();
  manifest.primaryPassed = manifest.cases.filter(c => c.name.startsWith('primary-')).length === 4 && manifest.cases.filter(c => c.name.startsWith('primary-')).every(c => c.pass);
  manifest.variantsCannotSatisfyPrimary = true;
  await writeFile(path.join(evidence, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  process.exitCode = manifest.failures.length || !manifest.primaryPassed || manifest.cases.some(c => !c.teardownClosed) || manifest.legacyNegativeChecks?.length !== 2 || !manifest.setStdFailure?.reportMissing ? 1 : 0;
  progress('harness-script-finished', { intendedExitCode: process.exitCode, cases: manifest.cases.length, primaryPassed: manifest.primaryPassed });
}
