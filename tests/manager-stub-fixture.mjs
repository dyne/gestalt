import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';

// Replacement Manager executables still supply the actual inherited scratch
// contract. Keep it inert; only the real public manager handles scratch hooks.
export function managerStubScratchRuntime(source) {
  const runtime = source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n[\s\S]*?^\/\/ END SESSION SCRATCH RUNTIME/m)?.[0];
  assert.ok(runtime, 'public manager scratch runtime missing');
  assert.ok(!runtime.includes('*/'), 'scratch runtime cannot be embedded in a fixture block comment');
  const manager = fileURLToPath(new URL('../public/gestalt', import.meta.url));
  return `/*
${runtime}
*/
if (process.argv[2] === '__scratch-hook') {
  const hook = require('node:child_process').spawnSync(${JSON.stringify(manager)}, process.argv.slice(2),
    { stdio: 'inherit', env: process.env });
  if (hook.error) throw hook.error;
  process.exit(hook.status ?? 1);
}
`;
}
