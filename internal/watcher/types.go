package watcher

import (
	"sync"
	"time"

	"gestalt/internal/logging"
	"github.com/fsnotify/fsnotify"
)

// Event represents a single filesystem change.
type Event struct {
	Path      string
	Op        fsnotify.Op
	Timestamp time.Time
}

// Handle releases watcher resources for a registration.
type Handle interface {
	Close() error
}

// Watch registers a callback for filesystem events on a path.
type Watch interface {
	Watch(path string, callback func(Event)) (Handle, error)
}

// Options controls watcher behavior.
type Options struct {
	Logger   *logging.Logger
	Debounce time.Duration
	WatchDir bool
}

// Watcher is the concrete fsnotify-backed implementation.
type Watcher struct {
	watcher           *fsnotify.Watcher
	mutex             sync.Mutex
	callbacks         map[string][]callbackEntry
	debounce          map[string]debounceEntry
	debounceDuration  time.Duration
	events            chan fsnotify.Event
	errors            chan error
	done              chan struct{}
	closed            bool
	logger            *logging.Logger
	watchDirRecursive bool
	nextID            uint64
}
