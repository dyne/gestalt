/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

import type { DatabaseSync, StatementSync } from 'node:sqlite';

type TransactionDatabase = Pick<DatabaseSync, 'exec' | 'isTransaction'>;
type StatementDatabase = Pick<DatabaseSync, 'prepare'>;
type SynchronousResult<T> = T extends PromiseLike<unknown> ? never : T;

export function withImmediateTransaction<T>(
  db: TransactionDatabase,
  action: () => SynchronousResult<T>,
): SynchronousResult<T> {
  db.exec('BEGIN IMMEDIATE');
  try {
    const result = action();
    db.exec('COMMIT');
    return result;
  } catch (error) {
    if (db.isTransaction) {
      try {
        db.exec('ROLLBACK');
      } catch {}
    }
    throw error;
  }
}

/**
 * Auth has existing positional-parameter SQL, while SQLTagStore only accepts
 * tagged-template parameters. This small LRU keeps that SQL unchanged and only
 * receives repeated static statements from the authorization store.
 */
export class AuthStatementCache {
  private readonly statements = new Map<string, StatementSync>();

  constructor(
    private readonly db: StatementDatabase,
    private readonly maxSize = 32,
  ) {
    if (!Number.isInteger(maxSize) || maxSize < 1)
      throw new RangeError('Statement cache size must be a positive integer');
  }

  prepare(sql: string): StatementSync {
    const cached = this.statements.get(sql);
    if (cached) {
      this.statements.delete(sql);
      this.statements.set(sql, cached);
      return cached;
    }

    const statement = this.db.prepare(sql);
    if (this.statements.size === this.maxSize) {
      const oldest = this.statements.keys().next().value;
      if (oldest !== undefined) this.statements.delete(oldest);
    }
    this.statements.set(sql, statement);
    return statement;
  }
}
