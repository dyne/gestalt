#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { readFileSync } from 'node:fs';
import path from 'node:path';

// Windows owns the new native Node runtime. Unix proves the same common
// libraries and its actual compiled-to-legacy entry, never the retired mapped
// namespace experiment. The Manager suite covers the existing Unix backend.
const focused = ['archive', 'diagnostics', 'app-server', 'capabilities', 'skills-owner', 'skills-transfer', 'scratch', 'hook-command', 'storage-read']
  .map(name => `tests/native-${name}.test.mjs`);
const packageFocused = process.env.GESTALT_NATIVE_PACKAGE_FOCUSED === '1';
if (packageFocused) {
  if (process.platform !== 'win32') throw Error('The packed native runtime gate requires Windows');
  const directory = process.env.GESTALT_NATIVE_RUNTIME_ARTIFACTS;
  if (!directory || !path.isAbsolute(directory) || !JSON.parse(readFileSync(path.join(directory, 'artifacts.json'))).package)
    throw Error('The packed native runtime gate requires an explicitly prepared actual tarball');
}
const files = packageFocused ? ['tests/native-runtime-installed.test.mjs']
  : [...focused, 'tests/native-cli.test.mjs', ...(process.platform === 'win32' ? ['tests/native-runtime-installed.test.mjs'] : [])];
const result = spawnSync(process.execPath, ['--test', '--test-reporter=tap',
  ...(packageFocused ? ['--test-name-pattern=^actual compiled (?:AppServer|CLI|skills|scratch|diagnostics)'] : []), ...files], {
  cwd: fileURLToPath(new URL('..', import.meta.url)), stdio: 'inherit', shell: false,
});
if (result.error) { console.error(result.error.message); process.exitCode = 1; }
else process.exitCode = result.status ?? (result.signal === 'SIGINT' ? 130 : result.signal === 'SIGTERM' ? 143 : 1);
