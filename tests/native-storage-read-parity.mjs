// Explicit ordinary Windows gate. No registration in the default local suite.
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdir, mkdtemp, writeFile, readFile, lstat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { isDeepStrictEqual } from 'node:util';
import { NativePrivateStorage } from '../src/cli/platform/storage.ts';
import { windowsSystem } from '../src/cli/platform/windows-system.ts';
import { windowsReadArguments, assertWindowsReadReply } from '../src/cli/runtime/windows-scratch-read.mjs';
import { verifyOwnershipBuildInputs } from './fixtures/native-runtime/source-binding.mjs';

assert.equal(process.platform, 'win32');
const args = process.argv.slice(2); assert.equal(args.length, 4); assert.equal(args[0], '--artifacts'); assert.equal(args[2], '--evidence');
const artifacts = path.resolve(args[1]), evidence = path.resolve(args[3]); await mkdir(evidence, { recursive: true });
const root = path.resolve(import.meta.dirname, '..'), manifest = JSON.parse(await readFile(path.join(artifacts, 'artifacts.json')));
const helper = path.join(artifacts, manifest.helper), provenance = JSON.parse(await readFile(helper + '.provenance.json'));
assert.equal(provenance.platform, 'win32'); assert.equal(provenance.architecture, process.arch);
assert.equal(createHash('sha256').update(await readFile(helper)).digest('hex'), provenance.artifactSha256);
assert.deepEqual(await verifyOwnershipBuildInputs(provenance, root), manifest.nativeHelperInputs);
const outer = await mkdtemp(path.join(tmpdir(), 'storage-parity-')), storage = await NativePrivateStorage.open(path.join(outer, 'private'));
const powershell = path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe');
const script = path.join(root, 'tests/fixtures/native-storage-acl.ps1');
function acl(file, operation = 'inspect', kind) {
  const bytes = execFileSync(powershell, ['-NoLogo', '-NoProfile', '-NonInteractive', '-File', script], {
    env: { ...process.env, GESTALT_STORAGE_PARITY_REQUEST: Buffer.from(JSON.stringify({ path: file, operation, kind })).toString('base64') },
    shell: false, windowsHide: true, timeout: 10000, maxBuffer: 65536,
  }); return JSON.parse(bytes.toString('utf8').replace(/^\uFEFF/, ''));
}
const results = [], construction = [];
let failure, expectedRequiredTests = null;
async function check(name, file, operation, expected) {
  const before = acl(file), info = await lstat(file), content = info.isFile() ? await readFile(file) : null;
  const request = operation === 'assert-private' ? { operation, path: file } : { operation, paths: [path.toNamespacedPath(file)] };
  let golden = true, goldenError = null;
  try { await windowsSystem(request); } catch (error) { golden = false; goldenError = error.message; }
  const native = spawnSync(helper, windowsReadArguments(request), { shell: false, windowsHide: true, timeout: 10000 });
  const accepted = native.status === 0 && native.signal === null && !native.error;
  let token = false; try { assertWindowsReadReply(native.stdout); token = true; } catch { /* refusal has no success authority */ }
  const after = acl(file), current = await lstat(file);
  const result = { name, operation, file, expected, golden, goldenError, native: { status: native.status, signal: native.signal, error: native.error?.message ?? null,
    stdout: native.stdout.toString(), stderr: native.stderr.toString() }, before, after, readonly: isDeepStrictEqual(before, after) && info.mode === current.mode && info.ino === current.ino && info.dev === current.dev &&
      (content === null || content.equals(await readFile(file))), pass: false };
  results.push(result);
  assert.equal(native.error, undefined); assert.equal(native.signal, null); assert.equal(golden, expected, 'Golden fixture predicate did not match intended case: ' + name);
  assert.equal(accepted, golden, 'Native/unchanged PowerShell parity differs: ' + name); assert.equal(token, accepted);
  if (!accepted) assert.match(native.stderr.toString(), /^GESTALT_STORAGE_CHECK_V1 /);
  assert.equal(result.readonly, true, 'Guard mutated owner/group/DACL/data/mode: ' + name); result.pass = true;
}
try {
  for (const name of ['positive ü space', 'others-read', 'others-write', 'inherit-only', 'deny-read', 'null', 'empty', 'changing', 'junction-target']) {
    const file = path.join(storage.root, name); await mkdir(file);
    const before = acl(file), snapshot = acl(file, 'apply', name.startsWith('positive') || ['changing', 'junction-target'].includes(name) ? 'private' : name);
    construction.push({ file, before, after: snapshot }); assert.equal(before.owner, snapshot.owner); assert.equal(before.group, snapshot.group); assert.equal(snapshot.owner, snapshot.sid);
    assert.equal(snapshot.administrator, false); if (name === 'null') assert.equal(snapshot.nullDacl, true); if (name === 'empty') assert.equal(snapshot.aceCount, 0);
    if (['others-read', 'others-write', 'inherit-only'].includes(name)) assert.ok(snapshot.rules.some(rule => rule.sid === 'S-1-1-0' && rule.type === 'Allow'), 'Actual Everyone allow was not established');
  }
  const positive = path.join(storage.root, 'positive ü space');
  await check('private directory', positive, 'assert-private', true); await check('namespace directory', positive, 'assert-namespace', true);
  const file = path.join(positive, 'resource ü.txt'); await writeFile(file, 'retained-data', { mode: 0o600 }); acl(file, 'apply', 'private'); await check('private file', file, 'assert-private', true);
  await check('another principal read private', path.join(storage.root, 'others-read'), 'assert-private', false);
  await check('another principal read namespace', path.join(storage.root, 'others-read'), 'assert-namespace', true);
  await check('another principal write namespace', path.join(storage.root, 'others-write'), 'assert-namespace', false);
  await check('inherit-only namespace', path.join(storage.root, 'inherit-only'), 'assert-namespace', true);
  await check('inherit-only private', path.join(storage.root, 'inherit-only'), 'assert-private', false);
  await check('deny does not replace conservative allow predicate', path.join(storage.root, 'deny-read'), 'assert-private', true);
  for (const operation of ['assert-private', 'assert-namespace']) await check('null DACL ' + operation, path.join(storage.root, 'null'), operation, false);
  await check('empty DACL private', path.join(storage.root, 'empty'), 'assert-private', false);
  await check('empty DACL namespace', path.join(storage.root, 'empty'), 'assert-namespace', true);
  await check('existing system owner private refusal', process.env.SystemRoot, 'assert-private', false);
  const junction = path.join(storage.root, 'junction'); const { symlink } = await import('node:fs/promises'); await symlink(path.join(storage.root, 'junction-target'), junction, 'junction');
  assert.equal((await lstat(junction)).isSymbolicLink(), true); await check('ordinary junction refused', junction, 'assert-private', false);
  const changing = path.join(storage.root, 'changing'); await check('fresh allow', changing, 'assert-private', true);
  acl(changing, 'apply', 'others-read'); await check('same path revalidated after ACL change', changing, 'assert-private', false);
  acl(changing, 'apply', 'private'); await check('restored private ACL', changing, 'assert-private', true);
  let cursor = path.dirname(storage.root); const parents = [];
  while (true) { parents.push(cursor); if (cursor === path.dirname(cursor)) break; cursor = path.dirname(cursor); }
  expectedRequiredTests = 23 + parents.length;
  for (const parent of parents) await check('actual namespace ancestor', parent, 'assert-namespace', true);
  const missing = path.join(storage.root, 'missing'), missingRequest = { operation: 'assert-private', path: missing };
  let goldenMissing = false; try { await windowsSystem(missingRequest); } catch { goldenMissing = true; }
  const missingResult = spawnSync(helper, windowsReadArguments(missingRequest), { timeout: 10000, shell: false, windowsHide: true });
  const missingPass = goldenMissing && missingResult.status === 1 && missingResult.signal === null && !missingResult.error && missingResult.stdout.length === 0;
  results.push({ name: 'missing path refuses without creation', goldenRefused: goldenMissing, nativeStatus: missingResult.status, pass: missingPass });
  assert.equal(missingPass, true); await assert.rejects(lstat(missing), { code: 'ENOENT' });
  for (const [name, argv] of [
    ['wrong mode', ['--storage-check-v1', 'execute', '--', path.toNamespacedPath(positive)]],
    ['extra private path', ['--storage-check-v1', 'private', '--', path.toNamespacedPath(positive), path.toNamespacedPath(positive)]],
    ['noncanonical path', ['--storage-check-v1', 'private', '--', positive]],
    ['129 namespace paths', ['--storage-check-v1', 'namespace', '--', ...Array(129).fill(path.toNamespacedPath(positive))]],
  ]) {
    const native = spawnSync(helper, argv, { timeout: 10000, shell: false, windowsHide: true });
    const pass = native.status === 2 && native.signal === null && !native.error && native.stdout.length === 0;
    results.push({ name, argv, native: { status: native.status, signal: native.signal, stderr: native.stderr.toString() }, pass }); assert.equal(pass, true);
  }
} catch (error) { failure = { message: error.message, stack: error.stack }; }
finally {
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ kind: 'ordinary-windows-native-storage-read-parity', pass: !failure,
    tests: results.length, passed: results.filter(result => result.pass).length, failed: results.filter(result => !result.pass).length,
    expectedRequiredTests, construction,
    harnessFailures: failure && results.every(result => result.pass) ? 1 : 0,
    skip: 0, failure: failure ?? null, results, fixture: storage.root, nativeHelper: provenance,
    optionalUNC: { executed: false, reason: 'No approved remote share fixture; no UNC support proof claimed' },
    scope: 'Required local NTFS cases cannot skip. Exact readonly DACL/owner/group/data/mode comparisons and fresh ACL mutation. No lifetime/kernel release claim.' }, null, 2) + '\n');
}
assert.equal(failure, undefined, failure?.stack);
