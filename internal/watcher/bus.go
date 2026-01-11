package watcher

import (
	"errors"

	"gestalt/internal/event"
)

// WatchFile registers a filesystem watch and publishes file change events.
func WatchFile(bus *event.Bus[Event], watch Watch, path string) (Handle, error) {
	if bus == nil {
		return nil, errors.New("event bus is nil")
	}
	if watch == nil {
		return nil, errors.New("watcher is nil")
	}
	if path == "" {
		return nil, errors.New("path is required")
	}

	handle, err := watch.Watch(path, func(event Event) {
		bus.Publish(Event{
			Type:      EventTypeFileChanged,
			Path:      event.Path,
			Op:        event.Op,
			Timestamp: event.Timestamp,
		})
	})
	if err != nil {
		return nil, err
	}
	return handle, nil
}
