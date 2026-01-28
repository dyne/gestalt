package flow

import (
	"testing"
	"time"

	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
)

func TestWatcherFilterAllows(t *testing.T) {
	filter := newWatcherFilter()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	event := watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      "README.md",
		Op:        fsnotify.Write,
		Timestamp: now,
	}

	if !filter.Allows(event, now) {
		t.Fatalf("expected watcher event to be allowed")
	}
	if filter.Allows(event, now.Add(100*time.Millisecond)) {
		t.Fatalf("expected duplicate watcher event to be deduped")
	}
	if !filter.Allows(event, now.Add(defaultWatcherDedupTTL+time.Millisecond)) {
		t.Fatalf("expected watcher event after dedupe window to be allowed")
	}
	if filter.Allows(watcher.Event{Type: watcher.EventTypeWatchError}, now) {
		t.Fatalf("expected watch error event to be filtered")
	}
}

func TestBuildEventSignal(t *testing.T) {
	fields := map[string]string{
		"type":      "file_changed",
		"path":      "README.md",
		"timestamp": "2026-01-01T12:00:00Z",
	}
	signal := BuildEventSignal(fields)
	if signal.EventID == "" {
		t.Fatalf("expected event id")
	}
	if signal.EventID != BuildEventID(fields) {
		t.Fatalf("expected deterministic event id")
	}
	if signal.Fields["path"] != "README.md" {
		t.Fatalf("expected fields to be preserved")
	}
}
