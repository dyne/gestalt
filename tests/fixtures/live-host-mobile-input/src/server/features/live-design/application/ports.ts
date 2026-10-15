/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

export type LiveAudience = {
  relayId: string;
  appId: string;
  liveId: string;
  generation: number;
  previewOrigin: string;
  authSessionHash: string;
  deviceId: string;
};

/** Read from authoritative private ownership state, never project metadata or request URLs. */
export interface LiveOwnershipReader {
  read(relayId: string): (LiveAudience & { active: boolean }) | null;
}
export type LaunchGrant = LiveAudience & {
  grantId: string;
  tokenHash: string;
  codeChallenge: string;
  expiresAt: string;
};
export type PreviewLease = LiveAudience & {
  leaseId: string;
  tokenHash: string;
  exchangedAt: string;
  leaseExpiresAt: string;
  absoluteExpiresAt: string;
  renewedAt: string | null;
};
export type PreviewInstance = Pick<
  LiveAudience,
  'relayId' | 'appId' | 'liveId' | 'generation' | 'previewOrigin'
>;
export type PreviewRevocationScope =
  | { authSessionHash: string }
  | { deviceId: string }
  | { liveId: string }
  | { liveId: string; throughGeneration: number };
export interface PreviewRevocationBarrier {
  /** Persist the denial barrier first, then close matching owned connections before resolving. */
  revoke(scope: PreviewRevocationScope): Promise<void>;
}

export interface PreviewGrantStore {
  saveGrant(grant: LaunchGrant): boolean;
  /** Validation and consume/create run synchronously under the same exclusive transaction. */
  exchange(
    grantId: string,
    validate: (grant: LaunchGrant) => PreviewLease | null,
  ): PreviewLease | null;
  readLease(leaseId: string): PreviewLease | null;
  findLease(tokenHash: string): PreviewLease | null;
  listLeases(authSessionHash: string, relayId: string): readonly PreviewLease[];
  renew(leaseId: string, update: (lease: PreviewLease) => PreviewLease | null): PreviewLease | null;
  attempt(key: string, now: number): boolean;
  revoke(match: PreviewRevocationScope): void;
  close(): void;
}

export interface PreviewSecrets {
  token(): string;
  id(): string;
  hash(value: string): string;
  challenge(verifier: string): string;
  equal(left: string, right: string): boolean;
}
export interface PreviewAuthentication {
  identity(
    cookie: string | undefined,
    now: string,
  ): { authSessionHash: string; deviceId: string } | null;
  authorized(authSessionHash: string, deviceId: string, now: string): boolean;
}
export type PreviewGrantDependencies = {
  store: PreviewGrantStore;
  owners: LiveOwnershipReader;
  secrets: PreviewSecrets;
  authentication: PreviewAuthentication;
  now(): Date;
  mobileOrigin: string;
  revocations?: PreviewRevocationBarrier;
};
