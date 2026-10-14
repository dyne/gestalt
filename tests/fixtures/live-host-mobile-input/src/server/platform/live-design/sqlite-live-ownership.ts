/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

import { randomUUID } from 'node:crypto';
import { chmodSync, lstatSync, mkdirSync, realpathSync, statSync } from 'node:fs';
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { z } from 'zod';
import {
  nextLiveState,
  type AppIdentity,
  type LiveFence,
  type LiveMutation,
  type LiveOwnershipStore,
  type LiveRun,
  type LiveStartClaim,
} from '../../features/live-design/application/ownership.js';
import { withImmediateTransaction } from '../auth/sqlite.js';

const integer = z.number().int().positive().max(Number.MAX_SAFE_INTEGER);
const id = z.string().regex(/^[A-Za-z0-9_-]{1,128}$/);
const appSchema = z
  .object({
    canonicalAppRoot: z.string(),
    registeredPath: z.string(),
    device: z.string(),
    inode: z.string(),
  })
  .strict();
const runSchema = z
  .object({
    version: z.literal(1),
    revision: integer,
    liveId: id,
    generation: integer,
    controllerId: id,
    controllerEpoch: integer,
    relayId: id,
    rootThreadId: id,
    provider: z.literal('codex'),
    appId: id,
    app: appSchema,
    targetId: id,
    targetIdentity: z.string().min(1).max(256),
    previewOrigin: z.url(),
    authSessionHash: z.string().min(1).max(256),
    deviceId: id,
    operationId: id,
    state: z.enum(['starting', 'active', 'stopping', 'error', 'recoveryRequired', 'idle']),
    phase: z.string().regex(/^[a-zA-Z:]+$/),
    failureCode: z
      .string()
      .regex(/^LIVE_[A-Z_]+$/)
      .nullable(),
    priorControls: z
      .object({
        version: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
        enabled: z.boolean(),
        planIdentity: z.string().nullable(),
      })
      .strict()
      .nullable()
      .default(null),
    controlsRestored: z.boolean().default(false),
    createdAt: z.iso.datetime(),
    updatedAt: z.iso.datetime(),
  })
  .strict();

export function liveAppIdentity(path: string): AppIdentity {
  const canonicalAppRoot = realpathSync(path);
  const stat = statSync(canonicalAppRoot, { bigint: true });
  if (!stat.isDirectory()) throw new Error('LIVE_APP_INVALID');
  return {
    canonicalAppRoot,
    registeredPath: resolve(path),
    device: String(stat.dev),
    inode: String(stat.ino),
  };
}
export function revalidateLiveApp(app: AppIdentity): void {
  const current = liveAppIdentity(app.registeredPath);
  if (
    current.canonicalAppRoot !== app.canonicalAppRoot ||
    current.device !== app.device ||
    current.inode !== app.inode
  )
    throw new Error('LIVE_APP_IDENTITY_CHANGED');
}
function overlaps(a: string, b: string): boolean {
  const contains = (parent: string, child: string) => {
    const tail = relative(parent, child);
    return tail === '' || (tail !== '..' && !tail.startsWith(`..${sep}`) && !isAbsolute(tail));
  };
  return contains(a, b) || contains(b, a);
}
type WriterReservation = {
  id: string;
  relayId: string;
  scopes: readonly AppIdentity[] | null;
  auxiliary?: boolean;
};
// Controller-issued opaque IDs distinguish discovery's shared ordinary admission.
// Existing stores still see its unknown scope and therefore refuse every Live claim.
const auxiliaryPrefix = 'auxiliary-';
const nativeAuxiliaryId = 'relay-native-auxiliary';

