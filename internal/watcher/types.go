package watcher

import (
	"sync"
	"time"

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

// Watcher is the concrete fsnotify-backed implementation.
type Watcher struct {
	watcher   *fsnotify.Watcher
	mutex     sync.Mutex
	callbacks map[string][]func(Event)
}
