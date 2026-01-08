// Package watcher provides filesystem event watching primitives used by Gestalt.
//
// The Watcher API is safe for concurrent use and delivers best-effort events:
// callers should assume events can be coalesced or dropped under load and use
// callbacks to trigger higher-level refreshes rather than rely on exact ordering.
package watcher
