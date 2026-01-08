package watcher

import (
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
