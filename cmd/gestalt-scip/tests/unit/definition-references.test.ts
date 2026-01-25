import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { decodeSymbolId } from '../../src/symbol-id-codec.js';

type DefinitionCommand = (symbolId: string, options: Record<string, unknown>) => Promise<void>;
type ReferencesCommand = (symbolId: string, options: Record<string, unknown>) => Promise<void>;
type SymbolsCommand = (query: string, options: Record<string, unknown>) => Promise<void>;

let cachedDefinition: DefinitionCommand | undefined;
let cachedReferences: ReferencesCommand | undefined;
let cachedSymbols: SymbolsCommand | undefined;

async function loadDefinitionCommand(): Promise<DefinitionCommand> {
  if (!cachedDefinition) {
    const module = await import('../../src/commands/definition.js');
    cachedDefinition = module.definitionCommand;
  }
  return cachedDefinition;
}

async function loadReferencesCommand(): Promise<ReferencesCommand> {
  if (!cachedReferences) {
    const module = await import('../../src/commands/references.js');
    cachedReferences = module.referencesCommand;
  }
  return cachedReferences;
}

async function loadSymbolsCommand(): Promise<SymbolsCommand> {
  if (!cachedSymbols) {
    const module = await import('../../src/commands/symbols.js');
    cachedSymbols = module.symbolsCommand;
  }
  return cachedSymbols;
}

function createTempRepo(): { root: string; scipDir: string } {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-defref-'));
  const scipDir = path.join(root, '.gestalt', 'scip');
  fs.mkdirSync(scipDir, { recursive: true });
  return { root, scipDir };
}

function writeIndex(filePath: string, documents: unknown[]): void {
  fs.writeFileSync(filePath, JSON.stringify({ documents }), 'utf-8');
}

async function captureOutput(run: () => Promise<void>): Promise<string> {
  const originalLog = console.log;
  const logs: string[] = [];
  console.log = (...args: unknown[]) => {
    logs.push(args.map((arg) => String(arg)).join(' '));
  };
  try {
    await run();
  } finally {
    console.log = originalLog;
  }
  return logs.join('\n');
}

test('definitionCommand returns symbol metadata for a definition', async () => {
  const definitionCommand = await loadDefinitionCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [
      { symbol: symbolId, symbolRoles: 1, range: [3, 0, 3, 7] },
      { symbol: symbolId, symbolRoles: 0, range: [10, 2, 10, 9] },
      { symbol: symbolId, symbolRoles: 0, range: [20, 4, 20, 11] },
    ],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const output = await captureOutput(() =>
    definitionCommand(symbolId, { scip: scipDir, format: 'json' })
  );
  const payload = JSON.parse(output);

  assert.equal(decodeSymbolId(payload.id), symbolId);
  assert.equal(payload.name, 'Manager');
  assert.equal(payload.kind, 'Struct');
  assert.equal(payload.file_path, 'internal/terminal/manager.go');
  assert.equal(payload.line, 3);
});

test('referencesCommand returns references and excludes definitions', async () => {
  const referencesCommand = await loadReferencesCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [
      { symbol: symbolId, symbolRoles: 1, range: [3, 0, 3, 7] },
      { symbol: symbolId, symbolRoles: 0, range: [10, 2, 10, 9] },
      { symbol: symbolId, symbolRoles: 0, range: [20, 4, 20, 11] },
    ],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const output = await captureOutput(() =>
    referencesCommand(symbolId, { scip: scipDir, format: 'json' })
  );
  const payload = JSON.parse(output);

  assert.equal(decodeSymbolId(payload.symbol), symbolId);
  assert.equal(payload.references.length, 2);
  assert.deepEqual(
    payload.references.map((reference: any) => reference.line),
    [10, 20]
  );
  assert.ok(payload.references.every((reference: any) => decodeSymbolId(reference.symbol) === symbolId));
});

test('definitionCommand supports TOON output format', async () => {
  const definitionCommand = await loadDefinitionCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [{ symbol: symbolId, symbolRoles: 1, range: [3, 0, 3, 7] }],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const output = await captureOutput(() =>
    definitionCommand(symbolId, { scip: scipDir, format: 'toon' })
  );

  assert.match(output, /name: Manager/);
  assert.match(output, /file_path: internal\/terminal\/manager.go/);
});

test('referencesCommand supports TOON output format', async () => {
  const referencesCommand = await loadReferencesCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [
      { symbol: symbolId, symbolRoles: 1, range: [3, 0, 3, 7] },
      { symbol: symbolId, symbolRoles: 0, range: [10, 2, 10, 9] },
    ],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const output = await captureOutput(() =>
    referencesCommand(symbolId, { scip: scipDir, format: 'toon' })
  );

  assert.match(output, /symbol:/);
  assert.match(output, /references\[/);
});

test('symbols output IDs are shell-safe and work with definition and references', async () => {
  const symbolsCommand = await loadSymbolsCommand();
  const definitionCommand = await loadDefinitionCommand();
  const referencesCommand = await loadReferencesCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [
      { symbol: symbolId, symbolRoles: 1, range: [3, 0, 3, 7] },
      { symbol: symbolId, symbolRoles: 0, range: [10, 2, 10, 9] },
    ],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const symbolsOutput = await captureOutput(() =>
    symbolsCommand('Manager', { scip: scipDir, format: 'json' })
  );
  const symbolsPayload = JSON.parse(symbolsOutput);
  const encodedId = symbolsPayload.symbols[0].id;

  assert.ok(!encodedId.includes(' '));
  assert.ok(!encodedId.includes('`'));
  assert.equal(decodeSymbolId(encodedId), symbolId);

  const definitionOutput = await captureOutput(() =>
    definitionCommand(encodedId, { scip: scipDir, format: 'json' })
  );
  const definitionPayload = JSON.parse(definitionOutput);
  assert.equal(decodeSymbolId(definitionPayload.id), symbolId);

  const referencesOutput = await captureOutput(() =>
    referencesCommand(encodedId, { scip: scipDir, format: 'json' })
  );
  const referencesPayload = JSON.parse(referencesOutput);
  assert.equal(decodeSymbolId(referencesPayload.symbol), symbolId);
});

test('definitionCommand reports missing symbols clearly', async () => {
  const definitionCommand = await loadDefinitionCommand();
  const { scipDir } = createTempRepo();
  writeIndex(path.join(scipDir, 'index-go.scip'), []);

  await assert.rejects(
    () => definitionCommand('missing', { scip: scipDir, format: 'json' }),
    /Symbol not found/
  );
});
