//go:build !noscip

package api

import (
	"testing"
	"time"

	"gestalt/internal/scip"
)

func TestQueryCacheEvictsOldest(t *testing.T) {
	cache := newQueryCache(time.Minute)
	cache.maxEntries = 2

	cache.setSymbols("first", []scip.Symbol{{ID: "first"}})
	cache.setSymbols("second", []scip.Symbol{{ID: "second"}})
	cache.setSymbols("third", []scip.Symbol{{ID: "third"}})

	if _, ok := cache.getSymbols("first"); ok {
		t.Fatalf("expected oldest entry to be evicted")
	}
	if _, ok := cache.getSymbols("second"); !ok {
		t.Fatalf("expected second entry to remain")
	}
	if _, ok := cache.getSymbols("third"); !ok {
		t.Fatalf("expected third entry to remain")
	}
}

func TestQueryCacheExpires(t *testing.T) {
	cache := newQueryCache(5 * time.Millisecond)
	cache.setSymbols("expiring", []scip.Symbol{{ID: "expiring"}})

	if _, ok := cache.getSymbols("expiring"); !ok {
		t.Fatalf("expected entry to be available before expiry")
	}

	time.Sleep(10 * time.Millisecond)

	if _, ok := cache.getSymbols("expiring"); ok {
		t.Fatalf("expected entry to expire")
	}
}
