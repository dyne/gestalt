import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

type FilesCommand = (filePath: string, options: Record<string, unknown>) => Promise<void>;

let cachedCommand: FilesCommand | undefined;

async function loadFilesCommand(): Promise<FilesCommand> {
  if (!cachedCommand) {
    const module = await import('../../src/commands/files.js');
    cachedCommand = module.filesCommand;
  }
  return cachedCommand;
}

function createTempRepo(): { scipDir: string } {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-files-'));
  const scipDir = path.join(root, '.gestalt', 'scip');
  fs.mkdirSync(scipDir, { recursive: true });
  return { scipDir };
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

test('filesCommand returns file content and symbols in JSON format', async () => {
  const filesCommand = await loadFilesCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    text: 'package terminal\n\ntype Manager struct{}',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [
      { symbol: symbolId, symbolRoles: 1, range: [2, 0, 2, 7] },
      { symbol: symbolId, symbolRoles: 0, range: [4, 2, 4, 9] },
    ],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const output = await captureOutput(() =>
    filesCommand('internal/terminal/manager.go', { scip: scipDir, format: 'json' })
  );
  const payload = JSON.parse(output);

  assert.equal(payload.path, 'internal/terminal/manager.go');
  assert.equal(payload.content, 'package terminal\n\ntype Manager struct{}');
  assert.equal(payload.symbols.length, 1);
  assert.equal(payload.occurrences, undefined);
});

test('filesCommand includes occurrences when --symbols is set', async () => {
  const filesCommand = await loadFilesCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    text: 'package terminal\n\ntype Manager struct{}',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [
      { symbol: symbolId, symbolRoles: 1, range: [2, 0, 2, 7] },
      { symbol: symbolId, symbolRoles: 0, range: [4, 2, 4, 9] },
    ],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const output = await captureOutput(() =>
    filesCommand('internal/terminal/manager.go', { scip: scipDir, format: 'json', symbols: true })
  );
  const payload = JSON.parse(output);

  assert.equal(payload.occurrences.length, 2);
  assert.ok(payload.occurrences.some((occurrence: any) => occurrence.kind === 'definition'));
});

test('filesCommand supports TOON output format', async () => {
  const filesCommand = await loadFilesCommand();
  const { scipDir } = createTempRepo();

  const symbolId = 'scip-go gomod example v1 `internal/terminal`/Manager#';
  const document = {
    relativePath: 'internal/terminal/manager.go',
    language: 'go',
    text: 'package terminal\n\ntype Manager struct{}',
    symbols: [
      {
        symbol: symbolId,
        documentation: ['```go\ntype Manager struct\n```'],
        kind: 49,
      },
    ],
    occurrences: [{ symbol: symbolId, symbolRoles: 1, range: [2, 0, 2, 7] }],
  };

  writeIndex(path.join(scipDir, 'index-go.scip'), [document]);

  const output = await captureOutput(() =>
    filesCommand('internal/terminal/manager.go', { scip: scipDir, format: 'toon' })
  );

  assert.match(output, /path: internal\/terminal\/manager.go/);
  assert.match(output, /symbols\[/);
});

test('filesCommand reports missing files clearly', async () => {
  const filesCommand = await loadFilesCommand();
  const { scipDir } = createTempRepo();
  writeIndex(path.join(scipDir, 'index-go.scip'), []);

  await assert.rejects(
    () => filesCommand('internal/terminal/missing.go', { scip: scipDir, format: 'json' }),
    /File not found/
  );
});
