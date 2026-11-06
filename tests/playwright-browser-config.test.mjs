import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { runInNewContext } from 'node:vm';

const manager = readFileSync(new URL('../public/gestalt', import.meta.url), 'utf8');
const browserSource = manager.slice(manager.indexOf('function browser(explicit) {'),
  manager.indexOf('// Never forward upstream environment controls'));

function browserResolver(available) {
  const calls = [];
  const browser = runInNewContext(`${browserSource}\nbrowser`, {
    path,
    process: { platform: 'linux' },
    fs: {
      constants: { X_OK: 1 },
      accessSync: (executable) => { if (!available.includes(executable)) throw Error('missing'); },
      realpathSync: (executable) => executable,
    },
    spawnSync: (executable, args) => {
      calls.push({ executable, args: [...args] });
      return { status: 0, stdout: 'Google Chrome 140.0.1.2' };
    },
  });
  return { browser, calls };
}

test('production MCP resolves system Chrome and never discovers a bundled browser cache', () => {
  const installed = browserResolver(['/opt/google/chrome/chrome']);
  assert.equal(installed.browser().executable, '/opt/google/chrome/chrome');
  assert.deepEqual(installed.calls, [{ executable: '/opt/google/chrome/chrome', args: ['--version'] }]);
  const cachedOnly = browserResolver(['/cache/ms-playwright/chromium/chrome']);
  assert.equal(cachedOnly.browser().reason, 'chrome-missing');
  assert.deepEqual(cachedOnly.calls, []);
  assert.equal(installed.browser('/missing/custom-chrome').ready, false);
  assert.equal(installed.calls.length, 1);
});

test('documentation browser tests select installed Chrome without a bundled fallback', async () => {
  const { default: config } = await import('../playwright.config.ts');
  assert.equal(config.use.channel, 'chrome');
  assert.deepEqual(config.use.launchOptions,
    process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : {});
});
