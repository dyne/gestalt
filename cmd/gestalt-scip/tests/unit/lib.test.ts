import test from 'node:test';
import assert from 'node:assert/strict';
import os from 'node:os';
import path from 'node:path';
import fs from 'node:fs';
import { pathToFileURL } from 'node:url';

process.env.SCIP_MODULE_DIR = path.join(process.cwd(), 'dist/src/lib');
process.env.SCIP_PROTO_PATH = path.join(process.cwd(), 'dist/bundle/scip.proto');

let cachedLib: any;

async function loadLib(): Promise<any> {
  if (!cachedLib) {
    cachedLib = await import('../../src/lib/index.js');
  }
  return cachedLib;
}

const definitionRole = 0x1;
const referenceRole = 0x2;

test('SymbolParser handles go-style symbols', async () => {
  const { SymbolParser } = await loadLib();
  const parser = new SymbolParser();
  const symbol =
    'scip-go gomod github.com/example/project v1.0.0 internal/terminal/`manager.go`/Manager#';

  const parsed = parser.parse(symbol);

  assert.equal(parsed.packageName, 'github.com/example/project');
  assert.equal(parsed.filePath, 'internal/terminal/manager.go');
  assert.equal(parsed.displayName, 'Manager');
});

test('buildSymbolIndex falls back to document path when symbol has no file path', async () => {
  const { buildSymbolIndex } = await loadLib();
  const symbol = 'scip-go gomod github.com/example/project v1.0.0 Manager#';
  const scipIndex = {
    documents: [
      {
        relativePath: 'internal/terminal/manager.go',
        occurrences: [
          {
            symbol,
            range: [10, 2, 10, 9],
            symbolRoles: definitionRole,
          },
        ],
      },
    ],
  };

  const index = buildSymbolIndex(scipIndex);
  const keys = Array.from(index.keys());

  assert.equal(keys.length, 1);
  assert.equal(keys[0], 'github.com/example/project:internal/terminal/manager.go:Manager');
});

test('QueryEngine finds definitions and references by name', async () => {
  const { QueryEngine, buildSymbolIndex } = await loadLib();
  const symbol =
    'scip-go gomod github.com/example/project v1.0.0 internal/terminal/`manager.go`/Manager#';
  const scipIndex = {
    documents: [
      {
        relativePath: 'internal/terminal/manager.go',
        occurrences: [
          {
            symbol,
            range: [5, 0, 5, 7],
            symbolRoles: definitionRole,
          },
          {
            symbol,
            range: [25, 10, 25, 17],
            symbolRoles: referenceRole,
          },
        ],
      },
    ],
  };

  const index = buildSymbolIndex(scipIndex);
  const engine = new QueryEngine(index);
  const results = engine.find('Manager');

  assert.equal(results.length, 2);
  assert.equal(results.filter((result: any) => result.isDefinition).length, 1);
});

test('QueryEngine searchContent finds matches with context', async () => {
  const { QueryEngine } = await loadLib();
  const engine = new QueryEngine(new Map());
  const index = {
    documents: [
      {
        relativePath: 'src/app.ts',
        language: 'typescript',
        text: ['const value = 1;', 'const other = value + 1;', 'export {};'].join('\n'),
      },
    ],
  };

  const results = engine.searchContent('value', { indexes: [index], contextLines: 1 });

  assert.equal(results.length, 2);
  assert.equal(results[0].file_path, 'src/app.ts');
  assert.equal(results[0].line, 1);
  assert.equal(results[0].column, 7);
  assert.deepEqual(results[0].context_before, []);
  assert.deepEqual(results[0].context_after, ['const other = value + 1;']);
});

test('QueryEngine searchContent throws on invalid regex', async () => {
  const { QueryEngine } = await loadLib();
  const engine = new QueryEngine(new Map());
  const index = {
    documents: [
      {
        relativePath: 'src/app.ts',
        language: 'typescript',
        text: 'const value = 1;',
      },
    ],
  };

  assert.throws(
    () => engine.searchContent('[invalid', { indexes: [index], contextLines: 1 }),
    /Invalid regex pattern/
  );
});

test('QueryEngine searchContent falls back to file content when text is missing', async () => {
  const { QueryEngine } = await loadLib();
  const engine = new QueryEngine(new Map());
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-search-'));
  const filePath = path.join(root, 'src', 'app.ts');
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, 'const Manager = 1;\n', 'utf-8');

  const index = {
    metadata: { projectRoot: pathToFileURL(root).toString() },
    documents: [
      {
        relativePath: 'src/app.ts',
        language: 'typescript',
      },
    ],
  };

  const results = engine.searchContent('manager', { indexes: [index], contextLines: 0 });

  assert.equal(results.length, 1);
  assert.equal(results[0].file_path, 'src/app.ts');
  assert.equal(results[0].line, 1);
});

test('QueryEngine searchContent stops after reaching the limit', async () => {
  const { QueryEngine } = await loadLib();
  const engine = new QueryEngine(new Map());

  const docWithMatch = {
    relativePath: 'src/app.ts',
    language: 'typescript',
    text: 'const match = 1;',
  };
  const docShouldNotRead: any = {
    relativePath: 'src/skip.ts',
    language: 'typescript',
  };
  Object.defineProperty(docShouldNotRead, 'text', {
    get() {
      throw new Error('should not read');
    },
  });

  const results = engine.searchContent('match', {
    indexes: [{ documents: [docWithMatch, docShouldNotRead] }],
    contextLines: 0,
    limit: 1,
  });

  assert.equal(results.length, 1);
  assert.equal(results[0].file_path, 'src/app.ts');
});

test('QueryEngine searchContent caches file reads for repeated documents', async () => {
  const { QueryEngine } = await loadLib();
  const engine = new QueryEngine(new Map());
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-search-cache-'));
  const filePath = path.join(root, 'src', 'app.ts');
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, 'const match = 1;\n', 'utf-8');

  const index = {
    metadata: { projectRoot: pathToFileURL(root).toString() },
    documents: [
      { relativePath: 'src/app.ts', language: 'typescript' },
      { relativePath: 'src/app.ts', language: 'typescript' },
    ],
  };

  const mutableFs = fs as unknown as { readFileSync: (...args: any[]) => any };
  const originalRead = mutableFs.readFileSync;
  let readCount = 0;
  mutableFs.readFileSync = (...args: any[]): any => {
    readCount += 1;
    return originalRead(...args);
  };

  try {
    const results = engine.searchContent('match', { indexes: [index], contextLines: 0 });
    assert.equal(results.length, 2);
  } finally {
    mutableFs.readFileSync = originalRead;
  }

  assert.equal(readCount, 1);
});

test('loadScipIndex reads JSON fixtures without protobuf parsing', async () => {
  const { loadScipIndex } = await loadLib();
  const tempPath = path.join(process.cwd(), 'dist/tests/unit/temp-index.json');
  const fixture = {
    documents: [
      {
        relativePath: 'internal/api/routes.go',
        occurrences: [],
      },
    ],
  };

  fs.mkdirSync(path.dirname(tempPath), { recursive: true });
  fs.writeFileSync(tempPath, JSON.stringify(fixture), 'utf-8');

  const loaded = loadScipIndex(tempPath);
  assert.equal(loaded.documents?.[0]?.relativePath, 'internal/api/routes.go');
});
