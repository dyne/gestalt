package watcher

import (
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (watcher *Watcher) handleError(err error) {
	if err == nil {
		return
	}
	atomic.AddUint64(&watcher.errorCount, 1)
	watcher.logWarn("watcher error", map[string]string{
		"error": err.Error(),
	})
	watcher.scheduleRestart(err)
}

func restartDelay(attempt int) time.Duration {
	return restartBaseDelay * time.Duration(1<<attempt)
}

func (watcher *Watcher) scheduleRestart(err error) {
	if watcher == nil {
		return
	}
	watcher.restartMutex.Lock()
	if watcher.closed {
		watcher.restartMutex.Unlock()
		return
	}
	if watcher.restartTimer != nil {
		watcher.restartMutex.Unlock()
		return
	}
	if watcher.restartAttempts >= maxRestartAttempts {
		watcher.restartMutex.Unlock()
		watcher.notifyError(err)
		return
	}
	delay := restartDelay(watcher.restartAttempts)
	watcher.restartAttempts++
	watcher.restartTimer = time.AfterFunc(delay, watcher.performRestart)
	watcher.restartMutex.Unlock()
}

func (watcher *Watcher) performRestart() {
	if watcher == nil {
		return
	}
	restartErr := watcher.restart()

	watcher.restartMutex.Lock()
	watcher.restartTimer = nil
	if restartErr == nil {
		watcher.restartAttempts = 0
		watcher.restartMutex.Unlock()
		return
	}
	watcher.restartMutex.Unlock()

	watcher.logWarn("watcher restart failed", map[string]string{
		"error": restartErr.Error(),
	})
	watcher.scheduleRestart(restartErr)
}

func (watcher *Watcher) notifyError(err error) {
	if watcher == nil || watcher.errorHandler == nil || err == nil {
		return
	}
	watcher.errorHandler(err)
}

func (watcher *Watcher) restart() error {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return nil
	}

	paths := make([]string, 0, len(watcher.callbacks))
	for path := range watcher.callbacks {
		paths = append(paths, path)
	}
	watcher.mutex.Unlock()

	replacement, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	for _, path := range paths {
		if err := replacement.Add(path); err != nil {
			watcher.logWarn("watcher re-add failed", map[string]string{
				"path":  path,
				"error": err.Error(),
			})
		}
	}

	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		_ = replacement.Close()
		return nil
	}
	previous := watcher.watcher
	watcher.watcher = replacement
	watcher.mutex.Unlock()

	watcher.startForwarder(replacement)
	if previous != nil {
		_ = previous.Close()
	}
	return nil
}
