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

test('searchCommand respects case-sensitive flag', async () => {
  const searchCommand = await loadSearchCommand();
  const { scipDir } = createTempRepo();

  const document = {
    relativePath: 'src/app.ts',
    language: 'typescript',
    text: 'const Value = 1;',
  };

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [document]);

  const output = await captureOutput(() =>
    searchCommand('value', { scip: scipDir, format: 'json', caseSensitive: true })
  );
  const payload = JSON.parse(output);

  assert.equal(payload.matches.length, 0);
});

test('searchCommand supports OR clauses and regex patterns', async () => {
  const searchCommand = await loadSearchCommand();
  const { scipDir } = createTempRepo();

  const document = {
    relativePath: 'src/app.ts',
    language: 'typescript',
    text: ['handleScipReindex', 'warning: be careful', 'all good'].join('\n'),
  };

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [document]);

  const orOutput = await captureOutput(() =>
    searchCommand('error|warning|fail', { scip: scipDir, format: 'json' })
  );
  const orPayload = JSON.parse(orOutput);
  assert.equal(orPayload.matches.length, 1);

  const regexOutput = await captureOutput(() =>
    searchCommand('handle.*Reindex', { scip: scipDir, format: 'json' })
  );
  const regexPayload = JSON.parse(regexOutput);
  assert.equal(regexPayload.matches.length, 1);
});

test('searchCommand filters by language and applies limits', async () => {
  const searchCommand = await loadSearchCommand();
  const { scipDir } = createTempRepo();

  const tsDocument = {
    relativePath: 'src/app.ts',
    language: 'typescript',
    text: 'const match = 1;\nconst matchAgain = 2;',
  };
  const goDocument = {
    relativePath: 'internal/app.go',
    language: 'go',
    text: 'var match = 1',
  };

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [tsDocument]);
  writeIndex(path.join(scipDir, 'index-go.scip'), [goDocument]);

  const goOnlyOutput = await captureOutput(() =>
    searchCommand('match', { scip: scipDir, format: 'json', language: 'go' })
  );
  const goOnlyPayload = JSON.parse(goOnlyOutput);
  assert.equal(goOnlyPayload.matches.length, 1);
  assert.equal(goOnlyPayload.matches[0].file_path, 'internal/app.go');

  const limitedOutput = await captureOutput(() =>
    searchCommand('match', { scip: scipDir, format: 'json', limit: 1 })
  );
  const limitedPayload = JSON.parse(limitedOutput);
  assert.equal(limitedPayload.matches.length, 1);
});

test('searchCommand supports text output with context', async () => {
  const searchCommand = await loadSearchCommand();
  const { scipDir } = createTempRepo();

  const document = {
    relativePath: 'src/app.ts',
    language: 'typescript',
    text: ['before', 'FindMe here', 'after'].join('\n'),
  };

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [document]);

  const output = await captureOutput(() =>
    searchCommand('findme', { scip: scipDir, format: 'text', context: 1 })
  );

  assert.match(output, /pattern: findme/);
  assert.match(output, /src\/app.ts:2:1/);
  assert.match(output, /1: before/);
  assert.match(output, /2: FindMe here/);
  assert.match(output, /3: after/);
});

test('searchCommand supports TOON output format', async () => {
  const searchCommand = await loadSearchCommand();
  const { scipDir } = createTempRepo();

  const document = {
    relativePath: 'src/app.ts',
    language: 'typescript',
    text: 'const match = 1;',
  };

  writeIndex(path.join(scipDir, 'index-typescript.scip'), [document]);

  const output = await captureOutput(() =>
    searchCommand('match', { scip: scipDir, format: 'toon' })
  );

  assert.match(output, /pattern: match/);
  assert.match(output, /matches\[/);
});

test('searchCommand rejects invalid regex patterns', async () => {
  const searchCommand = await loadSearchCommand();
  const { scipDir } = createTempRepo();
  writeIndex(path.join(scipDir, 'index-typescript.scip'), []);

  await assert.rejects(
    () => searchCommand('[invalid', { scip: scipDir }),
    /Invalid regex pattern/
  );
});
