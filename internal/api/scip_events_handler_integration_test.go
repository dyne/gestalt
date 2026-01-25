package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/event"

	"github.com/gorilla/websocket"
)

func TestSCIPEventsWebSocketStream(t *testing.T) {
	bus := event.NewBus[event.SCIPEvent](context.Background(), event.BusOptions{
		Name:        "scip_events_test",
		HistorySize: 4,
	})
	t.Cleanup(bus.Close)

	handler := &SCIPEventsHandler{Bus: bus}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/scip/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	timestamp := time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC)
	scipEvent := event.SCIPEvent{
		EventType:  "progress",
		Language:   "go",
		Message:    "indexing",
		OccurredAt: timestamp,
	}
	bus.Publish(scipEvent)

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var payload scipEventPayload
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload.Type != scipEvent.EventType {
		t.Fatalf("expected event type %q, got %q", scipEvent.EventType, payload.Type)
	}
	if payload.Language != scipEvent.Language {
		t.Fatalf("expected language %q, got %q", scipEvent.Language, payload.Language)
	}
	if payload.Message != scipEvent.Message {
		t.Fatalf("expected message %q, got %q", scipEvent.Message, payload.Message)
	}
	if !payload.Timestamp.Equal(scipEvent.OccurredAt) {
		t.Fatalf("expected timestamp %v, got %v", scipEvent.OccurredAt, payload.Timestamp)
	}
}
