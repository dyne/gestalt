import * as fs from 'fs';
import * as path from 'path';
import { scip } from '../bundle/scip.js';

export function mergeIndexes(inputs: string[], output: string): void {
  if (inputs.length === 0) {
    throw new Error('no input indexes provided');
  }
  if (!output) {
    throw new Error('output path is required');
  }
  const outputAbsolute = path.resolve(output);
  for (const input of inputs) {
    if (path.resolve(input) === outputAbsolute) {
      throw new Error(`output path must differ from input: ${input}`);
    }
  }

  const merged = new scip.Index();
  const documents: Array<InstanceType<typeof scip.Document>> = [];
  const externalSymbols: Array<InstanceType<typeof scip.SymbolInformation>> = [];
  const documentPaths = new Set<string>();
  const symbolIds = new Set<string>();

  for (const input of inputs) {
    const payload = fs.readFileSync(input);
    const index = scip.Index.deserializeBinary(payload);

    if (!merged.metadata && index.metadata) {
      merged.metadata = index.metadata;
    }

    for (const doc of index.documents) {
      const relativePath = doc.relative_path;
      if (!relativePath) {
        throw new Error(`document with empty relative path in ${input}`);
      }
      if (documentPaths.has(relativePath)) {
        throw new Error(`duplicate document path ${relativePath} in ${input}`);
      }
      documentPaths.add(relativePath);
      documents.push(doc);
    }

    for (const symbol of index.external_symbols) {
      const symbolId = symbol.symbol;
      if (!symbolId) {
        continue;
      }
      if (symbolIds.has(symbolId)) {
        continue;
      }
      symbolIds.add(symbolId);
      externalSymbols.push(symbol);
    }
  }

  merged.documents = documents;
  merged.external_symbols = externalSymbols;

  fs.mkdirSync(path.dirname(output), { recursive: true });
  const payload = Buffer.from(merged.serializeBinary());
  fs.writeFileSync(output, payload, { mode: 0o644 });
}
