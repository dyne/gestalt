import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { buildCli } from '../scripts/build-cli.mjs';
import { managerHelp, managerVersion } from '../scripts/manager-metadata.mjs';
import { runCli } from '../src/cli/dispatch.ts';
import { commandNames, internalCommandNames } from '../src/cli/contracts.ts';
import { createRuntimeHandlers } from '../src/cli/runtime/index.ts';

const root = resolve(import.meta.dirname, '..');
const source = await readFile(join(root, 'public/gestalt'), 'utf8');
const metadata = { version: managerVersion(source), help: managerHelp(source) };

test('shared metadata rejects absent, duplicate and non-strict version identities', () => {
  for (const text of ['', "readonly GESTALT_CLI_VERSION='01.2.3'", source + "\nreadonly GESTALT_CLI_VERSION='1.2.3'\n"])
    assert.throws(() => managerVersion(text), /exactly one strict manager version/);
  assert.throws(() => managerHelp(''), /exactly one manager usage block/);
});

test('compiled Windows Node artifact runs without Bash, a runtime compiler or source files', { skip: process.platform !== 'win32' ? 'Unix direct entry uses the packaged compatibility backend' : false }, async t => {
  const fixture = await mkdtemp(join(tmpdir(), 'native cli consumer '));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const entry = await buildCli(root, join(fixture, 'gestalt.mjs'));
  assert.match(await readFile(entry, 'utf8'), /^#!\/usr\/bin\/env node\n/);
  const run = argv => spawnSync(process.execPath, [entry, ...argv], {
    cwd: fixture, encoding: 'utf8', timeout: 10000,
    env: { ...process.env, HOME: fixture, USERPROFILE: fixture, CODEX_HOME: join(fixture, 'isolated Codex'),
      GESTALT_HOME: join(fixture, 'isolated Gestalt'), GESTALT_MOBILE_BIN: '', PATH: '', NODE_PATH: '', NODE_OPTIONS: '' },
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
    assert.equal(result.stdout, `gestalt ${metadata.version}\n` + ['Serena', 'Playwright', 'Playwright readiness', 'Gestalt Mobile'].map(label => label.padEnd(24) + 'DEFERRED (native optional feature)\n').join('') +
      ['uv (Serena)', 'Python (Serena)'].map(label => label.padEnd(24) + 'NOT REQUIRED (native Serena deferred)\n').join(''));
    assert.equal(result.stderr, '');
  }
  const unknown = run(['not a command']);
  assert.equal(unknown.status, 1);
  assert.equal(unknown.stdout, '');
  assert.equal(unknown.stderr, "gestalt: unknown command: not a command (run 'gestalt help')\n");
  // Component bodies have their own later native milestones. Core dispatch is
  // exercised by isolated runtime/transfer/protocol contracts, never by calling
  // a scratch hook against the operator's inherited live owner in this smoke.
  for (const command of ['serena', 'playwright']) {
    const result = run([command]);
    assert.equal(result.status, 1, result.stderr);
    assert.equal(result.stdout, '');
    assert.match(result.stderr, new RegExp(`native ${command} support is deferred.*existing supported Unix`));
  }
  const paths = run(['path', 'codex-home']);
  assert.equal(paths.status, 0, paths.stderr); assert.equal(paths.stdout, join(fixture, 'isolated Codex') + '\n');
  const skills = run(['skills', '--help']); assert.equal(skills.status, 0, skills.stderr); assert.match(skills.stdout, /^Usage: gestalt skills/);
  const retired = run(['xerj', 'mcp']);
  assert.equal(retired.status, 1);
  assert.equal(retired.stdout, '');
  assert.match(retired.stderr, /Xerj has been retired.*User data is retained/);
});

test('deferred native tool requests refuse before touching absent or existing profiles', async t => {
  const fixture = await mkdtemp(join(tmpdir(), 'native deferred ü '));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const { mkdir, readdir, lstat, writeFile } = await import('node:fs/promises');
  const codexHome = join(fixture, 'Codex'), gestaltHome = join(fixture, 'Gestalt');
  const environment = { HOME: fixture, USERPROFILE: fixture, CODEX_HOME: codexHome, GESTALT_HOME: gestaltHome, PATH: '' };
  for (const present of [false, true]) {
    if (present) {
      await mkdir(codexHome); await mkdir(gestaltHome);
      await writeFile(join(codexHome, 'config.toml'), 'model="operator"\n');
    }
    const inventory = async () => {
      const entries = [];
      for (const name of (await readdir(fixture)).sort()) {
        const directory = join(fixture, name), info = await lstat(directory);
        entries.push([name, info.mode, info.mtimeMs]);
        for (const child of (await readdir(directory)).sort()) entries.push([name, child, (await readFile(join(directory, child))).toString('base64')]);
      }
      return entries;
    };
    const before = await inventory();
    for (const command of ['serena', 'playwright']) {
      const output = [], error = [], streams = { stdin: (async function* () { throw new Error('deferred command consumed stdin'); })(), stdout: async bytes => output.push(bytes), stderr: async bytes => error.push(bytes) };
      assert.equal(await runCli({ argv: [command, 'install', '--cwd', 'literal ü'], cwd: fixture, environment }, streams, metadata, createRuntimeHandlers(metadata)), 1);
      assert.equal(output.length, 0); assert.match(Buffer.concat(error).toString(), /support is deferred.*existing supported Unix.*No profile or installation was changed/);
      assert.deepEqual(await inventory(), before);
    }
  }
});

test('compiled Unix direct entry uses the byte-identical packaged legacy manager', { skip: process.platform === 'win32' }, async t => {
  const fixture = await mkdtemp(join(tmpdir(), 'native cli Unix consumer ü '));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const entry = await buildCli(root, join(fixture, 'gestalt.mjs'));
  assert.equal(await readFile(join(fixture, 'legacy/gestalt'), 'utf8'), source);
  const environment = { ...process.env, HOME: fixture, USERPROFILE: fixture, CODEX_HOME: join(fixture, 'Codex ü'), GESTALT_HOME: join(fixture, 'Gestalt ü') };
  for (const argv of [['help'], ['path', 'codex-home'], ['xerj'], ['unknown command']]) {
    const options = { cwd: fixture, encoding: 'utf8', timeout: 10000, env: environment };
    const actual = spawnSync(process.execPath, [entry, ...argv], options);
    const legacy = spawnSync(join(root, 'public/gestalt'), argv, options);
    assert.equal(actual.error, undefined); assert.equal(legacy.error, undefined);
    assert.equal(actual.status, legacy.status); assert.equal(actual.signal, legacy.signal);
    assert.equal(actual.stdout, legacy.stdout); assert.equal(actual.stderr, legacy.stderr);
  }
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
  if (process.platform === 'win32') {
    assert.equal(result.status, 1); assert.equal(result.stdout, '');
    assert.equal(result.stderr, "gestalt: unknown command: unknown command (run 'gestalt help')\n");
  } else {
    const legacy = spawnSync(join(root, 'public/gestalt'), ['unknown command'], { cwd: root, encoding: 'utf8', timeout: 15000 });
    assert.equal(result.status, legacy.status); assert.equal(result.stdout, legacy.stdout); assert.equal(result.stderr, legacy.stderr);
  }
});

test('compiled replacement API imports without CLI effects and honors explicit cancelled invocation', async t => {
  const fixture = await mkdtemp(join(tmpdir(), 'native update API ü '));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const entry = await buildCli(root, join(fixture, 'gestalt.mjs'));
  const program = `const api=await import(${JSON.stringify(pathToFileURL(entry).href+'?test-import-safe=1')});
const controller=new AbortController();controller.abort({status:143});const errors=[];
const outcome=await api.continueUpdatedManager({argv:['__updated-manager','update','gestalt'],cwd:${JSON.stringify(fixture)},environment:{HOME:${JSON.stringify(fixture)}}},
{stdin:(async function*(){})(),stdout:async()=>{throw new Error('unexpected CLI output')},stderr:async b=>errors.push(Buffer.from(b).toString())},
{commands:{apiVersion:1,launch:async()=>{throw new Error('cancelled handoff launched a command')}},signal:controller.signal});
console.log(JSON.stringify({version:api.UPDATED_MANAGER_API_VERSION,capabilities:api.UPDATED_MANAGER_API_CAPABILITIES,outcome,errors}));`;
  const result = spawnSync(process.execPath, ['--input-type=module', '-e', program], {
    cwd: fixture, encoding: 'utf8', timeout: 15000, env: { ...process.env, NODE_OPTIONS: '', NODE_PATH: '' },
  });
  assert.equal(result.status, 0, result.stderr); assert.equal(result.stderr, '');
  const receipt = JSON.parse(result.stdout);
  assert.equal(receipt.version, 1); assert.ok(receipt.capabilities.includes('host-filesystem-v1'));
  assert.equal(receipt.outcome.status, 143); assert.equal(receipt.outcome.ownership.kind, 'released');
  assert.deepEqual(receipt.errors, ['gestalt: setup interrupted\n']);
});
