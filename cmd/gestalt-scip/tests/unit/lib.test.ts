import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import fs from 'node:fs';

process.env.SCIP_MODULE_DIR = path.join(process.cwd(), 'dist/src/lib');
process.env.SCIP_PROTO_PATH = path.join(process.cwd(), 'dist/bundle/scip.proto');

const { QueryEngine, SymbolParser, buildSymbolIndex, loadScipIndex } = await import(
  '../../src/lib/index.js'
);

const definitionRole = 0x1;
const referenceRole = 0x2;

test('SymbolParser handles go-style symbols', () => {
  const parser = new SymbolParser();
  const symbol =
    'scip-go gomod github.com/example/project v1.0.0 internal/terminal/`manager.go`/Manager#';

  const parsed = parser.parse(symbol);

  assert.equal(parsed.packageName, 'github.com/example/project');
  assert.equal(parsed.filePath, 'internal/terminal/manager.go');
  assert.equal(parsed.displayName, 'Manager');
});

test('buildSymbolIndex falls back to document path when symbol has no file path', () => {
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

test('QueryEngine finds definitions and references by name', () => {
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
  assert.equal(results.filter((result) => result.isDefinition).length, 1);
});

test('loadScipIndex reads JSON fixtures without protobuf parsing', () => {
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
