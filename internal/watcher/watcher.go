package watcher

import (
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/logging"
	"github.com/fsnotify/fsnotify"
)

const (
	defaultDebounce        = 100 * time.Millisecond
	defaultMaxWatches      = 100
	defaultCleanupInterval = time.Minute
)

var ErrMaxWatchesExceeded = errors.New("max watches exceeded")

type debounceEntry struct {
	timer *time.Timer
	event Event
}

type callbackEntry struct {
	id       uint64
	callback func(Event)
}

type watchHandle struct {
	watcher *Watcher
	path    string
	id      uint64
	once    sync.Once
}

func (handle *watchHandle) Close() error {
	if handle == nil || handle.watcher == nil {
		return nil
	}
	var err error
	handle.once.Do(func() {
		err = handle.watcher.removeCallback(handle.path, handle.id)
	})
	return err
}

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
	}

	instance.startForwarder(watcher)
	go instance.run()
	go instance.cleanupLoop()
	return instance, nil
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
	entry := callbackEntry{callback: callback, id: watcher.nextID}
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

	return &watchHandle{watcher: watcher, path: path, id: entry.id}, nil
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

func (watcher *Watcher) handleEvent(event fsnotify.Event) {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return
	}
	if watcher.callbacks[event.Name] == nil {
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
	if watcher.callbacks[path] != nil {
		for _, entry := range watcher.callbacks[path] {
			callbacks = append(callbacks, entry.callback)
		}
	}
	watcher.mutex.Unlock()

	for _, callback := range callbacks {
		callback(event)
		atomic.AddUint64(&watcher.eventsDelivered, 1)
	}
}

func (watcher *Watcher) handleError(err error) {
	if err == nil {
		return
	}
	atomic.AddUint64(&watcher.errorCount, 1)
	watcher.logWarn("watcher error", map[string]string{
		"error": err.Error(),
	})
	if restartErr := watcher.restart(); restartErr != nil {
		watcher.logWarn("watcher restart failed", map[string]string{
			"error": restartErr.Error(),
		})
	}
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
			shouldRemove = true
			if watcher.activeWatches > 0 {
				watcher.activeWatches--
			}
			activeCount = watcher.activeWatches
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
			if watcher.activeWatches > 0 {
				watcher.activeWatches--
			}
		} else {
			watcher.callbacks[path] = callbacks
		}
	}
	watcher.mutex.Unlock()
}

func (watcher *Watcher) logWarn(message string, fields map[string]string) {
	if watcher == nil || watcher.logger == nil {
		return
	}
	watcher.logger.Warn(message, fields)
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
			if watcher.activeWatches > 0 {
				watcher.activeWatches--
			}
			paths = append(paths, path)
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
