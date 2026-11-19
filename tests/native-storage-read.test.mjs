import assert from 'node:assert/strict';
import test from 'node:test';
import { randomUUID, createHash } from 'node:crypto';
import { readFile, mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { windowsReadArguments, assertWindowsReadReply, storageReadToken, windowsReadFailure } from '../src/cli/runtime/windows-scratch-read.mjs';
import { createScratchOwner } from '../src/cli/runtime/scratch-owner.mjs';
import { releasedBorrowerInventory } from './fixtures/native-released-borrowers.mjs';
import { verifyOwnershipBuildInputs } from './fixtures/native-runtime/source-binding.mjs';
import { createReadinessObserver } from './fixtures/native-cli-spawn-capture.mjs';

test('Windows native read requests preserve literal paths and reject unsupported/missing/oversized operations', () => {
  const request = { operation: 'assert-private', path: 'C:\\private space é\\lease.json' }, before = { ...request };
  assert.deepEqual(windowsReadArguments(request), ['--storage-check-v1', 'private', '--', '\\\\?\\C:\\private space é\\lease.json']); assert.deepEqual(request, before);
  assert.equal(windowsReadArguments({ operation: 'assert-namespace', paths: Array(128).fill('C:\\private') }).length, 131); // C argc additionally includes argv[0].
  for (const bad of [{ operation: 'create-directory', path: request.path }, { operation: 'assert-private', path: 'C:relative' },
    { operation: 'assert-private', path: 'C:\\nul\0byte' }, { operation: 'assert-namespace', paths: [] }, { operation: 'assert-namespace', paths: Array(129).fill(request.path) }])
    assert.throws(() => windowsReadArguments(bad));
});
test('native read forwarding preserves an already extended drive root without removing its separator', () => {
  const root = '\\\\?\\C:\\';
  assert.equal(root.length, 7);
  const request = Object.freeze({ operation: 'assert-namespace', paths: Object.freeze([root]) });
  assert.deepEqual(windowsReadArguments(request), ['--storage-check-v1', 'namespace', '--', root]);
});
test('native read path conversion happens once and forwarding is idempotent without changing requests', () => {
  // UNC vectors establish literal argv only, not remote filesystem support.
  const vectors = [
    ['C:\\', '\\\\?\\C:\\'], ['C:/', '\\\\?\\C:\\'], ['\\\\?\\C:\\', '\\\\?\\C:\\'],
    ['C:\\folder\\resource', '\\\\?\\C:\\folder\\resource'], ['\\\\?\\C:\\folder\\resource', '\\\\?\\C:\\folder\\resource'],
    ['C:\\space ü\\résumé.txt', '\\\\?\\C:\\space ü\\résumé.txt'], ['\\\\?\\C:\\space ü\\résumé.txt', '\\\\?\\C:\\space ü\\résumé.txt'],
    ['\\\\server\\share\\space ü', '\\\\?\\UNC\\server\\share\\space ü'], ['\\\\?\\UNC\\server\\share\\space ü', '\\\\?\\UNC\\server\\share\\space ü'],
    ['\\\\?\\UNC\\server\\share\\', '\\\\?\\UNC\\server\\share\\'],
  ];
  for (const [input, expected] of vectors) for (const operation of ['assert-private', 'assert-namespace']) {
    const request = operation === 'assert-private' ? { operation, path: input } : { operation, paths: Object.freeze([input]) };
    const before = structuredClone(request); Object.freeze(request);
    const argv = windowsReadArguments(request);
    assert.deepEqual(argv, ['--storage-check-v1', operation === 'assert-private' ? 'private' : 'namespace', '--', expected]);
    assert.deepEqual(request, before);
    const forwarded = operation === 'assert-private' ? { operation, path: argv[3] } : { operation, paths: argv.slice(3) };
    assert.deepEqual(windowsReadArguments(forwarded), argv);
  }
});
test('native storage success requires its exact token and never accepts process ownership frames or noisy/partial replies', () => {
  assert.doesNotThrow(() => assertWindowsReadReply(Buffer.from(storageReadToken)));
  for (const bad of ['', 'GESTALT_STORAGE_CHECK_V1_OK', storageReadToken.replace('\n', '\r\n'), '\uFEFF' + storageReadToken,
    storageReadToken + 'extra', 'Rwindows-job', '{"kind":"released"}']) assert.throws(() => assertWindowsReadReply(Buffer.from(bad)));
  assert.throws(() => createScratchOwner(undefined, 'relative-helper'));
  assert.match(windowsReadFailure({ stderr: Buffer.from('GESTALT_STORAGE_CHECK_V1 guard error=50 index=0\n') }), /unsupported/);
  assert.match(windowsReadFailure({ code: 'ENOENT' }), /matching target-built helper/);
});
test('passive observation labels the literal native read mode while preserving the original synchronous result', () => {
  const observer = createReadinessObserver(), result = Buffer.from(storageReadToken), receiver = {};
  const wrapped = observer.sync(function (...args) { assert.equal(this, receiver); assert.deepEqual(args, ['/fixed/helper', ['--storage-check-v1', 'private', '--', 'literal'], {}]); return result; }, 'execFileSync');
  assert.equal(wrapped.call(receiver, '/fixed/helper', ['--storage-check-v1', 'private', '--', 'literal'], {}), result);
  assert.equal(observer.report().events[0].privateOperation, 'native-read-private');
});
test('native read stage diagnostics remain refusals and preserve stage, error and path index', () => {
  for (const stage of ['metadata', 'type', 'security', 'descriptor', 'owner', 'ace', 'full']) {
    const stderr = Buffer.from(`GESTALT_STORAGE_CHECK_V1 ${stage} error=5 index=127\n`);
    assert.equal(windowsReadFailure({ stderr }), `Windows private namespace/ACL validation failed (${stage}, error 5, path index 127)`);
    assert.throws(() => assertWindowsReadReply(stderr));
  }
  assert.match(windowsReadFailure({ stderr: Buffer.from('GESTALT_STORAGE_CHECK_V1 ace error=50 index=0\n') }), /representation is unsupported/);
  for (const incomplete of ['GESTALT_STORAGE_CHECK_V1 security error=5', 'GESTALT_STORAGE_CHECK_V1 metadata error=5 index=0\nextra'])
    assert.match(windowsReadFailure({ stderr: Buffer.from(incomplete) }), /unconfirmed/);
});
function inventory() {
  const context = { ownerId: 'service-' + randomUUID(), workspacePath: 'C:\\private\\workspace' }, primaryId = randomUUID();
  const make = (leaseId, state, consumers) => ({ file: 'C:\\private\\leases\\' + leaseId + '.json', sha256: 'a'.repeat(64), lease: {
    schemaVersion: 1, ownerId: context.ownerId, workspacePath: context.workspacePath, leaseId, state, processIdentity: randomUUID(),
    createdAt: '2026-11-17T00:00:00.000Z', updatedAt: '2026-11-17T00:01:00.000Z', consumers,
  } });
  const primary = { context, lease: { leaseId: primaryId } }, before = [make(primaryId, 'active', ['fixture'])];
  return { primary, before, after: [...structuredClone(before), ...Array.from({ length: 5 }, () => make(randomUUID(), 'released', []))] };
}
test('released borrower schema uses empty consumers with exact identities while separately retaining five-record readiness', () => {
  const { primary, before, after } = inventory(), result = releasedBorrowerInventory(primary, before, after, 5);
  assert.equal(result.pass, true); assert.equal(result.kernelReleaseClaimed, false); assert.equal(result.liveConsumerLabelAfterReleaseClaimed, false);
  assert.equal(releasedBorrowerInventory(primary, before, after.slice(0, -1)).pass, true);
  assert.equal(releasedBorrowerInventory(primary, before, after.slice(0, -1), 5).pass, false);
});
for (const fault of ['live-consumer', 'wrong-owner', 'wrong-workspace', 'duplicate-identity', 'filename', 'schema', 'date', 'primary-changed', 'missing-existing', 'active', 'missing-inventory'])
  test('released borrower gate refuses ' + fault, () => {
    const { primary, before, after } = inventory();
    if (fault === 'live-consumer') after[1].lease.consumers = ['manager-runtime'];
    if (fault === 'wrong-owner') after[1].lease.ownerId += 'other';
    if (fault === 'wrong-workspace') after[1].lease.workspacePath += 'other';
    if (fault === 'duplicate-identity') after[1].lease.processIdentity = after[2].lease.processIdentity;
    if (fault === 'filename') after[1].file += '.other';
    if (fault === 'schema') after[1].lease.schemaVersion = 2;
    if (fault === 'date') after[1].lease.updatedAt = 'not a date';
    if (fault === 'primary-changed') after[0].sha256 = 'b'.repeat(64);
    if (fault === 'missing-existing') after.shift();
    if (fault === 'active') after[1].lease.state = 'active';
    assert.equal(releasedBorrowerInventory(primary, before, fault === 'missing-inventory' ? undefined : after, 5).pass, false);
  });
test('Windows native provenance requires every translation and header byte, not only the original ownership C', async t => {
  const directory = await mkdtemp(path.join(tmpdir(), 'native-read-inputs-')); t.after(() => rm(directory, { recursive: true, force: true }));
  const paths = ['owned-process.c', 'windows-storage-check.c', 'windows-storage-check.h'].map(name => 'src/cli/platform/native/' + name), sourceInputs = [];
  for (const relative of paths) { const file = path.join(directory, relative), bytes = Buffer.from(relative); await mkdir(path.dirname(file), { recursive: true }); await writeFile(file, bytes); sourceInputs.push({ path: relative, bytes: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex') }); }
  const provenance = { platform: 'win32', sourceSha256: sourceInputs[0].sha256, sourceInputs };
  assert.deepEqual(await verifyOwnershipBuildInputs(provenance, directory), sourceInputs);
  await assert.rejects(verifyOwnershipBuildInputs({ ...provenance, sourceInputs: sourceInputs.slice(0, 1) }, directory));
  await writeFile(path.join(directory, paths[2]), 'changed header'); await assert.rejects(verifyOwnershipBuildInputs(provenance, directory));
});
test('native read hooks preserve shared and Unix code and the reviewed Windows ownership baseline', async () => {
  const source = await readFile(new URL('../src/cli/platform/native/owned-process.c', import.meta.url), 'utf8');
  const include = '#include "windows-storage-check.h"\n';
  const dispatch = `  if (argc > 1 && !wcscmp(argv[1], L"--storage-check-v1")) {\n    size_t units;\n    if (!command_line_quote(argc, argv, NULL, WINDOWS_COMMAND_LINE_UNITS - 1, &units)) return 2;\n    return gestalt_windows_storage_check(argc, argv);\n  }\n`;
  assert.equal(source.split(include).length, 2);
  assert.equal(source.split(dispatch).length, 2);
  assert.ok(source.includes(`#include <fcntl.h>\n${include}#else\n#include <unistd.h>`));
  const windowsEntry = '\n#ifdef _WIN32\nint wmain(int argc, wchar_t **argv) {\n';
  assert.ok(source.includes(`${windowsEntry}${dispatch}  InitializeCriticalSection(&output_lock);`));

  const splitAt = (body, boundary) => {
    const parts = body.split(boundary);
    assert.equal(parts.length, 2, 'Expected one platform boundary: ' + boundary.trim());
    return [parts[0], boundary + parts[1]];
  };
  const digest = body => createHash('sha256').update(body).digest('hex');
  const withoutStorage = source.replace(include, '').replace(dispatch, '');
  const [sharedAndUnix, ownershipTail] = splitAt(withoutStorage, '\n#else\nstatic volatile LONG output_failed = 0;\n');
  const [windowsOwnership, entryPoints] = splitAt(ownershipTail, '\n#endif\n\n#ifndef _WIN32\nint main(');
  const [unixMain, windowsMain] = splitAt(entryPoints, windowsEntry);

  // These regions match the accepted 283c74b helper before the Windows byte-input correction.
  assert.equal(digest(sharedAndUnix + unixMain), 'c33fde90b40c0e60d1376dd9d617ac3109d620e8a869a898959e846567cd39ac');
  assert.equal(digest(windowsMain), '284f6d2b86dc0ee89bb2ab7bc575bac190c14c88f2289053411548f7d0215b4b');
  // Only the Windows ownership region is re-sealed for the reviewed byte-input worker (source SHA-256 8b8499245e42).
  assert.equal(digest(windowsOwnership), '67c94f4bda37b138897f671aae5e044f51c7a0c2c33454559dcf865d3028dc6c');
});
