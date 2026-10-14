/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

import { spawn } from 'node:child_process';
import { constants, closeSync, fstatSync, lstatSync, openSync, realpathSync } from 'node:fs';
import { isAbsolute, join } from 'node:path';
import { SqliteLiveOwnership } from './sqlite-live-ownership.js';

/** Private controller construction only; election is not managed-runtime admission. */
export async function electLiveController(input: {
  privateDirectory: string;
  controllerId: string;
  initialize?: boolean;
}): Promise<{ ownership: SqliteLiveOwnership; close(): Promise<void> }> {
  const directory = input.privateDirectory;
  const stat = lstatSync(directory);
  if (
    process.platform !== 'linux' ||
    !isAbsolute(directory) ||
    realpathSync(directory) !== directory ||
    !stat.isDirectory() ||
    stat.uid !== process.getuid?.() ||
    (stat.mode & 0o777) !== 0o700
  )
    throw new Error('LIVE_PRIVATE_STATE_INVALID');

  // Never unlink the lock: two contenders must always elect against the same inode.
  // The OS releases it when the holder dies, including when its parent pipe closes.
  const path = join(directory, 'controller.lock');
  const descriptor = openSync(
    path,
    constants.O_CREAT | constants.O_RDWR | constants.O_NOFOLLOW,
    0o600,
  );
  try {
    const lock = fstatSync(descriptor);
    if (!lock.isFile() || lock.uid !== process.getuid?.() || (lock.mode & 0o777) !== 0o600)
      throw new Error('LIVE_PRIVATE_STATE_INVALID');
    const holder = spawn(
      '/usr/bin/flock',
      [
        '--nonblock',
        '--no-fork',
        '--conflict-exit-code',
        '75',
        '/proc/self/fd/3',
        process.execPath,
        '--input-type=module',
        '--eval',
        "process.stdout.write('elected\\n'); process.stdin.resume(); process.stdin.on('end', () => process.exit(0));",
      ],
      { stdio: ['pipe', 'pipe', 'ignore', descriptor], env: {} },
    );
    const stdin = holder.stdin!;
    const stdout = holder.stdout!;
    const exited = new Promise<void>((resolve) => holder.once('close', () => resolve()));
    const release = async () => {
      stdin.end();
      await exited;
    };
    let ownership: SqliteLiveOwnership | undefined;
    try {
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error('LIVE_CONTROLLER_UNAVAILABLE')), 5000);
        const finish = (error?: Error) => {
          clearTimeout(timer);
          stdout.off('data', ready);
          holder.off('error', failed);
          holder.off('exit', stopped);
          if (error) reject(error);
          else resolve();
        };
        let output = '';
        const ready = (chunk: Buffer) => {
          output += chunk.toString('utf8');
          if (output === 'elected\n') finish();
          else if (output.length >= 8) finish(new Error('LIVE_CONTROLLER_UNAVAILABLE'));
        };
        const failed = () => finish(new Error('LIVE_CONTROLLER_UNAVAILABLE'));
        const stopped = (code: number | null) =>
          finish(new Error(code === 75 ? 'LIVE_CONTROLLER_BUSY' : 'LIVE_CONTROLLER_UNAVAILABLE'));
        stdout.on('data', ready);
        holder.once('error', failed);
        holder.once('exit', stopped);
      });
      const current = lstatSync(path);
      if (current.dev !== lock.dev || current.ino !== lock.ino)
        throw new Error('LIVE_PRIVATE_STATE_INVALID');
      ownership = new SqliteLiveOwnership(join(directory, 'live.sqlite'), input.controllerId, {
        initialize: input.initialize,
      });
      ownership.recoverElectedController();
      // Unexpected holder loss must revoke the authority of every outstanding callback.
      const lost = () => {
        ownership?.close();
        ownership = undefined;
      };
      holder.once('exit', lost);
      let closed = false;
      return {
        ownership,
        async close() {
          if (closed) return;
          closed = true;
          holder.off('exit', lost);
          ownership?.close();
          ownership = undefined;
          await release();
        },
      };
    } catch (error) {
      ownership?.close();
      // Only this exact child is owned; no PID-based process adoption or signal.
      holder.kill();
      await exited;
      throw error;
    }
  } finally {
    closeSync(descriptor);
  }
}
