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

func (watcher *Watcher) handleEvent(event fsnotify.Event) {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return
	}
	callbacks := watcher.callbacks[event.Name]
	if len(callbacks) == 0 && watcher.watchDirRecursive {
		for path, entries := range watcher.callbacks {
			if !hasDirWatch(entries) {
				continue
			}
			if !isWithinPath(path, event.Name) {
				continue
			}
			callbacks = append(callbacks, entries...)
		}
	}
	if len(callbacks) == 0 {
		watcher.mutex.Unlock()
		return
	}

	entry := watcher.debounce[event.Name]
	entry.event = Event{
		Path:      event.Name,
		Op:        event.Op,
		Timestamp: time.Now().UTC(),
	}
	if entry.timer == nil {
		entry.timer = time.AfterFunc(watcher.debounceDuration, func() {
			watcher.flush(event.Name)
		})
	} else {
		entry.timer.Reset(watcher.debounceDuration)
	}
	watcher.debounce[event.Name] = entry
	watcher.mutex.Unlock()
}

func (watcher *Watcher) flush(path string) {
	var (
		callbacks []func(Event)
		event     Event
	)

	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return
	}
	entry, ok := watcher.debounce[path]
	if !ok {
		watcher.mutex.Unlock()
		return
	}
	delete(watcher.debounce, path)
	event = entry.event
	if entries := watcher.callbacks[path]; len(entries) > 0 {
		for _, entry := range entries {
			callbacks = append(callbacks, entry.callback)
		}
	} else if watcher.watchDirRecursive {
		for watchPath, entries := range watcher.callbacks {
			if !hasDirWatch(entries) {
				continue
			}
			if !isWithinPath(watchPath, path) {
				continue
			}
			for _, entry := range entries {
				callbacks = append(callbacks, entry.callback)
			}
		}
	}
	watcher.mutex.Unlock()

	for _, callback := range callbacks {
		callback(event)
		atomic.AddUint64(&watcher.eventsDelivered, 1)
	}
}
