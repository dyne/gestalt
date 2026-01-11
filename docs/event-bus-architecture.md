# Event Bus Architecture (Design)

This document defines the unified event bus architecture for Gestalt. It is a
design reference for the implementation steps that follow.

## Goals

- Provide a single, reusable fan-out primitive for events and streams.
- Keep the bus passive: no internal worker goroutines, synchronous Publish.
- Make the API type-safe and easy to test.
- Support filtered subscriptions and backpressure policies.
- Integrate with metrics without coupling callers to metrics internals.

## Non-goals (deferred)

- Event persistence or replay (planned in a later step).
- Priority scheduling between subscribers.
- Delivery guarantees across process boundaries.

## Core API sketch

The bus is a concrete struct in `internal/event` (not an interface).

```go
package event

type Event interface {
	Type() string
	Timestamp() time.Time
}

type BusOptions struct {
	Name                   string
	SubscriberBufferSize   int
	BlockOnFull            bool
	WriteTimeout           time.Duration
	MaxSubscribers         int
	SlowSubscriberThreshold time.Duration
}

type Bus[T any] struct {
	// internal: subscriber map, options, metrics handle, closed state
}

func NewBus[T any](ctx context.Context, opts BusOptions) *Bus[T]

func (b *Bus[T]) Subscribe() (<-chan T, func())
func (b *Bus[T]) SubscribeFiltered(filter func(T) bool) (<-chan T, func())
func (b *Bus[T]) Publish(event T)
func (b *Bus[T]) Close()
```

### Event-specific helpers

For `T` that satisfies `event.Event`, provide convenience helpers:

- `SubscribeType(eventType string)`
- `SubscribeTypes(eventTypes ...string)`

These helpers are thin wrappers around `SubscribeFiltered`.

## Concurrency and lifecycle

- `Subscribe`, `Publish`, `Close` are safe to call concurrently.
- `Publish` is synchronous fan-out. No goroutines are spawned per publish.
- Callers can publish asynchronously by calling `Publish` in their own goroutine.
- `NewBus` accepts a context; if provided, it starts a tiny goroutine to call
  `Close()` when `ctx.Done()` fires. This is the only internal goroutine.
- After `Close`, all subscriber channels are closed and future `Subscribe`
  calls return a closed channel and a no-op cancel function.

## Backpressure and delivery

The bus supports two delivery policies:

1. **Drop mode** (`BlockOnFull=false`): non-blocking send. If a subscriber's
   channel buffer is full, the event is dropped for that subscriber.
2. **Block mode** (`BlockOnFull=true`): send blocks up to `WriteTimeout`.
   On timeout, the subscriber is treated as unhealthy and is removed; the
   event is still delivered to remaining subscribers.

Defaults:

- `SubscriberBufferSize`: 128.
- `BlockOnFull`: false.
- `WriteTimeout`: 0 (ignored when `BlockOnFull=false`).
- `SlowSubscriberThreshold`: optional. If set, log warnings when a send blocks
  longer than the threshold.

### Bus policies per use case

- **Filesystem/config/agent/workflow events**: drop mode; losing some events is
  acceptable and avoids head-of-line blocking.
- **Logs**: block mode with short timeout (e.g., 100ms). Do not drop log
  entries; remove slow subscribers on timeout.
- **Terminal output stream**: block mode with a high timeout (3m) and a larger
  buffer (256+). Preserve stream ordering. Retain slow-subscriber warnings.

## Filtering

- `SubscribeFiltered` attaches a filter function to a subscription.
- If the filter returns false, the event is not sent and does not consume buffer.
- Filter panics are recovered; the subscriber is removed and a warning is logged.

## Error handling

- `Publish` treats zero-length payloads for byte slices as no-ops.
- If a subscriber closes its channel, `Publish` recovers from the send panic,
  removes the subscriber, and continues.
- `Publish` does not return errors; metrics and logs capture drop/timeout events.

## Metrics

Each bus reports to `metrics.Registry` using its `Name`:

- `events_published_total`
- `events_dropped_total`
- `subscribers_active`

For buses carrying `event.Event`, also track counts per `event.Type()`.

## Usage guidelines

Use the event bus when:

- You need fan-out to multiple independent consumers.
- You want to decouple producers and consumers.
- You can tolerate async delivery or per-subscriber drop behavior.

Use direct function calls when:

- The caller depends on a synchronous response or error propagation.
- You need strict ordering with caller-controlled backpressure.
- The coupling is intentional and unlikely to change.

## Invariants

- The bus is a fan-out mechanism only; it does not store history.
- Output/log buffers remain separate from the bus.
- The frontend and backend ship together; WebSocket event formats are verified
  via contract tests in later steps.
