import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
import type { CommandInvocation, CommandStreams, OwnedProcessLease, OwnedProcessPort, OwnedProcessResult, OwnershipClaim, ProcessResult, OwnedTerminalLease, OwnedTerminalPort, OwnedTerminalStreams, TerminalSize } from '../contracts.ts';
import { commandTarget } from './executables.ts';

const maximumFrame = 1024 * 1024;
function packet(kind: string, bytes: Uint8Array = new Uint8Array()): Buffer {
  const body = Buffer.allocUnsafe(bytes.byteLength + 5);
  body.writeUInt32LE(bytes.byteLength + 1); body[4] = kind.charCodeAt(0); body.set(bytes, 5);
  return body;
}
function boundedInteger(value: number, minimum: number, maximum: number, name: string): number {
  if (!Number.isSafeInteger(value) || value < minimum || value > maximum) throw new Error(`Invalid ${name}`);
  return value;
}

/** Persisted identities cannot reopen a kernel ownership object or authorize a signal. */
export function recoveredOwnershipClaim(lease: string): OwnershipClaim {
  return { kind: 'unknown', lease, reason: 'No live kernel ownership handle; retain scratch' };
}

export class NativeOwnedProcesses implements OwnedProcessPort, OwnedTerminalPort {
  private readonly helper: string;
  constructor(helper: string) {
    if (!path.isAbsolute(helper)) throw new Error('Target-built ownership helper must be absolute');
    this.helper = helper;
  }

  async launch(executable: string, invocation: CommandInvocation, streams: CommandStreams,
    limits: { readonly graceMs?: number; readonly deadlineMs?: number } = {}): Promise<OwnedProcessLease> {
    return this.launchNative(executable, invocation, streams, limits);
  }

  async launchTerminal(executable: string, invocation: CommandInvocation, streams: OwnedTerminalStreams,
    size: TerminalSize, limits: { readonly graceMs?: number; readonly deadlineMs?: number } = {}): Promise<OwnedTerminalLease> {
    return this.launchNative(executable, invocation, {
      stdin: streams.input, stdout: bytes => streams.output(bytes),
      stderr: async () => { throw new Error('Unexpected separate stderr from owned terminal'); },
    }, limits, size);
  }

