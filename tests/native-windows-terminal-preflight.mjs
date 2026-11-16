// Explicit Windows correction lane. Variants are diagnostics, never product.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { encodeFrame, decodeFrames, terminalVariants } from './fixtures/native-windows-terminal-wire.mjs';

assert.equal(process.platform, 'win32', 'Preflight requires actual Windows, not mocked platform');
const root = path.resolve(import.meta.dirname, '..');
const evidence = path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE ?? 'native-evidence', 'windows-terminal-preflight');
await mkdir(evidence, { recursive: true, mode: 0o700 });
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const sourcePath = path.join(root, 'src/cli/platform/native/owned-process.c');
const original = await readFile(sourcePath, 'utf8');
const variants = terminalVariants(original);
const manifest = { kind: 'standard-user-windows-terminal-preflight', observedAt: new Date().toISOString(), platform: process.platform, arch: process.arch, node: process.version,
  sourcePath, sourceSha256: digest(Buffer.from(original)), cases: [], builds: [], failures: [], fullNativeSuite: 'not executed by this script', productionSelectionChanged: false };
async function build(name, source, role, origin) {
  const location = path.join(evidence, 'builds', name), fixtureSource = path.join(location, 'src/cli/platform/native/owned-process.c');
  await mkdir(path.dirname(fixtureSource), { recursive: true }); await writeFile(fixtureSource, source);
  const executable = await buildOwnedProcess(location, path.join(location, 'bin'));
  manifest.builds.push({ name, role, origin, fixtureSource, sourceSha256: digest(Buffer.from(source)), executable,
    provenance: JSON.parse(await readFile(`${executable}.provenance.json`, 'utf8')) });
  return executable;
}
async function scenario(helper, variant, target, transport, stop = false) {
  const name = `${variant}-${target}-${transport}${stop ? '-stop' : ''}`, directory = path.join(evidence, name);
  await mkdir(directory, { recursive: true }); const reportPath = path.join(directory, 'target.json');
  const record = { name, observedAt: new Date().toISOString(), helper, target, transport, stop, events: [], pass: false };
  const args = ['--controller', String(process.pid), '--grace', '50', ...(transport === 'terminal' ? ['--terminal', '91', '37'] : []), '--',
    ...(target === 'c' ? [targetExecutable, reportPath, transport] : [process.execPath, path.join(root, 'tests/fixtures/native-windows-terminal-node.mjs'), reportPath, stop ? 'hold' : 'input'])];
  const child = spawn(helper, args, { shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
  record.command = [helper, ...args]; let raw = Buffer.alloc(0), errors = Buffer.alloc(0), closed = false, childError;
  let resolveClose; const close = new Promise(resolve => { resolveClose = resolve; });
  child.stdin.on('error', error => { record.inputError = error.message; });
  child.on('error', error => { childError = error.message; });
  child.stdout.on('data', bytes => {
    if (raw.length + bytes.length > 1024 * 1024) { record.outputOverflow = true; child.kill(); } else raw = Buffer.concat([raw, bytes]);
  });
  child.stderr.on('data', bytes => { if (errors.length + bytes.length <= 1024 * 1024) errors = Buffer.concat([errors, bytes]); else record.stderrOverflow = true; });
  child.on('close', (code, signal) => { closed = true; record.helperExit = { code, signal }; resolveClose(); });
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
  }
}
let targetExecutable;
try {
  const targetSource = path.join(root, 'tests/fixtures/native-windows-terminal-probe.c');
  targetExecutable = await build('target-probe', await readFile(targetSource, 'utf8'), 'original independent Win32 diagnostic target, not ownership implementation', targetSource);
  for (const variant of variants) variant.helper = variant.name === 'primary'
    ? await buildOwnedProcess(root, path.join(evidence, 'builds/primary/bin'))
    : await build(variant.name, variant.source, variant.role, sourcePath);
  manifest.builds.push({ name: 'primary', role: variants[0].role, executable: variants[0].helper, provenance: JSON.parse(await readFile(`${variants[0].helper}.provenance.json`, 'utf8')) });
  await scenario(variants[0].helper, 'primary', 'c', 'byte');
  for (const variant of variants) for (const target of ['c', 'node']) await scenario(variant.helper, variant.name, target, 'terminal');
  await scenario(variants[0].helper, 'primary', 'node', 'terminal', true);
} catch (error) { manifest.failures.push({ message: error.message, stack: error.stack }); }
finally {
  manifest.finishedAt = new Date().toISOString();
  manifest.primaryPassed = manifest.cases.filter(c => c.name.startsWith('primary-')).length === 4 && manifest.cases.filter(c => c.name.startsWith('primary-')).every(c => c.pass);
  manifest.variantsCannotSatisfyPrimary = true;
  await writeFile(path.join(evidence, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  process.exitCode = manifest.failures.length || !manifest.primaryPassed || manifest.cases.some(c => !c.teardownClosed) ? 1 : 0;
}
