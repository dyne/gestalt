package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"gestalt/internal/flow"
	"gestalt/internal/notify"
	"gestalt/internal/otel"
)

func TestNotificationsSSEStreamDeliversToast(t *testing.T) {
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	defer otel.SetActiveLogHub(previous)

	server := newSSEEventsServer(t, &NotificationsSSEHandler{})
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/notifications/stream")
	if err != nil {
		t.Fatalf("get notification stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	sink := notify.NewOTelSink(hub)
	event := notify.Event{
		Fields: map[string]string{
			"notify.type":  "toast",
			"type":         flow.CanonicalNotifyEventType("toast"),
			"notify.level": "info",
		},
		OccurredAt: time.Now().UTC(),
		Level:      "info",
		Message:    "hello",
	}
	if err := sink.Emit(context.Background(), event); err != nil {
		t.Fatalf("emit toast: %v", err)
	}

	data := readSSEDataFrame(t, reader, time.Second)
	var payload notificationPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode notification payload: %v", err)
	}
	if payload.Type != "toast" {
		t.Fatalf("expected type %q, got %q", "toast", payload.Type)
	}
	if payload.Level != "info" {
		t.Fatalf("expected level %q, got %q", "info", payload.Level)
	}
	if payload.Message != "hello" {
		t.Fatalf("expected message %q, got %q", "hello", payload.Message)
	}
	if payload.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}
