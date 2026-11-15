import { Readable } from 'node:stream';

/** A lease borrows input; ending its wait must not destroy the caller's stream. */
export function borrowedProcessInput(input: AsyncIterable<Uint8Array>): {
  next: () => Promise<IteratorResult<Uint8Array>>;
  close: () => void;
} {
  let closed = false;
  let wake: (() => void) | undefined;
  if (input instanceof Readable) {
    let failure: Error | undefined;
    const changed = () => wake?.();
    const failed = (error: Error) => { failure = error; changed(); };
    input.on('readable', changed);
    input.on('end', changed);
    input.on('error', failed);
    input.on('close', changed);
    return {
      next: async () => {
        while (!closed) {
          // Install the wake before read(), which may synchronously update state.
          const available = new Promise<void>(resolve => { wake = resolve; });
          try {
            if (failure || input.errored) throw failure ?? input.errored;
            const chunk: Uint8Array | null = input.read();
            if (chunk !== null) return { done: false, value: chunk };
            if (input.readableEnded) return { done: true, value: undefined };
            if (input.destroyed) throw new Error('Borrowed process input closed before EOF');
            await available;
          } finally { wake = undefined; }
        }
        return { done: true, value: undefined };
      },
      close: () => {
        if (closed) return;
        closed = true;
        input.off('readable', changed);
        input.off('end', changed);
        input.off('error', failed);
        input.off('close', changed);
        input.pause();
        changed();
      },
    };
  }
  const iterator = input[Symbol.asyncIterator]();
  const cancelled = new Promise<IteratorReturnResult<undefined>>(resolve => {
    wake = () => resolve({ done: true, value: undefined });
  });
  let ended = false;
  return {
    next: async () => {
      if (closed || ended) return { done: true, value: undefined };
      const result = await Promise.race([iterator.next(), cancelled]);
      if (closed) return { done: true, value: undefined };
      ended = Boolean(result.done);
      return result;
    },
    close: () => {
      if (closed) return;
      closed = true; wake?.(); wake = undefined;
      // Generic iterables must cooperate with return() to cancel their own I/O.
      // Never let an unbounded foreign return hold native settlement open.
      if (!ended) {
        try { void Promise.resolve(iterator.return?.()).catch(() => {}); }
        catch { /* A failed foreign return cannot change the native claim. */ }
      }
    },
  };
}
