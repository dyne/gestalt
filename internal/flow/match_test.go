package flow

import (
	"testing"
	"time"

	eventpkg "gestalt/internal/event"
	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
)

func TestMatchTriggerWatcherEvent(t *testing.T) {
	timestamp := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	event := watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      "README.md",
		Op:        fsnotify.Write,
		Timestamp: timestamp,
	}
	normalized := NormalizeEvent(event)
	trigger := EventTrigger{
		EventType: watcher.EventTypeFileChanged,
		Where: map[string]string{
			"path": "README.md",
			"op":   fsnotify.Write.String(),
		},
	}
	if !MatchTrigger(trigger, normalized) {
		t.Fatalf("expected watcher event to match trigger")
	}
}

func TestMatchTriggerWorkflowEventContext(t *testing.T) {
	event := eventpkg.WorkflowEvent{
		EventType:  "workflow_paused",
		WorkflowID: "workflow-1",
		SessionID:  "session-1",
		Context: map[string]any{
			"bell_context": "ding",
		},
		OccurredAt: time.Now().UTC(),
	}
	normalized := NormalizeEvent(event)
	trigger := EventTrigger{
		EventType: "workflow_paused",
		Where: map[string]string{
			"context.bell_context": "DING",
		},
	}
	if !MatchTrigger(trigger, normalized) {
		t.Fatalf("expected workflow event to match trigger")
	}
}

func TestMatchTriggerTerminalData(t *testing.T) {
	event := eventpkg.TerminalEvent{
		EventType:  "terminal_resized",
		TerminalID: "t1",
		Data: map[string]any{
			"cols": 80,
			"rows": 24,
		},
		OccurredAt: time.Now().UTC(),
	}
	normalized := NormalizeEvent(event)
	trigger := EventTrigger{
		EventType: "terminal_resized",
		Where: map[string]string{
			"data.cols": "80",
			"data.rows": "24",
		},
	}
	if !MatchTrigger(trigger, normalized) {
		t.Fatalf("expected terminal event to match trigger")
	}
}
