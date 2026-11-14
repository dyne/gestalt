import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { buildCli } from '../scripts/build-cli.mjs';
import { managerHelp, managerVersion } from '../scripts/manager-metadata.mjs';
import { runCli } from '../src/cli/dispatch.ts';
import { commandNames, internalCommandNames } from '../src/cli/contracts.ts';

const root = resolve(import.meta.dirname, '..');
const source = await readFile(join(root, 'public/gestalt'), 'utf8');
const metadata = { version: managerVersion(source), help: managerHelp(source) };

test('shared metadata rejects absent, duplicate and non-strict version identities', () => {
  for (const text of ['', "readonly GESTALT_CLI_VERSION='01.2.3'", source + "\nreadonly GESTALT_CLI_VERSION='1.2.3'\n"])
    assert.throws(() => managerVersion(text), /exactly one strict manager version/);
  assert.throws(() => managerHelp(''), /exactly one manager usage block/);
});

test('compiled Node artifact runs outside the repository without a runtime compiler or source files', async t => {
  const fixture = await mkdtemp(join(tmpdir(), 'native cli consumer '));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const entry = await buildCli(root, join(fixture, 'gestalt.mjs'));
  assert.match(await readFile(entry, 'utf8'), /^#!\/usr\/bin\/env node\n/);
  const run = argv => spawnSync(process.execPath, [entry, ...argv], {
    cwd: fixture, encoding: 'utf8', timeout: 10000,
    env: { ...process.env, PATH: '', NODE_PATH: '', NODE_OPTIONS: '' },
  });
  for (const argv of [[], ['help'], ['--help'], ['-h'], ['help', 'ignored']]) {
    const result = run(argv);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, metadata.help);
    assert.equal(result.stderr, '');
  }
  for (const alias of ['version', '--version', '-V']) {
    const result = run([alias]);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, `gestalt ${metadata.version}\n`);
    assert.equal(result.stderr, '');
  }
  const unknown = run(['not a command']);
  assert.equal(unknown.status, 1);
  assert.equal(unknown.stdout, '');
  assert.equal(unknown.stderr, "gestalt: unknown command: not a command (run 'gestalt help')\n");
  for (const command of [...commandNames.filter(name => !['help', 'version'].includes(name)), ...internalCommandNames]) {
    const result = run([command]);
    assert.equal(result.status, 1, result.stderr);
    assert.equal(result.stdout, '');
    assert.equal(result.stderr, `gestalt: development handler is not implemented: ${command}\n`);
  }
  const retired = run(['xerj', 'mcp']);
  assert.equal(retired.status, 1);
  assert.equal(retired.stdout, '');
  assert.match(retired.stderr, /Xerj has been retired.*User data is retained/);
});

test('dispatch forwards argv, cwd, environment, binary streams and handler exit without shell evaluation', async () => {
  const argv = ['--', 'a b', '', '--literal=$HOME', '"quoted"', 'é'];
  const environment = { RETAINED: 'value', GESTALT_SESSION_TMP_CONTEXT: 'caller-owned', TMPDIR: 'caller-directory' };
  const input = Uint8Array.of(0, 255, 10, 13, 128);
  const out = [], err = [];
  const streams = {
    stdin: (async function* () { yield input; })(),
    stdout: async bytes => { out.push(bytes); },
    stderr: async bytes => { err.push(bytes); },
  };
  const status = await runCli({ argv: ['cli', ...argv], cwd: '/workspace with spaces', environment }, streams, metadata, {
    cli: async (invocation, forwarded) => {
      assert.deepEqual(invocation.argv, argv);
      assert.equal(invocation.cwd, '/workspace with spaces');
      assert.equal(invocation.environment, environment);
      assert.equal(forwarded, streams);
      for await (const bytes of forwarded.stdin) await forwarded.stdout(bytes);
      await forwarded.stderr(Uint8Array.of(255, 0));
      return 23;
    },
  });
  assert.equal(status, 23);
  assert.deepEqual(out, [input]);
  assert.deepEqual(err, [Uint8Array.of(255, 0)]);
});

test('development entry builds and forwards argv and numeric exit status', () => {
  const result = spawnSync(process.execPath, [join(root, 'scripts/dev-cli.mjs'), 'unknown command'], {
    cwd: root, encoding: 'utf8', timeout: 15000,
  });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.equal(result.stderr, "gestalt: unknown command: unknown command (run 'gestalt help')\n");
});
