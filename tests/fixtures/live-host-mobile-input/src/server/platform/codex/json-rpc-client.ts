/*
 * Copyright (C) 2026 Dyne.org foundation
 * Designed by Denis Roio <jaromil@dyne.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

import { createInterface } from 'node:readline';
import type { Readable, Writable } from 'node:stream';

export const CODEX_THREAD_NOT_FOUND = 'CODEX_THREAD_NOT_FOUND';
export const CODEX_JSON_RPC_ERROR = 'CODEX_JSON_RPC_ERROR';
export const CODEX_THREAD_WRITER_BUSY = 'CODEX_THREAD_WRITER_BUSY';

type CodexJsonRpcErrorKind =
  typeof CODEX_THREAD_NOT_FOUND | typeof CODEX_THREAD_WRITER_BUSY | typeof CODEX_JSON_RPC_ERROR;

/** A bounded representation of an app-server JSON-RPC failure. */
export class CodexJsonRpcError extends Error {
  constructor(
    readonly code: number | undefined,
    message: string,
    readonly kind: CodexJsonRpcErrorKind = classifyCodexJsonRpcError(code, message),
  ) {
    super(boundMessage(message));
    this.name = 'CodexJsonRpcError';
  }
}

/**
 * Codex 0.146 reports a rollout removed by an upgrade as -32600.  Keep the
 * message match deliberately narrow: other invalid-request responses must not
 * be treated as permission to replace a durable thread.
 */
export function isMissingCodexThreadRollout(error: unknown): boolean {
  return error instanceof CodexJsonRpcError && error.kind === CODEX_THREAD_NOT_FOUND;
}

/** Compatibility shim for the confirmed Codex 0.146 active-writer response. */
export function isCodexThreadWriterBusy(error: unknown): boolean {
  return error instanceof CodexJsonRpcError && error.kind === CODEX_THREAD_WRITER_BUSY;
}

function classifyCodexJsonRpcError(
  code: number | undefined,
  message: string,
): CodexJsonRpcErrorKind {
  if (code !== -32600) return CODEX_JSON_RPC_ERROR;
  if (/^no rollout found for thread id\b/i.test(message)) return CODEX_THREAD_NOT_FOUND;
  // Fixture-backed exact protocol wording: do not broaden this into a heuristic.
  if (/^thread .* already has an active writer$/i.test(message)) return CODEX_THREAD_WRITER_BUSY;
  return CODEX_JSON_RPC_ERROR;
}

function boundMessage(message: string): string {
  return message
    .replace(/((?:authorization|token|api[_ -]?key|password)\s*[:=]\s*)\S+/gi, '$1[REDACTED]')
    .replace(/(authentication failed:\s*).*/i, '$1[REDACTED]')
    .slice(0, 256);
}

export class JsonRpcClient {
  private sequence = 0;
  private readonly pending = new Map<
    number,
    { resolve(value: unknown): void; reject(reason: unknown): void }
  >();
  private readonly notifications = new Set<
    (notification: { method: string; params: unknown }) => void
  >();
  private readonly serverRequests = new Set<
    (request: { id: number; method: string; params: unknown }) => Promise<unknown> | unknown
  >();
  private readonly serverResponseSettlements = new Set<
    (settlement: { id: number; outcome: 'resultWritten' | 'errorWritten' | 'writeFailed' }) => void
  >();
  private failure: unknown = null;
  constructor(
    input: Readable,
    private readonly output: Writable,
  ) {
    createInterface({ input, crlfDelay: Infinity }).on('line', (line) => this.receive(line));
  }
  request(method: string, params: unknown): Promise<unknown> {
    if (this.failure) return Promise.reject(this.failure);
    const id = ++this.sequence;
    this.output.write(`${JSON.stringify({ jsonrpc: '2.0', id, method, params })}\n`);
    return new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }));
  }
  fail(error: unknown): void {
    if (this.failure) return;
    this.failure = error;
    for (const { reject } of this.pending.values()) reject(error);
    this.pending.clear();
  }
  onNotification(
    listener: (notification: { method: string; params: unknown }) => void,
  ): () => void {
    this.notifications.add(listener);
    return () => this.notifications.delete(listener);
  }
  onServerRequest(
    listener: (request: {
      id: number;
      method: string;
      params: unknown;
    }) => Promise<unknown> | unknown,
  ): () => void {
    this.serverRequests.add(listener);
    return () => this.serverRequests.delete(listener);
  }
  onServerResponseSettled(
    listener: (settlement: {
      id: number;
      outcome: 'resultWritten' | 'errorWritten' | 'writeFailed';
    }) => void,
  ): () => void {
    this.serverResponseSettlements.add(listener);
    return () => this.serverResponseSettlements.delete(listener);
  }
  private writeServerResponse(
    id: number,
    response: Readonly<{ result?: unknown; error?: unknown }>,
    outcome: 'resultWritten' | 'errorWritten',
  ): void {
    const line = `${JSON.stringify({ jsonrpc: '2.0', id, ...response })}\n`;
    try {
      this.output.write(line, (error) => {
        const settled = error ? 'writeFailed' : outcome;
        this.serverResponseSettlements.forEach((listener) => listener({ id, outcome: settled }));
      });
    } catch {
      this.serverResponseSettlements.forEach((listener) =>
        listener({ id, outcome: 'writeFailed' }),
      );
    }
  }
  private receive(line: string): void {
    try {
      const message = JSON.parse(line) as {
        id?: number;
        method?: string;
        params?: unknown;
        result?: unknown;
        error?: unknown;
      };
      if (message.id === undefined) {
        if (message.method)
          this.notifications.forEach((listener) =>
            listener({ method: message.method!, params: message.params }),
          );
        return;
      }
      if (message.method) {
        const listener = this.serverRequests.values().next().value as
          | ((request: {
              id: number;
              method: string;
              params: unknown;
            }) => Promise<unknown> | unknown)
          | undefined;
        if (!listener) return;
        void Promise.resolve(
          listener({ id: message.id, method: message.method, params: message.params }),
        )
          .then((result) => this.writeServerResponse(message.id!, { result }, 'resultWritten'))
          .catch((error: unknown) =>
            this.writeServerResponse(
              message.id!,
              { error: { message: error instanceof Error ? error.message : 'REQUEST_FAILED' } },
              'errorWritten',
            ),
          );
        return;
      }
      const pending = this.pending.get(message.id);
      if (!pending) return;
      this.pending.delete(message.id);
      if (message.error) {
        const error = message.error as { code?: unknown; message?: unknown };
        pending.reject(
          new CodexJsonRpcError(
            typeof error.code === 'number' ? error.code : undefined,
            typeof error.message === 'string' ? error.message : 'JSON_RPC_ERROR',
          ),
        );
      } else pending.resolve(message.result);
    } catch {
      /* malformed protocol messages are ignored at this boundary */
    }
  }
}
