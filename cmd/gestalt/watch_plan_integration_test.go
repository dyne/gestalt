package main

import (
	"context"
	"os"
	"testing"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/watcher"
)

func TestWatchPlanFilePublishesEvents(t *testing.T) {
	file, err := os.CreateTemp("", "gestalt-plan-*")
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

	fsWatcher, err := watcher.New()
	if err != nil {
		t.Skipf("skipping watcher test (fsnotify unavailable): %v", err)
	}
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, nil)
	watchPlanFile(bus, fsWatcher, logger, path)

	events := make(chan watcher.Event, 1)
	subscription, cancel := bus.SubscribeFiltered(func(event watcher.Event) bool {
		return event.Type == watcher.EventTypeFileChanged && event.Path == path
	})
	defer cancel()
	go func() {
		for event := range subscription {
			select {
			case events <- event:
			default:
			}
		}
	}()

	if err := os.WriteFile(path, []byte("update"), 0600); err != nil {
		t.Fatalf("write plan file: %v", err)
	}

	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for plan event")
	}
}
