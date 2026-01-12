package watcher

import (
	"errors"
	"os"
	"path/filepath"
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

func TestWatcherDispatchesDirectoryEvent(t *testing.T) {
	watcher, err := NewWithOptions(Options{WatchDir: true})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	dir := t.TempDir()
	events := make(chan Event, 1)
	handle, err := watcher.Watch(dir, func(event Event) {
		select {
		case events <- event:
		default:
		}
	})
	if err != nil {
		t.Fatalf("watch dir: %v", err)
	}
	defer handle.Close()

	filePath := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	event, ok := waitForEvent(events)
	if !ok {
		t.Fatal("timed out waiting for directory event")
	}
	if event.Path != filePath {
		t.Fatalf("expected path %q, got %q", filePath, event.Path)
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

func TestWatcherDebounceCoalescesEvents(t *testing.T) {
	watcher, err := NewWithOptions(Options{Debounce: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	file, err := os.CreateTemp("", "gestalt-watcher-debounce-*")
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

	events := make(chan Event, 10)
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

	for i := 0; i < 3; i++ {
		if err := os.WriteFile(path, []byte("update"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	received := 0
	select {
	case <-events:
		received++
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced event")
	}

	time.Sleep(200 * time.Millisecond)
	for {
		select {
		case <-events:
			received++
		default:
			if received != 1 {
				t.Fatalf("expected 1 debounced event, got %d", received)
			}
			return
		}
	}
}

func TestWatcherInvalidPathReturnsError(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	invalidPath := filepath.Join(os.TempDir(), "gestalt-watcher-missing", "missing.txt")
	if _, err := watcher.Watch(invalidPath, func(Event) {}); err == nil {
		t.Fatalf("expected error for invalid path")
	}
}

func TestWatcherErrorHandlerAfterRetryLimit(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	done := make(chan error, 1)
	watcher.SetErrorHandler(func(err error) {
		done <- err
	})

	watcher.restartMutex.Lock()
	watcher.restartAttempts = maxRestartAttempts
	watcher.restartMutex.Unlock()

	watcher.handleError(errors.New("boom"))

	select {
	case err := <-done:
		if err == nil || err.Error() != "boom" {
			t.Fatalf("expected boom error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected error handler to be called")
	}
}
