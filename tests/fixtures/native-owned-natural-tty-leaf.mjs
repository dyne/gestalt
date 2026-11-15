import assert from 'node:assert/strict';
import { once } from 'node:events';
import { journal, resources } from './native-owned-observer.mjs';
const [record] = process.argv.slice(2);
process.on('beforeExit', code => journal(record, 'leaf-beforeExit', { code, ...resources() }));
process.on('exit', code => journal(record, 'leaf-exit', { code }));
assert.deepEqual([process.stdin.isTTY, process.stdout.isTTY, process.stderr.isTTY], [true, true, true]);
const raw = Boolean(process.stdin.isRaw); journal(record, 'leaf-start', resources());
try {
  process.stdin.setRawMode(true); process.stdout.write('NATURAL_LEAF_READY\n');
  while (true) { const [bytes] = await once(process.stdin, 'data'); journal(record, 'leaf-input', { hex: bytes.toString('hex') }); if (bytes.includes(4)) break; }
} finally { process.stdin.setRawMode(raw); process.stdin.pause(); }
process.exitCode = 23;
journal(record, 'leaf-post-return', { exitCode: process.exitCode, ...resources() });
// No process.exit, deadline or background timer can provide a successful exit.
