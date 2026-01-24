package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type callbackEntry struct {
	id       uint64
	callback func(Event)
	isDir    bool
}

type watchHandle struct {
	watcher        *Watcher
	path           string
	id             uint64
	recursivePaths []string
	once           sync.Once
}

func (handle *watchHandle) Close() error {
	if handle == nil || handle.watcher == nil {
		return nil
	}
	var err error
	handle.once.Do(func() {
		if handle.recursivePaths != nil {
			handle.watcher.removeRecursiveWatches(handle.recursivePaths)
		}
		err = handle.watcher.removeCallback(handle.path, handle.id)
	})
	return err
}

// Watch registers a callback for filesystem events on a path.
func (watcher *Watcher) Watch(path string, callback func(Event)) (Handle, error) {
	if watcher == nil {
		return nil, errors.New("watcher is nil")
	}
	if path == "" {
		return nil, errors.New("path is required")
	}
	if callback == nil {
		return nil, errors.New("callback is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return nil, errors.New("watcher is closed")
	}

	needsAdd := watcher.callbacks[path] == nil
	if needsAdd && watcher.activeWatches >= watcher.maxWatches {
		watcher.mutex.Unlock()
		return nil, ErrMaxWatchesExceeded
	}
	watcher.nextID++
	entry := callbackEntry{callback: callback, id: watcher.nextID, isDir: info.IsDir()}
	watcher.callbacks[path] = append(watcher.callbacks[path], entry)
	if needsAdd {
		watcher.activeWatches++
	}
	activeCount := watcher.activeWatches
	watcher.mutex.Unlock()

	if needsAdd {
		if err := watcher.watcher.Add(path); err != nil {
			watcher.dropCallback(path, entry.id)
			watcher.logWarn("watch add failed", map[string]string{
				"path":  path,
				"error": err.Error(),
			})
			return nil, err
		}
		watcher.logDebug("watch added", path, activeCount)
	}

	var recursivePaths []string
	if entry.isDir && watcher.watchRecursive {
		paths, err := watcher.addRecursiveWatches(path)
		if err != nil {
			if handleErr := watcher.removeCallback(path, entry.id); handleErr != nil {
				watcher.logWarn("watch rollback failed", map[string]string{
					"path":  path,
					"error": handleErr.Error(),
				})
			}
			return nil, err
		}
		recursivePaths = paths
	}

	return &watchHandle{watcher: watcher, path: path, id: entry.id, recursivePaths: recursivePaths}, nil
}

// WatchContext registers a callback and closes the handle when the context is done.
func (watcher *Watcher) WatchContext(ctx context.Context, path string, callback func(Event)) (Handle, error) {
	handle, err := watcher.Watch(path, callback)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		return handle, nil
	}
	go func() {
		<-ctx.Done()
		_ = handle.Close()
	}()
	return handle, nil
}

func (watcher *Watcher) removeCallback(path string, id uint64) error {
	if watcher == nil {
		return nil
	}

	shouldRemove := false
	activeCount := 0
	watcher.mutex.Lock()
	callbacks := watcher.callbacks[path]
	if len(callbacks) > 0 {
		for index, candidate := range callbacks {
			if candidate.id == id {
				callbacks = append(callbacks[:index], callbacks[index+1:]...)
				break
			}
		}
		if len(callbacks) == 0 {
			delete(watcher.callbacks, path)
			if watcher.recursiveWatches[path] == 0 {
				shouldRemove = true
				if watcher.activeWatches > 0 {
					watcher.activeWatches--
				}
				activeCount = watcher.activeWatches
			}
		} else {
			watcher.callbacks[path] = callbacks
		}
	}
	watcher.mutex.Unlock()

	if shouldRemove && watcher.watcher != nil {
		if err := watcher.watcher.Remove(path); err != nil {
			watcher.logWarn("watch remove failed", map[string]string{
				"path":  path,
				"error": err.Error(),
			})
			return err
		}
		watcher.logDebug("watch removed", path, activeCount)
	}
	return nil
}

func (watcher *Watcher) dropCallback(path string, id uint64) {
	if watcher == nil {
		return
	}

	watcher.mutex.Lock()
	callbacks := watcher.callbacks[path]
	if len(callbacks) > 0 {
		for index, candidate := range callbacks {
			if candidate.id == id {
				callbacks = append(callbacks[:index], callbacks[index+1:]...)
				break
			}
		}
		if len(callbacks) == 0 {
			delete(watcher.callbacks, path)
			if watcher.recursiveWatches[path] == 0 {
				if watcher.activeWatches > 0 {
					watcher.activeWatches--
				}
			}
		} else {
			watcher.callbacks[path] = callbacks
		}
	}
	watcher.mutex.Unlock()
}

func (watcher *Watcher) hasCallbacksLocked(path string) bool {
	if watcher == nil {
		return false
	}
	if len(watcher.callbacks[path]) > 0 {
		return true
	}
	if !watcher.watchDirRecursive {
		return false
	}
	for watchPath, entries := range watcher.callbacks {
		if !hasDirWatch(entries) {
			continue
		}
		if isWithinPath(watchPath, path) {
			return true
		}
	}
	return false
}

func (watcher *Watcher) callbacksForPathLocked(path string) []func(Event) {
	if watcher == nil {
		return nil
	}
	if entries := watcher.callbacks[path]; len(entries) > 0 {
		callbacks := make([]func(Event), 0, len(entries))
		for _, entry := range entries {
			callbacks = append(callbacks, entry.callback)
		}
		return callbacks
	}
	if !watcher.watchDirRecursive {
		return nil
	}

	callbacks := []func(Event){}
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
	return callbacks
}

func hasDirWatch(entries []callbackEntry) bool {
	for _, entry := range entries {
		if entry.isDir {
			return true
		}
	}
	return false
}

func isWithinPath(parent, child string) bool {
	parentPath := filepath.Clean(parent)
	childPath := filepath.Clean(child)
	rel, err := filepath.Rel(parentPath, childPath)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
