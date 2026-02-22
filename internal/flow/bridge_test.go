package flow

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	eventpkg "gestalt/internal/event"
	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
)

type fakeDispatcher struct {
	signalCh chan struct{}
	err      error
}

func (dispatcher *fakeDispatcher) Dispatch(ctx context.Context, request ActivityRequest) error {
	if dispatcher.signalCh != nil {
		select {
		case dispatcher.signalCh <- struct{}{}:
		default:
		}
	}
	if dispatcher.err != nil {
		return dispatcher.err
	}
	return nil
}

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
		"type":      "file-change",
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

func TestBridgeStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := NewFileRepository(filepath.Join(t.TempDir(), "automations.json"), nil)
	if err := repo.Save(Config{
		Version: ConfigVersion,
		Triggers: []EventTrigger{
			{ID: "t1", EventType: "file-change"},
		},
		BindingsByTriggerID: map[string][]ActivityBinding{
			"t1": {
				{ActivityID: "toast_notification", Config: map[string]any{"level": "info", "message_template": "hi"}},
			},
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	signals := make(chan struct{}, 10)
	service := NewService(repo, &fakeDispatcher{signalCh: signals, err: errors.New("boom")}, nil)
	bridge := NewBridge(BridgeOptions{
		Service:     service,
		WatcherBus:  eventpkg.NewBus[watcher.Event](context.Background(), eventpkg.BusOptions{}),
		ConfigBus:   eventpkg.NewBus[eventpkg.ConfigEvent](context.Background(), eventpkg.BusOptions{}),
		AgentBus:    eventpkg.NewBus[eventpkg.AgentEvent](context.Background(), eventpkg.BusOptions{}),
		TerminalBus: eventpkg.NewBus[eventpkg.TerminalEvent](context.Background(), eventpkg.BusOptions{}),
		WorkflowBus: eventpkg.NewBus[eventpkg.WorkflowEvent](context.Background(), eventpkg.BusOptions{}),
	})

	if err := bridge.Start(ctx); err != nil {
		t.Fatalf("start bridge: %v", err)
	}

	bridge.WatcherBus.Publish(watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      "README.md",
		Op:        fsnotify.Write,
		Timestamp: time.Now().UTC(),
	})
	select {
	case <-signals:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected signal before cancel")
	}

	cancel()
	time.Sleep(20 * time.Millisecond)

	bridge.WatcherBus.Publish(watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      "README.md",
		Op:        fsnotify.Write,
		Timestamp: time.Now().UTC(),
	})
	select {
	case <-signals:
		t.Fatalf("expected no signal after cancel")
	case <-time.After(100 * time.Millisecond):
	}
}
