package watcher

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"gestalt/internal/event"
)

const (
	EventTypeFileChanged      = "file_changed"
	EventTypeGitBranchChanged = "git_branch_changed"
	EventTypeWatchError       = "watch_error"
)

// EventHub manages subscriptions and publishes higher-level events.
type EventHub struct {
	watcher       Watch
	mutex         sync.Mutex
	subscriptions map[string]func()
	watches       map[string]Handle
	nextID        uint64
	ctx           context.Context
	cancel        context.CancelFunc
	closeOnce     sync.Once
	bus           *event.Bus[Event]
}

// NewEventHub creates an EventHub tied to the provided context.
func NewEventHub(ctx context.Context, watcher Watch) *EventHub {
	if ctx == nil {
		ctx = context.Background()
	}
	derived, cancel := context.WithCancel(ctx)
	hub := &EventHub{
		watcher:       watcher,
		subscriptions: make(map[string]func()),
		watches:       make(map[string]Handle),
		ctx:           derived,
		cancel:        cancel,
		bus: event.NewBus[Event](derived, event.BusOptions{
			Name: "watcher_events",
		}),
	}
	go func() {
		<-derived.Done()
		_ = hub.Close()
	}()
	return hub
}

// Subscribe registers a listener for an event type.
func (hub *EventHub) Subscribe(eventType string, listener func(Event)) string {
	if hub == nil || hub.bus == nil || eventType == "" || listener == nil {
		return ""
	}

	events, cancel := hub.bus.SubscribeFiltered(func(event Event) bool {
		return event.Type == eventType
	})

	hub.mutex.Lock()
	hub.nextID++
	id := strconv.FormatUint(hub.nextID, 10)
	hub.subscriptions[id] = cancel
	hub.mutex.Unlock()

	go func() {
		for event := range events {
			listener(event)
		}
	}()

	return id
}

// Unsubscribe removes a subscription by ID.
func (hub *EventHub) Unsubscribe(id string) {
	if hub == nil || id == "" {
		return
	}

	hub.mutex.Lock()
	cancel, ok := hub.subscriptions[id]
	delete(hub.subscriptions, id)
	hub.mutex.Unlock()

	if ok && cancel != nil {
		cancel()
	}
}

// Publish broadcasts an event to subscribers of its type.
func (hub *EventHub) Publish(event Event) {
	if hub == nil || hub.bus == nil || event.Type == "" {
		return
	}
	hub.bus.Publish(event)
}

// WatchFile registers a filesystem watch and publishes file change events.
func (hub *EventHub) WatchFile(path string) error {
	if hub == nil {
		return errors.New("event hub is nil")
	}
	if hub.watcher == nil {
		return errors.New("watcher is nil")
	}
	if path == "" {
		return errors.New("path is required")
	}

	hub.mutex.Lock()
	if _, ok := hub.watches[path]; ok {
		hub.mutex.Unlock()
		return nil
	}
	hub.watches[path] = nil
	hub.mutex.Unlock()

	handle, err := hub.watcher.Watch(path, func(event Event) {
		hub.Publish(Event{
			Type:      EventTypeFileChanged,
			Path:      event.Path,
			Op:        event.Op,
			Timestamp: event.Timestamp,
		})
	})
	if err != nil {
		hub.mutex.Lock()
		delete(hub.watches, path)
		hub.mutex.Unlock()
		return err
	}

	hub.mutex.Lock()
	hub.watches[path] = handle
	hub.mutex.Unlock()
	return nil
}

// UnwatchFile stops watching a path.
func (hub *EventHub) UnwatchFile(path string) error {
	if hub == nil || path == "" {
		return nil
	}

	hub.mutex.Lock()
	handle := hub.watches[path]
	delete(hub.watches, path)
	hub.mutex.Unlock()

	if handle == nil {
		return nil
	}
	return handle.Close()
}

// Close shuts down the hub and releases watcher registrations.
func (hub *EventHub) Close() error {
	if hub == nil {
		return nil
	}

	var closeErr error
	hub.closeOnce.Do(func() {
		if hub.cancel != nil {
			hub.cancel()
		}
		if hub.bus != nil {
			hub.bus.Close()
		}

		hub.mutex.Lock()
		watches := hub.watches
		hub.watches = make(map[string]Handle)
		subscriptions := hub.subscriptions
		hub.subscriptions = make(map[string]func())
		hub.mutex.Unlock()

		for _, cancel := range subscriptions {
			if cancel != nil {
				cancel()
			}
		}

		for _, handle := range watches {
			if handle == nil {
				continue
			}
			if err := handle.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

// SubscriberCount reports the number of active subscriptions.
func (hub *EventHub) SubscriberCount() int {
	if hub == nil {
		return 0
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	return len(hub.subscriptions)
}
