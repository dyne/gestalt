#!/usr/bin/env node
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, realpathSync, symlinkSync } from 'node:fs';
import path from 'node:path';

const { PAIR_CODEX, PAIR_LOCAL_AGENTS, PAIR_MARKETPLACE_TRACE, CODEX_HOME } = process.env;
const args = process.argv.slice(2);
const name = 'dyne-gestalt-agents';
const log = fields => appendFileSync(PAIR_MARKETPLACE_TRACE, JSON.stringify({ observedAt: new Date().toISOString(), ...fields }) + '\n');
function actual(argv, stdio = 'inherit') {
  const result = spawnSync(PAIR_CODEX, argv, { encoding: 'utf8', stdio, shell: false, timeout: 60_000 });
  if (result.error) throw result.error;
  return result;
}
function registeredSource() {
  const result = actual(['plugin', 'marketplace', 'list'], ['ignore', 'pipe', 'inherit']);
  assert.equal(result.status, 0, 'Could not read the actual local Codex registration');
  return result.stdout.split(/\r?\n/).map(line => /^\s*(\S+)\s+(.+?)\s*$/.exec(line)).find(row => row?.[1] === name)?.[2];
}

try {
  for (const value of [PAIR_CODEX, PAIR_LOCAL_AGENTS, PAIR_MARKETPLACE_TRACE, CODEX_HOME]) assert.ok(path.isAbsolute(value ?? ''));
  if (args[0] === 'plugin' && args[1] === 'marketplace' && ['add', 'upgrade'].includes(args[2])) {
    assert.equal(args.length, 4);
    assert.ok(args[2] === 'add' ? [PAIR_LOCAL_AGENTS, 'dyne/gestalt-agents'].includes(args[3]) : args[3] === name);
    const cache = path.join(CODEX_HOME, '.tmp', 'marketplaces', name);
    const source = realpathSync(PAIR_LOCAL_AGENTS);
    let registered = registeredSource();
    if (!registered) {
      const argv = ['plugin', 'marketplace', 'add', source];
      log({ event: 'local-add', incoming: args, actual: argv });
      const result = actual(argv);
      assert.equal(result.status, 0, 'Actual Codex rejected local marketplace registration');
      registered = registeredSource();
    } else log({ event: 'local-repeat', incoming: args, actual: ['plugin', 'marketplace', 'list'] });
    assert.equal(realpathSync(registered), source, 'Codex active registration must use the verified fixture source');
    if (!existsSync(cache)) {
      mkdirSync(path.dirname(cache), { recursive: true, mode: 0o700 });
      symlinkSync(source, cache, 'dir');
    }
    assert.equal(realpathSync(cache), source, 'Manager must pass through the same verified fixture source');
    log({ event: 'registration-verified', source, cache, status: 0 });
  } else {
    const permitted = [
      ['--version'], ['app-server', '--stdio'], ['plugin', 'add', 'context-mode@' + name], ['plugin', 'add', 'gestalt@' + name],
      ['plugin', 'list', '--marketplace', name, '--json'], ['plugin', 'marketplace', 'list'], ['mcp', 'get', 'context-mode', '--json'],
    ].some(command => args.length === command.length && command.every((arg, index) => arg === args[index]));
    assert.ok(permitted, 'Unexpected command in the isolated pairing check');
    process.execve(PAIR_CODEX, [PAIR_CODEX, ...args], process.env);
  }
} catch (error) {
  console.error('unix pairing Codex wrapper: ' + error.message);
  process.exitCode = 1;
}
