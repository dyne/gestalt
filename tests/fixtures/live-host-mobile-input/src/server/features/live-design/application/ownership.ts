/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

import type { LiveAudience, LiveOwnershipReader } from './ports.js';

export type LiveState = 'starting' | 'active' | 'stopping' | 'error' | 'recoveryRequired' | 'idle';
export type AppIdentity = Readonly<{
  canonicalAppRoot: string;
  registeredPath: string;
  device: string;
  inode: string;
}>;
export type LiveFence = Readonly<{
  liveId: string;
  generation: number;
  controllerEpoch: number;
  revision: number;
}>;
export type LiveControlIntent = Readonly<{
  version: number;
  enabled: boolean;
  planIdentity: string | null;
}>;
export type LiveRun = LiveAudience &
  LiveFence & {
    version: 1;
    controllerId: string;
    rootThreadId: string;
    provider: 'codex';
    app: AppIdentity;
    targetId: string;
    targetIdentity: string;
    operationId: string;
    state: LiveState;
    phase: string;
    failureCode: string | null;
    priorControls: LiveControlIntent | null;
    controlsRestored: boolean;
    createdAt: string;
    updatedAt: string;
  };
export type LiveStartClaim = Omit<
  LiveRun,
  | keyof LiveFence
  | 'version'
  | 'controllerId'
  | 'state'
  | 'phase'
  | 'failureCode'
  | 'createdAt'
  | 'updatedAt'
  | 'priorControls'
  | 'controlsRestored'
>;
export type LiveMutation =
  | { event: 'phase'; phase: string }
  | { event: 'captureControls'; intent: LiveControlIntent }
  | { event: 'controlsRestored' }
  | { event: 'ready' }
  | { event: 'resume' }
  | { event: 'stop' }
  | { event: 'failed'; code: string }
  | { event: 'recover'; code: string }
  | { event: 'cleaned' };

export interface LiveOwnershipStore extends LiveOwnershipReader {
  retry(input: LiveStartClaim): LiveRun | null;
  claim(input: LiveStartClaim): { run: LiveRun; acquired: boolean };
  current(liveId: string): LiveRun | null;
  assert(fence: LiveFence): LiveRun;
  mutate(fence: LiveFence, mutation: LiveMutation): LiveRun;
  assertRestorable(fence: LiveFence): LiveRun;
}

export function nextLiveState(run: LiveRun, mutation: LiveMutation): LiveState {
  const allowed: Record<LiveMutation['event'], readonly LiveState[]> = {
    phase: ['starting', 'stopping'],
    captureControls: ['starting'],
    controlsRestored: ['idle'],
    ready: ['starting'],
    resume: ['stopping'],
    stop: ['starting', 'active', 'error', 'recoveryRequired'],
    failed: ['starting', 'active', 'stopping'],
    recover: ['starting', 'active', 'stopping', 'error', 'recoveryRequired'],
    cleaned: ['stopping'],
  };
  if (!allowed[mutation.event].includes(run.state)) throw new Error('LIVE_STATE_CONFLICT');
  if (mutation.event === 'ready' && run.phase !== 'route:ack')
    throw new Error('LIVE_STATE_CONFLICT');
  switch (mutation.event) {
    case 'captureControls':
      if (run.priorControls !== null) throw new Error('LIVE_STATE_CONFLICT');
      return run.state;
    case 'controlsRestored':
    case 'phase':
      return run.state;
    case 'ready':
      return 'active';
    case 'resume':
      return 'starting';
    case 'stop':
      return 'stopping';
    case 'failed':
      return 'error';
    case 'recover':
      return 'recoveryRequired';
    case 'cleaned':
      return 'idle';
  }
}
