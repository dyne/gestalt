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
		EventType:  "terminal-resized",
		TerminalID: "t1",
		Data: map[string]any{
			"cols": 80,
			"rows": 24,
		},
		OccurredAt: time.Now().UTC(),
	}
	normalized := NormalizeEvent(event)
	trigger := EventTrigger{
		EventType: "terminal-resized",
		Where: map[string]string{
			"data.cols": "80",
			"data.rows": "24",
		},
	}
	if !MatchTrigger(trigger, normalized) {
		t.Fatalf("expected terminal event to match trigger")
	}
}

func TestMatchBindings(t *testing.T) {
	config := Config{
		Version: ConfigVersion,
		Triggers: []EventTrigger{
			{ID: "t1", EventType: "file-change", Where: map[string]string{"path": "README.md"}},
			{ID: "t2", EventType: "workflow_paused"},
		},
		BindingsByTriggerID: map[string][]ActivityBinding{
			"t1": {
				{ActivityID: "toast_notification"},
			},
			"t2": {
				{ActivityID: "post_webhook"},
			},
		},
	}

	normalized := map[string]string{
		"type": "file-change",
		"path": "README.md",
	}

	matches := MatchBindings(config, normalized)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Trigger.ID != "t1" || matches[0].Binding.ActivityID != "toast_notification" {
		t.Fatalf("unexpected match: %#v", matches[0])
	}
}

func TestMatchTriggerSessionIDWildcard(t *testing.T) {
	normalized := map[string]string{
		"type":       "agent-turn-complete",
		"session.id": "Coder 3",
	}
	trigger := EventTrigger{
		EventType: "agent-turn-complete",
		Where: map[string]string{
			"session.id": "coder",
		},
	}
	if !MatchTrigger(trigger, normalized) {
		t.Fatalf("expected session id wildcard to match")
	}
}

func TestMatchTriggerSessionIDExact(t *testing.T) {
	normalized := map[string]string{
		"type":       "agent-turn-complete",
		"session.id": "coder 2",
	}
	trigger := EventTrigger{
		EventType: "agent-turn-complete",
		Where: map[string]string{
			"session.id": "coder 1",
		},
	}
	if MatchTrigger(trigger, normalized) {
		t.Fatalf("expected exact session id to not match different session")
	}
}

func TestMatchTriggerSessionIDEmptyAllowsAll(t *testing.T) {
	normalized := map[string]string{
		"type":       "agent-turn-complete",
		"session.id": "coder 5",
	}
	trigger := EventTrigger{
		EventType: "agent-turn-complete",
		Where: map[string]string{
			"session.id": "",
		},
	}
	if !MatchTrigger(trigger, normalized) {
		t.Fatalf("expected empty session id to match any session")
	}
}
