//go:build !noscip

package scip

import (
	"container/list"
	"sync"
)

type cacheEntry struct {
	key   string
	value *Symbol
}

type lruCache struct {
	maxEntries int
	mu         sync.Mutex
	entries    map[string]*list.Element
	order      *list.List
}

func newLRUCache(maxEntries int) *lruCache {
	if maxEntries <= 0 {
		maxEntries = 128
	}
	return &lruCache{
		maxEntries: maxEntries,
		entries:    make(map[string]*list.Element),
		order:      list.New(),
	}
}

func (cache *lruCache) Get(key string) (*Symbol, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	element, ok := cache.entries[key]
	if !ok {
		return nil, false
	}
	cache.order.MoveToFront(element)
	entry := element.Value.(*cacheEntry)
	return cloneSymbol(entry.value), true
}

func (cache *lruCache) Add(key string, value *Symbol) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if element, ok := cache.entries[key]; ok {
		element.Value.(*cacheEntry).value = cloneSymbol(value)
		cache.order.MoveToFront(element)
		return
	}

	element := cache.order.PushFront(&cacheEntry{
		key:   key,
		value: cloneSymbol(value),
	})
	cache.entries[key] = element

	if cache.order.Len() > cache.maxEntries {
		cache.removeOldest()
	}
}

func (cache *lruCache) removeOldest() {
	element := cache.order.Back()
	if element == nil {
		return
	}
	cache.order.Remove(element)
	entry := element.Value.(*cacheEntry)
	delete(cache.entries, entry.key)
}
