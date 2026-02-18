package flow

import (
	"fmt"
	"time"

	"gestalt/internal/watcher"
)

type bridgeLimiter struct {
	limit       int
	window      time.Duration
	count       int
	windowStart time.Time
}

func newBridgeLimiter(limit int, window time.Duration) *bridgeLimiter {
	if limit <= 0 {
		limit = 1
	}
	return &bridgeLimiter{limit: limit, window: window}
}

func (limiter *bridgeLimiter) Allow(now time.Time) bool {
	if limiter == nil || limiter.limit <= 0 {
		return true
	}
	if limiter.window <= 0 {
		return true
	}
	if limiter.windowStart.IsZero() || now.Sub(limiter.windowStart) >= limiter.window {
		limiter.windowStart = now
		limiter.count = 0
	}
	if limiter.count >= limiter.limit {
		return false
	}
	limiter.count++
	return true
}

type watcherDeduper struct {
	window time.Duration
	seen   map[string]time.Time
}

func (deduper *watcherDeduper) Allow(event watcher.Event, now time.Time) bool {
	if deduper == nil || deduper.window <= 0 {
		return true
	}
	if deduper.seen == nil {
		deduper.seen = make(map[string]time.Time)
	}
	key := dedupeKey(event)
	if key == "" {
		return true
	}
	if last, ok := deduper.seen[key]; ok {
		if now.Sub(last) < deduper.window {
			return false
		}
	}
	for seenKey, seenAt := range deduper.seen {
		if now.Sub(seenAt) >= deduper.window {
			delete(deduper.seen, seenKey)
		}
	}
	deduper.seen[key] = now
	return true
}

func dedupeKey(event watcher.Event) string {
	path := event.Path
	typeValue := event.Type
	if typeValue == "" && path == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", typeValue, path)
}
