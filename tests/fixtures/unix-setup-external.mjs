#!/usr/bin/env node
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, realpathSync, symlinkSync } from 'node:fs';
import path from 'node:path';

const args = process.argv.slice(2);
const {
  CODEX_HOME, GESTALT_HOME, UNIX_SETUP_CODEX, UNIX_SETUP_NPM, UNIX_SETUP_AGENTS,
  UNIX_SETUP_MOBILE_PACKAGE, UNIX_SETUP_TRACE,
} = process.env;
const name = 'dyne-gestalt-agents';
const log = fields => appendFileSync(UNIX_SETUP_TRACE, JSON.stringify({ observedAt: new Date().toISOString(), ...fields }) + '\n');
const execute = (target, argv) => process.execve(target, [target, ...argv], process.env);

function codex(argv, capture = false) {
  const result = spawnSync(UNIX_SETUP_CODEX, argv, {
    encoding: 'utf8', stdio: ['ignore', capture ? 'pipe' : 'inherit', 'inherit'], shell: false, timeout: 60_000,
  });
  if (result.error) throw result.error;
  assert.equal(result.status, 0, 'Actual Codex rejected the local marketplace command');
  return result.stdout;
}

function registeredSource() {
  return codex(['plugin', 'marketplace', 'list'], true).split(/\r?\n/)
    .map(line => /^\s*(\S+)\s+(.+?)\s*$/.exec(line)).find(row => row?.[1] === name)?.[2];
}

function marketplace() {
  assert.equal(args.length, 4);
  const source = realpathSync(UNIX_SETUP_AGENTS);
  assert.ok(args[2] === 'add' ? [source, 'dyne/gestalt-agents'].includes(args[3]) : args[3] === name);
  if (!registeredSource()) {
    const actual = ['plugin', 'marketplace', 'add', source];
    log({ event: 'local-marketplace-add', incoming: args, actual });
    codex(actual);
  }
  const registered = registeredSource();
  assert.ok(registered && realpathSync(registered) === source);
  const cache = path.join(CODEX_HOME, '.tmp', 'marketplaces', name);
  if (!existsSync(cache)) {
    mkdirSync(path.dirname(cache), { recursive: true, mode: 0o700 });
    symlinkSync(source, cache, 'dir');
  }
  assert.equal(realpathSync(cache), source);
  log({ event: 'local-marketplace-verified', incoming: args, source, cache, registered });
}

try {
  for (const value of [CODEX_HOME, GESTALT_HOME, UNIX_SETUP_CODEX, UNIX_SETUP_NPM, UNIX_SETUP_AGENTS, UNIX_SETUP_MOBILE_PACKAGE, UNIX_SETUP_TRACE])
    assert.ok(path.isAbsolute(value ?? ''));
  const tool = path.basename(import.meta.filename);
  if (tool === 'npm') {
    if (args[0] === 'install' && /^gestalt-mobile@/.test(args.at(-1) ?? '')) {
      const prefix = args.indexOf('--prefix');
      assert.ok(prefix > 0);
      assert.equal(args[prefix + 1], path.join(GESTALT_HOME, 'mobile'));
      const actual = [...args.slice(0, -1), UNIX_SETUP_MOBILE_PACKAGE];
      log({ event: 'local-mobile-package', incoming: args, actual });
      execute(UNIX_SETUP_NPM, actual);
    } else execute(UNIX_SETUP_NPM, args);
  } else {
    assert.equal(tool, 'codex');
    if (args[0] === 'plugin' && args[1] === 'marketplace' && ['add', 'upgrade'].includes(args[2])) marketplace();
    else {
      const permitted = [
        ['--version'], ['app-server', '--stdio'], ['plugin', 'add', 'context-mode@' + name], ['plugin', 'add', 'gestalt@' + name],
        ['plugin', 'list', '--marketplace', name, '--json'], ['plugin', 'marketplace', 'list'], ['mcp', 'get', 'context-mode', '--json'],
      ].some(command => args.length === command.length && command.every((value, index) => value === args[index]));
      assert.ok(permitted, 'Unexpected Codex command in isolated installed setup');
      execute(UNIX_SETUP_CODEX, args);
    }
  }
} catch (error) {
  console.error('isolated Unix setup edge: ' + error.message);
  process.exitCode = 1;
}
