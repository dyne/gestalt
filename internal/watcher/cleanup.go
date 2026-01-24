package watcher

import "time"

func (watcher *Watcher) cleanupLoop() {
	ticker := time.NewTicker(watcher.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			watcher.cleanup()
		case <-watcher.done:
			return
		}
	}
}

func (watcher *Watcher) cleanup() {
	if watcher == nil {
		return
	}
	paths := make([]string, 0)
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return
	}
	for path, callbacks := range watcher.callbacks {
		if len(callbacks) == 0 {
			delete(watcher.callbacks, path)
			if watcher.recursiveWatches[path] == 0 {
				if watcher.activeWatches > 0 {
					watcher.activeWatches--
				}
				paths = append(paths, path)
			}
		}
	}
	activeCount := watcher.activeWatches
	watcher.mutex.Unlock()

	for _, path := range paths {
		if watcher.watcher == nil {
			continue
		}
		if err := watcher.watcher.Remove(path); err != nil {
			watcher.logWarn("watch cleanup failed", map[string]string{
				"path":  path,
				"error": err.Error(),
			})
			continue
		}
		watcher.logDebug("watch cleaned", path, activeCount)
	}
}
