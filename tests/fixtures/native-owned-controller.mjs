import { NativeOwnedProcesses } from '../../src/cli/platform/processes.ts';
import { PassThrough } from 'node:stream';
const [helper, fixture, token, mode] = process.argv.slice(2);
process.send?.({ kind: 'starting' });
const adapter = new NativeOwnedProcesses(helper), input = new PassThrough();
const invocation = {
  argv: [fixture, 'root', 'resistant', token], cwd: process.cwd(), environment: { ...process.env, GESTALT_OWNED_FIXTURE: token },
};
const launched = process.env.GESTALT_TEST_OWNED_TERMINAL === '1'
  ? adapter.launchTerminal(process.execPath, invocation, { input, output: async bytes => { process.stdout.write(bytes); } }, { columns: 100, rows: 35 })
  : adapter.launch(process.execPath, invocation, { stdin: (async function* () {})(), stdout: async bytes => { process.stdout.write(bytes); }, stderr: async bytes => { process.stderr.write(bytes); } });
if (mode === 'immediate-crash') setImmediate(() => process.kill(process.pid, 'SIGKILL'));
const lease = await launched;
process.send?.({ kind: 'lease-ready', claim: lease.claim() });
if (lease.claim().kind === 'unknown') throw new Error(`Ownership launch refused: ${JSON.stringify(lease.claim())}`);
process.on('message', message => { if (message === 'crash') process.kill(process.pid, 'SIGKILL'); });
await lease.completed;
