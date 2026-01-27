import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { hashSourceFiles, recentIndexAge, saveMetadata } from '../../src/lib/index-metadata.js';

const tenMinutesMs = 10 * 60 * 1000;

test('hashSourceFiles is deterministic', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-meta-'));
  const filePath = path.join(root, 'main.go');
  fs.writeFileSync(filePath, 'package main\n', 'utf8');

  const first = hashSourceFiles(root, ['go']);
  const second = hashSourceFiles(root, ['go']);

  assert.equal(first, second);
});

test('recentIndexAge reports recent metadata', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-meta-'));
  const indexPath = path.join(root, 'index.scip');
  fs.writeFileSync(indexPath, 'scip', 'utf8');

  saveMetadata(indexPath, {
    created_at: new Date().toISOString(),
    project_root: root,
    files_hashed: 'hash',
    languages: ['go'],
  });

  const result = recentIndexAge(indexPath, tenMinutesMs);
  assert.equal(result.recent, true);
  assert.ok(result.ageMs <= tenMinutesMs);
});
