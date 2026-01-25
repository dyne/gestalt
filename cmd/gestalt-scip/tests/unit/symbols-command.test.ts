import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

type SymbolsCommand = (query: string, options: Record<string, unknown>) => Promise<void>;

let cachedCommand: SymbolsCommand | undefined;

async function loadSymbolsCommand(): Promise<SymbolsCommand> {
  if (!cachedCommand) {
    const module = await import('../../src/commands/symbols.js');
    cachedCommand = module.symbolsCommand;
  }
  return cachedCommand;
}

function createTempRepo(): { root: string; scipDir: string } {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-symbols-'));
  const scipDir = path.join(root, '.gestalt', 'scip');
  fs.mkdirSync(scipDir, { recursive: true });
  return { root, scipDir };
}

function writeIndex(filePath: string, documents: unknown[]): void {
  const index = { documents };
  fs.writeFileSync(filePath, JSON.stringify(index), 'utf-8');
}

function makeDocument(
  relativePath: string,
  language: string,
  symbol: string,
  kind: number,
  documentation: string,
  line: number
): unknown {
  return {
    relativePath,
    language,
    symbols: [
      {
        symbol,
        documentation: [documentation],
        kind,
      },
    ],
    occurrences: [
      {
        symbol,
        symbolRoles: 1,
        range: [line, 0, line, 6],
      },
    ],
  };
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

test('symbolsCommand merges languages by default and supports JSON output', async () => {
  const symbolsCommand = await loadSymbolsCommand();
  const { scipDir } = createTempRepo();

  const goSymbolOne = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const goSymbolTwo = 'scip-go gomod example v1 `internal/api`/Manager#';
  const goSymbolThree = 'scip-go gomod example v1 `internal/runtime`/Manager#';
  const tsSymbol = 'scip-typescript npm example 1.0.0 src/`manager.ts`/Manager#';

  writeIndex(path.join(scipDir, 'index-go.scip'), [
    makeDocument('internal/terminal/manager.go', 'go', goSymbolOne, 49, '```go\ntype Manager struct\n```', 4),
    makeDocument('internal/api/manager.go', 'go', goSymbolTwo, 49, '```go\ntype Manager struct\n```', 8),
    makeDocument('internal/runtime/manager.go', 'go', goSymbolThree, 49, '```go\ntype Manager struct\n```', 12),
  ]);

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [
    makeDocument('src/manager.ts', 'typescript', tsSymbol, 7, '```ts\nclass Manager {}\n```', 2),
  ]);

  const output = await captureOutput(() =>
    symbolsCommand('Manager', { scip: scipDir, format: 'json' })
  );

  const payload = JSON.parse(output);
  assert.equal(payload.query, 'Manager');
  assert.equal(payload.symbols.length, 4);
  const languages = new Set(payload.symbols.map((symbol: any) => symbol.language));
  assert.deepEqual(Array.from(languages).sort(), ['go', 'typescript']);
});

test('symbolsCommand filters by language and applies limits', async () => {
  const symbolsCommand = await loadSymbolsCommand();
  const { scipDir } = createTempRepo();

  const goSymbolOne = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const goSymbolTwo = 'scip-go gomod example v1 `internal/api`/Manager#';
  const goSymbolThree = 'scip-go gomod example v1 `internal/runtime`/Manager#';
  const tsSymbol = 'scip-typescript npm example 1.0.0 src/`manager.ts`/Manager#';

  writeIndex(path.join(scipDir, 'index-go.scip'), [
    makeDocument('internal/terminal/manager.go', 'go', goSymbolOne, 49, '```go\ntype Manager struct\n```', 4),
    makeDocument('internal/api/manager.go', 'go', goSymbolTwo, 49, '```go\ntype Manager struct\n```', 8),
    makeDocument('internal/runtime/manager.go', 'go', goSymbolThree, 49, '```go\ntype Manager struct\n```', 12),
  ]);

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [
    makeDocument('src/manager.ts', 'typescript', tsSymbol, 7, '```ts\nclass Manager {}\n```', 2),
  ]);

  const goOnlyOutput = await captureOutput(() =>
    symbolsCommand('Manager', { scip: scipDir, language: 'go', format: 'json' })
  );
  const goOnlyPayload = JSON.parse(goOnlyOutput);
  assert.equal(goOnlyPayload.symbols.length, 3);
  assert.deepEqual(
    Array.from(new Set(goOnlyPayload.symbols.map((symbol: any) => symbol.language))),
    ['go']
  );

  const limitedOutput = await captureOutput(() =>
    symbolsCommand('Manager', { scip: scipDir, limit: 2, format: 'json' })
  );
  const limitedPayload = JSON.parse(limitedOutput);
  assert.equal(limitedPayload.symbols.length, 2);
});

test('symbolsCommand provides readable text output', async () => {
  const symbolsCommand = await loadSymbolsCommand();
  const { scipDir } = createTempRepo();

  const goSymbol = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  writeIndex(path.join(scipDir, 'index-go.scip'), [
    makeDocument('internal/terminal/manager.go', 'go', goSymbol, 49, '```go\ntype Manager struct\n```', 4),
  ]);

  const output = await captureOutput(() =>
    symbolsCommand('Manager', { scip: scipDir, format: 'text' })
  );

  assert.match(output, /internal\/terminal\/manager.go:5/);
  assert.match(output, /type Manager struct/);
});
