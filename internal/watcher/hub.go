package watcher

import (
	"context"
	"errors"
	"strconv"
	"sync"
)

const (
	EventTypeFileChanged      = "file_changed"
	EventTypeGitBranchChanged = "git_branch_changed"
)

// EventHub manages subscriptions and publishes higher-level events.
type EventHub struct {
	watcher           Watch
	mutex             sync.Mutex
	subscribers       map[string]map[string]func(Event)
	subscriptionTypes map[string]string
	watches           map[string]Handle
	nextID            uint64
	ctx               context.Context
	cancel            context.CancelFunc
	closeOnce         sync.Once
}

// NewEventHub creates an EventHub tied to the provided context.
func NewEventHub(ctx context.Context, watcher Watch) *EventHub {
	if ctx == nil {
		ctx = context.Background()
	}
	derived, cancel := context.WithCancel(ctx)
	hub := &EventHub{
		watcher:           watcher,
		subscribers:       make(map[string]map[string]func(Event)),
		subscriptionTypes: make(map[string]string),
		watches:           make(map[string]Handle),
		ctx:               derived,
		cancel:            cancel,
	}
	go func() {
		<-derived.Done()
		_ = hub.Close()
	}()
	return hub
}

// Subscribe registers a listener for an event type.
func (hub *EventHub) Subscribe(eventType string, listener func(Event)) string {
	if hub == nil || eventType == "" || listener == nil {
		return ""
	}

	hub.mutex.Lock()
	defer hub.mutex.Unlock()

	hub.nextID++
	id := strconv.FormatUint(hub.nextID, 10)
	if hub.subscribers[eventType] == nil {
		hub.subscribers[eventType] = make(map[string]func(Event))
	}
	hub.subscribers[eventType][id] = listener
	hub.subscriptionTypes[id] = eventType
	return id
}

// Unsubscribe removes a subscription by ID.
func (hub *EventHub) Unsubscribe(id string) {
	if hub == nil || id == "" {
		return
	}

	hub.mutex.Lock()
	defer hub.mutex.Unlock()

	eventType, ok := hub.subscriptionTypes[id]
	if !ok {
		return
	}
	delete(hub.subscriptionTypes, id)

	listeners := hub.subscribers[eventType]
	if listeners == nil {
		return
	}
	delete(listeners, id)
	if len(listeners) == 0 {
		delete(hub.subscribers, eventType)
	}
}

// Publish broadcasts an event to subscribers of its type.
func (hub *EventHub) Publish(event Event) {
	if hub == nil || event.Type == "" {
		return
	}

	hub.mutex.Lock()
	listeners := hub.subscribers[event.Type]
	if len(listeners) == 0 {
		hub.mutex.Unlock()
		return
	}
	callbacks := make([]func(Event), 0, len(listeners))
	for _, listener := range listeners {
		callbacks = append(callbacks, listener)
	}
	hub.mutex.Unlock()

	for _, callback := range callbacks {
		callback(event)
	}
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

		hub.mutex.Lock()
		watches := hub.watches
		hub.watches = make(map[string]Handle)
		hub.subscribers = make(map[string]map[string]func(Event))
		hub.subscriptionTypes = make(map[string]string)
		hub.mutex.Unlock()

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
