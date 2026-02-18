package notify

import (
	"context"
	"testing"
	"time"

	"gestalt/internal/otel"
)

func TestOTelSinkEmitsRecord(t *testing.T) {
	hub := otel.NewLogHub(time.Minute)
	sink := NewOTelSink(hub)
	eventTime := time.Now().UTC()
	err := sink.Emit(context.Background(), Event{
		Fields: map[string]string{
			"notify.type":    "plan-L1-wip",
			"session.id":     "session-1",
			"gestalt.source": "backend",
		},
		OccurredAt: eventTime,
		Level:      "info",
		Message:    "plan-L1-wip",
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	records := hub.SnapshotSince(time.Time{})
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	record := records[0]
	if record["severityText"] != "info" {
		t.Fatalf("expected severity info, got %v", record["severityText"])
	}
	body, ok := record["body"].(map[string]any)
	if !ok || body["stringValue"] != "plan-L1-wip" {
		t.Fatalf("unexpected body: %#v", record["body"])
	}
	if record["timeUnixNano"] == "" {
		t.Fatalf("expected timeUnixNano to be set")
	}
	attrs, ok := record["attributes"].([]any)
	if !ok || len(attrs) == 0 {
		t.Fatalf("expected attributes to be set")
	}
}
