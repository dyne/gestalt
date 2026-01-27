import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { scip } from '../../src/bundle/scip.js';
import { mergeIndexes } from '../../src/lib/scip-merge.js';

function writeIndex(filePath: string, index: InstanceType<typeof scip.Index>): void {
  const payload = Buffer.from(index.serializeBinary());
  fs.writeFileSync(filePath, payload);
}

test('mergeIndexes rejects duplicate document paths', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-merge-'));
  const indexOne = new scip.Index({
    documents: [new scip.Document({ relative_path: 'src/app.ts' })],
  });
  const indexTwo = new scip.Index({
    documents: [new scip.Document({ relative_path: 'src/app.ts' })],
  });
  const onePath = path.join(root, 'one.scip');
  const twoPath = path.join(root, 'two.scip');
  const outputPath = path.join(root, 'out.scip');
  writeIndex(onePath, indexOne);
  writeIndex(twoPath, indexTwo);

  assert.throws(() => mergeIndexes([onePath, twoPath], outputPath), /duplicate document path/);
});

test('mergeIndexes preserves metadata and de-dupes external symbols', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-merge-'));
  const metadata = new scip.Metadata({ project_root: 'root-one' });
  const indexOne = new scip.Index({
    metadata,
    documents: [new scip.Document({ relative_path: 'src/a.ts' })],
    external_symbols: [new scip.SymbolInformation({ symbol: 'symA' })],
  });
  const indexTwo = new scip.Index({
    metadata: new scip.Metadata({ project_root: 'root-two' }),
    documents: [new scip.Document({ relative_path: 'src/b.ts' })],
    external_symbols: [
      new scip.SymbolInformation({ symbol: 'symA' }),
      new scip.SymbolInformation({ symbol: 'symB' }),
    ],
  });
  const onePath = path.join(root, 'one.scip');
  const twoPath = path.join(root, 'two.scip');
  const outputPath = path.join(root, 'out.scip');
  writeIndex(onePath, indexOne);
  writeIndex(twoPath, indexTwo);

  mergeIndexes([onePath, twoPath], outputPath);

  const merged = scip.Index.deserializeBinary(fs.readFileSync(outputPath));
  assert.equal(merged.metadata?.project_root, 'root-one');
  assert.equal(merged.documents.length, 2);
  const symbols = merged.external_symbols.map((symbol) => symbol.symbol).sort();
  assert.deepEqual(symbols, ['symA', 'symB']);
});
