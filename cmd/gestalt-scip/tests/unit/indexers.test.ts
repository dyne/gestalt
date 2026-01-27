import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import zlib from 'node:zlib';

import { detectLanguages, downloadBinary } from '../../src/lib/indexers.js';

function buildTarGz(name: string, content: Buffer): Buffer {
  const header = Buffer.alloc(512, 0);
  header.write(name, 0, 100, 'utf8');
  header.write('0000777\0', 100, 8, 'ascii');
  header.write('0000000\0', 108, 8, 'ascii');
  header.write('0000000\0', 116, 8, 'ascii');
  const sizeOct = content.length.toString(8).padStart(11, '0');
  header.write(`${sizeOct}\0`, 124, 12, 'ascii');
  const mtimeOct = Math.floor(Date.now() / 1000).toString(8).padStart(11, '0');
  header.write(`${mtimeOct}\0`, 136, 12, 'ascii');
  header.fill(' ', 148, 156);
  header[156] = 0;
  header.write('ustar', 257, 5, 'ascii');
  header.write('00', 263, 2, 'ascii');

  let checksum = 0;
  for (const byte of header) {
    checksum += byte;
  }
  const checksumOct = checksum.toString(8).padStart(6, '0');
  header.write(`${checksumOct}\0 `, 148, 8, 'ascii');

  const size = content.length;
  const paddedSize = Math.ceil(size / 512) * 512;
  const body = Buffer.alloc(paddedSize, 0);
  content.copy(body, 0, 0, content.length);
  const end = Buffer.alloc(1024, 0);
  const tar = Buffer.concat([header, body, end]);
  return zlib.gzipSync(tar);
}

test('detectLanguages finds supported markers', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-indexers-'));
  fs.writeFileSync(path.join(root, 'go.mod'), 'module example\n', 'utf8');
  fs.writeFileSync(path.join(root, 'package.json'), '{"name":"demo"}', 'utf8');
  fs.writeFileSync(path.join(root, 'requirements.txt'), 'requests', 'utf8');

  const languages = detectLanguages(root);

  assert.deepEqual(languages, ['go', 'typescript', 'python']);
});

test('downloadBinary extracts tar.gz archives from file URLs', async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-indexer-'));
  const archivePath = path.join(root, 'scip-go.tar.gz');
  const destination = path.join(root, 'scip-go');
  const payload = Buffer.from('binary-content');
  const archive = buildTarGz('scip-go', payload);
  fs.writeFileSync(archivePath, archive);

  await downloadBinary(`file://${archivePath}`, destination);

  assert.ok(fs.existsSync(destination));
  assert.equal(fs.readFileSync(destination, 'utf8'), payload.toString('utf8'));
});
