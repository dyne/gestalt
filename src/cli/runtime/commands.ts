import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { NativeExecutables } from '../platform/executables.ts';
import { NativeOwnedProcesses } from '../platform/processes.ts';
import type { CommandInvocation, CommandStreams, Environment, OwnedProcessLease, OwnedProcessPort, OwnershipClaim, OwnedTerminalLease, OwnedTerminalPort, TerminalSize } from '../contracts.ts';
import { scratchEnvironment, scratchContext, scratchBorrow, scratchReleaseReady } from './scratch-runtime.mjs';

export const resultStatus = (result: Awaited<OwnedProcessLease['result']>): number => result.kind === 'exit' ? result.code :
  result.kind === 'signal' ? (Number.isInteger(Number(result.signal)) ? 128 + Number(result.signal) :
    ({ SIGINT: 130, SIGTERM: 143, SIGHUP: 129 }[result.signal] ?? 143)) : 1;

export interface RuntimeTerminal {
  size(): TerminalSize;
  enter(): () => void;
  onResize(listener: () => void): () => void;
}

/** Commands keep the host's filesystem identity. Only selected external leaves
 * enter the native ownership object; a command outcome never certifies release. */
export class RuntimeCommands {
  readonly active = new Set<OwnedProcessLease>();
  readonly unknown: OwnershipClaim[] = [];
  private readonly resolver = new NativeExecutables();
  private readonly listeners = new Map<NodeJS.Signals, () => void>();
  interrupted = 0;
  readonly invocation: CommandInvocation;
  readonly streams: CommandStreams;
  readonly manager: string;
  readonly terminal: RuntimeTerminal | undefined;
  private readonly injected: OwnedProcessPort | undefined;
  constructor(invocation: CommandInvocation, streams: CommandStreams, manager: string, injected?: OwnedProcessPort,
    terminal?: RuntimeTerminal) {
    this.invocation = invocation; this.streams = streams; this.manager = manager; this.injected = injected; this.terminal = terminal;
  }
  async resolve(name: string): Promise<string | null> { return this.resolver.resolve(name, this.invocation.environment); }
  async launch(name: string, argv: readonly string[], streams: CommandStreams = this.streams,
    environment: Environment = this.invocation.environment, cwd = this.invocation.cwd): Promise<OwnedProcessLease> {
    return this.launchCommand(name, argv, streams, environment, cwd);
  }
  async launchTerminal(name: string, argv: readonly string[], environment: Environment = this.invocation.environment,
    cwd = this.invocation.cwd): Promise<OwnedTerminalLease> {
    if (!this.terminal) throw new Error('Interactive Codex requires a terminal on stdin and stdout');
    const restore = this.terminal.enter();
    let unresize = () => {};
    try {
      const lease = await this.launchCommand(name, argv, this.streams, environment, cwd, this.terminal.size()) as OwnedTerminalLease;
      unresize = this.terminal.onResize(() => { void lease.resize(this.terminal!.size()).catch(() => lease.stop()); });
      void lease.completed.finally(() => { unresize(); restore(); });
      return lease;
    } catch (error) { unresize(); restore(); throw error; }
  }
  private async launchCommand(name: string, argv: readonly string[], streams: CommandStreams,
    environment: Environment, cwd: string, terminal?: TerminalSize): Promise<OwnedProcessLease> {
    if (this.interrupted) throw new Error('runtime interrupted before command launch');
    const executable = await this.resolve(name);
    if (!executable) throw new Error(`${name} is required and was not found on PATH`);
    const helper = path.join(path.dirname(this.manager), 'native', process.platform === 'win32' ? 'gestalt-owned-process.exe' : 'gestalt-owned-process');
    if (!this.injected && !await this.resolve(helper)) throw new Error(`target-built native process ownership helper is required: ${helper}; install the matching native Gestalt artifact`);
    const owner = scratchContext(environment), borrower = owner ? scratchBorrow(owner, 'manager-runtime') : null;
    let raw: OwnedProcessLease;
    try {
      const port: OwnedProcessPort & Partial<OwnedTerminalPort> = this.injected || new NativeOwnedProcesses(helper), invocation = { argv, cwd, environment };
      if (terminal) {
        if (typeof port.launchTerminal !== 'function') throw new Error('The selected command owner does not support an owned terminal');
        raw = await port.launchTerminal(executable, invocation,
          { input: streams.stdin, output: streams.stdout }, terminal, { graceMs: 1000, deadlineMs: 10000 });
      } else raw = await port.launch(executable, invocation, streams, { graceMs: 1000, deadlineMs: 10000 });
    }
    catch (error) {
      this.unknown.push({ kind: 'unknown', lease: randomUUID(), reason: `native launch supplied no release proof: ${String(error)}` });
      throw error;
    }
    let settled: Promise<OwnershipClaim> | undefined;
    const record = (claim: OwnershipClaim) => settled ??= (async () => {
      this.active.delete(lease);
      try {
        if (claim.kind === 'released' && claim.lease === raw.id && borrower && !await scratchReleaseReady(borrower))
          claim = { kind: 'unknown', lease: raw.id, reason: 'Runtime borrower release is unconfirmed; owner remains protected' };
      } catch (error) { claim = { kind: 'unknown', lease: raw.id, reason: `Runtime borrower metadata could not be released: ${String(error)}` }; }
      if (claim.kind !== 'released' || claim.lease !== raw.id) {
        claim = { kind: 'unknown', lease: raw.id, reason: claim.kind === 'unknown' ? claim.reason : 'No matching native release proof' };
        this.unknown.push(claim);
      }
      return claim;
    })();
    const failed = (error: unknown) => record({ kind: 'unknown', lease: raw.id, reason: String(error) });
    const lease: OwnedProcessLease = { id: raw.id, result: raw.result, claim: () => raw.claim(),
      completed: raw.completed.then(record, failed), stop: async () => { try { return record(await raw.stop()); } catch (error) { return failed(error); } } };
    if (terminal) Object.assign(lease, { resize: (size: TerminalSize) => (raw as OwnedTerminalLease).resize(size) });
    this.active.add(lease);
    if (this.interrupted) void lease.stop();
    return lease;
  }
  listen(): () => void {
    for (const [signal, status] of [['SIGINT', 130], ['SIGTERM', 143], ['SIGHUP', 129]] as const) {
      const listener = () => { this.interrupted ||= status; void this.stop(); };
      this.listeners.set(signal, listener); process.on(signal, listener);
    }
    return () => { for (const [signal, listener] of this.listeners) process.removeListener(signal, listener); this.listeners.clear(); };
  }
  async stop(): Promise<void> { await Promise.allSettled([...this.active].map(lease => lease.stop())); }
  async capture(name: string, argv: readonly string[], environment: Environment = diagnosticEnvironment(this.invocation.environment),
    limit = 1024 * 1024, timeout = 10000): Promise<{ status: number; stdout: string; stderr: string; claim: OwnershipClaim }> {
    const output: Buffer[] = [], errors: Buffer[] = []; let bytes = 0, exceeded = false;
    let lease: OwnedProcessLease | undefined;
    const receive = (target: Buffer[]) => async (chunk: Uint8Array) => {
      bytes += chunk.length;
      if (bytes > limit) { exceeded = true; void lease?.stop(); } else target.push(Buffer.from(chunk));
    };
    lease = await this.launch(name, argv, { stdin: (async function* () {})(), stdout: receive(output), stderr: receive(errors) }, environment);
    const timer = setTimeout(() => { exceeded = true; void lease?.stop(); }, timeout);
    try {
      const result = await lease.result, claim = await lease.stop();
      return { status: this.interrupted || (exceeded ? 1 : resultStatus(result)), stdout: Buffer.concat(output).toString('utf8'),
        stderr: Buffer.concat(errors).toString('utf8'), claim };
    } finally { clearTimeout(timer); }
  }
}

/** Validate and preserve the caller's temporary namespace. Read-only diagnostic
 * handlers must not launch commands or publish borrowers; an explicit runtime
 * operation uses the same validated owner rather than an ambient-TMP fallback. */
export function diagnosticEnvironment(environment: Environment): Environment {
  return scratchEnvironment(environment);
}