  private async launchNative(executable: string, invocation: CommandInvocation, streams: CommandStreams,
    limits: { readonly graceMs?: number; readonly deadlineMs?: number }, terminal?: TerminalSize): Promise<OwnedTerminalLease> {
    if (!['linux', 'win32', 'darwin'].includes(process.platform))
      throw new Error('Native process lifetime adapter is unavailable on this target');
    if (!path.isAbsolute(executable) || !path.isAbsolute(invocation.cwd)) throw new Error('Owned executable and cwd must be absolute');
    const grace = boundedInteger(limits.graceMs ?? 250, 0, 60000, 'grace interval');
    const deadline = boundedInteger(limits.deadlineMs ?? 10000, grace + 1, 120000, 'cleanup deadline');
    const target = await commandTarget(executable);
    const dimensions = (size: TerminalSize) => {
      const bytes = Buffer.alloc(4);
      bytes.writeUInt16LE(boundedInteger(size.columns, 1, 32767, 'terminal columns'), 0);
      bytes.writeUInt16LE(boundedInteger(size.rows, 1, 32767, 'terminal rows'), 2);
      return bytes;
    };
    if (terminal) dimensions(terminal);
    const id = randomUUID();
    const expectedBackend = process.platform === 'win32' ? 'windows-job' : process.platform === 'darwin' ? 'macos-session-group' : 'linux-pid-namespace';
    let claim: OwnershipClaim = recoveredOwnershipClaim(id);
    let result: ProcessResult | undefined;
    let nativeEmpty = false, backend = '', settled = false, verificationLost = false;
    let inputWaiter: { resolve: () => void; reject: (error: Error) => void } | undefined;
    let resizeWaiter: { resolve: () => void; reject: (error: Error) => void } | undefined;
    let finish!: (claim: OwnershipClaim) => void;
    const completed = new Promise<OwnershipClaim>(resolve => { finish = resolve; });
    let finishResult!: (result: OwnedProcessResult) => void;
    const commandResult = new Promise<OwnedProcessResult>(resolve => { finishResult = resolve; });
    const terminalArguments = terminal ? ['--terminal', String(terminal.columns), String(terminal.rows)] : [];
    const child = spawn(this.helper, ['--controller', String(process.pid), '--grace', String(grace), ...terminalArguments, '--', target.executable, ...target.prefix, ...invocation.argv], {
      cwd: invocation.cwd, env: invocation.environment, shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'],
    });
    // Only this object owns the helper channel. It is never rediscovered by PID.
    const unknown = (reason: string) => { verificationLost = true; claim = { kind: 'unknown', lease: id, reason }; };
    const settle = () => {
      if (!settled) {
        settled = true;
        inputWaiter?.reject(new Error('Ownership helper ended before stdin drain acknowledgement'));
        inputWaiter = undefined;
        resizeWaiter?.reject(new Error('Ownership helper ended before terminal resize acknowledgement'));
        resizeWaiter = undefined;
        finishResult(result ?? { kind: 'unknown', message: 'Supervisor ended before the command outcome could be observed' });
        finish(claim);
      }
    };
    let ready!: () => void;
    const started = new Promise<void>(resolve => { ready = resolve; });
    let stopPromise: Promise<OwnershipClaim> | undefined;
    let timer: NodeJS.Timeout | undefined;
    let diagnostics = '';
    child.stderr.setEncoding('utf8');
    child.stderr.on('data', value => { diagnostics = (diagnostics + value).slice(-4096); });
    child.on('error', error => { result = { kind: 'spawn-error', message: error.message }; unknown(`Ownership helper spawn failed: ${error.message}`); ready(); settle(); });
    child.stdin.on('error', () => { /* The exact helper close determines the claim. */ });
    const send = (kind: string, bytes?: Uint8Array) => new Promise<void>((resolve, reject) => {
      child.stdin.write(packet(kind, bytes), error => error ? reject(error) : resolve());
    });
    const sendInput = (bytes: Uint8Array) => new Promise<void>((resolve, reject) => {
      if (inputWaiter) { reject(new Error('Overlapping native stdin frames')); return; }
      const waiter = { resolve, reject }; inputWaiter = waiter;
      void send('I', bytes).catch(error => {
        if (inputWaiter === waiter) inputWaiter = undefined;
        reject(error);
      });
    });
    const read = (async () => {
      let buffered = Buffer.alloc(0);
      for await (const chunk of child.stdout) {
        buffered = Buffer.concat([buffered, chunk]);
        while (buffered.length >= 4) {
          const length = buffered.readUInt32LE();
          if (length < 1 || length > maximumFrame) throw new Error('Invalid native ownership frame');
          if (buffered.length < length + 4) break;
          const kind = String.fromCharCode(buffered[4]!);
          const bytes = buffered.subarray(5, length + 4);
          buffered = buffered.subarray(length + 4);
          if (kind === 'O') await streams.stdout(bytes);
          else if (kind === 'B') await streams.stderr(bytes);
          else if (kind === 'A') {
            if (bytes.length || !inputWaiter) throw new Error('Unexpected native stdin drain acknowledgement');
            const waiter = inputWaiter; inputWaiter = undefined; waiter.resolve();
          }
          else if (kind === 'Z') {
            if (bytes.length || !terminal || !resizeWaiter) throw new Error('Unexpected native terminal resize acknowledgement');
            const waiter = resizeWaiter; resizeWaiter = undefined; waiter.resolve();
          }
          else if (kind === 'R') {
            if (backend || nativeEmpty) throw new Error('Repeated native ownership readiness');
            backend = bytes.toString();
            if (backend !== expectedBackend) throw new Error('Unexpected native ownership backend');
            if (!verificationLost && !settled) claim = { kind: 'active', lease: id, backend }; ready();
          } else if (kind === 'F') {
            if (result) throw new Error('Repeated native process result');
            const text = bytes.toString();
            if (/^exit:\d+$/.test(text)) result = { kind: 'exit', code: Number(text.slice(5)) };
            else if (/^signal:\d+$/.test(text)) result = { kind: 'signal', signal: text.slice(7) };
            else if (text.startsWith('spawn-error:')) result = { kind: 'spawn-error', message: text.slice(12) };
            else throw new Error('Invalid native process result');
            finishResult(result);
          } else if (kind === 'D') {
            const name = bytes.toString();
            if (process.platform === 'darwin' || !result || nativeEmpty || name !== expectedBackend) throw new Error('Invalid native empty-ownership proof');
            backend = name; nativeEmpty = true;
          } else if (kind === 'U') {
            if (process.platform !== 'darwin') throw new Error('Unexpected best-effort ownership result');
            unknown(bytes.toString() || 'macOS complete descendant absence is unknown; retain scratch');
          } else if (kind === 'X') throw new Error(bytes.toString());
          else throw new Error('Unknown native ownership frame');
        }
      }
      if (buffered.length) throw new Error('Incomplete native ownership frame');
    })();
    void read.catch(error => { unknown(`Ownership verification failed: ${error.message}`); child.kill('SIGKILL'); });
    child.once('close', (code, signal) => {
      void read.then(() => {
        if (resizeWaiter) unknown(`Terminal ended before resize acknowledgement; retain scratch${claim.kind === 'unknown' ? `; ${claim.reason}` : ''}`);
        if (!settled && !verificationLost && code === 0 && !signal && nativeEmpty && result)
          claim = { kind: 'released', lease: id, backend, result };
        else if (!settled && !verificationLost) unknown(`No complete native absence proof (${signal ?? code}); ${diagnostics}`);
      }, () => {}).finally(() => { clearTimeout(timer); ready(); child.stdin.destroy(); settle(); });
    });
    const stop = () => {
      stopPromise ??= settled ? Promise.resolve(claim) : new Promise<OwnershipClaim>(resolve => {
        timer = setTimeout(() => {
          unknown('Cleanup deadline reached without native absence proof; retain scratch');
          child.kill('SIGKILL'); resolve(claim);
        }, deadline);
        void send('S').catch(() => {});
        void completed.then(resolve);
      });
      return stopPromise;
    };
    // EOF on this private control channel contains controller crashes. Target
    // stdin EOF is a separate frame and never releases kernel ownership.
    await send('L').catch(error => { unknown(error.message); child.kill('SIGKILL'); });
    const startupTimer = setTimeout(() => { unknown('Native launch readiness deadline reached; retain scratch'); child.kill('SIGKILL'); ready(); }, Math.max(10000, deadline));
    await started; clearTimeout(startupTimer);
    if (claim.kind === 'active') {
      void (async () => {
        for await (const chunk of streams.stdin) {
          if (settled) break;
          // A drained-frame acknowledgement bounds native buffering to 32KiB.
          // stop() writes S independently while this input producer is paused.
          for (let offset = 0; offset < chunk.byteLength && !settled; offset += 32768)
            await sendInput(chunk.subarray(offset, offset + 32768));
        }
        if (!settled) { if (terminal) void stop(); else await send('E'); }
      })().catch(() => { if (!settled) void stop(); });
    }
    let resizeTail = Promise.resolve();
    const resize = async (size: TerminalSize): Promise<void> => {
      if (!terminal) return Promise.reject(new Error('This lease has no terminal'));
      const bytes = dimensions(size);
      const operation = resizeTail.then(() => new Promise<void>((resolve, reject) => {
        if (settled || verificationLost || stopPromise || claim.kind !== 'active') { reject(new Error('Terminal is no longer actively owned')); return; }
        const resizeTimer = setTimeout(() => {
          if (resizeWaiter !== waiter) return;
          resizeWaiter = undefined;
          unknown('Terminal resize acknowledgement deadline reached; retain scratch');
          child.kill('SIGKILL'); reject(new Error('Terminal resize acknowledgement deadline reached'));
        }, deadline);
        const waiter = {
          resolve: () => { clearTimeout(resizeTimer); resolve(); },
          reject: (error: Error) => { clearTimeout(resizeTimer); reject(error); },
        };
        resizeWaiter = waiter;
        void send('W', bytes).catch(error => {
          if (resizeWaiter === waiter) resizeWaiter = undefined;
          unknown('Terminal resize control write failed; retain scratch'); child.kill('SIGKILL'); waiter.reject(error);
        });
      }));
      resizeTail = operation.catch(() => {});
      return operation;
    };
    return { id, claim: () => claim, result: commandResult, completed, stop, resize };
  }
}
