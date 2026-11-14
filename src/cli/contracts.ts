/** Migration inventory, not a replacement parser. public/gestalt owns dispatch until cutover. */
export const commandNames = [
  'install', 'agents', 'update', 'update-restart', 'cli', 'mobile', 'scratch',
  'serena', 'playwright', 'path', 'skills', 'skills-export', 'skills-import',
  'doctor', 'version', 'help',
] as const;
export const internalCommandNames = [
  '__updated-manager', '__update-restart-worker', '__codex-app-server',
  '__cli-owned', '__scratch-hook',
] as const;
export const commandAliases = { '--help': 'help', '-h': 'help', '--version': 'version', '-V': 'version' } as const;
export const retiredCommandNames = ['xerj'] as const;
export const updateComponents = ['all', 'gestalt', 'serena', 'playwright'] as const;
export const scratchEnvironmentNames = ['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'] as const;

export type CommandName = typeof commandNames[number];
export type TargetPlatform = 'win32' | 'darwin' | 'linux';
export type TargetArchitecture = 'x64' | 'arm64';
export type Environment = Readonly<Record<string, string | undefined>>;
export type Bytes = Uint8Array;

/** Caller passes tokens, never shell source. Diagnostics must stay off protocol stdout. */
export interface CommandInvocation {
  readonly argv: readonly string[];
  readonly cwd: string;
  readonly environment: Environment;
}
export interface CommandStreams {
  readonly stdin: AsyncIterable<Bytes>;
  stdout(bytes: Bytes): Promise<void>;
  stderr(bytes: Bytes): Promise<void>;
}
export type CommandHandler = (invocation: CommandInvocation, streams: CommandStreams) => Promise<number>;

/** Paths are target-native; implementations must reject escapes and redirected write targets. */
export interface PathPort {
  resolve(...parts: string[]): string;
  join(...parts: string[]): string;
  canonical(path: string): Promise<string>;
  assertContained(root: string, candidate: string): Promise<void>;
}
/** Resolution handles native binaries and npm shims without interpolating user arguments. */
export interface ExecutablePort {
  resolve(name: string, environment: Environment): Promise<string | null>;
}
export interface ProcessIdentity {
  readonly pid: number;
  /** OS creation identity; PID alone cannot authorize a signal or recovery. */
  readonly started: string;
  readonly executable: string;
}
export type ProcessResult =
  | { readonly kind: 'exit'; readonly code: number }
  | { readonly kind: 'signal'; readonly signal: string }
  | { readonly kind: 'spawn-error'; readonly message: string };
export interface ProcessPort {
  run(executable: string, invocation: CommandInvocation, streams: CommandStreams): Promise<ProcessResult>;
  identity(pid: number): Promise<ProcessIdentity | null>;
  signal(expected: ProcessIdentity, signal: 'interrupt' | 'terminate'): Promise<boolean>;
}
/** Fail closed if ownership/privacy cannot be proved, including Windows ACLs. */
export interface PrivateStoragePort {
  assertPrivate(path: string, kind: 'file' | 'directory'): Promise<void>;
  createDirectory(path: string): Promise<void>;
  /** Exclusive staged write + durable atomic replacement; no symlink/hardlink targets. */
  replaceFile(path: string, bytes: Bytes): Promise<void>;
  acquireLock(path: string): Promise<{ release(): Promise<void> }>;
}
export interface ArchiveMember {
  readonly path: string;
  readonly kind: 'file' | 'directory' | 'symlink' | 'hardlink';
}
export interface ArchivePort {
  list(archive: string): Promise<readonly ArchiveMember[]>;
  /** Validate all members before extraction; reject escapes, links and unexpected roots. */
  extract(archive: string, stagingDirectory: string, expectedRoot: string): Promise<void>;
  create(sourceDirectory: string, destination: string): Promise<void>;
}
export interface IpcPort {
  /** Private local endpoint: Unix socket or Windows named pipe; never implicit TCP. */
  connect(endpoint: string, expectedOwner: ProcessIdentity): Promise<CommandStreams>;
}
export interface PlatformPorts {
  readonly platform: TargetPlatform;
  readonly architecture: TargetArchitecture;
  readonly paths: PathPort;
  readonly executables: ExecutablePort;
  readonly processes: ProcessPort;
  readonly storage: PrivateStoragePort;
  readonly archives: ArchivePort;
  readonly ipc: IpcPort;
}
