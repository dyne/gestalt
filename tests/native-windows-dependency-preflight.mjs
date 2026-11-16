// Targeted actual Windows quoting + natural nested-TTY attribution. No model,
// L4 orchestration, registry changes, production fallback or forced-success exit.
import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { once } from 'node:events';
import { mkdir, mkdtemp, readFile, readdir, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { PassThrough } from 'node:stream';
import { setTimeout as delay } from 'node:timers/promises';
import { build } from 'esbuild';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { NativeOwnedProcesses } from '../src/cli/platform/processes.ts';
import { countProbeSource, vectorBytes, wideReader, quoteReference, quotingCases, checkCountResult } from './fixtures/native-command-line.mjs';
import { journal, observeOwnedHelpers } from './fixtures/native-owned-observer.mjs';
import { encodeFrame, decodeFrames } from './fixtures/native-windows-terminal-wire.mjs';
assert.equal(process.platform, 'win32', 'Actual Windows required, not mocked process.platform');
const root = path.resolve(import.meta.dirname, '..'), evidence = path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE ?? 'native-evidence', 'windows-terminal-preflight/dependency-correction');
await mkdir(evidence, { recursive: true });
const progress = (phase, value) => journal(path.join(evidence, 'progress.jsonl'), phase, value);
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const manifest = { observedAt: new Date().toISOString(), node: process.version, arch: process.arch, cases: [], builds: [], failures: [], registryWrites: false, fullNativeSuite: 'UNEXECUTED; this lane is preflight only', EOFAttribution: 'PENDING actual comparison; a timeout stop is never natural success' };
async function save() { await writeFile(path.join(evidence, 'manifest.partial.json'), JSON.stringify(manifest, null, 2) + '\n'); }
async function bounded(promise, label, ms = 15000) { let timer; try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(Error(`${label} deadline`)), ms); })]); } finally { clearTimeout(timer); } }
async function fixtureBuild(name, source, role) {
  progress('build-before', { name, role });
  const location = path.join(evidence, 'builds', name), file = path.join(location, 'src/cli/platform/native/owned-process.c');
  await mkdir(path.dirname(file), { recursive: true }); await writeFile(file, source);
  const executable = await buildOwnedProcess(location, path.join(location, 'bin'));
  manifest.builds.push({ name, role, sourceSha256: hash(Buffer.from(source)), executable, provenance: JSON.parse(await readFile(executable + '.provenance.json', 'utf8')) });
  progress('build-after', { name }); await save(); return executable;
}
function argvReport(bytes) {
  const r = wideReader(bytes); assert.equal(r.integer(), 0x41524756); const argc = r.integer(), tty = [r.integer(), r.integer(), r.integer()].map(Boolean), inJob = Boolean(r.integer()), commandLine = r.string(), argv = [];
  for (let i = 0; i < argc; i++) argv.push(r.string()); r.done(); return { tty, inJob, commandLine, argv };
}
async function apiScenario(name, helper, target, argv, terminal, natural = false, host = false, expectedOuterRejection = false) {
  const location = path.join(evidence, name); await mkdir(location, { recursive: true });
  const scratch = await mkdtemp(path.join(tmpdir(), 'q-')), report = path.join(scratch, 'argv.bin'), leafRecord = path.join(location, 'leaf.jsonl'), hostRecord = path.join(location, 'host.jsonl');
  const record = { name, terminal, natural, expectedOuterRejection, observedAt: new Date().toISOString(), pass: false, forcedCleanup: false, inputEndedBeforeNaturalResult: false, scratch };
  const input = new PassThrough(), chunks = [], observer = observeOwnedHelpers(helper, location, 'outer'); let lease, completedObserved = false;
  try {
    const invocation = { argv: natural ? host ? [hostBundle, helper, leafBundle, leafRecord, hostRecord, location] : [leafBundle, leafRecord] : argv,
      cwd: root, environment: { ...process.env, GESTALT_TEST_ARGV_REPORT: path.toNamespacedPath(report) } };
    record.invocation = { executable: target, argv: invocation.argv, cwd: invocation.cwd };
    lease = terminal ? await new NativeOwnedProcesses(helper).launchTerminal(target, invocation, { input, output: async bytes => { chunks.push(Buffer.from(bytes)); } }, { columns: 91, rows: 37 }, { graceMs: 50, deadlineMs: 5000 }) :
      await new NativeOwnedProcesses(helper).launch(target, invocation, { stdin: input, stdout: async bytes => { chunks.push(Buffer.from(bytes)); }, stderr: async bytes => { chunks.push(Buffer.from(bytes)); } }, { graceMs: 50, deadlineMs: 5000 });
    assert.equal(expectedOuterRejection, false, 'Expected pre-helper outer command-line rejection; a lease is not that proof');
    record.initialClaim = lease.claim();
    if (natural) {
      const end = Date.now() + 15000;
      while (!Buffer.concat(chunks).includes(Buffer.from('NATURAL_LEAF_READY')) && Date.now() < end) await delay(10);
      assert.ok(Buffer.concat(chunks).includes(Buffer.from('NATURAL_LEAF_READY')), 'Readiness is an input precondition only');
      input.write(Buffer.from([4])); record.sentInputHex = '04'; progress('EOF-control-sent', { name });
    }
    record.result = await bounded(lease.result, name + ' natural result');
    record.completed = await bounded(lease.completed, name + ' completed'); completedObserved = true; record.finalClaim = lease.claim();
    assert.deepEqual(record.result, { kind: 'exit', code: 23 }); assert.equal(record.completed.kind, 'released'); assert.equal(record.completed.backend, 'windows-job'); assert.equal(record.completed.lease, lease.id);
    if (!natural) {
      const bytes = await readFile(report); await writeFile(path.join(location, 'argv.bin'), bytes); const actual = argvReport(bytes); record.target = { ...actual, argv: undefined, argvSha256: hash(vectorBytes(actual.argv)), commandUnitsIncludingNul: actual.commandLine.length + 1 };
      assert.deepEqual(actual.argv, [target, ...argv]); assert.deepEqual(actual.tty, [terminal, terminal, terminal]); assert.equal(actual.inJob, true); assert.equal(actual.commandLine, quoteReference([target, ...argv]));
    } else {
      const leaf = (await readFile(leafRecord, 'utf8')).trim().split('\n').map(JSON.parse); record.leafPhases = leaf;
      assert.ok(leaf.some(r => r.phase === 'leaf-exit' && r.code === 23), 'Actual leaf exit, not output text');
      if (host) { const events = (await readFile(hostRecord, 'utf8')).trim().split('\n').map(JSON.parse); record.hostPhases = events; assert.ok(events.some(r => r.phase === 'host-exit' && r.code === 23)); assert.ok(events.some(r => r.phase === 'inner-completed' && r.completed.kind === 'released')); }
    }
    record.pass = true;
  } catch (error) {
    record.error = { message: error.message, code: error.code, stack: error.stack };
    if (expectedOuterRejection) {
      try {
        assert.equal(error.code, 'ENAMETOOLONG'); assert.equal(lease, undefined);
        assert.equal(observer.traces.length, 0); assert.equal(chunks.length, 0);
        await assert.rejects(readFile(report), { code: 'ENOENT' });
        record.boundaryObservation = 'Expected safe pre-helper ENAMETOOLONG: no lease, helper frames or payload report; not a target-limit launch';
        record.payloadNeverStarted = true; record.pass = true;
      } catch (verification) { record.boundaryVerificationError = verification.message; }
    }
  }
  finally {
    if (lease && (!completedObserved || lease.claim().kind === 'active')) {
      record.forcedCleanup = true;
      record.cleanupClaim = await bounded(lease.stop(), name + ' cleanup', 8000).catch(error => ({ kind: 'unknown', reason: error.message }));
      record.cleanupCompleted = await bounded(lease.completed, name + ' cleanup completion', 5000).catch(error => ({ kind: 'unknown', reason: error.message }));
    }
    input.end(); observer.restore(); record.traces = observer.traces; record.traceCompleteness = observer.traces.every(t => t.close) ? 'complete' : 'partial'; record.outputBytes = Buffer.concat(chunks).length;
    await writeFile(path.join(location, 'output.bin'), Buffer.concat(chunks));
    if (observer.traces.length !== (expectedOuterRejection ? 0 : 1)) { record.traceError = 'Unexpected production helper observation count'; record.pass = false; }
    for (const trace of observer.traces) {
      try {
        const raw = await readFile(trace.stdout), parsed = decodeFrames(raw); trace.raw = { bytes: raw.length, sha256: hash(raw), pendingBytes: parsed.pendingBytes, error: parsed.error, frames: parsed.frames.map(f => ({ kind: f.kind, text: 'OB'.includes(f.kind) ? undefined : f.bytes.toString(), bytes: f.bytes.length })) };
        assert.equal(parsed.error, undefined); assert.equal(parsed.pendingBytes, 0); assert.equal(trace.errors.length, 0);
        if (record.pass) { assert.equal(parsed.frames.filter(f => f.kind === 'D').length, 1); assert.equal(parsed.frames.find(f => f.kind === 'D').bytes.toString(), 'windows-job'); assert.equal(parsed.frames.find(f => f.kind === 'F').bytes.toString(), 'exit:23'); }
      }
      catch (error) { record.traceError = error.message; record.pass = false; }
    }
    if (natural && host) {
      try { record.hostPartialPhases = (await readFile(hostRecord, 'utf8')).trim().split('\n').map(JSON.parse); } catch { /* Missing record remains explicit. */ }
      record.innerRaw = [];
      for (const name of (await readdir(location)).filter(n => /^inner-\d+\.stdout\.bin$/.test(n))) {
        const raw = await readFile(path.join(location, name)), parsed = decodeFrames(raw);
        record.innerRaw.push({ name, bytes: raw.length, sha256: hash(raw), error: parsed.error, pendingBytes: parsed.pendingBytes, frames: parsed.frames.map(f => ({ kind: f.kind, text: 'OB'.includes(f.kind) ? undefined : f.bytes.toString(), bytes: f.bytes.length })) });
      }
    }
    record.finishedAt = new Date().toISOString(); await writeFile(path.join(location, 'result.json'), JSON.stringify(record, null, 2) + '\n'); manifest.cases.push(record); await save(); progress('case-after', { name, pass: record.pass, forcedCleanup: record.forcedCleanup });
  }
}
async function vectorScenario(name, probe, argv, terminal, rejected = false) {
  const location = path.join(evidence, name); await mkdir(location, { recursive: true }); const vector = path.join(location, 'argv.vec'), report = path.join(location, 'argv.bin'); await writeFile(vector, vectorBytes(argv));
  const record = { name, role: 'TEST-ONLY file entry calls exact production run_owned_windows; not shipping transport', terminal, rejected, pass: false, unitsIncludingNul: quoteReference(argv).length + 1 };
  const child = spawn(probe, ['--vector', path.toNamespacedPath(vector), terminal ? 'terminal' : 'byte'], { shell: false, windowsHide: true, env: { ...process.env, GESTALT_TEST_ARGV_REPORT: path.toNamespacedPath(report) }, stdio: ['pipe', 'pipe', 'pipe'] });
  const out = [], err = []; child.stdout.on('data', b => out.push(b)); child.stderr.on('data', b => err.push(b)); child.stdin.on('error', () => {}); const close = once(child, 'close'); child.stdin.write(encodeFrame('L'));
  try {
    record.exit = await bounded(close, name); const raw = Buffer.concat(out), decoded = decodeFrames(raw); record.frames = decoded.frames.map(f => ({ kind: f.kind, text: 'OB'.includes(f.kind) ? undefined : f.bytes.toString() }));
    assert.equal(decoded.error, undefined); assert.equal(decoded.pendingBytes, 0); assert.deepEqual(record.exit, [0, null]); assert.equal(record.frames.find(f => f.kind === 'D')?.text, 'windows-job');
    if (rejected) {
      assert.match(record.frames.find(f => f.kind === 'F')?.text, /^spawn-error:.*\(206\)$/); let absent = false; try { await readFile(report); } catch (error) { absent = error.code === 'ENOENT'; } assert.equal(absent, true); record.payloadNeverStarted = true;
    } else { const actual = argvReport(await readFile(report)); assert.deepEqual(actual.argv, argv); assert.equal(actual.commandLine.length + 1, 32767); assert.deepEqual(actual.tty, [terminal, terminal, terminal]); assert.equal(actual.inJob, true); assert.equal(record.frames.find(f => f.kind === 'F')?.text, 'exit:23'); record.actual = { unitsIncludingNul: actual.commandLine.length + 1, argvSha256: hash(vectorBytes(actual.argv)) }; }
    record.pass = true;
  } catch (error) { record.error = { message: error.message, stack: error.stack }; }
  finally {
    if (child.exitCode === null && child.signalCode === null) {
      record.forcedCleanup = true; child.stdin.destroy();
      await bounded(close, name + ' channel EOF', 8000).catch(async () => {
        record.forcedHelperTermination = true; child.kill();
        await bounded(close, name + ' helper close', 5000).catch(() => { record.cleanupUnknown = true; child.stdout.destroy(); child.stderr.destroy(); child.unref(); });
      });
    }
    await writeFile(path.join(location, 'stdout.bin'), Buffer.concat(out)); await writeFile(path.join(location, 'stderr.bin'), Buffer.concat(err)); record.finishedAt = new Date().toISOString(); manifest.cases.push(record); await writeFile(path.join(location, 'result.json'), JSON.stringify(record, null, 2) + '\n'); await save();
  }
}
let leafBundle, hostBundle;
try {
  progress('preflight-start');
  const source = await readFile(path.join(root, 'src/cli/platform/native/owned-process.c'), 'utf8'); manifest.productionSourceSha256 = hash(Buffer.from(source));
  const helper = await buildOwnedProcess(root, path.join(evidence, 'builds/primary')); manifest.builds.push({ name: 'primary', role: 'UNCHANGED production helper with quoting correction', executable: helper, provenance: JSON.parse(await readFile(helper + '.provenance.json', 'utf8')) });
  const count = await fixtureBuild('count', await countProbeSource(source), 'TEST-ONLY native compilation of exact quoting block/explicit UTF16 corpus');
  const captureBytes = await readFile(path.join(root, 'tests/fixtures/native-owned-query-vector.json')), capture = JSON.parse(captureBytes); manifest.countCases = [];
  manifest.capture = { sourceSha256: hash(captureBytes), argvSha256: hash(vectorBytes([capture.target, ...capture.argv])), argc: capture.argv.length + 1, unitsIncludingNul: quoteReference([capture.target, ...capture.argv]).length + 1, role: 'INERT captured data, no hook/PowerShell execution' };
  for (const item of quotingCases(capture)) { const result = spawnSync(count, [], { input: vectorBytes(item.argv), timeout: 5000, maxBuffer: 1024 * 1024, shell: false }); try { assert.equal(result.status, 0); manifest.countCases.push({ name: item.name, pass: true, ...checkCountResult(item, result.stdout) }); } catch (error) { manifest.countCases.push({ name: item.name, pass: false, error: error.message }); } }
  const probeSource = '#define wmain accepted_owned_entry\n' + source + '\n#undef wmain\n' + await readFile(path.join(root, 'tests/fixtures/native-command-line-vector.h'), 'utf8') + '\n' + await readFile(path.join(root, 'tests/fixtures/native-windows-command-line-probe.c'), 'utf8');
  const probe = await fixtureBuild('probe', probeSource, 'TEST-ONLY C target/argv-file entry, includes entire byte-exact production helper/lifecycle');
  const literals = ['--echo', '', 'space é東京🚀', 'a"b', 'tail\\', '\\"', '&|<>^%!$()', ...capture.argv];
  for (const terminal of [false, true]) await apiScenario('production-literal-' + (terminal ? 'terminal' : 'byte'), helper, probe, literals, terminal);
  const edge = [probe, '--echo', 'é東京🚀', '']; edge[3] = 'a'.repeat(32767 - (quoteReference(edge).length + 1)); assert.equal(quoteReference(edge).length + 1, 32767);
  for (const terminal of [false, true]) { await vectorScenario('OS-boundary-' + (terminal ? 'terminal' : 'byte'), probe, edge, terminal); await vectorScenario('OS-oversize-' + (terminal ? 'terminal' : 'byte'), probe, [...edge.slice(0, -1), edge.at(-1) + 'a'], terminal, true); }
  await apiScenario('production-outer-limit-rejection', helper, probe, edge.slice(1), false, false, false, true);
  // Actual production API near-limit leaves explicit budget for helper/control
  // argv. Exact OS-boundary calls above isolate that existing outer ceiling.
  const near = ['--echo', '', '']; near[2] = 'a'.repeat(32000 - (quoteReference([probe, ...near]).length + 1));
  await apiScenario('production-near-limit-byte', helper, probe, near, false);
  const bundleDir = path.join(evidence, 'bundles'); await mkdir(bundleDir, { recursive: true }); leafBundle = path.join(bundleDir, 'leaf.mjs'); hostBundle = path.join(bundleDir, 'host.mjs');
  for (const [entry, output] of [['native-owned-natural-tty-leaf.mjs', leafBundle], ['native-owned-natural-tty-host.mjs', hostBundle]]) {
    const built = await build({ entryPoints: [path.join(root, 'tests/fixtures', entry)], outfile: output, bundle: true, platform: 'node', target: 'node24', format: 'esm', metafile: true });
    manifest.builds.push({ role: 'TEST-ONLY natural return diagnostic bundle', output, sha256: hash(await readFile(output)), inputs: await Promise.all(Object.keys(built.metafile.inputs).map(async file => ({ path: file, sha256: hash(await readFile(path.resolve(file))) }))) });
  }
  await apiScenario('natural-direct-leaf', helper, process.execPath, [], true, true, false);
  await apiScenario('natural-nested-host', helper, process.execPath, [], true, true, true);
} catch (error) { manifest.failures.push({ message: error.message, stack: error.stack }); }
finally {
  manifest.finishedAt = new Date().toISOString(); manifest.pass = manifest.failures.length === 0 && manifest.countCases?.length === 16 && manifest.countCases.every(c => c.pass) && manifest.cases.length === 10 && manifest.cases.every(c => c.pass && !c.forcedCleanup);
  manifest.EOFAttribution = 'Compare exact leaf/host/inner result+completion+claim/resource events and raw traces; any forced cleanup remains a failed natural result, not fixed EOF.';
  await save(); await writeFile(path.join(evidence, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n'); process.exitCode = manifest.pass ? 0 : 1; progress('preflight-finished', { pass: manifest.pass, intendedExit: process.exitCode });
}
