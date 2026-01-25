import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

let cachedMain: any;

async function loadMain(): Promise<any> {
  if (!cachedMain) {
    cachedMain = await import('../../src/main.js');
  }
  return cachedMain;
}

async function captureError(run: () => Promise<void>): Promise<string> {
  const originalError = console.error;
  const errors: string[] = [];
  console.error = (...args: unknown[]) => {
    errors.push(args.map((arg) => String(arg)).join(' '));
  };
  try {
    await run();
  } finally {
    console.error = originalError;
  }
  return errors.join('\n');
}

test('CLI help lists all commands', async () => {
  const { program } = await loadMain();
  const help = program.helpInformation();
  assert.match(help, /symbols/);
  assert.match(help, /definition/);
  assert.match(help, /references/);
  assert.match(help, /files/);
});

test('symbols help includes key options', async () => {
  const { program } = await loadMain();
  const symbolsCommand = program.commands.find((command: any) => command.name() === 'symbols');
  assert.ok(symbolsCommand, 'symbols command should be registered');
  const help = symbolsCommand.helpInformation();
  assert.match(help, /--language/);
  assert.match(help, /--limit/);
  assert.match(help, /--format/);
});

test('CLI returns non-zero exit code on errors', async () => {
  const { run } = await loadMain();
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-scip-cli-'));
  const originalExitCode = process.exitCode;
  process.exitCode = 0;

  const stderr = await captureError(() =>
    run(['node', 'gestalt-scip', 'symbols', 'Manager', '--scip', tempDir])
  );

  assert.equal(process.exitCode, 1);
  assert.match(stderr, /No \.scip files found/);
  process.exitCode = originalExitCode;
});
