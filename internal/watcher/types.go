package watcher

import (
	"sync"
	"time"

	"gestalt/internal/logging"
	"github.com/fsnotify/fsnotify"
)

const (
	EventTypeFileChanged      = "file_changed"
	EventTypeGitBranchChanged = "git_branch_changed"
	EventTypeWatchError       = "watch_error"
)

// Event represents a single filesystem change.
type Event struct {
	Type      string
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
	// WatchDir enables fan-out from directory watches; it does not add recursive watches.
	WatchDir        bool
	MaxWatches      int
	CleanupInterval time.Duration
	ErrorHandler    func(error)
}

// Metrics describes watcher activity.
type Metrics struct {
	ActiveWatches   int
	EventsDelivered uint64
	EventsDropped   uint64
	Errors          uint64
	RestartAttempts int
}

// Watcher is the concrete fsnotify-backed implementation.
type Watcher struct {
	watcher           *fsnotify.Watcher
	mutex             sync.Mutex
	callbacks         map[string][]callbackEntry
	debouncer         *debouncer
	events            chan fsnotify.Event
	errors            chan error
	done              chan struct{}
	closed            bool
	logger            *logging.Logger
	watchDirRecursive bool
	nextID            uint64
	maxWatches        int
	activeWatches     int
	cleanupInterval   time.Duration
	eventsDelivered   uint64
	eventsDropped     uint64
	errorCount        uint64
	errorHandler      func(error)
	restartMutex      sync.Mutex
	restartAttempts   int
	restartTimer      *time.Timer
}
