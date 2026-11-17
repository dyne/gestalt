#!/usr/bin/env node
import type { Writable } from 'node:stream';
import { realpathSync } from 'node:fs';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { runCli } from './dispatch.ts';
import type { Bytes, CommandInvocation, CommandStreams, OwnedProcessPort } from './contracts.ts';
import { createSetupHandlers } from './setup/commands.ts';
import { createRuntimeHandlers } from './runtime/index.ts';
import { selectInteractiveRuntime } from './platform/interactive.ts';
export { scratchHookHandler as associateSessionScratch } from './runtime/scratch.ts';
export const SESSION_SCRATCH_API_VERSION = 1;

declare const GESTALT_BUILD_VERSION: string;
declare const GESTALT_BUILD_HELP: string;
export const UPDATED_MANAGER_API_VERSION = 1;
export const UPDATED_MANAGER_API_CAPABILITIES = ['host-filesystem-v1', 'async-owned-commands-v1', 'cancellation-v1'] as const;

/** Versioned host-side handoff. Filesystem orchestration never enters a mapped
 * user namespace; only the injected external command capability does. */
export async function continueUpdatedManager(invocation: CommandInvocation, streams: CommandStreams,
  options: { readonly commands: OwnedProcessPort & { readonly apiVersion: number }; readonly signal: AbortSignal }) {
  if (options.commands?.apiVersion !== 1 || typeof options.commands.launch !== 'function' ||
      !options.signal || invocation.argv[0] !== '__updated-manager') throw new Error('invalid managed updated-manager handoff');
  let unknown = false;
  const metadata = { version: GESTALT_BUILD_VERSION, help: GESTALT_BUILD_HELP };
  const status = await runCli({ argv: [...invocation.argv], cwd: invocation.cwd, environment: { ...invocation.environment } }, streams,
    metadata, createSetupHandlers(metadata, undefined, false, { commands: options.commands, signal: options.signal, unknown: () => { unknown = true; } }));
  return { apiVersion: UPDATED_MANAGER_API_VERSION, status, ownership: { kind: unknown ? 'unknown' : 'released' } };
}

function write(stream: Writable, bytes: Bytes): Promise<void> {
  return new Promise((resolve, reject) => {
    stream.write(bytes, error => error ? reject(error) : resolve());
  });
}

export async function main(): Promise<number> {
  // A direct Unix entry replaces itself before reading stdin or acquiring any
  // JS-owned resource. The unchanged legacy launcher retains its private-fd,
  // sandbox and cleanup contracts; imported continuations never call execve.
  if (process.platform === 'linux' || process.platform === 'darwin') {
    const selected = selectInteractiveRuntime({ legacyManager: fileURLToPath(new URL('./legacy/gestalt', import.meta.url)) });
    if (selected.kind === 'unix-compatibility') selected.exec({ argv: process.argv.slice(2), cwd: process.cwd(), environment: process.env });
  }
  const streams: CommandStreams = {
    stdin: process.stdin,
    stdout: bytes => write(process.stdout, bytes),
    stderr: bytes => write(process.stderr, bytes),
  };
  const metadata = { version: GESTALT_BUILD_VERSION, help: GESTALT_BUILD_HELP };
  return runCli({ argv: process.argv.slice(2), cwd: process.cwd(), environment: process.env }, streams,
    metadata, { ...createSetupHandlers(metadata), ...createRuntimeHandlers(metadata,
      process.stdin.isTTY && process.stdout.isTTY ? {
        size: () => ({ columns: process.stdout.columns || 80, rows: process.stdout.rows || 24 }),
        enter: () => {
          const wasRaw = process.stdin.isRaw;
          process.stdin.setRawMode(true);
          return () => { process.stdin.setRawMode(wasRaw); process.stdin.pause(); };
        },
        onResize: listener => { process.stdout.on('resize', listener); return () => { process.stdout.off('resize', listener); }; },
      } : undefined) });
}

// A digest-qualified import by an updating instance must have no CLI effects.
function invokedDirectly(): boolean {
  try { return Boolean(process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href); }
  catch { return false; }
}
if (invokedDirectly()) {
  main().then(code => { process.exitCode = code; }).catch(error => {
    console.error(`gestalt: ${error instanceof Error ? error.message : 'CLI failed'}`);
    process.exitCode = 1;
  });
}
