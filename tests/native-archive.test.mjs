import assert from 'node:assert/strict';
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile, link, truncate } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { gunzipSync, gzipSync } from 'node:zlib';
import test from 'node:test';
import { NativeArchives } from '../src/cli/platform/archive.ts';
import { windowsSystem } from '../src/cli/platform/windows-system.ts';
import { execFileSync } from 'node:child_process';

const archives = new NativeArchives();
async function privateStage(directory) {
  if (process.platform === 'win32') await windowsSystem({ operation: 'create-directory', path: directory });
  else await mkdir(directory, { mode: 0o700 });
}
async function fixture(t) {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt archive é '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = path.join(root, 'gestalt-skills'); await mkdir(source);
  await mkdir(path.join(source, 'nested space 東京'));
  await writeFile(path.join(source, 'nested space 東京', 'file'), Buffer.from([0, 255, 10, 13]));
  const archive = path.join(root, 'archive.tar.gz'); await archives.create(source, archive);
  return { root, source, archive };
}
test('Node archive round trip preserves names and binary bytes without external tools', async t => {
  const { root, archive } = await fixture(t), stage = path.join(root, 'stage'); await privateStage(stage);
  assert.deepEqual(await archives.list(archive), [
    { path: 'gestalt-skills', kind: 'directory' }, { path: 'gestalt-skills/nested space 東京', kind: 'directory' },
    { path: 'gestalt-skills/nested space 東京/file', kind: 'file' },
  ]);
  await archives.extract(archive, stage, 'gestalt-skills');
  assert.deepEqual(await readFile(path.join(stage, 'gestalt-skills/nested space 東京/file')), Buffer.from([0, 255, 10, 13]));
  await assert.rejects(archives.create(path.join(stage, 'gestalt-skills'), archive), /EEXIST/);
});

async function altered(archive, mutate) {
  const bytes = gunzipSync(await readFile(archive)); mutate(bytes);
  // Recompute each modified header's checksum; test validation beyond integrity.
  for (let offset = 0; offset + 512 <= bytes.length; ) {
    const header = bytes.subarray(offset, offset + 512); if (header.every(b => b === 0)) break;
    header.fill(32, 148, 156); const sum = header.reduce((total, b) => total + b, 0);
    header.write(sum.toString(8).padStart(7, '0') + '\0', 148, 8, 'ascii');
    offset += 512 + Math.ceil(Number.parseInt(header.subarray(124, 136).toString().replace(/\0.*$/, ''), 8) / 512) * 512;
  }
  await writeFile(archive, gzipSync(bytes));
}
for (const unsafe of ['/escape', '../escape', 'gestalt-skills/../escape', 'gestalt-skills\\escape', 'C:/escape', 'gestalt-skills/CON', 'gestalt-skills/a.'])
  test(`archive rejects unsafe path ${JSON.stringify(unsafe)} before any extraction`, async t => {
    const { root, archive } = await fixture(t), stage = path.join(root, 'stage'); await privateStage(stage);
    await altered(archive, bytes => { bytes.fill(0, 0, 100); bytes.write(unsafe); });
    await assert.rejects(archives.extract(archive, stage, 'gestalt-skills'), /Unsafe/);
    assert.deepEqual(await readdir(stage), []);
  });
for (const type of ['1', '2', '3', '6', 'K']) test(`archive rejects tar type ${type} before extraction`, async t => {
  const { root, archive } = await fixture(t), stage = path.join(root, 'stage'); await privateStage(stage);
  await altered(archive, bytes => { bytes[156] = type.charCodeAt(0); });
  await assert.rejects(archives.extract(archive, stage, 'gestalt-skills'), /link or special/);
  assert.deepEqual(await readdir(stage), []);
});
test('archive accepts existing native tar PAX names/metadata without adopting owner, mode or extension authority', async t => {
  const { root, source } = await fixture(t), archive = path.join(root, 'existing-pax.tar.gz'), stage = path.join(root, 'stage'); await privateStage(stage);
  execFileSync('tar', ['--format=pax', '-czf', archive, '-C', path.dirname(source), path.basename(source)], { shell: false, stdio: 'pipe' });
  await archives.extract(archive, stage, 'gestalt-skills');
  assert.deepEqual(await readFile(path.join(stage, 'gestalt-skills/nested space 東京/file')), Buffer.from([0, 255, 10, 13]));
});
test('archive rejects case collisions, wrong roots, damaged checksums and trailing payload', async t => {
  const { root, archive } = await fixture(t), original = await readFile(archive), stage = path.join(root, 'stage'); await privateStage(stage);
  await altered(archive, bytes => { bytes.fill(0, 512, 612); bytes.write('GESTALT-SKILLS', 512); });
  await assert.rejects(archives.extract(archive, stage, 'gestalt-skills'), /case-colliding/);
  await writeFile(archive, original); await assert.rejects(archives.extract(archive, stage, 'other-root'), /Unexpected/);
  const checksum = gunzipSync(original); checksum[1] ^= 1; await writeFile(archive, gzipSync(checksum));
  await assert.rejects(archives.list(archive), /checksum/);
  await writeFile(archive, gzipSync(Buffer.concat([gunzipSync(original), Buffer.from('extra')])));
  await assert.rejects(archives.list(archive), /trailing/); assert.deepEqual(await readdir(stage), []);
});
test('archive creation refuses hardlinks and existing extraction payloads', async t => {
  const { root, source, archive } = await fixture(t), stage = path.join(root, 'stage'); await privateStage(stage);
  await writeFile(path.join(stage, 'keep'), 'private data');
  await assert.rejects(archives.extract(archive, stage, 'gestalt-skills'), /empty/);
  await link(path.join(source, 'nested space 東京/file'), path.join(source, 'hardlink'));
  await assert.rejects(archives.create(source, path.join(root, 'bad.tar.gz')), /link/);
  assert.equal(await readFile(path.join(stage, 'keep'), 'utf8'), 'private data');
});
test('archive refuses oversized compressed input and declared payload before extraction', async t => {
  const { root, archive } = await fixture(t), original = await readFile(archive), stage = path.join(root, 'stage'); await privateStage(stage);
  await truncate(archive, 64 * 1024 * 1024 + 1);
  await assert.rejects(archives.extract(archive, stage, 'gestalt-skills'), /bounded file/); assert.deepEqual(await readdir(stage), []);
  await writeFile(archive, original);
  await altered(archive, bytes => { bytes.write((256 * 1024 * 1024 + 1).toString(8).padStart(11, '0') + '\0', 124, 12, 'ascii'); });
  await assert.rejects(archives.extract(archive, stage, 'gestalt-skills'), /member size/); assert.deepEqual(await readdir(stage), []);
});
