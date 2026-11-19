import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { lstat, mkdir, mkdtemp, readFile, realpath, rmdir, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import { NativePrivateStorage } from '../../src/cli/platform/storage.ts';
import { digest, json, now, root, save, verifyHelper } from './common.mjs';

const [prebuilt, output] = process.argv.slice(2);
assert.equal(process.argv.length, 4);
assert.equal(process.platform, 'win32');
assert.ok(path.isAbsolute(prebuilt) && path.isAbsolute(output));
await mkdir(output, { recursive: true });
const resultFile = path.join(output, 'guard-result.json');
const execute = promisify(execFile);
const evidence = { state: 'started', observedAt: now(), phases: [], nativeBindingRequests: 0, fixture: null };
async function phase(id, action) {
  const row = { id, startedAt: now(), monotonicStartMs: performance.now(), state: 'started' };
  evidence.phases.push(row); await save(resultFile, evidence);
  try { row.detail = await action(); row.state = 'pass'; return row.detail; }
  catch (error) { row.state = 'fail'; row.error = { message: error.message, code: error.code ?? null }; throw error; }
  finally { row.observedAt = now(); row.monotonicEndMs = performance.now(); await save(resultFile, evidence); }
}
try {
  const token = await json(path.join(path.dirname(output), 'consumer-token.json'));
  assert.equal(token.administrator, false);
  evidence.helper = await verifyHelper(path.join(prebuilt, 'native'));
  const helper = evidence.helper.helper.path;
  const binding = async () => { evidence.nativeBindingRequests++; return helper; };
  const powerShell = path.join(process.env.SystemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
  evidence.powerShell = await digest(powerShell);
  const script = path.join(root, 'tests/fixtures/native-storage-acl.ps1');
  let privateRoot, nested;
  async function acl(file, receiptName, operation = 'inspect', kind) {
    assert.equal(file, nested, 'ACL fixture mutation is confined to its exact owned nested directory');
    const { stdout, stderr } = await execute(powerShell, ['-NoLogo', '-NoProfile', '-NonInteractive', '-File', script], {
      env: { ...process.env, GESTALT_STORAGE_PARITY_REQUEST: Buffer.from(JSON.stringify({ path: file, operation, kind })).toString('base64') },
      encoding: 'buffer', shell: false, windowsHide: true, timeout: 50_000, maxBuffer: 65536,
    });
    await Promise.all([
      writeFile(path.join(output, receiptName + '.stdout'), stdout, { flag: 'wx', mode: 0o600 }),
      writeFile(path.join(output, receiptName + '.stderr'), stderr, { flag: 'wx', mode: 0o600 }),
    ]);
    assert.equal(stderr.length, 0);
    const receipt = JSON.parse(stdout.toString('utf8').replace(/^\uFEFF/, ''));
    assert.equal(receipt.administrator, false); assert.equal(receipt.sid, token.sid); assert.equal(receipt.owner, token.sid);
    return receipt;
  }
  function assertNativeDenial(error) {
    const denied = error.cause;
    assert.equal(denied?.code, 1); assert.equal(denied.signal, null); assert.equal(denied.killed, false);
    assert.ok(Buffer.isBuffer(denied.stdout) && denied.stdout.length === 0);
    assert.equal(denied.stderr.toString(), 'GESTALT_STORAGE_CHECK_V1 ace error=5 index=0\n');
    return true;
  }
  await phase('fresh-fixture', async () => {
    evidence.fixture = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt integrated guard ü ')));
    privateRoot = path.join(evidence.fixture, 'private ü root');
    return { root: privateRoot };
  });
  let storage;
  await phase('integrated-private-open', async () => {
    storage = await NativePrivateStorage.open(privateRoot, undefined, binding);
    assert.equal(storage.root, privateRoot);
    assert.ok((await lstat(privateRoot)).isDirectory());
    assert.equal(evidence.nativeBindingRequests, 3);
  });
  await phase('integrated-nested-create', async () => {
    nested = path.join(privateRoot, 'nested ü leaf');
    await storage.createDirectory(nested);
    assert.ok((await lstat(nested)).isDirectory());
    assert.equal(evidence.nativeBindingRequests, 12);
  });
  await phase('private-refusal-with-foreign-read', async () => {
    const before = await acl(nested, 'private-foreign-read-before', 'apply', 'others-read');
    assert.ok(before.rules.some(rule => rule.sid === 'S-1-1-0' && rule.type === 'Allow' && (rule.mask & 1) === 1));
    const reads = evidence.nativeBindingRequests;
    await assert.rejects(storage.assertPrivate(nested, 'directory'), assertNativeDenial);
    assert.equal(evidence.nativeBindingRequests - reads, 3);
    const after = await acl(nested, 'private-foreign-read-after');
    assert.deepEqual(after, before);
    return { before, after, refusedWithoutRepair: true };
  });
  await phase('namespace-refusal-with-foreign-dacl-write', async () => {
    const before = await acl(nested, 'namespace-foreign-dacl-write-before', 'apply', 'others-write');
    // In the rights field of this ACL fixture, SDDL WD is WRITE_DAC.
    const writeDacl = 0x00040000;
    assert.ok(before.rules.some(rule => rule.sid === 'S-1-1-0' && rule.type === 'Allow' &&
      (rule.mask & writeDacl) === writeDacl));
    const destination = path.join(nested, 'must remain absent');
    const reads = evidence.nativeBindingRequests;
    await assert.rejects(NativePrivateStorage.open(destination, undefined, binding), assertNativeDenial);
    assert.equal(evidence.nativeBindingRequests - reads, 1);
    await assert.rejects(lstat(destination), { code: 'ENOENT' });
    const after = await acl(nested, 'namespace-foreign-dacl-write-after');
    assert.deepEqual(after, before);
    return { before, after, destinationCreated: false };
  });
  await phase('restore-fixture-and-clean-up', async () => {
    const restored = await acl(nested, 'restored-private', 'apply', 'private');
    assert.ok(restored.rules.every(rule => rule.sid !== 'S-1-1-0'));
    await storage.assertPrivate(nested, 'directory');
    assert.equal(evidence.nativeBindingRequests, 19);
    await rmdir(nested); await rmdir(privateRoot); await rmdir(evidence.fixture);
    return { removed: true };
  });
  evidence.phaseLog = await digest(process.env.FINAL_WINDOWS_PHASE_LOG);
  const events = (await readFile(evidence.phaseLog.path, 'utf8')).trim().split(/\r?\n/).map(JSON.parse);
  const reads = events.filter(row => row.event === 'command-dispatched' && row.nativeRead);
  const readResults = reads.map(row => events.find(event => event.pid === row.pid && event.id === row.id && event.event === 'command-observed'));
  assert.equal(reads.length, 19);
  assert.equal(readResults.filter(row => row?.status === 0 && row.signal === null).length, 17);
  assert.equal(readResults.filter(row => row?.status === 1 && row.signal === null).length, 2);
  const legacy = events.filter(row => row.event === 'command-dispatched' && ['assert-private','assert-namespace'].includes(row.adapter));
  assert.equal(legacy.length, 0);
  evidence.state = 'pass';
} catch (error) {
  evidence.state = 'fail';
  evidence.failure = { message: error.message, stack: error.stack };
  process.exitCode = 1;
} finally {
  evidence.finishedAt = now();
  evidence.scope = { specialAce: 'unexecuted', remoteUNC: 'unexecuted', workerOwnership: 'unexercised' };
  await save(resultFile, evidence);
}
