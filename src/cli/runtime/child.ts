import { EventEmitter, once } from 'node:events';
import { PassThrough } from 'node:stream';
import type { Environment, OwnedProcessLease } from '../contracts.ts';
import { RuntimeCommands, resultStatus } from './commands.ts';

/** Stream/event seam for the extracted native-config protocol. It owns no PID
 * and exposes no group signalling: every termination awaits the same lease. */
export function managedChild(commands: RuntimeCommands, executable: string, argv: readonly string[],
  options: { readonly cwd: string; readonly env: Environment; readonly stdio: readonly unknown[] }) {
  const child = new EventEmitter() as EventEmitter & {
    stdin: PassThrough; stdout: PassThrough; stderr: PassThrough;
    exitCode: number | null; signalCode: string | null;
    kill(signal?: string): boolean; stop(): Promise<void>;
  };
  child.stdin = new PassThrough(); child.stdout = new PassThrough(); child.stderr = new PassThrough();
  child.exitCode = null; child.signalCode = null;
  let lease: OwnedProcessLease | undefined, requested = false;
  const input = options.stdio[0] === 'ignore' ? (async function* () {})() : options.stdio[0] === 3 ? commands.streams.stdin : child.stdin;
  const pipe = (stream: PassThrough, index: 1 | 2) => async (bytes: Uint8Array) => {
    if (options.stdio[index] === 'ignore') return;
    if (options.stdio[index] === 'inherit') return index === 1 ? commands.streams.stdout(bytes) : commands.streams.stderr(bytes);
    if (!stream.write(bytes)) await once(stream, 'drain');
  };
  const started = (async () => {
    try {
      lease = await commands.launch(executable, argv, { stdin: input, stdout: pipe(child.stdout, 1), stderr: pipe(child.stderr, 2) }, options.env, options.cwd);
      if (requested) await lease.stop();
      const result = await lease.result;
      child.exitCode = resultStatus(result); child.emit('exit', child.exitCode, null);
      await lease.stop(); child.stdout.end(); child.stderr.end(); child.stdin.destroy();
      child.emit('close', child.exitCode, null);
    } catch (error) {
      child.exitCode = 127; child.emit('error', error);
      child.stdin.destroy(); child.stdout.end(); child.stderr.end(); child.emit('exit', 127, null); child.emit('close', 127, null);
    }
  })();
  child.kill = () => { requested = true; void lease?.stop(); return true; };
  child.stop = async () => { requested = true; child.stdout.resume(); child.stderr.resume(); await lease?.stop(); await started; };
  return child;
}
