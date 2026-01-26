import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

type SearchCommand = (pattern: string, options: Record<string, unknown>) => Promise<void>;

let cachedCommand: SearchCommand | undefined;

async function loadSearchCommand(): Promise<SearchCommand> {
  if (!cachedCommand) {
    const module = await import('../src/commands/search.js');
    cachedCommand = module.searchCommand;
  }
  return cachedCommand;
}

function createTempRepo(): { scipDir: string } {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-search-'));
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

test('searchCommand rejects empty pattern', async () => {
  const searchCommand = await loadSearchCommand();
  await assert.rejects(() => searchCommand('   ', {}), /must not be empty/);
});

test('searchCommand finds matches case-insensitively by default', async () => {
  const searchCommand = await loadSearchCommand();
  const { scipDir } = createTempRepo();

  const document = {
    relativePath: 'src/app.ts',
    language: 'typescript',
    text: ['const Value = 1;', 'const other = 2;', 'Value++;'].join('\n'),
  };

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [document]);

  const output = await captureOutput(() =>
    searchCommand('value', { scip: scipDir, format: 'json' })
  );
  const payload = JSON.parse(output);

  assert.equal(payload.pattern, 'value');
  assert.equal(payload.matches.length, 2);
  assert.equal(payload.matches[0].file_path, 'src/app.ts');
  assert.equal(payload.matches[0].line, 1);
  assert.equal(payload.matches[0].column, 7);
});
