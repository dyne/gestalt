import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { verifySourceRecord } from './fixtures/native-runtime/source-binding.mjs';
import { verifyFrozenMetadata, saveHttpFailure } from './fixtures/native-runtime/codex.mjs';
import { encodeFrame, decodeFrames, terminalVariants } from './fixtures/native-windows-terminal-wire.mjs';

test('Windows diagnostic variants bind one current source location and preserve LF/CRLF checkout identity and ownership checks', async () => {
  const source = await readFile(new URL('../src/cli/platform/native/owned-process.c', import.meta.url), 'utf8');
  for (const eol of ['\n', '\r\n']) {
    const actual = source.replaceAll('\r\n', '\n').replaceAll('\n', eol), variants = terminalVariants(actual);
    assert.equal(variants[0].source, actual);
    for (const variant of variants.slice(1)) {
      assert.equal(variant.source.includes('\r\n'), eol === '\r\n');
      for (const proof of ['PROC_THREAD_ATTRIBUTE_JOB_LIST', 'JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE', 'WaitForSingleObject(process.hProcess, 5000)', "message('D', \"windows-job\")"])
        assert.equal(variant.source.split(proof).length, actual.split(proof).length);
    }
    assert.throws(() => terminalVariants(actual.replace('BOOL created = line && CreateProcessW', 'changed')), /Exactly one/);
  }
});

test('Windows preflight preserves binary chunks, distinguishes incomplete frames and never resynchronizes corrupt stdout into release', () => {
  const bytes = Buffer.from('é東京\x04'), ready = encodeFrame('R', Buffer.from('windows-job'));
  const output = encodeFrame('O', bytes), done = encodeFrame('D', Buffer.from('windows-job'));
  const raw = Buffer.concat([ready, output, done]);
  for (let length = 0; length < raw.length; length++) {
    const decoded = decodeFrames(raw.subarray(0, length)); assert.equal(decoded.error, undefined);
    assert.ok(decoded.frames.every(f => f.kind !== 'D'));
  }
  assert.deepEqual(decodeFrames(raw).frames.map(f => [f.kind, f.bytes]), [['R', Buffer.from('windows-job')], ['O', bytes], ['D', Buffer.from('windows-job')]]);
  assert.equal(decodeFrames(raw).pendingBytes, 0);
  for (const corruption of [Buffer.from('TARGET_STDOUT_READY'), Buffer.from([0, 0, 0, 0]), Buffer.from([1, 0, 0, 0, 255])]) {
    const decoded = decodeFrames(Buffer.concat([ready, corruption, done]));
    assert.equal(decoded.error.offset, ready.length); assert.deepEqual(decoded.frames.map(f => f.kind), ['R']);
  }
});

test('source binding accepts canonical checkout and origin umask modes with identical bytes/blob/executable bits', async () => {
  const entries = JSON.parse(await readFile(new URL('./fixtures/native-runtime/agents-source.json', import.meta.url))).files;
  for (const expected of entries) {
    for (const mode of [expected.gitMode.slice(-3), expected.physicalMode]) {
      const actual = { bytes: expected.bytes, sha256: expected.sha256, mode, regularFile: true };
      const observed = verifySourceRecord(expected, actual, `${expected.gitMode} blob ${expected.blobOid}\t${expected.path}`);
      assert.equal(observed.mode, mode); assert.equal(observed.canonicalPhysicalMode, expected.gitMode.slice(-3));
      assert.equal(observed.physicalModeDelta, mode === expected.gitMode.slice(-3) ? '0' : '20');
    }
  }
});
test('source binding refuses corrupt bytes/blob, wrong executable class, world write and special mode bits', () => {
  const bytes = Buffer.from('fixed source\n'), sha256 = createHash('sha256').update(bytes).digest('hex');
  const blobOid = createHash('sha1').update(Buffer.from(`blob ${bytes.length}\0`)).update(bytes).digest('hex');
  const expected = { path: 'fixture.mjs', gitMode: '100644', blobOid, bytes: bytes.length, sha256, physicalMode: '664' };
  const actual = { regularFile: true, bytes: bytes.length, sha256, mode: '644' }, tree = `100644 blob ${blobOid}\tfixture.mjs`;
  assert.throws(() => verifySourceRecord(expected, { ...actual, sha256: '0'.repeat(64) }, tree), /byte digest/);
  assert.throws(() => verifySourceRecord(expected, actual, tree.replace(blobOid, '0'.repeat(40))), /blob or mode/);
  assert.throws(() => verifySourceRecord(expected, actual, tree.replace('100644', '100755')), /blob or mode/);
  for (const mode of ['755', '666', '4644']) assert.throws(() => verifySourceRecord(expected, { ...actual, mode }, tree));
  assert.throws(() => verifySourceRecord(expected, { ...actual, regularFile: false }, tree), /regular source/);
});
test('frozen official metadata retains exact source pin and fixed published digest for every target', async () => {
  const metadata = JSON.parse(await readFile(new URL('./fixtures/native-runtime/codex-release.json', import.meta.url)));
  assert.equal(metadata.assets.length, 5);
  for (const asset of metadata.assets) assert.deepEqual(verifyFrozenMetadata(metadata, asset.name, asset.publishedDigest.slice(7)), asset);
  const first = metadata.assets[0];
  assert.throws(() => verifyFrozenMetadata(metadata, first.name, '0'.repeat(64)), /artifact digest differs/);
  assert.throws(() => verifyFrozenMetadata({ ...metadata, commit: '0'.repeat(40) }, first.name, first.publishedDigest.slice(7)));
});
test('HTTP failure evidence retains status/rate headers and body while excluding credentials and URL query', async () => {
  const directory = await mkdtemp(path.join(tmpdir(), 'http-diagnostic-'));
  try {
    const response = new Response('{"message":"denied"}', { status: 403, statusText: 'Forbidden', headers: {
      'x-ratelimit-remaining': '0', 'x-github-request-id': 'fixture', Authorization: 'secret-value', 'Set-Cookie': 'secret-cookie',
    } });
    const file = await saveHttpFailure(response, directory, 'https://github.com/openai/codex?token=secret-query');
    const text = await readFile(file, 'utf8'), record = JSON.parse(text);
    assert.equal(record.status, 403); assert.equal(record.headers['x-ratelimit-remaining'], '0'); assert.match(record.body, /denied/);
    assert.equal(record.bodyTruncated, false); assert.doesNotMatch(text, /secret-value|secret-cookie|secret-query|Authorization|Set-Cookie/);
  } finally { await rm(directory, { recursive: true, force: true }); }
});
