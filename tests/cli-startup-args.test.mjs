import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const source = await readFile(new URL('../public/gestalt', import.meta.url), 'utf8');
const body = 'const appendFlags = flags => {' + source.split('const appendFlags = flags => {')[1].split('\nasync function snapshot(')[0];
const append = new Function('args', 'playwrightController', body + '\nreturn appendFlags([\'GENERATED_CONFIG\']);');

for (const [label, args, insertion] of [
  ['global user config', ['-c', 'developer_instructions="USER"', 'exec', 'PROMPT'], 2],
  ['subcommand user config', ['exec', '-c', 'developer_instructions="USER"', 'PROMPT'], 3],
  ['last original config controls placement', ['-c', 'first=true', 'exec', '--config=second=true', 'PROMPT'], 4],
  ['attached short config', ['-cdeveloper_instructions="USER"', 'exec', 'PROMPT'], 1],
  ['global hooks stay before composed hooks', ['-c', 'hooks.SessionStart=[]', 'exec', 'PROMPT'], 2],
  ['model operand named exec', ['-m', 'exec', 'resume', '--last'], 4],
  ['profile operand named resume', ['--profile', 'resume', 'exec', 'PROMPT'], 4],
  ['attached model operand', ['--model=exec', 'fork', '--last'], 3],
  ['command-like prompt', ['ordinary prompt', 'exec'], 2],
  ['literal command after separator', ['--', 'exec', '-c', 'PROMPT'], 0],
  ['subcommand literal prompt', ['exec', '--', 'resume', '-c', 'PROMPT'], 1],
]) test(`generated CLI config preserves parse scope: ${label}`, () => {
  assert.deepEqual(append(args, undefined), ['--no-daemon', ...args.slice(0, insertion), '-c', 'GENERATED_CONFIG', ...args.slice(insertion)]);
});

test('explicit embedded mode and remote mode keep their native selection', () => {
  assert.deepEqual(append(['exec', '--no-daemon', 'PROMPT'], undefined), ['exec', '--no-daemon', 'PROMPT', '-c', 'GENERATED_CONFIG']);
  assert.deepEqual(append(['--remote', 'ws://fixture', 'PROMPT'], undefined), ['--remote', 'ws://fixture', 'PROMPT', '-c', 'GENERATED_CONFIG']);
});
