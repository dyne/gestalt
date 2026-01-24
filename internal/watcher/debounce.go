package watcher

import (
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

type debounceEntry struct {
	timer *time.Timer
	event Event
}

type debouncer struct {
	duration time.Duration
	entries  map[string]debounceEntry
}

func newDebouncer(duration time.Duration) *debouncer {
	return &debouncer{
		duration: duration,
		entries:  make(map[string]debounceEntry),
	}
}

func (debouncer *debouncer) schedule(path string, event Event, flush func(string)) bool {
	if debouncer == nil {
		return false
	}
	entry := debouncer.entries[path]
	dropped := entry.timer != nil
	entry.event = event
	if entry.timer == nil {
		entry.timer = time.AfterFunc(debouncer.duration, func() {
			flush(path)
		})
	} else {
		entry.timer.Reset(debouncer.duration)
	}
	debouncer.entries[path] = entry
	return dropped
}

func (debouncer *debouncer) pop(path string) (Event, bool) {
	if debouncer == nil {
		return Event{}, false
	}
	entry, ok := debouncer.entries[path]
	if !ok {
		return Event{}, false
	}
	delete(debouncer.entries, path)
	return entry.event, true
}

func (debouncer *debouncer) stop() {
	if debouncer == nil {
		return
	}
	for _, entry := range debouncer.entries {
		if entry.timer != nil {
			entry.timer.Stop()
		}
	}
	debouncer.entries = nil
}

func (watcher *Watcher) handleEvent(event fsnotify.Event) {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return
	}
	if !watcher.hasCallbacksLocked(event.Name) {
		watcher.mutex.Unlock()
		return
	}

	entry := Event{
		Path:      event.Name,
		Op:        event.Op,
		Timestamp: time.Now().UTC(),
	}
	if watcher.debouncer != nil {
		dropped := watcher.debouncer.schedule(event.Name, entry, watcher.flush)
		if dropped {
			atomic.AddUint64(&watcher.eventsDropped, 1)
		}
	}
	watcher.mutex.Unlock()
}

func (watcher *Watcher) flush(path string) {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return
	}
	if watcher.debouncer == nil {
		watcher.mutex.Unlock()
		return
	}
	event, ok := watcher.debouncer.pop(path)
	if !ok {
		watcher.mutex.Unlock()
		return
	}
	callbacks := watcher.callbacksForPathLocked(path)
	watcher.mutex.Unlock()

	for _, callback := range callbacks {
		callback(event)
		atomic.AddUint64(&watcher.eventsDelivered, 1)
	}
}