/** Shared private controller DB; opening another connection never takes over or clears claims. */
export class SqliteLiveOwnership implements LiveOwnershipStore {
  private readonly db: DatabaseSync;
  private readonly privateDirectory: string;
  private admittedEpoch: number;
  constructor(
    path: string,
    private readonly controllerId: string,
    options: { initialize?: boolean } = {},
  ) {
    id.parse(controllerId);
    mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
    if (lstatSync(dirname(path)).isSymbolicLink()) throw new Error('LIVE_PRIVATE_STATE_INVALID');
    this.privateDirectory = realpathSync(dirname(path));
    chmodSync(this.privateDirectory, 0o700);
    let exists = true;
    try {
      if (!lstatSync(path).isFile()) throw new Error('LIVE_PRIVATE_STATE_INVALID');
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
      exists = false;
    }
    if (!exists && !options.initialize) throw new Error('LIVE_STATE_UNAVAILABLE');
    this.db = new DatabaseSync(path);
    try {
      chmodSync(path, 0o600);
      this.db.exec('PRAGMA busy_timeout=5000;');
      if (!exists)
        this.db.exec(`
        CREATE TABLE live_controller (singleton INTEGER PRIMARY KEY CHECK(singleton=1), version INTEGER NOT NULL, id TEXT NOT NULL, epoch INTEGER NOT NULL, generation INTEGER NOT NULL);
        CREATE TABLE live_runs (liveId TEXT PRIMARY KEY, relayId TEXT NOT NULL, appRoot TEXT NOT NULL, appIdentity TEXT NOT NULL, state TEXT NOT NULL, record TEXT NOT NULL);
        CREATE UNIQUE INDEX live_relay_claim ON live_runs(relayId) WHERE state!='idle';
        CREATE UNIQUE INDEX live_app_claim ON live_runs(appRoot) WHERE state!='idle';
        CREATE UNIQUE INDEX live_inode_claim ON live_runs(appIdentity) WHERE state!='idle';
        CREATE TABLE live_writers (id TEXT PRIMARY KEY, relayId TEXT UNIQUE NOT NULL, scopes TEXT);
      `);
      if (!exists)
        this.db.prepare('INSERT INTO live_controller VALUES (1,1,?,1,0)').run(controllerId);
      this.admittedEpoch = this.controller().epoch;
      this.list();
    } catch (error) {
      this.db.close();
      throw error;
    }
  }
  private controller(): { epoch: number; generation: number } {
    const row = this.db.prepare('SELECT * FROM live_controller WHERE singleton=1').get();
    if (
      !row ||
      row.version !== 1 ||
      row.id !== this.controllerId ||
      !integer.safeParse(row.epoch).success ||
      !z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER).safeParse(row.generation).success
    )
      throw new Error('LIVE_STATE_UNAVAILABLE');
    return { epoch: Number(row.epoch), generation: Number(row.generation) };
  }
  private assertController(): void {
    if (this.admittedEpoch !== this.controller().epoch) throw new Error('LIVE_GENERATION_STALE');
  }
  private decode(row: Record<string, unknown>): LiveRun {
    try {
      const run = runSchema.parse(JSON.parse(String(row.record)));
      if (
        run.liveId !== row.liveId ||
        run.relayId !== row.relayId ||
        run.app.canonicalAppRoot !== row.appRoot ||
        `${run.app.device}:${run.app.inode}` !== row.appIdentity ||
        run.state !== row.state ||
        run.controllerId !== this.controllerId
      )
        throw new Error();
      return Object.freeze({ ...run, app: Object.freeze(run.app) });
    } catch {
      throw new Error('LIVE_STATE_UNAVAILABLE');
    }
  }
  private list(): LiveRun[] {
    return this.db
      .prepare('SELECT * FROM live_runs')
      .all()
      .map((row) => this.decode(row));
  }
  current(liveId: string): LiveRun | null {
    this.controller();
    const row = this.db.prepare('SELECT * FROM live_runs WHERE liveId=?').get(liveId);
    return row ? this.decode(row) : null;
  }
  read(relayId: string): (LiveRun & { active: boolean }) | null {
    this.controller();
    const run = this.list().find((run) => run.relayId === relayId && run.state !== 'idle');
    if (!run) return null;
    revalidateLiveApp(run.app);
    return {
      ...run,
      active:
        run.state === 'active' &&
        run.controllerEpoch === this.controller().epoch &&
        run.controllerEpoch === this.admittedEpoch,
    };
  }
  assert(fence: LiveFence): LiveRun {
    this.assertController();
    const run = this.current(fence.liveId);
    if (
      !run ||
      run.generation !== fence.generation ||
      run.controllerEpoch !== fence.controllerEpoch ||
      run.revision !== fence.revision ||
      run.controllerEpoch !== this.controller().epoch
    )
      throw new Error('LIVE_GENERATION_STALE');
    return run;
  }
  private generation(): number {
    const generation = this.controller().generation + 1;
    integer.parse(generation);
    this.db.prepare('UPDATE live_controller SET generation=? WHERE singleton=1').run(generation);
    return generation;
  }
  retry(input: LiveStartClaim): LiveRun | null {
    this.assertController();
    revalidateLiveApp(input.app);
    const runs = this.list();
    const prior = runs.find(
      (run) => run.relayId === input.relayId && run.operationId === input.operationId,
    );
    if (prior) {
      if (
        prior.appId !== input.appId ||
        prior.targetId !== input.targetId ||
        prior.authSessionHash !== input.authSessionHash ||
        prior.deviceId !== input.deviceId ||
        prior.app.canonicalAppRoot !== input.app.canonicalAppRoot ||
        prior.app.device !== input.app.device ||
        prior.app.inode !== input.app.inode ||
        prior.rootThreadId !== input.rootThreadId ||
        prior.targetIdentity !== input.targetIdentity ||
        prior.previewOrigin !== input.previewOrigin
      )
        throw new Error('IDEMPOTENCY_KEY_REUSED');
      if (
        runs.some(
          (run) =>
            run.relayId === input.relayId && run.state !== 'idle' && run.liveId !== prior.liveId,
        )
      )
        throw new Error('LIVE_GENERATION_STALE');
      return prior;
    }
    return null;
  }
  claim(input: LiveStartClaim): { run: LiveRun; acquired: boolean } {
    if (input.provider !== 'codex') throw new Error('LIVE_PROVIDER_UNSUPPORTED');
    revalidateLiveApp(input.app);
    const privatePath = relative(input.app.canonicalAppRoot, this.privateDirectory);
    if (
      privatePath === '' ||
      (privatePath !== '..' && !privatePath.startsWith(`..${sep}`) && !isAbsolute(privatePath))
    )
      throw new Error('LIVE_PRIVATE_STATE_INVALID');
    return withImmediateTransaction(this.db, () => {
      const runs = this.list();
      const prior = this.retry(input);
      if (prior) return { run: prior, acquired: false };
      if (runs.some((run) => run.state !== 'idle' && run.relayId === input.relayId))
        throw new Error('LIVE_MODE_ACTIVE');
      if (
        runs.some(
          (run) =>
            run.state !== 'idle' &&
            (overlaps(run.app.canonicalAppRoot, input.app.canonicalAppRoot) ||
              (run.app.device === input.app.device && run.app.inode === input.app.inode)),
        )
      )
        throw new Error('LIVE_APP_BUSY');
      if (
        this.writers().some(
          (writer) =>
            writer.relayId === input.relayId ||
            writer.scopes === null ||
            writer.scopes.some((scope) =>
              overlaps(scope.canonicalAppRoot, input.app.canonicalAppRoot),
            ),
        )
      )
        throw new Error('LIVE_SESSION_BUSY');
      const now = new Date().toISOString();
      const run: LiveRun = {
        ...input,
        version: 1,
        revision: 1,
        liveId: randomUUID(),
        generation: this.generation(),
        controllerId: this.controllerId,
        controllerEpoch: this.controller().epoch,
        state: 'starting',
        phase: 'claim',
        failureCode: null,
        priorControls: null,
        controlsRestored: false,
        createdAt: now,
        updatedAt: now,
      };
      runSchema.parse(run);
      this.db
        .prepare('INSERT INTO live_runs VALUES (?,?,?,?,?,?)')
        .run(
          run.liveId,
          run.relayId,
          run.app.canonicalAppRoot,
          `${run.app.device}:${run.app.inode}`,
          run.state,
          JSON.stringify(run),
        );
      return { run: this.current(run.liveId)!, acquired: true };
    });
  }
  mutate(fence: LiveFence, mutation: LiveMutation): LiveRun {
    return withImmediateTransaction(this.db, () => {
      const run = this.assert(fence);
      const next: LiveRun = {
        ...run,
        priorControls: mutation.event === 'captureControls' ? mutation.intent : run.priorControls,
        controlsRestored: mutation.event === 'controlsRestored' ? true : run.controlsRestored,
        state: nextLiveState(run, mutation),
        revision: run.revision + 1,
        updatedAt: new Date().toISOString(),
        phase:
          mutation.event === 'phase'
            ? mutation.phase
            : mutation.event === 'captureControls' || mutation.event === 'controlsRestored'
              ? run.phase
              : mutation.event === 'recover' || mutation.event === 'failed'
                ? run.phase
                : mutation.event,
        failureCode:
          mutation.event === 'cleaned' || mutation.event === 'ready'
            ? null
            : 'code' in mutation
              ? mutation.code
              : run.failureCode,
        generation:
          mutation.event === 'recover' || mutation.event === 'resume'
            ? this.generation()
            : run.generation,
      };
      runSchema.parse(next);
      this.save(next);
      return this.current(next.liveId)!;
    });
  }
  private save(run: LiveRun): void {
    runSchema.parse(run);
    this.db
      .prepare('UPDATE live_runs SET state=?,record=? WHERE liveId=?')
      .run(run.state, JSON.stringify(run), run.liveId);
  }
  assertRestorable(fence: LiveFence): LiveRun {
    const run = this.assert(fence);
    if (run.state !== 'idle') throw new Error('LIVE_STATE_CONFLICT');
    if (
      this.list().some(
        (other) =>
          other.liveId !== run.liveId &&
          (other.generation > run.generation || other.state !== 'idle') &&
          (other.relayId === run.relayId ||
            overlaps(other.app.canonicalAppRoot, run.app.canonicalAppRoot) ||
            (other.app.device === run.app.device && other.app.inode === run.app.inode)),
      )
    )
      throw new Error('LIVE_GENERATION_STALE');
    return run;
  }
  /** Called only by the elected shared controller after external controller/process reconciliation. */
  recoverController(expectedEpoch: number): void {
    withImmediateTransaction(this.db, () => {
      this.assertController();
      if (this.controller().epoch !== expectedEpoch) throw new Error('LIVE_GENERATION_STALE');
      const epoch = expectedEpoch + 1;
      integer.parse(epoch);
      this.db.prepare('UPDATE live_controller SET epoch=? WHERE singleton=1').run(epoch);
      for (const run of this.list().filter(
        (run) => run.state !== 'idle' || (run.priorControls !== null && !run.controlsRestored),
      ))
        this.save({
          ...run,
          controllerEpoch: epoch,
          // Proven-clean pending restoration keeps its ordering relative to later claims.
          // The new epoch/revision still revokes every pre-takeover callback.
          generation: run.state === 'idle' ? run.generation : this.generation(),
          revision: run.revision + 1,
          state: run.state === 'idle' ? 'idle' : 'recoveryRequired',
          phase: run.phase,
          failureCode: run.state === 'idle' ? run.failureCode : 'LIVE_CONTROLLER_LOST',
          updatedAt: new Date().toISOString(),
        });
    });
    this.admittedEpoch = expectedEpoch + 1;
  }
  /** Call only after acquiring the OS controller election; existing claims stay held. */
  recoverElectedController(): void {
    this.recoverController(this.admittedEpoch);
  }
  private writerScopes(record: string): readonly AppIdentity[] | null {
    const scopes = z.array(appSchema).parse(JSON.parse(record));
    try {
      for (const scope of scopes) revalidateLiveApp(scope);
    } catch {
      return null;
    } // Changed/unknown effective permission scopes conservatively block every app.
    return scopes;
  }
  private writers(): WriterReservation[] {
    return this.db
      .prepare('SELECT * FROM live_writers')
      .all()
      .map((row) => ({
        id: String(row.id),
        relayId: String(row.relayId),
        scopes: row.scopes === null ? null : this.writerScopes(String(row.scopes)),
        // Preserve the earlier known controller catalog record without deleting
        // its unknown scope or pretending its processes have become quiet.
        auxiliary:
          row.scopes === null &&
          (String(row.id).startsWith(auxiliaryPrefix) || row.relayId === nativeAuxiliaryId),
      }));
  }
  checkOrdinary(relayId: string, effectiveScopes: readonly string[] | null): void {
    this.assertController();
    const scopes = effectiveScopes === null ? null : effectiveScopes.map(liveAppIdentity);
    if (
      this.list().some(
        (run) =>
          run.state !== 'idle' &&
          (run.relayId === relayId ||
            scopes === null ||
            scopes.some(
              (scope) =>
                overlaps(scope.canonicalAppRoot, run.app.canonicalAppRoot) ||
                (scope.device === run.app.device && scope.inode === run.app.inode),
            )),
      )
    )
      throw new Error('LIVE_MODE_ACTIVE');
  }
  assertWriter(reservation: WriterReservation, effectiveScopes?: readonly string[] | null): void {
    this.assertController();
    const current = this.writers().find(
      (writer) => writer.id === reservation.id && writer.relayId === reservation.relayId,
    );
    if (!current || JSON.stringify(current.scopes) !== JSON.stringify(reservation.scopes))
      throw new Error('LIVE_GENERATION_STALE');
    if (
      effectiveScopes !== undefined &&
      JSON.stringify(effectiveScopes?.map(liveAppIdentity) ?? null) !==
        JSON.stringify(current.scopes)
    )
      throw new Error('LIVE_SESSION_BUSY');
    this.checkOrdinary(
      reservation.relayId,
      current.scopes?.map((scope) => scope.registeredPath) ?? null,
    );
  }
  /** Trusted runtime's entire effective write scopes, not client-selected app paths. Null means unknown. */
  reserveWriter(relayId: string, effectiveScopes: readonly string[] | null): WriterReservation {
    id.parse(relayId);
    if (relayId === nativeAuxiliaryId) throw new Error('LIVE_SESSION_BUSY');
    const scopes =
      effectiveScopes === null ? null : effectiveScopes.map((scope) => liveAppIdentity(scope));
    if (scopes?.length === 0) throw new Error('LIVE_SESSION_BUSY');
    return withImmediateTransaction(this.db, () => {
      this.assertController();
      if (
        this.list().some(
          (run) =>
            run.state !== 'idle' &&
            (run.relayId === relayId ||
              scopes === null ||
              scopes.some((scope) => overlaps(scope.canonicalAppRoot, run.app.canonicalAppRoot))),
        )
      )
        throw new Error('LIVE_MODE_ACTIVE');
      if (
        this.writers().some(
          (writer) =>
            writer.relayId === relayId ||
            (!writer.auxiliary &&
              (writer.scopes === null ||
                scopes === null ||
                writer.scopes.some((existing) =>
                  scopes.some((scope) =>
                    overlaps(existing.canonicalAppRoot, scope.canonicalAppRoot),
                  ),
                ))),
        )
      )
        throw new Error('LIVE_SESSION_BUSY');
      const reservation = { id: randomUUID(), relayId, scopes, auxiliary: false };
      this.db
        .prepare('INSERT INTO live_writers VALUES (?,?,?)')
        .run(reservation.id, relayId, scopes === null ? null : JSON.stringify(scopes));
      return reservation;
    });
  }
  /** Shared controller discovery claim: coexists with ordinary writers, excludes ALL Live claims. */
  reserveAuxiliary(relayId: string): WriterReservation {
    id.parse(relayId);
    return withImmediateTransaction(this.db, () => {
      this.checkOrdinary(relayId, null);
      const existing = this.writers().find((writer) => writer.relayId === relayId);
      if (existing) {
        if (!existing.auxiliary || existing.scopes !== null) throw new Error('LIVE_SESSION_BUSY');
        return existing;
      }
      const reservation: WriterReservation = {
        id: `${auxiliaryPrefix}${randomUUID()}`,
        relayId,
        scopes: null,
        auxiliary: true,
      };
      this.db.prepare('INSERT INTO live_writers VALUES (?,?,NULL)').run(reservation.id, relayId);
      return reservation;
    });
  }
  /** Runtime may release only after its root/descendants/approvals/background commands are settled. */
  releaseQuiescentWriter(
    reservation: WriterReservation,
    outstanding: {
      roots: number;
      descendants: number;
      commands: number;
      approvals: number;
      unknown: boolean;
    },
  ): void {
    if (
      outstanding.unknown ||
      Object.entries(outstanding).some(([key, value]) => key !== 'unknown' && value !== 0)
    )
      throw new Error('LIVE_SESSION_BUSY');
    withImmediateTransaction(this.db, () => {
      this.assertController();
      const result = this.db
        .prepare('DELETE FROM live_writers WHERE id=? AND relayId=?')
        .run(reservation.id, reservation.relayId);
      if (result.changes !== 1) throw new Error('LIVE_GENERATION_STALE');
    });
  }
  close(): void {
    this.db.close();
  }
}
