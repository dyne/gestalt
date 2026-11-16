import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { authorizeFixtureHooks } from './fixtures/native-cli-hook-trust.mjs';

const expected = [
  { kind: 'verified-installed-context-mode', sourcePath: '/isolated/Codex/hooks.json', eventName: 'sessionStart', command: 'exact accepted managed command' },
  { kind: 'explicit-blocking-fixture', sourcePath: '/isolated/Codex/config.toml', eventName: 'sessionStart', command: 'exact isolated fixture command' },
];
const hooks = () => expected.map((entry, index) => ({ ...entry, source: 'user', enabled: true, key: entry.sourcePath + ':' + index,
  currentHash: 'sha256:' + String(index).repeat(64), trustStatus: 'untrusted' }));

test('fixture authorization selects only the exact verified commands, events and source files', () => {
  const selected = authorizeFixtureHooks(hooks(), expected);
  assert.deepEqual(selected.map(value => value.kind), expected.map(value => value.kind));
  assert.deepEqual(selected.map(value => value.key), hooks().map(value => value.key));
});
for (const fault of ['extra', 'command', 'source', 'event', 'disabled', 'duplicate-key']) test('fixture authorization refuses ' + fault, () => {
  const actual = hooks();
  if (fault === 'extra') actual.push({ ...actual[0], key: 'foreign', command: 'unverified' });
  if (fault === 'command') actual[0].command += ' changed';
  if (fault === 'source') actual[0].sourcePath = '/operator/live/hooks.json';
  if (fault === 'event') actual[0].eventName = 'userPromptSubmit';
  if (fault === 'disabled') actual[0].enabled = false;
  if (fault === 'duplicate-key') actual[1].key = actual[0].key;
  assert.throws(() => authorizeFixtureHooks(actual, expected));
});

for (const writable of [true, false]) test(`passive spawn capture preserves literal argv, binary streams and exit with capture ${writable ? 'available' : 'unavailable'}`, async t => {
  const fixture = await mkdtemp(path.join(tmpdir(), 'core-slice-capture-'));
  t.after(() => rm(fixture, { recursive: true, force: true }));
  const capture = writable ? path.join(fixture, 'calls.jsonl') : path.join(fixture, 'missing-parent/calls.jsonl');
  const preload = pathToFileURL(path.resolve(import.meta.dirname, 'fixtures/native-cli-spawn-capture.mjs')).href;
  const child = `const input=[];for await(const b of process.stdin)input.push(b);process.stdout.write(Buffer.concat(input));process.stderr.write(JSON.stringify(process.argv.slice(1)));process.exitCode=23;`;
  const program = `import{spawn}from'node:child_process';const child=spawn(process.execPath,['--input-type=module','-e',${JSON.stringify(child)},'literal ü & $HOME',''],{stdio:['pipe','pipe','pipe']});process.stdin.pipe(child.stdin);child.stdout.pipe(process.stdout);child.stderr.pipe(process.stderr);child.on('exit',code=>process.exitCode=code);`;
  const input = Buffer.from([0, 255, 13, 10, 128]);
  const result = spawnSync(process.execPath, ['--import=' + preload, '--input-type=module', '-e', program],
    { env: { ...process.env, GESTALT_SLICE_CODEX_BINARY: process.execPath, GESTALT_SLICE_SPAWN_CAPTURE: capture }, input, timeout: 10000 });
  assert.equal(result.status, 23, result.stderr.toString()); assert.deepEqual(result.stdout, input);
  assert.deepEqual(JSON.parse(result.stderr), ['literal ü & $HOME', '']);
  if (writable) {
    const calls = (await readFile(capture, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
    assert.equal(calls.length, 1); assert.equal(calls[0].target, process.execPath); assert.deepEqual(calls[0].argv.slice(-2), ['literal ü & $HOME', '']);
  }
});
