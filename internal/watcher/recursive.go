package watcher

import (
	"io/fs"
	"path/filepath"
)

func (watcher *Watcher) addRecursiveWatches(root string) ([]string, error) {
	if watcher == nil || !watcher.watchRecursive {
		return nil, nil
	}
	paths, err := collectRecursiveDirs(root)
	if err != nil {
		return nil, err
	}

	added := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := watcher.addRecursiveWatch(path); err != nil {
			watcher.removeRecursiveWatches(added)
			return nil, err
		}
		added = append(added, path)
	}

	return added, nil
}

func collectRecursiveDirs(root string) ([]string, error) {
	dirs := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	return dirs, err
}

func (watcher *Watcher) addRecursiveWatch(path string) error {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return nil
	}
	if watcher.isPathWatchedLocked(path) {
		watcher.recursiveWatches[path]++
		watcher.mutex.Unlock()
		return nil
	}
	if watcher.activeWatches >= watcher.maxWatches {
		watcher.mutex.Unlock()
		return ErrMaxWatchesExceeded
	}
	watcher.recursiveWatches[path] = 1
	watcher.activeWatches++
	activeCount := watcher.activeWatches
	watcher.mutex.Unlock()

	if watcher.watcher == nil {
		watcher.dropRecursiveWatch(path)
		return nil
	}
	if err := watcher.watcher.Add(path); err != nil {
		watcher.dropRecursiveWatch(path)
		watcher.logWarn("watch add failed", map[string]string{
			"path":  path,
			"error": err.Error(),
		})
		return err
	}
	watcher.logDebug("watch added", path, activeCount)
	return nil
}

func (watcher *Watcher) removeRecursiveWatches(paths []string) {
	if watcher == nil {
		return
	}
	for _, path := range paths {
		watcher.removeRecursiveWatch(path)
	}
}

func (watcher *Watcher) removeRecursiveWatch(path string) {
	if watcher == nil {
		return
	}

	shouldRemove := false
	activeCount := 0

	watcher.mutex.Lock()
	count := watcher.recursiveWatches[path]
	if count > 1 {
		watcher.recursiveWatches[path] = count - 1
		watcher.mutex.Unlock()
		return
	}
	if count == 1 {
		delete(watcher.recursiveWatches, path)
		if len(watcher.callbacks[path]) == 0 {
			shouldRemove = true
			if watcher.activeWatches > 0 {
				watcher.activeWatches--
			}
			activeCount = watcher.activeWatches
		}
	}
	watcher.mutex.Unlock()

	if shouldRemove && watcher.watcher != nil {
		if err := watcher.watcher.Remove(path); err != nil {
			watcher.logWarn("watch remove failed", map[string]string{
				"path":  path,
				"error": err.Error(),
			})
			return
		}
		watcher.logDebug("watch removed", path, activeCount)
	}
}

func (watcher *Watcher) dropRecursiveWatch(path string) {
	watcher.mutex.Lock()
	count := watcher.recursiveWatches[path]
	if count > 1 {
		watcher.recursiveWatches[path] = count - 1
		watcher.mutex.Unlock()
		return
	}
	if count == 1 {
		delete(watcher.recursiveWatches, path)
		if watcher.activeWatches > 0 {
			watcher.activeWatches--
		}
	}
	watcher.mutex.Unlock()
}

func (watcher *Watcher) isPathWatchedLocked(path string) bool {
	if watcher == nil {
		return false
	}
	if len(watcher.callbacks[path]) > 0 {
		return true
	}
	return watcher.recursiveWatches[path] > 0
}
