package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/config"
	"gestalt/internal/event"

	"github.com/gorilla/websocket"
)

func TestConfigEventsWebSocketStream(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &ConfigEventsHandler{}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/config/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	timestamp := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	configEvent := event.ConfigEvent{
		EventType:  "config_extracted",
		ConfigType: "agent",
		Path:       "/config/agents/example.toml",
		ChangeType: "extracted",
		OccurredAt: timestamp,
	}

	bus := config.Bus()
	if bus == nil {
		t.Fatal("expected config bus")
	}
	bus.Publish(configEvent)

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var payload configEventPayload
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload.Type != configEvent.EventType {
		t.Fatalf("expected event type %q, got %q", configEvent.EventType, payload.Type)
	}
	if payload.ConfigType != configEvent.ConfigType {
		t.Fatalf("expected config type %q, got %q", configEvent.ConfigType, payload.ConfigType)
	}
	if payload.Path != configEvent.Path {
		t.Fatalf("expected path %q, got %q", configEvent.Path, payload.Path)
	}
	if payload.ChangeType != configEvent.ChangeType {
		t.Fatalf("expected change type %q, got %q", configEvent.ChangeType, payload.ChangeType)
	}
	if !payload.Timestamp.Equal(configEvent.OccurredAt) {
		t.Fatalf("expected timestamp %v, got %v", configEvent.OccurredAt, payload.Timestamp)
	}
}
