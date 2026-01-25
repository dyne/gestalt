import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { run } from '../../src/main.js';
import { scip as scipProto } from '../../src/bundle/scip.js';

type RepoFixture = {
  root: string;
  scipDir: string;
  symbolId: string;
  filePath: string;
  documentText: string;
};

type RunResult = {
  stdout: string;
  stderr: string;
  exitCode: number;
};

function createRepoFixture(): RepoFixture {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-integration-'));
  const scipDir = path.join(root, '.gestalt', 'scip');
  fs.mkdirSync(scipDir, { recursive: true });

  const filePath = 'internal/terminal/manager.go';
  const documentText = [
    'package terminal',
    '',
    'type Manager struct{}',
    '',
    'func NewManager() *Manager {',
    '  return &Manager{}',
    '}',
    '',
    'func (m *Manager) Start() {}',
  ].join('\n');

  const absoluteFilePath = path.join(root, filePath);
  fs.mkdirSync(path.dirname(absoluteFilePath), { recursive: true });
  fs.writeFileSync(absoluteFilePath, documentText, 'utf-8');

  const symbolId = 'scip-go gomod gestalt v0 `internal/terminal`/Manager#';
  writeProtobufIndex(scipDir, filePath, symbolId, documentText);

  return { root, scipDir, symbolId, filePath, documentText };
}

function writeProtobufIndex(scipDir: string, filePath: string, symbolId: string, documentText: string): void {
  const definitionRole = scipProto.SymbolRole.Definition;
  const occurrences = [
    scipProto.Occurrence.fromObject({ symbol: symbolId, symbol_roles: definitionRole, range: [2, 5, 2, 12] }),
    scipProto.Occurrence.fromObject({ symbol: symbolId, symbol_roles: 0, range: [4, 18, 4, 25] }),
    scipProto.Occurrence.fromObject({ symbol: symbolId, symbol_roles: 0, range: [5, 10, 5, 17] }),
    scipProto.Occurrence.fromObject({ symbol: symbolId, symbol_roles: 0, range: [8, 10, 8, 17] }),
  ];

  const symbols = [
    scipProto.SymbolInformation.fromObject({
      symbol: symbolId,
      kind: scipProto.SymbolInformation.Kind.Struct,
      documentation: ['```go\ntype Manager struct{}\n```'],
    }),
  ];

  const document = scipProto.Document.fromObject({
    language: 'go',
    relative_path: filePath,
    text: documentText,
    occurrences,
    symbols,
  });

  const index = scipProto.Index.fromObject({ documents: [document] });
  const binary = Buffer.from(index.serializeBinary());
  fs.writeFileSync(path.join(scipDir, 'index-go.scip'), binary);
}

async function captureOutput(runCommand: () => Promise<void>): Promise<{ stdout: string; stderr: string }> {
  const originalLog = console.log;
  const originalError = console.error;
  const logs: string[] = [];
  const errors: string[] = [];

  console.log = (...args: unknown[]) => {
    logs.push(args.map((arg) => String(arg)).join(' '));
  };
  console.error = (...args: unknown[]) => {
    errors.push(args.map((arg) => String(arg)).join(' '));
  };

  try {
    await runCommand();
  } finally {
    console.log = originalLog;
    console.error = originalError;
  }

  return {
    stdout: logs.join('\n'),
    stderr: errors.join('\n'),
  };
}

async function runInRepo(root: string, args: string[]): Promise<RunResult> {
  const originalCwd = process.cwd();
  const originalExitCode = process.exitCode;

  process.chdir(root);
  process.exitCode = 0;

  try {
    const { stdout, stderr } = await captureOutput(() => run(['node', 'gestalt-scip', ...args]));
    const exitCode = process.exitCode ?? 0;
    return { stdout, stderr, exitCode };
  } finally {
    process.chdir(originalCwd);
    process.exitCode = originalExitCode;
  }
}

test('symbols command auto-discovers protobuf SCIP indexes', async () => {
  const fixture = createRepoFixture();
  const result = await runInRepo(fixture.root, ['symbols', 'Manager', '--format', 'json']);

  assert.equal(result.exitCode, 0);
  assert.equal(result.stderr, '');

  const payload = JSON.parse(result.stdout);
  assert.equal(payload.query, 'Manager');
  assert.equal(payload.symbols.length, 1);
  assert.equal(payload.symbols[0].id, fixture.symbolId);
  assert.equal(payload.symbols[0].kind, 'Struct');
  assert.equal(payload.symbols[0].language, 'go');
  assert.equal(payload.symbols[0].file_path, fixture.filePath);
  assert.equal(payload.symbols[0].line, 2);
});

test('definition and references commands return protobuf-backed results', async () => {
  const fixture = createRepoFixture();

  const definitionResult = await runInRepo(fixture.root, [
    'definition',
    fixture.symbolId,
    '--format',
    'json',
  ]);
  assert.equal(definitionResult.exitCode, 0);
  const definitionPayload = JSON.parse(definitionResult.stdout);
  assert.equal(definitionPayload.id, fixture.symbolId);
  assert.equal(definitionPayload.file_path, fixture.filePath);
  assert.equal(definitionPayload.line, 2);

  const referencesResult = await runInRepo(fixture.root, [
    'references',
    fixture.symbolId,
    '--format',
    'json',
  ]);
  assert.equal(referencesResult.exitCode, 0);
  const referencesPayload = JSON.parse(referencesResult.stdout);
  assert.equal(referencesPayload.symbol, fixture.symbolId);
  assert.deepEqual(
    referencesPayload.references.map((reference: any) => reference.line),
    [4, 5, 8]
  );
});

test('files command returns document text and symbol occurrences', async () => {
  const fixture = createRepoFixture();
  const result = await runInRepo(fixture.root, [
    'files',
    fixture.filePath,
    '--symbols',
    '--format',
    'json',
  ]);

  assert.equal(result.exitCode, 0);

  const payload = JSON.parse(result.stdout);
  assert.equal(payload.path, fixture.filePath);
  assert.equal(payload.content, fixture.documentText);
  assert.equal(payload.symbols.length, 1);
  assert.equal(payload.occurrences.length, 4);

  const kinds = new Set(payload.occurrences.map((occurrence: any) => occurrence.kind));
  assert.ok(kinds.has('definition'));
  assert.ok(kinds.has('reference'));
});
