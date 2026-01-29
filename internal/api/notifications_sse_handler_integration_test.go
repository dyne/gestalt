package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"gestalt/internal/notification"
)

func TestNotificationsSSEStreamDeliversToast(t *testing.T) {
	server := newSSEEventsServer(t, &NotificationsSSEHandler{})
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/notifications/stream")
	if err != nil {
		t.Fatalf("get notification stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	notification.PublishToast("info", "hello")

	data := readSSEDataFrame(t, reader, time.Second)
	var payload notificationPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode notification payload: %v", err)
	}
	if payload.Type != notification.EventTypeToast {
		t.Fatalf("expected type %q, got %q", notification.EventTypeToast, payload.Type)
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
