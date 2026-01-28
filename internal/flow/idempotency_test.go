package flow

import "testing"

func TestEventDeduper(t *testing.T) {
	deduper := NewEventDeduper(2)
	if deduper.Seen("alpha") {
		t.Fatalf("expected alpha to be new")
	}
	if !deduper.Seen("alpha") {
		t.Fatalf("expected alpha to be seen on second call")
	}
	if deduper.Seen("beta") {
		t.Fatalf("expected beta to be new")
	}
	if deduper.Seen("gamma") {
		t.Fatalf("expected gamma to be new")
	}
	if deduper.Seen("alpha") {
		t.Fatalf("expected alpha to be evicted after limit exceeded")
	}
}

func TestBuildEventIDStable(t *testing.T) {
	first := BuildEventID(map[string]string{
		"type":      "workflow_paused",
		"session":   "session-1",
		"timestamp": "2026-01-28T00:00:00Z",
	})
	second := BuildEventID(map[string]string{
		"timestamp": "2026-01-28T00:00:00Z",
		"session":   "session-1",
		"type":      "workflow_paused",
	})
	if first == "" || second == "" {
		t.Fatalf("expected non-empty event ids")
	}
	if first != second {
		t.Fatalf("expected stable event id, got %q and %q", first, second)
	}
}

func TestBuildEventIDEmpty(t *testing.T) {
	if BuildEventID(nil) != "" {
		t.Fatalf("expected empty event id for nil map")
	}
	if BuildEventID(map[string]string{}) != "" {
		t.Fatalf("expected empty event id for empty map")
	}
}

func TestBuildIdempotencyKey(t *testing.T) {
	key := BuildIdempotencyKey("event", "trigger", "activity")
	if key != "event/trigger/activity" {
		t.Fatalf("unexpected idempotency key: %q", key)
	}
	if BuildIdempotencyKey("", "", "") != "" {
		t.Fatalf("expected empty key when no parts provided")
	}
}

func TestHeartbeatGuards(t *testing.T) {
	if ShouldSkipSend(nil) {
		t.Fatalf("expected nil heartbeat to allow send")
	}
	if ShouldSkipWebhook(nil) {
		t.Fatalf("expected nil heartbeat to allow webhook")
	}
	if !ShouldSkipSend(&ActivityHeartbeat{Sent: true}) {
		t.Fatalf("expected send skip when sent flag set")
	}
	if !ShouldSkipWebhook(&ActivityHeartbeat{Posted: true}) {
		t.Fatalf("expected webhook skip when posted flag set")
	}
}
