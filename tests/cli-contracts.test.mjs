import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { commandNames, internalCommandNames, commandAliases, retiredCommandNames, updateComponents } from '../src/cli/contracts.ts';
import { buildNpmPackage } from '../scripts/build-npm-package.mjs';

const root = resolve(import.meta.dirname, '..');
const source = await readFile(join(root, 'public/gestalt'), 'utf8');

test('migration inventory covers the current dispatcher, aliases and update selection', () => {
  const main = source.slice(source.indexOf('\nmain() {'));
  const dispatched = [...main.matchAll(/^    ([\w|-]+(?: \| [\w-]+)*)\)/gm)]
    .flatMap(match => match[1].split(/ \| /));
  assert.deepEqual(dispatched.sort(), [...commandNames, ...internalCommandNames.filter(name => name !== '__updated-manager'),
    ...Object.keys(commandAliases), ...retiredCommandNames].sort());
  assert.match(main, /\[\[ \$\{1:-\} == __updated-manager \]\]/);
  assert.deepEqual(updateComponents, ['all', 'gestalt', 'serena', 'playwright']);
});

test('real standalone and npm wrappers preserve read-only help/version/error contracts', async t => {
  const fixture = await mkdtemp(join(tmpdir(), 'cli contracts '));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const codexHome = join(fixture, 'codex'), gestaltHome = join(fixture, 'managed');
  await mkdir(codexHome); await mkdir(gestaltHome);
  // Package generation is identical to release generation; existing package tests also
  // install its tarball and exercise argv, stdin, stdout and exit forwarding offline.
  const packaged = await buildNpmPackage(root);
  const env = { ...process.env, CODEX_HOME: codexHome, GESTALT_HOME: gestaltHome };
  const version = /^readonly GESTALT_CLI_VERSION='([^']+)'$/m.exec(source)[1];
  for (const entry of [join(root, 'public/gestalt'), join(packaged, 'bin/gestalt')]) {
    const run = args => spawnSync('bash', [entry, ...args], { env, encoding: 'utf8', timeout: 15000 });
    const help = run([]);
    assert.equal(help.status, 0, help.stderr);
    assert.equal(help.stderr, '');
    assert.match(help.stdout, /^Usage: gestalt/);
    for (const alias of ['help', '--help', '-h']) assert.equal(run([alias]).stdout, help.stdout);
    for (const alias of ['version', '--version', '-V']) {
      const result = run([alias]);
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.stdout.split('\n')[0], `gestalt ${version}`);
    }
    for (const [args, message] of [
      [['unknown command'], "unknown command: unknown command (run 'gestalt help')"],
      [['agents', 'invalid'], 'usage: gestalt agents install|update'],
      [['path', 'invalid'], 'unknown managed path: invalid'],
      [['update', '--component'], '--component requires'],
      [['update', 'serena', '--extra-skills'], '--extra-skills requires'],
      [['xerj', 'mcp'], 'Xerj has been retired'],
      [['update', 'xerj'], 'Xerj has been retired'],
    ]) {
      const result = run(args);
      assert.equal(result.status, 1, result.stderr);
      assert.equal(result.stdout, '');
      assert.ok(result.stderr.includes(message), result.stderr);
    }
    const path = run(['path', 'codex-home']);
    assert.equal(path.status, 0, path.stderr);
    assert.equal(path.stdout, `${codexHome}\n`);
  }
});
