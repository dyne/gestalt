import assert from 'node:assert/strict';
import path from 'node:path';
import { writeFileSync } from 'node:fs';
import { NativeOwnedProcesses } from '../../src/cli/platform/processes.ts';
import { journal, resources, observeOwnedHelpers } from './native-owned-observer.mjs';
const [helper, leaf, leafRecord, record, traceDirectory] = process.argv.slice(2);
process.on('beforeExit', code => journal(record, 'host-beforeExit', { code, ...resources() }));
process.on('exit', code => journal(record, 'host-exit', { code }));
assert.deepEqual([process.stdin.isTTY, process.stdout.isTTY, process.stderr.isTTY], [true, true, true]);
const observer = observeOwnedHelpers(helper, traceDirectory, 'inner'), raw = Boolean(process.stdin.isRaw);
journal(record, 'host-start', resources());
try {
  process.stdin.setRawMode(true);
  const lease = await new NativeOwnedProcesses(helper).launchTerminal(process.execPath, { argv: [leaf, leafRecord], cwd: path.dirname(leaf), environment: process.env },
    { input: process.stdin, output: bytes => new Promise((resolve, reject) => process.stdout.write(bytes, error => error ? reject(error) : resolve())) }, { columns: 91, rows: 37 }, { graceMs: 50, deadlineMs: 5000 });
  journal(record, 'inner-launch', { claim: lease.claim(), ...resources() });
  const result = await lease.result; journal(record, 'inner-result', { result, claim: lease.claim(), ...resources() });
  const completed = await lease.completed; journal(record, 'inner-completed', { completed, claim: lease.claim(), ...resources() });
  assert.deepEqual(result, { kind: 'exit', code: 23 }); assert.equal(completed.kind, 'released'); assert.equal(completed.backend, 'windows-job'); assert.equal(completed.lease, lease.id);
  const stop = await lease.stop(); journal(record, 'inner-idempotent-stop', { claim: stop });
  process.exitCode = 23;
} catch (error) { journal(record, 'host-error', { message: error.message, stack: error.stack }); process.exitCode = 1; }
finally {
  process.stdin.setRawMode(raw); process.stdin.pause(); observer.restore();
  writeFileSync(path.join(traceDirectory, 'inner-traces.json'), JSON.stringify(observer.traces, null, 2));
  journal(record, 'host-post-return', { exitCode: process.exitCode, ...resources() });
}
// Natural exit is observed by the outer lease. No process.exit or timer success.
