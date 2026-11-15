import { homedir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { lstat, realpath } from 'node:fs/promises';
import type { CommandInvocation, CommandStreams, Environment, OwnedProcessLease, OwnedProcessPort, OwnedProcessResult, OwnershipClaim } from '../contracts.ts';
import { NativeExecutables } from '../platform/executables.ts';
import { NativeOwnedProcesses } from '../platform/processes.ts';
import { NativePaths, validateNativePath } from '../platform/paths.ts';
import { NativePrivateStorage } from '../platform/storage.ts';
import { borrowSetupScratch, releaseSetupScratch } from './scratch-context.mjs';

export class CommandFailure extends Error {
  readonly status: number;
  constructor(message: string, status = 1) { super(message); this.status = status; }
}
export const absent = (error: unknown): boolean => (error as NodeJS.ErrnoException).code === 'ENOENT';
export async function optionalStat(file: string) {
  try { return await lstat(file); } catch (error) { if (absent(error)) return null; throw error; }
}
export interface SetupPaths {
  readonly userHome: string;
  readonly codexHome: string;
  readonly gestaltHome: string;
  readonly marketplace: string;
  readonly marketplaceName: string;
  readonly manager: string;
  readonly unixProbe: string | undefined;
  readonly ownershipHelper?: string;
}
export async function setupPaths(environment: Environment): Promise<SetupPaths> {
  const userHome = await realpath(environment.HOME || environment.USERPROFILE || homedir());
  const native = new NativePaths();
  const managed = async (value: string, label: string) => {
    validateNativePath(value);
    if (!path.isAbsolute(value)) throw new Error(`${label} must be an absolute path: ${value}`);
    const result = await native.canonical(value);
    if (result === path.parse(result).root || result === userHome) throw new Error(`refusing unsafe ${label}: ${result}`);
    return result;
  };
  const codexHome = await managed(environment.CODEX_HOME || path.join(userHome, '.codex-gestalt'), 'CODEX_HOME');
  const gestaltHome = await managed(environment.GESTALT_HOME || path.join(userHome, '.gestalt'), 'GESTALT_HOME');
  for (const [left, right] of [[codexHome, gestaltHome], [gestaltHome, codexHome]] as const) {
    const relative = path.relative(left, right);
    if (relative === '' || (!path.isAbsolute(relative) && relative !== '..' && !relative.startsWith(`..${path.sep}`)))
      throw new Error('CODEX_HOME and GESTALT_HOME must be separate, non-nested directories');
  }
  const marketplaceName = environment.GESTALT_MARKETPLACE_NAME || 'dyne-gestalt-agents';
  if (!/^[A-Za-z0-9._+-]+$/.test(marketplaceName)) throw new Error('invalid marketplace name');
  const manager = fileURLToPath(import.meta.url);
  return { userHome, codexHome, gestaltHome, marketplaceName,
    marketplace: environment.GESTALT_MARKETPLACE || 'dyne/gestalt-agents', manager,
    unixProbe: path.join(path.dirname(manager), 'native', 'gestalt-unix-peer') };
}

/** A single foreground operation owns its children and forwards interruption
 * before rolling back manager state. Never launch a command through a shell. */
export class SetupContext {
  readonly environment: Environment;
  readonly invocation: CommandInvocation;
  readonly streams: CommandStreams;
  readonly paths: SetupPaths;
  readonly version: string;
  private readonly executables = new NativeExecutables();
  private readonly ownedProcesses: OwnedProcessPort | undefined;
  private interrupted = 0;
  private readonly active = new Set<OwnedProcessLease>();
  private readonly cancellation = new AbortController();
  private unsafeStatus = 0;
  readonly unknownOwnership: { claim: OwnershipClaim; result: OwnedProcessResult; borrower: unknown }[] = [];
  private readonly handlers = new Map<NodeJS.Signals, () => void>();
  constructor(invocation: CommandInvocation, streams: CommandStreams, paths: SetupPaths, version: string, ownedProcesses?: OwnedProcessPort) {
    this.invocation = invocation; this.streams = streams; this.paths = paths; this.version = version;
    this.ownedProcesses = ownedProcesses;
    this.environment = { ...invocation.environment, CODEX_HOME: paths.codexHome, GESTALT_HOME: paths.gestaltHome };
  }
  async log(message: string): Promise<void> { await this.streams.stderr(Buffer.from(`gestalt: ${message}\n`)); }
  canRollback(): boolean { return this.unsafeStatus === 0 && this.unknownOwnership.length === 0 && this.active.size === 0; }
  get signal(): AbortSignal { return this.cancellation.signal; }
  check(): void {
    if (this.unsafeStatus) throw new CommandFailure('setup subprocess absence is unconfirmed; configuration snapshots and locks must be retained', this.interrupted || this.unsafeStatus);
    if (this.interrupted) throw new CommandFailure('setup interrupted', this.interrupted);
  }
  listen(): () => void {
    for (const [signal, status] of [['SIGINT', 130], ['SIGTERM', 143], ['SIGHUP', 129]] as const) {
      const handler = () => {
        this.interrupted ||= status;
        this.cancellation.abort({ status: this.interrupted });
        void this.stopActive();
      };
      this.handlers.set(signal, handler); process.on(signal, handler);
    }
    return () => { for (const [signal, handler] of this.handlers) process.removeListener(signal, handler); };
  }
  async require(name: string): Promise<string> {
    const executable = await this.executables.resolve(name, this.environment);
    if (!executable) throw new Error(`${name} is required and was not found on PATH`);
    return executable;
  }
  async requireOwnership(): Promise<string> {
    const selected = this.paths.ownershipHelper || path.join(path.dirname(this.paths.manager), 'native',
      process.platform === 'win32' ? 'gestalt-owned-process.exe' : 'gestalt-owned-process');
    const helper = await this.executables.resolve(selected, this.environment);
    if (!helper) throw new Error(`target-built native process ownership helper is required: ${selected}; install the matching native Gestalt artifact`);
    return helper;
  }
  private async retainUnknown(claim: OwnershipClaim, result: OwnedProcessResult, borrower: unknown): Promise<void> {
    this.unknownOwnership.push({ claim, result, borrower });
    try {
      const storage = await this.storage(path.join(this.paths.gestaltHome, 'run/setup-claims'));
      await storage.replaceFile(path.join(storage.root, `${claim.lease}.json`), Buffer.from(JSON.stringify({
        schemaVersion: 1, scope: 'setup-process-ownership', claim, result, borrower, observedAt: new Date().toISOString(),
      }) + '\n'));
    } catch {
      await this.log('setup ownership is unknown; preserve any active borrower and transaction state (ownership report could not be written)');
    }
  }
  /** The canonical Agents API owns host filesystem transactions. Only its
   * selected external commands cross this borrowed native ownership boundary. */
  async managedCommands(): Promise<OwnedProcessPort & { readonly apiVersion: 1 }> {
    const port = this.ownedProcesses || new NativeOwnedProcesses(await this.requireOwnership());
    return { apiVersion: 1, launch: async (executable, invocation, streams, limits) => {
      this.check();
      const borrower = borrowSetupScratch(invocation.environment);
      let raw: OwnedProcessLease;
      try { raw = await port.launch(executable, invocation, streams, limits); }
      catch (error) { this.unsafeStatus = this.interrupted || 1; throw error; }
      let recorded: Promise<OwnershipClaim> | undefined;
      const finish = (claim: OwnershipClaim): Promise<OwnershipClaim> => recorded ??= (async () => {
        const result = await raw.result.catch((error: Error): OwnedProcessResult => ({ kind: 'unknown', message: error.message }));
        if (claim.kind === 'released' && claim.lease === raw.id) {
          if (borrower && !await releaseSetupScratch(borrower)) {
            this.unsafeStatus = this.interrupted || 1;
            claim = { kind: 'unknown', lease: raw.id, reason: 'setup borrower release is unconfirmed; owner remains protected' };
          }
        } else claim = { kind: 'unknown', lease: raw.id,
          reason: claim.kind === 'unknown' ? claim.reason : 'no matching native release proof' };
        if (claim.kind !== 'released') await this.retainUnknown(claim, result, borrower);
        this.active.delete(lease);
        return claim;
      })();
      const lease: OwnedProcessLease = { id: raw.id, claim: () => raw.claim(), result: raw.result,
        completed: raw.completed.then(finish, (error: Error) => finish({ kind: 'unknown', lease: raw.id, reason: error.message })),
        stop: async () => {
          try { return finish(await raw.stop()); }
          catch (error) { return finish({ kind: 'unknown', lease: raw.id, reason: String(error) }); }
        } };
      this.active.add(lease);
      if (this.interrupted) void lease.stop();
      return lease;
    } };
  }
  async stopActive(): Promise<void> {
    await Promise.allSettled([...this.active].map(lease => lease.stop()));
  }
  retainDelegatedState(): void { this.unsafeStatus ||= this.interrupted || 1; }
  async run(name: string, argv: readonly string[], options: { capture?: boolean; cwd?: string; environment?: Environment; program?: string } = {}): Promise<string> {
    this.check();
    const executable = await this.require(name), commands = await this.managedCommands();
    const environment = options.environment || this.environment;
    const chunks: Buffer[] = []; let bytes = 0;
    let outputFailure: unknown;
    let lease: OwnedProcessLease;
    try {
      lease = await commands.launch(executable,
        { argv, cwd: options.cwd || this.invocation.cwd, environment }, {
          stdin: (async function* () { if (options.program !== undefined) yield Buffer.from(options.program); })(),
          stdout: async chunk => {
            if (!options.capture) return this.streams.stdout(chunk);
            bytes += chunk.length;
            if (bytes > 16 * 1024 * 1024) {
              outputFailure = new Error('command output exceeds supported size');
              void this.stopActive();
            } else chunks.push(Buffer.from(chunk));
          }, stderr: chunk => this.streams.stderr(chunk),
        }, { graceMs: 1000, deadlineMs: 10_000 });
    } catch (error) {
      // A failed adapter call supplies no ownership proof. The borrower is kept.
      this.unsafeStatus = this.interrupted || 1;
      throw error;
    }
    if (this.interrupted || outputFailure) void lease.stop();
    const result = await lease.result;
    // Command outcome is separate from the proof that all owned writers have
    // stopped. stop() also bounds cleanup after a normally exited installer.
    const claim = await lease.stop();
    const status = result.kind === 'exit' ? result.code : result.kind === 'signal'
      ? (Number.isInteger(Number(result.signal)) ? 128 + Number(result.signal) : ({ SIGINT: 130, SIGTERM: 143, SIGHUP: 129 }[result.signal] ?? 143)) : 1;
    if (claim.kind !== 'released') {
      // macOS preserves successful command results with retained authority.
      if (process.platform !== 'darwin' || status !== 0 || this.interrupted || outputFailure || result.kind === 'unknown') {
        this.unsafeStatus = this.interrupted || status || 1;
        const prerequisite = process.platform === 'linux' && claim.kind === 'unknown' && /Namespace setup refused/.test(claim.reason)
          ? '; Linux setup requires a host profile permitting unprivileged user, mount and PID namespaces' : '';
        throw new CommandFailure(`native setup subprocess absence is unconfirmed: ${claim.kind === 'unknown' ? claim.reason : 'no release proof'}${prerequisite}; transaction state requires inspection`, this.unsafeStatus);
      }
    }
    this.check();
    if (outputFailure) throw outputFailure;
    if (status !== 0) throw new CommandFailure(`${name} failed (exit ${status})`, status);
    return Buffer.concat(chunks).toString('utf8');
  }
  async storage(root: string): Promise<NativePrivateStorage> {
    // Destination-local private roots; aliases are resolved before any write.
    const parent = path.dirname(root);
    if (!(await optionalStat(parent))) {
      const storage = await this.storage(parent);
      await storage.assertPrivate(storage.root, 'directory');
    }
    return NativePrivateStorage.open(root, this.paths.unixProbe);
  }
}
