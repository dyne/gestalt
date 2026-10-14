/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { JsonRpcClient } from './json-rpc-client.js';
import { profileAppServerCommand } from '../catalog/profile-command.js';

export type CodexProcess = {
  child: ChildProcessWithoutNullStreams;
  rpc: JsonRpcClient;
  close(): void;
  onExit(listener: () => void): () => void;
};

export function launchCodexAppServer(input: {
  profile: string;
  cwd: string;
  environment?: Readonly<Record<string, string>>;
}): CodexProcess {
  const launch = profileAppServerCommand();
  const child = spawn(launch.command, launch.args, {
    cwd: input.cwd,
    env: codexChildEnvironment(input.environment),
    shell: false,
    stdio: 'pipe',
  });
  const rpc = new JsonRpcClient(child.stdout, child.stdin);
  child.once('error', (error) => rpc.fail(error));
  return {
    child,
    rpc,
    close: () => child.kill('SIGTERM'),
    onExit: (listener) => {
      child.once('exit', listener);
      return () => child.off('exit', listener);
    },
  };
}

/** Prevents a relay's own ambient status path from reaching discovery-only children. */
export function codexChildEnvironment(
  environment?: Readonly<Record<string, string>>,
): NodeJS.ProcessEnv {
  const inherited = { ...process.env };
  delete inherited.GESTALT_MOBILE_ORG_PLAN_STATUS_FILE;
  delete inherited.GESTALT_MOBILE_ORG_PLAN_STATUS_DIRECTORY;
  delete inherited.GESTALT_MOBILE_ORG_PLAN_MEASUREMENT_URL;
  delete inherited.GESTALT_MOBILE_ORG_PLAN_MEASUREMENT_TOKEN;
  return { ...inherited, ...environment };
}
