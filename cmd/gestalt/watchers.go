package main

import (
	"sync"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
)

func watchPlanFile(bus *event.Bus[watcher.Event], watch watcher.Watch, logger *logging.Logger, planPath string) {
	if bus == nil || watch == nil {
		return
	}
	if planPath == "" {
		planPath = plan.DefaultPlansDir()
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
				err := startWatch()
				if err == nil {
					if logger != nil {
						logger.Info("Watching plans path for changes", map[string]string{
							"path": planPath,
						})
					}
					return
				}
				if logger != nil {
					logger.Warn("plans watch retry failed", map[string]string{
						"path":  planPath,
						"error": err.Error(),
					})
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
			logger.Warn("plans watch failed", map[string]string{
				"path":  planPath,
				"error": err.Error(),
			})
		}
		startRetry()
	} else if logger != nil {
		logger.Info("Watching plans path for changes", map[string]string{
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
