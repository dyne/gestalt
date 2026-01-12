package server

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
)

func PreparePlanFile(logger *logging.Logger) string {
	targetPath := plan.DefaultPath()
	legacyPath := "PLAN.org"

	legacyInfo, err := os.Stat(legacyPath)
	if err != nil || legacyInfo.IsDir() {
		return targetPath
	}

	if _, err := os.Stat(targetPath); err == nil {
		if logger != nil {
			logger.Warn("PLAN.org exists in multiple locations; using .gestalt/PLAN.org", map[string]string{
				"path": targetPath,
			})
		}
		return targetPath
	} else if !os.IsNotExist(err) {
		if logger != nil {
			logger.Warn("plan path check failed", map[string]string{
				"path":  targetPath,
				"error": err.Error(),
			})
		}
		return targetPath
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		if logger != nil {
			logger.Warn("plan migration failed", map[string]string{
				"path":  targetPath,
				"error": err.Error(),
			})
		}
		return targetPath
	}
	if err := copyFile(legacyPath, targetPath); err != nil {
		if logger != nil {
			logger.Warn("plan migration failed", map[string]string{
				"path":  targetPath,
				"error": err.Error(),
			})
		}
		return targetPath
	}
	if logger != nil {
		logger.Info("Migrated PLAN.org to .gestalt/PLAN.org", map[string]string{
			"path": targetPath,
		})
	}
	return targetPath
}

func WatchPlanFile(bus *event.Bus[watcher.Event], watch watcher.Watch, logger *logging.Logger, planPath string) {
	if bus == nil || watch == nil {
		return
	}
	if planPath == "" {
		planPath = plan.DefaultPath()
	}

	var retryMutex sync.Mutex
	retrying := false
	var handleMu sync.Mutex
	var handle watcher.Handle

	stopWatch := func() {
		handleMu.Lock()
		if handle != nil {
			_ = handle.Close()
			handle = nil
		}
		handleMu.Unlock()
	}

	startWatch := func() error {
		newHandle, err := watcher.WatchFile(bus, watch, planPath)
		if err != nil {
			return err
		}
		handleMu.Lock()
		if handle != nil {
			_ = handle.Close()
		}
		handle = newHandle
		handleMu.Unlock()
		return nil
	}

	startRetry := func() {
		retryMutex.Lock()
		if retrying {
			retryMutex.Unlock()
			return
		}
		retrying = true
		retryMutex.Unlock()

		go func() {
			defer func() {
				retryMutex.Lock()
				retrying = false
				retryMutex.Unlock()
			}()
			backoff := 100 * time.Millisecond
			for {
				if err := startWatch(); err == nil {
					if logger != nil {
						logger.Info("Watching plan file for changes", map[string]string{
							"path": planPath,
						})
					}
					return
				}
				time.Sleep(backoff)
				if backoff < 2*time.Second {
					backoff *= 2
				}
			}
		}()
	}

	if err := startWatch(); err != nil {
		if logger != nil {
			logger.Warn("plan watch failed", map[string]string{
				"path":  planPath,
				"error": err.Error(),
			})
		}
		startRetry()
	} else if logger != nil {
		logger.Info("Watching plan file for changes", map[string]string{
			"path": planPath,
		})
	}

	events, _ := bus.SubscribeFiltered(func(event watcher.Event) bool {
		return event.Type == watcher.EventTypeFileChanged && event.Path == planPath
	})
	go func() {
		for event := range events {
			if event.Op&(fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			stopWatch()
			startRetry()
		}
	}()
}
