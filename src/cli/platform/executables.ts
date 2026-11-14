import { constants } from 'node:fs';
import { access, readFile, realpath, stat } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import path from 'node:path';
import { pipeline } from 'node:stream/promises';
import { Readable, Writable } from 'node:stream';
import type { CommandInvocation, CommandStreams, Environment, ExecutablePort, ProcessResult } from '../contracts.ts';

function environmentValue(environment: Environment, name: string): string | undefined {
  const matches = Object.keys(environment).filter(key => process.platform === 'win32' ? key.toUpperCase() === name : key === name);
  if (matches.length > 1) throw new Error(`Ambiguous case aliases for ${name}`);
  const key = matches[0];
  return key ? environment[key] : undefined;
}

export class NativeExecutables implements ExecutablePort {
  async resolve(name: string, environment: Environment): Promise<string | null> {
    if (!name || name.includes('\0')) throw new Error('Invalid executable name');
    const explicit = /[\\/]/.test(name) || path.isAbsolute(name);
    if (explicit && !path.isAbsolute(name)) throw new Error('Explicit executable path must be absolute');
    const extensions = process.platform === 'win32' && !path.extname(name)
      ? (environmentValue(environment, 'PATHEXT') ?? '.COM;.EXE;.BAT;.CMD').split(';').filter(ext => /^\.[a-z0-9]+$/i.test(ext)) : [''];
    const directories = explicit ? [''] : (environmentValue(environment, 'PATH') ?? '').split(path.delimiter)
      .map(value => value.replace(/^"(.*)"$/, '$1')).filter(value => path.isAbsolute(value));
    for (const directory of directories) for (const extension of extensions) {
      const candidate = explicit ? name + extension : path.join(directory, name + extension);
      try {
        if (!(await stat(candidate)).isFile()) continue;
        await access(candidate, process.platform === 'win32' ? constants.F_OK : constants.X_OK);
        return await realpath(candidate);
      } catch (error) {
        if (!['ENOENT', 'ENOTDIR', 'EACCES'].includes((error as NodeJS.ErrnoException).code ?? '')) throw error;
      }
    }
    return null;
  }
}

/** Recognize the complete npm cmd-shim Node template; never evaluate batch source. */
async function commandTarget(executable: string): Promise<{ executable: string; prefix: string[] }> {
  if (!/\.(cmd|bat)$/i.test(executable)) return { executable, prefix: [] };
  const source = (await readFile(executable, 'utf8')).replace(/\r\n/g, '\n');
  const template = /^@ECHO off\nGOTO start\n:find_dp0\nSET dp0=%~dp0\nEXIT \/b\n:start\nSETLOCAL\nCALL :find_dp0\n\nIF EXIST "%dp0%\\node\.exe" \(\n  SET "_prog=%dp0%\\node\.exe"\n\) ELSE \(\n  SET "_prog=node"\n\)\n\nendLocal & goto #_undefined_# 2>NUL \|\| title %COMSPEC% & set PATHEXT=%PATHEXT:;\.JS;=;% & "%_prog%"  "%dp0%\\([^"\n%]+)" %\*\n$/;
  const match = source.match(template);
  if (!match?.[1]) throw new Error('Unsupported batch shim; a standard npm Node shim is required');
  const target = await realpath(path.resolve(path.dirname(executable), match[1].split('\\').join(path.sep)));
  const firstLine = (await readFile(target, 'utf8')).split(/\r?\n/, 1)[0];
  if (!/^#!\s*(?:\/usr\/bin\/env\s+)?node\s*$/.test(firstLine ?? '')) throw new Error('npm shim target is not a plain Node entry');
  return { executable: process.execPath, prefix: [target] };
}

/** Stream forwarding only. Complete process ownership is supplied by the owned-process adapter. */
export async function forwardCommand(executable: string, invocation: CommandInvocation, streams: CommandStreams): Promise<ProcessResult> {
  const target = await commandTarget(executable);
  const child = spawn(target.executable, [...target.prefix, ...invocation.argv], {
    cwd: invocation.cwd, env: invocation.environment, shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'],
  });
  const result = new Promise<ProcessResult>(resolve => {
    child.once('error', error => resolve({ kind: 'spawn-error', message: error.message }));
    child.once('close', (code, signal) => resolve(signal ? { kind: 'signal', signal } : { kind: 'exit', code: code ?? 1 }));
  });
  const sink = (write: (bytes: Uint8Array) => Promise<void>) => new Writable({ write(chunk, _encoding, done) { write(chunk).then(() => done(), done); } });
  let inputError: unknown;
  void pipeline(Readable.from(streams.stdin), child.stdin).catch(error => {
    if (!['EPIPE', 'ERR_STREAM_PREMATURE_CLOSE', 'ERR_STREAM_DESTROYED'].includes(error.code)) inputError = error;
  });
  const output = Promise.all([pipeline(child.stdout, sink(bytes => streams.stdout(bytes))), pipeline(child.stderr, sink(bytes => streams.stderr(bytes)))]);
  const settled = await result;
  // A child may close before consuming an unbounded caller input.
  child.stdin.destroy();
  await output;
  if (inputError) throw inputError;
  return settled;
}
