import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

let cachedDiscovery: any;

async function loadDiscovery(): Promise<any> {
  if (!cachedDiscovery) {
    cachedDiscovery = await import('../../src/scip-discovery.js');
  }
  return cachedDiscovery;
}

function createTempDir(prefix: string): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

function writeEmptyFile(filePath: string): void {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, '');
}

test('discoverScipFiles finds indexes in nearest .gestalt/scip directory', async () => {
  const { discoverScipFiles } = await loadDiscovery();
  const repoRoot = createTempDir('gestalt-scip-');
  const scipDir = path.join(repoRoot, '.gestalt', 'scip');
  writeEmptyFile(path.join(scipDir, 'index.scip'));
  writeEmptyFile(path.join(scipDir, 'index-go.scip'));

  const nestedDir = path.join(repoRoot, 'src', 'internal', 'terminal');
  fs.mkdirSync(nestedDir, { recursive: true });

  const files = discoverScipFiles(nestedDir);
  const languages = files.map((file: any) => file.language).sort();

  assert.deepEqual(languages, ['default', 'go']);
  assert.equal(files.length, 2);
});

test('discoverScipFiles extracts language from filename patterns', async () => {
  const { discoverScipFiles } = await loadDiscovery();
  const repoRoot = createTempDir('gestalt-scip-lang-');
  const scipDir = path.join(repoRoot, '.gestalt', 'scip');
  writeEmptyFile(path.join(scipDir, 'index-typescript.scip'));

  const files = discoverScipFiles(repoRoot);

  assert.equal(files.length, 1);
  assert.equal(files[0].language, 'typescript');
});

test('discoverScipFiles returns empty array when no indexes exist', async () => {
  const { discoverScipFiles } = await loadDiscovery();
  const repoRoot = createTempDir('gestalt-scip-empty-');

  const files = discoverScipFiles(repoRoot);

  assert.deepEqual(files, []);
});

test('discoverScipFiles respects explicit directory override with .scip files', async () => {
  const { discoverScipFiles } = await loadDiscovery();
  const repoRoot = createTempDir('gestalt-scip-override-');
  const scipDir = path.join(repoRoot, '.gestalt', 'scip');
  writeEmptyFile(path.join(scipDir, 'index-go.scip'));

  const overrideDir = path.join(repoRoot, 'custom-scip');
  writeEmptyFile(path.join(overrideDir, 'index-typescript.scip'));

  const files = discoverScipFiles(overrideDir);

  assert.equal(files.length, 1);
  assert.equal(files[0].language, 'typescript');
  assert.equal(path.dirname(files[0].path), overrideDir);
});

test('loadAllScipFiles loads all .scip files from a directory', async () => {
  const { loadAllScipFiles } = await loadDiscovery();
  const repoRoot = createTempDir('gestalt-scip-load-');
  const scipDir = path.join(repoRoot, '.gestalt', 'scip');
  writeEmptyFile(path.join(scipDir, 'index.scip'));
  writeEmptyFile(path.join(scipDir, 'index-go.scip'));

  const indexes = loadAllScipFiles(scipDir);

  assert.equal(indexes.size, 2);
  assert.ok(indexes.has('default'));
  assert.ok(indexes.has('go'));
});
