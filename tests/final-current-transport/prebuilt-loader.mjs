import assert from 'node:assert/strict';
import { appendFileSync } from 'node:fs';
import { registerHooks } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

assert.equal(process.version, 'v24.22.3');
assert.equal(process.platform, 'win32');
assert.equal(typeof registerHooks, 'function');
assert.ok(path.isAbsolute(process.env.FINAL_WIN_PREBUILT ?? ''));
const parent = new URL('../native-setup-installed.test.mjs', import.meta.url).href;
const adapter = new URL('./prebuilt-adapter.mjs', import.meta.url).href;
const approved = new Set(['../scripts/build-cli.mjs', '../scripts/build-owned-process.mjs']);
const receipt = path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, 'fixture-loader.ndjson');
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (context.parentURL === parent && approved.has(specifier)) {
      appendFileSync(receipt, JSON.stringify({ fixture: fileURLToPath(parent), replacedImport: specifier, adapter: fileURLToPath(adapter) }) + '\n');
      return { url: adapter, shortCircuit: true };
    }
    const resolved = nextResolve(specifier, context);
    assert.notEqual(resolved.url, adapter, 'Unexpected direct prebuilt adapter import');
    return resolved;
  },
});
