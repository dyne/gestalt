package watcher

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestWatcherDispatchesWriteEvent(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	file, err := os.CreateTemp("", "gestalt-watcher-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	events := make(chan Event, 1)
	handle, err := watcher.Watch(path, func(event Event) {
		select {
		case events <- event:
		default:
		}
	})
	if err != nil {
		t.Fatalf("watch path: %v", err)
	}
	defer handle.Close()

	if err := os.WriteFile(path, []byte("update"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	event, ok := waitForEvent(events)
	if !ok {
		t.Fatal("timed out waiting for write event")
	}
	if event.Path != path {
		t.Fatalf("expected path %q, got %q", path, event.Path)
	}
}

func TestWatcherDispatchesRemoveEvent(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	file, err := os.CreateTemp("", "gestalt-watcher-remove-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	events := make(chan Event, 1)
	handle, err := watcher.Watch(path, func(event Event) {
		select {
		case events <- event:
		default:
		}
	})
	if err != nil {
		t.Fatalf("watch path: %v", err)
	}
	defer handle.Close()

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	event, ok := waitForEvent(events)
	if !ok {
		t.Fatal("timed out waiting for remove event")
	}
	if event.Path != path {
		t.Fatalf("expected path %q, got %q", path, event.Path)
	}
}

func waitForEvent(events <-chan Event) (Event, bool) {
	select {
	case event := <-events:
		return event, true
	case <-time.After(2 * time.Second):
		return Event{}, false
	}
}

func TestWatcherEnforcesMaxWatches(t *testing.T) {
	watcher, err := NewWithOptions(Options{MaxWatches: 1})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	first, err := os.CreateTemp("", "gestalt-watcher-limit-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	firstPath := first.Name()
	if err := first.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(firstPath)
	})

	second, err := os.CreateTemp("", "gestalt-watcher-limit-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	secondPath := second.Name()
	if err := second.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(secondPath)
	})

	handle, err := watcher.Watch(firstPath, func(Event) {})
	if err != nil {
		t.Fatalf("watch first path: %v", err)
	}
	defer handle.Close()

	if _, err := watcher.Watch(secondPath, func(Event) {}); err == nil {
		t.Fatalf("expected max watches error")
	} else if !errors.Is(err, ErrMaxWatchesExceeded) {
		t.Fatalf("expected max watches error, got %v", err)
	}
}

func TestWatcherCleanupRemovesEmptyWatches(t *testing.T) {
	watcher, err := NewWithOptions(Options{MaxWatches: 2})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	file, err := os.CreateTemp("", "gestalt-watcher-cleanup-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	handle, err := watcher.Watch(path, func(Event) {})
	if err != nil {
		t.Fatalf("watch path: %v", err)
	}
	defer handle.Close()

	watcher.mutex.Lock()
	watcher.callbacks[path] = nil
	watcher.mutex.Unlock()

	watcher.cleanup()
	metrics := watcher.Metrics()
	if metrics.ActiveWatches != 0 {
		t.Fatalf("expected 0 active watches, got %d", metrics.ActiveWatches)
	}
}
