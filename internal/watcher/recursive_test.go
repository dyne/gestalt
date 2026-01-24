package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherRecursiveWatchDispatchesNestedEvent(t *testing.T) {
	watcher, err := NewWithOptions(Options{WatchDir: true, WatchRecursive: true})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}

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

	filePath := filepath.Join(nestedDir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	event, ok := waitForEvent(events)
	if !ok {
		t.Fatal("timed out waiting for recursive event")
	}
	if event.Path != filePath {
		t.Fatalf("expected path %q, got %q", filePath, event.Path)
	}
}

func TestWatchContextCancels(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	file, err := os.CreateTemp("", "gestalt-watcher-context-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	defer os.Remove(path)

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 1)
	handle, err := watcher.WatchContext(ctx, path, func(event Event) {
		select {
		case events <- event:
		default:
		}
	})
	if err != nil {
		t.Fatalf("watch context: %v", err)
	}
	defer handle.Close()

	cancel()

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		watcher.mutex.Lock()
		_, ok := watcher.callbacks[path]
		watcher.mutex.Unlock()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected watch to be removed after context cancel")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := os.WriteFile(path, []byte("update"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	select {
	case <-events:
		t.Fatal("unexpected event after cancel")
	case <-time.After(200 * time.Millisecond):
	}
}
