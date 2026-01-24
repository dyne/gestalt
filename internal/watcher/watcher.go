package watcher

import (
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"gestalt/internal/logging"
	"github.com/fsnotify/fsnotify"
)

const (
	defaultDebounce        = 100 * time.Millisecond
	defaultMaxWatches      = 100
	defaultCleanupInterval = time.Minute
	maxRestartAttempts     = 3
	restartBaseDelay       = 200 * time.Millisecond
)

var ErrMaxWatchesExceeded = errors.New("max watches exceeded")

// New creates a Watcher with default options.
func New() (*Watcher, error) {
	return NewWithOptions(Options{})
}

// NewWithOptions creates a Watcher with custom options.
func NewWithOptions(options Options) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	logger := options.Logger
	if logger == nil {
		logger = logging.NewLoggerWithOutput(logging.NewLogBuffer(logging.DefaultBufferSize), logging.LevelInfo, nil)
	}

	debounce := options.Debounce
	if debounce <= 0 {
		debounce = defaultDebounce
	}

	maxWatches := options.MaxWatches
	if maxWatches <= 0 {
		maxWatches = defaultMaxWatches
	}

	cleanupInterval := options.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = defaultCleanupInterval
	}

	instance := &Watcher{
		watcher:           watcher,
		callbacks:         make(map[string][]callbackEntry),
		debounce:          make(map[string]debounceEntry),
		debounceDuration:  debounce,
		events:            make(chan fsnotify.Event, 16),
		errors:            make(chan error, 4),
		done:              make(chan struct{}),
		logger:            logger,
		watchDirRecursive: options.WatchDir,
		maxWatches:        maxWatches,
		cleanupInterval:   cleanupInterval,
		errorHandler:      options.ErrorHandler,
	}

	instance.startForwarder(watcher)
	go instance.run()
	go instance.cleanupLoop()
	return instance, nil
}

// Close shuts down the watcher and stops event processing.
func (watcher *Watcher) Close() error {
	if watcher == nil {
		return nil
	}

	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return nil
	}
	watcher.closed = true
	for _, entry := range watcher.debounce {
		if entry.timer != nil {
			entry.timer.Stop()
		}
	}
	watcher.debounce = nil
	watcher.mutex.Unlock()

	close(watcher.done)
	if watcher.watcher == nil {
		return nil
	}
	return watcher.watcher.Close()
}

func (watcher *Watcher) run() {
	for {
		select {
		case event := <-watcher.events:
			watcher.handleEvent(event)
		case err := <-watcher.errors:
			watcher.handleError(err)
		case <-watcher.done:
			return
		}
	}
}

func (watcher *Watcher) startForwarder(source *fsnotify.Watcher) {
	if source == nil {
		return
	}

	go func() {
		for {
			select {
			case event, ok := <-source.Events:
				if !ok {
					return
				}
				select {
				case watcher.events <- event:
				case <-watcher.done:
					return
				}
			case err, ok := <-source.Errors:
				if !ok {
					return
				}
				select {
				case watcher.errors <- err:
				case <-watcher.done:
					return
				}
			case <-watcher.done:
				return
			}
		}
	}()
}

func (watcher *Watcher) logWarn(message string, fields map[string]string) {
	if watcher == nil || watcher.logger == nil {
		return
	}
	watcher.logger.Warn(message, fields)
}

// SetErrorHandler configures a callback for unrecoverable watcher failures.
func (watcher *Watcher) SetErrorHandler(handler func(error)) {
	if watcher == nil {
		return
	}
	watcher.mutex.Lock()
	watcher.errorHandler = handler
	watcher.mutex.Unlock()
}

func (watcher *Watcher) logDebug(message, path string, activeCount int) {
	if watcher == nil || watcher.logger == nil {
		return
	}
	watcher.logger.Debug(message, map[string]string{
		"path":           path,
		"active_watches": strconv.Itoa(activeCount),
	})
}

// Metrics reports current watcher stats.
func (watcher *Watcher) Metrics() Metrics {
	if watcher == nil {
		return Metrics{}
	}
	watcher.mutex.Lock()
	active := watcher.activeWatches
	watcher.mutex.Unlock()
	return Metrics{
		ActiveWatches:   active,
		EventsDelivered: atomic.LoadUint64(&watcher.eventsDelivered),
		Errors:          atomic.LoadUint64(&watcher.errorCount),
	}
}
