import { constants, accessSync, statSync } from 'node:fs';
import path from 'node:path';
import type { CommandInvocation, OwnedProcessPort, OwnedTerminalPort } from '../contracts.ts';
import { NativeOwnedProcesses } from './processes.ts';

export type InteractiveBackend = 'windows-owned' | 'unix-compatibility';
export function interactiveBackendFor(platform: NodeJS.Platform): InteractiveBackend {
  if (platform === 'win32') return 'windows-owned';
  if (platform === 'linux' || platform === 'darwin') return 'unix-compatibility';
  throw new Error('Interactive CLI execution is unavailable on this platform');
}

export type InteractiveRuntime =
  | { readonly kind: 'windows-owned'; readonly owned: OwnedProcessPort & OwnedTerminalPort }
  | { readonly kind: 'unix-compatibility'; readonly legacyEntry: string; readonly exec: (invocation: CommandInvocation) => never };

/** Select at the CLI composition boundary, before acquiring JS resources.
 * Unix delegates to the existing packaged manager, not to a namespace helper.
 * Compatibility is process replacement; it returns no ownership/release claim.
 */
export function selectInteractiveRuntime(artifacts: {
  readonly legacyManager?: string;
  readonly ownedHelper?: string;
}): InteractiveRuntime {
  if (interactiveBackendFor(process.platform) === 'windows-owned') {
    if (!artifacts.ownedHelper) throw new Error('The matching Windows ownership helper is required');
    return { kind: 'windows-owned', owned: new NativeOwnedProcesses(artifacts.ownedHelper) };
  }
  const manager = artifacts.legacyManager;
  if (!manager || !path.isAbsolute(manager)) throw new Error('The packaged Unix compatibility manager must be absolute');
  return { kind: 'unix-compatibility', legacyEntry: manager, exec: invocation => {
    if (interactiveBackendFor(process.platform) !== 'unix-compatibility') throw new Error('Unix compatibility cannot run on Windows');
    if (typeof process.execve !== 'function') throw new Error('This Node runtime lacks Unix process replacement; use the existing Unix Gestalt entry');
    if (!path.isAbsolute(invocation.cwd)) throw new Error('Unix compatibility cwd must be absolute');
    if (!statSync(manager).isFile()) throw new Error('Unix compatibility manager must be a regular executable file');
    accessSync(manager, constants.X_OK);
    const environment: NodeJS.ProcessEnv = {};
    for (const [key, value] of Object.entries(invocation.environment)) if (value !== undefined) environment[key] = value;
    const previous = process.cwd();
    try {
      process.chdir(invocation.cwd);
      // Same UID/PID namespace, inherited terminal and caller environment.
      // No JS exit/finally callback runs on success; legacy lifecycle owns it.
      return process.execve(manager, [manager, ...invocation.argv], environment);
    } catch (error) { process.chdir(previous); throw error; }
  } };
}
