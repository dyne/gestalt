import { spawn } from 'node:child_process';
import { homedir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { lstat, realpath } from 'node:fs/promises';
import type { CommandInvocation, CommandStreams, Environment } from '../contracts.ts';
import { NativeExecutables, commandTarget } from '../platform/executables.ts';
import { NativePaths, validateNativePath } from '../platform/paths.ts';
import { NativePrivateStorage } from '../platform/storage.ts';

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
    unixProbe: environment.GESTALT_NATIVE_UNIX_PROBE || path.join(path.dirname(manager), 'native', 'gestalt-unix-peer') };
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
  private interrupted = 0;
  private active: ReturnType<typeof spawn> | null = null;
  private readonly handlers = new Map<NodeJS.Signals, () => void>();
  constructor(invocation: CommandInvocation, streams: CommandStreams, paths: SetupPaths, version: string) {
    this.invocation = invocation; this.streams = streams; this.paths = paths; this.version = version;
    this.environment = { ...invocation.environment, CODEX_HOME: paths.codexHome, GESTALT_HOME: paths.gestaltHome };
  }
  async log(message: string): Promise<void> { await this.streams.stderr(Buffer.from(`gestalt: ${message}\n`)); }
  check(): void { if (this.interrupted) throw new CommandFailure('setup interrupted; previous configuration restored', this.interrupted); }
  listen(): () => void {
    for (const [signal, status] of [['SIGINT', 130], ['SIGTERM', 143], ['SIGHUP', 129]] as const) {
      const handler = () => { this.interrupted ||= status; this.active?.kill(signal); };
      this.handlers.set(signal, handler); process.on(signal, handler);
    }
    return () => { for (const [signal, handler] of this.handlers) process.removeListener(signal, handler); };
  }
  async require(name: string): Promise<string> {
    const executable = await this.executables.resolve(name, this.environment);
    if (!executable) throw new Error(`${name} is required and was not found on PATH`);
    return executable;
  }
  async run(name: string, argv: readonly string[], options: { capture?: boolean; cwd?: string; environment?: Environment; program?: string } = {}): Promise<string> {
    this.check();
    const command = await commandTarget(await this.require(name));
    const child = spawn(command.executable, [...command.prefix, ...argv], {
      cwd: options.cwd || this.invocation.cwd, env: options.environment || this.environment,
      shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'],
    });
    this.active = child;
    child.stdin.on('error', () => {});
    child.stdin.end(options.program);
    const chunks: Buffer[] = []; let bytes = 0;
    let outputFailure: unknown;
    const copy = async (stream: AsyncIterable<Buffer>, write: (bytes: Buffer) => Promise<void>) => {
      try { for await (const chunk of stream) await write(chunk); }
      catch (error) { outputFailure = error; child.kill('SIGTERM'); }
    };
    const output = Promise.all([
      copy(child.stdout, async chunk => {
        if (!options.capture) return this.streams.stdout(chunk);
        bytes += chunk.length;
        if (bytes > 16 * 1024 * 1024) throw new Error('command output exceeds supported size');
        chunks.push(chunk);
      }),
      copy(child.stderr, chunk => this.streams.stderr(chunk)),
    ]);
    try {
      const status = await new Promise<number>((resolve, reject) => {
        child.once('error', reject);
        child.once('close', (code, signal) => resolve(code ?? (signal === 'SIGINT' ? 130 : signal === 'SIGHUP' ? 129 : 143)));
      });
      await output;
      this.check();
      if (outputFailure) throw outputFailure;
      if (status !== 0) throw new CommandFailure(`${name} failed (exit ${status})`, status);
      return Buffer.concat(chunks).toString('utf8');
    } finally { this.active = null; }
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
